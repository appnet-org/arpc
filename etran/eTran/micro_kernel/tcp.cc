#include <runtime/tcp.h>

#include <unordered_map>
#include <list>
#include <mutex>

#include <base/ipc.h>
#include <runtime/app_if.h>
#include <runtime/ebpf_if.h>

#include "trans_ebpf.h"
#include "nic.h"

// mirco_kernel.cc
extern class eTranNIC *etran_nic;
extern unsigned int opt_arpc_case;

// ebpf.cc
extern class eTranTCP *etran_tcp;

uint64_t next_tcp_cc_to_tsc = UINT64_MAX;
// TODO: configure these parameters
const unsigned int TCP_HANDSHAKE_TIMEOUT = 50; // ms
const unsigned int TCP_CLOSE_TIMEOUT = 100;    // ms

// this map includes all TCP connections of all states except for CONN_WAIT_RX_SYN
std::unordered_map<struct flow_tuple, struct tcp_connection *, flow_tuple_hash, flow_tuple_equal> tcp_connections;
std::mutex tcp_connections_lock;
std::unordered_map<struct listen_tuple, std::vector<struct tcp_listener *>, listen_tuple_hash, listen_tuple_equal> tcp_listeners;
std::mutex tcp_listeners_lock;

std::list<struct tcp_connection *> tcp_handshake_list;

// ebpf.cc
extern class eTranTCP *etran_tcp;

static inline int alloc_cc_idx(uint32_t *idx)
{
    if (etran_tcp->_avail_cc_idxs.empty())
    {
        return -1;
    }
    *idx = etran_tcp->_avail_cc_idxs.front();
    etran_tcp->_avail_cc_idxs.pop_front();
    return 0;
}

static inline void free_cc_idx(uint32_t idx)
{
    etran_tcp->_avail_cc_idxs.push_back(idx);
}

// control_plane.cc
extern int alloc_port(uint16_t port);
extern int alloc_port(void);
extern int free_port(uint16_t port);
extern int record_port(struct app_ctx *actx, uint16_t local_port, uint16_t remote_port);
extern int unrecord_port(struct app_ctx *actx, uint16_t port);

// cc.cc
extern void timely_cc(struct tcp_connection *c, struct bpf_cc_snapshot *cc, uint64_t curr_tsc);
extern void dctcp_wnd_cc(struct tcp_connection *c, struct bpf_cc_snapshot *cc, uint64_t curr_tsc);
extern void dctcp_rate_cc(struct tcp_connection *c, struct bpf_cc_snapshot *cc, uint64_t curr_tsc);

// APPIN_TCP_EVENT_NEWCONN
void notify_app_tcp_event_newconn(struct app_ctx_per_thread *tctx, opaque_ptr l, int fd, uint32_t remote_ip, uint16_t remote_port);
// APPIN_TCP_CONN_ACCEPT
void notify_app_tcp_event_accept(struct app_ctx_per_thread *tctx, opaque_ptr c, int fd, int32_t status, uint32_t rx_buf_size, uint32_t tx_buf_size, uint32_t local_ip, uint32_t remote_ip, uint16_t remote_port, uint32_t qid, bool backlog);
// APPIN_TCP_CONN_OPEN
void notify_app_tcp_conn_open(struct app_ctx_per_thread *tctx, opaque_ptr c, int fd, int32_t status, struct tcp_connection *conn);
// APPIN_TCP_STATUS_BIND
void notify_app_tcp_status_bind(struct app_ctx_per_thread *tctx, opaque_ptr c, int fd, int32_t status);
// APPIN_TCP_STATUS_LISTEN
void notify_app_tcp_status_listen(struct app_ctx_per_thread *tctx, opaque_ptr l, int fd, int32_t status);
// APPIN_TCP_STATUS_CLOSE
void notify_app_tcp_status_close(struct app_ctx_per_thread *tctx, opaque_ptr c, int fd, int32_t status);

void slow_path_send_tcp(struct app_ctx *actx, struct pkt_tcp *tcphdr, uint16_t len, bool to, uint32_t qid);

// internal functions
static inline struct tcp_connection *find_tcp_conn_slowpath(opaque_ptr opaque_connection)
{
    std::lock_guard<std::mutex> lock(tcp_connections_lock);
    for (auto it = tcp_connections.begin(); it != tcp_connections.end(); it++)
    {
        if (it->second->opaque_connection == opaque_connection)
        {
            return it->second;
        }
    }
    return nullptr;
}

static inline void reg_tcp_conn_slowpath(struct tcp_connection *c)
{
    std::lock_guard<std::mutex> lock(tcp_connections_lock);
    tcp_connections.insert(std::make_pair(flow_tuple(c->remote_ip, c->remote_port, c->local_ip, c->local_port), c));
}
// already holding tcp_connections_lock
static inline void unreg_tcp_conn_slowpath(struct tcp_connection *c)
{
    for (auto it = tcp_connections.begin(); it != tcp_connections.end(); it++)
    {
        if (it->second == c)
        {
            tcp_connections.erase(it);
            break;
        }
    }
}
static void _tcp_connection_close(struct tcp_connection *c, enum connection_status status, bool send_rst);
static void tcp_connection_close(struct kref *ref);

static int tcp_synack_pkt(struct tcp_connection *c, struct pkt_tcp *p, struct tcp_opts *opts);

// This function is called when connection is not established
static void send_tcp_reset(struct app_ctx *actx, const struct pkt_tcp *orig_p);
static void send_tcp_control(struct tcp_connection *c, uint8_t flags, int ts_opt, uint32_t ts_echo, uint16_t mss_opt);

static int reg_tcp_conn_ebpf(struct tcp_connection *c, bool listen);
static void unreg_tcp_conn_ebpf(struct tcp_connection *c);

static struct tcp_connection *tcp_conn_lookup(struct pkt_tcp *p);

static struct tcp_listener *tcp_listener_lookup(struct pkt_tcp *p);
static void tcp_listener_accept(struct tcp_listener *l);

static void tcp_connection_pkt(struct tcp_connection *c, struct pkt_tcp *p, uint32_t qid, struct tcp_opts *opts);

static void tcp_listener_pkt(struct tcp_listener *l, struct pkt_tcp *p, uint32_t qid, struct tcp_opts *opts);

static inline int parse_tcp_opts(struct pkt_tcp *p, struct tcp_opts *opts)
{
    uint8_t *opt = (uint8_t *)(p + 1);
    uint16_t opts_len = TCPH_HDRLEN(&p->tcp) * 4 - TCP_HLEN;
    uint16_t off = 0;
    uint8_t opt_kind, opt_len;

    while (off < opts_len)
    {
        opt_kind = opt[off];
        if (opt_kind == TCP_OPT_END_OF_OPTIONS)
            break;
        else if (opt_kind == TCP_OPT_NO_OP)
        {
            off++;
            continue;
        }
        else
        {
            opt_len = opt[off + 1];
            if (opt_kind == TCP_OPT_MSS)
            {
                if (opt_len != sizeof(struct tcp_mss_opt))
                    return -1;
                opts->mss = (struct tcp_mss_opt *)(opt + off);
#ifdef DEBUG_TCP
                fprintf(stdout, "Recognize MSS option = %u\n", ntohs(opts->mss->mss));
#endif
            }
            else if (opt_kind == TCP_OPT_TIMESTAMP)
            {
                if (opt_len != sizeof(struct tcp_timestamp_opt))
                    return -1;
                opts->ts = (struct tcp_timestamp_opt *)(opt + off);
#ifdef DEBUG_TCP
                fprintf(stdout, "Recognize TimeStamp option\n");
#endif
            }
            else
            {
#ifdef DEBUG_TCP
                fprintf(stdout, "Unrecognized option kind = %u\n", opt_kind);
#endif
                return -1;
            }
            off += opt_len;
        }
    }
    return 0;
}

