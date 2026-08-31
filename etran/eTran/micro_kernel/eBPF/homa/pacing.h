#pragma once

#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>

#include "ebpf_fib.h"
#include "ebpf_queue.h"
#include "ebpf_utils.h"
#include "eTran_defs.h"
#include "rpc.h"

// #define DISABLE_PACER_FIFO

/* Maximum number of packets that can be sent in one batch */
#define MAX_PKT_SEND 16
/* Maximum number of RPC that can be "seen" in the throttle list */
#define MAX_RPC_CHECK 4

struct {
    __uint(type, BPF_MAP_TYPE_PKT_QUEUE);
    __uint(key_size, sizeof(__u32));
    __uint(value_size, sizeof(__u32));
    /* Maximum queued packets for one RPC */
    __uint(max_entries, MAX_QUEUE_SIZE);
    /* Maximum queued RPCs */
    __uint(map_extra, MAX_BUCKET_SIZE);
} homa_throttle_list SEC(".maps");

struct pacer_wrapper_t {
    struct bpf_timer t;
    __u8 ready;
};
/***************** NET_TX_ACTION trigger *****************/
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct pacer_wrapper_t);
} homa_nettx_map SEC(".maps");
/********************* PACER trigger *********************/
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct pacer_wrapper_t);
} homa_pacer_map SEC(".maps");

struct pacer_next_info_t {
    __u32 bytes_remaining;
    __u64 rpcid;
    __u16 local_port;
};

SEC(".bss.pacer")
__u64 __attribute__((__aligned__(CACHE_LINE_SIZE))) pacer_lock;
struct pacer_next_info_t __attribute__((__aligned__(CACHE_LINE_SIZE))) pacer_next_info;
__u64 __attribute__((__aligned__(CACHE_LINE_SIZE))) next_pacer_round;

#define NEXT_RPC(next_pacer_round) \
do { \
    next_pacer_round++; \
    if (next_pacer_round >= MAX_RPC_CHECK) \
        next_pacer_round = 0; \
} while (0)

/**
 * @brief Try to grab the pacer lock
 * @return 1 if success, 0 if failed
 */
static __always_inline int try_pacer_lock(void)
{
    if (__sync_bool_compare_and_swap(&pacer_lock, (__u64)0, (__u64)1)) {
        return 1;
    }
    return 0;
}
/**
 * @brief Release the pacer lock
 */
static __always_inline void release_pacer_lock(void)
{
    __sync_fetch_and_sub(&pacer_lock, (__u64)1);
}

/**
 * @brief do fib lookup and enqueue the packet to the throttle list
 */
static __always_inline 
int enqueue_pkt_to_rl(struct xdp_md *ctx, __u64 rpc_qid, struct ethhdr *eth, struct iphdr *iph)
{
    int err = fib_lookup(ctx, eth, iph);
    if (unlikely(err))
        log_err("bpf_fib_lookup failed, check routing table in kernel");
    return bpf_redirect_map(&homa_throttle_list, rpc_qid, 0);
}

/**
 * @brief callback function executed by pacer
 */
