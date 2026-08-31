#include "ebpf.h"

#include <list>

#include <runtime/defs.h>
#include <runtime/ebpf_if.h>
#include <runtime/tcp.h>

#include "nic.h"
#include "trans_ebpf.h"

/* micro_kernel.cc */
extern class eTranNIC *etran_nic;

extern int opt_workload_type;
extern unsigned int opt_tcp_rx_buf_size;
extern unsigned int opt_tcp_tx_buf_size;
extern unsigned int opt_arpc_case;

class eTranEntrance *etran_entrance;
class eTranTCP *etran_tcp;
class eTranHoma *etran_homa;

static void remove_xdp(void)
{
    uint32_t prog_id;
    
    bpf_xdp_query_id(etran_entrance->_ebpf.ifindex, etran_entrance->_ebpf.attach_mode, &prog_id);

    bpf_xdp_detach(etran_entrance->_ebpf.ifindex, XDP_FLAGS_UPDATE_IF_NOEXIST, 0);
}

void xsk_delete_socket(struct xsk_socket_info *xsk_info)
{
    if (!xsk_info)
        return;

    xsk_socket__delete(xsk_info->xsk);

    xsk_umem__delete(xsk_info->umem);

    free(xsk_info);
}

struct xsk_socket_info *xsk_configure_socket(struct nic_queue_info *nic_queue, int proto)
{
    int ret = 0;
    struct xsk_socket_config cfg = {0};
    struct ebpf_info_t *ebpf_info;

    struct buffer_pool_wrapper *bpw = nic_queue->bpw;
    unsigned int qid = nic_queue->qid;

    struct xsk_socket_info *xsk_info =
        reinterpret_cast<struct xsk_socket_info *>(calloc(1, sizeof(struct xsk_socket_info)));

    if (!xsk_info)
    {
        perror("calloc");
        return nullptr;
    }

    if (proto == IPPROTO_HOMA)
        ebpf_info = &etran_homa->_ebpf;
    else if (proto == IPPROTO_TCP)
        ebpf_info = &etran_tcp->_ebpf;
    else
        return nullptr;

    xsk_info->umem = bpw->bp->umem;
    xsk_info->umem_area = reinterpret_cast<char *>(bpw->shm_umem->addr);

    xsk_info->fq = &bpw->bp->fq[qid];
    xsk_info->cq = &bpw->bp->cq[qid];

    cfg.rx_size = XSK_RING_CONS__DEFAULT_NUM_DESCS;
    cfg.tx_size = XSK_RING_PROD__DEFAULT_NUM_DESCS;
    cfg.libxdp_flags = XSK_LIBXDP_FLAGS__INHIBIT_PROG_LOAD;
    cfg.xdp_flags = XDP_FLAGS_REPLACE;

    if (ebpf_info->attach_mode == XDP_MODE_NATIVE)
        cfg.xdp_flags |= XDP_FLAGS_DRV_MODE;
    else
        cfg.xdp_flags |= XDP_FLAGS_SKB_MODE;

    cfg.bind_flags = ebpf_info->xdp_bind_flags;

    if (ebpf_info->xdp_egress_fd > 0)
        cfg.xdp_egress_fd = (uint32_t)ebpf_info->xdp_egress_fd;

    if (ebpf_info->xdp_gen_fd > 0)
        cfg.xdp_gen_fd = (uint32_t)ebpf_info->xdp_gen_fd;

    ret = xsk_socket__create_shared(&xsk_info->xsk, ebpf_info->if_name.c_str(), qid, xsk_info->umem, &xsk_info->rx,
                                    &xsk_info->tx, xsk_info->fq, xsk_info->cq, &cfg);

    if (ret)
    {
        perror("xsk_socket__create_shared");
        free(xsk_info);
        return nullptr;
    }

    return xsk_info;
}

