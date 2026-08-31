// SPDX-License-Identifier: GPL-2.0

#include <time.h>

#include <linux/bpf.h>
#include <linux/types.h>
#include <bpf/bpf_helpers.h>
#include <xdp/parsing_helpers.h>

#include <intf/intf_ebpf.h>
#include "ebpf_fib.h"
#include "ebpf_kfunc.h"
#include "ebpf_lb.h"
#include "ebpf_queue.h"
#include "ebpf_utils.h"

#include "eTran_defs.h"
#include "bss_data_defs.h"
#include "rpc.h"
#include "pacing.h"
#include "homa.h"

char LICENSE[] SEC("license") = "GPL";

// #define HELP_PACER

#define XDP_GEN_RETURN_DROP(last_grant) (last_grant ? XDP_ABORTED : XDP_DROP)

SEC("xdp_gen")
int xdp_gen_prog(struct xdp_md *ctx)
{
    int err = 0;
    int last_grant = 0; // if this is true, we should return XDP_ABORTED
    int no_work = 0;
    __u32 cpu = bpf_get_smp_processor_id();
    if (unlikely(cpu >= MAX_CPU))
    {
        bpf_printk("ERROR: CPU Mapping, cpu=%d\n", cpu);
        return XDP_ABORTED;
    }
    struct ret_grant_info gi = {0};
    void *data;
    void *data_end;
    int send_fifo_rpc = 0;

    unsigned int gi_idx;
    use_cached_lb_choice[cpu] = 0;

    if (finish_grant_choose[cpu] == 0)
    {
        // force to clear the last cached rpc
        update_grant_for_cached_rpc(cpu);

        bpf_tail_call(ctx, &xdp_gen_tail_call_map, XDP_GEN_CHOOSE_RPC_TO_GRANT);
        // fallthrough: bpf_tail_call failed
        return XDP_ABORTED;
    }

    granting_idx[cpu]++;
    // bpf_printk("DEBUG: granting_idx[cpu]: %d\n", granting_idx[cpu]);

    if (nr_grant_ready[cpu] == 0 && !need_grant_fifo[cpu])
    {
        last_grant = 1;
        no_work = 1;
        goto reset;
    }

    // bpf_printk("need_grant_fifo[cpu]: %d", need_grant_fifo[cpu]);
    if (need_grant_fifo[cpu] == 1)
    {
        // If we have RPC in the FIFO queue, we should grant it at last
        if (granting_idx[cpu] == min(nr_grant_candidate[cpu], HOMA_OVERCOMMITMENT) + 1)
        {
            last_grant = 1;
        }
    }
    else if (granting_idx[cpu] == min(nr_grant_candidate[cpu], HOMA_OVERCOMMITMENT))
    {
        // after processing the packet, we should return XDP_ABORTED to terminate
        last_grant = 1;
    }

    if (need_grant_fifo[cpu] == 1 && last_grant == 1)
    {
        // it's time to grant the RPC in the FIFO queue
        err = grant_fifo_rpc(&gi);
        if (err)
        {
            no_work = 1;
            need_grant_fifo[cpu] = 0; // error or no fifo rpc to grant
            goto reset;
        }
        send_fifo_rpc = 1;
    }
    else
    {
        // grant the RPC in the Priority queue
        gi_idx = (granting_idx[cpu] - 1);
        gi_idx = gi_idx % HOMA_OVERCOMMITMENT;

        err = grant_prio_rpc(&gi, gi_idx);
        if (err)
        {
            no_work = 1;
            goto reset;
        }
    }

    if (unlikely(err = bpf_xdp_adjust_tail(ctx, -HOMA_GRANT_HEADER_CUTOFF)))
    {
        bpf_printk("ERROR: bpf_xdp_adjust_tail failed: %d\n", err);
        return XDP_GEN_RETURN_DROP(last_grant);
    }

    data = (void *)(long)ctx->data;
    data_end = (void *)(long)ctx->data_end;

    // Ethernet header
    struct ethhdr *eth = (struct ethhdr *)data;
    if (unlikely(eth + 1 > data_end))
    {
        return XDP_GEN_RETURN_DROP(last_grant);
    }

    // IP header
    struct iphdr *iph = (struct iphdr *)(eth + 1);
    if (unlikely(iph + 1 > data_end))
    {
        return XDP_GEN_RETURN_DROP(last_grant);
    }

    iph->saddr = bpf_htonl(local_ip);
    iph->daddr = bpf_htonl(gi.remote_ip);
    iph->version = IPVERSION;
    iph->protocol = IPPROTO_HOMA;
    iph->ihl = 0x5;
    iph->tos = gi.priority << 5;
    iph->tot_len = bpf_htons(sizeof(struct iphdr) + sizeof(struct grant_header));
    iph->id = 0;
    iph->ttl = IPDEFTTL;
    iph->check = 0;

    // grant header
    struct grant_header *gh = (struct grant_header *)(iph + 1);
    if (unlikely(gh + 1 > data_end))
    {
        return XDP_GEN_RETURN_DROP(last_grant);
    }

    gh->common.type = GRANT;
    gh->common.dport = bpf_htons(gi.dport);
    gh->common.sport = bpf_htons(gi.sport);
    gh->common.sender_id = bpf_cpu_to_be64(gi.rpcid);
    gh->offset = bpf_htonl(gi.newgrant);
    gh->priority = gi.priority;
    gh->resend_all = 0;

    //   bpf_printk("Grant to offset(%u)", gi.newgrant);

reset:
    if (last_grant)
    {
        granting_idx[cpu] = 0;
        nr_grant_candidate[cpu] = 0;
        finish_grant_choose[cpu] = 0;
#ifdef HELP_PACER
        help_pacer();
#endif
        // bpf_printk("reset granting_idx");
    }

    if (no_work)
    {
        // bpf_printk("no_work: last_grant: %d\n", last_grant);
        return XDP_GEN_RETURN_DROP(last_grant);
    }

    err = fib_lookup(ctx, eth, iph);
    if (unlikely(err))
    {
        bpf_printk("ERROR: bpf_fib_lookup failed in XDP_GEN, check routing table in kernel,"
                   "last_grant = %d, need_grant_fifo[cpu] = %d\n",
                   last_grant, need_grant_fifo[cpu]);
        return XDP_GEN_RETURN_DROP(last_grant);
    }

    // flush the FIFO queue
    if (send_fifo_rpc)
        need_grant_fifo[cpu] = 0;

    return XDP_TX;
}

// Fill IP header except for addresses
static __always_inline void fill_ip_hdr(struct iphdr *iph, __u32 len)
{
    /* fill ip header */
    iph->ihl = 5;
    iph->version = 4;
    iph->tot_len = bpf_htons(len - sizeof(struct ethhdr));
    iph->id = bpf_htons(0);
    iph->frag_off = 0;
    iph->ttl = 0xff;

    __u64 csum = 0;
    ipv4_csum_inline(iph, &csum);
    iph->check = csum;
}

SEC("xdp_egress")
int xdp_egress_prog(struct xdp_md *ctx)
{
    struct homa_meta_info *data_meta = NULL;
    void *data_end = NULL;
    void *data = NULL;
    struct ethhdr *eth;
    struct iphdr *iph;
    struct common_header *c;
    struct data_header *d;
    int action, ret;
    __u64 rpc_qid = MAX_BUCKET_SIZE;
    bool trigger = false;

    /* adjust data_meta to access metadata in headroom */
    CHECK_AND_DROP_LOG(bpf_xdp_adjust_meta(ctx, -(int)sizeof(*data_meta)) != 0, "xdp_adjust_meta failed");
    
    /* verify after calling bpf_xdp_ajdust_meta() */
    data_meta = (struct homa_meta_info *)(long)ctx->data_meta;
    data_end = (void *)(long)ctx->data_end;
    data = (void *)(long)ctx->data;
    
    CHECK_AND_DROP_LOG(data_meta + 1 > data, "data_meta + 1 > data_end");

    eth = (struct ethhdr *)data;
    iph = (struct iphdr *)(eth + 1);
    c = (struct common_header *)(iph + 1);
    d = (struct data_header *)c;
    
    CHECK_AND_DROP_LOG(d + 1 > data_end, "d + 1 > data_end");

    CHECK_AND_DROP_LOG(iph->protocol != IPPROTO_HOMA, "not HOMA protocol");

    /* this packet is sent from control path directly */
    if (unlikely(data_meta->tx.slowpath)) {
        return xmit_packet(ctx, eth, iph);
    }
    CHECK_AND_DROP_LOG(c->type != DATA, "not DATA packet");
    
    if (unlikely(d->retransmit)) {
        // TODO: we should throttle retransmitted packets
        return xmit_packet(ctx, eth, iph);
    }
    
    __u64 buffer_addr = data_meta->tx.buffer_addr;

    if (rpc_is_client(bpf_be64_to_cpu(d->common.sender_id)))
        action = client_request(iph, d, buffer_addr, &rpc_qid, &trigger);
    else
        action = server_response(iph, d, buffer_addr, &rpc_qid, &trigger);
    
    CHECK_AND_DROP_LOG(action != XDP_TX && action != XDP_REDIRECT, "action != XDP_TX && action != XDP_REDIRECT");

    /* piggyback ACK in this packet to free server rpc at remote side */
    struct dead_client_rpc_info dead_crpc = {0};
    ret = dequeue_dead_crpc(bpf_ntohl(iph->daddr), &dead_crpc);
    if (!ret) {
        d->seg.ack.rpcid = bpf_cpu_to_be64(dead_crpc.rpcid);
        d->seg.ack.dport = bpf_htons(dead_crpc.remote_port);
        d->seg.ack.sport = bpf_htons(dead_crpc.local_port);
    }
    else {
        d->seg.ack.rpcid = 0;
        d->seg.ack.dport = 0;
        d->seg.ack.sport = 0;
    }

    fill_ip_hdr(iph, (data_end - data));

    if (action == XDP_TX)
        return xmit_packet(ctx, eth, iph);
    
    ret = enqueue_pkt_to_rl(ctx, rpc_qid, eth, iph);
    
    if (trigger)
        kick_pacer();
    
    return ret;
}

