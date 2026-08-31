#include <assert.h>
#include <errno.h>
#include <ifaddrs.h>
#include <poll.h>
#include <pthread.h>
#include <sched.h>
#include <signal.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>
#include <filesystem>
#include <iostream>
#include <map>
#include <sstream>
#include <string>
#include <mutex>

#include <sys/epoll.h>
#include <sys/ioctl.h>
#include <sys/mman.h>
#include <sys/select.h>
#include <sys/signalfd.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <sys/un.h>
#include <linux/if_link.h>

#include <base/ipc.h>
#include <shm/shm_wrapper.h>
#include <intf/intf.h>
#include <intf/intf_ebpf.h>

#include <runtime/app_if.h>
#include <runtime/defs.h>
#include <runtime/ebpf_if.h>
#include <runtime/tcp.h>
#include <runtime/homa.h>
#include <utils/utils.h>

#include "ebpf.h"
#include "trans_ebpf.h"
#include "nic.h"

#define TICK_US 1000

/* Unidx domain socket for application enrollment */
int uds_sockfd;
/* epoll fd for application enrollment */
static int epfd;
/* list of active applications */
static struct apps_info active_apps;
/* epoll fd for receving packets from control path XSKs */
static int xsk_epfd;
/* track key/value in BPF_MAP_TYPE_XSKMAP */
static std::map<int, int> tcp_monitor_xsk_map;
static std::map<int, int> homa_monitor_xsk_map;
/* available ports in [PORT_MIN, PORT_MAX] */
static std::list<uint16_t> available_ports;
/* track status of port */
static std::unordered_map<uint16_t, bool> port_is_available;
static pthread_t micro_kernel_thread;

std::set<uint16_t> rule_dst_ports;
std::set<uint16_t> rule_src_ports;

// micro_kernel.cc
extern class eTranNIC *etran_nic;

// ebpf.cc
extern class eTranEntrance *etran_entrance;
extern class eTranTCP *etran_tcp;
extern class eTranHoma *etran_homa;

// tcp.cc
extern uint64_t next_tcp_cc_to_tsc;

/* manage Unix domain socket */
static int init_uds_ctx(void);
void destroy_uds_ctx(int sockfd);

/* notify application to exit */
static void kill_app(int pid);

/* add/delete control path XSK fds to BPF_MAP_TYPE_XSKMAP 
 * note: each transport protocol has its own BPF_MAP_TYPE_XSKMAP
 */
static int register_xsk_map(int xsk_fd, int xsk_map_key, int proto);
static void unregister_xsk_map(int xsk_fd, int xsk_map_key, int proto);

/* For each packet redirected to control path XSK:
 * step1. use queue index to get the key in BPF_MAP_TYPE_XSKMAP
 * step2. use the key to get the XSK fd
*/
static int register_slow_path_map(unsigned int qid, int xsk_map_key, int proto);
static void unregister_slow_path_map(unsigned int qid, int proto);

/* manage RSS context for this application */
static int create_rss_context(struct app_ctx *actx, unsigned int nr_nic_queues, std::vector<unsigned int> &qids);
static void destroy_rss_context(struct app_ctx *actx);

/* manage application resources */
static int alloc_app_resources(struct register_request &req, int fd);
static void free_app_resources(struct app_ctx *actx);

/* called when microkernel exits */
static void destroy_all_apps(void);
/* called when application exits */
static void destroy_app(int fd);

/* find application context with fd */
static struct app_ctx *find_actx_with_fd(int fd);

/* accept and create context for new application */
static int accept_app(void);
/* process requests from application */
static int process_app_req(int fd, struct register_request *req);
/* reponse application's requests */
static void response_app_req(int app_fd, enum resp_type type);

/* poll events from Unix domain socket */
static int poll_uds(int timeout_ms);
/* poll events from lrpc channel */
static void poll_lrpc(void);
/* poll events from network */
static int poll_network(int timeout_ms);

/* dump ebpf maps */
static void dump_xsk_map(int proto);
static void dump_homa_port_map(void);
static void dump_homa_rpc_map(void);
static void dump_tcp_conn_map(void);

static void kill_app(int pid)
{
    kill(pid, SIGUSR1);
}

static int init_uds_ctx(void)
{
    int sockfd;
    struct sockaddr_un addr;
    mode_t old_mask;

    sockfd = socket(AF_UNIX, SOCK_STREAM, 0);
    if (sockfd < 0)
    {
        perror("socket failed\n");
        goto err;
    }

    memset(&addr, 0, sizeof(addr));
    addr.sun_family = AF_UNIX;
    strncpy(addr.sun_path, MICRO_KERNEL_SOCK_PATH.c_str(), sizeof(addr.sun_path) - 1);

    old_mask = umask(0);
    unlink(MICRO_KERNEL_SOCK_PATH.c_str());
    if (bind(sockfd, (struct sockaddr *)&addr, sizeof(struct sockaddr_un)) < 0)
    {
        perror("bind failed\n");
        goto err_close_fd;
    }
    umask(old_mask);

    if (listen(sockfd, MAX_SUPPORT_APP) < 0)
    {
        perror("listen failed\n");
        goto err_close_fd;
    }

    return sockfd;

err_close_fd:
    close(sockfd);
err:
    return -errno;
}

void destroy_uds_ctx(int sockfd)
{
    unlink(MICRO_KERNEL_SOCK_PATH.c_str());
    close(sockfd);
}

/* try to allocate port for application */
int alloc_port(uint16_t port)
{
    if (port < PORT_MIN || port > PORT_MAX)
        return -EINVAL;

    if (!port_is_available[port])
        return -EINVAL;

    available_ports.remove(port);

    port_is_available[port] = false;

    return 0;
}

/* try to allocate random port for application */
int alloc_port(void)
{
    uint16_t port;

    if (available_ports.empty())
        return -1;

    port = available_ports.front();
    available_ports.pop_front();
    assert(port_is_available[port]);
    port_is_available[port] = false;

    return port;
}

int free_port(uint16_t port)
{
    if (port < PORT_MIN || port > PORT_MAX)
        return -EINVAL;

    if (port_is_available[port])
        return -EINVAL;

    port_is_available[port] = true;
    available_ports.push_back(port);

    return 0;
}

