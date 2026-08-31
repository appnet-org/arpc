// SPDX-License-Identifier: GPL-2.0

#include <linux/bpf.h>
#include <linux/types.h>
#include <time.h>
#include <bpf/bpf_helpers.h>

#include "xdp/parsing_helpers.h"

#include <intf/intf_ebpf.h>
#include "../ebpf_fib.h"
#include "../ebpf_kfunc.h"
#include "../ebpf_queue.h"
#include "../ebpf_utils.h"
#include "eTran_defs.h"
#include "tcp.h"

char LICENSE[] SEC("license") = "GPL";

#define XDP_EGRESS_DROP XDP_DROP

// key: ctx->rx_queue_index
// value: struct slow_path_info
struct
{
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, MAX_NIC_QUEUES);
    __uint(key_size, sizeof(__u32));
    __uint(value_size, sizeof(struct slow_path_info));
} slow_path_map SEC(".maps");

struct
{
    __uint(type, BPF_MAP_TYPE_XSKMAP);
    __uint(max_entries, MAX_XSK_FD);
    __uint(key_size, sizeof(int));
    __uint(value_size, sizeof(int));
} xsks_map SEC(".maps");

SEC("xdp_gen")
int xdp_gen_prog(struct xdp_md *ctx)
{
    struct ethhdr *eth;
    struct iphdr *iph;
    struct tcphdr *tcph;
    struct tcp_timestamp_opt *ts_opt;
    struct bpf_tcp_ack *ack;
    void *data, *data_end;
    int err = 0;

    __u32 cpu = bpf_get_smp_processor_id();

    if (unlikely(cpu >= MAX_CPU)) {
        xdp_gen_log_panic("cpu >= MAX_CPU");
        return XDP_ABORTED;
    }

    // reset rx_cached_ts
    rx_cached_ts[cpu] = 0;

    if (prev_conn[cpu] != NULL_CONN) {
        if (enqueue_prev_ack(cpu)) {
            xdp_gen_log_panic("enqueue_prev_ack failed");
        }
        prev_conn[cpu] = NULL_CONN;
    }

    xdp_gen_log("XDP_GEN at CPU#%u", cpu);

    if (unlikely(err = bpf_xdp_adjust_tail(ctx, -TCP_ACK_HEADER_CUTOFF))) {
        xdp_gen_log_panic("bpf_xdp_adjust_tail failed: %d", err);
        return XDP_ABORTED;
    }

    data = (void *)(long)ctx->data;
    data_end = (void *)(long)ctx->data_end;

    eth = (struct ethhdr *)data;
    if (unlikely(eth + 1 > data_end)) {
        xdp_gen_log_panic("eth + 1 > data_end");
        return XDP_ABORTED;
    }
    iph = (struct iphdr *)(eth + 1);
    if (unlikely(iph + 1 > data_end)) {
        xdp_gen_log_panic("iph + 1 > data_end");
        return XDP_ABORTED;
    }
    tcph = (struct tcphdr *)(iph + 1);
    if (unlikely(tcph + 1 > data_end)) {
        xdp_gen_log_panic("tcph + 1 > data_end");
        return XDP_ABORTED;
    }
    ts_opt = (struct tcp_timestamp_opt *)(tcph + 1);
    if (unlikely(ts_opt + 1 > data_end)) {
        xdp_gen_log_panic("ts_opt + 1 > data_end");
        return XDP_ABORTED;
    }

    if (ackqueue_empty()) {
        xdp_gen_log("ackqueue is empty");
        return XDP_ABORTED;
    }

    ack = dequeue_ack();
    if (unlikely(!ack)) {
        xdp_gen_log_panic("dequeue_ack failed");
        return XDP_ABORTED;
    }

    ts_opt->kind = TCPI_OPT_TIMESTAMPS;
    ts_opt->length = sizeof(*ts_opt) / 4;
    ts_opt->ts_val = bpf_htonl(ack->ts_val);
    ts_opt->ts_ecr = bpf_htonl(ack->ts_ecr);
    
    #ifdef XDP_GEN_DEBUG
    __u32 now = (__u32)bpf_ktime_get_ns();
    xdp_gen_log("ACK delay: %u ns", (__u32)bpf_ktime_get_ns() - ack->ts_val);
    #endif
    
    tcph->source = bpf_htons(ack->local_port);
    tcph->dest = bpf_htons(ack->remote_port);
    tcph->seq = bpf_htonl(ack->seq);
    tcph->ack_seq = bpf_htonl(ack->ack);
    tcph->doff = 5 + TS_OPT_SIZE / 4;
    tcph->res1 = 0;
    tcph->fin = 0;
    tcph->syn = 0;
    tcph->rst = 0;
    tcph->psh = 0;
    tcph->ack = 1;
    tcph->urg = 0;
    tcph->ece = ack->ecn_flags;
    tcph->cwr = 0;
    tcph->window = bpf_htons(ack->rxwnd);
    tcph->urg_ptr = 0;
    tcph->check = 0;

    iph->saddr = bpf_htonl(ack->local_ip);
    iph->daddr = bpf_htonl(ack->remote_ip);
    fill_ip_hdr(iph, 0, false);

    xdp_gen_log("send ack packet, seq(%u), ack_seq(%u), rxwnd(%u)", ack->seq, ack->ack, ack->rxwnd);

    return xmit_packet_fib_lookup(ctx, eth, iph);
}

