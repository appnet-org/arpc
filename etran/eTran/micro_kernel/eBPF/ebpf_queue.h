#pragma once

#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>

#include "ebpf_fib.h"

struct bpf_map {
    enum bpf_map_type map_type;
    __u32 key_size;
    __u32 value_size;
    __u32 max_entries;
    __u32 id;
} __attribute__((preserve_access_index));

struct xdp_frame {
    void *data;
    __u16 len;
    __u16 headroom;
    __u32 metasize;
    __u32 frame_sz;
    __u32 flags;
    int tx_ifindex;
} __attribute__((preserve_access_index));

/* Operations on BPF_MAP_TYPE_PKT_QUEUE */
extern int pkt_queue_enqueue(struct bpf_map *map, struct xdp_frame *xdp, __u64 index) __ksym;
extern struct xdp_frame *pkt_queue_dequeue(struct bpf_map *map, __u64 flags, __u64 *rank) __ksym;
extern bool pkt_queue_empty(struct bpf_map *map, __u64 flags, __u64 *rank) __ksym;

/* Operations on frame stored in BPF_MAP_TYPE_PKT_QUEUE */
extern int bpf_dynptr_from_xdp_frame(struct xdp_frame *xdp, __u64 flags, struct bpf_dynptr *ptr__uninit) __ksym;
extern void *bpf_dynptr_slice(const struct bpf_dynptr *ptr, __u32 offset, void *buffer, __u32 buffer__szk) __ksym;
extern void *bpf_dynptr_slice_rdwr(const struct bpf_dynptr *ptr, __u32 offset, void *buffer, __u32 buffer__szk) __ksym;
extern int bpf_packet_drop(struct xdp_frame *pkt) __ksym;
extern int bpf_packet_send(struct xdp_frame *pkt, int ifindex, __u64 flags) __ksym;
extern int bpf_packet_flush(void) __ksym;