int record_port(struct app_ctx *actx, uint16_t local_port, uint16_t remote_port)
{
    actx->ports.insert(local_port);

    if (actx->proto != IPPROTO_TCP)
        return 0;

    std::string cmd;
    std::string res;

    /* only support TCP */
    if (!remote_port && rule_dst_ports.find(local_port) == rule_dst_ports.end()){
        cmd = "ethtool -U " + etran_nic->_if_name + " flow-type tcp4 dst-port " + std::to_string(local_port) + " context " + std::to_string(actx->rss_ctx_id);
        rule_dst_ports.insert(local_port);
    }
    else if (rule_src_ports.find(remote_port) == rule_src_ports.end()){
        cmd = "ethtool -U " + etran_nic->_if_name + " flow-type tcp4 src-port " + std::to_string(remote_port) + " context " + std::to_string(actx->rss_ctx_id);
        rule_src_ports.insert(remote_port);
    }
    if (cmd.empty())
        return 0;
    if (!exec_cmd(cmd, res) || res.empty())
    {
        fprintf(stderr, "Failed to configure NIC flow director(%s)\n", cmd.c_str());
        return -1;
    }

    size_t pos = res.find("Added rule with ID ");
    if (pos != std::string::npos)
    {
        actx->flow_director_rules.push_back({std::stoi(res.substr(pos + 19)), remote_port ? remote_port : local_port, remote_port ? true : false});
    }
    else
    {
        fprintf(stderr, "The required pattern was not found.\n");
        return -1;
    }

    std::cout << cmd << std::endl;

    return 0;
}

int unrecord_port(struct app_ctx *actx, uint16_t port)
{
    actx->ports.erase(port);
    if (actx->proto != IPPROTO_TCP)
        return 0;
    for (auto it = actx->flow_director_rules.begin(); it != actx->flow_director_rules.end(); it++)
    {
        std::string cmd;
        std::string res;
        cmd = "ethtool -U " + etran_nic->_if_name + " delete " + std::to_string(it->id);
        if (it->source)
            rule_src_ports.erase(it->port);
        else
            rule_dst_ports.erase(it->port);
        std::cout << cmd << std::endl;
        exec_cmd(cmd);
    }

    actx->flow_director_rules.clear();

    return 0;
}

static void destroy_rss_context(struct app_ctx *actx)
{
    if (actx->rss_ctx_id == 0 || actx->proto != IPPROTO_TCP)
        return;
    std::string cmd;
    cmd = "ethtool -X " + etran_nic->_if_name + " context " + std::to_string(actx->rss_ctx_id) + " delete";
    if (exec_cmd(cmd))
    {
        printf("Destroy RSS context success: %d\n", actx->rss_ctx_id);
        actx->rss_ctx_id = 0;
    }
    else
        fprintf(stderr, "Failed to destroy RSS context: %d\n", actx->rss_ctx_id);
}

static int create_rss_context(struct app_ctx *actx, unsigned int nr_nic_queues, std::vector<unsigned int> &qids)
{
    std::string cmd;
    std::string res;
    int weight[MAX_NIC_QUEUES] = {0};

    // XXX: Homa doesn't support RSS, we can only support one Homa application with queue-level isolation
    if (actx->proto != IPPROTO_TCP)
    {
        cmd = "ethtool -X " + etran_nic->_if_name + " equal " + std::to_string(nr_nic_queues);
        return !exec_cmd(cmd);
    }

    for (unsigned int i = 0; i < nr_nic_queues; i++)
    {
        int qidx = qids[i];
        weight[qidx] = 1;
    }
    // convert weight to string like '1 1 1 0 1 0 0'
    std::string weight_str;
    for (unsigned int i = 0; i < MAX_NIC_QUEUES; i++)
    {
        weight_str += std::to_string(weight[i]);
        if (i != MAX_NIC_QUEUES - 1)
            weight_str += " ";
    }
    cmd = "ethtool -X " + etran_nic->_if_name + " context new weight " + weight_str;
    std::cout << cmd << std::endl;
    if (!exec_cmd(cmd, res) || res.empty())
    {
        fprintf(stderr, "Failed to configure NIC flow director\n");
        return -1;
    }
    size_t pos = res.find("New RSS context is ");
    if (pos != std::string::npos)
    {
        actx->rss_ctx_id = std::stoi(res.substr(pos + 19));
        printf("Created RSS context: %d\n", actx->rss_ctx_id);
    }
    else
    {
        fprintf(stderr, "The required pattern was not found.\n");
        return -1;
    }

    return 0;
}

static void unregister_xsk_map(int xsk_fd, int xsk_map_key, int proto)
{
    int xsk_map_fd;

    struct xdp_program *xdp_prog;

    if (proto == IPPROTO_HOMA)
        xdp_prog = etran_homa->_ebpf.xdp_prog;
    else
        xdp_prog = etran_tcp->_ebpf.xdp_prog;

    xsk_map_fd = bpf_map__fd(bpf_object__find_map_by_name(xdp_program__bpf_obj(xdp_prog), "xsks_map"));
    if (xsk_map_fd < 0)
    {
        fprintf(stderr, "ERROR: no xsk map found: %s\n", strerror(xsk_map_fd));
        return;
    }

    if (bpf_map_delete_elem(xsk_map_fd, &xsk_map_key))
    {
        fprintf(stderr, "ERROR: delete xsk map key %d failed\n", xsk_map_key);
        return;
    }
    if (proto == IPPROTO_HOMA)
        homa_monitor_xsk_map.erase(xsk_map_key);
    else
        tcp_monitor_xsk_map.erase(xsk_map_key);
    etran_nic->_available_xsk_keys.push_back(xsk_map_key);
}

static int register_xsk_map(int xsk_fd, int xsk_map_key, int proto)
{
    int xsk_map_fd;

    struct xdp_program *xdp_prog;

    if (proto == IPPROTO_HOMA)
        xdp_prog = etran_homa->_ebpf.xdp_prog;
    else
        xdp_prog = etran_tcp->_ebpf.xdp_prog;

    xsk_map_fd = bpf_map__fd(bpf_object__find_map_by_name(xdp_program__bpf_obj(xdp_prog), "xsks_map"));
    if (xsk_map_fd < 0)
    {
        fprintf(stderr, "ERROR: no xsk map found: %s\n", strerror(xsk_map_fd));
        return -EINVAL;
    }

    if (bpf_map_update_elem(xsk_map_fd, &xsk_map_key, &xsk_fd, BPF_NOEXIST) == EEXIST)
    {
        fprintf(stderr, "ERROR: xsk map key %d already exists\n", xsk_map_key);
        return -EEXIST;
    }

    if (proto == IPPROTO_HOMA)
        homa_monitor_xsk_map[xsk_map_key] = xsk_fd;
    else
        tcp_monitor_xsk_map[xsk_map_key] = xsk_fd;

    return 0;
}