SEC("xdp_egress")
int xdp_egress_prog(struct xdp_md *ctx)
{
    int ret = 0;
    void *data, *data_end;
    struct ethhdr *eth;
    struct iphdr *iph;
    struct tcphdr *tcph;
    struct bpf_tcp_conn *c;
    struct ebpf_flow_tuple key;
    struct meta_info *data_meta;

    if (unlikely(ret = bpf_xdp_adjust_meta(ctx, -(int)sizeof(*data_meta)))) {
        xdp_egress_log_panic("xdp_adjust_meta failed: %d", ret);
        return XDP_EGRESS_DROP;
    }

    data = (void *)(long)ctx->data;
    data_end = (void *)(long)ctx->data_end;
    data_meta = (struct meta_info *)(long)ctx->data_meta;

    if (unlikely(data_meta + 1 > data)) {
        xdp_egress_log_panic("data_meta + 1 > data_end");
        return XDP_EGRESS_DROP;
    }

    // Ethernet header length check
    eth = (struct ethhdr *)data;
    if (unlikely(eth + 1 > data_end)) {
        xdp_egress_log_err("eth + 1 > data_end");
        return XDP_EGRESS_DROP;
    }
    // IP header length check
    iph = (struct iphdr *)(eth + 1);
    if (unlikely(iph + 1 > data_end)) {
        xdp_egress_log_err("iph + 1 > data_end");
        goto err_pkt;
    }
    // TODO: use another way to distinguish packets from slowpath, e.g., a encrpyted key
    if (unlikely(data_meta->tx.slowpath && !(data_meta->tx.flag & FLAG_TO))) {
        xdp_egress_log("slowpath packet");
        return xmit_packet_fib_lookup(ctx, eth, iph);
    }

    // TCP header length check
    tcph = (struct tcphdr *)(iph + 1);
    if (unlikely(tcph + 1 > data_end)) {
        xdp_egress_log_err("tcph + 1 > data_end");
        goto err_pkt;
    }

    key.local_ip = bpf_ntohl(iph->saddr);
    key.remote_ip = bpf_ntohl(iph->daddr);
    key.local_port = bpf_ntohs(tcph->source);
    key.remote_port = bpf_ntohs(tcph->dest);
    
    c = bpf_map_lookup_elem(&bpf_tcp_conn_map, &key);
    if (unlikely(!c)) {
        xdp_egress_log_err("bpf_tcp_conn not found");
        goto err_pkt;
    }

    // // qid check
    // if (unlikely(c->qid != ctx->rx_queue_index)) {
    //     xdp_egress_log_err("ctx->rx_queue_index(%u) != c->qid(%u)", ctx->rx_queue_index, c->qid);
    //     goto err_pkt;
    // }
    
    // address and port check
    if (unlikely(c->local_ip != key.local_ip || c->remote_ip != key.remote_ip ||
                 c->local_port != key.local_port || c->remote_port != key.remote_port)) {
        xdp_egress_log_err("address and port check failed");
        goto err_pkt;
    }

    ret = tcp_tx_process(iph, tcph, c, data_meta, data_end);

    if (ret == XDP_DROP) {
        if (data_meta->tx.flag) {
            return XDP_EGRESS_DROP;
        }
        xdp_egress_log_err("TCP thinks the packet is invalid");
        goto err_pkt;
    }
    
    eth->h_proto = bpf_htons(ETH_P_IP);
    __builtin_memcpy(eth->h_dest, c->remote_mac, ETH_ALEN);
    __builtin_memcpy(eth->h_source, c->local_mac, ETH_ALEN);

    if (ret == XDP_REDIRECT) {
        kick_tw();
        return ret;
    } else if (ret == XDP_PASS) {
        goto redirect;
    }

    #ifdef XDP_EGRESS_DEBUG
    bpf_printk("");
    xdp_egress_dump_eth(eth);
    xdp_egress_dump_ip(iph);
    xdp_egress_dump_tcp(tcph);
    #endif
    return XDP_TX;

redirect:
    // make verifier happy
    if (unlikely(ctx->rx_queue_index >= MAX_NIC_QUEUES)) {
        xdp_egress_log_err("qid >= MAX_NIC_QUEUES");
        return XDP_EGRESS_DROP;
    }
    // FIXME: fail to redirect?
    return bpf_redirect_map(&xsks_map, c->qid2xsk[ctx->rx_queue_index], XDP_DROP);

err_pkt:
    xdp_egress_log_err("XDP_EGRESS drops packet");
    return XDP_EGRESS_DROP;
}