void _tcp_connection_close(struct tcp_connection *c, enum connection_status status, bool send_rst)
{
    /* this struct is created for listener */
    if (c->type == TCP_CONN_TYPE_FAKE)
    {
        unrecord_port(c->tctx->actx, c->local_port);
        free_port(c->local_port);
        std::lock_guard<std::mutex> lock(tcp_connections_lock);
        tcp_connections.erase(flow_tuple(c->remote_ip, c->remote_port, c->local_ip, c->local_port));
        delete c;
        return;
    }

    /* step1: remove from tcp_connections */
    tcp_connections_lock.lock();
    unreg_tcp_conn_slowpath(c);
    tcp_connections_lock.unlock();

    /* step2: delete ebpf states if needed */
    if (c->status == CONN_OPEN || c->status == CONN_WAIT_TX_SYNACK)
    {
        unreg_tcp_conn_ebpf(c);
    }

    /* step3: free port if this connection is created through connect() */
    if (!c->listener)
    {
        unrecord_port(c->tctx->actx, c->local_port);
        free_port(c->local_port);
    }

    /* step4: send TCP_RST if needed */
    if (send_rst)
    {
        printf("Send TCP_RST, %u\n", c->local_port);
        send_tcp_control(c, TCP_RST, 0, 0, 0);
    }

    fprintf(stdout, "TCP connection (%p, %d) is closed\n", c, status);

    delete c;
}

void tcp_connection_close(struct kref *ref)
{
    struct tcp_connection *c = container_of(ref, struct tcp_connection, ref);
    _tcp_connection_close(c, CONN_CLOSED, true);
}

static inline void tcp_conn_get(struct tcp_connection *c)
{
    kref_get(&c->ref);
}

static inline void tcp_conn_put(struct tcp_connection *c)
{
    kref_put(&c->ref, c->release);
}

static int tcp_synack_pkt(struct tcp_connection *c, struct pkt_tcp *p, struct tcp_opts *opts)
{
    uint32_t ecn_flags = TCPH_FLAGS(&p->tcp) & (TCP_ECE | TCP_CWR);

    /* stop timer */
    c->next_timeout_tsc = 0;

    /* check TCP flags */
    if ((TCPH_FLAGS(&p->tcp) & (TCP_SYN | TCP_ACK)) != (TCP_SYN | TCP_ACK))
    {
        fprintf(stderr, "tcp_synack_pkt: unexpected flags %x\n",
                TCPH_FLAGS(&p->tcp));
        goto fail;
    }

    if (opts->ts == nullptr)
    {
        fprintf(stderr, "tcp_synack_pkt: no timestamp option received\n");
        goto fail;
    }

    /* update connection state */
    memcpy(c->local_mac, (uint8_t *)p->eth.dest.addr, ETH_ALEN);
    memcpy(c->remote_mac, (uint8_t *)p->eth.src.addr, ETH_ALEN);
    c->remote_seq = ntohl(p->tcp.seqno) + 1;
    c->local_seq = ntohl(p->tcp.ackno);
    c->syn_ts = ntohl(opts->ts->ts_val);

    if (ecn_flags == TCP_ECE)
    {
        c->flags |= ECN_ENABLE;
    }

    /* add eBPF state */
    if (reg_tcp_conn_ebpf(c, false))
    {
        fprintf(stderr, "tcp_synack_pkt: failed to register connection\n");
        goto fail;
    }

    c->status = CONN_OPEN;

    /* notify application connect() success */
    notify_app_tcp_conn_open(c->tctx, c->opaque_connection, c->fd, 1, c);

    /* send ACK */
    send_tcp_control(c, TCP_ACK, 1, c->syn_ts, 0);

    return 0;

fail:
    /* restore connection state */
    memset(c->local_mac, 0, ETH_ALEN);
    memset(c->remote_mac, 0, ETH_ALEN);
    c->remote_seq = 0;
    c->local_seq = 0;
    c->syn_ts = 0;
    c->flags = 0;

    /* rearm a TCP handshake timer */
    c->next_timeout_tsc = get_cycles() + us_to_cycles(TCP_HANDSHAKE_TIMEOUT * 1000);

    return -1;
}

static void send_tcp_reset(struct app_ctx *actx, const struct pkt_tcp *orig_p)
{
    struct pkt_tcp *p;
    struct tcp_hdr *tcp;
    uint16_t len = sizeof(*p);

    p = (struct pkt_tcp *)calloc(1, sizeof(*p));
    if (!p)
    {
        fprintf(stderr, "send_tcp_reset: failed to allocate memory\n");
        return;
    }

    tcp = &p->tcp;

    memcpy(&p->eth, &orig_p->eth, sizeof(p->eth));
    p->eth.dest = orig_p->eth.src;
    p->eth.src = orig_p->eth.dest;

    memcpy(&p->ip, &orig_p->ip, sizeof(p->ip));
    p->ip.src = orig_p->ip.dest;
    p->ip.dest = orig_p->ip.src;

    /* fill tcp header */
    p->tcp.src = orig_p->tcp.dest;
    p->tcp.dest = orig_p->tcp.src;
    p->tcp.seqno = 0;
    p->tcp.ackno = 0;
    TCPH_HDRLEN_FLAGS_SET(&p->tcp, 5, TCP_RST);
    p->tcp.wnd = 0;
    p->tcp.chksum = 0;
    p->tcp.urgp = 0;

    /* calculate header checksums */
    p->ip.chksum = ip_fast_csum((const void *)&p->ip, p->ip._v_hl);
    p->tcp.chksum = tcp_csum(p->ip.src, p->ip.dest, len, IPPROTO_TCP, (uint8_t *)tcp);

    /* send packet */
    slow_path_send_tcp(actx, p, len, false, POISON_32);

    free(p);
    return;
}

static void send_tcp_control(struct tcp_connection *c, uint8_t flags, int ts_opt, uint32_t ts_echo, uint16_t mss_opt)
{
    struct app_ctx *actx = c->tctx->actx;
    struct pkt_tcp *p;
    struct tcp_mss_opt *opt_mss;
    struct tcp_timestamp_opt *opt_ts;
    struct tcp_hdr *tcp;

    uint32_t remote_ip = c->remote_ip;
    uint16_t remote_port = c->remote_port;
    uint32_t local_ip = c->local_ip;
    uint16_t local_port = c->local_port;
    uint32_t local_seq = c->local_seq;
    uint32_t remote_seq = c->remote_seq;

    uint8_t optlen;
    uint16_t len, off_ts, off_mss;

    /* calculate header length depending on options */
    optlen = 0;
    off_mss = optlen;
    optlen += (mss_opt ? sizeof(*opt_mss) : 0);
    off_ts = optlen;
    optlen += (ts_opt ? sizeof(*opt_ts) : 0);
    optlen = (optlen + 3) & ~3;
    len = sizeof(*p) + optlen;

    p = (struct pkt_tcp *)calloc(1, len);
    if (!p)
    {
        fprintf(stderr, "send_tcp_control: failed to allocate memory\n");
        return;
    }
    tcp = &p->tcp;

    /* fill ipv4 header */
    IPH_VHL_SET(&p->ip, 4, 5);
    p->ip._tos = 0;
    p->ip.len = htons(len - offsetof(struct pkt_tcp, ip));
    p->ip.id = htons(0);
    p->ip.offset = htons(0);
    p->ip.ttl = 0xff;
    p->ip.proto = IP_PROTO_TCP;
    p->ip.chksum = 0;
    p->ip.src = htonl(local_ip);
    p->ip.dest = htonl(remote_ip);

    /* fill tcp header */
    p->tcp.src = htons(local_port);
    p->tcp.dest = htons(remote_port);
    p->tcp.seqno = htonl(local_seq);
    p->tcp.ackno = htonl(remote_seq);
    TCPH_HDRLEN_FLAGS_SET(&p->tcp, 5 + optlen / 4, flags);
    p->tcp.wnd = htons(11680); /* TODO */
    p->tcp.chksum = 0;
    p->tcp.urgp = htons(0);

    /* if requested: add mss option */
    if (mss_opt)
    {
        opt_mss = (struct tcp_mss_opt *)((uint8_t *)(p + 1) + off_mss);
        opt_mss->kind = TCP_OPT_MSS;
        opt_mss->length = sizeof(*opt_mss);
        opt_mss->mss = htons(mss_opt);
    }

    /* if requested: add timestamp option */
    if (ts_opt)
    {
        opt_ts = (struct tcp_timestamp_opt *)((uint8_t *)(p + 1) + off_ts);
        opt_ts->kind = TCP_OPT_TIMESTAMP;
        opt_ts->length = sizeof(*opt_ts);
        opt_ts->ts_val = htonl(0);
        opt_ts->ts_ecr = htonl(ts_echo);
    }

    /* calculate header checksums */
    p->ip.chksum = ip_fast_csum((const void *)&p->ip, p->ip._v_hl);
    p->tcp.chksum = tcp_csum(p->ip.src, p->ip.dest, len, IPPROTO_TCP, (uint8_t *)tcp);

    /* send packet */
    slow_path_send_tcp(actx, p, len, false, c->qid);

    free(p);

    return;
}

