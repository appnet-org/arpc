#include <linux/bpf.h>
#include <linux/types.h>
#include <bpf/bpf_helpers.h>
#include <xdp/parsing_helpers.h>

#include <tran_def/homa.h>
#include <tran_def/tcp.h>

#include "ebpf_utils.h"

#define MAX_TRANSPORTS 16
#define MAX_UMEM_ID 16

#define TCP_TRAN_XDP_IDX 0
#define HOMA_TRAN_XDP_IDX 1

/* XDP and CPUMAP selects the transport based on packet header while 
 * XDP_GEN and XDP_EGRESS selects the transport based on the umem_id
*/
struct {
    __uint(type, BPF_MAP_TYPE_PROG_ARRAY);
    __uint(max_entries, MAX_TRANSPORTS);
    __uint(key_size, sizeof(int));
    __uint(value_size, sizeof(int));
} tran_xdp_map SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, MAX_UMEM_ID);
    __uint(key_size, sizeof(int));
    __uint(value_size, sizeof(int));
} umem_id_tran_map SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_PROG_ARRAY);
    __uint(max_entries, MAX_TRANSPORTS);
    __uint(key_size, sizeof(int));
    __uint(value_size, sizeof(int));
} tran_xdp_gen_map SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_PROG_ARRAY);
    __uint(max_entries, MAX_TRANSPORTS);
    __uint(key_size, sizeof(int));
    __uint(value_size, sizeof(int));
} tran_xdp_egress_map SEC(".maps");

SEC("xdp_sock")
int xdp_sock_prog(struct xdp_md *ctx)
{
    struct hdr_cursor nh = {0};
    struct ethhdr *eth;
    struct iphdr *iph;
    void *data, *data_end;
    int proto_type;
    
    data = (void *)(long)ctx->data;
    data_end = (void *)(long)ctx->data_end;
    nh.pos = data;

    /* check Ethernet header */
    proto_type = parse_ethhdr(&nh, data_end, &eth);
    if (unlikely(proto_type != bpf_htons(ETH_P_IP)))
        return XDP_DROP;

    /* check IP header */
    proto_type = parse_iphdr(&nh, data_end, &iph);
    
    switch (proto_type) {
        case IPPROTO_TCP:
            bpf_tail_call(ctx, &tran_xdp_map, TCP_TRAN_XDP_IDX);
            break;
        case IPPROTO_HOMA:
            bpf_tail_call(ctx, &tran_xdp_map, HOMA_TRAN_XDP_IDX);
            break;
        default:
            return XDP_PASS;
    }
    return XDP_PASS;
}

SEC("xdp_gen")
int xdp_gen_prog(struct xdp_md *ctx)
{
    int umem_id = ctx->umem_id;
    int *tran_idx = bpf_map_lookup_elem(&umem_id_tran_map, &umem_id);
    if (unlikely(!tran_idx))
        return XDP_DROP;
    bpf_tail_call(ctx, &tran_xdp_gen_map, *tran_idx);
    return XDP_ABORTED;
}

SEC("xdp_egress")
int xdp_egress_prog(struct xdp_md *ctx)
{
    int umem_id = ctx->umem_id;
    int *tran_idx = bpf_map_lookup_elem(&umem_id_tran_map, &umem_id);
    if (unlikely(!tran_idx))
        return XDP_DROP;
    bpf_tail_call(ctx, &tran_xdp_egress_map, *tran_idx);
    return XDP_DROP;
}

SEC("xdp/cpumap")
int xdp_cpumap_prog(struct xdp_md *ctx)
{
    return XDP_DROP;
}