SEC("xdp_sock")
int xdp_sock_prog(struct xdp_md *ctx)
{
    int ret = 0;
    void *data, *data_end;
    struct meta_info *data_meta;
    struct hdr_cursor nh = {0};
    struct ethhdr *eth;
    struct iphdr *iph;
    struct tcphdr *tcph;
    int proto_type;
    int tcp_length;
    struct slow_path_info *sp;
    struct bpf_tcp_conn *c;
    struct ebpf_flow_tuple key;
    
    __u32 qid = ctx->rx_queue_index;
    __u32 pkt_len = ctx->data_end - ctx->data;
    __u32 cpu = bpf_get_smp_processor_id();

    // make verifier happy
    if (unlikely(cpu >= MAX_CPU)) {
        xdp_log_panic("cpu >= MAX_CPU");
        return XDP_DROP;
    }

    xdp_log("XDP receive pkt at Queue#%u", ctx->rx_queue_index);

    if (unlikely(ret = bpf_xdp_adjust_meta(ctx, -(int)sizeof(*data_meta)))) {
        xdp_log_err("xdp_adjust_meta failed: %d", ret);
        return XDP_DROP;
    }

    data = (void *)(long)ctx->data;
    data_end = (void *)(long)ctx->data_end;
    data_meta = (struct meta_info *)(long)ctx->data_meta;
    nh.pos = data;

    if (unlikely(data_meta + 1 > data)) {
        xdp_log_err("data_meta + 1 > data_end");
        return XDP_DROP;
    }

    // Initialize 32 bytes of memory to all bits set to 1
    __builtin_memset(data_meta, 0xFF, 32);
    data_meta->rx.qid = qid;
    
    // Ethernet header
    proto_type = parse_ethhdr(&nh, data_end, &eth);
    if (unlikely(proto_type != bpf_htons(ETH_P_IP))) {
        xdp_log_err("proto_type != ETH_P_IP");
        return XDP_DROP;
    }

    // IP header
    proto_type = parse_iphdr(&nh, data_end, &iph);
    if (unlikely(proto_type != IPPROTO_TCP)) {
        xdp_log_err("proto_type != IPPROTO_TCP");
        return XDP_PASS;
    }

    // TCP header
    tcp_length = parse_tcphdr(&nh, data_end, &tcph);
    if (unlikely(tcp_length < 0)) {
        xdp_log_err("tcp_length < 0");
        return XDP_DROP;
    }

    // filter out SYN, SYN-ACK, RST packets
    if (unlikely(is_tcp_syn(tcph) || is_tcp_syn_ack(tcph) || is_tcp_rst(tcph))) {
        goto slowpath;
    }

    struct tcp_timestamp_opt *ts_opt = (struct tcp_timestamp_opt *)(tcph + 1);
    if (unlikely(ts_opt + 1 > data_end)) {
        xdp_log_err("ts_opt + 1 > data_end");
        return XDP_DROP;
    }

    key.local_ip = bpf_ntohl(iph->daddr);
    key.remote_ip = bpf_ntohl(iph->saddr);
    key.local_port = bpf_ntohs(tcph->dest);
    key.remote_port = bpf_ntohs(tcph->source);

    c = bpf_map_lookup_elem(&bpf_tcp_conn_map, &key);
    if (unlikely(!c)) {
        xdp_log_err("bpf_tcp_conn not found");
        return XDP_DROP;
    }

    if (prev_conn[cpu] != NULL_CONN && prev_conn[cpu] != c->cc_idx) {
        if (enqueue_prev_ack(cpu)) {
            xdp_log_panic("enqueue_prev_ack failed");
        }
        prev_conn[cpu] = NULL_CONN;
    }

    data_meta->rx.conn = c->opaque_connection;

    ret = tcp_rx_process(tcph, c, pkt_len, data_meta, (iph->tos & IPTOS_ECN_CE) == IPTOS_ECN_CE, cpu);
    
    if (likely(ret == XDP_REDIRECT && qid < MAX_NIC_QUEUES)) {
        return bpf_redirect_map(&xsks_map, c->qid2xsk[qid], XDP_DROP);
    }

    return XDP_DROP;

slowpath:
    sp = bpf_map_lookup_elem(&slow_path_map, &qid);
    if (unlikely(!sp || !sp->active)) {
        xdp_log_err("ERROR: slow_path_info not found or inactive");
        return XDP_DROP;
    }
    return bpf_redirect_map(&xsks_map, sp->sp_xsk_map_key, XDP_DROP);
}

SEC("xdp/cpumap")
int xdp_cpumap_prog(struct xdp_md *ctx)
{
    return XDP_DROP;
}