SEC(".bss.lb_cnt") int lb_cnt = 0;

SEC("xdp_sock")
int xdp_sock_prog(struct xdp_md *ctx)
{
    unsigned int socket_id = 0;
    struct homa_meta_info *data_meta = NULL;
    void *data = NULL;
    void *data_end = NULL;
    struct hdr_cursor nh = {0};
    
    struct iphdr *iph;
    struct common_header *homa_common_hdr;
    struct data_header *homa_data_hdr;
    struct grant_header *homa_grant_hdr;
    struct busy_header *homa_busy_hdr;
    struct resend_header *homa_resend_hdr;
    struct unknown_header *homa_unknown_hdr;
    struct slow_path_info *sp;
    
    int proto_type;
    int single_packet = 0;
    int ret = 0;
    __u32 remote_ip = 0;
    __u32 qid = ctx->rx_queue_index;
    __u16 local_port = 0;
    struct target_xsk *target_xsk;

    #ifdef TEST_PACKET_LOST
    __u32 rand = bpf_get_prandom_u32();
    if (rand % 1000 == 500)
        return XDP_DROP;
    #endif

    unsigned int current_cpu = bpf_get_smp_processor_id();
    CHECK_AND_DROP_LOG(current_cpu >= MAX_CPU || current_cpu != ctx->rx_queue_index, "CPU Mapping");

    CHECK_AND_DROP_LOG(bpf_xdp_adjust_meta(ctx, -(int)sizeof(*data_meta)) != 0, "xdp_adjust_meta failed");

    data_meta = (struct homa_meta_info *)(long)ctx->data_meta;
    data_end = (void *)(long)ctx->data_end;
    data = (void *)(long)ctx->data;

    CHECK_AND_DROP_LOG(data_meta + 1 > data, "data_meta + 1 > data_end");

    data_meta->rx.reap_client_buffer_addr = POISON_64;
    data_meta->rx.reap_server_buffer_addr = POISON_64;
    data_meta->rx.qid = ctx->rx_queue_index;

    /* Ethernet and IP header has already been parsed by the entrance program */
    nh.pos = data + sizeof(struct ethhdr) + sizeof(struct iphdr);
    iph = (struct iphdr *)(data + sizeof(struct ethhdr));

    proto_type = homa_parse_common_hdr(&nh, data_end, &homa_common_hdr);
    CHECK_AND_DROP_LOG(proto_type < 0, "homa_parse_common_hdr failed");
    
    remote_ip = bpf_ntohl(iph->saddr);
    local_port = bpf_ntohs(homa_common_hdr->dport);

    if (unlikely(proto_type != DATA))
        goto ctrl_pkt;
    
    single_packet = homa_parse_data_hdr(&nh, data_end, &homa_data_hdr);
    CHECK_AND_DROP_LOG(single_packet < 0, "homa_parse_data_hdr failed");

// load balancing
#ifdef LB
    unsigned int target_cpu = current_cpu;

    // All short messages bypass load balancing mechanism
    if (likely(single_packet))
        goto bypass_lb;

    set_current_active(current_cpu);

    // we enforce load balancing in batch
    // case1: use cached choice
    if (use_cached_lb_choice[current_cpu] && lb_threshold[current_cpu] && lb_threshold[current_cpu] < LB_THRESHOLD)
    {
        target_cpu = lb_cache_choice[current_cpu];
        lb_threshold[current_cpu]++;
        // bpf_printk("Cache choice, target_cpu = %d, threshold = %d", target_cpu, lb_threshold[current_cpu]);
        if (lb_threshold[current_cpu] == LB_THRESHOLD)
            lb_threshold[current_cpu] = 0;
    }
    // case2: choose a new core
    else if (!lb_threshold[current_cpu] || use_cached_lb_choice[current_cpu] == 0)
    {
        use_cached_lb_choice[current_cpu] = 1;
        lb_threshold[current_cpu] = 1;
        target_cpu = choose_core(current_cpu);
        lb_cache_choice[current_cpu] = target_cpu;
        // bpf_printk("New choice, target_cpu = %d, threshold = %d", target_cpu, lb_threshold[current_cpu]);
    }

    if (target_cpu != current_cpu)
    {
        // bpf_printk("CPU#%d -> CPU#%d", current_cpu, target_cpu);
        return bpf_redirect_map(&cpumap, target_cpu, 0);
    }
// bpf_printk("Process at CPU#%d locally\n", current_cpu);
#else
    goto bypass_lb;
#endif

bypass_lb:

    reclaim_rpc(homa_data_hdr, remote_ip, data_meta);
    
    if (rpc_is_client(local_id(bpf_be64_to_cpu(homa_data_hdr->common.sender_id))))
        ret = client_response(homa_data_hdr, remote_ip, data_meta, single_packet);
    else
        ret = server_request(homa_data_hdr, remote_ip, single_packet);

    CHECK_AND_DROP_LOG(ret == XDP_DROP, "XDP_DROP for error rpc state");

    target_xsk = bpf_map_lookup_elem(&port_tbl, &local_port);
    CHECK_AND_DROP_LOG(!target_xsk, "Can't find corresponding XSK fd for this packet");
    
    socket_id = target_xsk->xsk_map_idx[current_cpu];
    CHECK_AND_DROP_LOG(socket_id < 0, "socket_id < 0");

    return bpf_redirect_map(&xsks_map, socket_id, XDP_DROP);

ctrl_pkt:

    switch (proto_type)
    {
        case RESEND:
            CHECK_AND_DROP(homa_parse_resend_hdr(&nh, data_end, &homa_resend_hdr) != 0);
            ret = resend_pkt(homa_resend_hdr, data_meta, remote_ip);
            if (ret == UNKNOWN || ret == BUSY) {
                return xmit_ctrl_pkt(ctx, ret);
            }
            goto drop;
        case UNKNOWN:
            CHECK_AND_DROP(homa_parse_unknown_hdr(&nh, data_end, &homa_unknown_hdr) != 0);
            ret = unknown_pkt(ctx, homa_unknown_hdr, data_meta, data_end, remote_ip);
            goto drop;
        case GRANT:
            CHECK_AND_DROP(homa_parse_grant_hdr(&nh, data_end, &homa_grant_hdr) != 0);
            grant_pkt(homa_grant_hdr, remote_ip);
            goto drop;
        case BUSY:
            CHECK_AND_DROP(homa_parse_busy_hdr(&nh, data_end, &homa_busy_hdr) != 0);
            busy_pkt(homa_busy_hdr, remote_ip);
            goto drop;
        default:
            return XDP_DROP;
    }
    
    /* redirect to slow_path */
    sp = bpf_map_lookup_elem(&slow_path_map, &qid);
    CHECK_AND_DROP_LOG(!sp || !sp->active, "slow_path_info not found or inactive");
    
    return bpf_redirect_map(&xsks_map, sp->sp_xsk_map_key, XDP_DROP);

drop:
    target_xsk = bpf_map_lookup_elem(&port_tbl, &local_port);
    CHECK_AND_DROP_LOG(!target_xsk, "Can't find corresponding XSK fd for this packet");
    
    socket_id = target_xsk->xsk_map_idx[current_cpu];
    CHECK_AND_DROP_LOG(socket_id < 0, "socket_id < 0");
    
    return bpf_redirect_map(&xsks_map, socket_id, XDP_DROP);
}

