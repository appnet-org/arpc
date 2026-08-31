#include "eTran_common.h"

#include <signal.h>
#include <unistd.h>

#include <base/ipc.h>
#include <base/compiler.h>
#include <base/lrpc.h>
#include <shm/shm_wrapper.h>

#include <sys/un.h>
#include <sys/epoll.h>
#include <linux/if_ether.h>
#include <net/if.h>
#include <netinet/ip.h>
#include <netinet/tcp.h>

#include <xskbp/xsk_buffer_pool.h>
#include <app_if.h>

static struct app_ctx *actx = nullptr;
static std::mutex actx_lock;
static unsigned int tctx_id = 0;

static __thread struct app_ctx_per_thread *tctx;

static inline struct app_ctx_per_thread *alloc_tctx(void)
{
    std::lock_guard<std::mutex> lock(actx_lock);
    if (tctx_id >= actx->nr_app_threads)
        return NULL;
    return &actx->tctx[tctx_id++];
}

struct app_ctx_per_thread *eTran_get_tctx(void)
{
    if (likely(tctx))
        return tctx;
    tctx = alloc_tctx();
    return tctx;
}

// interpose.cc
extern int (*libc_epoll_create1)(int flags);
extern int (*libc_epoll_ctl)(int epfd, int op, int fd,
                             struct epoll_event *event);
extern int (*libc_epoll_wait)(int epfd, struct epoll_event *events,
                              int maxevents, int timeout);

/**
 * @brief initialize umem rings (Fill Ring and Completion Ring)
 */
static int umem_ring_init(void)
{
    struct buffer_pool_wrapper *bpw;
    struct xdp_mmap_offsets off;
    int xsk_fd;
    void *map_addr;

    if (!actx)
        return -EINVAL;

    bpw = &actx->bpw;
    if (!bpw || !bpw->bp)
        return -EINVAL;

    for (unsigned int i = 0; i < actx->nr_nic_queues; i++)
    {
        unsigned int qid = actx->nic_qid[i];
        unsigned int qidx = actx->qid2idx[qid];
        actx->uring[qidx].fq = &bpw->bp->fq[qid];
        actx->uring[qidx].cq = &bpw->bp->cq[qid];
        actx->uring[qidx].fq_lock = &bpw->bp->fq_lock[qid];
        actx->uring[qidx].cq_lock = &bpw->bp->cq_lock[qid];
        atomic32_init(&actx->uring[qidx].fq_work);
        atomic32_init(&actx->uring[qidx].cq_work);

        // use fds from the first thread
        xsk_fd = actx->tctx[0].txrx_xsk_fd[i];

        if (xsk_get_mmap_offsets(xsk_fd, &off))
            goto err;

        map_addr = actx->uring[qidx].fq->ring - off.fr.desc;
        actx->uring[qidx].fill_map =
            mmap(NULL, off.fr.desc + XSK_RING_PROD__DEFAULT_NUM_DESCS * 2 * sizeof(__u64), PROT_READ | PROT_WRITE,
                 MAP_SHARED | MAP_POPULATE, xsk_fd, XDP_UMEM_PGOFF_FILL_RING);
        if (actx->uring[qidx].fill_map == MAP_FAILED)
        {
            perror("fill mmap failed");
            goto err;
        }

        actx->uring[qidx].fill_offset = reinterpret_cast<uintptr_t>(actx->uring[qidx].fill_map) -  reinterpret_cast<uintptr_t>(map_addr);
        actx->uring[qidx].fill_map_size = off.fr.desc + XSK_RING_PROD__DEFAULT_NUM_DESCS * 2 * sizeof(__u64);

        map_addr = actx->uring[qidx].cq->ring - off.cr.desc;
        actx->uring[qidx].comp_map =
            mmap(NULL, off.cr.desc + XSK_RING_CONS__DEFAULT_NUM_DESCS * sizeof(__u64), PROT_READ | PROT_WRITE,
                 MAP_SHARED | MAP_POPULATE, xsk_fd, XDP_UMEM_PGOFF_COMPLETION_RING);
        if (actx->uring[qidx].comp_map == MAP_FAILED)
        {
            perror("comp mmap failed");
            goto err;
        }

        actx->uring[qidx].comp_offset = reinterpret_cast<uintptr_t>(actx->uring[qidx].comp_map) -  reinterpret_cast<uintptr_t>(map_addr);
        actx->uring[qidx].comp_map_size = off.cr.desc + XSK_RING_CONS__DEFAULT_NUM_DESCS * sizeof(__u64);
    }

    return 0;
err:
    return -1;
}

