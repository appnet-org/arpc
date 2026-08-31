#pragma once

#include <sys/socket.h>
#include <sys/mman.h>
#include <bpf/libbpf.h>
#include <xdp/xsk.h>

#include <base/atomic.h>
#include <base/lock.h>
#include <tcp_if.h>
#include <homa_if.h>

#include <string>

struct xsk_umem_rings {
    struct xsk_ring_prod *fq;
    uintptr_t fill_offset;
    spinlock_t *fq_lock;
    atomic32_t fq_work;
    
    struct xsk_ring_cons *cq;
    uintptr_t comp_offset;
    spinlock_t *cq_lock;
    atomic32_t cq_work;

    void *fill_map;
    size_t fill_map_size;

    void *comp_map;
    size_t comp_map_size;
};

struct xsk_app_stats {
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

struct xsk_ring_stats {
	unsigned long rx_dropped_npkts;
	unsigned long rx_invalid_npkts;
	unsigned long tx_invalid_npkts;
	unsigned long rx_full_npkts;
	unsigned long rx_fill_empty_npkts;
	unsigned long tx_empty_npkts;
	unsigned long prev_rx_dropped_npkts;
	unsigned long prev_rx_invalid_npkts;
	unsigned long prev_tx_invalid_npkts;
	unsigned long prev_rx_full_npkts;
	unsigned long prev_rx_fill_empty_npkts;
	unsigned long prev_tx_empty_npkts;
};

struct xsk_socket_info {
    int fd;

    struct xsk_ring_cons rx;
    struct xsk_ring_prod tx;

    char *umem_area;

    void *rx_map;
    size_t rx_map_size;

    void *tx_map;
    size_t tx_map_size;

    struct xsk_app_stats app_stats;
    struct xsk_ring_stats ring_stats;