SEC("xdp/cpumap")
int xdp_cpumap_prog(struct xdp_md *ctx)
{
    unsigned int socket_id = 0;
    void *data = NULL;
    void *data_end = NULL;
    struct homa_meta_info *data_meta = NULL;
    struct hdr_cursor nh = {0};
    struct ethhdr *eth;
    struct iphdr *iph;
    struct common_header *homa_common_hdr;
    struct data_header *homa_data_hdr;
    int eth_type;
    int ip_type;
    int proto_type;
    int single_packet = 0;
    int is_client;
    int ret;

    __u32 remote_ip;
    __u64 rpcid;
    __u16 local_port;

    struct target_xsk *target_xsk;

    unsigned int current_cpu = bpf_get_smp_processor_id();
    if (current_cpu >= MAX_CPU || current_cpu != ctx->rx_queue_index)
    {
        bpf_printk("XDP/CPUMAP ERROR: CPU Mapping, current_cpu=%d, ctx->rx_queue_index = %d\n", current_cpu,
                   ctx->rx_queue_index);
        return XDP_DROP;
    }

    //   bpf_printk("XDP/CPUMAP at CPU#%d", current_cpu);

    set_current_active(current_cpu);

    data_meta = (struct homa_meta_info *)(long)ctx->data_meta;
    data_end = (void *)(long)ctx->data_end;
    data = (void *)(long)ctx->data;
    nh.pos = data;

    if (unlikely(data_meta + 1 > data))
    {
        bpf_printk("ERROR: data_meta + 1 > data_end\n");
        __sync_fetch_and_sub(&core_info[current_cpu].softirq_backlog, 1);
        return XDP_DROP;
    }

    // Ethernet header
    eth_type = parse_ethhdr(&nh, data_end, &eth);
    if (unlikely(eth_type != bpf_htons(ETH_P_IP)))
    {
        __sync_fetch_and_sub(&core_info[current_cpu].softirq_backlog, 1);
        return XDP_PASS;
    }

    // IPv4 header
    ip_type = parse_iphdr(&nh, data_end, &iph);
    if (unlikely(ip_type != IPPROTO_HOMA))
    {
        __sync_fetch_and_sub(&core_info[current_cpu].softirq_backlog, 1);
        return XDP_PASS;
    }

    remote_ip = bpf_ntohl(iph->saddr);

    // Homa common header
    proto_type = homa_parse_common_hdr(&nh, data_end, &homa_common_hdr);
    if (unlikely(proto_type != DATA))
        goto err_pkt;

    // Homa data header
    single_packet = homa_parse_data_hdr(&nh, data_end, &homa_data_hdr);
    if (unlikely(single_packet < 0))
    {
        __sync_fetch_and_sub(&core_info[current_cpu].softirq_backlog, 1);
        return XDP_DROP;
    }

    set_current_active(current_cpu);

    rpcid = bpf_be64_to_cpu(homa_data_hdr->common.sender_id);
    rpcid = local_id(rpcid);

    is_client = rpc_is_client(rpcid);

    if (unlikely(homa_data_hdr->retransmit))
        goto bypass_reclaim;

    reclaim_rpc(homa_data_hdr, remote_ip, data_meta);

bypass_reclaim:

    local_port = bpf_ntohs(homa_data_hdr->common.dport);
    if (is_client)
        ret = client_response(homa_data_hdr, remote_ip, data_meta, single_packet);
    else
        ret = server_request(homa_data_hdr, remote_ip, single_packet);

    if (ret == XDP_DROP)
    {
        __sync_fetch_and_sub(&core_info[current_cpu].softirq_backlog, 1);
        log_err("XDP_DROP for error rpc state\n");
        return XDP_DROP;
    }

    target_xsk = bpf_map_lookup_elem(&port_tbl, &local_port);
    if (unlikely(!target_xsk))
    {
        // bpf_printk("CPUMAP ERROR: Can't find corresponding XSK fd for this packet, drop it, %u.\n", local_port);
        __sync_fetch_and_sub(&core_info[current_cpu].softirq_backlog, 1);
        return XDP_DROP;
    }
    socket_id = target_xsk->xsk_map_idx[current_cpu];
    if (socket_id < 0)
    {
        bpf_printk("ERROR: socket_id < 0\n");
        __sync_fetch_and_sub(&core_info[current_cpu].softirq_backlog, 1);
        return XDP_DROP;
    }

    __sync_fetch_and_sub(&core_info[current_cpu].softirq_backlog, 1);

    // bpf_printk("CPUMAP#%d --> socket_id:%d", current_cpu, socket_id);
    return bpf_redirect_map(&xsks_map, socket_id, XDP_DROP);

err_pkt:
    __sync_fetch_and_sub(&core_info[current_cpu].softirq_backlog, 1);
    return XDP_DROP;
}