static void unreg_tcp_conn_ebpf(struct tcp_connection *c)
{
    struct ebpf_flow_tuple key = {0};
    struct bpf_tcp_conn ebpf_c = {0};

    key.local_ip = c->local_ip;
    key.remote_ip = c->remote_ip;
    key.local_port = c->local_port;
    key.remote_port = c->remote_port;

    if (bpf_map_lookup_elem(etran_tcp->_tcp_connection_map_fd, &key, &ebpf_c))
    {
        fprintf(stderr, "unreg_tcp_conn_ebpf: failed to lookup ebpf map\n");
    }

    free_cc_idx(ebpf_c.cc_idx);

    if (bpf_map_delete_elem(etran_tcp->_tcp_connection_map_fd, &key))
    {
        fprintf(stderr, "unreg_tcp_conn_ebpf: failed to delete ebpf map\n");
    }

    return;
}

static int reg_tcp_conn_ebpf(struct tcp_connection *c, bool listen)
{
    struct ebpf_flow_tuple key = {0};
    struct bpf_tcp_conn ebpf_c = {0};
    uint32_t cc_idx;

    /* allocate a CC index */
    if (alloc_cc_idx(&cc_idx))
    {
        fprintf(stderr, "reg_tcp_conn_ebpf: failed to allocate cc idx\n");
        return -1;
    }

    /* initialize eBPF CC state */
    etran_tcp->_tcp_cc_map_mmap->entry[cc_idx].prev_desired_tx_ts = 0;
    etran_tcp->_tcp_cc_map_mmap->entry[cc_idx].rate = CC_DCTCP_MIN;
    etran_tcp->_tcp_cc_map_mmap->entry[cc_idx].cnt_tx_drops = 0;
    etran_tcp->_tcp_cc_map_mmap->entry[cc_idx].cnt_rx_acks = 0;
    etran_tcp->_tcp_cc_map_mmap->entry[cc_idx].cnt_rx_ack_bytes = 0;
    etran_tcp->_tcp_cc_map_mmap->entry[cc_idx].cnt_rx_ecn_bytes = 0;
    etran_tcp->_tcp_cc_map_mmap->entry[cc_idx].rtt_est = 0;

    /* initialize eBPF state */
    ebpf_c.opaque_connection = OPAQUE(c->opaque_connection);
    ebpf_c.qid = c->qid;
    memcpy(ebpf_c.local_mac, c->local_mac, ETH_ALEN);
    memcpy(ebpf_c.remote_mac, c->remote_mac, ETH_ALEN);
    ebpf_c.local_ip = c->local_ip;
    ebpf_c.remote_ip = c->remote_ip;
    ebpf_c.local_port = c->local_port;
    ebpf_c.remote_port = c->remote_port;

    ebpf_c.rx_buf_size = etran_tcp->_trans_params.tcp.rx_buf_size;
    ebpf_c.tx_buf_size = etran_tcp->_trans_params.tcp.tx_buf_size;
    ebpf_c.rx_avail = std::min(etran_tcp->_trans_params.tcp.rx_buf_size, (unsigned int)(0xffff << TCP_WND_SCALE));
    ebpf_c.rx_remote_avail = std::min(etran_tcp->_trans_params.tcp.rx_buf_size, (unsigned int)(0xffff << TCP_WND_SCALE));
    ebpf_c.rx_next_pos = 0;
    ebpf_c.rx_next_seq = c->remote_seq;

    ebpf_c.rx_dupack_cnt = 0;
    ebpf_c.rx_ooo_start = 0;
    ebpf_c.rx_ooo_len = 0;

    ebpf_c.tx_pending = 0;
    ebpf_c.tx_sent = 0;
    ebpf_c.tx_next_pos = 0;
    ebpf_c.tx_next_seq = listen ? c->local_seq + 1 : c->local_seq;
    ebpf_c.tx_next_ts = 0;

    ebpf_c.cc_idx = cc_idx;
    ebpf_c.ecn_enable = c->flags & ECN_ENABLE;
    ebpf_c.arpc_case = etran_tcp->_trans_params.tcp.arpc_case;

    for (unsigned int i = 0; i < c->tctx->actx->nr_nic_queues; i++)
    {
        ebpf_c.qid2xsk[c->tctx->actx->nic_qid[i]] = c->tctx->txrx_xsk_map_key[i];
    }

    key.local_ip = c->local_ip;
    key.remote_ip = c->remote_ip;
    key.local_port = c->local_port;
    key.remote_port = c->remote_port;

    if (bpf_map_update_elem(etran_tcp->_tcp_connection_map_fd, &key, &ebpf_c, BPF_ANY))
    {
        fprintf(stderr, "reg_tcp_conn_ebpf: failed to update ebpf map\n");
        free_cc_idx(cc_idx);
        return -1;
    }

    c->cc_idx = cc_idx;
    memset(&c->cc_data, 0, sizeof(c->cc_data));
#ifdef TIMELY
    c->algorithm = CC_TIMELY;
    c->cc_data.timely.slowstart = 1;
    c->cc_rate = window_to_rate(2 * 1448, TCP_RTT_INIT);
#else
#ifdef DCTCP
    c->algorithm = CC_DCTCP_WND;
    c->cc_rate = window_to_rate(2 * 1448, TCP_RTT_INIT);
    c->cc_data.dctcp_wnd.ecn_rate = 0;
    c->cc_data.dctcp_wnd.window = 2 * 1448;
    c->cc_data.dctcp_wnd.slowstart = 1;
#else
    c->algorithm = CC_NONE;
    c->cc_rate = window_to_rate(2 * 1448, TCP_RTT_INIT);
#endif
#endif

    return 0;
}