static int pacer_timer_cb(void *map, int *map_key, struct pacer_wrapper_t *pacer_wrapper)
{
    struct bpf_map *queue_map = (void *)&homa_throttle_list;
    struct bpf_timer *pacer = &pacer_wrapper->t;
    struct bpf_dynptr ptr;
    struct xdp_frame *pkt;
    struct data_header *d;
    struct rpc_state_cc __kptr *cc_node = NULL;
    struct rpc_state_cc __kptr *fifo_cc_node = NULL;
    struct bpf_rb_node *rb_node = NULL;
    struct rpc_state *rpc_slot = NULL;
    struct rpc_key_t hkey = {0};
    int work = 0;
    int nr_tx = 0;
    __u64 rpc_qid = MAX_BUCKET_SIZE;
    bool need_enqueue = true;
    bool do_fifo = false;

    if (!try_pacer_lock()) {
        bpf_timer_start(pacer, 0, BPF_F_TIMER_PACER | BPF_F_TIMER_PACER_WAKEUP);
        return 0;
    }

    if (!atomic_read(&nr_rpc_in_throttle)) {
        /* there is no RPC in throttle list */
        next_pacer_round = 0;
        release_pacer_lock();
        return 0;
    }

    /* starts from the head of the throttle list */
    if (next_pacer_round == 0) {
        pacer_next_info.bytes_remaining = 0;
        pacer_next_info.rpcid = 0;
        pacer_next_info.local_port = 0;
    }

#ifndef DISABLE_PACER_FIFO
    pacer_nonfifo_left -= PACER_FIFO_FRACTION;
    if (pacer_nonfifo_left <= 0 || next_pacer_round) {
        fifo_cc_node = bpf_obj_new(typeof(*fifo_cc_node));
        if (unlikely(!fifo_cc_node))
        {
            log_panic("bpf_obj_new failed");
            release_pacer_lock();
            return 0;
        }
    }
    if (pacer_nonfifo_left <= 0) {
        pacer_nonfifo_left += PACER_FIFO_INCREMENT;
        do_fifo = true;
    }
#else
    if (next_pacer_round)
    {
        fifo_cc_node = bpf_obj_new(typeof(*fifo_cc_node));
        if (!fifo_cc_node)
        {
            log_panic("bpf_obj_new failed");
            release_pacer_lock();
            return 0;
        }
    }
#endif

    THROTTLE_LOCK();

#ifndef DISABLE_PACER_FIFO
    if (fifo_cc_node && do_fifo) {
        /* select the oldest RPC to transmit */
        fifo_cc_node->birth = POISON_64;
        rb_node = bpf_rbtree_search_less(&troot, &fifo_cc_node->rbtree_link, less_birth_pacer);
    }
    else if (fifo_cc_node) {
        /* select the next ready RPC to transmit (not necessarily the highest priority) */
        fifo_cc_node->bytes_remaining = pacer_next_info.bytes_remaining;
        fifo_cc_node->hkey.rpcid = pacer_next_info.rpcid;
        fifo_cc_node->hkey.local_port = pacer_next_info.local_port;
        rb_node = bpf_rbtree_lower_bound(&troot, &fifo_cc_node->rbtree_link, srpt_less_pacer);
    }
    else {
        /* select the highest priority RPC to transmit */
        rb_node = bpf_rbtree_first(&troot);
    }
#else
    if (fifo_cc_node) {
        /* select the next ready RPC to transmit (not necessarily the highest priority) */
        fifo_cc_node->bytes_remaining = pacer_next_info.bytes_remaining;
        fifo_cc_node->hkey.rpcid = pacer_next_info.rpcid;
        fifo_cc_node->hkey.local_port = pacer_next_info.local_port;
        rb_node = bpf_rbtree_lower_bound(&troot, &fifo_cc_node->rbtree_link, srpt_less_pacer);
    }
    else {
        /* select the highest priority RPC to transmit */
        rb_node = bpf_rbtree_first(&troot);
    }
#endif

    if (unlikely(!rb_node)) {
        THROTTLE_UNLOCK();
        next_pacer_round = 0;
        bpf_timer_start(pacer, 0, BPF_F_TIMER_PACER | BPF_F_TIMER_PACER_WAKEUP);
        release_pacer_lock();
        return 0;
    }

    cc_node = container_of(rb_node, struct rpc_state_cc, rbtree_link);
    rb_node = bpf_rbtree_remove(&troot, &cc_node->rbtree_link);
    if (unlikely(!rb_node)) { /* this should never happen */
        THROTTLE_UNLOCK();
        
        NEXT_RPC(next_pacer_round);

        bpf_timer_start(pacer, 0, BPF_F_TIMER_PACER | BPF_F_TIMER_PACER_WAKEUP);
        release_pacer_lock();
        return 0;
    }
    cc_node = container_of(rb_node, struct rpc_state_cc, rbtree_link);
    atomic_dec(&nr_rpc_in_throttle);
    THROTTLE_UNLOCK();

    /* lookup the corresponding RPC state */
    hkey.rpcid = cc_node->hkey.rpcid;
    hkey.local_port = cc_node->hkey.local_port;
    hkey.remote_port = cc_node->hkey.remote_port;
    hkey.remote_ip = cc_node->hkey.remote_ip;
    rpc_slot = bpf_map_lookup_elem(&rpc_tbl, &hkey);
    if (unlikely(!rpc_slot)) {
        bpf_obj_drop(cc_node);
        
        NEXT_RPC(next_pacer_round);
        
        bpf_timer_start(pacer, 0, BPF_F_TIMER_PACER | BPF_F_TIMER_PACER_WAKEUP);
        release_pacer_lock();
        return 0;
    }
    
    rpc_qid = rpc_slot->qid;

    /* verify RPC state */
    if (unlikely(rpc_slot->state != BPF_RPC_OUTGOING || 
        rpc_qid == MAX_BUCKET_SIZE || rpc_slot->next_xmit_offset >= rpc_slot->message_length))
    {
        bpf_obj_drop(cc_node);
        
        NEXT_RPC(next_pacer_round);
        
        bpf_timer_start(pacer, 0, BPF_F_TIMER_PACER | BPF_F_TIMER_PACER_WAKEUP);
        release_pacer_lock();
        return 0;
    }

    while (1)
    {
        if (++work > MAX_PKT_SEND - 1)
            break;
        
        pkt = pkt_queue_dequeue(queue_map, rpc_qid, NULL);
        if (pkt == NULL)
            break;

        if (unlikely(bpf_dynptr_from_xdp_frame(pkt, 0, &ptr))) {
            log_panic("bpf_dynptr_from_xdp_frame failed");
            break;
        }

        d = bpf_dynptr_slice_rdwr(&ptr, sizeof(struct ethhdr) + sizeof(struct iphdr), NULL, sizeof(*d));
        if (unlikely(!d)) {
            log_panic("bpf_dynptr_slice_rdwr failed");
            break;
        }

        if (bpf_ntohl(d->seg.offset) + bpf_ntohl(d->seg.segment_length) > rpc_slot->cc.granted)
        {
            pkt_queue_enqueue(queue_map, pkt, rpc_qid);
            need_enqueue = false;
            break;
        }

        d->incoming = bpf_ntohl(rpc_slot->cc.granted);

        if (unlikely(bpf_packet_send(pkt, pkt->tx_ifindex, 0))) {
            log_err("bpf_packet_send failed");
            break;
        }

        /* no other CPU would contend for this modification */
        rpc_slot->next_xmit_offset = bpf_ntohl(d->seg.offset) + bpf_ntohl(d->seg.segment_length);

        nr_tx++;
    }

    if (nr_tx) {
        /* flush packets to NIC */
        bpf_packet_flush();
        __sync_fetch_and_sub(&rpc_slot->nr_pkts_in_rl, nr_tx);
        cc_node->bytes_remaining = rpc_slot->message_length - rpc_slot->next_xmit_offset;
    }

    /* Is this RPC completed? */
    if (rpc_slot->next_xmit_offset >= rpc_slot->message_length)
        need_enqueue = false;

    /* This RPC's grant is ready, but we haven't transmit all, so reinsert it to throttle list */
    if (need_enqueue) {
        /* we still select it as the next RPC to transmit */
        pacer_next_info.bytes_remaining = cc_node->bytes_remaining;
        pacer_next_info.rpcid = cc_node->hkey.rpcid;
        pacer_next_info.local_port = cc_node->hkey.local_port;

        THROTTLE_LOCK();
        if (bpf_rbtree_add(&troot, &cc_node->rbtree_link, srpt_less_pacer) == 0)
            atomic_inc(&nr_rpc_in_throttle);
        THROTTLE_UNLOCK();
        
        NEXT_RPC(next_pacer_round);
        
        bpf_timer_start(pacer, 0, BPF_F_TIMER_PACER | BPF_F_TIMER_PACER_WAKEUP);
        release_pacer_lock();
        return 0;
    }
    else {
        pacer_next_info.bytes_remaining = cc_node->bytes_remaining;
        pacer_next_info.rpcid = cc_node->hkey.rpcid;
        pacer_next_info.local_port = cc_node->hkey.local_port + 1;
    }

    bpf_obj_drop(cc_node);

    int pacer_flag = 0;
    if (atomic_read(&nr_rpc_in_throttle)) {
        pacer_flag = BPF_F_TIMER_PACER_WAKEUP;
        NEXT_RPC(next_pacer_round);
    }
    else
        next_pacer_round = 0;

    bpf_timer_start(pacer, 0, BPF_F_TIMER_PACER | pacer_flag);
    release_pacer_lock();
    return 0;
}

