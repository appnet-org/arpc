from bcc import BPF

UDP_PORT = 9000

bpf_text = f"""
#include <uapi/linux/ptrace.h>
#include <net/sock.h>
#include <linux/in.h>
#include <linux/in6.h>
#include <linux/uio.h>

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
    
        // Read first 13 bytes from msg->msg_iter.iov[0].iov_base
    const struct iovec *iovp = NULL;
    bpf_probe_read_user(&iovp, sizeof(iovp), &msg->msg_iter.iov);

    if (iovp == NULL)
        return 0;

    char buf[13] = {{0}};
    void *base = NULL;
    bpf_probe_read_user(&base, sizeof(base), &iovp->iov_base);
    bpf_probe_read_user(&buf, sizeof(buf), base);

    // Print as hex (safe, since bpf_trace_printk can't print strings with embedded nulls)
    bpf_trace_printk("UDP 0-2: %x %x %x\\n", buf[0], buf[1], buf[2]);
    bpf_trace_printk("UDP 3-5: %x %x %x\\n", buf[3], buf[4], buf[5]);
    bpf_trace_printk("UDP 6-8: %x %x %x\\n", buf[6], buf[7], buf[8]);
    bpf_trace_printk("UDP 9-11: %x %x %x\\n", buf[9], buf[10], buf[11]);
    bpf_trace_printk("UDP 12: %x\\n", buf[12]);

    
    return 0;
}}
"""

# Compile and attach
b = BPF(text=bpf_text)
b.attach_kprobe(event="udp_sendmsg", fn_name="trace_udp_sendmsg")

print("Tracing UDP sendmsg... Ctrl+C to exit.")
print("Run `sudo cat /sys/kernel/debug/tracing/trace_pipe` to view logs.")
while True:
    try:
        pass
    except KeyboardInterrupt:
        break