static void free_app_resources(struct app_ctx *actx)
{
    if (actx->proto == IPPROTO_TCP)
    {
        free_tcp_resources(actx);
    }

    if (actx->proto == IPPROTO_HOMA)
    {
        free_homa_resources(actx);
    }

    destroy_rss_context(actx);

    for (unsigned int i = 0; i < actx->nr_nic_queues; i++)
    {
        unsigned int qid = actx->nic_qid[i];

        if (!etran_nic->_nic_queues[qid].xsk_info)
            return;

        epoll_ctl(xsk_epfd, EPOLL_CTL_DEL, xsk_socket__fd(etran_nic->_nic_queues[qid].xsk_info->xsk), nullptr);

        unregister_xsk_map(xsk_socket__fd(etran_nic->_nic_queues[qid].xsk_info->xsk), etran_nic->_nic_queues[qid].xsk_map_key, actx->proto);

        xsk_delete_socket(etran_nic->_nic_queues[qid].xsk_info);

        etran_nic->_nic_queues[qid].xsk_info = nullptr;
        etran_nic->_nic_queues[qid].bpw = nullptr;
        etran_nic->_nic_queues[qid].xsk_map_key = 0;

        unregister_slow_path_map(qid, actx->proto);

        etran_nic->_available_qids.push_back(qid);
    }

    for (unsigned int i = 0; i < actx->nr_app_threads; i++)
        for (unsigned int j = 0; j < actx->nr_nic_queues; j++)
        {
            unregister_xsk_map(actx->tctx[i].txrx_xsk_fd[j], actx->tctx[i].txrx_xsk_map_key[j], actx->proto);
            close(actx->tctx[i].txrx_xsk_fd[j]);
        }

    for (auto it = actx->ports.begin(); it != actx->ports.end(); it++)
        free_port(*it);

    actx->ports.clear();

    bp_free(&actx->bpw);

    for (unsigned int i = 0; i < actx->nr_app_threads; i++)
        if (actx->tctx[i].lrpc)
            shm_wrapper_destroy(actx->tctx[i].lrpc);
}

static void destroy_all_apps(void)
{
    // Notify all apps to exit
    for (auto it = active_apps.apps.begin(); it != active_apps.apps.end(); it++)
    {
        kill_app((*it)->pid);
    }

    // Wait for all apps to exit
    while (1)
    {
        static int max_wait_time = 1;
        sleep(1);
        if (--max_wait_time == 0)
        {
            fprintf(stderr, "Wait for app exit timeout\n");
            break;
        }
    }

    // Free all resources
    for (auto it = active_apps.apps.begin(); it != active_apps.apps.end(); it++)
    {
        if ((*it)->done)
            free_app_resources(*it);
        close((*it)->fd);
        delete (*it);
    }
    active_apps.apps.clear();
}

static void destroy_app(int fd)
{
    if (active_apps.nr_app == 0)
        return;
    auto it = active_apps.apps.begin();
    for (; it != active_apps.apps.end(); it++)
    {
        if ((*it)->fd == fd)
        {
            if ((*it)->done)
                free_app_resources(*it);
            close(fd);
            kill_app((*it)->pid);
            active_apps.nr_app--;
            printf("Destroy app_fd: %d\n", fd);
            break;
        }
    }
    if (it != active_apps.apps.end())
    {
        delete (*it);
        active_apps.apps.erase(it);
    }
}

static struct app_ctx *find_actx_with_fd(int fd)
{
    for (auto it = active_apps.apps.begin(); it != active_apps.apps.end(); it++)
    {
        if ((*it)->fd == fd)
            return *it;
    }
    return nullptr;
}

static void response_app_req(int app_fd, enum resp_type type)
{
    struct app_ctx *actx = find_actx_with_fd(app_fd);
    struct register_response resp = {};
    assert(actx);

    switch (type)
    {
    case REG_XSK_FD:
        resp.type = type;
        if (write_all(app_fd, &resp, sizeof(resp)) != sizeof(resp))
            return;
        // UMEM fds
        transfer_fds(app_fd, actx->umem_fd, actx->nr_nic_queues);
        // TX/RX XSKs
        for (unsigned int i = 0; i < actx->nr_app_threads; i++)
            transfer_fds(app_fd, actx->tctx[i].txrx_xsk_fd, actx->nr_nic_queues);
        break;
    case REG_EVENT_FD:
        resp.type = type;
        if (write_all(app_fd, &resp, sizeof(resp)) != sizeof(resp))
            return;
        for (unsigned int i = 0; i < actx->nr_app_threads; i++)
            transfer_fd(app_fd, actx->tctx[i].evfd);
        break;
    case REG_DONE:
        resp.shm_bp_size = actx->bpw.shm_bp->size;
        resp.shm_umem_size = actx->bpw.shm_umem->size;
        resp.shm_lrpc_size = actx->tctx[0].lrpc->size;
        resp.bp_params = actx->bpw.bp_params;
        resp.ifindex = if_nametoindex(etran_nic->_if_name.c_str());
        for (unsigned int i = 0; i < actx->nr_nic_queues; i++)
            resp.nic_qid[i] = actx->nic_qid[i];
    case REG_FAIL:
    case UNKNWON:
        resp.type = type;
        write_all(app_fd, &resp, sizeof(resp));
        break;
    default:
        break;
    }
}

static void unregister_slow_path_map(unsigned int qid, int proto)
{
    int slow_path_map_fd;
    struct xdp_program *xdp_prog;

    if (proto == IPPROTO_HOMA)
        xdp_prog = etran_homa->_ebpf.xdp_prog;
    else
        xdp_prog = etran_tcp->_ebpf.xdp_prog;

    slow_path_map_fd = bpf_map__fd(bpf_object__find_map_by_name(xdp_program__bpf_obj(xdp_prog), "slow_path_map"));

    if (slow_path_map_fd < 0)
    {
        fprintf(stderr, "ERROR: no slow path map found: %s\n", strerror(slow_path_map_fd));
        return;
    }

    struct slow_path_info spi = {0};

    spi.active = false;
    spi.sp_xsk_map_key = 0;

    if (bpf_map_update_elem(slow_path_map_fd, &qid, &spi, BPF_ANY))
    {
        fprintf(stderr, "ERROR: slow path map key %d error\n", qid);
        return;
    }
}

