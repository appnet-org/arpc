#pragma once

#include <string.h>
#include <stdint.h>
#include <functional>
#include <list>

#include <base/kref.h>

#include <tran_def/tcp.h>
#include <intf/intf_ebpf.h>
#include <runtime/ebpf_if.h>

// Congestion Control Algorithms Selection
// #define TIMELY
#define DCTCP // window-based

// Maximum link bandwidth (kbps)
#define MAX_LINK_BANDWIDTH 25000000

// Low threshold (us)
#define CC_TIMELY_TLOW 30
// High threshold (us)
#define CC_TIMELY_THIGH 150
// Additive increment step (kbps)
#define CC_TIMELY_STEP 10000
// Multiplicative decrement factor
#define CC_TIMELY_BETA (0.8 * __UINT32_MAX__)
// EWMA weight for rtt diff
#define CC_TIMELY_ALPHA (0.02 * __UINT32_MAX__)
// Minimum RTT (us)
#define CC_TIMELY_MIN_RTT 11
// Minimum rate (kbps)
#define CC_TIMELY_MIN_RATE 500000
#define CC_TIMELY_INIT_RATE CC_TIMELY_MIN_RATE

#define CC_DCTCP_MINBYTES 64800
#define CC_DCTCP_WEIGHT (UINT32_MAX / 16)
#define CC_DCTCP_MIMD 0
#define CC_DCTCP_STEP 10000
#define CC_DCTCP_MIN 500000

// Number of intervals without ACKs before retransmit
#define REXMIT_INTS 4
#define TCP_RTT_INIT 50
#define TCP_RTO_MIN 4000

const unsigned int MAX_NR_CONN = 65536;
const uint16_t TCP_SYN_RETRY = 3;
/* How many RTTs */
const uint64_t CC_INTERVAL_RTT = 2;
const uint64_t CC_INTERVAL_US = 200;

/** Type of timeout */
enum timeout_type
{
    /** TCP handshake sent */
    TO_TCP_HANDSHAKE,
    /** TCP connection closed, ready to free */
    TO_TCP_CLOSED,
    TO_TCP_FAILED,
};

struct listen_tuple
{
    uint32_t local_ip;
    uint16_t local_port;

    // constructor
    listen_tuple(uint32_t local_ip, uint16_t local_port)
        : local_ip(local_ip), local_port(local_port) {}

    uint32_t hash() const
    {
        return (((std::hash<uint32_t>()(local_ip) << 1)) >> 1) ^ (std::hash<uint16_t>()(local_port));
    }
};

struct flow_tuple
{
    uint32_t remote_ip;
    uint16_t remote_port;
    uint32_t local_ip;
    uint16_t local_port;

    // constructor
    flow_tuple(uint32_t remote_ip, uint16_t remote_port, uint32_t local_ip, uint16_t local_port)
        : remote_ip(remote_ip), remote_port(remote_port), local_ip(local_ip), local_port(local_port) {}

    uint32_t hash() const
    {
        return ((std::hash<uint32_t>()(remote_ip) ^ (std::hash<uint32_t>()(local_ip) << 1)) >> 1) ^
               (std::hash<uint16_t>()(remote_port) << 1) ^ (std::hash<uint16_t>()(local_port));
    }
};

/** TCP connection state machine state. */
enum connection_status
{

    /// *********************tcp_accept()*********************
    // it is waiting for receiving SYN packet
    CONN_WAIT_RX_SYN,

    // it has already received SYN packet,
    // it is waiting for sending SYN-ACK packet
    CONN_WAIT_TX_SYNACK,
    /// *********************tcp_accept()*********************

    /// **********************tcp_open()**********************
    // it is waiting for sending SYN packet
    CONN_WAIT_TX_SYN,

    // it has already sent SYN packet,
    // it is waiting for receiving SYN-ACK packet
    CONN_WAIT_RX_SYNACK,
    /// **********************tcp_open()**********************

    // this connection is opened
    CONN_OPEN,

    // this connection is closed
    CONN_CLOSED,

    // this conenction is failed
    CONN_FAILED,
};

