#include <runtime/homa.h>

#include <mutex>
#include <unordered_map>
#include <set>

#include <linux/if_ether.h>
#include <linux/ip.h>
#include <net/ethernet.h>

#include <base/ipc.h>
#include <runtime/app_if.h>
#include <runtime/ebpf_if.h>

#include "trans_ebpf.h"
#include "nic.h"

std::unordered_map<uint16_t, struct homa_socket *> homa_sockets;

std::set<__u32> comp_xsk_infos;

#define MAP_BATCH_SIZE 8

extern class eTranNIC *etran_nic;

// control_plane.cc
extern int alloc_port(uint16_t port);
extern int free_port(uint16_t port);
extern int record_port(struct app_ctx *actx, uint16_t local_port, uint16_t remote_port);
extern int unrecord_port(struct app_ctx *actx, uint16_t port);

// ebpf.cc
extern class eTranHoma *etran_homa;

__u32 global_tick = 0;

const char *rpc_state_str[] = {
    "BPF_RPC_DEAD",
    "",
    "",
    "",
    "",
    "BPF_RPC_OUTGOING",
    "BPF_RPC_INCOMING",
    "",
    "BPF_RPC_IN_SERVICE",
};

struct hkey
{
    uint64_t rpcid;
    uint16_t local_port;
    uint16_t remote_port;
    uint32_t remote_ip;
};

struct hkey_hash
{
    size_t operator()(const struct hkey &k) const
    {
        return std::hash<uint64_t>()(k.rpcid) ^ std::hash<uint16_t>()(k.local_port) ^ std::hash<uint16_t>()(k.remote_port) ^ std::hash<uint32_t>()(k.remote_ip);
    }
};

struct hkey_equal
{
    bool operator()(const struct hkey &a, const struct hkey &b) const
    {
        return a.rpcid == b.rpcid && a.local_port == b.local_port && a.remote_port == b.remote_port && a.remote_ip == b.remote_ip;
    }
};

struct rpc_state_wrapper
{
    struct rpc_state data;
    __u32 last_scan_tick;
    __u32 done_timer_ticks;
    __u32 silent_ticks;
    __u32 resend_try;
};

std::unordered_map<struct hkey, struct rpc_state_wrapper, hkey_hash, hkey_equal> shadow_tbl;

struct hkey lookup_keys[MAP_BATCH_SIZE];
struct rpc_state lookup_values[MAP_BATCH_SIZE];
struct hkey delete_keys[MAX_RPC_TBL_SIZE];
int delete_count = 0;

static inline int rpc_is_client(__u64 id)
{
    return (id & 1) == 0;
}

/**
 * @brief Find the first segment of zeros in the bitmap
 * @param bitmap The bitmap to search
 * @param bit_width The width of the bitmap
 * @return A pair of integers representing the start and end of the segment of zeros
 */
std::pair<int, int> find_first_zero_segment(__u64 bitmap[12], int bit_width)
{
    int start = -1;
    int end = -1;

    int total_bits = 0;

    for (int i = 0; i < 12; ++i)
    {
        for (int j = 0; j < 64; ++j)
        {
            // If we have reached the bit_width, return the result
            if (total_bits == bit_width)
            {
                if (start == -1)
                {
                    // If no segment of zeros is found, return {-1, -1}
                    return {-1, -1};
                }
                // If the last segment of zeros goes until the end, return it
                return {start, end};
            }

            // Check if the j-th bit is 0
            if ((bitmap[i] & (__u64(1) << j)) == 0)
            {
                if (start == -1)
                {
                    // If start has not been set, set it
                    start = total_bits;
                }
                // Update end
                end = total_bits;
            }
            else
            {
                if (start != -1)
                {
                    // If start has been set and we encounter a 1, return the segment
                    return {start, end};
                }
            }

            ++total_bits;
        }
    }

    // If no segment of zeros is found, return {-1, -1}
    if (start == -1)
    {
        return {-1, -1};
    }

    // If the last segment of zeros goes until the end, return it
    return {start, end};
}

/**
 * @brief Get the first range that is missing in the bitmap
 * @param rpc_state The rpc_state of the RPC
 * @param resend The resend_header to craft
 * @return true if there is a range to resend, false otherwise
 */