static int register_slow_path_map(unsigned int qid, int xsk_map_key, int proto)
{
    int slow_path_map_fd;
    struct xdp_program *xdp_prog;

    if (proto == IPPROTO_HOMA)
        xdp_prog = etran_homa->_ebpf.xdp_prog;
    else
        xdp_prog = etran_tcp->_ebpf.xdp_prog;

    slow_path_map_fd = bpf_map__fd(bpf_object__find_map_by_name(xdp_program__bpf_obj(xdp_prog), "slow_path_map"));

    if (slow_path_map_fd < 0)
    {
        fprintf(stderr, "ERROR: no slow path map found: %s\n", strerror(slow_path_map_fd));
        return -EINVAL;
    }

    struct slow_path_info spi = {0};

    spi.active = true;
    spi.sp_xsk_map_key = xsk_map_key;

    if (bpf_map_update_elem(slow_path_map_fd, &qid, &spi, BPF_ANY))
    {
        fprintf(stderr, "ERROR: slow path map key %d error\n", qid);
        return -EEXIST;
    }

    return 0;
}

static inline int init_lrpc_channels(struct app_ctx *actx, const char *lrpc_prefix)
{
    std::string shm_lrpc_name;
    uint64_t lrpc_size;
    char *k_recv_head_wb, *app_recv_head_wb;
    char *channel_1, *channel_2;

    for (unsigned int i = 0; i < actx->nr_app_threads; i++)
    {
        actx->tctx[i].evfd = eventfd(0, EFD_NONBLOCK);
        if (actx->tctx[i].evfd < 0)
        {
            fprintf(stderr, "eventfd failed\n");
            return -EIO;
        }
    }

    for (unsigned int i = 0; i < actx->nr_app_threads; i++)
    {
        // shm_lrpc_name = "LRPC_" + std::to_string(actx->pid) + "_" + std::to_string(i);
        shm_lrpc_name = lrpc_prefix + std::to_string(actx->pid) + "_" + std::to_string(i);
        lrpc_size = (sizeof(struct head_wb) + sizeof(lrpc_msg) * LRPC_CHANNEL_SIZE) * 2;
        actx->tctx[i].lrpc = shm_wrapper_create(shm_lrpc_name, lrpc_size);
        if (!actx->tctx[i].lrpc)
        {
            fprintf(stderr, "shm_wrapper_create failed\n");
            return -ENOMEM;
        }

        k_recv_head_wb = (char *)actx->tctx[i].lrpc->addr;
        app_recv_head_wb = k_recv_head_wb + sizeof(struct head_wb);

        channel_1 = app_recv_head_wb + sizeof(struct head_wb);
        channel_2 = channel_1 + sizeof(lrpc_msg) * LRPC_CHANNEL_SIZE;

        if (lrpc_init_out(&actx->tctx[i].kernel_out, (lrpc_msg *)channel_1, LRPC_CHANNEL_SIZE,
                          (uint32_t *)app_recv_head_wb))
        {
            goto err;
        }

        if (lrpc_init_in(&actx->tctx[i].kernel_in, (lrpc_msg *)channel_2, LRPC_CHANNEL_SIZE,
                         (uint32_t *)k_recv_head_wb))
        {
            goto err;
        }

        actx->tctx[i].actx = actx;
    }

    return 0;

err:
    for (unsigned int i = 0; i < actx->nr_app_threads; i++)
        shm_wrapper_destroy(actx->tctx[i].lrpc);

    return -ENOMEM;
}