/**
 * @brief callback function executed by NET_TX_ACTION
 */
static int nettx_timer_cb(void *map, int *map_key, struct pacer_wrapper_t *pacer_wrapper)
{
    struct bpf_map *queue_map = (void *)&homa_throttle_list;
    struct bpf_timer *pacer = &pacer_wrapper->t;
    struct bpf_dynptr ptr;
    struct xdp_frame *pkt;
    struct data_header *d;
    struct rpc_state_cc __kptr *cc_node = NULL;
    struct rpc_state_cc __kptr *fifo_cc_node = NULL;
    struct bpf_rb_node *rb_node = NULL;
    struct rpc_state *rpc_slot = NULL;
    struct rpc_key_t hkey = {0};
    int work = 0;
    int nr_tx = 0;
    __u64 rpc_qid = MAX_BUCKET_SIZE;
    bool need_enqueue = true;
    bool do_fifo = false;

    if (!try_pacer_lock()) {
        /* pacer is running */
        return 0;
    }

    if (!atomic_read(&nr_rpc_in_throttle)) {
        /* there is no RPC in throttle list */
        next_pacer_round = 0;
        release_pacer_lock();
        return 0;
    }

    /* starts from the head of the throttle list */
    if (next_pacer_round == 0) {
        pacer_next_info.bytes_remaining = 0;
        pacer_next_info.rpcid = 0;
        pacer_next_info.local_port = 0;
    }

#ifndef DISABLE_PACER_FIFO
    pacer_nonfifo_left -= PACER_FIFO_FRACTION;
    if (pacer_nonfifo_left <= 0 || next_pacer_round) {
        fifo_cc_node = bpf_obj_new(typeof(*fifo_cc_node));
        if (unlikely(!fifo_cc_node))
        {
            log_panic("bpf_obj_new failed");
            release_pacer_lock();
            return 0;
        }
    }
    if (pacer_nonfifo_left <= 0) {
        pacer_nonfifo_left += PACER_FIFO_INCREMENT;
        do_fifo = true;
    }
#else
    if (next_pacer_round)
    {
        fifo_cc_node = bpf_obj_new(typeof(*fifo_cc_node));
        if (!fifo_cc_node)
        {
            log_panic("bpf_obj_new failed");
            release_pacer_lock();
            return 0;
        }
    }
#endif

    THROTTLE_LOCK();

#ifndef DISABLE_PACER_FIFO
    if (fifo_cc_node && do_fifo) {
        /* select the oldest RPC to transmit */
        fifo_cc_node->birth = POISON_64;
        rb_node = bpf_rbtree_search_less(&troot, &fifo_cc_node->rbtree_link, less_birth_pacer);
    }
    else if (fifo_cc_node) {
        /* select the next ready RPC to transmit (not necessarily the highest priority) */
        fifo_cc_node->bytes_remaining = pacer_next_info.bytes_remaining;
        fifo_cc_node->hkey.rpcid = pacer_next_info.rpcid;
        fifo_cc_node->hkey.local_port = pacer_next_info.local_port;
        rb_node = bpf_rbtree_lower_bound(&troot, &fifo_cc_node->rbtree_link, srpt_less_pacer);
    }
    else {
        /* select the highest priority RPC to transmit */
        rb_node = bpf_rbtree_first(&troot);
    }
#else
    if (fifo_cc_node) {
        /* select the next ready RPC to transmit (not necessarily the highest priority) */
        fifo_cc_node->bytes_remaining = pacer_next_info.bytes_remaining;
        fifo_cc_node->hkey.rpcid = pacer_next_info.rpcid;
        fifo_cc_node->hkey.local_port = pacer_next_info.local_port;
        rb_node = bpf_rbtree_lower_bound(&troot, &fifo_cc_node->rbtree_link, srpt_less_pacer);
    }
    else {
        /* select the highest priority RPC to transmit */
        rb_node = bpf_rbtree_first(&troot);
    }
#endif

    if (unlikely(!rb_node)) {
        THROTTLE_UNLOCK();
        next_pacer_round = 0;
        bpf_timer_start(pacer, 0, BPF_F_TIMER_PACER | BPF_F_TIMER_PACER_WAKEUP);
        release_pacer_lock();
        return 0;
    }

    cc_node = container_of(rb_node, struct rpc_state_cc, rbtree_link);
    rb_node = bpf_rbtree_remove(&troot, &cc_node->rbtree_link);
    if (unlikely(!rb_node)) { /* this should never happen */
        THROTTLE_UNLOCK();
        
        NEXT_RPC(next_pacer_round);

        bpf_timer_start(pacer, 0, BPF_F_TIMER_PACER | BPF_F_TIMER_PACER_WAKEUP);
        release_pacer_lock();
        return 0;
    }
    cc_node = container_of(rb_node, struct rpc_state_cc, rbtree_link);
    atomic_dec(&nr_rpc_in_throttle);
    THROTTLE_UNLOCK();

    /* lookup the corresponding RPC state */
    hkey.rpcid = cc_node->hkey.rpcid;
    hkey.local_port = cc_node->hkey.local_port;
    hkey.remote_port = cc_node->hkey.remote_port;
    hkey.remote_ip = cc_node->hkey.remote_ip;
    rpc_slot = bpf_map_lookup_elem(&rpc_tbl, &hkey);
    if (unlikely(!rpc_slot)) {
        bpf_obj_drop(cc_node);
        
        NEXT_RPC(next_pacer_round);
        
        bpf_timer_start(pacer, 0, BPF_F_TIMER_PACER | BPF_F_TIMER_PACER_WAKEUP);
        release_pacer_lock();
        return 0;
    }
    
    rpc_qid = rpc_slot->qid;

    /* verify RPC state */
    if (unlikely(rpc_slot->state != BPF_RPC_OUTGOING || 
        rpc_qid == MAX_BUCKET_SIZE || rpc_slot->next_xmit_offset >= rpc_slot->message_length))
    {
        bpf_obj_drop(cc_node);
        
        NEXT_RPC(next_pacer_round);
        
        bpf_timer_start(pacer, 0, BPF_F_TIMER_PACER | BPF_F_TIMER_PACER_WAKEUP);
        release_pacer_lock();
        return 0;
    }

    while (1)
    {
        if (++work > MAX_PKT_SEND - 1)
            break;
        
        pkt = pkt_queue_dequeue(queue_map, rpc_qid, NULL);
        if (pkt == NULL)
            break;

        if (unlikely(bpf_dynptr_from_xdp_frame(pkt, 0, &ptr))) {
            log_panic("bpf_dynptr_from_xdp_frame failed");
            break;
        }

        d = bpf_dynptr_slice_rdwr(&ptr, sizeof(struct ethhdr) + sizeof(struct iphdr), NULL, sizeof(*d));
        if (unlikely(!d)) {
            log_panic("bpf_dynptr_slice_rdwr failed");
            break;
        }

        if (bpf_ntohl(d->seg.offset) + bpf_ntohl(d->seg.segment_length) > rpc_slot->cc.granted)
        {
            pkt_queue_enqueue(queue_map, pkt, rpc_qid);
            need_enqueue = false;
            break;
        }

        d->incoming = bpf_ntohl(rpc_slot->cc.granted);

        if (unlikely(bpf_packet_send(pkt, pkt->tx_ifindex, 0))) {
            log_err("bpf_packet_send failed");
            break;
        }

        /* no other CPU would contend for this modification */
        rpc_slot->next_xmit_offset = bpf_ntohl(d->seg.offset) + bpf_ntohl(d->seg.segment_length);

        nr_tx++;
    }

    if (nr_tx) {
        /* flush packets to NIC */
        bpf_packet_flush();
        __sync_fetch_and_sub(&rpc_slot->nr_pkts_in_rl, nr_tx);
        cc_node->bytes_remaining = rpc_slot->message_length - rpc_slot->next_xmit_offset;
    }

    /* Is this RPC completed? */
    if (rpc_slot->next_xmit_offset >= rpc_slot->message_length)
        need_enqueue = false;

    /* This RPC's grant is ready, but we haven't transmit all, so reinsert it to throttle list */
    if (need_enqueue) {
        /* we still select it as the next RPC to transmit */
        pacer_next_info.bytes_remaining = cc_node->bytes_remaining;
        pacer_next_info.rpcid = cc_node->hkey.rpcid;
        pacer_next_info.local_port = cc_node->hkey.local_port;

        THROTTLE_LOCK();
        if (bpf_rbtree_add(&troot, &cc_node->rbtree_link, srpt_less_pacer) == 0)
            atomic_inc(&nr_rpc_in_throttle);
        THROTTLE_UNLOCK();
        
        NEXT_RPC(next_pacer_round);
        
        bpf_timer_start(pacer, 0, BPF_F_TIMER_PACER | BPF_F_TIMER_PACER_WAKEUP);
        release_pacer_lock();
        return 0;
    }
    else {
        pacer_next_info.bytes_remaining = cc_node->bytes_remaining;
        pacer_next_info.rpcid = cc_node->hkey.rpcid;
        pacer_next_info.local_port = cc_node->hkey.local_port + 1;
    }

    bpf_obj_drop(cc_node);

    int pacer_flag = 0;
    if (atomic_read(&nr_rpc_in_throttle)) {
        pacer_flag = BPF_F_TIMER_PACER_WAKEUP;
        NEXT_RPC(next_pacer_round);
    }
    else
        next_pacer_round = 0;

    bpf_timer_start(pacer, 0, BPF_F_TIMER_PACER | pacer_flag);
    release_pacer_lock();
    return 0;
}