bool get_resend_range(struct rpc_state *rpc_state, struct resend_header *resend)
{
    if (rpc_state->state == BPF_RPC_OUTGOING) {
        /* Haven't received any data for this message; request
         * retransmission of just the first packet (the sender
         * will send at least one full packet, regardless of
         * the length below).
         */
        resend->offset = 0;
        resend->length = 100;
        resend->priority = HOMA_MAX_PRIORITIES - 1;

        return true;
    }

    std::pair<int, int> hole = find_first_zero_segment(rpc_state->bitmap, rpc_state->bit_width);

    if (hole.first == -1 || hole.second == -1)
    {
        printf("Werid case, no hole found in the bitmap, but RPC#%llu has been timed out, %llu.\n", rpc_state->id,
               rpc_state->bitmap[0]);
        return false;
    }

    resend->offset = hole.first * HOMA_MSS;
    resend->length = (hole.second - hole.first + 1) * HOMA_MSS;
    resend->priority = HOMA_MAX_PRIORITIES - 1;

    return true;
}

/**
 * @brief Transmit a NEED_ACK packet through slowpath AF_XDP socket
 * @param rpc_state The rpc_state of the RPC
 * @return 0 on success, -1 on failure
 */
int xmit_need_ack_pkt(struct rpc_state *rpc_state)
{
    auto it = homa_sockets.find(rpc_state->local_port);
    assert(it != homa_sockets.end());
    struct app_ctx *actx = it->second->tctx->actx;
    struct thread_bcache *bc = &actx->iobuffer;
    unsigned int idx_tx = 0;

    /* Choose a random NIC queue to send the packet */
    __u32 tx_qid = actx->nic_qid[std::rand() % actx->nr_nic_queues];
    struct xsk_socket_info *xsk_info = etran_nic->_nic_queues[tx_qid].xsk_info;

    if (unlikely(thread_bcache_check(bc, 1) != 1)) {
        fprintf(stderr, "No buffer available for sending NEED_ACK packet\n");
        return -1;
    }

    if (unlikely(xsk_ring_prod__reserve(&xsk_info->tx, 1, &idx_tx) < 1)) {
        fprintf(stderr, "Cannot reserve space for sending NEED_ACK packet\n");
        return -1;
    }

    uint64_t buffer_addr = thread_bcache_cons(bc);
    buffer_addr = add_offset_tx_frame(buffer_addr);
    char *pkt = (char *)xsk_umem__get_data(xsk_info->umem_area, buffer_addr);

    /* Fill IP header */
    struct iphdr *iph = reinterpret_cast<struct iphdr *>(pkt + sizeof(struct ethhdr));

    iph->saddr = htonl(etran_nic->_local_ip);
    iph->daddr = htonl(rpc_state->remote_ip);
    iph->protocol = IPPROTO_HOMA;
    iph->version = IPVERSION;
    iph->ihl = 0x5;
    iph->tos = (HOMA_MAX_PRIORITIES - 1) << 5;
    iph->id = 0;
    iph->frag_off = (1 << 14);
    iph->ttl = IPDEFTTL;

    iph->tot_len = htons(sizeof(struct iphdr) + sizeof(struct need_ack_header));
    iph->check = 0;

    /* Fill NEED_ACK header */
    struct need_ack_header *r =
        reinterpret_cast<struct need_ack_header *>(pkt + sizeof(struct ethhdr) + sizeof(struct iphdr));

    r->common.type = NEED_ACK;
    r->common.sender_id = __cpu_to_be64(rpc_state->id);
    r->common.sport = htons(rpc_state->local_port);
    r->common.dport = htons(rpc_state->remote_port);

    struct xdp_desc *tx_desc = xsk_ring_prod__tx_desc(&xsk_info->tx, idx_tx);
    tx_desc->addr = buffer_addr;
    tx_desc->len = sizeof(struct ethhdr) + sizeof(struct iphdr) + sizeof(struct need_ack_header);
    tx_desc->options = 0;

    xsk_info->outstanding++;
    comp_xsk_infos.insert(tx_qid);
    
    xsk_ring_prod__submit(&xsk_info->tx, 1);
    kick_tx(xsk_info);
    
    return 0;
}

/**
 * @brief Transmit a RESEND packet through slowpath AF_XDP socket
 * @param rpc_state The rpc_state of the RPC
 * @param resend The resend_header to craft
 * @return 0 on success, -1 on failure
 */
