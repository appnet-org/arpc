#pragma once

#include <linux/if_link.h>
#include <net/if.h>
#include <bpf/libbpf.h>
#include <xdp/libxdp.h>
#include <xdp/xsk.h>

#include <intf/intf_ebpf.h>
#include <xskbp/xsk_buffer_pool.h>
#include <runtime/app_if.h>
#include <runtime/defs.h>
#include <runtime/tcp.h>

// copy from libxdp
struct xdp_program
{
    /* one of prog or prog_fd should be set */
    struct bpf_program *bpf_prog;
    struct bpf_object *bpf_obj;
    struct btf *btf;
    enum bpf_prog_type prog_type;
    int prog_fd;
    int link_fd;
    char *prog_name;
    char *attach_name;
    __u8 prog_tag[BPF_TAG_SIZE];
    __u32 prog_id;
    __u64 load_time;
    bool from_external_obj;
    bool is_frags;
    unsigned int run_prio;
    unsigned int chain_call_actions; /* bitmap */

    /* for building list of attached programs to multiprog */
    struct xdp_program *next;
};

struct bpf_prog_desc
{
    std::string name;
    enum bpf_prog_type prog_type;
    struct bpf_program *bpf_prog;
};

struct xsk_app_stats
{
    unsigned long rx_empty_polls;
    unsigned long fill_fail_polls;
    unsigned long copy_tx_sendtos;
    unsigned long tx_wakeup_sendtos;
    unsigned long opt_polls;
    unsigned long prev_rx_empty_polls;
    unsigned long prev_fill_fail_polls;
    unsigned long prev_copy_tx_sendtos;
    unsigned long prev_tx_wakeup_sendtos;
    unsigned long prev_opt_polls;
};

struct xsk_ring_stats
{
    unsigned long rx_frags;
    unsigned long rx_npkts;
    unsigned long tx_frags;
    unsigned long tx_npkts;
    unsigned long rx_dropped_npkts;
    unsigned long rx_invalid_npkts;
    unsigned long tx_invalid_npkts;
    unsigned long rx_full_npkts;
    unsigned long rx_fill_empty_npkts;
    unsigned long tx_empty_npkts;
    unsigned long prev_rx_frags;
    unsigned long prev_rx_npkts;
    unsigned long prev_tx_frags;
    unsigned long prev_tx_npkts;
    unsigned long prev_rx_dropped_npkts;
    unsigned long prev_rx_invalid_npkts;
    unsigned long prev_tx_invalid_npkts;
    unsigned long prev_rx_full_npkts;
    unsigned long prev_rx_fill_empty_npkts;
    unsigned long prev_tx_empty_npkts;
};

// wrapper for struct xsk_socket
struct xsk_socket_info
{
    struct xsk_socket *xsk;

    struct xsk_ring_cons rx;
    struct xsk_ring_prod tx;
    struct xsk_ring_prod *fq;
    struct xsk_ring_cons *cq;
    struct xsk_umem *umem;

    char *umem_area;

    struct xsk_app_stats app_stats;
    struct xsk_ring_stats ring_stats;
    unsigned int outstanding;
    unsigned int needfill;
};

struct nic_queue_info
{
    unsigned int qid;
    struct xsk_socket_info *xsk_info;
    int xsk_map_key;
    struct buffer_pool_wrapper *bpw;

    struct app_ctx *actx;
};

void xsk_delete_socket(struct xsk_socket_info *xsk_info);

struct xsk_socket_info *xsk_configure_socket(struct nic_queue_info *nic_queue, int proto);

#define HOMA_TX_FLAG_OFFSET 24
#define HOMA_TX_FROM_SLOW_PATH_OFFSET 20
#define HOMA_TX_BUFFER_NEXT_OFFSET 16
#define HOMA_TX_BUFFER_ADDR_OFFSET 8
static inline void homa_txmeta_clear_all(char *umem_area, uint64_t addr)
{
    char *x = (char *)xsk_umem__get_data(umem_area, addr) - 28;
    memset(x, 0, 28);
}

static inline void homa_txmeta_set_from_slowpath(char *umem_area, uint64_t addr, uint32_t flag)
{
    char *x = (char *)xsk_umem__get_data(umem_area, addr) - HOMA_TX_FROM_SLOW_PATH_OFFSET;
    *(uint32_t *)x = flag;
}