void notify_app_tcp_event_accept(struct app_ctx_per_thread *tctx, opaque_ptr c, int fd, int newfd, int32_t status, uint32_t rx_buf_size, uint32_t tx_buf_size, uint32_t local_ip, uint32_t remote_ip, uint16_t remote_port, uint32_t qid, bool backlog)
{
    lrpc_msg msg = {0};
    struct appin_tcp_conn_accept_t *kmsg = (struct appin_tcp_conn_accept_t *)msg.data;

    msg.cmd = APPIN_TCP_CONN_ACCEPT;
    kmsg->opaque_connection = c;
    kmsg->fd = fd;
    kmsg->newfd = newfd;
    kmsg->status = status;
    kmsg->backlog = backlog;
    if (status == 0)
    {
        kmsg->rx_buf_size = rx_buf_size;
        kmsg->tx_buf_size = tx_buf_size;
        kmsg->local_ip = etran_nic->_local_ip;
        kmsg->remote_ip = remote_ip;
        kmsg->remote_port = remote_port;
        kmsg->qid = qid;
    }

    if (lrpc_send(&tctx->kernel_out, &msg))
    {
        fprintf(stderr, "notify_app_tcp_event_accept: failed to send message\n");
    }

    kick_evfd(tctx->evfd);
}

void notify_app_tcp_event_newconn(struct app_ctx_per_thread *tctx, opaque_ptr l, int fd, uint32_t remote_ip, uint16_t remote_port)
{
    lrpc_msg msg = {0};
    struct appin_tcp_event_newconn_t *kmsg = (struct appin_tcp_event_newconn_t *)msg.data;

    msg.cmd = APPIN_TCP_EVENT_NEWCONN;
    kmsg->opaque_listener = l;
    kmsg->fd = fd;
    kmsg->remote_ip = remote_ip;
    kmsg->remote_port = remote_port;

    if (lrpc_send(&tctx->kernel_out, &msg))
    {
        fprintf(stderr, "notify_app_tcp_event_newconn: failed to send message\n");
    }

    kick_evfd(tctx->evfd);
}

void notify_app_tcp_conn_open(struct app_ctx_per_thread *tctx, opaque_ptr c, int fd, int32_t status, struct tcp_connection *conn)
{
    lrpc_msg msg = {0};
    struct appin_tcp_conn_open_t *kmsg = (struct appin_tcp_conn_open_t *)msg.data;

    msg.cmd = APPIN_TCP_CONN_OPEN;
    kmsg->opaque_connection = c;
    kmsg->fd = fd;
    kmsg->status = status;
    if (status != -1)
    {
        kmsg->local_ip = conn->local_ip;
        kmsg->local_port = conn->local_port;
    }
    if (status == 1)
    {
        kmsg->rx_buf_size = conn->rx_buf_size;
        kmsg->tx_buf_size = conn->tx_buf_size;
        kmsg->qid = conn->qid;
    }

    if (lrpc_send(&tctx->kernel_out, &msg))
    {
        fprintf(stderr, "notify_app_tcp_conn_open: failed to send message\n");
    }

    kick_evfd(tctx->evfd);
}

void notify_app_tcp_status_bind(struct app_ctx_per_thread *tctx, opaque_ptr c, int fd, int32_t status)
{
    lrpc_msg msg = {0};
    struct appin_tcp_status_t *kmsg = (struct appin_tcp_status_t *)msg.data;

    msg.cmd = APPIN_TCP_STATUS_BIND;
    kmsg->opaque_connection = c;
    kmsg->fd = fd;
    kmsg->status = status;

    if (lrpc_send(&tctx->kernel_out, &msg))
    {
        fprintf(stderr, "notify_app_tcp_status_bind: failed to send message\n");
    }

    kick_evfd(tctx->evfd);
}

void notify_app_tcp_status_listen(struct app_ctx_per_thread *tctx, opaque_ptr l, int fd, int32_t status)
{
    lrpc_msg msg = {0};
    struct appin_tcp_status_t *kmsg = (struct appin_tcp_status_t *)msg.data;

    msg.cmd = APPIN_TCP_STATUS_LISTEN;
    kmsg->opaque_listener = l;
    kmsg->fd = fd;
    kmsg->status = status;

    if (lrpc_send(&tctx->kernel_out, &msg))
    {
        fprintf(stderr, "notify_app_tcp_status_listen: failed to send message\n");
    }

    kick_evfd(tctx->evfd);
}

void notify_app_tcp_status_close(struct app_ctx_per_thread *tctx, opaque_ptr c, int fd, int32_t status)
{
    lrpc_msg msg = {0};
    struct appin_tcp_status_t *kmsg = (struct appin_tcp_status_t *)msg.data;

    msg.cmd = APPIN_TCP_STATUS_CLOSE;
    kmsg->opaque_connection = c;
    kmsg->fd = fd;
    kmsg->status = status;

    if (lrpc_send(&tctx->kernel_out, &msg))
    {
        fprintf(stderr, "notify_app_tcp_status_close: failed to send message\n");
    }

    kick_evfd(tctx->evfd);
}

static struct tcp_connection *tcp_conn_lookup(struct pkt_tcp *p)
{
    std::lock_guard<std::mutex> lock(tcp_connections_lock);
    struct flow_tuple key(ntohl(p->ip.src), ntohs(p->tcp.src), ntohl(p->ip.dest), ntohs(p->tcp.dest));
    auto it = tcp_connections.find(key);

    return it == tcp_connections.end() ? nullptr : it->second;
}

static struct tcp_listener *tcp_listener_lookup(struct pkt_tcp *p)
{
    std::lock_guard<std::mutex> lock(tcp_listeners_lock);
    struct listen_tuple key(ntohl(p->ip.dest), ntohs(p->tcp.dest));

    auto it = tcp_listeners.find(key);

    if (it == tcp_listeners.end())
        return nullptr;

    std::vector<struct tcp_listener *> &listener_list = it->second;

    if (listener_list.empty())
        return nullptr;

    // hash p's 4-tuple to get a index
    uint32_t hash_idx = (ntohl(p->ip.src) ^ ntohl(p->ip.dest) ^ ntohs(p->tcp.src) ^ ntohs(p->tcp.dest)) % listener_list.size();
    return listener_list[hash_idx];
}

static void tcp_listener_accept(struct tcp_listener *l)
{
    struct tcp_connection *c;
    struct backlog_slot *slot;
    struct pkt_tcp *pkt;
    struct tcp_opts opts = {0};
    uint32_t ecn_flags;
    int ret;

    if (!l->pending_conn)
        return;

    c = l->pending_conn;

    assert(l->backlog.size() > 0);

    slot = &l->backlog.front();
    l->backlog.pop_front();

    pkt = (struct pkt_tcp *)slot->pkt;

    ret = parse_tcp_opts(pkt, &opts);

    if (ret || opts.ts == nullptr || opts.mss == nullptr)
    {
        fprintf(stderr, "tcp_listener_accept: failed to parse TCP options\n");
        return;
    }

    c->qid = slot->qid;

    memcpy(c->local_mac, (uint8_t *)pkt->eth.dest.addr, ETH_ALEN);
    memcpy(c->remote_mac, (uint8_t *)pkt->eth.src.addr, ETH_ALEN);
    c->remote_ip = ntohl(pkt->ip.src);
    c->remote_port = ntohs(pkt->tcp.src);
    c->local_ip = etran_nic->_local_ip;
    c->local_port = l->listen_port;

    c->remote_seq = ntohl(pkt->tcp.seqno) + 1;
    c->local_seq = 1;
    c->syn_ts = ntohl(opts.ts->ts_val);

    ecn_flags = TCPH_FLAGS(&pkt->tcp) & (TCP_ECE | TCP_CWR);
    if (ecn_flags == (TCP_ECE | TCP_CWR))
    {
        c->flags |= ECN_ENABLE;
    }

    if (reg_tcp_conn_ebpf(c, true))
    {
        fprintf(stderr, "tcp_listener_accept: failed to register connection\n");
        return;
    }

    reg_tcp_conn_slowpath(c);

    c->status = CONN_WAIT_TX_SYNACK;
    c->listen_fd = l->fd;
    l->pending_conn = nullptr;

    tcp_handshake_list.push_back(c);

#ifdef DEBUG_TCP
    fprintf(stdout, "connection is set to CONN_WAIT_TX_SYNACK\n");
#endif

    return;
}