int xmit_resend_pkt(struct rpc_state *rpc_state, struct resend_header *resend)
{
    auto it = homa_sockets.find(rpc_state->local_port);
    assert(it != homa_sockets.end());
    struct app_ctx *actx = it->second->tctx->actx;
    struct thread_bcache *bc = &actx->iobuffer;
    unsigned int idx_tx = 0;

    /* Choose a random NIC queue to send the packet */
    __u32 tx_qid = actx->nic_qid[std::rand() % actx->nr_nic_queues];
    struct xsk_socket_info *xsk_info = etran_nic->_nic_queues[tx_qid].xsk_info;

    if (unlikely(thread_bcache_check(bc, 1) != 1)) {
        fprintf(stderr, "No buffer available for sending RESEND packet\n");
        return -1;
    }

    if (unlikely(xsk_ring_prod__reserve(&xsk_info->tx, 1, &idx_tx) < 1)) {
        fprintf(stderr, "Cannot reserve space for sending RESEND packet\n");
        return -1;
    }

    uint64_t buffer_addr = thread_bcache_cons(bc);
    buffer_addr = add_offset_tx_frame(buffer_addr);
    char *pkt = (char *)xsk_umem__get_data(xsk_info->umem_area, buffer_addr);

    homa_txmeta_clear_all(xsk_info->umem_area, buffer_addr);
    /* mark this packet is from control path */
    homa_txmeta_set_from_slowpath(xsk_info->umem_area, buffer_addr, 1);

    /* Fill IP header */
    struct iphdr *iph = reinterpret_cast<struct iphdr *>(pkt + sizeof(struct ethhdr));

    iph->saddr = htonl(etran_nic->_local_ip);
    iph->daddr = htonl(rpc_state->remote_ip);
    iph->protocol = IPPROTO_HOMA;
    iph->version = IPVERSION;
    iph->ihl = 0x5;
    iph->tos = (HOMA_MAX_PRIORITIES - 1) << 5;
    iph->id = 0;
    iph->frag_off = (1 << 14);
    iph->ttl = IPDEFTTL;

    iph->tot_len = htons(sizeof(struct iphdr) + sizeof(struct resend_header));
    iph->check = 0;

    /* Fill RESEND header */
    struct resend_header *r =
        reinterpret_cast<struct resend_header *>(pkt + sizeof(struct ethhdr) + sizeof(struct iphdr));

    r->common.type = RESEND;
    r->common.sender_id = __cpu_to_be64(rpc_state->id);
    r->common.sport = htons(rpc_state->local_port);
    r->common.dport = htons(rpc_state->remote_port);

    r->offset = htonl((__u32)resend->offset);
    r->length = htonl((__u32)resend->length);
    r->priority = resend->priority;

    struct xdp_desc *tx_desc = xsk_ring_prod__tx_desc(&xsk_info->tx, idx_tx);
    tx_desc->addr = buffer_addr;
    tx_desc->len = sizeof(struct ethhdr) + sizeof(struct iphdr) + sizeof(struct resend_header);
    tx_desc->options = 0;

    xsk_info->outstanding++;
    comp_xsk_infos.insert(tx_qid);
    
    xsk_ring_prod__submit(&xsk_info->tx, 1);
    kick_tx(xsk_info);
    
    return 0;
}

static void dump_retransmission(struct rpc_state *rpc_state, struct resend_header *resend, __u32 *resend_try, __u32 *silent_ticks)
{
    if (rpc_state->state == BPF_RPC_INCOMING)
    {
        printf("(%u)Issue resend[%u,%u): %s, #%llu, length:%u,"
                " incoming:%llu,"
                " bytes_remaining:%llu, tick:%u\n", (*resend_try),
                resend->offset, resend->offset + resend->length, rpc_state_str[rpc_state->state], rpc_state->id,
                rpc_state->message_length, rpc_state->cc.incoming, rpc_state->cc.bytes_remaining,
                *silent_ticks);
    }
    else
    {
        printf("(%u)Issue resend[%u,%u): %s, #%llu, length:%u,"
                " granted:%llu,"
                " next_xmit_offset:%llu, tick:%u\n", (*resend_try),
                resend->offset, resend->offset + resend->length, rpc_state_str[rpc_state->state], rpc_state->id,
                rpc_state->message_length, rpc_state->cc.granted, rpc_state->next_xmit_offset,
                *silent_ticks);
    }
}