SEC("xdp_gen/choose_rpc_to_grant")
int choose_rpc_to_grant_prog(struct xdp_md *ctx)
{
    DECALRE_NODES_8(struct rpc_state_cc __kptr *, cc_node);
    __u32 cpu = bpf_get_smp_processor_id();
    if (unlikely(cpu >= MAX_CPU))
    {
        bpf_printk("ERROR: CPU Mapping, cpu=%d\n", cpu);
        return XDP_ABORTED;
    }
    struct rpc_state_cc *n = NULL;
    struct bpf_rb_node *rb_node = NULL;
    __u16 nr_rpc = 0;
    struct remove_info *ri = NULL;
    __u16 next_peer_id = 0;
    __u32 min_last_bytes_remaining = 0;
    __u32 new_grant = 0;
    int available = 0;
    __u32 increment = 0;
    __u32 total_increment = 0;
    int priority = 0;
    int extra_levels = 0;
    int prio_idx = 0;
    int actual_rpc = 0;

    if (!try_grantable_lock())
    {
        return XDP_ABORTED;
    }

    ri = bpf_map_lookup_elem(&per_cpu_remove_info, ZERO_KEY);
    if (!ri)
    {
        release_grantable_lock();
        return XDP_ABORTED;
    }

    // Step1: init grant objects
    cc_node_0 = bpf_obj_new(typeof(*cc_node_0));
    if (!cc_node_0)
    {
        release_grantable_lock();
        return XDP_ABORTED;
    }
    cc_node_1 = bpf_refcount_acquire(cc_node_0);
    if (!cc_node_1)
    {
        bpf_obj_drop(cc_node_0);
        release_grantable_lock();
        return XDP_ABORTED;
    }
    cc_node_2 = bpf_refcount_acquire(cc_node_0);
    if (!cc_node_2)
    {
        bpf_obj_drop(cc_node_0);
        bpf_obj_drop(cc_node_1);
        release_grantable_lock();
        return XDP_ABORTED;
    }
    cc_node_3 = bpf_refcount_acquire(cc_node_0);
    if (!cc_node_3)
    {
        bpf_obj_drop(cc_node_0);
        bpf_obj_drop(cc_node_1);
        bpf_obj_drop(cc_node_2);
        release_grantable_lock();
        return XDP_ABORTED;
    }
    cc_node_4 = bpf_refcount_acquire(cc_node_0);
    if (!cc_node_4)
    {
        bpf_obj_drop(cc_node_0);
        bpf_obj_drop(cc_node_1);
        bpf_obj_drop(cc_node_2);
        bpf_obj_drop(cc_node_3);
        release_grantable_lock();
        return XDP_ABORTED;
    }
    cc_node_5 = bpf_refcount_acquire(cc_node_0);
    if (!cc_node_5)
    {
        bpf_obj_drop(cc_node_0);
        bpf_obj_drop(cc_node_1);
        bpf_obj_drop(cc_node_2);
        bpf_obj_drop(cc_node_3);
        bpf_obj_drop(cc_node_4);
        release_grantable_lock();
        return XDP_ABORTED;
    }
    cc_node_6 = bpf_refcount_acquire(cc_node_0);
    if (!cc_node_6)
    {
        bpf_obj_drop(cc_node_0);
        bpf_obj_drop(cc_node_1);
        bpf_obj_drop(cc_node_2);
        bpf_obj_drop(cc_node_3);
        bpf_obj_drop(cc_node_4);
        bpf_obj_drop(cc_node_5);
        release_grantable_lock();
        return XDP_ABORTED;
    }
    cc_node_7 = bpf_refcount_acquire(cc_node_0);
    if (!cc_node_7)
    {
        bpf_obj_drop(cc_node_0);
        bpf_obj_drop(cc_node_1);
        bpf_obj_drop(cc_node_2);
        bpf_obj_drop(cc_node_3);
        bpf_obj_drop(cc_node_4);
        bpf_obj_drop(cc_node_5);
        bpf_obj_drop(cc_node_6);
        release_grantable_lock();
        return XDP_ABORTED;
    }

    // step2: choose rpcs to grant (not dequeue)
    GRANT_LOCK();

    cc_node_0->tree_id = 1;
    cc_node_0->bytes_remaining = min_last_bytes_remaining;
    cc_node_0->peer_id = next_peer_id;
    rb_node = bpf_rbtree_lower_bound(&groot, &cc_node_0->rbtree_link, srpt_less_peer);
    cc_node_0 = NULL;
    if (rb_node)
    {
        n = container_of(rb_node, struct rpc_state_cc, rbtree_link);
        // no need to check, if rb_node!= NULL, tree_id must be 1
        min_last_bytes_remaining = n->bytes_remaining;
        ri->rpcid[0] = n->hkey.rpcid;
        ri->local_port[0] = n->hkey.local_port;
        ri->remote_port[0] = n->hkey.remote_port;
        ri->remote_ip[0] = n->hkey.remote_ip;
        next_peer_id = get_peerid(n->hkey.remote_ip) + 1;

        // grant the rpc
        new_grant = n->message_length - n->bytes_remaining + Homa_grant_window;
        if (new_grant > n->message_length)
            new_grant = n->message_length;
        available = Homa_max_incoming - total_incoming;
        increment = new_grant - n->incoming;
        if (increment > 0 && available > 0)
        {
            if (increment > available)
            {
                increment = available;
                new_grant = n->incoming + increment;
            }

            n->incoming = new_grant;
            remove[cpu][0] = n->incoming >= n->message_length;
            if (remove[cpu][0])
            {
                rb_node = bpf_rbtree_remove(&groot, &n->rbtree_link);
                if (rb_node)
                {
                    n = container_of(rb_node, struct rpc_state_cc, rbtree_link);
                    n->tree_id = 0;
                    rb_node = bpf_rbtree_lower_bound(&groot, &n->rbtree_link, srpt_less_rpc);
                    if (rb_node)
                    {
                        n = container_of(rb_node, struct rpc_state_cc, rbtree_link);

                        rb_node = bpf_rbtree_remove(&groot, &n->rbtree_link);
                        cc_node_0 = rb_node ? container_of(rb_node, struct rpc_state_cc, rbtree_link) : NULL;
                    }
                }
            }
            total_increment += increment;
            ri->newgrant[0] = new_grant;
        }
        nr_rpc++;
    }
    else
        goto out;
    cc_node_1->tree_id = 1;
    cc_node_1->bytes_remaining = min_last_bytes_remaining;
    cc_node_1->peer_id = next_peer_id;
    rb_node = bpf_rbtree_lower_bound(&groot, &cc_node_1->rbtree_link, srpt_less_peer);
    cc_node_1 = NULL;
    if (rb_node)
    {
        n = container_of(rb_node, struct rpc_state_cc, rbtree_link);
        // no need to check, if rb_node!= NULL, tree_id must be 1
        min_last_bytes_remaining = n->bytes_remaining;
        ri->rpcid[1] = n->hkey.rpcid;
        ri->local_port[1] = n->hkey.local_port;
        ri->remote_port[1] = n->hkey.remote_port;
        ri->remote_ip[1] = n->hkey.remote_ip;
        next_peer_id = get_peerid(n->hkey.remote_ip) + 1;

        // grant the rpc
        new_grant = n->message_length - n->bytes_remaining + Homa_grant_window;
        if (new_grant > n->message_length)
            new_grant = n->message_length;
        available = Homa_max_incoming - total_incoming;
        increment = new_grant - n->incoming;
        if (increment > 0 && available > 0)
        {
            if (increment > available)
            {
                increment = available;
                new_grant = n->incoming + increment;
            }

            n->incoming = new_grant;
            remove[cpu][1] = n->incoming >= n->message_length;
            if (remove[cpu][1])
            {
                rb_node = bpf_rbtree_remove(&groot, &n->rbtree_link);
                if (rb_node)
                {
                    n = container_of(rb_node, struct rpc_state_cc, rbtree_link);
                    n->tree_id = 0;
                    rb_node = bpf_rbtree_lower_bound(&groot, &n->rbtree_link, srpt_less_rpc);
                    if (rb_node)
                    {
                        n = container_of(rb_node, struct rpc_state_cc, rbtree_link);

                        rb_node = bpf_rbtree_remove(&groot, &n->rbtree_link);
                        cc_node_1 = rb_node ? container_of(rb_node, struct rpc_state_cc, rbtree_link) : NULL;
                    }
                }
            }
            total_increment += increment;
            ri->newgrant[1] = new_grant;
        }
        nr_rpc++;
    }
    else
        goto out;
    cc_node_2->tree_id = 1;
    cc_node_2->bytes_remaining = min_last_bytes_remaining;
    cc_node_2->peer_id = next_peer_id;
    rb_node = bpf_rbtree_lower_bound(&groot, &cc_node_2->rbtree_link, srpt_less_peer);
    cc_node_2 = NULL;
    if (rb_node)
    {
        n = container_of(rb_node, struct rpc_state_cc, rbtree_link);
        // no need to check, if rb_node!= NULL, tree_id must be 1
        min_last_bytes_remaining = n->bytes_remaining;
        ri->rpcid[2] = n->hkey.rpcid;
        ri->local_port[2] = n->hkey.local_port;
        ri->remote_port[2] = n->hkey.remote_port;
        ri->remote_ip[2] = n->hkey.remote_ip;
        next_peer_id = get_peerid(n->hkey.remote_ip) + 1;

        // grant the rpc
        new_grant = n->message_length - n->bytes_remaining + Homa_grant_window;
        if (new_grant > n->message_length)
            new_grant = n->message_length;
        available = Homa_max_incoming - total_incoming;
        increment = new_grant - n->incoming;
        if (increment > 0 && available > 0)
        {
            if (increment > available)
            {
                increment = available;
                new_grant = n->incoming + increment;
            }

            n->incoming = new_grant;
            remove[cpu][2] = n->incoming >= n->message_length;
            if (remove[cpu][2])
            {
                rb_node = bpf_rbtree_remove(&groot, &n->rbtree_link);
                if (rb_node)
                {
                    n = container_of(rb_node, struct rpc_state_cc, rbtree_link);
                    n->tree_id = 0;
                    rb_node = bpf_rbtree_lower_bound(&groot, &n->rbtree_link, srpt_less_rpc);
                    if (rb_node)
                    {
                        n = container_of(rb_node, struct rpc_state_cc, rbtree_link);

                        rb_node = bpf_rbtree_remove(&groot, &n->rbtree_link);
                        cc_node_2 = rb_node ? container_of(rb_node, struct rpc_state_cc, rbtree_link) : NULL;
                    }
                }
            }
            total_increment += increment;
            ri->newgrant[2] = new_grant;
        }
        nr_rpc++;
    }
    else
        goto out;
    cc_node_3->tree_id = 1;
    cc_node_3->bytes_remaining = min_last_bytes_remaining;
    cc_node_3->peer_id = next_peer_id;
    rb_node = bpf_rbtree_lower_bound(&groot, &cc_node_3->rbtree_link, srpt_less_peer);
    cc_node_3 = NULL;
    if (rb_node)
    {
        n = container_of(rb_node, struct rpc_state_cc, rbtree_link);
        // no need to check, if rb_node!= NULL, tree_id must be 1
        min_last_bytes_remaining = n->bytes_remaining;
        ri->rpcid[3] = n->hkey.rpcid;
        ri->local_port[3] = n->hkey.local_port;
        ri->remote_port[3] = n->hkey.remote_port;
        ri->remote_ip[3] = n->hkey.remote_ip;
        next_peer_id = get_peerid(n->hkey.remote_ip) + 1;

        // grant the rpc
        new_grant = n->message_length - n->bytes_remaining + Homa_grant_window;
        if (new_grant > n->message_length)
            new_grant = n->message_length;
        available = Homa_max_incoming - total_incoming;
        increment = new_grant - n->incoming;
        if (increment > 0 && available > 0)
        {
            if (increment > available)
            {
                increment = available;
                new_grant = n->incoming + increment;
            }

            n->incoming = new_grant;
            remove[cpu][3] = n->incoming >= n->message_length;
            if (remove[cpu][3])
            {
                rb_node = bpf_rbtree_remove(&groot, &n->rbtree_link);
                if (rb_node)
                {
                    n = container_of(rb_node, struct rpc_state_cc, rbtree_link);
                    n->tree_id = 0;
                    rb_node = bpf_rbtree_lower_bound(&groot, &n->rbtree_link, srpt_less_rpc);
                    if (rb_node)
                    {
                        n = container_of(rb_node, struct rpc_state_cc, rbtree_link);

                        rb_node = bpf_rbtree_remove(&groot, &n->rbtree_link);
                        cc_node_3 = rb_node ? container_of(rb_node, struct rpc_state_cc, rbtree_link) : NULL;
                    }
                }
            }
            total_increment += increment;
            ri->newgrant[3] = new_grant;
        }
        nr_rpc++;
    }
    else
        goto out;
    cc_node_4->tree_id = 1;
    cc_node_4->bytes_remaining = min_last_bytes_remaining;
    cc_node_4->peer_id = next_peer_id;
    rb_node = bpf_rbtree_lower_bound(&groot, &cc_node_4->rbtree_link, srpt_less_peer);
    cc_node_4 = NULL;
    if (rb_node)
    {
        n = container_of(rb_node, struct rpc_state_cc, rbtree_link);
        // no need to check, if rb_node!= NULL, tree_id must be 1
        min_last_bytes_remaining = n->bytes_remaining;
        ri->rpcid[4] = n->hkey.rpcid;
        ri->local_port[4] = n->hkey.local_port;
        ri->remote_port[4] = n->hkey.remote_port;
        ri->remote_ip[4] = n->hkey.remote_ip;
        next_peer_id = get_peerid(n->hkey.remote_ip) + 1;

        // grant the rpc
        new_grant = n->message_length - n->bytes_remaining + Homa_grant_window;
        if (new_grant > n->message_length)
            new_grant = n->message_length;
        available = Homa_max_incoming - total_incoming;
        increment = new_grant - n->incoming;
        if (increment > 0 && available > 0)
        {
            if (increment > available)
            {
                increment = available;
                new_grant = n->incoming + increment;
            }

            n->incoming = new_grant;
            remove[cpu][4] = n->incoming >= n->message_length;
            if (remove[cpu][4])
            {
                rb_node = bpf_rbtree_remove(&groot, &n->rbtree_link);
                if (rb_node)
                {
                    n = container_of(rb_node, struct rpc_state_cc, rbtree_link);
                    n->tree_id = 0;
                    rb_node = bpf_rbtree_lower_bound(&groot, &n->rbtree_link, srpt_less_rpc);
                    if (rb_node)
                    {
                        n = container_of(rb_node, struct rpc_state_cc, rbtree_link);

                        rb_node = bpf_rbtree_remove(&groot, &n->rbtree_link);
                        cc_node_4 = rb_node ? container_of(rb_node, struct rpc_state_cc, rbtree_link) : NULL;
                    }
                }
            }
            total_increment += increment;
            ri->newgrant[4] = new_grant;
        }
        nr_rpc++;
    }
    else
        goto out;
    cc_node_5->tree_id = 1;
    cc_node_5->bytes_remaining = min_last_bytes_remaining;
    cc_node_5->peer_id = next_peer_id;
    rb_node = bpf_rbtree_lower_bound(&groot, &cc_node_5->rbtree_link, srpt_less_peer);
    cc_node_5 = NULL;
    if (rb_node)
    {
        n = container_of(rb_node, struct rpc_state_cc, rbtree_link);
        // no need to check, if rb_node!= NULL, tree_id must be 1
        min_last_bytes_remaining = n->bytes_remaining;
        ri->rpcid[5] = n->hkey.rpcid;
        ri->local_port[5] = n->hkey.local_port;
        ri->remote_port[5] = n->hkey.remote_port;
        ri->remote_ip[5] = n->hkey.remote_ip;
        next_peer_id = get_peerid(n->hkey.remote_ip) + 1;

        // grant the rpc
        new_grant = n->message_length - n->bytes_remaining + Homa_grant_window;
        if (new_grant > n->message_length)
            new_grant = n->message_length;
        available = Homa_max_incoming - total_incoming;
        increment = new_grant - n->incoming;
        if (increment > 0 && available > 0)
        {
            if (increment > available)
            {
                increment = available;
                new_grant = n->incoming + increment;
            }

            n->incoming = new_grant;
            remove[cpu][5] = n->incoming >= n->message_length;
            if (remove[cpu][5])
            {
                rb_node = bpf_rbtree_remove(&groot, &n->rbtree_link);
                if (rb_node)
                {
                    n = container_of(rb_node, struct rpc_state_cc, rbtree_link);
                    n->tree_id = 0;
                    rb_node = bpf_rbtree_lower_bound(&groot, &n->rbtree_link, srpt_less_rpc);
                    if (rb_node)
                    {
                        n = container_of(rb_node, struct rpc_state_cc, rbtree_link);

                        rb_node = bpf_rbtree_remove(&groot, &n->rbtree_link);
                        cc_node_5 = rb_node ? container_of(rb_node, struct rpc_state_cc, rbtree_link) : NULL;
                    }
                }
            }
            total_increment += increment;
            ri->newgrant[5] = new_grant;
        }
        nr_rpc++;
    }
    else
        goto out;
    cc_node_6->tree_id = 1;
    cc_node_6->bytes_remaining = min_last_bytes_remaining;
    cc_node_6->peer_id = next_peer_id;
    rb_node = bpf_rbtree_lower_bound(&groot, &cc_node_6->rbtree_link, srpt_less_peer);
    cc_node_6 = NULL;
    if (rb_node)
    {
        n = container_of(rb_node, struct rpc_state_cc, rbtree_link);
        // no need to check, if rb_node!= NULL, tree_id must be 1
        min_last_bytes_remaining = n->bytes_remaining;
        ri->rpcid[6] = n->hkey.rpcid;
        ri->local_port[6] = n->hkey.local_port;
        ri->remote_port[6] = n->hkey.remote_port;
        ri->remote_ip[6] = n->hkey.remote_ip;
        next_peer_id = get_peerid(n->hkey.remote_ip) + 1;

        // grant the rpc
        new_grant = n->message_length - n->bytes_remaining + Homa_grant_window;
        if (new_grant > n->message_length)
            new_grant = n->message_length;
        available = Homa_max_incoming - total_incoming;
        increment = new_grant - n->incoming;
        if (increment > 0 && available > 0)
        {
            if (increment > available)
            {
                increment = available;
                new_grant = n->incoming + increment;
            }

            n->incoming = new_grant;
            remove[cpu][6] = n->incoming >= n->message_length;
            if (remove[cpu][6])
            {
                rb_node = bpf_rbtree_remove(&groot, &n->rbtree_link);
                if (rb_node)
                {
                    n = container_of(rb_node, struct rpc_state_cc, rbtree_link);
                    n->tree_id = 0;
                    rb_node = bpf_rbtree_lower_bound(&groot, &n->rbtree_link, srpt_less_rpc);
                    if (rb_node)
                    {
                        n = container_of(rb_node, struct rpc_state_cc, rbtree_link);

                        rb_node = bpf_rbtree_remove(&groot, &n->rbtree_link);
                        cc_node_6 = rb_node ? container_of(rb_node, struct rpc_state_cc, rbtree_link) : NULL;
                    }
                }
            }
            total_increment += increment;
            ri->newgrant[6] = new_grant;
        }
        nr_rpc++;
    }
    else
        goto out;
    cc_node_7->tree_id = 1;
    cc_node_7->bytes_remaining = min_last_bytes_remaining;
    cc_node_7->peer_id = next_peer_id;
    rb_node = bpf_rbtree_lower_bound(&groot, &cc_node_7->rbtree_link, srpt_less_peer);
    cc_node_7 = NULL;
    if (rb_node)
    {
        n = container_of(rb_node, struct rpc_state_cc, rbtree_link);
        // no need to check, if rb_node!= NULL, tree_id must be 1
        min_last_bytes_remaining = n->bytes_remaining;
        ri->rpcid[7] = n->hkey.rpcid;
        ri->local_port[7] = n->hkey.local_port;
        ri->remote_port[7] = n->hkey.remote_port;
        ri->remote_ip[7] = n->hkey.remote_ip;
        next_peer_id = get_peerid(n->hkey.remote_ip) + 1;

        // grant the rpc
        new_grant = n->message_length - n->bytes_remaining + Homa_grant_window;
        if (new_grant > n->message_length)
            new_grant = n->message_length;
        available = Homa_max_incoming - total_incoming;
        increment = new_grant - n->incoming;
        if (increment > 0 && available > 0)
        {
            if (increment > available)
            {
                increment = available;
                new_grant = n->incoming + increment;
            }

            n->incoming = new_grant;
            remove[cpu][7] = n->incoming >= n->message_length;
            if (remove[cpu][7])
            {
                rb_node = bpf_rbtree_remove(&groot, &n->rbtree_link);
                if (rb_node)
                {
                    n = container_of(rb_node, struct rpc_state_cc, rbtree_link);
                    n->tree_id = 0;
                    rb_node = bpf_rbtree_lower_bound(&groot, &n->rbtree_link, srpt_less_rpc);
                    if (rb_node)
                    {
                        n = container_of(rb_node, struct rpc_state_cc, rbtree_link);

                        rb_node = bpf_rbtree_remove(&groot, &n->rbtree_link);
                        cc_node_7 = rb_node ? container_of(rb_node, struct rpc_state_cc, rbtree_link) : NULL;
                    }
                }
            }
            total_increment += increment;
            ri->newgrant[7] = new_grant;
        }
        nr_rpc++;
    }

out:
    __sync_fetch_and_add(&total_incoming, total_increment);

    grant_nonfifo_left -= total_increment;
    if (grant_nonfifo_left <= 0)
    {
        grant_nonfifo_left += grant_nonfifo;
#ifndef DISABLE_GRANT_FIFO
        need_grant_fifo[cpu] = 1;
#endif
    }

    GRANT_UNLOCK();

    if (cc_node_0)
        bpf_obj_drop(cc_node_0);
    if (cc_node_1)
        bpf_obj_drop(cc_node_1);
    if (cc_node_2)
        bpf_obj_drop(cc_node_2);
    if (cc_node_3)
        bpf_obj_drop(cc_node_3);
    if (cc_node_4)
        bpf_obj_drop(cc_node_4);
    if (cc_node_5)
        bpf_obj_drop(cc_node_5);
    if (cc_node_6)
        bpf_obj_drop(cc_node_6);
    if (cc_node_7)
        bpf_obj_drop(cc_node_7);

    //   bpf_printk("%d RPCs are choosen to grant", nr_rpc);
    if (nr_rpc == 0)
    {
#ifdef HELP_PACER
        help_pacer();
#endif
        release_grantable_lock();
        return XDP_ABORTED;
    }

    nr_grant_candidate[cpu] = nr_rpc;

    for (int i = 0; i < nr_rpc; i++)
    {
        if (ri->newgrant[i & 7])
        {
            actual_rpc++;
            priority = HOMA_MAX_SCHED_PRIO - (prio_idx++);
            if (priority < 0)
                priority = 0;
            ri->priority[i & 7] = priority;
            // bpf_printk("Grant to RPC#%llu to offset: %lu", ri->rpcid[i&7], ri->newgrant[i&7]);
        }
    }
    nr_grant_ready[cpu] = actual_rpc;

    if (actual_rpc == 0)
    {
#ifdef HELP_PACER
        help_pacer();
#endif
        release_grantable_lock();
        return XDP_ABORTED;
    }

    extra_levels = HOMA_MAX_SCHED_PRIO + 1 - actual_rpc;
    if (extra_levels >= 0)
    {
        for (int i = 0; i < nr_rpc; i++)
        {
            if (ri->newgrant[i & 7])
            {
                priority = ri->priority[i & 7];
                priority -= extra_levels;
                if (priority)
                    ri->priority[i & 7] = priority;
            }
        }
    }
    bpf_tail_call(ctx, &xdp_gen_tail_call_map, XDP_GEN_COMPLETE_GRANT_1);

    // fallthrough: bpf_tail_call failed
    release_grantable_lock();
    return XDP_ABORTED;
}