    unsigned int deficit;
    unsigned int outstanding;
    unsigned int cached_needfill;
};

/* Up until and including Linux 5.3 */
struct xdp_ring_offset_v1
{
    __u64 producer;
    __u64 consumer;
    __u64 desc;
};

/* Up until and including Linux 5.3 */
struct xdp_mmap_offsets_v1
{
    struct xdp_ring_offset_v1 rx;
    struct xdp_ring_offset_v1 tx;
    struct xdp_ring_offset_v1 fr;
    struct xdp_ring_offset_v1 cr;
};

void xsk_delete_socket(struct xsk_socket_info *xsk_info);

void xsk_mmap_offsets_v1(struct xdp_mmap_offsets *off);
int xsk_get_mmap_offsets(int fd, struct xdp_mmap_offsets *off);

struct xsk_socket_info *xsk_configure_socket(int xsk_fd, unsigned int qid, int umem_fd, int ifindex,
                                             struct buffer_pool_wrapper *bpw);

static inline bool xsk_rxring_empty(struct xsk_ring_cons *rx)
{
    return xsk_cons_nb_avail(rx, 32) == 0;
}

#define POISON_64 __UINT64_MAX__
#define POISON_32 __UINT32_MAX__
#define POISON_16 __UINT16_MAX__

//////////////////////////// Homa ////////////////////////////
// Rx meta data
#define HOMA_RX_QID_OFFSET 20
#define HOMA_RX_REAP_CLIENT_BUFFER_OFFSET 16
#define HOMA_RX_REAP_SERVER_BUFFER_OFFSET 8

static inline uint32_t homa_rxmeta_qid(char *pkt)
{
    char *x = pkt - HOMA_RX_QID_OFFSET;
    return *(uint32_t *)x;
}

static inline uint64_t homa_rxmeta_reap_client_buffer(char *pkt)
{
    char *x = pkt - HOMA_RX_REAP_CLIENT_BUFFER_OFFSET;
    return *(uint64_t *)x;
}

static inline uint64_t homa_rxmeta_reap_server_buffer(char *pkt)
{
    char *x = pkt - HOMA_RX_REAP_SERVER_BUFFER_OFFSET;
    return *(uint64_t *)x;
}

// Tx meta data
#define HOMA_TX_FLAG_OFFSET 24
#define HOMA_TX_FROM_SLOW_PATH_OFFSET 20
#define HOMA_TX_BUFFER_NEXT_OFFSET 16
#define HOMA_TX_BUFFER_ADDR_OFFSET 8

/* 
 * Homa tx meta data flags
 * FLAG_UNDER_REAP: this packet is under reaping
 * FLAG_UNDER_RETRANSMISSION: this packet is under retransmission
 * When message completes, the we mark its first buffer with FLAG_UNDER_REAP, enqueue it to reap queue if FLAG_UNDER_RETRANSMISSION is not set.
 * When enqueuing a message to retransmission queue, we first check if FLAG_UNDER_REAP or FLAG_UNDER_RETRANSMISSION is set, if so, we don't enqueue it to retransmission queue.
 * When flushing retransmission queue, we abort the retransmission if FLAG_UNDER_REAP is set. We clear FLAG_UNDER_RETRANSMISSION when retransmission completes.
 */
#define FLAG_UNDER_REAP 0x1
#define FLAG_UNDER_RETRANSMISSION 0x2

static inline void homa_txmeta_clear_all(char *umem_area, uint64_t addr)
{
    char *x = (char *)xsk_umem__get_data(umem_area, addr) - 28;
    memset(x, 0, 28);
}

static inline uint32_t homa_txmeta_get_flag(char *umem_area, uint64_t addr)
{
    char *x = (char *)xsk_umem__get_data(umem_area, addr) - HOMA_TX_FLAG_OFFSET;
    return *(uint32_t *)x;
}

static inline uint64_t homa_txmeta_get_buffer_next(char *umem_area, uint64_t addr)
{
    char *x = (char *)xsk_umem__get_data(umem_area, addr) - HOMA_TX_BUFFER_NEXT_OFFSET;
    return *(uint64_t *)x;
}

static inline uint64_t homa_txmeta_get_buffer_addr(char *umem_area, uint64_t addr)
{
    char *x = (char *)xsk_umem__get_data(umem_area, addr) - HOMA_TX_BUFFER_ADDR_OFFSET;
    return *(uint64_t *)x;
}

static inline void homa_txmeta_set_from_slowpath(char *umem_area, uint64_t addr, uint32_t flag)
{
    char *x = (char *)xsk_umem__get_data(umem_area, addr) - HOMA_TX_FROM_SLOW_PATH_OFFSET;
    *(uint32_t *)x = flag;
}

static inline void homa_txmeta_set_buffer_next(char *umem_area, uint64_t addr, uint64_t buffer_next)
{
    char *x = (char *)xsk_umem__get_data(umem_area, addr) - HOMA_TX_BUFFER_NEXT_OFFSET;
    *(uint64_t *)x = buffer_next;
}

static inline void homa_txmeta_set_buffer_addr(char *umem_area, uint64_t addr, uint64_t buffer_addr)
{
    char *x = (char *)xsk_umem__get_data(umem_area, addr) - HOMA_TX_BUFFER_ADDR_OFFSET;
    *(uint64_t *)x = buffer_addr;
}

static inline void homa_txmeta_set_flag(char *umem_area, uint64_t addr, uint32_t flag)
{
    char *x = (char *)xsk_umem__get_data(umem_area, addr) - HOMA_TX_FLAG_OFFSET;
    *(uint32_t *)x = flag;
}

//////////////////////////// TCP ////////////////////////////
#define FLAG_SYNC           0x1
#define FLAG_TO             0x2
#define FORCE_RX_BUMP_MASK  0x80000000
#define RECOVERY_MASK       0x80000000
#define OOO_SEGMENT_MASK    0x80000000
#define OOO_FIN_MASK        0x40000000
#define OOO_CLEAR_MASK      0x20000000

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

static inline void rxmeta_set_pos(char *pkt, uint32_t pos)
{
    char *x = pkt - RX_POS_OFFSET;
    *(uint32_t *)x = pos;
}

static inline void rxmeta_set_poff(char *pkt, uint16_t poff)
{
    char *x = pkt - RX_POFF_OFFSET;
    *(uint16_t *)x = poff;
}

static inline void rxmeta_set_plen(char *pkt, uint16_t plen)
{
    char *x = pkt - RX_PLEN_OFFSET;
    *(uint16_t *)x = plen;
}

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

static inline uint64_t tcp_txmeta_get_from_slowpath(char *umem_area, uint64_t addr)
{
    char *x = (char *)xsk_umem__get_data(umem_area, addr) - TX_FROM_SLOW_PATH_OFFSET;
    return *(uint64_t *)x;
}

static inline uint64_t tcp_txmeta_get_flag(char *umem_area, uint64_t addr)
{
    char *x = (char *)xsk_umem__get_data(umem_area, addr) - TX_FLAG_OFFSET;
    return *(uint64_t *)x;
}

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

static inline unsigned int get_qidx_with_hash(uint32_t remote_ip, uint16_t remote_port, uint32_t local_ip, uint16_t local_port, unsigned int nr_nic_queues)
{
    struct eTran_tcp_flow_tuple ft = {remote_ip, remote_port, local_ip, local_port};
    return ft.hash() % nr_nic_queues;
}

static inline unsigned int tcp_get_nr_buffers_from_len(size_t len)
{
    return CEIL_DIV(len, TCP_MSS_W_TS);
}

static inline unsigned int homa_get_nr_buffers_from_len(size_t len)
{
    return CEIL_DIV(len, HOMA_MSS);
}

static inline int get_xsk_ring_stats(struct xsk_socket_info *xsk_info)
{
    struct xdp_statistics stats;
	socklen_t optlen;
	int err;
    int fd = xsk_info->fd;

	optlen = sizeof(stats);
	err = getsockopt(fd, SOL_XDP, XDP_STATISTICS, &stats, &optlen);
	if (err)
		return err;

	if (optlen == sizeof(struct xdp_statistics)) {
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

/*
 * The following functions are shared memory versions of the functions in libxdp.h
 * that are used to access the fill ring and comp ring in the shared memory.
 */
XDP_ALWAYS_INLINE __u32 xsk_prod_nb_free(struct xsk_ring_prod *r, __u32 nb, uintptr_t offset)
{
	__u32 free_entries = r->cached_cons - r->cached_prod;

	if (free_entries >= nb)
		return free_entries;

	/* Refresh the local tail pointer.
	 * cached_cons is r->size bigger than the real consumer pointer so
	 * that this addition can be avoided in the more frequently
	 * executed code that computs free_entries in the beginning of
	 * this function. Without this optimization it whould have been
	 * free_entries = r->cached_prod - r->cached_cons + r->size.
	 */
    __u32 *consumer = reinterpret_cast<__u32 *>(reinterpret_cast<uintptr_t>(r->consumer) + offset);
	r->cached_cons = __atomic_load_n(consumer, __ATOMIC_ACQUIRE);
	r->cached_cons += r->size;
	return r->cached_cons - r->cached_prod;
}

// shared memory version of xsk_ring_prod__reserve() for fill ring
XDP_ALWAYS_INLINE __u32 eTran_fq__reserve(struct xsk_ring_prod *prod, __u32 nb, __u32 *idx, uintptr_t offset)
{
	if (xsk_prod_nb_free(prod, nb, offset) < nb)
		return 0;

	*idx = prod->cached_prod;
	prod->cached_prod += nb;

	return nb;
}

// shared memory version of xsk_ring_prod__submit() for fill ring
XDP_ALWAYS_INLINE void eTran_fq__submit(struct xsk_ring_prod *prod, __u32 nb, uintptr_t offset)
{
	/* Make sure everything has been written to the ring before indicating
	 * this to the kernel by writing the producer pointer.
	 */
    __u32 *producer = reinterpret_cast<__u32*>(reinterpret_cast<uintptr_t>(prod->producer) + offset);
    __atomic_store_n(producer, *producer + nb, __ATOMIC_RELEASE);
}

// shared memory version of xsk_ring_prod__fill_addr() for fill ring
XDP_ALWAYS_INLINE __u64 *eTran_fq__fill_addr(struct xsk_ring_prod *fill,
						  __u32 idx, uintptr_t offset)
{
    void *ring = reinterpret_cast<void*>(reinterpret_cast<uintptr_t>(fill->ring) + offset);
	__u64 *addrs = (__u64 *)ring;

	return &addrs[idx & fill->mask];
}

XDP_ALWAYS_INLINE __u32 xsk_cons_nb_avail(struct xsk_ring_cons *r, __u32 nb, uintptr_t offset)
{
	__u32 entries = r->cached_prod - r->cached_cons;
    __u32 *producer = reinterpret_cast<__u32 *>(reinterpret_cast<uintptr_t>(r->producer) + offset);

	if (entries == 0) {
		r->cached_prod = __atomic_load_n(producer, __ATOMIC_ACQUIRE);
		entries = r->cached_prod - r->cached_cons;
	}

	return (entries > nb) ? nb : entries;
}

// shared memory version of xsk_ring_cons__peek() for comp ring
XDP_ALWAYS_INLINE __u32 eTran_cq__peek(struct xsk_ring_cons *cons, __u32 nb, __u32 *idx, uintptr_t offset)
{
	__u32 entries = xsk_cons_nb_avail(cons, nb, offset);

	if (entries > 0) {
		*idx = cons->cached_cons;
		cons->cached_cons += entries;
	}

	return entries;
}

// shared memory version of xsk_ring_cons__comp_addr() for comp ring
XDP_ALWAYS_INLINE const __u64 * eTran_cq__comp_addr(const struct xsk_ring_cons *comp, __u32 idx, uintptr_t offset)
{
    void *ring = reinterpret_cast<void*>(reinterpret_cast<uintptr_t>(comp->ring) + offset);
	const __u64 *addrs = (const __u64 *)ring;

	return &addrs[idx & comp->mask];
}

// shared memory version of xsk_ring_cons__release() for comp ring
XDP_ALWAYS_INLINE void eTran_cq__release(struct xsk_ring_cons *cons, __u32 nb, uintptr_t offset)
{
	/* Make sure data has been read before indicating we are done
	 * with the entries by updating the consumer pointer.
	 */
    __u32 *consumer = reinterpret_cast<__u32*>(reinterpret_cast<uintptr_t>(cons->consumer) + offset);
    __atomic_store_n(consumer, *consumer + nb, __ATOMIC_RELEASE);
}

static inline void kick_tx(int xsk_fd, struct xsk_ring_prod *tx)
{
    if (xsk_ring_prod__needs_wakeup(tx))
	    sendto(xsk_fd, NULL, 0, MSG_DONTWAIT, NULL, 0);
}

// shared memory version of xsk_ring_prod__needs_wakeup() for fill ring
XDP_ALWAYS_INLINE int eTran_fq__needs_wakeup(const struct xsk_ring_prod *r, uintptr_t offset)
{
    __u32 *flags = reinterpret_cast<__u32*>(reinterpret_cast<uintptr_t>(r->flags) + offset);
	return *flags & XDP_RING_NEED_WAKEUP;
}
static inline void kick_fq(int xsk_fd, struct xsk_ring_prod *fq, uintptr_t offset)
{
    if (eTran_fq__needs_wakeup(fq, offset))
        recvfrom(xsk_fd, NULL, 0, MSG_DONTWAIT, NULL, NULL);
}