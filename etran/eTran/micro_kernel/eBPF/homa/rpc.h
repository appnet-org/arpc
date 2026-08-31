#pragma once

#include <linux/bpf.h>
#include <linux/err.h>
#include <linux/types.h>
#include <bpf/bpf_helpers.h>

#include <intf/intf_ebpf.h>
#include "ebpf_kfunc.h"
#include "ebpf_utils.h"

#include "eTran_defs.h"
#include "bss_data_defs.h"
#include "homa.h"

// pacing.h
static __always_inline void kick_pacer(void);

/*
 * For rpc rbtree:
 *  key is tree_id ( = 0), peer_id, bytes_remaining, hkey
 *
 * For peer rbtree:
 *  key is tree_id ( = 1), bytes_remaining, peer_id
 */
static bool srpt_less_rpc(struct bpf_rb_node *node_a, const struct bpf_rb_node *node_b)
{
    struct rpc_state_cc *a;
    struct rpc_state_cc *b;
    int ret = 0;

    a = container_of(node_a, struct rpc_state_cc, rbtree_link);
    b = container_of(node_b, struct rpc_state_cc, rbtree_link);

    if (a->tree_id != b->tree_id)
        return a->tree_id < b->tree_id;

    if (a->peer_id != b->peer_id)
        return a->peer_id < b->peer_id;

    if (a->bytes_remaining != b->bytes_remaining)
        return a->bytes_remaining < b->bytes_remaining;

    ret = __builtin_memcmp(&a->hkey, &b->hkey, sizeof(struct rpc_key_t));

    if (ret <= 0)
        return 1;
    else
        return 0;
}

static bool srpt_less_peer(struct bpf_rb_node *node_a, const struct bpf_rb_node *node_b)
{
    struct rpc_state_cc *a;
    struct rpc_state_cc *b;

    a = container_of(node_a, struct rpc_state_cc, rbtree_link);
    b = container_of(node_b, struct rpc_state_cc, rbtree_link);

    if (a->tree_id != b->tree_id)
        return a->tree_id < b->tree_id;

    if (a->bytes_remaining != b->bytes_remaining)
        return a->bytes_remaining < b->bytes_remaining;

    if (a->peer_id != b->peer_id)
        return a->peer_id < b->peer_id;

    return 1;
}

struct dead_client_rpc_info {
    __u32 remote_ip;
    __u16 remote_port;
    __u16 local_port;
    __u64 rpcid;
};

struct
{
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, struct rpc_key_t);
    __type(value, struct rpc_state);
    __uint(max_entries, MAX_RPC_TBL_SIZE);
} rpc_tbl SEC(".maps");

struct
{
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, __u16);
    __type(value, struct target_xsk);
    __uint(max_entries, MAX_SERVER_PORT);
} port_tbl SEC(".maps");

// TODO: add new peer in slow-path
/* Store dead client rpc for freeing remote server rpc state */
#define QUEUE_SIZE 2048
struct ack_tbl
{
    __uint(type, BPF_MAP_TYPE_QUEUE);
    __type(value, struct dead_client_rpc_info);
    __uint(max_entries, QUEUE_SIZE);
} peer_ack_tbl_0 SEC(".maps"), peer_ack_tbl_1 SEC(".maps"), peer_ack_tbl_2 SEC(".maps"), peer_ack_tbl_3 SEC(".maps"),
    peer_ack_tbl_4 SEC(".maps"), peer_ack_tbl_5 SEC(".maps"), peer_ack_tbl_6 SEC(".maps"), peer_ack_tbl_7 SEC(".maps"),
    peer_ack_tbl_8 SEC(".maps"), peer_ack_tbl_9 SEC(".maps"), peer_ack_tbl_10 SEC(".maps"),
    peer_ack_tbl_11 SEC(".maps"), peer_ack_tbl_12 SEC(".maps"), peer_ack_tbl_13 SEC(".maps"),
    peer_ack_tbl_14 SEC(".maps"), peer_ack_tbl_15 SEC(".maps");

struct
{
    __uint(type, BPF_MAP_TYPE_ARRAY_OF_MAPS);
    __uint(max_entries, MAX_PEER);
    __type(key, __u32);
    __array(values, struct ack_tbl);
} peer_tbl SEC(".maps") = {.values = {&peer_ack_tbl_0, &peer_ack_tbl_1, &peer_ack_tbl_2, &peer_ack_tbl_3,
                                      &peer_ack_tbl_4, &peer_ack_tbl_5, &peer_ack_tbl_6, &peer_ack_tbl_7,
                                      &peer_ack_tbl_8, &peer_ack_tbl_9, &peer_ack_tbl_10, &peer_ack_tbl_11,
                                      &peer_ack_tbl_12, &peer_ack_tbl_13, &peer_ack_tbl_14, &peer_ack_tbl_15}};

static __always_inline
__u64 local_id(__u64 recv_rpcid)
{
    return recv_rpcid ^ 1;
}

static __always_inline
int rpc_is_client(__u64 id)
{
    return (id & 1) == 0;
}

static __always_inline
void enqueue_dead_crpc(__u32 remote_ip, __u16 remote_port, __u16 local_port, __u64 rpcid)
{
    struct dead_client_rpc_info dead_client = {
        .remote_ip = remote_ip,
        .remote_port = remote_port,
        .local_port = local_port,
        .rpcid = rpcid,
    };
    __u32 peer_id = get_peerid(remote_ip);
    struct ack_tbl *ack_tbl = bpf_map_lookup_elem(&peer_tbl, &peer_id);
    if (unlikely(!ack_tbl))
    {   /* this should never happen */
        log_panic("bpf_map_lookup_elem for peer_tbl failed.");
        return;
    }
    bpf_map_push_elem(ack_tbl, &dead_client, BPF_EXIST);
}

static __always_inline
int dequeue_dead_crpc(__u32 remote_ip, struct dead_client_rpc_info *dead_crpc)
{
    int ret;
    __u32 peer_id = get_peerid(remote_ip);
    struct ack_tbl *ack_tbl = bpf_map_lookup_elem(&peer_tbl, &peer_id);
    if (unlikely(!ack_tbl)) { // this should never happen
        log_panic("bpf_map_lookup_elem for peer_tbl failed.");
        return -1;
    }
    ret = bpf_map_pop_elem(ack_tbl, dead_crpc);
    return ret ? -1 : 0;
}