struct backlog_slot
{
    char pkt[256];
    uint32_t len;
    uint32_t qid;

    backlog_slot(char *pkt, uint32_t len, uint32_t qid) : len(len), qid(qid)
    {
        memcpy(this->pkt, pkt, len);
    }
};

struct tcp_listener
{
    struct app_ctx_per_thread *tctx;

    opaque_ptr opaque_listener;

    unsigned int max_backlog_size;

    std::list<struct backlog_slot> backlog;

    uint16_t listen_port;

    struct tcp_connection *pending_conn;

    struct tcp_connection *c;

    int fd;

    tcp_listener() {}
};

enum cc_algorithm
{
    CC_NONE,
    CC_TIMELY,
    CC_DCTCP_RATE,
    CC_DCTCP_WND,
};

/** Convert window in bytes to kbps */
// window: Bytes
// rtt: microseconds
static inline uint32_t window_to_rate(uint32_t window, uint32_t rtt)
{
    uint64_t time, rate;

    /* calculate how long [ns] it will take to send a window size's worth */
    time = (((uint64_t)window * 8 * 1000) / (MAX_LINK_BANDWIDTH / 1e6)) / 1000;

    /* we won't be able to send more than a window per rtt */
    if (time < rtt * 1000)
        time = rtt * 1000;

    /* convert time to rate */
    assert(time != 0);
    rate = ((uint64_t)window * 8 * 1000000) / time;
    if (rate > UINT32_MAX)
        rate = UINT32_MAX;

    return rate;
}

struct cc_dctcp_wnd
{
    /** Rate of ECN bits received. */
    uint32_t ecn_rate;
    /** Congestion window. */
    uint32_t window;
    /** Flag indicating whether flow is in slow start. */
    int slowstart;
};

struct cc_dctcp_rate
{
    /** Unprocessed acks */
    uint32_t unproc_acks;
    /** Unprocessed ack bytes */
    uint32_t unproc_ackb;
    /** Unprocessed ECN ack bytes */
    uint32_t unproc_ecnb;
    /** Unprocessed drops */
    uint32_t unproc_drops;
    /** Last timestamp (us). */
    uint32_t last_ts;

    /** Rate of ECN bits received. */
    uint32_t ecn_rate;
    /** Actual rate. */
    uint32_t act_rate;
    /** Flag indicating whether flow is in slow start. */
    int slowstart;
};

struct cc_timely
{
    /** Previous RTT. */
    uint32_t rtt_prev;
    /** RTT gradient. */
    int32_t rtt_diff;
    /** HAI counter. */
    uint32_t hai_cnt;
    /** Actual rate. */
    uint32_t act_rate;
    /** Last timestamp (us). */
    uint32_t last_ts;
    /** Flag indicating whether flow is in slow start. */
    int slowstart;
};

struct bpf_cc_snapshot
{
    /** Number of dropped segments */
    __u16 c_drops;
    /** Number of ACKs received */
    __u16 c_acks;
    /** Acknowledged bytes */
    __u32 c_ackb;
    /** Number of ACKd bytes with ECN marks */
    __u32 c_ecnb;
    /** Has pending data in transmit buffer */
    bool txp;
    /** Current rtt estimate */
    __u32 rtt;
};

enum tcp_connection_type
{
    TCP_CONN_TYPE_NORMAL = 0,
    /* used by listen() */
    TCP_CONN_TYPE_FAKE = 1,
};

struct tcp_connection
{

    enum tcp_connection_type type;

    struct app_ctx_per_thread *tctx;

    // if nullptr, this connection is created by open()
    struct tcp_listener *listener;
    int listen_fd;

    bool reuseport;

    opaque_ptr opaque_connection;

    int fd;

    /* Address information */
    __u8 local_mac[ETH_ALEN];
    __u8 remote_mac[ETH_ALEN];
    uint32_t remote_ip;
    uint16_t remote_port;
    uint32_t local_ip;
    uint16_t local_port;

    uint32_t rx_buf_size;
    uint32_t tx_buf_size;