static void check_timeout(struct hkey *hkey, struct rpc_state *rpc_state)
{
    struct rpc_state *shadow_rpc_state = &shadow_tbl[*hkey].data;
    __u32 *done_timer_ticks = &shadow_tbl[*hkey].done_timer_ticks;
    __u32 *silent_ticks = &shadow_tbl[*hkey].silent_ticks;
    __u32 *resend_try = &shadow_tbl[*hkey].resend_try;
    struct resend_header resend = {0};

    /* This is server RPC, and is waiting for client to acknowledge the response.
     * Send NEED_ACK packet to client if exceeds NEED_ACK_TICK. 
    */
    if (!rpc_is_client(rpc_state->id) && rpc_state->state == BPF_RPC_OUTGOING &&
        rpc_state->next_xmit_offset >= rpc_state->message_length)
    {
        if (*done_timer_ticks == 0) {
            *done_timer_ticks = global_tick;
            (*resend_try) = 0;
        }
        else
        {
            /* >= comparison that handles tick wrap-around. */
            if ((*done_timer_ticks + NEED_ACK_TICK - 1 - global_tick) & 1 << 31)
            {
                /* send NEED_ACK packet for this RPC */
                xmit_need_ack_pkt(rpc_state);
                (*resend_try)++;
            }
        }
        return;
    }

    /* RPC is transmitting data */
    if (rpc_state->state == BPF_RPC_OUTGOING && rpc_state->next_xmit_offset < rpc_state->message_length)
    {
        if (rpc_state->next_xmit_offset +
                std::min(rpc_state->message_length - rpc_state->next_xmit_offset, (__u64)HOMA_MSS) <=
            rpc_state->cc.granted)
        {
            /* there are granted bytes that we haven't transmitted, reset timeout tick and clear resend_try */
            *silent_ticks = 0;
            (*resend_try) = 0;
            // printf("There are enough granted bytes but we haven't transmitted: state=%s, rpcid = %llu, length = "
            //        "%u, rpc_state->next_xmit_offset=%llu, "
            //        "rpc_state->cc.granted=%llu\n",
            //        rpc_state_str[rpc_state->state], rpc_state->id, rpc_state->message_length,
            //        rpc_state->next_xmit_offset, rpc_state->cc.granted);
            return;
        }
    }

    /* RPC is receving data */
    if (rpc_state->state == BPF_RPC_INCOMING)
    {
        if (
            rpc_state->message_length - rpc_state->cc.bytes_remaining > (rpc_state->cc.incoming/HOMA_MSS) * HOMA_MSS ||
            ((rpc_state->message_length - rpc_state->cc.bytes_remaining == (rpc_state->cc.incoming/HOMA_MSS) * HOMA_MSS) && 
                rpc_state->cc.incoming != rpc_state->message_length)
            )
        {
            /* we have received all we have granted, this rpc is waiting for grant, reset timeout tick and clear resend_try */
            *silent_ticks = 0;
            (*resend_try) = 0;
            // printf("We have received all we have granted for RPC#%llu\n", rpc_state->id);
            return;
        }
    }
    else if (!rpc_is_client(rpc_state->id))
    {
        // This is server RPC, and we are not receiving data (in service or transmitting data)
        // reset timeout tick and clear resend_try
        *silent_ticks = 0;
        (*resend_try) = 0;
        return;
    }

    /* If we detect the following events, reset timeout tick and clear resend_try:
     * 1. receive DATA/GRANT/RESEND packets
     * 2. send GRANT packets
    */
    if (rpc_state->cc.bytes_remaining != shadow_rpc_state->cc.bytes_remaining ||
        rpc_state->cc.granted != shadow_rpc_state->cc.granted ||
        rpc_state->cc.incoming != shadow_rpc_state->cc.incoming ||
        rpc_state->resend_count != shadow_rpc_state->resend_count)
    {
        // printf("shadow_rpc_state->cc.bytes_remaining = %llu, rpc_state->cc.bytes_remaining = %llu\n",
        // shadow_rpc_state->cc.bytes_remaining, rpc_state->cc.bytes_remaining); printf("shadow_rpc_state->cc.granted =
        // %llu, rpc_state->cc.granted = %llu\n", shadow_rpc_state->cc.granted, rpc_state->cc.granted);
        // printf("shadow_rpc_state->cc.incoming = %llu, rpc_state->cc.incoming = %llu\n",
        // shadow_rpc_state->cc.incoming, rpc_state->cc.incoming);
        *silent_ticks = 0;
        (*resend_try) = 0;
        shadow_rpc_state->cc.bytes_remaining = rpc_state->cc.bytes_remaining;
        shadow_rpc_state->cc.granted = rpc_state->cc.granted;
        shadow_rpc_state->cc.incoming = rpc_state->cc.incoming;
        shadow_rpc_state->resend_count = rpc_state->resend_count;
        return;
    }

    if (*silent_ticks < (RESEND_TICK - 1))
        return;

    (*resend_try)++;

    /* issue retransmission for this RPC. */
    if (get_resend_range(rpc_state, &resend))
    {
        dump_retransmission(rpc_state, &resend, resend_try, silent_ticks);

        if (xmit_resend_pkt(rpc_state, &resend) == 0)
        {
            /* reset timeout tick */
            *silent_ticks = 0;
        }
    }
}