// step1: create a buffer pool and UMEM
// step2: create slow-path XSKs and rings(rx,tx,fill,comp), bind them to the UMEM 
// step3: create TX/RX XSKs (nr_nic_queues * nr_app_threads) step4:
// register XSK fd to the XSK map
static int alloc_app_resources(struct register_request &req, int fd)
{
    int xsk_fd;
    struct app_ctx *actx;
    std::vector<unsigned int> qids;
    struct xdp_options xdp_opts = {0};
    socklen_t optlen = sizeof(xdp_opts);
    int tran_idx;

    if (req.nr_nic_queues > etran_nic->_available_qids.size() || req.nr_app_threads > MAX_APP_THREADS)
    {
        fprintf(stderr,
                "alloc_app_resources: too many queues or threads, "
                "etran_nic->_available_qids.size()=%zu\n",
                etran_nic->_available_qids.size());
        return -E2BIG;
    }

    if (req.nr_nic_queues * req.nr_app_threads + req.nr_nic_queues > etran_nic->_available_xsk_keys.size())
    {
        fprintf(stderr,
                "alloc_app_resources: too many XSK keys, "
                "etran_nic->_available_xsk_keys.size()=%zu\n",
                etran_nic->_available_xsk_keys.size());
        return -E2BIG;
    }

    if (!SUPPORT_PROTO(req.proto))
    {
        fprintf(stderr, "alloc_app_resources: unsupported proto %d\n", req.proto);
        return -EINVAL;
    }

    actx = find_actx_with_fd(fd);

    if (!actx)
        return -ENOENT;

    actx->proto = req.proto;
    actx->nr_nic_queues = req.nr_nic_queues;
    actx->nr_app_threads = req.nr_app_threads;

    if (init_lrpc_channels(actx, SHM_LRPC_PREFIX))
    {
        fprintf(stderr, "init_lrpc_channels failed\n");
        goto err;
    }
    printf("init_lrpc_channels success\n");

    if (bp_init(&actx->bpw, actx->pid, SHM_BP_PREFIX, SHM_UMEM_PREFIX))
    {
        fprintf(stderr, "bp_init failed\n");
        goto err;
    }
    printf("bp_init success\n");

    for (unsigned int i = 0; i < req.nr_nic_queues; i++)
    {
        unsigned int qid = etran_nic->_available_qids.front();
        etran_nic->_available_qids.erase(etran_nic->_available_qids.begin());
        qids.push_back(qid);
    }

    for (unsigned int i = 0; i < req.nr_nic_queues; i++)
    {
        unsigned int qid = qids[i];

        etran_nic->_nic_queues[qid].actx = actx;
        etran_nic->_nic_queues[qid].bpw = &actx->bpw;
        etran_nic->_nic_queues[qid].xsk_info = xsk_configure_socket(&etran_nic->_nic_queues[qid], actx->proto);

        if (!etran_nic->_nic_queues[qid].xsk_info)
        {
            fprintf(stderr, "xsk_configure_socket failed for Queue#%d\n", qid);
            goto err;
        }

        actx->nic_qid[i] = qid;

        actx->qid2idx[qid] = i;

        actx->umem_fd[i] = xsk_socket__fd(etran_nic->_nic_queues[qid].xsk_info->xsk);

        epoll_event ev = {};
        ev.events = EPOLLIN | EPOLLERR;
        ev.data.fd = xsk_socket__fd(etran_nic->_nic_queues[qid].xsk_info->xsk);
        if (epoll_ctl(xsk_epfd, EPOLL_CTL_ADD, xsk_socket__fd(etran_nic->_nic_queues[qid].xsk_info->xsk), &ev))
        {
            fprintf(stderr, "epoll_ctl failed\n");
            goto err;
        }
        /* Get UMEM id for this application */
        if (getsockopt(xsk_socket__fd(etran_nic->_nic_queues[qid].xsk_info->xsk), SOL_XDP, XDP_OPTIONS, &xdp_opts, &optlen))
            goto err;
        actx->umem_id = xdp_opts.umem_id;
    }
    printf("init etran_nic->_nic_queues success\n");

    // TX/RX XSKs
    for (unsigned int i = 0; i < req.nr_app_threads; i++)
    {
        for (unsigned int j = 0; j < req.nr_nic_queues; j++)
        {
            xsk_fd = socket(AF_XDP, SOCK_RAW, 0);
            if (xsk_fd < 0)
                goto err;

            if (setsockopt(xsk_fd, SOL_SOCKET, SO_BINDTODEVICE, etran_entrance->_ebpf.if_name.c_str(),
                           strlen(etran_entrance->_ebpf.if_name.c_str())))
            {
                fprintf(stderr, "setsockopt SO_BINDTODEVICE failed\n");
                goto err;
            }

            if (etran_nic->_socket_busy_poll) {
                apply_socket_busy_poll(xsk_fd);
            }

            actx->tctx[i].txrx_xsk_fd[j] = xsk_fd;
        }
    }
    printf("init TX/RX XSKs success\n");

    if (actx->proto == IPPROTO_TCP)
        tran_idx = 0;
    else if (actx->proto == IPPROTO_HOMA)
        tran_idx = 1;
    if (bpf_map_update_elem(etran_entrance->_umem_id_tran_map_fd, &actx->umem_id, &tran_idx, BPF_ANY))
    {
        goto err;
    }
    printf("init UMEM and transport mapping success\n");

    for (unsigned int i = 0; i < req.nr_nic_queues; i++)
    {
        unsigned int qid = qids[i];
        etran_nic->_nic_queues[qid].xsk_map_key = etran_nic->_available_xsk_keys.front();

        if (register_xsk_map(xsk_socket__fd(etran_nic->_nic_queues[qid].xsk_info->xsk), etran_nic->_nic_queues[qid].xsk_map_key, actx->proto))
            goto err;

        if (register_slow_path_map(qid, etran_nic->_nic_queues[qid].xsk_map_key, actx->proto))
            goto err;

        etran_nic->_available_xsk_keys.erase(etran_nic->_available_xsk_keys.begin());
    }
    printf("register XSK map for slowpath success\n");

    for (unsigned int i = 0; i < req.nr_app_threads; i++)
    {
        for (unsigned int j = 0; j < req.nr_nic_queues; j++)
        {
            actx->tctx[i].txrx_xsk_map_key[j] = etran_nic->_available_xsk_keys.front();
            if (register_xsk_map(actx->tctx[i].txrx_xsk_fd[j], actx->tctx[i].txrx_xsk_map_key[j], actx->proto))
                goto err;
            etran_nic->_available_xsk_keys.erase(etran_nic->_available_xsk_keys.begin());
        }
    }
    printf("register XSK map for application success\n");
    printf("req.nr_app_threads(%u), req.nr_nic_queues(%u)\n", req.nr_app_threads, req.nr_nic_queues);
    if (create_rss_context(actx, req.nr_nic_queues, qids))
        goto err;

    if (thread_bcache_create(&actx->bpw, &actx->iobuffer))
        goto err;

    actx->done = true;

    return 0;

err:
    destroy_rss_context(actx);

    for (unsigned int i = 0; i < req.nr_app_threads; i++)
        for (unsigned int j = 0; j < req.nr_nic_queues; j++)
            if (actx->tctx[i].txrx_xsk_fd[j] > 0)
                close(actx->tctx[i].txrx_xsk_fd[j]);

    for (unsigned int i = 0; i < req.nr_nic_queues; i++)
    {
        unsigned int qid = qids[i];
        if (etran_nic->_nic_queues[qid].xsk_info)
            xsk_delete_socket(etran_nic->_nic_queues[qid].xsk_info);
        etran_nic->_nic_queues[qid].actx = nullptr;
        etran_nic->_nic_queues[qid].xsk_info = nullptr;
        etran_nic->_nic_queues[qid].bpw = nullptr;
        etran_nic->_nic_queues[qid].xsk_map_key = 0;
        etran_nic->_available_qids.push_back(qid);
    }

    bp_free(&actx->bpw);

    for (unsigned int i = 0; i < req.nr_app_threads; i++)
        if (actx->tctx[i].lrpc)
            shm_wrapper_destroy(actx->tctx[i].lrpc);

    return -ENOMEM;
}

static int process_app_req(int fd, struct register_request *req)
{
    struct msghdr message;
    struct iovec iov[1];

    memset(&message, 0, sizeof(message));
    memset(req, 0, sizeof(*req));

    iov[0].iov_base = req;
    iov[0].iov_len = sizeof(*req);

    message.msg_iov = iov;
    message.msg_iovlen = 1;

    if (recvmsg(fd, &message, 0) < 0)
        return -1;

    return 0;
}