#define inc_round(rpc_slot, round, last_round)                                                                     \
do {                                                                                                               \
    if (round < last_round || !(rpc_slot->bit_width & 63))                                                         \
    {                                                                                                              \
        if (rpc_slot->bitmap_##round == ~(__u64)0)                                                                 \
            rpc_slot->bitmap_round++;                                                                              \
    }                                                                                                              \
    else if (round == last_round)                                                                                  \
    {                                                                                                              \
        if (rpc_slot->bitmap_##round == ((__u64)1 << (rpc_slot->bit_width & 63)) - 1)                              \
            rpc_slot->bitmap_round++;                                                                              \
    }                                                                                                              \
} while (0)

#define and_bitmap(round, seq) (rpc_slot->bitmap_##round & ((__u64)1 << (seq & 63)))

#define or_bitmap(round, seq) (rpc_slot->bitmap_##round |= ((__u64)1 << (seq & 63)))

#define clear_all_bitmaps(rpc_slot)                                                                                    \
do {                                                                                                                    \
    rpc_slot->bitmap_0 = 0;                                                                                            \
    rpc_slot->bitmap_1 = 0;                                                                                            \
    rpc_slot->bitmap_2 = 0;                                                                                            \
    rpc_slot->bitmap_3 = 0;                                                                                            \
    rpc_slot->bitmap_4 = 0;                                                                                            \
    rpc_slot->bitmap_5 = 0;                                                                                            \
    rpc_slot->bitmap_6 = 0;                                                                                            \
    rpc_slot->bitmap_7 = 0;                                                                                            \
    rpc_slot->bitmap_8 = 0;                                                                                            \
    rpc_slot->bitmap_9 = 0;                                                                                            \
    rpc_slot->bitmap_10 = 0;                                                                                           \
    rpc_slot->bitmap_11 = 0;                                                                                           \
    rpc_slot->bitmap_round = 0;                                                                                        \
} while (0)

/**
 * @brief Set the bitmap for the given sequence number
 * @return 0: set success 1: set success and RPC is complete -1: set error
 */
static __always_inline
int set_bitmap(struct rpc_state *rpc_slot, __u16 seq)
{
    /* bit_width starts from 1 */
    __u16 tmp = rpc_slot->bit_width;
    if (tmp > 0)
        tmp--;

    __u8 last_round = tmp / 64;
    __u8 round = seq / 64;
    
    if (unlikely(round > 11))
        return -1;

    if (round == 0 && and_bitmap(0, seq))
        return -1;
    else if (round == 1 && and_bitmap(1, seq))
        return -1;
    else if (round == 2 && and_bitmap(2, seq))
        return -1;
    else if (round == 3 && and_bitmap(3, seq))
        return -1;
    else if (round == 4 && and_bitmap(4, seq))
        return -1;
    else if (round == 5 && and_bitmap(5, seq))
        return -1;
    else if (round == 6 && and_bitmap(6, seq))
        return -1;
    else if (round == 7 && and_bitmap(7, seq))
        return -1;
    else if (round == 8 && and_bitmap(8, seq))
        return -1;
    else if (round == 9 && and_bitmap(9, seq))
        return -1;
    else if (round == 10 && and_bitmap(10, seq))
        return -1;
    else if (round == 11 && and_bitmap(11, seq))
        return -1;

    if (round == 0)
    {
        or_bitmap(0, seq);
        inc_round(rpc_slot, 0, last_round);
    }
    else if (round == 1)
    {
        or_bitmap(1, seq);
        inc_round(rpc_slot, 1, last_round);
    }
    else if (round == 2)
    {
        or_bitmap(2, seq);
        inc_round(rpc_slot, 2, last_round);
    }
    else if (round == 3)
    {
        or_bitmap(3, seq);
        inc_round(rpc_slot, 3, last_round);
    }
    else if (round == 4)
    {
        or_bitmap(4, seq);
        inc_round(rpc_slot, 4, last_round);
    }
    else if (round == 5)
    {
        or_bitmap(5, seq);
        inc_round(rpc_slot, 5, last_round);
    }
    else if (round == 6)
    {
        or_bitmap(6, seq);
        inc_round(rpc_slot, 6, last_round);
    }
    else if (round == 7)
    {
        or_bitmap(7, seq);
        inc_round(rpc_slot, 7, last_round);
    }
    else if (round == 8)
    {
        or_bitmap(8, seq);
        inc_round(rpc_slot, 8, last_round);
    }
    else if (round == 9)
    {
        or_bitmap(9, seq);
        inc_round(rpc_slot, 9, last_round);
    }
    else if (round == 10)
    {
        or_bitmap(10, seq);
        inc_round(rpc_slot, 10, last_round);
    }
    else if (round == 11)
    {
        or_bitmap(11, seq);
        inc_round(rpc_slot, 11, last_round);
    }

    return rpc_slot->bitmap_round == tmp / 64 + 1;
}

static bool srpt_less_pacer(struct bpf_rb_node *node_a, const struct bpf_rb_node *node_b)
{
    struct rpc_state_cc *a;
    struct rpc_state_cc *b;

    a = container_of(node_a, struct rpc_state_cc, rbtree_link);
    b = container_of(node_b, struct rpc_state_cc, rbtree_link);

    if (a->bytes_remaining != b->bytes_remaining)
        return a->bytes_remaining < b->bytes_remaining;

    if (a->hkey.rpcid != b->hkey.rpcid)
        return a->hkey.rpcid < b->hkey.rpcid;

    if (a->hkey.local_port != b->hkey.local_port)
        return a->hkey.local_port < b->hkey.local_port;

    return 1;
}

/**
 * @brief update grant list for cached rpc, can be called by XDP or XDP_GEN
 */
static __always_inline
void update_grant_for_cached_rpc(__u32 cpu)
{
    if (cache_has_rpc[cpu] == 0)
        return;

    cache_has_rpc[cpu] = 0;
    update_delay_packets[cpu] = 0;

    struct rpc_key_t hkey = cache_rpc[cpu];
    struct rpc_state *rpc_slot = NULL;
    struct rpc_state_cc *cc_node_t0 = NULL;
    struct rpc_state_cc *search_cc_node = NULL;
    struct rpc_state_cc *old_cc_node_t0 = NULL;
    struct rpc_state_cc *cc_node_t1 = NULL;
    struct bpf_rb_node *rb_node = NULL;
    __u32 old_br = POISON_32, new_br = POISON_32;

    rpc_slot = bpf_map_lookup_elem(&rpc_tbl, &hkey);
    if (unlikely(!rpc_slot)) return;

    if (rpc_slot->cc.incoming >= rpc_slot->message_length)
        return;

    cc_node_t0 = bpf_obj_new(typeof(*cc_node_t0));
    if (unlikely(!cc_node_t0)) {
        log_err("bpf_obj_new for cc_node_t0 failed.");
        return;
    }

    cc_node_t1 = bpf_obj_new(typeof(*cc_node_t1));
    if (unlikely(!cc_node_t1)) {
        log_err("bpf_obj_new for cc_node_t1 failed.");
        bpf_obj_drop(cc_node_t0);
        return;
    }

    search_cc_node = bpf_refcount_acquire(cc_node_t1);
    if (unlikely(!search_cc_node)) {
        log_err("bpf_refcount_acquire for search_cc_node failed.");
        bpf_obj_drop(cc_node_t0);
        bpf_obj_drop(cc_node_t1);
        return;
    }

    old_cc_node_t0 = bpf_refcount_acquire(search_cc_node);
    if (unlikely(!old_cc_node_t0)) {
        log_err("bpf_refcount_acquire for old_cc_node_t0 failed.");
        bpf_obj_drop(cc_node_t0);
        bpf_obj_drop(cc_node_t1);
        bpf_obj_drop(search_cc_node);
        return;
    }

    cc_node_t0->tree_id = 0;
    cc_node_t0->peer_id = get_peerid(hkey.remote_ip);
    cc_node_t0->hkey.rpcid = hkey.rpcid;
    cc_node_t0->hkey.local_port = hkey.local_port;
    cc_node_t0->hkey.remote_port = hkey.remote_port;
    cc_node_t0->hkey.remote_ip = hkey.remote_ip;

    GRANT_LOCK();
    /* last_bytes_remaining is protected by grant lock */
    cc_node_t0->bytes_remaining = rpc_slot->cc.last_bytes_remaining;

    rb_node = bpf_rbtree_lower_bound(&groot, &cc_node_t0->rbtree_link, srpt_less_rpc);
    if (unlikely(!rb_node)) {
        /* this is not an error, the rpc has finished granting */
        GRANT_UNLOCK();
        goto out;
    }

    cc_node_t0 = container_of(rb_node, struct rpc_state_cc, rbtree_link);

    if (cc_node_t0->tree_id != 0 || cc_node_t0->peer_id != get_peerid(hkey.remote_ip) ||
        cc_node_t0->hkey.rpcid != hkey.rpcid || cc_node_t0->hkey.local_port != hkey.local_port ||
        cc_node_t0->hkey.remote_port != hkey.remote_port || cc_node_t0->hkey.remote_ip != hkey.remote_ip)
    {
        GRANT_UNLOCK();
        /* this is not an error, the rpc has finished granting */
        goto out;
    }

    new_br = rpc_slot->cc.bytes_remaining;
    old_br = cc_node_t0->bytes_remaining;

    if (unlikely(old_br == new_br)) {
        GRANT_UNLOCK();
        goto out;
    }

    /* record the value in rbtree for furture finding */
    rpc_slot->cc.last_bytes_remaining = new_br;

    rb_node = bpf_rbtree_remove(&groot, &cc_node_t0->rbtree_link);
    if (unlikely(!rb_node)) { /* this should never happen */
        GRANT_UNLOCK();
        goto out;
    }
    cc_node_t0 = container_of(rb_node, struct rpc_state_cc, rbtree_link);

    cc_node_t0->bytes_remaining = new_br;

    /* we can still use cc_node_t0 until GRANT_UNLOCK() */
    bpf_rbtree_add(&groot, &cc_node_t0->rbtree_link, srpt_less_rpc);

    /* we are already in Tree1 */
    if ((cc_node_t0->birth & 1))
    {
        cc_node_t1->tree_id = 1;
        cc_node_t1->bytes_remaining = old_br;
        cc_node_t1->peer_id = cc_node_t0->peer_id;
        rb_node = bpf_rbtree_lower_bound(&groot, &cc_node_t1->rbtree_link, srpt_less_peer);
        if (likely(rb_node != NULL))
        {
            cc_node_t1 = container_of(rb_node, struct rpc_state_cc, rbtree_link);
            if (likely(cc_node_t1->tree_id == 1 && cc_node_t1->peer_id == cc_node_t0->peer_id))
            {
                rb_node = bpf_rbtree_remove(&groot, &cc_node_t1->rbtree_link);
                if (likely(rb_node != NULL))
                {
                    cc_node_t1 = container_of(rb_node, struct rpc_state_cc, rbtree_link);
                    cc_node_t1->bytes_remaining = new_br;
                    bpf_rbtree_add(&groot, &cc_node_t1->rbtree_link, srpt_less_peer);
                }
                cc_node_t1 = NULL;
            }
            else {
                /* this should never happen */
                cc_node_t1 = NULL;
            }
        }
        else {
            /* this should never happen */
            cc_node_t1 = NULL;
        }
        
        GRANT_UNLOCK();
        goto out;
    }

    /* we get here because we was not the highest priority rpc */
    search_cc_node->tree_id = 0;
    search_cc_node->peer_id = cc_node_t0->peer_id;
    search_cc_node->bytes_remaining = 0;
    search_cc_node->hkey.rpcid = 0;
    search_cc_node->hkey.local_port = 0;
    search_cc_node->hkey.remote_port = 0;
    search_cc_node->hkey.remote_ip = 0;

    rb_node = bpf_rbtree_lower_bound(&groot, &search_cc_node->rbtree_link, srpt_less_rpc);
    if (!rb_node) { /* this should never happen */
        GRANT_UNLOCK();
        bpf_obj_drop(cc_node_t1);
        cc_node_t1 = NULL;
        search_cc_node = NULL;
        log_panic("update_grant_for_cached_rpc: impossible branch, seems a bug.");
        goto out;
    }
    else
    {
        search_cc_node = container_of(rb_node, struct rpc_state_cc, rbtree_link);
        if (unlikely(search_cc_node->tree_id != 0 || search_cc_node->peer_id != cc_node_t0->peer_id))
        {
            /* this should never happen */
            GRANT_UNLOCK();
            bpf_obj_drop(cc_node_t1);
            cc_node_t1 = NULL;
            search_cc_node = NULL;
            log_panic("update_grant_for_cached_rpc: impossible branch, seems a bug.");
            goto out;
        }
        else if (search_cc_node->hkey.rpcid == cc_node_t0->hkey.rpcid &&
                 search_cc_node->hkey.local_port == cc_node_t0->hkey.local_port &&
                 search_cc_node->hkey.remote_port == cc_node_t0->hkey.remote_port &&
                 search_cc_node->hkey.remote_ip == cc_node_t0->hkey.remote_ip)
        {
            /* we find that we are the highest priority one now, but we shoud first check that
             * if there is a rpc from this peer that is in Tree1 already, if so, we should remove it first
             */
            old_cc_node_t0->tree_id = 0;
            old_cc_node_t0->peer_id = cc_node_t0->peer_id;

            old_cc_node_t0->bytes_remaining = cc_node_t0->bytes_remaining;
            old_cc_node_t0->hkey.rpcid = cc_node_t0->hkey.rpcid;
            old_cc_node_t0->hkey.local_port = cc_node_t0->hkey.local_port;
            old_cc_node_t0->hkey.remote_port = cc_node_t0->hkey.remote_port;
            old_cc_node_t0->hkey.remote_ip = cc_node_t0->hkey.remote_ip + 1;

            rb_node = bpf_rbtree_lower_bound(&groot, &old_cc_node_t0->rbtree_link, srpt_less_rpc);
            if (rb_node)
            {
                old_cc_node_t0 = container_of(rb_node, struct rpc_state_cc, rbtree_link);
                if (old_cc_node_t0->tree_id == 0 && old_cc_node_t0->peer_id == cc_node_t0->peer_id)
                {   /* this case means that the old rpc is in Tree1 */
                    /* mark it is not in Tree2 */
                    old_cc_node_t0->birth &= ~(__u64)1;
                    /* use cc_node_t1 to search and remove it from Tree1 */
                    cc_node_t1->tree_id = 1;
                    cc_node_t1->peer_id = old_cc_node_t0->peer_id;
                    cc_node_t1->bytes_remaining = old_cc_node_t0->bytes_remaining;
                    rb_node = bpf_rbtree_lower_bound(&groot, &cc_node_t1->rbtree_link, srpt_less_peer);
                    if (rb_node)
                    {
                        cc_node_t1 = container_of(rb_node, struct rpc_state_cc, rbtree_link);
                        if (cc_node_t1->tree_id == 1 && cc_node_t1->peer_id == old_cc_node_t0->peer_id)
                        {
                            rb_node = bpf_rbtree_remove(&groot, &cc_node_t1->rbtree_link);
                            if (rb_node)
                            {
                                cc_node_t1 = container_of(rb_node, struct rpc_state_cc, rbtree_link);
                                /* update incoming from Tree1 here as it may be modified */
                                old_cc_node_t0->incoming = cc_node_t1->incoming;
                            }
                            else { /* this should never happen */
                                cc_node_t1 = NULL;
                            }
                        }
                        else { /* this should never happen */
                            cc_node_t1 = NULL;
                        }
                    }
                    else { /* this should never happen */
                        cc_node_t1 = NULL;
                    }
                }
            }
            old_cc_node_t0 = NULL;
            /* we get here means that there is no rpc from this peer is in Tree1 now, 
             * and we should insert this rpc to Tree1 as it has the highest priority 
             */
            if (cc_node_t1)
            {   /* cc_node_t1 may come from our bpf_obj_new() or from bpf_rbtree_lower_bound() */
                
                /* copy states to cc_node_t1 from cc_node_t0 except for tree_id */
                {
                    cc_node_t1->tree_id = 1;
                    cc_node_t1->peer_id = cc_node_t0->peer_id;
                    cc_node_t1->bytes_remaining = cc_node_t0->bytes_remaining;
                    cc_node_t1->incoming = cc_node_t0->incoming;
                    cc_node_t1->hkey.rpcid = cc_node_t0->hkey.rpcid;
                    cc_node_t1->hkey.local_port = cc_node_t0->hkey.local_port;
                    cc_node_t1->hkey.remote_ip = cc_node_t0->hkey.remote_ip;
                    cc_node_t1->hkey.remote_port = cc_node_t0->hkey.remote_port;
                    cc_node_t1->message_length = cc_node_t0->message_length;
                    /* mark this rpc is in Tree1 */
                    cc_node_t0->birth |= (__u64)1;
                    cc_node_t1->birth = cc_node_t0->birth;
                }
                
                bpf_rbtree_add(&groot, &cc_node_t1->rbtree_link, srpt_less_peer);
                cc_node_t1 = NULL;
            }
            search_cc_node = NULL;
        }
        else { /* we are not the highest priority one, do nothing */
            search_cc_node = NULL;
        }
    }

    GRANT_UNLOCK();

out:
    if (cc_node_t1)
        bpf_obj_drop(cc_node_t1);
    if (search_cc_node)
        bpf_obj_drop(search_cc_node);
    if (old_cc_node_t0)
        bpf_obj_drop(old_cc_node_t0);
    return;
}

/**
 * @brief cache the rpc for future grant update for performance
 */
static __always_inline
void cache_this_rpc(struct rpc_key_t hkey)
{
    __u32 cpu = bpf_get_smp_processor_id();
    if (unlikely(cpu >= MAX_CPU)) return;

    /* handle the already cached RPC first */
    if (cache_has_rpc[cpu])
    {
        if (cache_rpc[cpu].remote_ip == hkey.remote_ip && cache_rpc[cpu].local_port == hkey.local_port &&
            cache_rpc[cpu].remote_port == hkey.remote_port && cache_rpc[cpu].rpcid == hkey.rpcid)
        {
            /* same rpc */
            update_delay_packets[cpu]++;
            if (update_delay_packets[cpu] < MAX_UPDATE_DELAY_PACKETS) return;
        }
        /* not same rpc or has reached MAX_UPDATE_DELAY_PACKETS, update the grant_list */
        update_grant_for_cached_rpc(cpu);
    }

    /* cache the new RPC */
    cache_rpc[cpu] = hkey;
    cache_has_rpc[cpu] = 1;
    update_delay_packets[cpu] = 1;

    return;
}

struct ret_grant_info {
    __u16 sport;
    __u16 dport;
    __u64 rpcid;
    __u32 newgrant;
    __u32 remote_ip;
    __u8 priority;
};

struct grant_info {
    __u16 sport[HOMA_OVERCOMMITMENT];
    __u16 dport[HOMA_OVERCOMMITMENT];
    __u64 rpcid[HOMA_OVERCOMMITMENT];
    __u32 newgrant[HOMA_OVERCOMMITMENT];
    __u32 remote_ip[HOMA_OVERCOMMITMENT];
    __u8 priority[HOMA_OVERCOMMITMENT];
};

struct remove_info {
    __u64 rpcid[HOMA_OVERCOMMITMENT];
    __u16 local_port[HOMA_OVERCOMMITMENT];
    __u16 remote_port[HOMA_OVERCOMMITMENT];
    __u32 remote_ip[HOMA_OVERCOMMITMENT];
    __u32 newgrant[HOMA_OVERCOMMITMENT];
    __u8 priority[HOMA_OVERCOMMITMENT];
};

#define DECALRE_NODES_1(type, name) type name##_0 = NULL;

#define DECALRE_NODES_2(type, name)                                                                                    \
    type name##_0 = NULL;                                                                                              \
    type name##_1 = NULL;

#define DECALRE_NODES_3(type, name)                                                                                    \
    type name##_0 = NULL;                                                                                              \
    type name##_1 = NULL;                                                                                              \
    type name##_2 = NULL;

#define DECALRE_NODES_4(type, name)                                                                                    \
    type name##_0 = NULL;                                                                                              \
    type name##_1 = NULL;                                                                                              \
    type name##_2 = NULL;                                                                                              \
    type name##_3 = NULL;

#define DECALRE_NODES_5(type, name)                                                                                    \
    type name##_0 = NULL;                                                                                              \
    type name##_1 = NULL;                                                                                              \
    type name##_2 = NULL;                                                                                              \
    type name##_3 = NULL;                                                                                              \
    type name##_4 = NULL;

#define DECALRE_NODES_6(type, name)                                                                                    \
    type name##_0 = NULL;                                                                                              \
    type name##_1 = NULL;                                                                                              \
    type name##_2 = NULL;                                                                                              \
    type name##_3 = NULL;                                                                                              \
    type name##_4 = NULL;                                                                                              \
    type name##_5 = NULL;

#define DECALRE_NODES_7(type, name)                                                                                    \
    type name##_0 = NULL;                                                                                              \
    type name##_1 = NULL;                                                                                              \
    type name##_2 = NULL;                                                                                              \
    type name##_3 = NULL;                                                                                              \
    type name##_4 = NULL;                                                                                              \
    type name##_5 = NULL;                                                                                              \
    type name##_6 = NULL;

#define DECALRE_NODES_8(type, name)                                                                                      \
    type name##_0 = NULL;                                                                                              \
    type name##_1 = NULL;                                                                                              \
    type name##_2 = NULL;                                                                                              \
    type name##_3 = NULL;                                                                                              \
    type name##_4 = NULL;                                                                                              \
    type name##_5 = NULL;                                                                                              \
    type name##_6 = NULL;                                                                                              \
    type name##_7 = NULL;

static bool less_birth_pacer(struct bpf_rb_node *node_a, const struct bpf_rb_node *node_b)
{
    struct rpc_state_cc *a;
    struct rpc_state_cc *b;

    a = container_of(node_a, struct rpc_state_cc, rbtree_link);
    b = container_of(node_b, struct rpc_state_cc, rbtree_link);

    return a->birth < b->birth;
}

static bool less_birth_grant(struct bpf_rb_node *node_a, const struct bpf_rb_node *node_b)
{
    struct rpc_state_cc *a;
    struct rpc_state_cc *b;

    a = container_of(node_a, struct rpc_state_cc, rbtree_link);
    b = container_of(node_b, struct rpc_state_cc, rbtree_link);

    if (b->tree_id == 1)
        return 1;

    if (b->birth & 1)
        return 1;

    // if (b->incoming - (b->message_length - b->bytes_remaining) > Homa_unsched_bytes)
    //   return 1;

    return a->birth < b->birth;
}

/**
 * @brief grant FIFO RPC
 * @return 0: send GRANT packet 1: don't send GRANT packet -1: don't send GRANT packet due to error
 */
static __always_inline
int grant_fifo_rpc(struct ret_grant_info *gi)
{
    struct rpc_state_cc __kptr *cc_node_search = NULL;
    struct bpf_rb_node *rb_node = NULL;
    __u64 increment = 0, newgrant = 0;
    bool need_remove = false;

    cc_node_search = bpf_obj_new(typeof(*cc_node_search));
    if (unlikely(!cc_node_search))
        return -1;

    cc_node_search->birth = POISON_64;

    GRANT_LOCK();

    rb_node = bpf_rbtree_search_less(&groot, &cc_node_search->rbtree_link, less_birth_grant);
    if (unlikely(!rb_node)) {
        GRANT_UNLOCK();
        return 1;
    }
    cc_node_search = container_of(rb_node, struct rpc_state_cc, rbtree_link);

    increment = GRANT_FIFO_INCREMENT;
    newgrant = increment + cc_node_search->incoming;
    cc_node_search->incoming = newgrant;
    if (newgrant >= cc_node_search->message_length) {
        increment -= newgrant - cc_node_search->message_length;
        cc_node_search->incoming = cc_node_search->message_length;
        need_remove = true;
    }

    __sync_fetch_and_add(&total_incoming, increment);

    gi->rpcid = cc_node_search->hkey.rpcid;
    gi->sport = cc_node_search->hkey.local_port;
    gi->dport = cc_node_search->hkey.remote_port;
    gi->remote_ip = cc_node_search->hkey.remote_ip;
    gi->newgrant = cc_node_search->incoming;
    gi->priority = HOMA_MAX_SCHED_PRIO;

    if (need_remove) {
        rb_node = bpf_rbtree_remove(&groot, &cc_node_search->rbtree_link);
        if (rb_node)
            cc_node_search = container_of(rb_node, struct rpc_state_cc, rbtree_link);
        else
            cc_node_search = NULL;
    }
    else
        cc_node_search = NULL;

    GRANT_UNLOCK();
    
    if (cc_node_search)
        bpf_obj_drop(cc_node_search);

    return 0;
}

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, int);
    __type(value, struct grant_info);
} per_cpu_grant_info SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, int);
    __type(value, struct remove_info);
} per_cpu_remove_info SEC(".maps");