/**
 * @brief This function called every tick to detect the timeout of RPCs.
 * If needed, it will send RESEND packets to the remote peer.
 */
int poll_homa_to(void)
{
    struct hkey hkey;
    struct rpc_state rpc_state;
    int nr_active = 0;
    int err = 0;
    
    __u32 count = MAP_BATCH_SIZE;
    bool first = true;
    __u32 out_batch = 0;
    DECLARE_LIBBPF_OPTS(bpf_map_batch_opts, opts, .elem_flags = BPF_F_LOCK,
                        .flags = 0, );

    global_tick++;

    while (1)
    {
        err = bpf_map_lookup_batch(etran_homa->_homa_rpc_fd, first ? nullptr : &out_batch, &out_batch, lookup_keys, lookup_values, &count,
                                   &opts);
        first = false;
        if (err == -EFAULT || count == 0)
        {
            break;
        }
        for (__u32 i = 0; i < count; i++)
        {
            hkey = lookup_keys[i];
            rpc_state = lookup_values[i];

            if (homa_sockets.find(rpc_state.local_port) == homa_sockets.end() || rpc_state.state == BPF_RPC_DEAD) {
                if (homa_sockets.find(rpc_state.local_port) == homa_sockets.end())
                    printf("Homa timer:\tAbort RPC#%llu because no homa socket found (%llu)\n", rpc_state.id, rpc_state.buffer_head);
                delete_keys[delete_count++] = hkey;
                continue;
            }
            
            nr_active++;
            
            if (shadow_tbl.find(hkey) == shadow_tbl.end())
            {
                /* case#1: first time meeting this RPC, insert it to shadow table */
                shadow_tbl.insert({hkey, {rpc_state, global_tick, 0, 0, 0}});
            }
            else
            {
                /* case#2: not the first time meeting this RPC */
                shadow_tbl[hkey].last_scan_tick = global_tick;
                if (rpc_state.busy_count != shadow_tbl[hkey].data.busy_count)
                {
                    /* eBPF received BUSY packet and modify busy_count, reset timeout tick */
                    shadow_tbl[hkey].silent_ticks = 0;
                    shadow_tbl[hkey].data.busy_count = rpc_state.busy_count;
                    printf("Homa timer: Remote peer is busy, reset silent_ticks for RPC#%llu\n", rpc_state.id);
                }
                else {
                    shadow_tbl[hkey].silent_ticks++;
                }

                if (shadow_tbl[hkey].resend_try == ABORT_RESEND) {
                    /* abort this RPC */
                    delete_keys[delete_count++] = hkey;
                    shadow_tbl.erase(hkey);
                    printf("Homa timer:\tAbort RPC#%llu because of no response (%llu)\n", rpc_state.id, rpc_state.buffer_head);
                    continue;
                }
                
                check_timeout(&hkey, &rpc_state);

            }

        }
    }

    /* delete zombie RPCs in eBPF */
    while (delete_count)
    {
        count = delete_count;
        err = bpf_map_delete_batch(etran_homa->_homa_rpc_fd, delete_keys, &count, &opts);
        if (err != -EFAULT) {
            /* count is invalid only when err == -EFAULT */
            delete_count -= count;
        }
        
        if (count == 0 && err == -ENOENT) {
            /* all entries are already deleted */
            delete_count = 0;
        }
        
        if (count == 0)
            break;
    }

    /* delete entries that are not in eBPF */
    for (auto it = shadow_tbl.begin(); it != shadow_tbl.end();)
    {
        if (it->second.last_scan_tick != global_tick)
            it = shadow_tbl.erase(it);
        else
            ++it;
    }

    /* traverse all NIC queues that we have sent packets before */
    for (auto it = comp_xsk_infos.begin(); it != comp_xsk_infos.end();)
    {
        __u32 qid = *it;
        struct xsk_socket_info *xsk_info = etran_nic->_nic_queues[qid].xsk_info;
        struct app_ctx *actx = etran_nic->_nic_queues[qid].actx;
        struct xsk_ring_cons *cq = &actx->bpw.bp->cq[qid];
        spinlock_t *cq_lock = &actx->bpw.bp->cq_lock[qid];

        if (unlikely(!xsk_info || xsk_info->outstanding == 0))
        {
            it = comp_xsk_infos.erase(it);
            continue;
        }

        spin_lock(cq_lock);

        unsigned int idx_cq = 0;
        unsigned int rcvd = xsk_ring_cons__peek(cq, xsk_info->outstanding, &idx_cq);
        for (unsigned int i = 0; i < rcvd; i++)
        {
            uint64_t addr = *xsk_ring_cons__comp_addr(cq, idx_cq + i);
            thread_bcache_prod(&actx->iobuffer, addr);
        }
        if (rcvd)
        {
            xsk_ring_cons__release(cq, rcvd);
            xsk_info->outstanding -= rcvd;
        }
        spin_unlock(cq_lock);
        
        ++it;
    }

    if ((global_tick % 1000 == 0) && nr_active)
        printf("Homa timer:\t%d RPCs\n", nr_active);

    return 0;
}