static int accept_app(void)
{
    int new_fd;
    struct app_ctx *app = nullptr;
    struct ucred cred;
    socklen_t ucred_len = sizeof(struct ucred);

    new_fd = accept(uds_sockfd, nullptr, nullptr);
    if (new_fd < 0)
        return -1;

    if (active_apps.nr_app > MAX_SUPPORT_APP)
        goto err;

    app = new app_ctx();
    if (!app)
        goto err;

    if (getsockopt(new_fd, SOL_SOCKET, SO_PEERCRED, &cred, &ucred_len))
    {
        perror("getsockopt");
        goto err;
    }

    app->pid = cred.pid;
    printf("app->pid = %d\n", app->pid);

    app->done = false;
    app->fd = new_fd;

    active_apps.apps.push_back(app);
    active_apps.nr_app++;

    return new_fd;

err:
    if (app)
        delete app;
    close(new_fd);
    return -1;
}

int ctx_init(void)
{
    xsk_epfd = epoll_create1(0);
    if (xsk_epfd < 0)
    {
        perror("epoll_create1 failed for xsk_epfd");
        return -1;
    }

    for (uint16_t i = PORT_MIN; i <= PORT_MAX; i++)
    {
        available_ports.push_back(i);
        port_is_available[i] = true;
    }

    return 0;
}

static int poll_uds(int timeout_ms)
{
    struct epoll_event ev, events[MAX_SUPPORT_APP + 1];
    int nfds;
    int new_fd;
    struct register_request req;

    nfds = epoll_wait(epfd, events, MAX_SUPPORT_APP + 1, timeout_ms);

    if (nfds < 0)
        return -1;

    for (int i = 0; i < nfds; i++)
    {
        if (events[i].data.fd == uds_sockfd)
        {
            if (events[i].events & EPOLLERR)
            {
                perror("uds_sockfd error");
                return -1;
            }
            new_fd = accept_app();
            if (new_fd < 0)
            {
                perror("accept_app");
                continue;
            }
            ev.events = EPOLLIN | EPOLLERR | EPOLLHUP;
            ev.data.fd = new_fd;
            if (epoll_ctl(epfd, EPOLL_CTL_ADD, new_fd, &ev) < 0)
            {
                perror("epoll_ctl");
                continue;
            }
            printf("Accepted new app_fd: %d\n", new_fd);
        }
        else
        {
            if (events[i].events & EPOLLHUP || events[i].events & EPOLLERR)
            {
                epoll_ctl(epfd, EPOLL_CTL_DEL, events[i].data.fd, NULL);
                destroy_app(events[i].data.fd);
                continue;
            }
            else if (events[i].events & EPOLLIN)
            {
                if (process_app_req(events[i].data.fd, &req))
                {
                    epoll_ctl(epfd, EPOLL_CTL_DEL, events[i].data.fd, NULL);
                    destroy_app(events[i].data.fd);
                    continue;
                }
                switch (req.type)
                {
                case REG:
                    printf("Receive REG command for app_fd:%d\n", events[i].data.fd);
                    if (alloc_app_resources(req, events[i].data.fd))
                    {
                        fprintf(stderr, "alloc_app_resources failed\n");
                        response_app_req(events[i].data.fd, REG_FAIL);
                    }
                    else
                    {
                        response_app_req(events[i].data.fd, REG_XSK_FD);
                        response_app_req(events[i].data.fd, REG_EVENT_FD);
                        response_app_req(events[i].data.fd, REG_DONE);
                    }
                    break;
                case UNREG:
                    printf("Receive UNREG command for app_fd:%d\n", events[i].data.fd);
                    destroy_app(events[i].data.fd);
                    break;
                default:
                    printf("Receive UNKNWON command for app_fd:%d\n", events[i].data.fd);
                    response_app_req(events[i].data.fd, UNKNWON);
                    break;
                }
            }
        }
    }
    return 0;
}

static void *control_loop(void *arg)
{
    struct epoll_event ev;
    uint64_t tcp_s, tcp_e, homa_next_tsc = 0;

    pthread_cleanup_push((void (*)(void *))destroy_all_apps, nullptr);

    epfd = epoll_create1(0);
    if (epfd < 0)
    {
        perror("epoll_create1");
        exit(EXIT_FAILURE);
    }

    ev.events = EPOLLIN | EPOLLERR;
    ev.data.fd = uds_sockfd;
    if (epoll_ctl(epfd, EPOLL_CTL_ADD, uds_sockfd, &ev) < 0)
    {
        perror("epoll_ctl");
        exit(EXIT_FAILURE);
    }
    homa_next_tsc = get_cycles() + us_to_cycles(1000);
    
    for (;;)
    {
        /* poll unix domain socket */
        poll_uds(enrollment_to_ms);

        /* poll lrpc channels */
        poll_lrpc();

        /* poll network packets */
        poll_network(network_to_ms);
        
        /* poll tcp handshake events */
        poll_tcp_handshake_events();

        /* poll tcp congestion control and timeout events */
        tcp_s = get_cycles();
        poll_tcp_cc_to();
        tcp_e = get_cycles();

        /* poll homa timeout events */
        if (get_cycles() >= homa_next_tsc)
        {
            poll_homa_to();
            homa_next_tsc = get_cycles() + us_to_cycles(TICK_US);
        }

        /* decide how long to block */
        uint64_t t = cycles_to_us(tcp_e - tcp_s);
        uint64_t sleep_time = next_tcp_cc_to_tsc == UINT64_MAX ? TICK_US: CC_INTERVAL_US;
        if (t < sleep_time)
        {
            struct timespec ts = {
                .tv_sec = 0,
                .tv_nsec = (long int)(sleep_time - t) * 1000,
            };
            clock_nanosleep(CLOCK_MONOTONIC, 0, &ts, nullptr);
        }
    }

    pthread_cleanup_pop(1);

    return nullptr;
}

int thread_init(void)
{
    cpu_set_t cpu_set;
    uds_sockfd = init_uds_ctx();

    if (uds_sockfd < 0)
    {
        fprintf(stderr, "thread_init:init_uds_ctx failed\n");
        return -1;
    }

    if (pthread_create(&micro_kernel_thread, nullptr, control_loop, nullptr) != 0)
    {
        fprintf(stderr, "thread_init:pthread_create failed\n");
        return -1;
    }
    CPU_ZERO(&cpu_set);
    CPU_SET(CP_CPU, &cpu_set);
    pthread_setaffinity_np(micro_kernel_thread, sizeof(cpu_set_t), &cpu_set);

    return 0;
}

void wait_thread(void)
{
    pthread_join(micro_kernel_thread, nullptr);
    destroy_uds_ctx(uds_sockfd);
}