/** 
 * @brief grant priority RPC
 * @return 0: send GRANT packet -1: don't send GRANT packet
 */
static __always_inline 
int grant_prio_rpc(struct ret_grant_info *ret_gi, int idx)
{
    struct remove_info *ri;

    ri = bpf_map_lookup_elem(&per_cpu_remove_info, ZERO_KEY);
    if (unlikely(!ri))
    {
        /* this should never happen */
        log_panic("ERROR: per_cpu_grant_info is NULL\n");
        return -1;
    }

    if (unlikely(!ri->newgrant[idx]))
        return -1;

    ret_gi->sport = ri->local_port[idx];
    ret_gi->dport = ri->remote_port[idx];
    ret_gi->rpcid = ri->rpcid[idx];
    ret_gi->newgrant = ri->newgrant[idx];
    ret_gi->remote_ip = ri->remote_ip[idx];
    ret_gi->priority = ri->priority[idx];

    ri->newgrant[idx] = 0;

    return 0;
}

/**
 * @brief check nic queue length and decide whether to send packet, if so, update the link_idle_time
 * @return 0: don't send packet 1: send packet
 */
static __always_inline
int check_nic_queue(__u32 packet_bytes)
{
    __u64 now, idle, new_idle;
    __u64 ns_per_packet = ((packet_bytes + HDR_OVERHEAD) * ns_per_kbyte) / 1000;

    int try_cnt = 2;

    while (try_cnt--)
    {
        now = bpf_ktime_get_ns();
        idle = __sync_fetch_and_add(&link_idle_time, 0);
        if (now + max_nic_queue_ns < idle)
        {
            return 0;
        }
        if (idle < now)
        {
            new_idle = now + ns_per_packet;
        }
        else
        {
            new_idle = idle + ns_per_packet;
        }

        if (__sync_val_compare_and_swap(&link_idle_time, idle, new_idle) == idle)
            break;
    }

    return 1;
}