SEC("xdp_gen/complete_grant_1")
int complete_grant_1_prog(struct xdp_md *ctx)
{
    __u32 cpu = bpf_get_smp_processor_id();
    if (unlikely(cpu >= MAX_CPU))
    {
        bpf_printk("ERROR: CPU Mapping, cpu=%d\n", cpu);
        return XDP_ABORTED;
    }
    __u16 peer_id = 0;
    struct remove_info *ri = NULL;

    if (nr_grant_candidate[cpu] < 1)
    {
        release_grantable_lock();
        if (nr_grant_ready[cpu] > 0)
        {
            finish_grant_choose[cpu] = 1;
            return XDP_DROP;
        }
        else
            return XDP_ABORTED;
    }

    ri = bpf_map_lookup_elem(&per_cpu_remove_info, ZERO_KEY);
    if (!ri)
    {
        // this should never happen
        release_grantable_lock();
        return XDP_ABORTED;
    }

    if (remove[cpu][0])
    {
        struct rpc_state_cc *cc_node_t0 = NULL;
        struct rpc_state_cc *cc_node_t1 = NULL;
        struct bpf_rb_node *rb_node = NULL;
        peer_id = get_peerid(ri->remote_ip[0]);

        cc_node_t0 = bpf_obj_new(typeof(*cc_node_t0));
        if (!cc_node_t0)
        {
            release_grantable_lock();
            return XDP_ABORTED;
        }

        cc_node_t1 = bpf_refcount_acquire(cc_node_t0);
        if (!cc_node_t1)
        {
            bpf_obj_drop(cc_node_t0);
            release_grantable_lock();
            return XDP_ABORTED;
        }

        cc_node_t0->tree_id = 0;
        cc_node_t0->peer_id = peer_id;
        cc_node_t0->bytes_remaining = 0;
        cc_node_t0->hkey.rpcid = 0;
        cc_node_t0->hkey.local_port = 0;
        cc_node_t0->hkey.remote_port = 0;
        cc_node_t0->hkey.remote_ip = 0;
        GRANT_LOCK();
        rb_node = bpf_rbtree_lower_bound(&groot, &cc_node_t0->rbtree_link, srpt_less_rpc);
        if (rb_node)
        {
            cc_node_t0 = container_of(rb_node, struct rpc_state_cc, rbtree_link);
            if (cc_node_t0->tree_id == 0 && cc_node_t0->peer_id == peer_id && (cc_node_t0->birth & 1) == 0)
            {
                // we should add this rpc to peer tree
                cc_node_t1->tree_id = 1;
                cc_node_t1->peer_id = cc_node_t0->peer_id;
                cc_node_t1->bytes_remaining = cc_node_t0->bytes_remaining;
                cc_node_t1->incoming = cc_node_t0->incoming;
                cc_node_t1->hkey.rpcid = cc_node_t0->hkey.rpcid;
                cc_node_t1->hkey.local_port = cc_node_t0->hkey.local_port;
                cc_node_t1->hkey.remote_ip = cc_node_t0->hkey.remote_ip;
                cc_node_t1->hkey.remote_port = cc_node_t0->hkey.remote_port;
                cc_node_t1->message_length = cc_node_t0->message_length;
                // mark this rpc is in peer tree
                cc_node_t0->birth |= (__u64)1;
                cc_node_t1->birth = cc_node_t0->birth;

                bpf_rbtree_add(&groot, &cc_node_t1->rbtree_link, srpt_less_peer);
                cc_node_t1 = NULL;
            }
        }
        GRANT_UNLOCK();
        if (cc_node_t1)
            bpf_obj_drop(cc_node_t1);
    }

    bpf_tail_call(ctx, &xdp_gen_tail_call_map, XDP_GEN_COMPLETE_GRANT_2);
    // fallthrough: bpf_tail_call failed
    release_grantable_lock();
    return XDP_ABORTED;
}

