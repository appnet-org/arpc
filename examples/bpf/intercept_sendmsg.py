from bcc import BPF
import ctypes
import struct

MAX_DATA_LEN = 256
TASK_COMM_LEN = 16
UDP_PORT = 9000  # change to match your aRPC server port

bpf_text = f"""
#include <uapi/linux/ptrace.h>
#include <linux/inet.h>
#include <net/sock.h>
#include <bcc/proto.h>
#include <linux/uio.h>

struct data_t {{
    u32 pid;
    u32 dst_port;
    u32 src_port;
    u32 data_len;
    char task[TASK_COMM_LEN];
    char data[{MAX_DATA_LEN}];
}};

BPF_HASH(packet_count, u32, u64);
BPF_PERF_OUTPUT(events);

int trace_udp_sendmsg(struct pt_regs *ctx, struct sock *sk, struct msghdr *msg, size_t len) {{
    u16 dport = 0, sport = 0;

    // Get source port from sk
    sport = sk->__sk_common.skc_num;

    // Try to get destination port from msghdr->msg_name
    if (msg->msg_name != NULL) {{
        struct sockaddr_in sa = {{}};
        bpf_probe_read_user(&sa, sizeof(sa), msg->msg_name);
        dport = ntohs(sa.sin_port);
    }}
    
    if (dport != {UDP_PORT}) {{
        return 0;
    }}

    struct data_t data = {{0}};
    data.pid = bpf_get_current_pid_tgid() >> 32;
    data.dst_port = dport;
    data.src_port = sport;
    bpf_get_current_comm(&data.task, sizeof(data.task));

    const struct iovec *iov = msg->msg_iter.iov;
    size_t data_len = iov->iov_len;
    if (data_len > {MAX_DATA_LEN}) {{
        data_len = {MAX_DATA_LEN};
    }}
    data.data_len = data_len;

    if (bpf_probe_read_user(&data.data, data_len, iov->iov_base) != 0) {{
        return 0;
    }}

    u32 key = 0;
    u64 *count = packet_count.lookup(&key);
    if (count) (*count)++;
    else {{
        u64 init = 1;
        packet_count.update(&key, &init);
    }}

    events.perf_submit(ctx, &data, sizeof(data));
    return 0;
}}
"""

# Compile and attach
b = BPF(text=bpf_text)
b.attach_kprobe(event="udp_sendmsg", fn_name="trace_udp_sendmsg")

# Define event structure
class Data(ctypes.Structure):
    _fields_ = [
        ("pid", ctypes.c_uint),
        ("dst_port", ctypes.c_uint),
        ("src_port", ctypes.c_uint),
        ("data_len", ctypes.c_uint),
        ("task", ctypes.c_char * TASK_COMM_LEN),
        ("data", ctypes.c_char * MAX_DATA_LEN),
    ]

def decode_arpc_frame(data):
    try:
        offset = 0

        if len(data) < offset + 2:
            return f"Too short for service_len (offset={offset}, len={len(data)})", {}
        service_len = struct.unpack("<H", data[offset:offset+2])[0]
        offset += 2

        if len(data) < offset + service_len:
            return f"Too short for service (offset={offset}, service_len={service_len}, len={len(data)})", {}
        service = data[offset:offset+service_len].decode("utf-8", errors="ignore")
        offset += service_len

        if len(data) < offset + 2:
            return f"Too short for method_len (offset={offset}, len={len(data)})", {}
        method_len = struct.unpack("<H", data[offset:offset+2])[0]
        offset += 2

        if len(data) < offset + method_len:
            return f"Too short for method (offset={offset}, method_len={method_len}, len={len(data)})", {}
        method = data[offset:offset+method_len].decode("utf-8", errors="ignore")
        offset += method_len

        if len(data) < offset + 2:
            return f"Too short for header_len (offset={offset}, len={len(data)})", {}
        header_len = struct.unpack("<H", data[offset:offset+2])[0]
        offset += 2

        if len(data) < offset + header_len:
            return f"Too short for headers (offset={offset}, header_len={header_len}, len={len(data)})", {}
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

# Print perf event
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

print(f"Tracing aRPC (UDP) on port {UDP_PORT}... Ctrl+C to exit.")
while True:
    try:
        b.perf_buffer_poll()
    except KeyboardInterrupt:
        exit()