static int add_trans_to_entrance(class eTranEntrance *etran_entrance, class eTranTransport *trans)
{
    int key, err;
    key = etran_entrance->_key;
    err = bpf_map_update_elem(etran_entrance->_tran_xdp_map_fd, &key, &trans->_ebpf.xdp_fd, BPF_ANY);
    if (err)
    {
        fprintf(stderr, "ERROR: bpf_map_update_elem tran_xdp_map_fd\n");
        goto err1;
    }
    key = etran_entrance->_key;
    err = bpf_map_update_elem(etran_entrance->_tran_xdp_egress_map_fd, &key, &trans->_ebpf.xdp_egress_fd, BPF_ANY);
    if (err)
    {
        fprintf(stderr, "ERROR: bpf_map_update_elem etran_entrance->_tran_xdp_egress_map_fd\n");
        goto err2;
    }
    key = etran_entrance->_key;
    err = bpf_map_update_elem(etran_entrance->_tran_xdp_gen_map_fd, &key, &trans->_ebpf.xdp_gen_fd, BPF_ANY);
    if (err)
    {
        fprintf(stderr, "ERROR: bpf_map_update_elem etran_entrance->_tran_xdp_gen_map_fd\n");
        goto err3;
    }
    etran_entrance->_key++;
    return 0;
err3:
    key = etran_entrance->_key;
    bpf_map_delete_elem(etran_entrance->_tran_xdp_egress_map_fd, &key);
err2:
    key = etran_entrance->_key;
    bpf_map_delete_elem(etran_entrance->_tran_xdp_map_fd, &key);
err1:
    return -1;
}

int ebpf_init(void)
{
    struct trans_params_t trans_params = {};

    etran_entrance = new eTranEntrance(
        etran_nic,
        XDP_MODE_NATIVE,
        XDP_USE_NEED_WAKEUP | XDP_ZEROCOPY,
        false,
        0,
        (struct trans_params_t *)&trans_params);

    struct trans_params_t tcp_params;
    tcp_params.tcp.rx_buf_size = opt_tcp_rx_buf_size;
    tcp_params.tcp.tx_buf_size = opt_tcp_tx_buf_size;
    tcp_params.tcp.arpc_case = opt_arpc_case;

    etran_tcp = new eTranTCP(
        etran_nic,
        XDP_MODE_NATIVE,
        XDP_USE_NEED_WAKEUP | XDP_ZEROCOPY,
        false,
        0,
        (struct trans_params_t *)&tcp_params);

    struct trans_params_t homa_params;
    homa_params.homa.workload_type = opt_workload_type;

    etran_homa = new eTranHoma(
        etran_nic,
        XDP_MODE_NATIVE,
        XDP_USE_NEED_WAKEUP | XDP_ZEROCOPY,
        false,
        0,
        (struct trans_params_t *)&homa_params);

    /* Remove existing XDP program */
    remove_xdp();

    /* Load entrance program first */
    INIT_CHECK(etran_entrance->load_ebpf_programs());
    INIT_CHECK(etran_entrance->init_ebpf_maps());

    /* Load tcp eBPF programs and initialize eBPF maps */
    INIT_CHECK(etran_tcp->load_ebpf_programs());
    INIT_CHECK(etran_tcp->init_ebpf_maps());

    /* Load homa eBPF programs and initialize eBPF maps */
    INIT_CHECK(etran_homa->load_ebpf_programs());
    INIT_CHECK(etran_homa->init_ebpf_maps());

    /* Add tcp and homa programs to entrance */
    INIT_CHECK(add_trans_to_entrance(etran_entrance, etran_tcp));
    INIT_CHECK(add_trans_to_entrance(etran_entrance, etran_homa));

    printf("\tTCP: ARPC case %u enabled.\n", opt_arpc_case);

    return 0;
}

void ebpf_exit(void)
{
    if (etran_entrance) {
        remove_xdp();
        delete etran_entrance;
    }
    
    if (etran_tcp)
        delete etran_tcp;
    
    if (etran_homa)
        delete etran_homa;
}