/**
 * @brief initialize AF_XDP sockets
 */
static int txrx_xsk_init(void)
{
    struct epoll_event ev;
    int xsk_fd;
    int umem_fd;
    unsigned int tidx = 0;

    if (!actx)
        return -EINVAL;

    for (unsigned int i = 0; i < actx->nr_app_threads; i++)
    {
        for (unsigned int j = 0; j < actx->nr_nic_queues; j++)
        {
            xsk_fd = actx->tctx[i].txrx_xsk_fd[j];
            umem_fd = actx->umem_fd[j];
#ifdef DEBUG
            printf("txrx_xsk_init(): xsk_fd: %d, umem_fd: %d, nic_qid: %d, ifindex: "
                   "%d\n",
                   xsk_fd, umem_fd, actx->nic_qid[j], actx->ifindex);
#endif
            actx->tctx[i].txrx_xsk_info[j] =
                xsk_configure_socket(xsk_fd, actx->nic_qid[j], umem_fd, actx->ifindex, &actx->bpw);
            if (!actx->tctx[i].txrx_xsk_info[j])
                goto err;

            ev.events = EPOLLIN;
            ev.data.fd = xsk_fd;
            if (libc_epoll_ctl(actx->tctx[i].epfd, EPOLL_CTL_ADD, xsk_fd, &ev))
            {
                perror("epoll_ctl() failed");
                goto err;
            }
            // FIXME
            if (xsk_fd >= 1024 * 1024)
            {
                fprintf(stderr, "xsk_fd is too large\n");
                goto err;
            }
            actx->tctx[i].txrx_xsk_fd_to_idx[xsk_fd] = j;
        }
    }

    for (unsigned int i = 0; i < actx->nr_nic_queues; i++)
    {
        actx->tctx[tidx].cqidx[actx->tctx[tidx].cqk++] = i;
        actx->tctx[tidx].fqidx[actx->tctx[tidx].fqk++] = i;
        tidx = (tidx + 1) % actx->nr_app_threads;
    }

    return 0;
err:
    return -1;
}

/**
 * @brief initialize io buffer
 */
static int iobuffer_init(void)
{
    struct thread_bcache *bc;
    for (unsigned int i = 0; i < actx->nr_app_threads; i++)
    {
        bc = &actx->tctx[i].iobuffer;
        if (thread_bcache_create(&actx->bpw, bc))
            return -EIO;
    }
    return 0;
}

/**
 * @brief initialize memory pool
 */
static int mempool_init(void)
{
    for (unsigned int i = 0; i < actx->nr_app_threads; i++)
    {
        actx->tctx[i].mp = new Mempool(MBytes(16));
    }
    return 0;
}

/**
 * @brief populate fill ring in advance
 */
