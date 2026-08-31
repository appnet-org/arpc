/**
 * @file ebpf_lb.h
 * @brief Load balancing utilities for eBPF programs
 */

#pragma once

#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>

#include <intf/intf_ebpf.h>

struct {
    __uint(type, BPF_MAP_TYPE_CPUMAP);
    __type(key, __u32);
    __type(value, struct bpf_cpumap_val);
    __uint(max_entries, MAX_CPU);
} cpumap SEC(".maps");

enum {
    LB_POLICY_1 = 1,
    LB_POLICY_2 = 2,
    LB_POLICY_3 = 4
};

#define LB_POLICY LB_POLICY_1

#define LB_THRESHOLD 24
SEC(".bss.lb_threshold") __u64 lb_threshold[MAXMAX_CPU] = {0};
SEC(".bss.lb_cache_choice") __u64 lb_cache_choice[MAXMAX_CPU] = {0};

#define CORES_TO_CHECK 4

struct core_info_t {
    __u64 last_active;
    __u64 last_gro;
    __u64 softirq_backlog;
    unsigned int softirq_offset;
} __attribute__((__aligned__(CACHE_LINE_SIZE)));

/* CPU core load information */
SEC(".bss.core_info") struct core_info_t core_info[MAXMAX_CPU];
SEC(".data.gro_busy_usecs") __u64 gro_busy_usecs = 10;

static __always_inline void set_current_active(unsigned int this_cpu)
{
    this_cpu %= MAX_CPU;
    core_info[this_cpu].last_active = bpf_ktime_get_ns();
}

static __always_inline unsigned int choose_core(unsigned int this_cpu)
{
    unsigned int cpu = this_cpu;
    __u64 now = bpf_ktime_get_ns();
    unsigned int candidate;
    int i;
    if (LB_POLICY == LB_POLICY_1)
    {
        for (i = 0; i < CORES_TO_CHECK; i++)
        {
            // make verifier happy
            candidate = (cpu + i);
            if (candidate >= MAX_CPU) {
                candidate -= MAX_CPU;
            }
            candidate &= 31;

            if (core_info[candidate].softirq_backlog > 0)
            {
                // bpf_printk("candidate#%d has softirq backlog\n", candidate);
                continue;
            }

            if (core_info[candidate].last_gro + gro_busy_usecs > now)
            {
                // bpf_printk("candidate#%d is busy at GRO\n", candidate);
                continue;
            }

            break;
        }
        if (i == CORES_TO_CHECK)
        {
            unsigned int offset = core_info[this_cpu].softirq_offset + 1;
            if (offset >= CORES_TO_CHECK)
                offset = 0;
            core_info[this_cpu].softirq_offset = offset;
            // make verifier happy
            candidate = (cpu + offset);
            if (candidate >= MAX_CPU) {
                candidate -= MAX_CPU;
            }
            candidate &= 31;
        }
        __sync_fetch_and_add(&core_info[candidate].softirq_backlog, 1);
        core_info[candidate].last_gro = now;
        return candidate;
    }
    else if (LB_POLICY == LB_POLICY_2)
    {
    }
    else if (LB_POLICY == LB_POLICY_3)
    {
    }

    return cpu  % MAX_CPU;
}