SEC("xdp_gen/complete_grant_2")
int complete_grant_2_prog(struct xdp_md *ctx)
{
    __u32 cpu = bpf_get_smp_processor_id();
    if (unlikely(cpu >= MAX_CPU))
    {
        bpf_printk("ERROR: CPU Mapping, cpu=%d\n", cpu);
        return XDP_ABORTED;
    }
    __u16 peer_id = 0;
    struct remove_info *ri = NULL;

    if (nr_grant_candidate[cpu] < 2)
    {
        release_grantable_lock();
        if (nr_grant_ready[cpu] > 0)
        {
            finish_grant_choose[cpu] = 1;
            return XDP_DROP;
        }
        else
            return XDP_ABORTED;
    }

    ri = bpf_map_lookup_elem(&per_cpu_remove_info, ZERO_KEY);
    if (!ri)
    {
        // this should never happen
        release_grantable_lock();
        return XDP_ABORTED;
    }

    if (remove[cpu][1])
    {
        struct rpc_state_cc *cc_node_t0 = NULL;
        struct rpc_state_cc *cc_node_t1 = NULL;
        struct bpf_rb_node *rb_node = NULL;
        peer_id = get_peerid(ri->remote_ip[1]);

        cc_node_t0 = bpf_obj_new(typeof(*cc_node_t0));
        if (!cc_node_t0)
        {
            release_grantable_lock();
            return XDP_ABORTED;
        }

        cc_node_t1 = bpf_refcount_acquire(cc_node_t0);
        if (!cc_node_t1)
        {
            bpf_obj_drop(cc_node_t0);
            release_grantable_lock();
            return XDP_ABORTED;
        }

        cc_node_t0->tree_id = 0;
        cc_node_t0->peer_id = peer_id;
        cc_node_t0->bytes_remaining = 0;
        cc_node_t0->hkey.rpcid = 0;
        cc_node_t0->hkey.local_port = 0;
        cc_node_t0->hkey.remote_port = 0;
        cc_node_t0->hkey.remote_ip = 0;
        GRANT_LOCK();
        rb_node = bpf_rbtree_lower_bound(&groot, &cc_node_t0->rbtree_link, srpt_less_rpc);
        if (rb_node)
        {
            cc_node_t0 = container_of(rb_node, struct rpc_state_cc, rbtree_link);
            if (cc_node_t0->tree_id == 0 && cc_node_t0->peer_id == peer_id && (cc_node_t0->birth & 1) == 0)
            {
                // we should add this rpc to peer tree
                cc_node_t1->tree_id = 1;
                cc_node_t1->peer_id = cc_node_t0->peer_id;
                cc_node_t1->bytes_remaining = cc_node_t0->bytes_remaining;
                cc_node_t1->incoming = cc_node_t0->incoming;
                cc_node_t1->hkey.rpcid = cc_node_t0->hkey.rpcid;
                cc_node_t1->hkey.local_port = cc_node_t0->hkey.local_port;
                cc_node_t1->hkey.remote_ip = cc_node_t0->hkey.remote_ip;
                cc_node_t1->hkey.remote_port = cc_node_t0->hkey.remote_port;
                cc_node_t1->message_length = cc_node_t0->message_length;
                // mark this rpc is in peer tree
                cc_node_t0->birth |= (__u64)1;
                cc_node_t1->birth = cc_node_t0->birth;

                bpf_rbtree_add(&groot, &cc_node_t1->rbtree_link, srpt_less_peer);
                cc_node_t1 = NULL;
            }
        }
        GRANT_UNLOCK();
        if (cc_node_t1)
            bpf_obj_drop(cc_node_t1);
    }

    bpf_tail_call(ctx, &xdp_gen_tail_call_map, XDP_GEN_COMPLETE_GRANT_3);
    // fallthrough: bpf_tail_call failed
    release_grantable_lock();
    return XDP_ABORTED;
}
SEC("xdp_gen/complete_grant_3")
int complete_grant_3_prog(struct xdp_md *ctx)
{
    __u32 cpu = bpf_get_smp_processor_id();
    if (unlikely(cpu >= MAX_CPU))
    {
        bpf_printk("ERROR: CPU Mapping, cpu=%d\n", cpu);
        return XDP_ABORTED;
    }
    __u16 peer_id = 0;
    struct remove_info *ri = NULL;

    if (nr_grant_candidate[cpu] < 3)
    {
        release_grantable_lock();
        if (nr_grant_ready[cpu] > 0)
        {
            finish_grant_choose[cpu] = 1;
            return XDP_DROP;
        }
        else
            return XDP_ABORTED;
    }

    ri = bpf_map_lookup_elem(&per_cpu_remove_info, ZERO_KEY);
    if (!ri)
    {
        // this should never happen
        release_grantable_lock();
        return XDP_ABORTED;
    }

    if (remove[cpu][2])
    {
        struct rpc_state_cc *cc_node_t0 = NULL;
        struct rpc_state_cc *cc_node_t1 = NULL;
        struct bpf_rb_node *rb_node = NULL;
        peer_id = get_peerid(ri->remote_ip[2]);

        cc_node_t0 = bpf_obj_new(typeof(*cc_node_t0));
        if (!cc_node_t0)
        {
            release_grantable_lock();
            return XDP_ABORTED;
        }

        cc_node_t1 = bpf_refcount_acquire(cc_node_t0);
        if (!cc_node_t1)
        {
            bpf_obj_drop(cc_node_t0);
            release_grantable_lock();
            return XDP_ABORTED;
        }

        cc_node_t0->tree_id = 0;
        cc_node_t0->peer_id = peer_id;
        cc_node_t0->bytes_remaining = 0;
        cc_node_t0->hkey.rpcid = 0;
        cc_node_t0->hkey.local_port = 0;
        cc_node_t0->hkey.remote_port = 0;
        cc_node_t0->hkey.remote_ip = 0;
        GRANT_LOCK();
        rb_node = bpf_rbtree_lower_bound(&groot, &cc_node_t0->rbtree_link, srpt_less_rpc);
        if (rb_node)
        {
            cc_node_t0 = container_of(rb_node, struct rpc_state_cc, rbtree_link);
            if (cc_node_t0->tree_id == 0 && cc_node_t0->peer_id == peer_id && (cc_node_t0->birth & 1) == 0)
            {
                // we should add this rpc to peer tree
                cc_node_t1->tree_id = 1;
                cc_node_t1->peer_id = cc_node_t0->peer_id;
                cc_node_t1->bytes_remaining = cc_node_t0->bytes_remaining;
                cc_node_t1->incoming = cc_node_t0->incoming;
                cc_node_t1->hkey.rpcid = cc_node_t0->hkey.rpcid;
                cc_node_t1->hkey.local_port = cc_node_t0->hkey.local_port;
                cc_node_t1->hkey.remote_ip = cc_node_t0->hkey.remote_ip;
                cc_node_t1->hkey.remote_port = cc_node_t0->hkey.remote_port;
                cc_node_t1->message_length = cc_node_t0->message_length;
                // mark this rpc is in peer tree
                cc_node_t0->birth |= (__u64)1;
                cc_node_t1->birth = cc_node_t0->birth;

                bpf_rbtree_add(&groot, &cc_node_t1->rbtree_link, srpt_less_peer);
                cc_node_t1 = NULL;
            }
        }
        GRANT_UNLOCK();
        if (cc_node_t1)
            bpf_obj_drop(cc_node_t1);
    }

    bpf_tail_call(ctx, &xdp_gen_tail_call_map, XDP_GEN_COMPLETE_GRANT_4);
    // fallthrough: bpf_tail_call failed
    release_grantable_lock();
    return XDP_ABORTED;
}
SEC("xdp_gen/complete_grant_4")
int complete_grant_4_prog(struct xdp_md *ctx)
{
    __u32 cpu = bpf_get_smp_processor_id();
    if (unlikely(cpu >= MAX_CPU))
    {
        bpf_printk("ERROR: CPU Mapping, cpu=%d\n", cpu);
        return XDP_ABORTED;
    }
    __u16 peer_id = 0;
    struct remove_info *ri = NULL;

    if (nr_grant_candidate[cpu] < 4)
    {
        release_grantable_lock();
        if (nr_grant_ready[cpu] > 0)
        {
            finish_grant_choose[cpu] = 1;
            return XDP_DROP;
        }
        else
            return XDP_ABORTED;
    }

    ri = bpf_map_lookup_elem(&per_cpu_remove_info, ZERO_KEY);
    if (!ri)
    {
        // this should never happen
        release_grantable_lock();
        return XDP_ABORTED;
    }

    if (remove[cpu][3])
    {
        struct rpc_state_cc *cc_node_t0 = NULL;
        struct rpc_state_cc *cc_node_t1 = NULL;
        struct bpf_rb_node *rb_node = NULL;
        peer_id = get_peerid(ri->remote_ip[3]);

        cc_node_t0 = bpf_obj_new(typeof(*cc_node_t0));
        if (!cc_node_t0)
        {
            release_grantable_lock();
            return XDP_ABORTED;
        }

        cc_node_t1 = bpf_refcount_acquire(cc_node_t0);
        if (!cc_node_t1)
        {
            bpf_obj_drop(cc_node_t0);
            release_grantable_lock();
            return XDP_ABORTED;
        }

        cc_node_t0->tree_id = 0;
        cc_node_t0->peer_id = peer_id;
        cc_node_t0->bytes_remaining = 0;
        cc_node_t0->hkey.rpcid = 0;
        cc_node_t0->hkey.local_port = 0;
        cc_node_t0->hkey.remote_port = 0;
        cc_node_t0->hkey.remote_ip = 0;
        GRANT_LOCK();
        rb_node = bpf_rbtree_lower_bound(&groot, &cc_node_t0->rbtree_link, srpt_less_rpc);
        if (rb_node)
        {
            cc_node_t0 = container_of(rb_node, struct rpc_state_cc, rbtree_link);
            if (cc_node_t0->tree_id == 0 && cc_node_t0->peer_id == peer_id && (cc_node_t0->birth & 1) == 0)
            {
                // we should add this rpc to peer tree
                cc_node_t1->tree_id = 1;
                cc_node_t1->peer_id = cc_node_t0->peer_id;
                cc_node_t1->bytes_remaining = cc_node_t0->bytes_remaining;
                cc_node_t1->incoming = cc_node_t0->incoming;
                cc_node_t1->hkey.rpcid = cc_node_t0->hkey.rpcid;
                cc_node_t1->hkey.local_port = cc_node_t0->hkey.local_port;
                cc_node_t1->hkey.remote_ip = cc_node_t0->hkey.remote_ip;
                cc_node_t1->hkey.remote_port = cc_node_t0->hkey.remote_port;
                cc_node_t1->message_length = cc_node_t0->message_length;
                // mark this rpc is in peer tree
                cc_node_t0->birth |= (__u64)1;
                cc_node_t1->birth = cc_node_t0->birth;

                bpf_rbtree_add(&groot, &cc_node_t1->rbtree_link, srpt_less_peer);
                cc_node_t1 = NULL;
            }
        }
        GRANT_UNLOCK();
        if (cc_node_t1)
            bpf_obj_drop(cc_node_t1);
    }

    bpf_tail_call(ctx, &xdp_gen_tail_call_map, XDP_GEN_COMPLETE_GRANT_5);
    // fallthrough: bpf_tail_call failed
    release_grantable_lock();
    return XDP_ABORTED;
}
SEC("xdp_gen/complete_grant_5")
int complete_grant_5_prog(struct xdp_md *ctx)
{
    __u32 cpu = bpf_get_smp_processor_id();
    if (unlikely(cpu >= MAX_CPU))
    {
        bpf_printk("ERROR: CPU Mapping, cpu=%d\n", cpu);
        return XDP_ABORTED;
    }
    __u16 peer_id = 0;
    struct remove_info *ri = NULL;

    if (nr_grant_candidate[cpu] < 5)
    {
        release_grantable_lock();
        if (nr_grant_ready[cpu] > 0)
        {
            finish_grant_choose[cpu] = 1;
            return XDP_DROP;
        }
        else
            return XDP_ABORTED;
    }

    ri = bpf_map_lookup_elem(&per_cpu_remove_info, ZERO_KEY);
    if (!ri)
    {
        // this should never happen
        release_grantable_lock();
        return XDP_ABORTED;
    }

    if (remove[cpu][4])
    {
        struct rpc_state_cc *cc_node_t0 = NULL;
        struct rpc_state_cc *cc_node_t1 = NULL;
        struct bpf_rb_node *rb_node = NULL;
        peer_id = get_peerid(ri->remote_ip[4]);

        cc_node_t0 = bpf_obj_new(typeof(*cc_node_t0));
        if (!cc_node_t0)
        {
            release_grantable_lock();
            return XDP_ABORTED;
        }

        cc_node_t1 = bpf_refcount_acquire(cc_node_t0);
        if (!cc_node_t1)
        {
            bpf_obj_drop(cc_node_t0);
            release_grantable_lock();
            return XDP_ABORTED;
        }

        cc_node_t0->tree_id = 0;
        cc_node_t0->peer_id = peer_id;
        cc_node_t0->bytes_remaining = 0;
        cc_node_t0->hkey.rpcid = 0;
        cc_node_t0->hkey.local_port = 0;
        cc_node_t0->hkey.remote_port = 0;
        cc_node_t0->hkey.remote_ip = 0;
        GRANT_LOCK();
        rb_node = bpf_rbtree_lower_bound(&groot, &cc_node_t0->rbtree_link, srpt_less_rpc);
        if (rb_node)
        {
            cc_node_t0 = container_of(rb_node, struct rpc_state_cc, rbtree_link);
            if (cc_node_t0->tree_id == 0 && cc_node_t0->peer_id == peer_id && (cc_node_t0->birth & 1) == 0)
            {
                // we should add this rpc to peer tree
                cc_node_t1->tree_id = 1;
                cc_node_t1->peer_id = cc_node_t0->peer_id;
                cc_node_t1->bytes_remaining = cc_node_t0->bytes_remaining;
                cc_node_t1->incoming = cc_node_t0->incoming;
                cc_node_t1->hkey.rpcid = cc_node_t0->hkey.rpcid;
                cc_node_t1->hkey.local_port = cc_node_t0->hkey.local_port;
                cc_node_t1->hkey.remote_ip = cc_node_t0->hkey.remote_ip;
                cc_node_t1->hkey.remote_port = cc_node_t0->hkey.remote_port;
                cc_node_t1->message_length = cc_node_t0->message_length;
                // mark this rpc is in peer tree
                cc_node_t0->birth |= (__u64)1;
                cc_node_t1->birth = cc_node_t0->birth;

                bpf_rbtree_add(&groot, &cc_node_t1->rbtree_link, srpt_less_peer);
                cc_node_t1 = NULL;
            }
        }
        GRANT_UNLOCK();
        if (cc_node_t1)
            bpf_obj_drop(cc_node_t1);
    }

    bpf_tail_call(ctx, &xdp_gen_tail_call_map, XDP_GEN_COMPLETE_GRANT_6);
    // fallthrough: bpf_tail_call failed
    release_grantable_lock();
    return XDP_ABORTED;
}
SEC("xdp_gen/complete_grant_6")
int complete_grant_6_prog(struct xdp_md *ctx)
{
    __u32 cpu = bpf_get_smp_processor_id();
    if (unlikely(cpu >= MAX_CPU))
    {
        bpf_printk("ERROR: CPU Mapping, cpu=%d\n", cpu);
        return XDP_ABORTED;
    }
    __u16 peer_id = 0;
    struct remove_info *ri = NULL;

    if (nr_grant_candidate[cpu] < 6)
    {
        release_grantable_lock();
        if (nr_grant_ready[cpu] > 0)
        {
            finish_grant_choose[cpu] = 1;
            return XDP_DROP;
        }
        else
            return XDP_ABORTED;
    }

    ri = bpf_map_lookup_elem(&per_cpu_remove_info, ZERO_KEY);
    if (!ri)
    {
        // this should never happen
        release_grantable_lock();
        return XDP_ABORTED;
    }

    if (remove[cpu][5])
    {
        struct rpc_state_cc *cc_node_t0 = NULL;
        struct rpc_state_cc *cc_node_t1 = NULL;
        struct bpf_rb_node *rb_node = NULL;
        peer_id = get_peerid(ri->remote_ip[5]);

        cc_node_t0 = bpf_obj_new(typeof(*cc_node_t0));
        if (!cc_node_t0)
        {
            release_grantable_lock();
            return XDP_ABORTED;
        }

        cc_node_t1 = bpf_refcount_acquire(cc_node_t0);
        if (!cc_node_t1)
        {
            bpf_obj_drop(cc_node_t0);
            release_grantable_lock();
            return XDP_ABORTED;
        }

        cc_node_t0->tree_id = 0;
        cc_node_t0->peer_id = peer_id;
        cc_node_t0->bytes_remaining = 0;
        cc_node_t0->hkey.rpcid = 0;
        cc_node_t0->hkey.local_port = 0;
        cc_node_t0->hkey.remote_port = 0;
        cc_node_t0->hkey.remote_ip = 0;
        GRANT_LOCK();
        rb_node = bpf_rbtree_lower_bound(&groot, &cc_node_t0->rbtree_link, srpt_less_rpc);
        if (rb_node)
        {
            cc_node_t0 = container_of(rb_node, struct rpc_state_cc, rbtree_link);
            if (cc_node_t0->tree_id == 0 && cc_node_t0->peer_id == peer_id && (cc_node_t0->birth & 1) == 0)
            {
                // we should add this rpc to peer tree
                cc_node_t1->tree_id = 1;
                cc_node_t1->peer_id = cc_node_t0->peer_id;
                cc_node_t1->bytes_remaining = cc_node_t0->bytes_remaining;
                cc_node_t1->incoming = cc_node_t0->incoming;
                cc_node_t1->hkey.rpcid = cc_node_t0->hkey.rpcid;
                cc_node_t1->hkey.local_port = cc_node_t0->hkey.local_port;
                cc_node_t1->hkey.remote_ip = cc_node_t0->hkey.remote_ip;
                cc_node_t1->hkey.remote_port = cc_node_t0->hkey.remote_port;
                cc_node_t1->message_length = cc_node_t0->message_length;
                // mark this rpc is in peer tree
                cc_node_t0->birth |= (__u64)1;
                cc_node_t1->birth = cc_node_t0->birth;

                bpf_rbtree_add(&groot, &cc_node_t1->rbtree_link, srpt_less_peer);
                cc_node_t1 = NULL;
            }
        }
        GRANT_UNLOCK();
        if (cc_node_t1)
            bpf_obj_drop(cc_node_t1);
    }

    bpf_tail_call(ctx, &xdp_gen_tail_call_map, XDP_GEN_COMPLETE_GRANT_7);
    // fallthrough: bpf_tail_call failed
    release_grantable_lock();
    return XDP_ABORTED;
}
SEC("xdp_gen/complete_grant_7")
int complete_grant_7_prog(struct xdp_md *ctx)
{
    __u32 cpu = bpf_get_smp_processor_id();
    if (unlikely(cpu >= MAX_CPU))
    {
        bpf_printk("ERROR: CPU Mapping, cpu=%d\n", cpu);
        return XDP_ABORTED;
    }
    __u16 peer_id = 0;
    struct remove_info *ri = NULL;

    if (nr_grant_candidate[cpu] < 7)
    {
        release_grantable_lock();
        if (nr_grant_ready[cpu] > 0)
        {
            finish_grant_choose[cpu] = 1;
            return XDP_DROP;
        }
        else
            return XDP_ABORTED;
    }

    ri = bpf_map_lookup_elem(&per_cpu_remove_info, ZERO_KEY);
    if (!ri)
    {
        // this should never happen
        release_grantable_lock();
        return XDP_ABORTED;
    }

    if (remove[cpu][6])
    {
        struct rpc_state_cc *cc_node_t0 = NULL;
        struct rpc_state_cc *cc_node_t1 = NULL;
        struct bpf_rb_node *rb_node = NULL;
        peer_id = get_peerid(ri->remote_ip[6]);

        cc_node_t0 = bpf_obj_new(typeof(*cc_node_t0));
        if (!cc_node_t0)
        {
            release_grantable_lock();
            return XDP_ABORTED;
        }

        cc_node_t1 = bpf_refcount_acquire(cc_node_t0);
        if (!cc_node_t1)
        {
            bpf_obj_drop(cc_node_t0);
            release_grantable_lock();
            return XDP_ABORTED;
        }

        cc_node_t0->tree_id = 0;
        cc_node_t0->peer_id = peer_id;
        cc_node_t0->bytes_remaining = 0;
        cc_node_t0->hkey.rpcid = 0;
        cc_node_t0->hkey.local_port = 0;
        cc_node_t0->hkey.remote_port = 0;
        cc_node_t0->hkey.remote_ip = 0;
        GRANT_LOCK();
        rb_node = bpf_rbtree_lower_bound(&groot, &cc_node_t0->rbtree_link, srpt_less_rpc);
        if (rb_node)
        {
            cc_node_t0 = container_of(rb_node, struct rpc_state_cc, rbtree_link);
            if (cc_node_t0->tree_id == 0 && cc_node_t0->peer_id == peer_id && (cc_node_t0->birth & 1) == 0)
            {
                // we should add this rpc to peer tree
                cc_node_t1->tree_id = 1;
                cc_node_t1->peer_id = cc_node_t0->peer_id;
                cc_node_t1->bytes_remaining = cc_node_t0->bytes_remaining;
                cc_node_t1->incoming = cc_node_t0->incoming;
                cc_node_t1->hkey.rpcid = cc_node_t0->hkey.rpcid;
                cc_node_t1->hkey.local_port = cc_node_t0->hkey.local_port;
                cc_node_t1->hkey.remote_ip = cc_node_t0->hkey.remote_ip;
                cc_node_t1->hkey.remote_port = cc_node_t0->hkey.remote_port;
                cc_node_t1->message_length = cc_node_t0->message_length;
                // mark this rpc is in peer tree
                cc_node_t0->birth |= (__u64)1;
                cc_node_t1->birth = cc_node_t0->birth;

                bpf_rbtree_add(&groot, &cc_node_t1->rbtree_link, srpt_less_peer);
                cc_node_t1 = NULL;
            }
        }
        GRANT_UNLOCK();
        if (cc_node_t1)
            bpf_obj_drop(cc_node_t1);
    }

    bpf_tail_call(ctx, &xdp_gen_tail_call_map, XDP_GEN_COMPLETE_GRANT_8);
    // fallthrough: bpf_tail_call failed
    release_grantable_lock();
    return XDP_ABORTED;
}
SEC("xdp_gen/complete_grant_8")
int complete_grant_8_prog(struct xdp_md *ctx)
{
    __u32 cpu = bpf_get_smp_processor_id();
    if (unlikely(cpu >= MAX_CPU))
    {
        bpf_printk("ERROR: CPU Mapping, cpu=%d\n", cpu);
        return XDP_ABORTED;
    }
    __u16 peer_id = 0;
    struct remove_info *ri = NULL;

    if (nr_grant_candidate[cpu] < 8)
    {
        release_grantable_lock();
        if (nr_grant_ready[cpu] > 0)
        {
            finish_grant_choose[cpu] = 1;
            return XDP_DROP;
        }
        else
            return XDP_ABORTED;
    }

    ri = bpf_map_lookup_elem(&per_cpu_remove_info, ZERO_KEY);
    if (!ri)
    {
        // this should never happen
        release_grantable_lock();
        return XDP_ABORTED;
    }

    if (remove[cpu][7])
    {
        struct rpc_state_cc *cc_node_t0 = NULL;
        struct rpc_state_cc *cc_node_t1 = NULL;
        struct bpf_rb_node *rb_node = NULL;
        peer_id = get_peerid(ri->remote_ip[7]);

        cc_node_t0 = bpf_obj_new(typeof(*cc_node_t0));
        if (!cc_node_t0)
        {
            release_grantable_lock();
            return XDP_ABORTED;
        }

        cc_node_t1 = bpf_refcount_acquire(cc_node_t0);
        if (!cc_node_t1)
        {
            bpf_obj_drop(cc_node_t0);
            release_grantable_lock();
            return XDP_ABORTED;
        }

        cc_node_t0->tree_id = 0;
        cc_node_t0->peer_id = peer_id;
        cc_node_t0->bytes_remaining = 0;
        cc_node_t0->hkey.rpcid = 0;
        cc_node_t0->hkey.local_port = 0;
        cc_node_t0->hkey.remote_port = 0;
        cc_node_t0->hkey.remote_ip = 0;
        GRANT_LOCK();
        rb_node = bpf_rbtree_lower_bound(&groot, &cc_node_t0->rbtree_link, srpt_less_rpc);
        if (rb_node)
        {
            cc_node_t0 = container_of(rb_node, struct rpc_state_cc, rbtree_link);
            if (cc_node_t0->tree_id == 0 && cc_node_t0->peer_id == peer_id && (cc_node_t0->birth & 1) == 0)
            {
                // we should add this rpc to peer tree
                cc_node_t1->tree_id = 1;
                cc_node_t1->peer_id = cc_node_t0->peer_id;
                cc_node_t1->bytes_remaining = cc_node_t0->bytes_remaining;
                cc_node_t1->incoming = cc_node_t0->incoming;
                cc_node_t1->hkey.rpcid = cc_node_t0->hkey.rpcid;
                cc_node_t1->hkey.local_port = cc_node_t0->hkey.local_port;
                cc_node_t1->hkey.remote_ip = cc_node_t0->hkey.remote_ip;
                cc_node_t1->hkey.remote_port = cc_node_t0->hkey.remote_port;
                cc_node_t1->message_length = cc_node_t0->message_length;
                // mark this rpc is in peer tree
                cc_node_t0->birth |= (__u64)1;
                cc_node_t1->birth = cc_node_t0->birth;

                bpf_rbtree_add(&groot, &cc_node_t1->rbtree_link, srpt_less_peer);
                cc_node_t1 = NULL;
            }
        }
        GRANT_UNLOCK();
        if (cc_node_t1)
            bpf_obj_drop(cc_node_t1);
    }

    release_grantable_lock();
    if (nr_grant_ready[cpu] > 0)
    {
        finish_grant_choose[cpu] = 1;
        return XDP_DROP;
    }
    else
        return XDP_ABORTED;
}