static int populate_fill_ring(void)
{
    struct xsk_ring_prod *fq;
    unsigned int ret = 0;
    unsigned int idx = 0;

    for (unsigned int i = 0; i < actx->nr_nic_queues; i++)
    {
        int qidx = actx->qid2idx[actx->nic_qid[i]];
        struct thread_bcache tmp_bc;
        if (thread_bcache_create(&actx->bpw, &tmp_bc))
            return -EIO;
        if (thread_bcache_check(&tmp_bc, XSK_RING_PROD__DEFAULT_NUM_DESCS * 2) < XSK_RING_PROD__DEFAULT_NUM_DESCS * 2)
        {
            fprintf(stderr, "thread_bcache_check() failed\n");
            thread_bcache_free(&tmp_bc);
            return -ENOMEM;
        }

        fq = actx->uring[qidx].fq;
        ret = eTran_fq__reserve(fq, XSK_RING_PROD__DEFAULT_NUM_DESCS * 2, &idx, actx->uring[qidx].fill_offset);
        if (ret != XSK_RING_PROD__DEFAULT_NUM_DESCS * 2)
        {
            fprintf(stderr, "xsk_ring_prod__reserve() failed, %d\n", ret);
            thread_bcache_free(&tmp_bc);
            return -EIO;
        }

        for (unsigned int j = 0; j < XSK_RING_PROD__DEFAULT_NUM_DESCS * 2; j++)
        {
            *eTran_fq__fill_addr(fq, idx++, actx->uring[qidx].fill_offset) = thread_bcache_cons(&tmp_bc);
        }
        eTran_fq__submit(fq, XSK_RING_PROD__DEFAULT_NUM_DESCS * 2, actx->uring[qidx].fill_offset);

        thread_bcache_free(&tmp_bc);
    }
    return 0;
}

/**
 * @brief allocate resources for this application process
 */
static int resrouce_alloc(void)
{
    if (txrx_xsk_init())
        return -EIO;
#ifdef DEBUG
    printf("eTran_xsk_init(): txrx_xsk_init() done\n");
#endif

    if (umem_ring_init())
        return -EIO;
#ifdef DEBUG
    printf("eTran_xsk_init(): umem_ring_init() done\n");
#endif

    if (iobuffer_init())
        return -EIO;
#ifdef DEBUG
    printf("eTran_xsk_init(): iobuffer_init() done\n");
#endif

    if (mempool_init())
        return -EIO;

    if (populate_fill_ring())
        return -EIO;
#ifdef DEBUG
    printf("eTran_xsk_init(): populate_fill_ring() done\n");
#endif
    return 0;
}

/**
 * @brief free resources for this application process
 */
static int _resource_free(void)
{
    if (!actx)
        return -EINVAL;

    for (unsigned int i = 0; i < actx->nr_app_threads; i++)
    {
        thread_bcache_free(&actx->tctx[i].iobuffer);
        delete actx->tctx[i].mp;
    }

    for (unsigned int i = 0; i < actx->nr_app_threads; i++)
        for (unsigned int j = 0; j < actx->nr_nic_queues; j++)
            xsk_delete_socket(actx->tctx[i].txrx_xsk_info[j]);

    for (unsigned int i = 0; i < actx->nr_nic_queues; i++)
    {
        int qidx = actx->qid2idx[actx->nic_qid[i]];
        if (actx->uring[qidx].fill_map_size)
            munmap(actx->uring[qidx].fill_map, actx->uring[qidx].fill_map_size);
        if (actx->uring[qidx].comp_map_size)
            munmap(actx->uring[qidx].comp_map, actx->uring[qidx].comp_map_size);
    }

    return 0;
}

/**
 * @brief initialize lrpc channels, each channel is unidirectional
 *        (app --> kernel, kernel --> app)
 * @param shm_lrpc_name_base base name of shared memory for lrpc
 * @param shm_lrpc_size size of shared memory for each lrpc channel
 */