static __always_inline
int insert_grant_list(struct rpc_state *rpc_slot, struct rpc_key_t *hkey, __u32 message_length)
{
    struct rpc_state_cc *cc_node_t0 = NULL;
    struct rpc_state_cc *old_cc_node_t0 = NULL;
    struct rpc_state_cc *search_cc_node = NULL;
    struct rpc_state_cc *cc_node_t1 = NULL;
    struct bpf_rb_node *rb_node = NULL;
    
    /* allocate new object for Tree0 */
    cc_node_t0 = bpf_obj_new(typeof(*cc_node_t0));
    CHECK_AND_DROP_LOG(!cc_node_t0, "server_request: bpf_obj_new failed.");
    /* allocate new object for Tree1 */
    cc_node_t1 = bpf_obj_new(typeof(*cc_node_t1));
    if (unlikely(!cc_node_t1))
    {
        bpf_obj_drop(cc_node_t0);
        return XDP_DROP;
    }
    /* use bpf_refcount_acquire rather than bpf_obj_new for better performance */
    search_cc_node = bpf_refcount_acquire(cc_node_t1);
    if (unlikely(!search_cc_node))
    {
        bpf_obj_drop(cc_node_t0);
        bpf_obj_drop(cc_node_t1);
        return XDP_DROP;
    }

    old_cc_node_t0 = bpf_refcount_acquire(search_cc_node);
    if (unlikely(!old_cc_node_t0))
    {
        bpf_obj_drop(cc_node_t0);
        bpf_obj_drop(cc_node_t1);
        bpf_obj_drop(search_cc_node);
        return XDP_DROP;
    }

    /* prepare cc_node_t0 */
    {
        cc_node_t0->tree_id = 0;
        cc_node_t0->peer_id = get_peerid(hkey->remote_ip);
        
        /* note: the following two fields may be updated by other CPU, but it's OK */
        cc_node_t0->bytes_remaining = rpc_slot->cc.bytes_remaining;
        cc_node_t0->incoming = rpc_slot->cc.incoming;
        
        cc_node_t0->hkey.rpcid = hkey->rpcid;
        cc_node_t0->hkey.local_port = hkey->local_port;
        cc_node_t0->hkey.remote_ip = hkey->remote_ip;
        cc_node_t0->hkey.remote_port = hkey->remote_port;

        cc_node_t0->message_length = message_length;

        cc_node_t0->birth = bpf_ktime_get_ns();
        cc_node_t0->birth &= ~(__u64)1;
    }

    /* prepare search_cc_node */
    {
        search_cc_node->tree_id = 0;
        search_cc_node->peer_id = get_peerid(hkey->remote_ip);
        search_cc_node->bytes_remaining = 0;
        search_cc_node->hkey.rpcid = 0;
        search_cc_node->hkey.local_port = 0;
        search_cc_node->hkey.remote_port = 0;
        search_cc_node->hkey.remote_ip = 0;
    }

    GRANT_LOCK();
    /* last_bytes_remaining is used for future search, 
        * update here is safe as we can only modify it with grant lock 
        */
    rpc_slot->cc.last_bytes_remaining = cc_node_t0->bytes_remaining;

    /* insert CC object in Tree0
     * note: we can still use cc_node_t0 until GRANT_UNLOCK(), see Linux kernel doc of BPF Graph Data Structures 
     */
    bpf_rbtree_add(&groot, &cc_node_t0->rbtree_link, srpt_less_rpc);

    /* check if we are the shortest one (or the only one) with this peer */
    rb_node = bpf_rbtree_lower_bound(&groot, &search_cc_node->rbtree_link, srpt_less_rpc);
    if (unlikely(!rb_node))
    {   /* this should never happen */
        GRANT_UNLOCK();
        bpf_obj_drop(cc_node_t1);
        cc_node_t1 = NULL;
        search_cc_node = NULL;
        goto out;
    }
    else
    {
        search_cc_node = container_of(rb_node, struct rpc_state_cc, rbtree_link);
        if (unlikely(search_cc_node->tree_id != 0 || search_cc_node->peer_id != cc_node_t0->peer_id))
        {   /* this should never happen */
            GRANT_UNLOCK();
            bpf_obj_drop(cc_node_t1);
            cc_node_t1 = NULL;
            search_cc_node = NULL;
            goto out;
        }
        else if (search_cc_node->hkey.rpcid == cc_node_t0->hkey.rpcid &&
                    search_cc_node->hkey.local_port == cc_node_t0->hkey.local_port &&
                    search_cc_node->hkey.remote_port == cc_node_t0->hkey.remote_port &&
                    search_cc_node->hkey.remote_ip == cc_node_t0->hkey.remote_ip)
        {
            /* we find that we are the highest priority one now, but we shoud first check that
             * if there is a rpc from this peer that is in Tree1 already, if so, we should remove it first
             */
            old_cc_node_t0->tree_id = 0;
            old_cc_node_t0->peer_id = cc_node_t0->peer_id;

            old_cc_node_t0->bytes_remaining = cc_node_t0->bytes_remaining;
            old_cc_node_t0->hkey.rpcid = cc_node_t0->hkey.rpcid;
            old_cc_node_t0->hkey.local_port = cc_node_t0->hkey.local_port;
            old_cc_node_t0->hkey.remote_port = cc_node_t0->hkey.remote_port;
            old_cc_node_t0->hkey.remote_ip = cc_node_t0->hkey.remote_ip + 1;

            rb_node = bpf_rbtree_lower_bound(&groot, &old_cc_node_t0->rbtree_link, srpt_less_rpc);
            if (rb_node)
            {
                old_cc_node_t0 = container_of(rb_node, struct rpc_state_cc, rbtree_link);
                if (old_cc_node_t0->tree_id == 0 && old_cc_node_t0->peer_id == cc_node_t0->peer_id)
                {   /* this case means that the old rpc is in Tree1 */
                    /* mark it is not in Tree2 */
                    old_cc_node_t0->birth &= ~(__u64)1;
                    /* use cc_node_t1 to search and remove it from Tree1 */
                    cc_node_t1->tree_id = 1;
                    cc_node_t1->peer_id = old_cc_node_t0->peer_id;
                    cc_node_t1->bytes_remaining = old_cc_node_t0->bytes_remaining;
                    rb_node = bpf_rbtree_lower_bound(&groot, &cc_node_t1->rbtree_link, srpt_less_peer);
                    if (rb_node)
                    {
                        cc_node_t1 = container_of(rb_node, struct rpc_state_cc, rbtree_link);
                        if (cc_node_t1->tree_id == 1 && cc_node_t1->peer_id == old_cc_node_t0->peer_id)
                        {
                            rb_node = bpf_rbtree_remove(&groot, &cc_node_t1->rbtree_link);
                            if (rb_node)
                            {
                                cc_node_t1 = container_of(rb_node, struct rpc_state_cc, rbtree_link);
                                /* update incoming from Tree1 here as it may be modified */
                                old_cc_node_t0->incoming = cc_node_t1->incoming;
                            }
                            else { /* this should never happen */
                                cc_node_t1 = NULL;
                            }
                        }
                        else { /* this should never happen */
                            cc_node_t1 = NULL;
                        }
                    }
                    else { /* this should never happen */
                        cc_node_t1 = NULL;
                    }
                }
            }
            old_cc_node_t0 = NULL;
            /* we get here means that there is no rpc from this peer is in Tree1 now, 
             * and we should insert this rpc to Tree1 as it has the highest priority 
             */
            if (cc_node_t1)
            {   /* cc_node_t1 may come from our bpf_obj_new() or from bpf_rbtree_lower_bound() */
                
                /* copy states to cc_node_t1 from cc_node_t0 except for tree_id */
                {
                    cc_node_t1->tree_id = 1;
                    cc_node_t1->peer_id = cc_node_t0->peer_id;
                    cc_node_t1->bytes_remaining = cc_node_t0->bytes_remaining;
                    cc_node_t1->incoming = cc_node_t0->incoming;
                    cc_node_t1->hkey.rpcid = cc_node_t0->hkey.rpcid;
                    cc_node_t1->hkey.local_port = cc_node_t0->hkey.local_port;
                    cc_node_t1->hkey.remote_ip = cc_node_t0->hkey.remote_ip;
                    cc_node_t1->hkey.remote_port = cc_node_t0->hkey.remote_port;
                    cc_node_t1->message_length = cc_node_t0->message_length;
                    /* mark this rpc is in Tree1 */
                    cc_node_t0->birth |= (__u64)1;
                    cc_node_t1->birth = cc_node_t0->birth;
                }

                bpf_rbtree_add(&groot, &cc_node_t1->rbtree_link, srpt_less_peer);
                cc_node_t1 = NULL;
            }
            search_cc_node = NULL;
        }
        else { /* we are not the highest priority one, do nothing */
            search_cc_node = NULL;
        }
    }
    GRANT_UNLOCK();
out:
    if (cc_node_t1)
        bpf_obj_drop(cc_node_t1);
    if (search_cc_node)
        bpf_obj_drop(search_cc_node);
    if (old_cc_node_t0)
        bpf_obj_drop(old_cc_node_t0);
    
    return XDP_REDIRECT;
}

