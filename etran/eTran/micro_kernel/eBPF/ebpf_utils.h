#pragma once

#include <linux/if_ether.h>
#include <linux/types.h>
#include <linux/ip.h>

#define CHECK_AND_DROP_LOG(condition, message) \
    do { \
        if (unlikely(condition)) { \
            log_err(message); \
            return XDP_DROP; \
        } \
    } while (0)

#define CHECK_AND_DROP(condition) \
    do { \
        if (unlikely(condition)) { \
            return XDP_DROP; \
        } \
    } while (0)

#define CHECK_AND_PASS(condition) \
    do { \
        if (unlikely(condition)) { \
            return XDP_PASS; \
        } \
    } while (0)

#ifdef XDP_DEBUG
#define xdp_log(fmt, ...) bpf_printk("XDP: " fmt, ##__VA_ARGS__)
#define xdp_log_err(fmt, ...) bpf_printk("XDP ERROR: " fmt, ##__VA_ARGS__)
#define xdp_log_panic(fmt, ...) bpf_printk("XDP PANIC: " fmt, ##__VA_ARGS__)
#else
#define xdp_log(fmt, ...)
#define xdp_log_err(fmt, ...)
#define xdp_log_panic(fmt, ...)
#endif

#ifdef XDP_EGRESS_DEBUG
#define xdp_egress_log(fmt, ...) bpf_printk("XDP_EGRESS: " fmt, ##__VA_ARGS__)
#define xdp_egress_log_err(fmt, ...) bpf_printk("XDP_EGRESS ERROR: " fmt, ##__VA_ARGS__)
#define xdp_egress_log_panic(fmt, ...) bpf_printk("XDP_EGRESS PANIC: " fmt, ##__VA_ARGS__)
#else
#define xdp_egress_log(fmt, ...)
#define xdp_egress_log_err(fmt, ...)
#define xdp_egress_log_panic(fmt, ...)
#endif

#ifdef XDP_GEN_DEBUG
#define xdp_gen_log(fmt, ...) bpf_printk("XDP_GEN: " fmt, ##__VA_ARGS__)
#define xdp_gen_log_err(fmt, ...) bpf_printk("XDP_GEN ERROR: " fmt, ##__VA_ARGS__)
#define xdp_gen_log_panic(fmt, ...) bpf_printk("XDP_GEN PANIC: " fmt, ##__VA_ARGS__)
#else
#define xdp_gen_log(fmt, ...)
#define xdp_gen_log_err(fmt, ...)
#define xdp_gen_log_panic(fmt, ...)
#endif

#define log_panic(fmt, ...) bpf_printk("CPU#%d, PANIC: " fmt, bpf_get_smp_processor_id(), ##__VA_ARGS__)
#define log_err(fmt, ...) bpf_printk("CPU#%d, ERROR: " fmt, bpf_get_smp_processor_id(), ##__VA_ARGS__)

#define unlikely(x) __glibc_unlikely(x)
#define likely(x) __glibc_likely(x)

#define DIV_ROUND_UP(n, d) (((n) + (d) - 1) / (d))

#define ZERO_KEY (&(int){0})

#define bss_private(name) SEC(".bss." #name) __hidden __attribute__((aligned(8)))
#define bss_public(name) SEC(".bss." #name) __attribute__((aligned(8)))

#define data_private(name) SEC(".data." #name) __hidden __attribute__((aligned(8)))
#define data_public(name) SEC(".data." #name) __attribute__((aligned(8)))

#define __contains(name, node) __attribute__((btf_decl_tag("contains:" #name ":" #node)))

#define atomic_inc(ptr) __sync_fetch_and_add(ptr, 1)
#define atomic_dec(ptr) __sync_fetch_and_sub(ptr, 1)
#define atomic_read(ptr) __sync_fetch_and_add(ptr, 0)
#define atomic_add(ptr, val) __sync_fetch_and_add(ptr, val)
#define atomic_sub(ptr, val) __sync_fetch_and_sub(ptr, val)
#define atomic_cmpxchg(ptr, old, new) __sync_val_compare_and_swap(ptr, old, new)
#define atomic_xchg(ptr, new) __sync_lock_test_and_set(ptr, new)

#define min(x, y) (x < y ? x : y)
#define max(x, y) (x > y ? x : y)

// Compression function for Merkle-Damgard construction.
// This function is generated using the framework provided.
static inline __u64 fasthash_mix(__u64 h)
{
    h ^= h >> 23;
    h *= 0x2127599bf4325c37ULL;
    h ^= h >> 47;
    return h;
}

static inline __u64 fasthash64(const void *buf, __u64 len, __u64 seed)
{
    const __u64 m = 0x880355f21e6d1965ULL;
    const __u64 *pos = (const __u64 *)buf;
    const __u64 *end = pos + (len / 8);
    const unsigned char *pos2;
    __u64 h = seed ^ (len * m);
    __u64 v;

    while (pos != end)
    {
        v = *pos++;
        h ^= fasthash_mix(v);
        h *= m;
    }

    pos2 = (const unsigned char *)pos;
    v = 0;

    switch (len & 7)
    {
    case 7:
        v ^= (__u64)pos2[6] << 48;
    case 6:
        v ^= (__u64)pos2[5] << 40;
    case 5:
        v ^= (__u64)pos2[4] << 32;
    case 4:
        v ^= (__u64)pos2[3] << 24;
    case 3:
        v ^= (__u64)pos2[2] << 16;
    case 2:
        v ^= (__u64)pos2[1] << 8;
    case 1:
        v ^= (__u64)pos2[0];
        h ^= fasthash_mix(v);
        h *= m;
    }

    return fasthash_mix(h);
}

#define RPC_LOCK(rpc_slot) bpf_spin_lock(&rpc_slot->hash_lock);

#define RPC_UNLOCK(rpc_slot) bpf_spin_unlock(&rpc_slot->hash_lock);

#define GET_POINTER(cc_node, rpc_slot) cc_node = bpf_kptr_xchg(&rpc_slot->cc_node, NULL);

#define PUT_POINTER(cc_node, rpc_slot) \
do { \
    cc_node = bpf_kptr_xchg(&rpc_slot->cc_node, cc_node); \
    if (unlikely(cc_node != NULL)) { \
        bpf_obj_drop(cc_node); \
    } \
} while (0)

static __always_inline void dump_eth_addr(struct ethhdr *eth)
{
    bpf_printk("src mac = %x:%x:%x", eth->h_source[0], eth->h_source[1], eth->h_source[2]);
    bpf_printk(":%x:%x:%x", eth->h_source[3], eth->h_source[4], eth->h_source[5]);

    bpf_printk("dst mac = %x:%x:%x", eth->h_dest[0], eth->h_dest[1], eth->h_dest[2]);
    bpf_printk(":%x:%x:%x", eth->h_dest[3], eth->h_dest[4], eth->h_dest[5]);
}

static __always_inline __u16 csum_fold_helper(
    __u64 csum) {
  int i;
  for (i = 0; i < 4; i++) {
    if (csum >> 16)
      csum = (csum & 0xffff) + (csum >> 16);
  }
  return ~csum;
}

static __always_inline void ipv4_csum_inline(
    void* iph,
    __u64* csum) {
  __u16* next_iph_u16 = (__u16*)iph;
  for (int i = 0; i < sizeof(struct iphdr) >> 1; i++) {
    *csum += *next_iph_u16++;
  }
  *csum = csum_fold_helper(*csum);
}