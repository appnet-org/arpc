#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <linux/types.h>
#include <linux/pkt_cls.h>
#include <linux/in.h>

#define MAX_PAYLOAD_LEN 64
#define PAYLOAD_SIZE 64

typedef __u32 u32;
typedef __u16 u16;
typedef __u8 u8;

// Structure to hold packet metadata and a portion of its payload
struct data_t {
    u32 saddr;                  // Source IP address
    u32 daddr;                  // Destination IP address
    u16 sport;                  // Source port
    u16 dport;                  // Destination port
    u8 protocol;                // L4 protocol (e.g., UDP)
    u8 payload[MAX_PAYLOAD_LEN];// First MAX_PAYLOAD_LEN bytes of payload
    u32 payload_len;            // Actual payload length captured
};

// Perf event output map for sending data to user space
BPF_PERF_OUTPUT(events);


// Inline function to process packets
static __always_inline int handle_packet(struct __sk_buff *skb) {
    void *data = (void *)(long)skb->data;
    void *data_end = (void *)(long)skb->data_end;

    struct ethhdr *eth = data;
    if (data + sizeof(*eth) > data_end)
        return TC_ACT_OK;


    // Only handle IPv4 packets
    if (eth->h_proto != htons(ETH_P_IP))
        return TC_ACT_OK;

    struct iphdr *ip = data + sizeof(*eth);
    if (data + sizeof(*eth) + sizeof(*ip) > data_end)
        return TC_ACT_OK;

    struct data_t new_data = {};
    new_data.saddr = ip->saddr;
    new_data.daddr = ip->daddr;
    new_data.protocol = ip->protocol;

    u16 ip_hdr_len = ip->ihl * 4;

    // Only handle UDP packets
    if (ip->protocol == IPPROTO_UDP) {
        struct udphdr *udp = (void *)ip + sizeof(*ip);

        if ((void *)udp + sizeof(*udp) > data_end)
            return TC_ACT_OK;
        new_data.sport = ntohs(udp->source);
        new_data.dport = ntohs(udp->dest);

        // Only capture packets with source or dest port 9000
        if (new_data.sport != 9000 && new_data.dport != 9000)
            return TC_ACT_OK;

        // Extract payload
        u16 total_len = ntohs(udp->len); // Convert to host byte order
        u16 udp_hdr_len = sizeof(*udp);
        u16 payload_offset = sizeof(*eth) + ip_hdr_len + udp_hdr_len;

        
        if (skb->len > payload_offset)
        {
            if (data + payload_offset + PAYLOAD_SIZE > data_end)
                return TC_ACT_OK;
            bpf_skb_load_bytes(skb, payload_offset, new_data.payload, PAYLOAD_SIZE);
        }

        new_data.payload_len = PAYLOAD_SIZE;

    } else {
        // Ignore non-UDP packets
        return TC_ACT_OK;
    }

    // Submit the collected data to user space
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