static __always_inline
int client_request(struct iphdr *iph, struct data_header *d, __u64 buffer_addr,
                                          __u64 *rpc_qid, bool *trigger)
{
    struct rpc_state *rpc_slot = NULL;
    struct rpc_key_t hkey = {0};
    struct rpc_state_cc *cc_node = NULL;
    struct rpc_state_cc *ref_cc_node = NULL;

    hkey.local_port = bpf_ntohs(d->common.sport);
    hkey.remote_port = bpf_ntohs(d->common.dport);
    hkey.rpcid = bpf_be64_to_cpu(d->common.sender_id);
    bool first_packet = bpf_ntohs(d->common.seq) == 0 ? 1 : 0;
    __u32 message_length = bpf_ntohl(d->message_length);
    __u32 offset = bpf_ntohl(d->seg.offset);
    __u32 packet_bytes = bpf_ntohl(d->seg.segment_length);
    bool single_packet = message_length <= HOMA_MSS;

    hkey.remote_ip = bpf_ntohl(iph->daddr);
    
    if (first_packet) {
        /* create a new RPC state */
        struct rpc_state new_rpc_state = {0};
        new_rpc_state.state = BPF_RPC_OUTGOING;
        new_rpc_state.message_length = message_length;
        new_rpc_state.next_xmit_offset = packet_bytes;
        new_rpc_state.buffer_head = buffer_addr;
        new_rpc_state.remote_port = hkey.remote_port;
        new_rpc_state.local_port = hkey.local_port;
        new_rpc_state.remote_ip = hkey.remote_ip;
        new_rpc_state.id = hkey.rpcid;
        new_rpc_state.cc.granted = min(message_length, Homa_unsched_bytes);
        new_rpc_state.qid = MAX_BUCKET_SIZE;

        CHECK_AND_DROP_LOG(bpf_map_update_elem(&rpc_tbl, &hkey, &new_rpc_state, BPF_NOEXIST), "client_request, bpf_map_update_elem failed.");

        /* optimization for single-packet case */
        if (likely(single_packet)) {
            set_prio(iph, HOMA_MAX_PRIORITY - 1);
            d->incoming = bpf_htonl(message_length);
            return XDP_TX;
        }
    }

    /* lookup again */
    rpc_slot = bpf_map_lookup_elem(&rpc_tbl, &hkey);
    CHECK_AND_DROP_LOG(!rpc_slot, "client_request, bpf_map_lookup_elem failed.");

    if (offset + packet_bytes < Homa_unsched_bytes)
        set_prio(iph, get_prio(message_length));
    else
        set_prio(iph, atomic_read(&rpc_slot->cc.sched_prio));

    /* check if we have enough credit */
    __u64 cc_granted = atomic_read(&rpc_slot->cc.granted);
    d->incoming = bpf_htonl(cc_granted);

    if (offset + packet_bytes <= cc_granted) {
        /* we have enough credit, further check three conditions to determine if we can send packet directly 
         * 1) no packets in rate limiter
         * 2) packet size is small enough or NIC queue is not busy
         */
        if (atomic_read(&rpc_slot->nr_pkts_in_rl) == 0 &&
            (packet_bytes <= Homa_min_throttled_bytes || check_nic_queue(packet_bytes)))
        {
            /* this update is safe for no packets in rate limiter */
            rpc_slot->next_xmit_offset = offset + packet_bytes;
            return XDP_TX;
        }
    }

    /* Unfortunately, we should enqueue this packet to rate limiter */
    atomic_inc(&rpc_slot->nr_pkts_in_rl);

    if (unlikely(rpc_slot->qid == MAX_BUCKET_SIZE))
    {
        /* this RPC has not been enqueued before */
        rpc_slot->qid = allocate_qid();
        CHECK_AND_DROP_LOG(rpc_slot->qid == MAX_BUCKET_SIZE, "client_request, allocate_qid failed.");

        /* we create the qid, so we need to create an object and enqueue it to throttle list */
        cc_node = bpf_obj_new(typeof(*cc_node));
        CHECK_AND_DROP_LOG(!cc_node, "client_request, bpf_obj_new failed.");
        
        cc_node->birth = bpf_ktime_get_ns();
        cc_node->hkey.rpcid = hkey.rpcid;
        cc_node->hkey.local_port = hkey.local_port;
        cc_node->hkey.remote_ip = hkey.remote_ip;
        cc_node->hkey.remote_port = hkey.remote_port;
        cc_node->bytes_remaining = rpc_slot->message_length - rpc_slot->next_xmit_offset;
        ref_cc_node = bpf_refcount_acquire(cc_node);
        CHECK_AND_DROP_LOG(!ref_cc_node, "client_request, bpf_refcount_acquire failed.");

        THROTTLE_LOCK();
        
        /* insert ref pointer to throttle list */
        bpf_rbtree_add(&troot, &ref_cc_node->rbtree_link, srpt_less_pacer);
        atomic_inc(&nr_rpc_in_throttle);
        
        THROTTLE_UNLOCK();

        /* store pointer in map for future update */
        PUT_POINTER(cc_node, rpc_slot);
    }
    
    *rpc_qid = rpc_slot->qid;
    *trigger = cc_granted >= (offset + packet_bytes);

    return XDP_REDIRECT;
}

