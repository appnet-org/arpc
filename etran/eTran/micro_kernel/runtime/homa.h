#pragma once

#include <intf/intf_ebpf.h>
#include <tran_def/homa.h>
#include <runtime/ebpf_if.h>

#define HOMA_MSS (1514 - 14 - 20 - HOMA_DATA_H)

/* How many resends are need for aborting this RPC */
#define ABORT_RESEND 10
/* How many ticks are needed for sending NEED_ACK packet */
#define NEED_ACK_TICK 15
/* How many ticks are needed for sending RESEND packet */
#define RESEND_TICK 15

struct homa_socket
{
    struct app_ctx_per_thread *tctx;
    opaque_ptr opaque_socket;
    int fd;
    uint32_t local_ip;
    uint16_t local_port;

    homa_socket() {}
};

int homa_bind(struct app_ctx_per_thread *tctx, struct appout_homa_bind_t *homa_bind_msg_in);
int homa_close(struct app_ctx_per_thread *tctx, opaque_ptr opaque_socket);

void free_homa_resources(struct app_ctx *actx);

void process_homa_cmd(struct app_ctx_per_thread *tctx, lrpc_msg *msg);

int poll_homa_to(void);