static inline int init_lrpc_channels(std::string &shm_lrpc_name_base, size_t shm_lrpc_size)
{
    char *k_recv_head_wb, *app_recv_head_wb;
    char *channel_1, *channel_2;
    for (unsigned int i = 0; i < actx->nr_app_threads; i++)
    {
        std::string shm_lrpc_name = shm_lrpc_name_base + "_" + std::to_string(i);
#ifdef DEBUG
        printf("init_lrpc_channels(): shm_lrpc_name: %s\n", shm_lrpc_name.c_str());
#endif
        actx->tctx[i].lrpc = shm_wrapper_attach(std::string(shm_lrpc_name), shm_lrpc_size);
        if (!actx->tctx[i].lrpc)
            goto err;

        k_recv_head_wb = (char *)actx->tctx[i].lrpc->addr;
        app_recv_head_wb = k_recv_head_wb + sizeof(struct head_wb);

        channel_1 = app_recv_head_wb + sizeof(struct head_wb);
        channel_2 = channel_1 + sizeof(lrpc_msg) * LRPC_CHANNEL_SIZE;

        if (lrpc_init_in(&actx->tctx[i].app_in, (lrpc_msg *)channel_1, LRPC_CHANNEL_SIZE, (uint32_t *)app_recv_head_wb))
        {
            goto err;
        }

        if (lrpc_init_out(&actx->tctx[i].app_out, (lrpc_msg *)channel_2, LRPC_CHANNEL_SIZE, (uint32_t *)k_recv_head_wb))
        {
            goto err;
        }
    }
    return 0;
err:
    for (unsigned int i = 0; i < actx->nr_app_threads; i++)
        shm_wrapper_detach(actx->tctx[i].lrpc);

    return -1;
}