static __always_inline
int server_response(struct iphdr *iph, struct data_header *d, __u64 buffer_addr,
                                           __u64 *rpc_qid, bool *trigger)
{
    struct rpc_state *rpc_slot = NULL;
    struct rpc_key_t hkey = {0};
    struct rpc_state_cc *cc_node = NULL;
    struct rpc_state_cc *ref_cc_node = NULL;

    hkey.local_port = bpf_ntohs(d->common.sport);
    hkey.remote_port = bpf_ntohs(d->common.dport);
    hkey.rpcid = bpf_be64_to_cpu(d->common.sender_id);
    bool first_packet = bpf_ntohs(d->common.seq) == 0 ? 1 : 0;
    __u32 message_length = bpf_ntohl(d->message_length);
    __u32 offset = bpf_ntohl(d->seg.offset);
    __u32 packet_bytes = bpf_ntohl(d->seg.segment_length);
    bool single_packet = bpf_ntohl(d->message_length) <= HOMA_MSS;

    hkey.remote_ip = bpf_ntohl(iph->daddr);

    rpc_slot = bpf_map_lookup_elem(&rpc_tbl, &hkey);
    CHECK_AND_DROP_LOG(!rpc_slot, "server_response, bpf_map_lookup_elem failed.");
    
    if (first_packet) {
        /* first received response packet */
        rpc_slot->state = BPF_RPC_OUTGOING;
        rpc_slot->message_length = message_length;
        rpc_slot->next_xmit_offset = packet_bytes;
        rpc_slot->buffer_head = buffer_addr;
        rpc_slot->nr_pkts_in_rl = 0;
        rpc_slot->cc.sched_prio = 0;
        rpc_slot->cc.granted = min(message_length, Homa_unsched_bytes);
        rpc_slot->qid = MAX_BUCKET_SIZE;

        /* optimization for single-packet case */
        if (likely(single_packet)) {
            set_prio(iph, HOMA_MAX_PRIORITY - 1);
            d->incoming = bpf_htonl(message_length);
            return XDP_TX;
        }
    }

    if (offset + packet_bytes < Homa_unsched_bytes)
        set_prio(iph, get_prio(message_length));
    else
        set_prio(iph, atomic_read(&rpc_slot->cc.sched_prio));

    /* check if we have enough credit */
    __u64 cc_granted = atomic_read(&rpc_slot->cc.granted);
    d->incoming = bpf_htonl(cc_granted);

    if (offset + packet_bytes <= cc_granted) {
        /* we have enough credit, further check three conditions to determine if we can send packet directly 
         * 1) no packets in rate limiter
         * 2) packet size is small enough or NIC queue is not busy
         */
        if (atomic_read(&rpc_slot->nr_pkts_in_rl) == 0 &&
            (packet_bytes <= Homa_min_throttled_bytes || check_nic_queue(packet_bytes)))
        {
            /* this update is safe for no packets in rate limiter */
            rpc_slot->next_xmit_offset = offset + packet_bytes;
            return XDP_TX;
        }
    }

    /* Unfortunately, we should enqueue this packet to rate limiter */
    atomic_inc(&rpc_slot->nr_pkts_in_rl);

    if (unlikely(rpc_slot->qid == MAX_BUCKET_SIZE))
    {
        /* this RPC has not been enqueued before */
        rpc_slot->qid = allocate_qid();
        CHECK_AND_DROP_LOG(rpc_slot->qid == MAX_BUCKET_SIZE, "server_response, allocate_qid failed.");

        /* we create the qid, so we need to create an object and enqueue it to throttle list */
        cc_node = bpf_obj_new(typeof(*cc_node));
        CHECK_AND_DROP_LOG(!cc_node, "server_response, bpf_obj_new failed.");


        cc_node->birth = bpf_ktime_get_ns();
        cc_node->hkey.rpcid = hkey.rpcid;
        cc_node->hkey.local_port = hkey.local_port;
        cc_node->hkey.remote_ip = hkey.remote_ip;
        cc_node->hkey.remote_port = hkey.remote_port;
        cc_node->bytes_remaining = rpc_slot->message_length - rpc_slot->next_xmit_offset;
        ref_cc_node = bpf_refcount_acquire(cc_node);
        CHECK_AND_DROP_LOG(!ref_cc_node, "server_response, bpf_refcount_acquire failed.");

        THROTTLE_LOCK();
        
        /* insert ref pointer to throttle list */
        bpf_rbtree_add(&troot, &ref_cc_node->rbtree_link, srpt_less_pacer);
        atomic_inc(&nr_rpc_in_throttle);
        
        THROTTLE_UNLOCK();

        /* store pointer in map for future update */
        PUT_POINTER(cc_node, rpc_slot);
    }
    *rpc_qid = rpc_slot->qid;
    *trigger = cc_granted >= (offset + packet_bytes);

    return XDP_REDIRECT;
}

static __always_inline
int client_response(struct data_header *d, __u32 remote_ip, struct homa_meta_info *data_meta,
                                           int single_packet)
{
    struct rpc_state *rpc_slot = NULL;
    struct rpc_key_t hkey = {0};
    bool new_state = false;
    int complete = 0;

    hkey.rpcid = local_id(bpf_be64_to_cpu(d->common.sender_id));
    hkey.local_port = bpf_ntohs(d->common.dport);
    hkey.remote_port = bpf_ntohs(d->common.sport);
    __u16 seq = bpf_ntohs(d->common.seq);
    __u32 message_length = bpf_ntohl(d->message_length);
    __u32 incoming = bpf_ntohl(d->incoming);
    __u64 seg_length = bpf_ntohl(d->seg.segment_length);
    
    hkey.remote_ip = remote_ip;
    
    rpc_slot = bpf_map_lookup_elem(&rpc_tbl, &hkey);
    CHECK_AND_DROP_LOG(!rpc_slot, "client_response, bpf_map_lookup_elem failed.");
    
    RPC_LOCK(rpc_slot);

    if (unlikely(rpc_slot->state == BPF_RPC_DEAD)) {
        RPC_UNLOCK(rpc_slot);
        return XDP_DROP;
    }

    new_state = rpc_slot->state == BPF_RPC_OUTGOING;
    
    if (new_state) { /* first response packet */
        if (likely(single_packet)) 
        {
            /* ensure that only we can delete it */
            rpc_slot->state = BPF_RPC_DEAD;
            RPC_UNLOCK(rpc_slot);
            
            /* userspace will use this metadata to free buffers */
            data_meta->rx.reap_client_buffer_addr = rpc_slot->buffer_head;

            /* if we allocate qid for this rpc, we need to free it */
            if (rpc_slot->qid != MAX_BUCKET_SIZE)
                free_qid(rpc_slot->qid);

            bpf_map_delete_elem(&rpc_tbl, &hkey);
            
            enqueue_dead_crpc(hkey.remote_ip, hkey.remote_port, hkey.local_port, hkey.rpcid);

            return XDP_REDIRECT;
        }
        
        rpc_slot->state = BPF_RPC_INCOMING;
        rpc_slot->bit_width = DIV_ROUND_UP(message_length, HOMA_MSS);
        
        clear_all_bitmaps(rpc_slot);

        set_bitmap(rpc_slot, seq);

        rpc_slot->message_length = message_length;
        rpc_slot->cc.incoming = incoming;
        
        rpc_slot->cc.bytes_remaining = message_length - seg_length;

        RPC_UNLOCK(rpc_slot);

        __sync_fetch_and_add(&total_incoming, (__u64)(incoming - seg_length));

        /* free rpc_state_cc object if it exists (used for pacing before) */
        struct rpc_state_cc *cc_node = NULL;
        GET_POINTER(cc_node, rpc_slot);
        if (cc_node)
            bpf_obj_drop(cc_node);
    }
    else {   /* not the first response packet */

        complete = set_bitmap(rpc_slot, seq);
        if (complete == -1)
        {
            RPC_UNLOCK(rpc_slot);
            bpf_printk("set_bitmap failed, rpcid = %llu, seq = %u", hkey.rpcid, seq);
            return XDP_DROP;
        }
        if (incoming > rpc_slot->cc.incoming)
            rpc_slot->cc.incoming = incoming;
        rpc_slot->cc.bytes_remaining -= seg_length;

        if (complete == 1)
        {   /* all response packets have been received */
            rpc_slot->state = BPF_RPC_DEAD;
            RPC_UNLOCK(rpc_slot);
            /* userspace will use this metadata to free buffers */
            data_meta->rx.reap_client_buffer_addr = rpc_slot->buffer_head;

            /* if we allocate qid for this rpc, we need to free it */
            if (rpc_slot->qid != MAX_BUCKET_SIZE)
                free_qid(rpc_slot->qid);

            enqueue_dead_crpc(hkey.remote_ip, hkey.remote_port, hkey.local_port, hkey.rpcid);

            /* note: after we delete the rpc state, our CC object may still be in the grant_list, 
             * but it would be finally removed, don't worry about it 
             */
            bpf_map_delete_elem(&rpc_tbl, &hkey);

            return XDP_REDIRECT;
        }
        
        RPC_UNLOCK(rpc_slot);
        
        __sync_fetch_and_sub(&total_incoming, (__u64)seg_length);
    }

    bool need_schedule = message_length > rpc_slot->cc.incoming;

    if (need_schedule)
        cache_this_rpc(hkey);
    
    if (!new_state || !need_schedule)
        return XDP_REDIRECT;

    return insert_grant_list(rpc_slot, &hkey, message_length);
}