void shutdown_thread(void)
{
    pthread_cancel(micro_kernel_thread);
}

static void dump_homa_rpc_map(void)
{
    int map_fd;
    bool empty = true;

    map_fd = etran_homa->_homa_rpc_fd;

    if (map_fd < 0)
    {
        fprintf(stderr, "ERROR: no map found: %s\n", strerror(map_fd));
        return;
    }

    empty = true;

    struct rpc_key_t key = {0}, next_key = {0};
    struct rpc_state value;
    std::cout << "\n===========================HOMA RPCs===========================\n";

    while (bpf_map_get_next_key(map_fd, &key, &next_key) == 0)
    {
        if (bpf_map_lookup_elem(map_fd, &next_key, &value) == 0)
        {
            if (value.state == BPF_RPC_INCOMING || value.state == BPF_RPC_IN_SERVICE) {
                printf("RPC#%llu "
                "state: %d "
                "length: %u "
                "incoming: %llu "
                "bytes_remaining: %llu\n"
                ,
                value.id, value.state, value.message_length, value.cc.incoming, value.cc.bytes_remaining);
                empty = true;
            } else if (value.state == BPF_RPC_OUTGOING) {
                printf("RPC#%llu "
                "state: %d "
                "length: %u "
                "granted: %llu "
                "next_xmit_offset: %llu\n"
                ,
                value.id, value.state, value.message_length, value.cc.granted, value.next_xmit_offset);
            } else if (value.state == BPF_RPC_DEAD) {
                printf("RPC is dead\n");
            }
            empty = false;
        }
        key = next_key;
    } 

    if (empty)
        std::cout << "Empty\n";
    std::cout << "===========================HOMA RPCs===========================\n";
}

static void dump_homa_port_map(void)
{
    int map_fd;
    bool empty = true;

    map_fd = etran_homa->_homa_port_tbl_fd;

    if (map_fd < 0)
    {
        fprintf(stderr, "ERROR: no map found: %s\n", strerror(map_fd));
        return;
    }

    empty = true;

    uint16_t key = {0}, next_key = {0};
    struct target_xsk value;
    std::cout << "\n===========================HOMA Ports===========================\n";

    while (bpf_map_get_next_key(map_fd, &key, &next_key) == 0)
    {
        if (bpf_map_lookup_elem(map_fd, &next_key, &value) == 0)
        {
            std::cout << "port: " << next_key << " ";
            for (int i = 0; i < MAX_CPU; i++)
            {
                std::cout << value.xsk_map_idx[i] << " ";
            }
            std::cout << std::endl;
            empty = false;
        }
        key = next_key;
    }

    if (empty)
        std::cout << "Empty\n";
    std::cout << "===========================HOMA Ports===========================\n";
}

static void dump_tcp_conn_map(void)
{
    int map_fd;
    bool empty = true;

    map_fd = etran_tcp->_tcp_connection_map_fd;

    if (map_fd < 0)
    {
        fprintf(stderr, "ERROR: no map found: %s\n", strerror(map_fd));
        return;
    }

    empty = true;
    struct ebpf_flow_tuple key = {0}, next_key = {0};
    struct bpf_tcp_conn value;
    std::cout << "\n===========================TCP Connections===========================\n";
    while (bpf_map_get_next_key(map_fd, &key, &next_key) == 0)
    {
        if (bpf_map_lookup_elem(map_fd, &next_key, &value) == 0)
        {
            std::cout << "bpf_tcp_conn: \n";
            std::cout << "local_ip: " << value.local_ip << " ";
            std::cout << "remote_ip: " << value.remote_ip << " ";
            std::cout << "local_port: " << value.local_port << " ";
            std::cout << "remote_port: " << value.remote_port << " ";
            printf("\n==============================================================\n");
            empty = false;
        }
        key = next_key;
    }
    if (empty)
        std::cout << "Empty\n";
    std::cout << "===========================TCP Connections===========================\n";
}

static void dump_xsk_map(int proto)
{
    std::map<int, int> *monitor_xsk_map;
    if (proto == IPPROTO_HOMA)
        monitor_xsk_map = &homa_monitor_xsk_map;
    else
        monitor_xsk_map = &tcp_monitor_xsk_map;
    printf("\n===========================XSK MAP(%s)===========================\n", proto == IPPROTO_HOMA ? "HOMA" : "TCP");
    if (monitor_xsk_map->empty())
    {
        std::cout << "Empty\n";
        printf("===========================XSK MAP(%s)===========================\n", proto == IPPROTO_HOMA ? "HOMA" : "TCP");
        return;
    }

    for (auto it = monitor_xsk_map->begin(); it != monitor_xsk_map->end(); it++)
    {
        std::cout << "key: " << std::setw(5) << it->first << " |value: " << it->second << std::endl;
    }
    printf("===========================XSK MAP(%s)===========================\n", proto == IPPROTO_HOMA ? "HOMA" : "TCP");
}

void dump_bpf_maps(void)
{
    dump_xsk_map(IPPROTO_HOMA);
    dump_xsk_map(IPPROTO_TCP);
    dump_tcp_conn_map();
    dump_homa_port_map();
    dump_homa_rpc_map();
}

void dump_ring_stats(void)
{
    for (unsigned int i = 0; i < etran_nic->_num_queues; i++)
    {
        if (!etran_nic->_nic_queues[i].xsk_info)
            continue;
        get_xsk_ring_stats(etran_nic->_nic_queues[i].xsk_info);
        dump_xsk_ring_stats(etran_nic->_nic_queues[i].xsk_info, std::to_string(i));
    }
}

void kick_napi(void)
{
    for (unsigned int i = 0; i < etran_nic->_num_queues; i++)
    {
        if (!etran_nic->_nic_queues[i].xsk_info)
            continue;
        kick_tx(etran_nic->_nic_queues[i].xsk_info);
        kick_fq(etran_nic->_nic_queues[i].xsk_info);
    }
}

static void process_cmd(struct app_ctx_per_thread *tctx, lrpc_msg *msg_in)
{
    switch (msg_in->cmd)
    {
    /***************** TCP *****************/
    case APPOUT_TCP_OPEN:
    case APPOUT_TCP_BIND:
    case APPOUT_TCP_LISTEN:
    case APPOUT_TCP_ACCEPT:
    case APPOUT_TCP_CLOSE:
        process_tcp_cmd(tctx, msg_in);
        break;
    /***************** Homa *****************/
    case APPOUT_HOMA_BIND:
    case APPOUT_HOMA_CLOSE:
        process_homa_cmd(tctx, msg_in);
        break;
    default:
        printf("Unknown command %ld\n", msg_in->cmd);
    }
}