int eTran_init(struct eTran_cfg *cfg)
{
    struct epoll_event ev;
    struct sockaddr_un addr;
    struct register_request req = {};
    struct register_response resp = {};
    std::string shm_bp_name;
    std::string shm_umem_name;
    std::string shm_lrpc_name;
    size_t shm_bp_size;
    size_t shm_umem_size;
    size_t shm_lrpc_size;
    struct ucred cred;
    socklen_t ucred_len = sizeof(struct ucred);

    signal(SIGUSR1, eTran_intr);
    if (!cfg || 
        (cfg->proto != IPPROTO_TCP && cfg->proto != IPPROTO_HOMA) || 
            cfg->nr_nic_queues == 0 || cfg->nr_app_threads == 0)
        return -EINVAL;

    if (actx)
        return -EEXIST;

    actx = new app_ctx();
    if (!actx)
        return -ENOMEM;
    
    actx->proto = cfg->proto;
    actx->nr_nic_queues = cfg->nr_nic_queues;
    actx->nr_app_threads = cfg->nr_app_threads;

    actx->fd = socket(AF_UNIX, SOCK_STREAM, 0);
    if (actx->fd < 0)
        return -EIO;
#ifdef DEBUG
    printf("eTran_init(): actx->fd: %d\n", actx->fd);
#endif

    memset(&addr, 0, sizeof(addr));
    addr.sun_family = AF_UNIX;
    strncpy(addr.sun_path, MICRO_KERNEL_SOCK_PATH, sizeof(addr.sun_path) - 1);
    /* connect through Unix domain socket */
    if (connect(actx->fd, (struct sockaddr *)&addr, sizeof(addr)) < 0)
    {
        close(actx->fd);
        return -EIO;
    }

    if (getsockopt(actx->fd, SOL_SOCKET, SO_PEERCRED, &cred, &ucred_len))
    {
        perror("getsockopt");
        return -EIO;
    }

    actx->pid = cred.pid;

    /* craft register request */
    req.type = REG;
    req.proto = cfg->proto;
    req.nr_nic_queues = cfg->nr_nic_queues;
    req.nr_app_threads = cfg->nr_app_threads;

    if (write_all(actx->fd, &req, sizeof(req)) < 0)
        return -EIO;

    /* receive AF_XDP socket fds, including control path fds and data path fds */
    if (read_all(actx->fd, &resp, sizeof(resp)) < 0 || resp.type != REG_XSK_FD)
        return -EIO;
    
    if (receive_fds(actx->fd, actx->umem_fd, actx->nr_nic_queues))
        return -EIO;

    for (unsigned int i = 0; i < actx->nr_app_threads; i++)
        if (receive_fds(actx->fd, actx->tctx[i].txrx_xsk_fd, actx->nr_nic_queues))
            return -EIO;

    /* receive event fds for lrpc channel */
    if (read_all(actx->fd, &resp, sizeof(resp)) < 0 || resp.type != REG_EVENT_FD)
        return -EIO;

    for (unsigned int i = 0; i < actx->nr_app_threads; i++)
    {
        if (receive_fd(actx->fd, &actx->tctx[i].evfd))
            return -EIO;
        actx->tctx[i].tid = i;
        actx->tctx[i].epfd = libc_epoll_create1(0);
        if (actx->tctx[i].epfd == -1)
        {
            perror("epoll_create1");
            return -EIO;
        }
        ev.data.fd = actx->tctx[i].evfd;
        ev.events = EPOLLIN;
        if (libc_epoll_ctl(actx->tctx[i].epfd, EPOLL_CTL_ADD, actx->tctx[i].evfd, &ev) == -1)
        {
            perror("epoll_ctl: listen_sock");
            return -EIO;
        }
    }

    /* receive shared memory and NIC queue information */
    if (read_all(actx->fd, &resp, sizeof(resp)) < 0 || resp.type != REG_DONE)
        return -EIO;

    shm_bp_size = resp.shm_bp_size;
    shm_umem_size = resp.shm_umem_size;
    shm_lrpc_size = resp.shm_lrpc_size;
    actx->bpw.bp_params = resp.bp_params;
    actx->ifindex = resp.ifindex;

    for (unsigned int i = 0; i < actx->nr_nic_queues; i++)
    {
        actx->nic_qid[i] = resp.nic_qid[i];
        actx->qid2idx[actx->nic_qid[i]] = i;
    }

#ifdef DEBUG
    printf("Queue IDs: [");
    for (unsigned int i = 0; i < actx->nr_nic_queues; i++)
        printf("%u ", actx->nic_qid[i]);
    printf("]\n");
    printf("UMEM fds: [");
    for (unsigned int i = 0; i < actx->nr_nic_queues; i++)
        printf("%u ", actx->umem_fd[i]);
    printf("]\n");
#endif

    shm_bp_name = SHM_BP_PREFIX + std::to_string(getpid());
    shm_umem_name = SHM_UMEM_PREFIX + std::to_string(getpid());
    shm_lrpc_name = SHM_LRPC_PREFIX + std::to_string(getpid());

#ifdef DEBUG
    printf("eTran_init(): shm_bp_size: %zu, shm_umem_size: %zu, shm_lrpc_size: %zu, shm_bp_name: %s, "
           "shm_umem_name: %s, shm_lrpc_name: %s\n",
           shm_bp_size, shm_umem_size, shm_lrpc_size, shm_bp_name.c_str(), shm_umem_name.c_str(),
           shm_lrpc_name.c_str());
#endif

    actx->bpw.shm_bp = shm_wrapper_attach(std::string(shm_bp_name), shm_bp_size);
    if (!actx->bpw.shm_bp)
        goto err;

    actx->bpw.shm_umem = shm_wrapper_attach(std::string(shm_umem_name), shm_umem_size);
    if (!actx->bpw.shm_umem)
        goto err;

    actx->bpw.bp = reinterpret_cast<struct buffer_pool *>(actx->bpw.shm_bp->addr);

    if (init_lrpc_channels(shm_lrpc_name, shm_lrpc_size))
        goto err;

    if (resrouce_alloc())
        goto err;

    // XXX: I don't know why we need to kick the fill queue here
    // without this code, the microkenrel can't receive any packet
    for (unsigned int i = 0; i < actx->nr_nic_queues; i++)
    {
        int qidx = actx->qid2idx[actx->nic_qid[i]];
        kick_fq(actx->umem_fd[i], actx->uring[qidx].fq, actx->uring[qidx].fill_offset);
    }

    actx->done = true;

    return 0;
err:
    _resource_free();
    if (actx->bpw.shm_bp)
        shm_wrapper_detach(actx->bpw.shm_bp);
    if (actx->bpw.shm_umem)
        shm_wrapper_detach(actx->bpw.shm_umem);
    for (unsigned int i = 0; i < actx->nr_app_threads; i++)
    {
        shm_wrapper_detach(actx->tctx[i].lrpc);
        close(actx->tctx[i].evfd);
    }
    return -EIO;
}

