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

static __always_inline int payload_contains_bob(const char *s, int len) {
#pragma unroll
    for (int i = 0; i < MAX_PAYLOAD_LEN - 3; i++) {
        if (s[i] == 'B' && s[i+1] == 'o' && s[i+2] == 'b') {
            bpf_trace_printk("Dropping packet with payload containing 'Bob' at offset %d\n", i);
            return 1;
        }
    }
    return 0;
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

    if (skb->len < payload_offset + 3)
        return TC_ACT_OK;

    char buf[MAX_PAYLOAD_LEN] = {};
    if (bpf_skb_load_bytes(skb, payload_offset, buf, MAX_PAYLOAD_LEN) < 0)
        return TC_ACT_OK;

    __builtin_memcpy(new_data.payload, buf, MAX_PAYLOAD_LEN);
    new_data.payload_len = MAX_PAYLOAD_LEN;
    events.perf_submit(skb, &new_data, sizeof(new_data));

    // Do this last: check if we need to drop
    bpf_trace_printk("Checking payload for 'Bob'\n");
    if (payload_contains_bob(buf, MAX_PAYLOAD_LEN)) {
        return TC_ACT_SHOT;  // Drop after emitting
    }

    return TC_ACT_OK;
}


int tc_ingress(struct __sk_buff *skb) {
    return handle_packet(skb);
}

int tc_egress(struct __sk_buff *skb) {
    return handle_packet(skb);
}