static void poll_lrpc(void)
{
    struct app_ctx *actx;
    lrpc_msg msg = {0};

    for (auto it = active_apps.apps.begin(); it != active_apps.apps.end(); it++)
    {
        actx = *it;
        if (!actx->done)
            continue;

        for (unsigned int i = 0; i < actx->nr_app_threads; i++)
        {
            if (lrpc_empty(&actx->tctx[i].kernel_in))
                continue;
            lrpc_recv(&actx->tctx[i].kernel_in, &msg);
            process_cmd(&actx->tctx[i], &msg);
        }
    }
}

static struct nic_queue_info *find_nicq_with_fd(int fd)
{
    for (unsigned int i = 0; i < etran_nic->_num_queues; i++)
    {
        if (etran_nic->_nic_queues[i].xsk_info && xsk_socket__fd(etran_nic->_nic_queues[i].xsk_info->xsk) == fd) {
            return &etran_nic->_nic_queues[i];
        }
    }
    return nullptr;
}

static void process_packet(int xsk_fd)
{
    unsigned int rcvd = 0;
    unsigned int idx_rx = 0;
    struct thread_bcache *bc;
    struct app_ctx *actx;
    struct nic_queue_info *nicq = find_nicq_with_fd(xsk_fd);
    if (!nicq)
        return;

    bc = &nicq->actx->iobuffer;
    actx = nicq->actx;

    struct xsk_socket_info *xsk_info = nicq->xsk_info;

    rcvd = xsk_ring_cons__peek(&xsk_info->rx, IO_BATCH_SIZE, &idx_rx);
    if (unlikely(!rcvd))
        return;

    for (unsigned int i = 0; i < rcvd; i++)
    {
        const struct xdp_desc *desc = xsk_ring_cons__rx_desc(&xsk_info->rx, idx_rx++);
        uint64_t addr = desc->addr;

        addr = xsk_umem__add_offset_to_addr(addr);
        char *pkt = reinterpret_cast<char *>(xsk_umem__get_data(xsk_info->umem_area, addr));

        // this packet is recevied from which queue
        uint32_t qid = rxmeta_qid(pkt);
        etran_nic->_nic_queues[qid].xsk_info->needfill++;
#ifdef DEBUG_TCP
        fprintf(stdout, "Receive from NAPI ID: %d\n", qid);
#endif
        // fprintf(stdout, "Receive from NAPI ID: %d\n", qid);
        struct eth_hdr *eth = (struct eth_hdr *)pkt;
        struct ip_hdr *ip = (struct ip_hdr *)(eth + 1);
        if (ip->proto == IPPROTO_TCP)
        {
            tcp_packet(nicq->actx, (struct pkt_tcp *)pkt, qid);
        }
        else
        {
            fprintf(stderr, "Unknown protocol %d, pktlen=%u\n", ip->proto, desc->len);
        }

        thread_bcache_prod(bc, addr);
    }

    xsk_ring_cons__release(&xsk_info->rx, rcvd);

    // replenish fill ring
    for (unsigned int i = 0; i < actx->nr_nic_queues; i++)
    {
        int qid = actx->nic_qid[i];
        if (!etran_nic->_nic_queues[qid].xsk_info->needfill)
            continue;
        if (!spin_lock_try(&actx->bpw.bp->fq_lock[qid]))
            continue;

        struct xsk_ring_prod *fq = &actx->bpw.bp->fq[qid];
        unsigned int idx = 0;

        if (xsk_ring_prod__reserve(fq, etran_nic->_nic_queues[qid].xsk_info->needfill, &idx) < etran_nic->_nic_queues[qid].xsk_info->needfill)
        {
            /* xxx */
            continue;
        }

        for (unsigned int j = 0; j < etran_nic->_nic_queues[qid].xsk_info->needfill; j++)
        {
            assert(thread_bcache_check(bc, 1) == 1);
            *xsk_ring_prod__fill_addr(fq, idx++) = thread_bcache_cons(bc);
        }
        xsk_ring_prod__submit(fq, etran_nic->_nic_queues[qid].xsk_info->needfill);

        etran_nic->_nic_queues[qid].xsk_info->needfill = 0;

        spin_unlock(&actx->bpw.bp->fq_lock[qid]);
    }
}

static int poll_network(int timeout_ms)
{
    /* traverse all NIC queues */
    for (unsigned int i = 0; i < etran_nic->_num_queues; i++)
    {
        if (!etran_nic->_nic_queues[i].xsk_info)
            continue;

        // reclaim completion buffers
        if (etran_nic->_nic_queues[i].xsk_info->outstanding)
        {
            struct app_ctx *actx = etran_nic->_nic_queues[i].actx;
            struct thread_bcache *bc = &actx->iobuffer;
            // don't compete with fastpath
            if (!spin_lock_try(&actx->bpw.bp->cq_lock[i]))
                continue;
            struct xsk_ring_cons *cq = &actx->bpw.bp->cq[i];
            unsigned int idx_cq = 0;
            unsigned int rcvd = xsk_ring_cons__peek(cq, etran_nic->_nic_queues[i].xsk_info->outstanding, &idx_cq);
            for (unsigned int j = 0; j < rcvd; j++)
            {
                uint64_t addr = *xsk_ring_cons__comp_addr(cq, idx_cq++);
                thread_bcache_prod(bc, addr);
            }
            if (rcvd)
            {
                xsk_ring_cons__release(cq, rcvd);
                etran_nic->_nic_queues[i].xsk_info->outstanding -= rcvd;
            }
            spin_unlock(&actx->bpw.bp->cq_lock[i]);
        }
    }

    epoll_event events[MAX_NIC_QUEUES];
    int nfds = epoll_wait(xsk_epfd, events, MAX_NIC_QUEUES, timeout_ms);
    if (!nfds)
        return 0;
    for (int i = 0; i < nfds; i++)
    {
        if (events[i].events & EPOLLERR)
        {
            fprintf(stderr, "EPOLLERR\n");
            continue;
        }
        if (events[i].events & EPOLLIN)
        {
            process_packet(events[i].data.fd);
        }
    }

    return 0;
}