static void tcp_connection_pkt(struct tcp_connection *c, struct pkt_tcp *p, uint32_t qid, struct tcp_opts *opts)
{
    uint32_t ecn_flags = 0;
    if (c->status == CONN_WAIT_RX_SYNACK)
    {
        c->qid = qid;
        if (tcp_synack_pkt(c, p, opts))
        {
            /* this is not our expected SYN-ACK packet */
            fprintf(stderr, "tcp_synack_pkt() failed\n");
        }
        return;
    }
    if (c->status == CONN_OPEN && ((TCPH_FLAGS(&p->tcp) & ~ecn_flags) == TCP_SYN))
    {
        /* handle re-transmitted SYN for dropped SYN-ACK */
        /* TODO: should only do this if we're still waiting for initial ACK,
         * otherwise we should send a challenge ACK */
        if (opts->ts == nullptr)
        {
            fprintf(stderr, "tcp_connection_pkt: re-transmitted SYN does not have TS "
                            "option\n");
            tcp_conn_put(c);
            return;
        }

        /* send ECN accepting SYN-ACK */
        if (c->flags & ECN_ENABLE)
        {
            ecn_flags = TCP_ECE;
        }

        send_tcp_control(c, TCP_SYN | TCP_ACK | ecn_flags, 1, htonl(opts->ts->ts_val), TCP_MSS);
        return;
    }
    if (c->status == CONN_OPEN && ((TCPH_FLAGS(&p->tcp) & TCP_SYN)))
    {
        /* silently ignore a re-transmited SYN_ACK */
        return;
    }

    if (c->status == CONN_OPEN && ((TCPH_FLAGS(&p->tcp) & TCP_RST)))
    {
#ifdef DEBUG_TCP
        fprintf(stdout, "tcp_connection_pkt: received RST\n");
#endif
        /* notify application that we received RST */
        notify_app_tcp_status_close(c->tctx, c->opaque_connection, c->fd, 0);
        /* close the connection */
        tcp_conn_put(c);
        return;
    }

    if (c->status == CONN_CLOSED && (TCPH_FLAGS(&p->tcp) & TCP_FIN))
    {
        /* silently ignore a FIN for an already closed connection: TODO figure out
         * why necessary*/
        send_tcp_control(c, TCP_ACK, 1, 0, 0);
        return;
    }
}

static void tcp_listener_pkt(struct tcp_listener *l, struct pkt_tcp *p, uint32_t qid, struct tcp_opts *opts)
{
    struct backlog_slot *slot;
    struct pkt_tcp *pkt;
    uint16_t len;

    len = sizeof(p->eth) + ntohs(p->ip.len);

    if ((TCPH_FLAGS(&p->tcp) & ~(TCP_ECE | TCP_CWR)) != TCP_SYN)
    {
        fprintf(stderr, "tcp_listener_pkt: Not a SYN (flags %x)\n", TCPH_FLAGS(&p->tcp));
        // send_tcp_reset(l->tctx->actx, p);
        return;
    }

    if (l->backlog.size() >= l->max_backlog_size)
        return;

    /* make sure we don't already have this 4-tuple */
    for (auto it = l->backlog.begin(); it != l->backlog.end(); it++)
    {
        slot = &(*it);
        pkt = (struct pkt_tcp *)slot->pkt;
        if (ntohl(pkt->ip.src) == ntohl(p->ip.src) && ntohs(pkt->tcp.src) == ntohs(p->tcp.src) &&
            ntohl(pkt->ip.dest) == ntohl(p->ip.dest) && ntohs(pkt->tcp.dest) == ntohs(p->tcp.dest))
        {
            return;
        }
    }

    l->backlog.push_back(backlog_slot((char *)p, len, qid));

    // notify application
    notify_app_tcp_event_newconn(l->tctx, l->opaque_listener, l->fd, ntohl(p->ip.src), ntohs(p->tcp.src));

    if (l->pending_conn)
    {
#ifdef DEBUG_TCP
        fprintf(stdout, "tcp_listener_pkt() calls tcp_listener_accept()\n");
#endif
        tcp_listener_accept(l);
    }
}

static inline void snapshot_cc(struct bpf_cc_snapshot *stats, uint32_t cc_idx)
{
    stats->c_drops = etran_tcp->_tcp_cc_map_mmap->entry[cc_idx].cnt_tx_drops;
    stats->c_acks = etran_tcp->_tcp_cc_map_mmap->entry[cc_idx].cnt_rx_acks;
    stats->c_ackb = etran_tcp->_tcp_cc_map_mmap->entry[cc_idx].cnt_rx_ack_bytes;
    stats->c_ecnb = etran_tcp->_tcp_cc_map_mmap->entry[cc_idx].cnt_rx_ecn_bytes;
    stats->txp = etran_tcp->_tcp_cc_map_mmap->entry[cc_idx].txp;
    stats->rtt = etran_tcp->_tcp_cc_map_mmap->entry[cc_idx].rtt_est;
    // // dump all above stats
    // printf("cc_idx (%u): c_drops = %u, c_acks = %u, c_ackb = %u, c_ecnb = %u, txp = %u, rtt = %u\n",
    //     cc_idx, stats->c_drops, stats->c_acks, stats->c_ackb, stats->c_ecnb, stats->txp, stats->rtt);
}

// convert kbps to Bps
static inline void set_cc_rate(uint32_t cc_idx, uint32_t new_rate)
{
    uint32_t v = new_rate * 1e3 / 8;
    if (v > 3125000000)
        v = 3125000000; // 25Gbps
    etran_tcp->_tcp_cc_map_mmap->entry[cc_idx].rate = v;
}

/**
 * @brief send a dummy packet to modify eBPF state due to timeout
 */
static inline int issue_retransmit(struct tcp_connection *c)
{
    struct app_ctx *actx = c->tctx->actx;
    struct pkt_tcp *p;

    uint32_t remote_ip = c->remote_ip;
    uint16_t remote_port = c->remote_port;
    uint32_t local_ip = c->local_ip;
    uint16_t local_port = c->local_port;

    uint16_t len = sizeof(*p);

    p = (struct pkt_tcp *)calloc(1, len);
    if (!p)
    {
        fprintf(stderr, "send_tcp_control: failed to allocate memory\n");
        return -1;
    }

    p->ip.proto = 0;
    p->ip.src = htonl(local_ip);
    p->ip.dest = htonl(remote_ip);

    p->tcp.src = htons(local_port);
    p->tcp.dest = htons(remote_port);

    /* send packet */
    slow_path_send_tcp(actx, p, len, true, c->qid);

    free(p);

    return 0;
}

static inline void handle_retransmission(struct tcp_connection *c, struct bpf_cc_snapshot *stats, uint64_t curr_tsc)
{
    uint32_t cur_ts = cycles_to_us(curr_tsc);
    uint32_t rtt = (stats->rtt ? stats->rtt : TCP_RTT_INIT);

    /* check for re-transmits */
    if (stats->txp && stats->c_ackb == 0)
    {
        if (c->cnt_tx_pending++ == 0)
        {
            c->ts_tx_pending = cur_ts;
        }
        else if (c->cnt_tx_pending >= REXMIT_INTS &&
                 (cur_ts - c->ts_tx_pending) >= std::max(REXMIT_INTS * rtt, (unsigned int)TCP_RTO_MIN))
        {
#ifdef DEBUG_TCP
// printf("Timeout for connection (%p) to(%u)us, rtt(%u)us, send a dummpy packet to eBPF\n", c, (cur_ts - c->ts_tx_pending), rtt);
#endif
            if (issue_retransmit(c) == 0)
            {
                c->cnt_tx_pending = 0;
                c->cc_rexmits++;
            }
        }
    }
    else
    {
        c->cnt_tx_pending = 0;
    }
}