// RX meta data
#define RX_QID_OFFSET 32
#define RX_CONN_OFFSET 28
#define RX_POS_OFFSET 20
#define RX_POFF_OFFSET 16
#define RX_PLEN_OFFSET 14
#define RX_XSK_BUDGET_OFFSET 12
#define RX_ACK_BYTES_OFFSET 8
#define RX_GO_BACK_POS_OFFSET 8
#define RX_OOO_BUMP_OFFSET 4

#define RECOVERY_MASK 0x80000000
#define OOO_SEGMENT_MASK 0x80000000
#define OOO_FIN_MASK 0x40000000
#define OOO_CLEAR_MASK      0x20000000

static inline uint32_t rxmeta_qid(char *pkt)
{
    char *x = pkt - RX_QID_OFFSET;
    return *(uint32_t *)x;
}

static inline uint64_t rxmeta_conn(char *pkt)
{
    char *x = pkt - RX_CONN_OFFSET;
    return *(uint64_t *)x;
}

static inline uint32_t rxmeta_pos(char *pkt)
{
    char *x = pkt - RX_POS_OFFSET;
    return *(uint32_t *)x;
}

static inline uint16_t rxmeta_poff(char *pkt)
{
    char *x = pkt - RX_POFF_OFFSET;
    return *(uint16_t *)x;
}

static inline uint16_t rxmeta_plen(char *pkt)
{
    char *x = pkt - RX_PLEN_OFFSET;
    return *(uint16_t *)x;
}

static inline uint32_t rxmeta_xskbudget(char *pkt)
{
    char *x = pkt - RX_XSK_BUDGET_OFFSET;
    return *(uint32_t *)x;
}

static inline uint32_t rxmeta_ackbytes(char *pkt)
{
    char *x = pkt - RX_ACK_BYTES_OFFSET;
    return *(uint32_t *)x;
}

static inline uint32_t rxmeta_go_back_pos(char *pkt)
{
    char *x = pkt - RX_GO_BACK_POS_OFFSET;
    return *(uint32_t *)x;
}

static inline uint32_t rxmeta_ooo_bump(char *pkt)
{
    char *x = pkt - RX_OOO_BUMP_OFFSET;
    return *(uint32_t *)x;
}

// Tx meta data
#define TX_FROM_SLOW_PATH_OFFSET 32
#define TX_FLAG_OFFSET 24
#define TX_RX_BUMP_OFFSET 16
#define TX_PENDING_OFFSET 12
#define TX_POS_OFFSET 8
#define TX_PLEN_OFFSET 4

static inline void tcp_txmeta_clear_all(char *umem_area, uint64_t addr)
{
    char *x = (char *)xsk_umem__get_data(umem_area, addr) - 32;
    memset(x, 0, 32);
}

static inline void tcp_txmeta_from_slowpath(char *umem_area, uint64_t addr, uint64_t flag)
{
    char *x = (char *)xsk_umem__get_data(umem_area, addr) - TX_FROM_SLOW_PATH_OFFSET;
    *(uint64_t *)x = flag;
}

static inline void tcp_txmeta_flag(char *umem_area, uint64_t addr, uint64_t flag)
{
    char *x = (char *)xsk_umem__get_data(umem_area, addr) - TX_FLAG_OFFSET;
    *(uint64_t *)x = flag;
}

static inline void tcp_txmeta_rxbump(char *umem_area, uint64_t addr, uint32_t rxbump)
{
    char *x = (char *)xsk_umem__get_data(umem_area, addr) - TX_RX_BUMP_OFFSET;
    *(uint32_t *)x = rxbump;
}

static inline void tcp_txmeta_pending(char *umem_area, uint64_t addr, uint32_t pending)
{
    char *x = (char *)xsk_umem__get_data(umem_area, addr) - TX_PENDING_OFFSET;
    *(uint32_t *)x = pending;
}

static inline void tcp_txmeta_pos(char *umem_area, uint64_t addr, uint32_t pos)
{
    char *x = (char *)xsk_umem__get_data(umem_area, addr) - TX_POS_OFFSET;
    *(uint32_t *)x = pos;
}
static inline void tcp_txmeta_plen(char *umem_area, uint64_t addr, uint32_t plen)
{
    char *x = (char *)xsk_umem__get_data(umem_area, addr) - TX_PLEN_OFFSET;
    *(uint32_t *)x = plen;
}

static inline void kick_tx(struct xsk_socket_info *xsk_info)
{
    if (xsk_ring_prod__needs_wakeup(&xsk_info->tx))
        sendto(xsk_socket__fd(xsk_info->xsk), nullptr, 0, MSG_DONTWAIT, nullptr, 0);
}

