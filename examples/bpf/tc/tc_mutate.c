#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <linux/types.h>
#include <linux/pkt_cls.h>
#include <linux/in.h>

#define MAX_PAYLOAD_LEN 64
#define PAYLOAD_SIZE 128

typedef __u32 u32;
typedef __u16 u16;
typedef __u8 u8;

BPF_PERF_OUTPUT(events);

struct data_t {
    u32 saddr;
    u32 daddr;
    u16 sport;
    u16 dport;
    u8 protocol;
    u8 payload[MAX_PAYLOAD_LEN];
    u32 payload_len;
};

// Convert "Bob" to "BOB" in-place
static __always_inline void to_uppercase(char *s, int len) {
    bpf_trace_printk("to_uppercase\n");
#pragma unroll
    for (int i = 0; i < MAX_PAYLOAD_LEN - 3; i++) {
        // Look for "Bob"
        if (s[i] == 'B' && s[i+1] == 'o' && s[i+2] == 'b') {
            s[i+1] -= 0x20; // 'o' -> 'O'
            s[i+2] -= 0x20; // 'b' -> 'B'
            bpf_trace_printk("Modified Bob to BOB at offset %d\n", i);
            break;
        }
    }
}


static __always_inline int handle_packet(struct __sk_buff *skb) {
    void *data = (void *)(long)skb->data;
    void *data_end = (void *)(long)skb->data_end;

    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return TC_ACT_OK;

    if (eth->h_proto != htons(ETH_P_IP))
        return TC_ACT_OK;

    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end)
        return TC_ACT_OK;

    u16 ip_hdr_len = ip->ihl * 4;
    struct udphdr *udp = (void *)ip + ip_hdr_len;
    if ((void *)(udp + 1) > data_end)
        return TC_ACT_OK;

    struct data_t new_data = {};
    new_data.saddr = ip->saddr;
    new_data.daddr = ip->daddr;
    new_data.protocol = ip->protocol;

    new_data.sport = ntohs(udp->source);
    new_data.dport = ntohs(udp->dest);

    if (new_data.sport != 9000 && new_data.dport != 9000)
        return TC_ACT_OK;

    u16 udp_hdr_len = sizeof(*udp);
    u16 payload_offset = sizeof(*eth) + ip_hdr_len + udp_hdr_len;

    if (skb->len < payload_offset + 11)
        return TC_ACT_OK;

    char buf[MAX_PAYLOAD_LEN] = {};
    if (bpf_skb_load_bytes(skb, payload_offset, buf, MAX_PAYLOAD_LEN) < 0)
        return TC_ACT_OK;

    // Try to mutate in place
    to_uppercase(buf, MAX_PAYLOAD_LEN);
    bpf_skb_store_bytes(skb, payload_offset, buf, MAX_PAYLOAD_LEN, 0);

    // Optionally emit to userspace
    __builtin_memcpy(new_data.payload, buf, MAX_PAYLOAD_LEN);
    new_data.payload_len = MAX_PAYLOAD_LEN;
    events.perf_submit(skb, &new_data, sizeof(new_data));

    return TC_ACT_OK;
}

// Ingress hook: process incoming packets
int tc_ingress(struct __sk_buff *skb) {
    return handle_packet(skb);
}

// Egress hook: process outgoing packets
int tc_egress(struct __sk_buff *skb) {
    return handle_packet(skb);
}