/**
 * @brief Poll congestion control and timeout events for TCP
 */
void poll_tcp_cc_to(void)
{
    struct tcp_connection *c;
    struct bpf_cc_snapshot stats;
    uint32_t last;
    uint64_t curr_tsc = get_cycles();

    std::list<struct tcp_connection *> to_put;

    next_tcp_cc_to_tsc = UINT64_MAX;
    
    tcp_connections_lock.lock();
    for (auto it = tcp_connections.begin(); it != tcp_connections.end();)
    {
        c = it->second;
        
        /* skip tcp_connection created for listener */
        if (unlikely(c->type == TCP_CONN_TYPE_FAKE)) {
            it++;
            continue;
        }

        /* handshake timeout */
        if (unlikely(c->status != CONN_OPEN))
        {
            if (c->next_timeout_tsc && c->next_timeout_tsc <= get_cycles()) {
                if (c->status == CONN_WAIT_RX_SYNACK && c->syn_attempts <= 3)
                {
                    c->status = CONN_WAIT_TX_SYN;
                    c->syn_attempts++;
                    /* add to tcp_handshake_list again */
                    tcp_handshake_list.push_back(c);
                }
                else
                {
                    if (c->syn_attempts > 3)
                    {
                        printf("Handshake timeout for connection (%p), %d\n", c, c->syn_attempts);
                        to_put.push_back(c);
                    }
                }
            }
            it++;
            continue;
        }

        if (opt_arpc_case == ARPC_CASE_BASIC)
        {
            it++;
            continue;
        }

        next_tcp_cc_to_tsc = std::min(next_tcp_cc_to_tsc, c->cc_last_tsc + us_to_cycles(c->cc_last_rtt * CC_INTERVAL_RTT));

        __u32 t_us = cycles_to_us((curr_tsc - c->cc_last_tsc));
        if (t_us < c->cc_last_rtt * CC_INTERVAL_RTT)
        {
            /* we handle CC event every CC_INTERVAL_RTT RTTs */
            it++;
            continue;
        }
        
        /* snapshot cc */
        snapshot_cc(&stats, c->cc_idx);

        /* calculate difference to last time */
        last = c->cc_last_drops;
        c->cc_last_drops = stats.c_drops;
        stats.c_drops -= last;

        last = c->cc_last_acks;
        c->cc_last_acks = stats.c_acks;
        stats.c_acks -= last;

        last = c->cc_last_ackb;
        c->cc_last_ackb = stats.c_ackb;
        stats.c_ackb -= last;

        last = c->cc_last_ecnb;
        c->cc_last_ecnb = stats.c_ecnb;
        stats.c_ecnb -= last;

        /* Run congestion control only for case3 and the full eTran case. */
        if (opt_arpc_case >= ARPC_CASE_CC && c->algorithm == CC_TIMELY)
        {
            timely_cc(c, &stats, curr_tsc);
            set_cc_rate(c->cc_idx, c->cc_rate);
        }
        else if (opt_arpc_case >= ARPC_CASE_CC && c->algorithm == CC_DCTCP_WND)
        {
            dctcp_wnd_cc(c, &stats, curr_tsc);
            set_cc_rate(c->cc_idx, c->cc_rate);
        }
        else if (opt_arpc_case >= ARPC_CASE_CC && c->algorithm == CC_DCTCP_RATE)
        {
            dctcp_rate_cc(c, &stats, curr_tsc);
            set_cc_rate(c->cc_idx, c->cc_rate);
        }

        if (opt_arpc_case >= ARPC_CASE_RELIABLE)
            handle_retransmission(c, &stats, curr_tsc);

        c->cc_last_tsc = curr_tsc;
        it++;
    }
    tcp_connections_lock.unlock();

    /* release references for handshake failed connections */
    while (!to_put.empty())
    {
        struct tcp_connection *c = to_put.front();
        to_put.pop_front();
        tcp_conn_put(c);
    }

}

int poll_tcp_handshake_events(void)
{
    struct tcp_connection *c;
    int work = 0;
    while (!tcp_handshake_list.empty())
    {
        auto it = tcp_handshake_list.begin();
        c = *it;
        tcp_handshake_list.pop_front();
        switch (c->status)
        {
        case CONN_WAIT_TX_SYN:
            c->status = CONN_WAIT_RX_SYNACK;
            /* arm a TCP handshake timer */
            c->next_timeout_tsc = get_cycles() + us_to_cycles(TCP_HANDSHAKE_TIMEOUT * 1000);
            /* send SYN packet */
            send_tcp_control(c, TCP_SYN | TCP_ECE | TCP_CWR, 1, 0, TCP_MSS);
            work++;
            break;
        case CONN_WAIT_TX_SYNACK:
            c->status = CONN_OPEN;
            /* send SYN-ACK packet */
            if (c->flags & ECN_ENABLE)
                send_tcp_control(c, TCP_SYN | TCP_ACK | TCP_ECE, 1, c->syn_ts, TCP_MSS);
            else
                send_tcp_control(c, TCP_SYN | TCP_ACK, 1, c->syn_ts, TCP_MSS);
            /* notify application to accept() */
            notify_app_tcp_event_accept(c->tctx, c->opaque_connection, c->listen_fd, c->fd, 0, c->rx_buf_size, c->tx_buf_size, c->local_ip, c->remote_ip, c->remote_port, c->qid, !c->listener->backlog.empty());
            work++;
            break;
        default:
            break;
        }
    }
    return work;
}

// external functions
int tcp_listen(struct app_ctx_per_thread *tctx, struct appout_tcp_listen_t *tcp_listen_msg_in)
{
    struct tcp_connection *c;
    struct tcp_listener *listener;
    uint16_t port;
    opaque_ptr opaque_connection = tcp_listen_msg_in->opaque_connection;
    opaque_ptr opaque_listener = tcp_listen_msg_in->opaque_listener;
    unsigned int backlog = tcp_listen_msg_in->backlog;

    c = find_tcp_conn_slowpath(opaque_connection);

    // it seems that no bind() for it
    if (!c)
        return -1;

    // not a connection used for listen
    if (c->type != TCP_CONN_TYPE_FAKE)
        return -1;

    port = c->local_port;

    listener = new tcp_listener();
    if (!listener)
        return -1;

    listener->opaque_listener = opaque_listener;
    listener->tctx = tctx;
    listener->max_backlog_size = backlog;
    listener->listen_port = port;
    listener->pending_conn = nullptr;
    listener->c = c;
    listener->fd = c->fd;

    tctx->listeners.push_back(listener);

    tcp_listeners_lock.lock();
    auto it = tcp_listeners.find(listen_tuple(etran_nic->_local_ip, port));
    if (it != tcp_listeners.end())
    {
        std::vector<tcp_listener *> &listener_list = it->second;
        listener_list.push_back(listener);
    }
    else
    {
        // first listen() on this socket, create a new listener list
        std::vector<tcp_listener *> listener_list = std::vector<tcp_listener *>();
        listener_list.push_back(listener);
        tcp_listeners.insert(std::make_pair(listen_tuple(etran_nic->_local_ip, port), listener_list));
    }
    tcp_listeners_lock.unlock();

    notify_app_tcp_status_listen(tctx, opaque_listener, c->fd, 0);

    return 0;
}