    /** Peer sequence number. */
    uint32_t remote_seq;
    /** Local sequence number. */
    uint32_t local_seq;

    enum connection_status status;
    /** Timestamp received with SYN/SYN-ACK packet */
    uint32_t syn_ts;

    /* Timeouts */
    uint16_t syn_attempts;

    /* Congestion control */
    enum cc_algorithm algorithm;
    /* Congestion control */
    /* CC map index */
    uint32_t cc_idx;
    /** Timestamp when control loop ran last (cycles) */
    uint64_t cc_last_tsc;
    /** Last rtt estimate */
    uint32_t cc_last_rtt;
    /** Number of dropped segments */
    uint16_t cc_last_drops;
    /** Number of ACKs received */
    uint16_t cc_last_acks;
    /** Acknowledged bytes */
    uint32_t cc_last_ackb;
    /** Number of ACKd bytes with ECN marks */
    uint32_t cc_last_ecnb;
    /** Congestion rate limit (kbps). */
    uint32_t cc_rate;
    /** Had retransmits. */
    uint32_t cc_rexmits;

    union
    {
        struct cc_timely timely;
        struct cc_dctcp_rate dctcp_rate;
        struct cc_dctcp_wnd dctcp_wnd;
    } cc_data;
    /** #control intervals with data in tx buffer but no ACKs */
    uint32_t cnt_tx_pending;
    /** Timestamp when flow was first not moving */
    uint32_t ts_tx_pending;

    uint32_t qid;
    uint32_t flags;
#define ECN_ENABLE (1 << 0)

    uint64_t next_timeout_tsc;

    struct kref ref;

    void (*release)(struct kref *ref);

    tcp_connection()
    {
        memset(this, 0, sizeof(*this));
        kref_init(&ref);
    }

    ~tcp_connection()
    {
    }
};

struct listen_tuple_hash
{
    std::size_t operator()(const listen_tuple &k) const
    {
        return (((std::hash<uint32_t>()(k.local_ip) << 1)) >> 1) ^ (std::hash<uint16_t>()(k.local_port));
    }
};

struct listen_tuple_equal
{
    bool operator()(const listen_tuple &lhs, const listen_tuple &rhs) const
    {
        return lhs.local_ip == rhs.local_ip && lhs.local_port == rhs.local_port;
    }
};

struct flow_tuple_hash
{
    std::size_t operator()(const flow_tuple &k) const
    {
        return ((std::hash<uint32_t>()(k.remote_ip) ^ (std::hash<uint32_t>()(k.local_ip) << 1)) >> 1) ^
               (std::hash<uint16_t>()(k.remote_port) << 1) ^ (std::hash<uint16_t>()(k.local_port));
    }
};

struct flow_tuple_equal
{
    bool operator()(const flow_tuple &lhs, const flow_tuple &rhs) const
    {
        return lhs.remote_ip == rhs.remote_ip && lhs.remote_port == rhs.remote_port && lhs.local_ip == rhs.local_ip &&
               lhs.local_port == rhs.local_port;
    }
};

// external functions
int poll_tcp_handshake_events(void);

void poll_tcp_cc_to(void);

int tcp_bind(struct app_ctx_per_thread *tctx, struct appout_tcp_bind_t *tcp_bind_msg_in);

int tcp_listen(struct app_ctx_per_thread *tctx, struct appout_tcp_listen_t *tcp_listen_msg_in);

int tcp_accept(struct app_ctx_per_thread *tctx, opaque_ptr opaque_listener, opaque_ptr opaque_connection, uint16_t local_port);

int tcp_open(struct app_ctx_per_thread *tctx, struct appout_tcp_open_t *tcp_open_msg_in);

int tcp_close(struct app_ctx_per_thread *tctx, struct appout_tcp_close_t *tcp_close_msg_in);

int tcp_packet(struct app_ctx *actx, struct pkt_tcp *p, uint32_t qid);

void *tcp_timer_loop(void *arg);
void process_tcp_cmd(struct app_ctx_per_thread *tctx, lrpc_msg *msg);

void free_tcp_resources(struct app_ctx *actx);