/**
 * @brief kick the NET_TX_ACTION timer
 */
static __always_inline void kick_nettx(void)
{
    struct pacer_wrapper_t *nettx_wrapper = NULL;
    struct bpf_timer *pacer = NULL;
    bool contention = false;

    nettx_wrapper = bpf_map_lookup_elem(&homa_nettx_map, &(__u32){0});
    if (unlikely(!nettx_wrapper)) {
        log_panic("nettx_wrapper not found");
        return;
    }
    pacer = &nettx_wrapper->t;
    if (unlikely(nettx_wrapper->ready == 0))
    {
        /* this lock ensure that only we can initialize the pacer */
        THROTTLE_LOCK();
        
        if (nettx_wrapper->ready == 0)
            nettx_wrapper->ready = 1;
        else
            contention = true;
        
        THROTTLE_UNLOCK();
        if (!contention)
        {
            /* first time to initialize the pacer */
            bpf_timer_init(pacer, &homa_nettx_map, CLOCK_MONOTONIC | BPF_F_TIMER_NET_TX);
            bpf_timer_set_callback(pacer, nettx_timer_cb);
        }
    }
    /* raise softirq */
    bpf_timer_start(pacer, 0, BPF_F_TIMER_IMMEDIATE);
}

/**
 * @brief kick the PACER timer
 */