static int reg_homa_socket_ebpf(struct app_ctx_per_thread *tctx, uint16_t port)
{
    uint16_t key = port;
    struct target_xsk v;
    memset(&v, -1, sizeof(v));

    for (unsigned int i = 0; i < tctx->actx->nr_nic_queues; i++)
    {
        v.xsk_map_idx[tctx->actx->nic_qid[i]] = tctx->txrx_xsk_map_key[i];
    }

    if (bpf_map_update_elem(etran_homa->_homa_port_tbl_fd, &key, &v, BPF_ANY))
    {
        return -1;
    }

    return 0;
}

void unreg_homa_socket_ebpf(uint16_t port)
{
    uint16_t key = port;
    bpf_map_delete_elem(etran_homa->_homa_port_tbl_fd, &key);
}

void notify_app_homa_status_bind(struct app_ctx_per_thread *tctx, opaque_ptr opaque_socket, int fd, int32_t status)
{
    lrpc_msg msg = {0};
    struct appin_homa_status_t *kmsg = (struct appin_homa_status_t *)msg.data;

    msg.cmd = APPIN_HOMA_STATUS_BIND;
    kmsg->opaque_socket = opaque_socket;
    kmsg->fd = fd;
    kmsg->status = status;

    if (lrpc_send(&tctx->kernel_out, &msg))
    {
        fprintf(stderr, "notify_app_tcp_status_bind: failed to send message\n");
    }

    kick_evfd(tctx->evfd);
}

void notify_app_homa_status_close(struct app_ctx_per_thread *tctx, opaque_ptr opaque_socket, int fd, int32_t status)
{
    lrpc_msg msg = {0};
    struct appin_homa_status_t *kmsg = (struct appin_homa_status_t *)msg.data;

    msg.cmd = APPIN_HOMA_STATUS_CLOSE;
    kmsg->opaque_socket = opaque_socket;
    kmsg->fd = fd;
    kmsg->status = status;

    if (lrpc_send(&tctx->kernel_out, &msg))
    {
        fprintf(stderr, "notify_app_tcp_status_bind: failed to send message\n");
    }

    kick_evfd(tctx->evfd);
}