static inline void kick_fq(struct xsk_socket_info *xsk_info)
{
    if (xsk_ring_prod__needs_wakeup(xsk_info->fq))
        recvfrom(xsk_socket__fd(xsk_info->xsk), nullptr, 0, MSG_DONTWAIT, nullptr, nullptr);
}

static inline int get_xsk_ring_stats(struct xsk_socket_info *xsk_info)
{
    struct xdp_statistics stats;
    socklen_t optlen;
    int err;
    int fd = xsk_socket__fd(xsk_info->xsk);

    optlen = sizeof(stats);
    err = getsockopt(fd, SOL_XDP, XDP_STATISTICS, &stats, &optlen);
    if (err)
        return err;

    if (optlen == sizeof(struct xdp_statistics))
    {
        xsk_info->ring_stats.rx_dropped_npkts = stats.rx_dropped;
        xsk_info->ring_stats.rx_invalid_npkts = stats.rx_invalid_descs;
        xsk_info->ring_stats.tx_invalid_npkts = stats.tx_invalid_descs;
        xsk_info->ring_stats.rx_full_npkts = stats.rx_ring_full;
        xsk_info->ring_stats.rx_fill_empty_npkts = stats.rx_fill_ring_empty_descs;
        xsk_info->ring_stats.tx_empty_npkts = stats.tx_ring_empty_descs;
        return 0;
    }

    return -EINVAL;
}

static inline void dump_xsk_ring_stats(struct xsk_socket_info *xsk_info, std::string name)
{
    struct xsk_ring_stats *stats = &xsk_info->ring_stats;
    printf("Statistics for XSK Ring(%s) after last dump:\n", name.c_str());
    fprintf(stderr, "RX dropped npkts: %lu, RX invalid npkts: %lu, TX invalid npkts: %lu\n",
            stats->rx_dropped_npkts - stats->prev_rx_dropped_npkts,
            stats->rx_invalid_npkts - stats->prev_rx_invalid_npkts,
            stats->tx_invalid_npkts - stats->prev_tx_invalid_npkts);
    fprintf(stderr, "RX full npkts: %lu, RX fill empty npkts: %lu, TX empty npkts: %lu\n",
            stats->rx_full_npkts - stats->prev_rx_full_npkts,
            stats->rx_fill_empty_npkts - stats->prev_rx_fill_empty_npkts,
            stats->tx_empty_npkts - stats->prev_tx_empty_npkts);

    stats->prev_rx_dropped_npkts = stats->rx_dropped_npkts;
    stats->prev_rx_invalid_npkts = stats->rx_invalid_npkts;
    stats->prev_tx_invalid_npkts = stats->tx_invalid_npkts;
    stats->prev_rx_full_npkts = stats->rx_full_npkts;
    stats->prev_rx_fill_empty_npkts = stats->rx_fill_empty_npkts;
    stats->prev_tx_empty_npkts = stats->tx_empty_npkts;
}


static inline void apply_socket_busy_poll(int xsk_fd)
{
    int ret;
	int sock_opt;

	sock_opt = 1;

    ret = setsockopt(xsk_fd, SOL_SOCKET, SO_PREFER_BUSY_POLL,
		       (void *)&sock_opt, sizeof(sock_opt));
	if (ret == -EPERM) {
        fprintf(stderr, "Ignore SO_PREFER_BUSY_POLL as it failed: this option needs privileged mode.\n");
    } else if (ret < 0) {
        fprintf(stderr, "Ignore SO_PREFER_BUSY_POLL as it failed\n");
    }

	sock_opt = 20;
	if (setsockopt(xsk_fd, SOL_SOCKET, SO_BUSY_POLL,
		       (void *)&sock_opt, sizeof(sock_opt)) < 0) {
        fprintf(stderr, "Ignore SO_BUSY_POLL as it failed\n");
    }

	sock_opt = 64;
    ret = setsockopt(xsk_fd, SOL_SOCKET, SO_BUSY_POLL_BUDGET,
		       (void *)&sock_opt, sizeof(sock_opt));
	if (ret == -EPERM) {
        fprintf(stderr, "Ignore SO_BUSY_POLL_BUDGET as it failed: this option needs privileged mode.\n");
    } else if (ret < 0) {
        fprintf(stderr, "Ignore SO_BUSY_POLL_BUDGET as it failed\n");
    }
}