int tcp_accept(struct app_ctx_per_thread *tctx, struct appout_tcp_accept_t *tcp_accept_msg_in)
{
    struct tcp_connection *c;
    struct tcp_listener *listener = nullptr;

    opaque_ptr opaque_listener = tcp_accept_msg_in->opaque_listener;
    opaque_ptr opaque_connection = tcp_accept_msg_in->opaque_connection;
    uint16_t local_port = tcp_accept_msg_in->local_port;
    int newfd = tcp_accept_msg_in->newfd;

    for (auto it = tctx->listeners.begin(); it != tctx->listeners.end(); it++)
    {
        if ((*it)->listen_port == local_port && (*it)->opaque_listener == opaque_listener)
        {
            listener = *it;
            break;
        }
    }

    // no listener found
    if (!listener)
        return -1;

    // already have a pending connection
    if (listener->pending_conn)
        return -1;

    // exceeded max number of connections
    if (tcp_connections.size() > MAX_NR_CONN)
        return -1;

    c = new tcp_connection();
    if (!c)
        return -1;
    c->release = tcp_connection_close;

    c->type = TCP_CONN_TYPE_NORMAL;
    c->tctx = tctx;
    c->listener = listener;
    c->opaque_connection = opaque_connection;
    c->remote_ip = 0;
    c->remote_port = 0;
    c->local_ip = etran_nic->_local_ip;
    c->local_port = listener->listen_port;
    c->rx_buf_size = etran_tcp->_trans_params.tcp.rx_buf_size;
    c->tx_buf_size = etran_tcp->_trans_params.tcp.tx_buf_size;
    c->remote_seq = 0;
    c->local_seq = 0;
    c->status = CONN_WAIT_RX_SYN;
    c->syn_ts = 0;
    c->syn_attempts = 0;
    c->algorithm = CC_NONE;
    c->cc_idx = 0;
    c->cc_last_tsc = 0;
    c->cc_last_rtt = TCP_RTT_INIT;
    c->cc_last_drops = 0;
    c->cc_last_acks = 0;
    c->cc_last_ackb = 0;
    c->cc_last_ecnb = 0;
    c->cc_rate = CC_TIMELY_INIT_RATE;
    c->cc_rexmits = 0;
    c->cc_data = {0};
    c->cnt_tx_pending = 0;
    c->ts_tx_pending = 0;
    c->qid = POISON_32;
    c->flags = 0;
    c->fd = newfd;

    listener->pending_conn = c;

    if (listener->backlog.size() > 0)
    {
#ifdef DEBUG_TCP
        fprintf(stdout, "tcp_accept() calls tcp_listener_accept()\n");
#endif
        tcp_listener_accept(listener);
    }

    // notify application after sending SYN-ACK

    return 0;
}

int tcp_open(struct app_ctx_per_thread *tctx, struct appout_tcp_open_t *tcp_open_msg_in)
{
    struct tcp_connection *c;
    uint16_t local_port = 0;
    int ret;

    opaque_ptr opaque_connection = tcp_open_msg_in->opaque_connection;
    int fd = tcp_open_msg_in->fd;
    uint32_t remote_ip = tcp_open_msg_in->remote_ip;
    uint16_t remote_port = tcp_open_msg_in->remote_port;

    c = find_tcp_conn_slowpath(opaque_connection);

    /* no bind() called for this connection before */
    if (!c)
    {
        c = new tcp_connection();
        if (!c)
            return -1;
        c->release = tcp_connection_close;
        ret = alloc_port();
        if (ret == -1)
        {
            delete c;
            return -1;
        }
        local_port = ret;
        record_port(tctx->actx, local_port, remote_port);
    }
    c->release = tcp_connection_close;

    c->type = TCP_CONN_TYPE_NORMAL;
    c->tctx = tctx;
    c->listener = nullptr;
    c->reuseport = false;
    c->opaque_connection = opaque_connection;
    c->fd = fd;
    c->remote_ip = remote_ip;
    c->remote_port = remote_port;
    c->local_ip = etran_nic->_local_ip;
    if (local_port)
        c->local_port = local_port;
    c->rx_buf_size = etran_tcp->_trans_params.tcp.rx_buf_size;
    c->tx_buf_size = etran_tcp->_trans_params.tcp.tx_buf_size;
    c->status = CONN_WAIT_TX_SYN;
    c->algorithm = CC_NONE;
    c->cc_idx = POISON_32;
    c->cc_last_rtt = TCP_RTT_INIT;
    c->cc_rate = CC_TIMELY_INIT_RATE;
    c->qid = POISON_32;
    c->flags = 0;
    c->remote_seq = 0;
    c->local_seq = 0;

    reg_tcp_conn_slowpath(c);

    tcp_handshake_list.push_back(c);

    notify_app_tcp_conn_open(tctx, c->opaque_connection, c->fd, 0, c);

    return 0;
}

int tcp_bind(struct app_ctx_per_thread *tctx, struct appout_tcp_bind_t *tcp_bind_msg_in)
{
    struct tcp_connection *c;

    opaque_ptr opaque_connection = tcp_bind_msg_in->opaque_connection;
    uint32_t _local_ip = tcp_bind_msg_in->local_ip;
    uint16_t local_port = tcp_bind_msg_in->local_port;
    bool reuseport = tcp_bind_msg_in->reuseport;

    // FIXME
    (void)_local_ip;

    c = find_tcp_conn_slowpath(opaque_connection);
    if (c)
        return -EADDRINUSE;

    c = new tcp_connection();
    if (!c)
        return -ENOMEM;
    c->release = tcp_connection_close;

    if (alloc_port(local_port))
    {
        if (reuseport)
        {
            if (tctx->actx->ports.find(local_port) != tctx->actx->ports.end())
            {
                // ok, this port belongs to this application
            }
            else
            {
                // this port is in use by other applications
                delete c;
                return -EADDRINUSE;
            }
        }
        else
        {
            delete c;
            return -EADDRINUSE;
        }
    }

    c->type = TCP_CONN_TYPE_FAKE;
    c->reuseport = reuseport;
    c->fd = tcp_bind_msg_in->fd;
    /* update owner thread */
    c->tctx = tctx;
    c->local_port = local_port;
    c->remote_port = (uint16_t)opaque_connection;
    c->local_ip = (uint16_t)(opaque_connection >> 32);
    c->remote_ip = (uint16_t)(opaque_connection >> 16);
    c->opaque_connection = opaque_connection;
    c->flags = 0;

    record_port(c->tctx->actx, c->local_port, 0);

    reg_tcp_conn_slowpath(c);

    notify_app_tcp_status_bind(tctx, c->opaque_connection, c->fd, 0);

    return 0;
}

int tcp_packet(struct app_ctx *actx, struct pkt_tcp *p, uint32_t qid)
{
    struct tcp_connection *c;
    struct tcp_listener *l;
    struct tcp_opts opts = {0};
    int ret = 0;

#ifdef DEBUG_TCP
    fprintf(stdout, "tcp_packet()\n");
#endif
    // fprintf(stdout, "tcp_packet()\n");

    if (parse_tcp_opts(p, &opts))
        return -1;

    if ((c = tcp_conn_lookup(p)))
    {
#ifdef DEBUG_TCP
        fprintf(stdout, "A corresponding connection is found\n");
#endif
        tcp_connection_pkt(c, p, qid, &opts);
    }
    else
    {
        if ((l = tcp_listener_lookup(p)))
        {
#ifdef DEBUG_TCP
            fprintf(stdout, "A corresponding listener is found\n");
#endif
            tcp_listener_pkt(l, p, qid, &opts);
        }
        else
        {
#ifdef DEBUG_TCP
            fprintf(stdout, "No connection and listener are found, send RST back\n");
#endif
            fprintf(stdout, "No connection and listener are found, send RST back, %u, %d\n", htons(p->tcp.dest), (TCPH_FLAGS(&p->tcp)));
            ret = -1;
            /* send reset if the packet received wasn't a reset */
            if (!(TCPH_FLAGS(&p->tcp) & TCP_RST))
                send_tcp_reset(actx, p);
        }
    }
    return ret;
}

