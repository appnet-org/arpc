from bcc import BPF
import ctypes
import socket
import struct
import os

MAX_DATA_LEN = 256
UDP_PORT = 9000  # aRPC port
IFACE = "lo"   # CHANGE to your actual interface

bpf_text = f"""
#include <uapi/linux/bpf.h>
#include <uapi/linux/if_ether.h>
#include <uapi/linux/ip.h>
#include <uapi/linux/udp.h>
#include <uapi/linux/in.h>

#define MAX_DATA_LEN {MAX_DATA_LEN}

struct ethernet_t {{
    unsigned char dst[6];
    unsigned char src[6];
    unsigned short proto;
}};

struct data_t {{
    u32 dst_port;
    u32 src_port;
    u32 data_len;
    char data[MAX_DATA_LEN];
}};

BPF_PERF_OUTPUT(events);
BPF_HASH(packet_count, u32, u64);

int handle_egress(struct __sk_buff *skb) {{
    u32 offset = 0;
    struct ethernet_t eth = {{}};
    bpf_skb_load_bytes(skb, offset, &eth, sizeof(eth));
    offset += sizeof(eth);
    if (eth.proto != htons(ETH_P_IP)) return 1;

    struct iphdr ip = {{}};
    bpf_skb_load_bytes(skb, offset, &ip, sizeof(ip));
    offset += sizeof(ip);
    if (ip.protocol != IPPROTO_UDP) return 1;

    struct udphdr udp = {{}};
    bpf_skb_load_bytes(skb, offset, &udp, sizeof(udp));
    offset += sizeof(udp);
    if (udp.dest != htons({UDP_PORT})) return 1;

    u32 hdr_len = sizeof(eth) + sizeof(ip) + sizeof(udp);
    u32 udp_len = bpf_ntohs(udp.len);
    u32 payload_len = 0;
    if (udp_len > sizeof(udp)) {{
        payload_len = udp_len - sizeof(udp);
        if (payload_len > MAX_DATA_LEN)
            payload_len = MAX_DATA_LEN;

        struct data_t pkt = {{}};
        bpf_skb_load_bytes(skb, hdr_len, &pkt.data, payload_len);
        pkt.data_len = payload_len;
        pkt.src_port = bpf_ntohs(udp.source);
        pkt.dst_port = bpf_ntohs(udp.dest);

        u32 key = 0;
        u64 *count = packet_count.lookup(&key);
        if (count) (*count)++;
        else {{
            u64 init = 1;
            packet_count.update(&key, &init);
        }}

        events.perf_submit_skb(skb, skb->len, &pkt, sizeof(pkt));
    }}
    return 1;
}}
"""

b = BPF(text=bpf_text)
fn = b.load_func("handle_egress", BPF.SCHED_CLS)

# Attach to egress on the selected interface
os.system(f"tc qdisc del dev {IFACE} clsact > /dev/null 2>&1")
os.system(f"tc qdisc add dev {IFACE} clsact")
os.system(f"tc filter add dev {IFACE} egress bpf da obj {b.fn_table['handle_egress'].name}.o sec 'classifier'")

# Define event structure
class Data(ctypes.Structure):
    _fields_ = [
        ("dst_port", ctypes.c_uint),
        ("src_port", ctypes.c_uint),
        ("data_len", ctypes.c_uint),
        ("data", ctypes.c_char * MAX_DATA_LEN),
    ]

def decode_arpc_frame(data):
    try:
        offset = 0
        if len(data) < offset + 2:
            return f"Too short for service_len", {}
        service_len = struct.unpack("<H", data[offset:offset+2])[0]
        offset += 2
        service = data[offset:offset+service_len].decode("utf-8", errors="ignore")
        offset += service_len

        method_len = struct.unpack("<H", data[offset:offset+2])[0]
        offset += 2
        method = data[offset:offset+method_len].decode("utf-8", errors="ignore")
        offset += method_len

        header_len = struct.unpack("<H", data[offset:offset+2])[0]
        offset += 2
        headers_raw = data[offset:offset+header_len]
        offset += header_len

        return "OK", {
            "service": service,
            "method": method,
            "headers": headers_raw.hex(),
            "payload": data[offset:].hex()
        }
    except Exception as e:
        return f"Error: {e}", {}

def print_event(cpu, data, size):
    event = ctypes.cast(data, ctypes.POINTER(Data)).contents
    payload = bytes(event.data[:event.data_len])
    print(f"\n[{event.task.decode()}] pid={event.pid} sent {event.data_len} bytes "
          f"(src_port={event.src_port} -> dst_port={event.dst_port})")

    print(f"  Raw payload (hex): {payload.hex()}")

    status, frame = decode_arpc_frame(payload)
    if status != "OK":
        print(f"  Decode failed: {status}")
    else:
        print(f"  Service: {frame['service']}")
        print(f"   Method:  {frame['method']}")
        print(f"   Headers: {frame['headers']}")
        print(f"   Payload (hex): {frame['payload'][:32]}...")

    count = b["packet_count"][ctypes.c_uint(0)].value
    print(f"Total packets: {count}")

b["events"].open_perf_buffer(print_event)

print(f"Tracing UDP aRPC on port {UDP_PORT} via TC on {IFACE}... Ctrl+C to exit.")
try:
    while True:
        b.perf_buffer_poll()
except KeyboardInterrupt:
    print("Detaching TC hooks...")
    os.system(f"tc qdisc del dev {IFACE} clsact")
