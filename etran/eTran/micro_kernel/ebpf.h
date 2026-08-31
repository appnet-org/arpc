#ifndef EBPF_H
#define EBPF_H

#include <vector>

#include <runtime/defs.h>
#include <runtime/ebpf_if.h>

#include "nic.h"

struct ebpf_info_t
{
    /* interface name */
    std::string if_name;
    /* interface index */
    int ifindex;
    /* XDP program */
    struct xdp_program *xdp_prog;
    /* XDP attach mode */
    enum xdp_attach_mode attach_mode;
    /* XDP bind flags */
    unsigned int xdp_bind_flags;
    /* enable busy polling */
    bool busy_poll;
    /* busy polling batch size */
    int busy_poll_batch_size;
    /* XDP program fd */
    int xdp_fd;
    /* XDP_EGRESS program fd */
    int xdp_egress_fd;
    /* XDP_GEN program fd */
    int xdp_gen_fd;
    /* CPUMAP program fd */
    int cpumap_prog_fd;
};

#endif // EBPF_H