int tcp_close(struct app_ctx_per_thread *tctx, struct appout_tcp_close_t *tcp_close_msg_in)
{
    struct tcp_connection *c = nullptr;

    opaque_ptr opaque_connection = tcp_close_msg_in->opaque_connection;
    int fd = tcp_close_msg_in->fd;

    c = find_tcp_conn_slowpath(opaque_connection);
    if (!c)
        return -1;

    tcp_conn_put(c);

    notify_app_tcp_status_close(tctx, opaque_connection, fd, 1);

    return 0;
}

void slow_path_send_tcp(struct app_ctx *actx, struct pkt_tcp *tcphdr, uint16_t len, bool to, uint32_t qid)
{
    uint64_t buffer_addr;
    char *pkt;
    struct xdp_desc *desc;
    unsigned int idx_tx = 0;
    uint32_t local_ip = ntohl(tcphdr->ip.src);
    uint16_t local_port = ntohs(tcphdr->tcp.src);
    uint32_t remote_ip = ntohl(tcphdr->ip.dest);
    uint16_t remote_port = ntohs(tcphdr->tcp.dest);

    struct thread_bcache *bc = &actx->iobuffer;

    if (qid == POISON_32)
    {
        struct flow_tuple flow_tuple = {remote_ip, remote_port, local_ip, local_port};
        qid = actx->nic_qid[flow_tuple.hash() % actx->nr_nic_queues];
    }

    struct nic_queue_info *nic_queue = &etran_nic->_nic_queues[qid];

    struct xsk_socket_info *xsk_info = nic_queue->xsk_info;

    // NAPI is too busy, don't bother it anymore for retransmission
    if (to && xsk_prod_nb_free(&xsk_info->tx, XSK_RING_PROD__DEFAULT_NUM_DESCS) < (XSK_RING_PROD__DEFAULT_NUM_DESCS >> 2)) 
        return;

    if (thread_bcache_check(bc, 1) < 1 || xsk_ring_prod__reserve(&xsk_info->tx, 1, &idx_tx) < 1) 
        return;

    buffer_addr = thread_bcache_cons(bc);

    buffer_addr = add_offset_tx_frame(buffer_addr);

    pkt = (char *)xsk_umem__get_data(xsk_info->umem_area, buffer_addr);

    tcp_txmeta_clear_all(xsk_info->umem_area, buffer_addr);

    tcp_txmeta_from_slowpath(xsk_info->umem_area, buffer_addr, 1);

    if (to)
        tcp_txmeta_flag(xsk_info->umem_area, buffer_addr, FLAG_TO);

    memcpy(pkt, tcphdr, len);

    desc = xsk_ring_prod__tx_desc(&xsk_info->tx, idx_tx);
    desc->addr = buffer_addr;
    desc->len = len;
    desc->options = 0;

    xsk_ring_prod__submit(&xsk_info->tx, 1);

    kick_tx(xsk_info);

    return;
}

void process_tcp_cmd(struct app_ctx_per_thread *tctx, lrpc_msg *msg_in)
{
    struct appout_tcp_listen_t *tcp_listen_msg_in;
    struct appout_tcp_open_t *tcp_open_msg_in;
    struct appout_tcp_bind_t *tcp_bind_msg_in;
    struct appout_tcp_close_t *tcp_close_msg_in;
    struct appout_tcp_accept_t *tcp_accept_msg_in;
    switch (msg_in->cmd)
    {
    case APPOUT_TCP_OPEN:
        tcp_open_msg_in = (struct appout_tcp_open_t *)msg_in->data;
        if (tcp_open(tctx, tcp_open_msg_in))
            notify_app_tcp_conn_open(tctx, tcp_open_msg_in->opaque_connection, tcp_open_msg_in->fd, -1, nullptr);
        break;
    case APPOUT_TCP_BIND:
        tcp_bind_msg_in = (struct appout_tcp_bind_t *)msg_in->data;
        if (tcp_bind(tctx, tcp_bind_msg_in))
            notify_app_tcp_status_bind(tctx, tcp_bind_msg_in->opaque_connection, tcp_bind_msg_in->fd, -1);
        break;
    case APPOUT_TCP_LISTEN:
        tcp_listen_msg_in = (struct appout_tcp_listen_t *)msg_in->data;
        if (tcp_listen(tctx, tcp_listen_msg_in))
            notify_app_tcp_status_listen(tctx, tcp_listen_msg_in->opaque_listener, tcp_listen_msg_in->fd, -1);
        break;
    case APPOUT_TCP_ACCEPT:
        tcp_accept_msg_in = (struct appout_tcp_accept_t *)msg_in->data;
        if (tcp_accept(tctx, tcp_accept_msg_in))
            notify_app_tcp_event_accept(tctx, tcp_accept_msg_in->opaque_connection, tcp_accept_msg_in->fd, tcp_accept_msg_in->newfd, -1, 0, 0, 0, 0, 0, 0, 0);
        break;
    case APPOUT_TCP_CLOSE:
        tcp_close_msg_in = (struct appout_tcp_close_t *)msg_in->data;
        if (tcp_close(tctx, tcp_close_msg_in))
            notify_app_tcp_status_close(tctx, tcp_close_msg_in->opaque_connection, tcp_close_msg_in->fd, -1);
        break;
    }
}

static void free_connections(struct app_ctx *actx)
{
    struct tcp_connection *c;
    std::list<struct tcp_connection *> to_put;

    tcp_connections_lock.lock();
    for (auto it = tcp_connections.begin(); it != tcp_connections.end(); it++)
    {
        c = it->second;
        if (c->tctx->actx != actx)
            continue;
        to_put.push_back(c);
    }
    tcp_connections_lock.unlock();

    /* avoid holding the tcp_connections_lock */
    while (!to_put.empty())
    {
        c = to_put.front();
        to_put.pop_front();
        tcp_conn_put(c);
    }
}

static void free_listeners(struct app_ctx *actx)
{
    struct tcp_listener *listener;
    std::lock_guard<std::mutex> lock(tcp_listeners_lock);
    auto _it = tcp_listeners.begin();
#ifdef DEBUG_TCP
    int work = 0;
#endif

    while (_it != tcp_listeners.end())
    {
        bool del = false;
        std::vector<struct tcp_listener *> &listener_list = _it->second;
        auto it = listener_list.begin();
        while (it != listener_list.end())
        {
            listener = *it;
            if (listener->tctx->actx == actx)
            {
                listener->backlog.clear();
                if (listener->pending_conn)
                    delete listener->pending_conn;

                delete listener;
                it = listener_list.erase(it);
#ifdef DEBUG_TCP
                work++;
#endif
                del = true;
            }
            else
                it++;
        }

        if (del)
            _it = tcp_listeners.erase(_it);
        else
            _it++;
    }
#ifdef DEBUG_TCP
    if (work)
        fprintf(stdout, "Free %d listeners for application:%d\n", work, actx->pid);
#endif
}

void free_tcp_resources(struct app_ctx *actx)
{
    /* traverse tcp_connections and tcp_listeners and free them */
    free_connections(actx);
    free_listeners(actx);
}