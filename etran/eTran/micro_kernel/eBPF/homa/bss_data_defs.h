#pragma once

/* Bandwidth of the link in Mbps */
#define LINK_MBPS 25000

/* XDP_GEN packet size */
#define XDP_GEN_PKT_SIZE 128

/* Maximum number of ports, must be the power of 2 */
#define MAX_SERVER_PORT 64

/* Force update every MAX_UPDATE_DELAY_PACKETS packets */
#define MAX_UPDATE_DELAY_PACKETS 32

#define DEFAULT_MTU 1500
#define ETH_HDR_LEN 14
#define IP_HDR_LEN 20
#define HOMA_HDR_LEN 60
#define HDR_OVERHEAD (ETH_HDR_LEN + IP_HDR_LEN + HOMA_HDR_LEN)
#define HOMA_MSS (DEFAULT_MTU - IP_HDR_LEN - HOMA_HDR_LEN)

SEC(".bss.local_ip") 
__u32 local_ip;

/************************* Homa parameters *************************/
#define HOMA_MAX_PRIORITY 8
#define HOMA_MAX_SCHED_PRIO 3
#define HOMA_OVERCOMMITMENT 8
#define GRANT_FIFO_FRACTION 100 // unit: thousand
#define GRANT_FIFO_INCREMENT 10000
#define PACER_FIFO_FRACTION 50 // unit: thousand
#define PACER_FIFO_INCREMENT 1000
SEC(".data.homa_params")
__u32 Homa_min_throttled_bytes = 200;
__u32 Homa_unsched_bytes = 60000;
__u32 Homa_max_incoming = 480000;
__u32 Homa_grant_window = 100000;
__u64 ns_per_kbyte = (101 * (8 * (__u64)1000000) / LINK_MBPS) / 100;
__u64 max_nic_queue_ns = 3000;

int grant_nonfifo_left = 0;
__u64 grant_nonfifo = ((1000 * GRANT_FIFO_INCREMENT) / GRANT_FIFO_FRACTION - GRANT_FIFO_INCREMENT);

int pacer_nonfifo_left = 0;
__u64  pacer_nonfifo = ((1000 * PACER_FIFO_INCREMENT) / PACER_FIFO_FRACTION - PACER_FIFO_INCREMENT);
/************************* Homa parameters *************************/

SEC(".data.workload_type") 
__u8 workload_type = 5;
SEC(".data.workload_cutoff")
__u32 workload_cutoff[5][HOMA_MAX_PRIORITY] = {{1000000, 12288, 2112, 1280, 832, 576, 384, 192},
                                          {1000000, 1000000, 1000000, 7168, 1920, 640, 448, 320},
                                          {1000000, 1000000, 1000000, 1000000, 1000000, 63488, 12288, 3008},
                                          {1000000, 1000000, 1000000, 1000000, 1000000, 1000000, 1000000, 68608},
                                          {1000000, 1000000, 1000000, 1000000, 1000000, 1000000, 1000000, 1000000}};

SEC(".bss.homa_global_state")
/* Total incoming bytes */
__u64 __attribute__((__aligned__(CACHE_LINE_SIZE))) total_incoming;
/* Number of RPCs that are chosen to grant */
__u64 __attribute__((__aligned__(CACHE_LINE_SIZE))) nr_grant_candidate[MAXMAX_CPU];
/* Number of RPCs that are ready to grant */
int __attribute__((__aligned__(CACHE_LINE_SIZE))) nr_grant_ready[MAXMAX_CPU];
/* Lock for grantable list */
__u64 __attribute__((__aligned__(CACHE_LINE_SIZE))) grantable_lock;
/* When NIC is idle */
__u64 __attribute__((__aligned__(CACHE_LINE_SIZE))) link_idle_time;
/* Whether finish grant choose */
__u64 __attribute__((__aligned__(CACHE_LINE_SIZE))) finish_grant_choose[MAXMAX_CPU];
/* Current index of granting */
__u64 __attribute__((__aligned__(CACHE_LINE_SIZE))) granting_idx[MAXMAX_CPU];
/* Whether to grant FIFO RPCs */
__u64 __attribute__((__aligned__(CACHE_LINE_SIZE))) need_grant_fifo[MAXMAX_CPU];
/* Number of RPCs in throttle list */
__u64 __attribute__((__aligned__(CACHE_LINE_SIZE))) nr_rpc_in_throttle;
/* Whether to remove the RPC from grantable list */
__u64 __attribute__((__aligned__(CACHE_LINE_SIZE))) remove[MAXMAX_CPU][HOMA_OVERCOMMITMENT];
/* Accumulated delay update packets */
__u64 __attribute__((__aligned__(CACHE_LINE_SIZE))) update_delay_packets[MAXMAX_CPU];

/* Whether to use cached lb choice */
__u64 __attribute__((__aligned__(CACHE_LINE_SIZE))) use_cached_lb_choice[MAXMAX_CPU];

int __attribute__((__aligned__(CACHE_LINE_SIZE))) cache_has_rpc[MAXMAX_CPU];
/* Cached RPC waiting for granting */
struct rpc_key_t __attribute__((__aligned__(CACHE_LINE_SIZE))) cache_rpc[MAXMAX_CPU];

bss_public(B) struct bpf_spin_lock grant_list_lock;
bss_public(B) struct bpf_rb_root groot __contains(rpc_state_cc, rbtree_link);

bss_public(C) struct bpf_spin_lock throttle_list_lock;
bss_public(C) struct bpf_rb_root troot __contains(rpc_state_cc, rbtree_link);

#define GRANT_LOCK() bpf_spin_lock(&grant_list_lock)
#define GRANT_UNLOCK() bpf_spin_unlock(&grant_list_lock)

#define THROTTLE_LOCK() bpf_spin_lock(&throttle_list_lock)
#define THROTTLE_UNLOCK() bpf_spin_unlock(&throttle_list_lock)

// return 1 if lock is acquired, 0 otherwise
static __always_inline int try_grantable_lock(void)
{
    if (__sync_bool_compare_and_swap(&grantable_lock, (__u64)0, (__u64)1))
    {
        return 1;
    }
    return 0;
}

static __always_inline void release_grantable_lock(void)
{
    __sync_fetch_and_sub(&grantable_lock, (__u64)1);
}
