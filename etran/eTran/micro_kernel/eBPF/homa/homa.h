#pragma once

#include <linux/bpf.h>
#include <linux/types.h>
#include <bpf/bpf_helpers.h>
#include <xdp/parsing_helpers.h>

#include <tran_def/homa.h>

#define HOMA_GRANT_HEADER_CUTOFF                                                                                       \
    (int)(XDP_GEN_PKT_SIZE - sizeof(struct grant_header) - sizeof(struct ethhdr) - sizeof(struct iphdr))

struct {
    __uint(type, BPF_MAP_TYPE_QUEUE);
    __type(value, __u64);
    __uint(max_entries, MAX_BUCKET_SIZE);
} avail_pacing_idx SEC(".maps");

static __always_inline __u64 allocate_qid(void)
{
    __u64 qid = MAX_BUCKET_SIZE;
    long ret = bpf_map_pop_elem(&avail_pacing_idx, &qid);
    if (ret)
    {
        log_err("bpf_map_pop_elem for avail_pacing_idx failed.");
        return MAX_BUCKET_SIZE;
    }
    // bpf_printk("allocate qid: %lu, %d", qid, ret);
    return qid;
}

static __always_inline void free_qid(__u64 qid)
{
    long ret = bpf_map_push_elem(&avail_pacing_idx, &qid, BPF_ANY);
    if (ret)
    {
        log_err("bpf_map_push_elem for avail_pacing_idx failed.");
    }
    // bpf_printk("return qid: %lu", qid);
}

// FIXME: peerid should be unique
static inline __u16 get_peerid(__u16 remote_ip)
{
    return remote_ip & (MAX_PEER - 1);
}

static __always_inline __u8 get_prio(__u32 message_length)
{
    __u8 idx = 0;
    __u8 w = 0;
    for (__u8 i = HOMA_MAX_PRIORITY - 1; i >= 0; i--)
    {
        idx &= (HOMA_MAX_PRIORITY - 1); // make verfier happy
        w = workload_type - 1;
        w &= (HOMA_MAX_PRIORITY - 1); // make verfier happy
        if (workload_cutoff[w][idx] >= message_length)
        {
            return i;
        }
    }
    return 0;
}

static __always_inline void set_prio(struct iphdr *iph, __u8 prio)
{
    iph->tos = prio << 5;
}

static __always_inline int homa_parse_common_hdr(struct hdr_cursor *nh, void *data_end,
                                                 struct common_header **homa_common_hdr)
{
    struct common_header *homa_common_h = nh->pos;

    if (homa_common_h + 1 > data_end)
        return -1;

    *homa_common_hdr = homa_common_h;

    return homa_common_h->type;
}

/**
 * @brief This function returns if the packet is a single-packet message.
 *
 * @param nh
 * @param data_end
 * @param homa_data_hdr
 * @return __always_inline 0: multi-packet, 1: single packet
 */
static __always_inline int homa_parse_data_hdr(struct hdr_cursor *nh, void *data_end,
                                               struct data_header **homa_data_hdr)
{
    struct data_header *homa_data_h = nh->pos;

    if (homa_data_h + 1 > data_end)
        return -1;

    nh->pos = homa_data_h + 1;
    *homa_data_hdr = homa_data_h;

    return bpf_ntohl(homa_data_h->message_length) <= HOMA_MSS;
}

static __always_inline int homa_parse_resend_hdr(struct hdr_cursor *nh, void *data_end,
                                                 struct resend_header **homa_resend_hdr)
{
    struct resend_header *homa_resend_h = nh->pos;

    if (homa_resend_h + 1 > data_end)
        return -1;

    nh->pos = homa_resend_h + 1;
    *homa_resend_hdr = homa_resend_h;

    return 0;
}

static __always_inline int homa_parse_grant_hdr(struct hdr_cursor *nh, void *data_end,
                                                struct grant_header **homa_grant_hdr)
{
    struct grant_header *homa_grant_h = nh->pos;

    if (homa_grant_h + 1 > data_end)
        return -1;

    nh->pos = homa_grant_h + 1;
    *homa_grant_hdr = homa_grant_h;

    return 0;
}

static __always_inline int homa_parse_unknown_hdr(struct hdr_cursor *nh, void *data_end,
                                                  struct unknown_header **homa_unknown_hdr)
{
    struct unknown_header *homa_unknown_h = nh->pos;

    if (homa_unknown_h + 1 > data_end)
        return -1;

    nh->pos = homa_unknown_h + 1;
    *homa_unknown_hdr = homa_unknown_h;

    return 0;
}

static __always_inline int homa_parse_busy_hdr(struct hdr_cursor *nh, void *data_end,
                                               struct busy_header **homa_busy_hdr)
{
    struct busy_header *homa_busy_h = nh->pos;

    if (homa_busy_h + 1 > data_end)
        return -1;

    nh->pos = homa_busy_h + 1;
    *homa_busy_hdr = homa_busy_h;

    return 0;
}