static __always_inline
int server_request(struct data_header *d, __u32 remote_ip, int single_packet)
{
    struct rpc_state *rpc_slot = NULL;
    struct rpc_state new_rpc_slot_lock = {0};
    struct rpc_key_t hkey = {0};
    bool new_state = false;
    int complete = 0;

    bool need_schedule = false;

    hkey.rpcid = local_id(bpf_be64_to_cpu(d->common.sender_id));
    hkey.local_port = bpf_ntohs(d->common.dport);
    hkey.remote_port = bpf_ntohs(d->common.sport);
    __u16 seq = bpf_ntohs(d->common.seq);
    __u32 message_length = bpf_ntohl(d->message_length);
    __u32 incoming = bpf_ntohl(d->incoming);
    __u64 seg_length = bpf_ntohl(d->seg.segment_length);

    hkey.remote_ip = remote_ip;

    rpc_slot = bpf_map_lookup_elem(&rpc_tbl, &hkey);
    if (!rpc_slot) {
        
        CHECK_AND_DROP_LOG(d->retransmit, "server_request: retransmitted packet tries to create state.");
        
        rpc_slot = &new_rpc_slot_lock;
        rpc_slot->remote_ip = remote_ip;
        rpc_slot->remote_port = hkey.remote_port;
        rpc_slot->local_port = hkey.local_port;
        rpc_slot->id = hkey.rpcid;
        rpc_slot->state = single_packet ? BPF_RPC_IN_SERVICE : BPF_RPC_INCOMING;
        rpc_slot->buffer_head = POISON_64;

        rpc_slot->bit_width = DIV_ROUND_UP(message_length, HOMA_MSS);
        clear_all_bitmaps(rpc_slot);
        set_bitmap(rpc_slot, seq);

        rpc_slot->message_length = message_length;
        rpc_slot->cc.incoming = incoming;
        rpc_slot->cc.bytes_remaining = message_length - seg_length;

        need_schedule = message_length > rpc_slot->cc.incoming;
        
        long ret = bpf_map_update_elem(&rpc_tbl, &hkey, rpc_slot, BPF_NOEXIST);

        if (ret == -EEXIST) {
            /* we want to create the state but it already exists, lookup again and update */
            rpc_slot = bpf_map_lookup_elem(&rpc_tbl, &hkey);
            CHECK_AND_DROP_LOG(!rpc_slot, "server_request: bpf_map_lookup_elem failed.");
            goto update;
        }
        else if (ret != 0) {
            log_panic("RPC stable is overflow!!!");
            return XDP_DROP;
        }
        else {
            /* we create the rpc_slot successfully */
            new_state = true;
            __sync_fetch_and_add(&total_incoming, (__u64)(incoming - seg_length));
            /* lookup again */
            rpc_slot = bpf_map_lookup_elem(&rpc_tbl, &hkey);
            CHECK_AND_DROP_LOG(!rpc_slot, "server_request: bpf_map_lookup_elem failed.");
        }
    }
    else {
        /* RPC state exists */
    update:
        RPC_LOCK(rpc_slot);
        
        if (unlikely(rpc_slot->state == BPF_RPC_DEAD))
        {
            RPC_UNLOCK(rpc_slot);
            return XDP_DROP;
        }
        complete = set_bitmap(rpc_slot, seq);
        if (complete == 1) {
            rpc_slot->state = BPF_RPC_IN_SERVICE;
        }
        else if (complete == -1)
        {
            RPC_UNLOCK(rpc_slot);
            bpf_printk("set_bitmap failed, rpcid = %llu, seq = %u", hkey.rpcid, seq);
            return XDP_DROP;
        }
        
        if (incoming > rpc_slot->cc.incoming)
            rpc_slot->cc.incoming = incoming;
        
        rpc_slot->cc.bytes_remaining -= seg_length;
        
        need_schedule = message_length > rpc_slot->cc.incoming;
        
        RPC_UNLOCK(rpc_slot);
        
        __sync_fetch_and_sub(&total_incoming, (__u64)seg_length);
    }

    if (likely(single_packet) || complete == 1)
        return XDP_REDIRECT;

    if (need_schedule)
        cache_this_rpc(hkey);

    if (!new_state || !need_schedule)
        return XDP_REDIRECT;

    return insert_grant_list(rpc_slot, &hkey, message_length);
}

static __always_inline
void reclaim_rpc(struct data_header *d, __u32 remote_ip, struct homa_meta_info *data_meta)
{
    struct rpc_state *delete_rpc_slot = NULL;
    __u64 del_rpcid = bpf_be64_to_cpu(d->seg.ack.rpcid);
    __u16 del_local_port = bpf_ntohs(d->seg.ack.dport);
    __u16 del_remote_port = bpf_ntohs(d->seg.ack.sport);
    
    if (del_rpcid == 0 && del_local_port == 0 && del_remote_port == 0)
        return;

    del_rpcid = local_id(del_rpcid);

    struct rpc_key_t delete_hkey = {0};
    delete_hkey.rpcid = del_rpcid;
    delete_hkey.local_port = del_local_port;
    delete_hkey.remote_port = del_remote_port;
    delete_hkey.remote_ip = remote_ip;

    delete_rpc_slot = bpf_map_lookup_elem(&rpc_tbl, &delete_hkey);
    if (!delete_rpc_slot)
        return;

    RPC_LOCK(delete_rpc_slot);
    if (delete_rpc_slot->state == BPF_RPC_DEAD)
    {
        RPC_UNLOCK(delete_rpc_slot);
        return;
    }
    /* ensure that only we can delete it */
    delete_rpc_slot->state = BPF_RPC_DEAD;
    RPC_UNLOCK(delete_rpc_slot);
    
    data_meta->rx.reap_server_buffer_addr = delete_rpc_slot->buffer_head;
    
    if (delete_rpc_slot->qid != MAX_BUCKET_SIZE)
        free_qid(delete_rpc_slot->qid);

    /* free rpc_state_cc object if it exists */
    struct rpc_state_cc *cc_node = NULL;
    GET_POINTER(cc_node, delete_rpc_slot);
    if (cc_node)
        bpf_obj_drop(cc_node);

    bpf_map_delete_elem(&rpc_tbl, &delete_hkey);
}

static __always_inline
int xmit_ctrl_pkt(struct xdp_md *ctx, enum homa_packet_type type)
{
    __u16 port_swap;
    __u32 ip_swap;
    __u8 mac_swap[ETH_ALEN];

    int length = (int)sizeof(struct common_header) - (int)sizeof(struct resend_header); // < 0

    if (bpf_xdp_adjust_tail(ctx, length))
    {
        log_err("bpf_xdp_adjust_tail failed.");
        return XDP_DROP;
    }
    // reverifiy
    void *data = (void *)(long)ctx->data;
    void *data_end = (void *)(long)ctx->data_end;
    struct ethhdr *eth = (struct ethhdr *)data;
    if (eth + 1 > data_end)
        return XDP_DROP;
    struct iphdr *iph = (struct iphdr *)(eth + 1);
    if (iph + 1 > data_end)
        return XDP_DROP;
    struct common_header *homa_common_h = (struct common_header *)(iph + 1);
    if (homa_common_h + 1 > data_end)
        return XDP_DROP;

    homa_common_h->type = type;

    homa_common_h->sender_id = bpf_cpu_to_be64(local_id(bpf_be64_to_cpu(homa_common_h->sender_id)));

    port_swap = homa_common_h->sport;
    homa_common_h->sport = homa_common_h->dport;
    homa_common_h->dport = port_swap;

    ip_swap = iph->saddr;
    iph->saddr = iph->daddr;
    iph->daddr = ip_swap;

    __builtin_memcpy(mac_swap, eth->h_source, ETH_ALEN);
    __builtin_memcpy(eth->h_source, eth->h_dest, ETH_ALEN);
    __builtin_memcpy(eth->h_dest, mac_swap, ETH_ALEN);

    return XDP_TX;
}