// internal functions
static inline struct homa_socket *find_homa_socket_slowpath(opaque_ptr opaque_socket)
{
    for (auto it = homa_sockets.begin(); it != homa_sockets.end(); it++)
    {
        if (it->second->opaque_socket == opaque_socket)
        {
            return it->second;
        }
    }
    return nullptr;
}

static inline void reg_homa_socket_slowpath(struct homa_socket *s)
{
    homa_sockets.insert(std::make_pair(s->local_port, s));
}

static inline void unreg_homa_socket_slowpath(struct homa_socket *s)
{
    for (auto it = homa_sockets.begin(); it != homa_sockets.end(); it++)
    {
        if (it->second == s)
        {
            homa_sockets.erase(it);
            break;
        }
    }
}

int homa_bind(struct app_ctx_per_thread *tctx, struct appout_homa_bind_t *homa_bind_msg_in)
{
    struct homa_socket *hs;
    opaque_ptr opaque_socket = homa_bind_msg_in->opaque_socket;
    int fd = homa_bind_msg_in->fd;
    uint32_t _local_ip = homa_bind_msg_in->local_ip;
    uint16_t local_port = homa_bind_msg_in->local_port;

    // FIXME
    (void)_local_ip;

    hs = find_homa_socket_slowpath(opaque_socket);

    if (hs)
        return -EADDRINUSE;

    hs = new homa_socket();
    if (!hs)
        return -ENOMEM;

    if (alloc_port(local_port))
    {
        delete hs;
        return -EADDRINUSE;
    }

    hs->tctx = tctx;
    hs->opaque_socket = opaque_socket;
    hs->fd = fd;
    hs->local_ip = etran_nic->_local_ip;
    hs->local_port = local_port;

    record_port(tctx->actx, hs->local_port, 0);

    if (reg_homa_socket_ebpf(tctx, hs->local_port))
    {
        unrecord_port(tctx->actx, hs->local_port);
        free_port(hs->local_port);
        delete hs;
        return -1;
    }

    reg_homa_socket_slowpath(hs);

    notify_app_homa_status_bind(tctx, hs->opaque_socket, hs->fd, 0);

    return 0;
}

int homa_close(struct app_ctx_per_thread *tctx, opaque_ptr opaque_socket)
{
    struct homa_socket *s;

    s = find_homa_socket_slowpath(opaque_socket);

    if (!s)
        return -ENOENT;

    unreg_homa_socket_slowpath(s);

    unreg_homa_socket_ebpf(s->local_port);

    unrecord_port(tctx->actx, s->local_port);

    free_port(s->local_port);

    notify_app_homa_status_close(tctx, opaque_socket, s->fd, 0);

    delete s;

    return 0;
}

void process_homa_cmd(struct app_ctx_per_thread *tctx, lrpc_msg *msg_in)
{
    struct appout_homa_bind_t *homa_bind_msg_in;
    struct appout_homa_close_t *homa_close_msg_in;
    switch (msg_in->cmd)
    {
    case APPOUT_HOMA_BIND:
        homa_bind_msg_in = (struct appout_homa_bind_t *)msg_in->data;
        if (homa_bind(tctx, homa_bind_msg_in))
        {
            notify_app_homa_status_bind(tctx, homa_bind_msg_in->opaque_socket, homa_bind_msg_in->fd, -1);
        }
        break;
    case APPOUT_HOMA_CLOSE:
        homa_close_msg_in = (struct appout_homa_close_t *)msg_in->data;
        if (homa_close(tctx, homa_close_msg_in->opaque_socket))
        {
            notify_app_homa_status_close(tctx, homa_close_msg_in->opaque_socket, homa_close_msg_in->fd, -2);
        }
        break;
    default:
        printf("Unknown command %ld\n", msg_in->cmd);
    }
}

static void free_homa_sockets(struct app_ctx *actx)
{
    struct homa_socket *s;
    auto it = homa_sockets.begin();

    while (it != homa_sockets.end())
    {
        s = it->second;
        if (s->tctx->actx == actx)
        {
            it = homa_sockets.erase(it);
            unreg_homa_socket_ebpf(s->local_port);
            unrecord_port(actx, s->local_port);
            free_port(s->local_port);
            delete s;
        }
        else
            it++;
    }
}

void free_homa_resources(struct app_ctx *actx)
{
    free_homa_sockets(actx);
}