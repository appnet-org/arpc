#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <linux/types.h>
#include <linux/pkt_cls.h>
#include <linux/in.h>

#define MAX_PAYLOAD_LEN 64

typedef __u32 u32;
typedef __u16 u16;
typedef __u8 u8;

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

    int i = 55;
    s[i] = 'A';
    s[i+1] = 'A';
    s[i+2] = 'A';
    // bpf_trace_printk("s: %s\n", s);
    // Check if the string is "Bob"
    // #pragma unroll
    // for (int i = 0; i < MAX_PAYLOAD_LEN - 3; i++) {
    //     // Look for "Bob"
    //     if (s[i] == 'B' && s[i+1] == 'o' && s[i+2] == 'b') {
    //         s[i+1] -= 0x20; // 'o' -> 'O'
    //         s[i+2] -= 0x20; // 'b' -> 'B'
    //         bpf_trace_printk("Modified Bob to BOB at offset %d\n", i);
    //         break;
    //     }
    // }
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

    if (ntohs(udp->dest) != 9000 && ntohs(udp->source) != 9000)
        return TC_ACT_OK;

    // Ensure packet data is readable
    if (bpf_skb_pull_data(skb, skb->len) < 0)
        return TC_ACT_OK;

    // Refresh data/data_end pointers
    data = (void *)(long)skb->data;
    data_end = (void *)(long)skb->data_end;
    eth = data;
    ip = (void *)(eth + 1);
    udp = (void *)ip + ip_hdr_len;

    void *payload = (void *)(udp + 1);
    if (payload >= data_end)
        return TC_ACT_OK;

    // Dynamically calculate payload length
    u64 payload_offset = (char *)payload - (char *)data;
    u64 payload_len = (char *)data_end - (char *)payload;

    if (payload_len == 0)
        return TC_ACT_OK;

    // Limit to MAX_SAFE_LEN for verifier safety
    if (payload_len > MAX_PAYLOAD_LEN)
        payload_len = MAX_PAYLOAD_LEN;

    char buf[MAX_PAYLOAD_LEN + 1] = {};
#pragma unroll
    for (int i = 0; i < MAX_PAYLOAD_LEN; i++) {
        if (i >= payload_len)
            break;

        void *p = (char *)payload + i;
        if (p + 1 > data_end)
            break;

        char c = *(char *)p;

        if (c == 'b') {
            *(char *)p = 'B'; 
        }

        buf[i] = c;
    }

    buf[payload_len] = '\0';

    return TC_ACT_OK;
}


// Ingress hook: process incoming packets
int tc_ingress(struct __sk_buff *skb) {
    return TC_ACT_OK;
    // return handle_packet(skb);
}

// Egress hook: process outgoing packets
int tc_egress(struct __sk_buff *skb) {
    return handle_packet(skb);
}