void eTran_intr(int sig)
{
    printf("Microkernel exits, we should exit at once\n");
    // it seems that there is no need for us to do some cleanup things.
    exit(0);
}

void eTran_exit(void)
{
    if (!actx)
        return;
    struct register_request req = {};

    _resource_free();

    if (actx->bpw.shm_bp)
        shm_wrapper_detach(actx->bpw.shm_bp);
    if (actx->bpw.shm_umem)
        shm_wrapper_detach(actx->bpw.shm_umem);

    for (unsigned int i = 0; i < actx->nr_app_threads; i++)
    {
        if (actx->tctx[i].lrpc)
            shm_wrapper_detach(actx->tctx[i].lrpc);
        close(actx->tctx[i].evfd);
    }

    if (!actx->done)
        return;

    req.type = UNREG;

    if (write_all(actx->fd, &req, sizeof(req)) < 0)
        return;

    close(actx->fd);

    free(actx);

    printf("eTran exit successfully.\n");
}

void eTran_dump_io_stats(struct app_ctx_per_thread *tctx)
{
    for (unsigned int i = 0; i < tctx->actx->nr_nic_queues; i++)
    {
        if (get_xsk_ring_stats(tctx->txrx_xsk_info[i]))
        {
            fprintf(stderr, "get_xsk_ring_stats failed%u\n", i);
        }
        dump_xsk_ring_stats(tctx->txrx_xsk_info[i], std::string(std::to_string(tctx->tid) + "/" + std::to_string(i)));
    }
}

void eTran_dump_all_io_stats(void)
{
    for (unsigned int i = 0; i < actx->nr_app_threads; i++)
    {
        eTran_dump_io_stats(&actx->tctx[i]);
    }
}

__attribute__((constructor))
void pre_main(int argc, char *argv[])
{
    struct eTran_cfg cfg = {0};
    const char* nr_app_threads_env = std::getenv("ETRAN_NR_APP_THREADS");
    const char* nr_nic_queues_env = std::getenv("ETRAN_NR_NIC_QUEUES");
    const char* proto_env = std::getenv("ETRAN_PROTO");

    if (!proto_env) {
        std::cerr << "Environment variables ETRAN_PROTO (tcp/TCP, homa/HOMA) must be set." << std::endl;
        exit(EXIT_FAILURE);
    }

    try {
        if (std::string(proto_env) == "TCP" || std::string(proto_env) == "tcp")
            cfg.proto = IPPROTO_TCP;
        else if (std::string(proto_env) == "HOMA" || std::string(proto_env) == "homa")
            cfg.proto = IPPROTO_HOMA;
        else {
            std::cerr << "Invalid protocol: " << proto_env << std::endl;
            exit(EXIT_FAILURE);
        }
        if (cfg.proto == IPPROTO_TCP) {
            if (!nr_app_threads_env || !nr_nic_queues_env) {
                std::cerr << "Environment variables ETRAN_NR_APP_THREADS and ETRAN_NR_NIC_QUEUES should also be set for TCP." << std::endl;
                exit(EXIT_FAILURE);
            }
            cfg.nr_app_threads = std::stoi(nr_app_threads_env);
            cfg.nr_nic_queues = std::stoi(nr_nic_queues_env);
        }
    } catch (const std::invalid_argument& e) {
        std::cerr << "Invalid environment variable value: " << e.what() << std::endl;
        exit(EXIT_FAILURE);
    } catch (const std::out_of_range& e) {
        std::cerr << "Environment variable value out of range: " << e.what() << std::endl;
        exit(EXIT_FAILURE);
    } catch (const std::exception& e) {
        std::cerr << "Unknown exception: " << e.what() << std::endl;
        exit(EXIT_FAILURE);
    }

    if (cfg.proto == IPPROTO_HOMA)
        return;

    if (eTran_init(&cfg))
    {
        std::cout << "eTran init failed." << std::endl;
        exit(EXIT_FAILURE);
    }
}

__attribute__((destructor))
void post_main(void)
{
    eTran_exit();
}