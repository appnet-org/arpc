#pragma once

#include <linux/bpf.h>
#include <linux/types.h>
#include <bpf/bpf_helpers.h>
#include <xdp/parsing_helpers.h>

#include <tran_def/homa.h>

#include "ebpf_kfunc.h"
#include "ebpf_utils.h"

#include "bss_data_defs.h"

/************** Tail call programs at XDP_GEN **************/
enum {
    XDP_GEN_CHOOSE_RPC_TO_GRANT = 0,
    XDP_GEN_COMPLETE_GRANT_1,
    XDP_GEN_COMPLETE_GRANT_2,
    XDP_GEN_COMPLETE_GRANT_3,
    XDP_GEN_COMPLETE_GRANT_4,
    XDP_GEN_COMPLETE_GRANT_5,
    XDP_GEN_COMPLETE_GRANT_6,
    XDP_GEN_COMPLETE_GRANT_7,
    XDP_GEN_COMPLETE_GRANT_8,
    XDP_GEN_MAX_TAIL_CALL
};

struct {
    __uint(type, BPF_MAP_TYPE_PROG_ARRAY);
    __uint(max_entries, XDP_GEN_MAX_TAIL_CALL);
    __uint(key_size, sizeof(int));
    __uint(value_size, sizeof(int));
} xdp_gen_tail_call_map SEC(".maps");
/************** Tail call programs at XDP_GEN **************/

// ctx->rx_queue_index --> struct slow_path_info
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, MAX_NIC_QUEUES);
    __uint(key_size, sizeof(__u32));
    __uint(value_size, sizeof(struct slow_path_info));
} slow_path_map SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_XSKMAP);
    __uint(max_entries, MAX_XSK_FD);
    __uint(key_size, sizeof(int));
    __uint(value_size, sizeof(int));
} xsks_map SEC(".maps");