static __always_inline void kick_pacer(void)
{
    struct pacer_wrapper_t *pacer_wrapper = NULL;
    struct bpf_timer *pacer = NULL;
    bool contention = false;
    
    pacer_wrapper = bpf_map_lookup_elem(&homa_pacer_map, &(__u32){0});
    if (unlikely(!pacer_wrapper)) {
        log_panic("pacer_wrapper not found");
        return;
    }
    
    pacer = &pacer_wrapper->t;
    
    if (unlikely(pacer_wrapper->ready == 0))
    {
        /* this lock ensure that only we can initialize the pacer */
        THROTTLE_LOCK();
        
        if (pacer_wrapper->ready == 0)
            pacer_wrapper->ready = 1;
        else
            contention = true;
        
        THROTTLE_UNLOCK();
        if (!contention)
        {
            /* first time to initialize the pacer */
            bpf_timer_init(pacer, &homa_pacer_map, CLOCK_MONOTONIC | BPF_F_TIMER_NET_TX);
            bpf_timer_set_callback(pacer, pacer_timer_cb);
            next_pacer_round = 0;
        }
    }
    
    /* wakeup pacer kthread */
    bpf_timer_start(pacer, 0, BPF_F_TIMER_PACER_WAKEUP | BPF_F_TIMER_PACER);
}

/**
 * @brief check if the NIC is busy
 * @return true if busy, false otherwise
 */
static __always_inline bool nic_busy(void)
{
    __u64 now = bpf_ktime_get_ns();
    __u64 idle = __sync_fetch_and_add(&link_idle_time, 0);
    if (now + max_nic_queue_ns / 2 < idle)
        return true;
    return false;
}

/**
 * @brief help the pacer to transmit packets
 */
static __always_inline void help_pacer(void)
{
    if (nic_busy())
        return;
    kick_nettx();
}