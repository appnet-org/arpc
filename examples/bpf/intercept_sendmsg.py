from bcc import BPF
import ctypes
import struct

MAX_DATA_LEN = 256
TASK_COMM_LEN = 16
UDP_PORT = 9000  # change to match your aRPC server port

bpf_text = f"""
#include <uapi/linux/ptrace.h>
#include <net/sock.h>
#include <linux/in.h>
#include <linux/in6.h>
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
    u16 sport = sk->__sk_common.skc_num;
    u16 dport = 0;

    if (msg->msg_name) {{
        u16 family = 0;
        bpf_probe_read_user(&family, sizeof(family), msg->msg_name);

        if (family == AF_INET6) {{
            struct sockaddr_in6 sa6 = {{}};
            bpf_probe_read_user(&sa6, sizeof(sa6), msg->msg_name);
            dport = ntohs(sa6.sin6_port);
        }} else if (family == AF_INET) {{
            struct sockaddr_in sa4 = {{}};
            bpf_probe_read_user(&sa4, sizeof(sa4), msg->msg_name);
            dport = ntohs(sa4.sin_port);
        }}
    }} else {{
        return 0;
    }}
    
    if (dport != {UDP_PORT}) {{
        return 0;
    }}
    
    bpf_trace_printk("traced udp_sendmsg for sport %d, dport %d\\n", sport, dport);
    
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

# Print perf event
def print_event(cpu, data, size):
    event = ctypes.cast(data, ctypes.POINTER(Data)).contents
    payload = bytes(event.data[:event.data_len])
    print(f"\n[{event.task.decode()}] pid={event.pid} sent {event.data_len} bytes "
          f"(src_port={event.src_port} -> dst_port={event.dst_port})")

    print(f"  Raw payload (hex): {payload.hex()}")

# Start tracing
b["events"].open_perf_buffer(print_event)


print("Tracing UDP sendmsg... Ctrl+C to exit.")
while True:
    try:
        b.perf_buffer_poll()
    except KeyboardInterrupt:
        exit()