static __always_inline
int resend_pkt(struct resend_header *homa_resend_h, struct homa_meta_info *data_meta, __u32 remote_ip)
{
    struct rpc_key_t hkey = {0};
    struct rpc_state *rpc_slot = NULL;
    int ret = RESEND;
    bool need_kick = false;

    __u64 rpcid = bpf_be64_to_cpu(homa_resend_h->common.sender_id);
    rpcid = local_id(rpcid);

    hkey.rpcid = rpcid;
    hkey.local_port = bpf_ntohs(homa_resend_h->common.dport);
    hkey.remote_port = bpf_ntohs(homa_resend_h->common.sport);
    hkey.remote_ip = remote_ip;

    rpc_slot = bpf_map_lookup_elem(&rpc_tbl, &hkey);
    if (unlikely(!rpc_slot)) {
        return UNKNOWN;
    }

    RPC_LOCK(rpc_slot);

    if (rpc_slot->state == BPF_RPC_DEAD) {
        RPC_UNLOCK(rpc_slot);
        return UNKNOWN;
    }

    if (!rpc_is_client(rpcid) && rpc_slot->state != BPF_RPC_OUTGOING) {
        /* We are the server for this RPC. If we haven't received
         * all of the bytes we've granted then request a resend
         * of the missing bytes; otherwise just send a BUSY.
         */
        if (
            rpc_slot->message_length - rpc_slot->cc.bytes_remaining > (rpc_slot->cc.incoming/HOMA_MSS) * HOMA_MSS || 
            ((rpc_slot->message_length - rpc_slot->cc.bytes_remaining == (rpc_slot->cc.incoming/HOMA_MSS) * HOMA_MSS) && 
            rpc_slot->cc.incoming != rpc_slot->message_length)
            )
        {
            // case#1: we have received all the bytes we've granted
            ret = BUSY;
        }
        else
        {
            // case#2: it seems that we lost some packets, which causes **client** to
            // send RESEND packets. Leave server timeout to handle this.
            ret = 0;
        }
        goto out;
    }

    // if (rpc_slot->next_xmit_offset < rpc_slot->cc.granted) {
    if (bpf_ntohl(homa_resend_h->offset) >= rpc_slot->next_xmit_offset) {
        /* We have chosen not to transmit data from this message;
         * send BUSY instead.
         */
        ret = BUSY;
        if (rpc_slot->next_xmit_offset < rpc_slot->message_length && 
            rpc_slot->next_xmit_offset + 1420 <= rpc_slot->cc.granted)
            need_kick = true;
    }
    else
    {
        if (homa_resend_h->length == 0)
        {
            /* This RESEND is from a server just trying to determine
             * whether the client still cares about the RPC; return
             * BUSY so the server doesn't time us out.
             */
            ret = BUSY;
        }
    }
out:
    if (ret == RESEND) {
        data_meta->rx.reap_server_buffer_addr = rpc_slot->buffer_head;
        rpc_slot->resend_count++;
    }

    RPC_UNLOCK(rpc_slot);

    if (need_kick) {
        kick_pacer();
    }

    return ret;
}

static __always_inline
int unknown_pkt(struct xdp_md *ctx, struct unknown_header *homa_unknown_h,
                                       struct homa_meta_info *data_meta, void *data_end, __u32 remote_ip)
{
    struct rpc_key_t hkey = {0};
    struct rpc_state *rpc_slot = NULL;
    __u32 next_xmit_offset = 0;

    __u64 rpcid = bpf_be64_to_cpu(homa_unknown_h->common.sender_id);
    rpcid = local_id(rpcid);

    __u16 remote_port = bpf_ntohs(homa_unknown_h->common.sport);
    __u16 local_port = bpf_ntohs(homa_unknown_h->common.dport);

    hkey.rpcid = rpcid;
    hkey.local_port = local_port;
    hkey.remote_port = remote_port;
    hkey.remote_ip = remote_ip;

    rpc_slot = bpf_map_lookup_elem(&rpc_tbl, &hkey);
    if (unlikely(!rpc_slot)) {
        return -1;
    }
    RPC_LOCK(rpc_slot);
    if (unlikely(rpc_slot->state == BPF_RPC_DEAD))
    {
        RPC_UNLOCK(rpc_slot);
        return XDP_DROP;
    }

    if (rpc_is_client(rpcid))
    {
        if (rpc_slot->state == BPF_RPC_OUTGOING)
        {
            /* It appears that everything we've already transmitted
             * has been lost; retransmit it.
             */
            data_meta->rx.reap_server_buffer_addr = rpc_slot->buffer_head;
            next_xmit_offset = rpc_slot->next_xmit_offset;
            RPC_UNLOCK(rpc_slot);
            if (bpf_xdp_adjust_tail(ctx, sizeof(struct resend_header) - sizeof(struct unknown_header)))
            {
                // log_panic("bpf_xdp_adjust_tail failed.");
                return -1;
            }
            // reverifiy
            void *data = (void *)(long)ctx->data;
            void *data_end = (void *)(long)ctx->data_end;
            struct ethhdr *eth = (struct ethhdr *)data;
            if (eth + 1 > data_end) {
                return -1;
            }
            struct iphdr *iph = (struct iphdr *)(eth + 1);
            if (iph + 1 > data_end) {
                return -1;
            }
            struct resend_header *homa_resend_h = (struct resend_header *)(iph + 1);
            if (homa_resend_h + 1 > data_end) {
                return -1;
            }
            homa_resend_h->common.type = RESEND;
            homa_resend_h->offset = 0;
            homa_resend_h->length = bpf_htonl(next_xmit_offset);
            return 0;
        }
    }
    else
    {
        rpc_slot->state = BPF_RPC_DEAD;
        data_meta->rx.reap_client_buffer_addr = rpc_slot->buffer_head;
        rpc_slot->buffer_head = __UINT64_MAX__;

        RPC_UNLOCK(rpc_slot);

        homa_unknown_h->common.type = RESEND;

        bpf_map_delete_elem(&rpc_tbl, &hkey);
        
        return 0;
    }
    RPC_UNLOCK(rpc_slot);
    return -1;
}

static __always_inline
void busy_pkt(struct busy_header *homa_busy_h, __u32 remote_ip)
{
    struct rpc_state *rpc_slot = NULL;
    struct rpc_key_t hkey = {0};

    __u64 rpcid = bpf_be64_to_cpu(homa_busy_h->common.sender_id);
    rpcid = local_id(rpcid);

    hkey.rpcid = rpcid;
    hkey.local_port = bpf_ntohs(homa_busy_h->common.dport);
    hkey.remote_port = bpf_ntohs(homa_busy_h->common.sport);
    hkey.remote_ip = remote_ip;

    rpc_slot = bpf_map_lookup_elem(&rpc_tbl, &hkey);
    if (unlikely(!rpc_slot)) {
        log_err("receive unknown busy pkt");
        return;
    }

    if (rpc_slot->state == BPF_RPC_DEAD) {
        return;
    }

    rpc_slot->busy_count++;
}

static __always_inline void grant_pkt(struct grant_header *homa_grant_h, __u32 remote_ip)
{
    struct rpc_key_t hkey = {0};
    struct rpc_state *rpc_slot = NULL;
    struct rpc_state_cc *cc_node = NULL;
    struct rpc_state_cc *ref_cc_node = NULL;

    __u32 offset = bpf_ntohl(homa_grant_h->offset);
    __u64 rpcid = bpf_be64_to_cpu(homa_grant_h->common.sender_id);
    rpcid = local_id(rpcid);

    hkey.rpcid = rpcid;
    hkey.local_port = bpf_ntohs(homa_grant_h->common.dport);
    hkey.remote_port = bpf_ntohs(homa_grant_h->common.sport);
    hkey.remote_ip = remote_ip;

    rpc_slot = bpf_map_lookup_elem(&rpc_tbl, &hkey);
    if (unlikely(!rpc_slot)) {
        log_err("receive unknown grant pkt");
        return;
    }
    if (rpc_slot->state != BPF_RPC_OUTGOING)
        return;

    atomic_xchg(&rpc_slot->cc.sched_prio, homa_grant_h->priority);

    if (rpc_slot->cc.granted < offset)
    {
        // Since we don't enforce load balancing for grant packets, only one CPU will
        // handle this grant packet. So we don't need any lock
        atomic_xchg(&rpc_slot->cc.granted, offset);

        // bpf_printk("RPC#%llu, grant = %u", rpcid, offset);
        
        if (atomic_read(&rpc_slot->qid) != MAX_BUCKET_SIZE)
        {
            GET_POINTER(cc_node, rpc_slot);
            if (unlikely(!cc_node)) {
                // werid case
                kick_pacer();
                return;
            }
            ref_cc_node = bpf_refcount_acquire(cc_node);
            if (unlikely(!ref_cc_node)) {
                // this should never happen
                bpf_printk("We are receiving GRANT, but we can't get cc_node reference for RPC#%llu", rpcid);
                PUT_POINTER(cc_node, rpc_slot);
                kick_pacer();
                return;
            }
            THROTTLE_LOCK();
            if (bpf_rbtree_add(&troot, &ref_cc_node->rbtree_link, srpt_less_pacer) == 0)
                atomic_inc(&nr_rpc_in_throttle);
            THROTTLE_UNLOCK();
            PUT_POINTER(cc_node, rpc_slot);
        }
    }
    kick_pacer();
    // bpf_printk("kick pacer for RPC#%llu", rpcid);
}