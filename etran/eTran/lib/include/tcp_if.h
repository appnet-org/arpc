#pragma once

#include <base/mem_pool.h>

#include <list>
#include <cstring>

// TODO: move this macro
#define POISON_64 __UINT64_MAX__

// 1460 - 12 (Timestamp option)
constexpr unsigned int TCP_MSS_W_TS = 1448;
// timestamp option size
constexpr unsigned int TS_OPT_SIZE = 12;

struct eTran_tcp_flow_tuple {
    uint32_t remote_ip;
    uint16_t remote_port;
    uint32_t local_ip;
    uint16_t local_port;

    // constructor
    eTran_tcp_flow_tuple(uint32_t remote_ip, uint16_t remote_port, uint32_t local_ip, uint16_t local_port)
        : remote_ip(remote_ip), remote_port(remote_port), local_ip(local_ip), local_port(local_port) {}
    
    uint32_t hash() const
    {
        return ((std::hash<uint32_t>()(remote_ip) ^ (std::hash<uint32_t>()(local_ip) << 1)) >> 1) ^
               (std::hash<uint16_t>()(remote_port) << 1) ^ (std::hash<uint16_t>()(local_port));
    }
};

struct eTran_tcp_flow_tuple_hash
{
    std::size_t operator()(const eTran_tcp_flow_tuple &k) const
    {
        return ((std::hash<uint32_t>()(k.remote_ip) ^ (std::hash<uint32_t>()(k.local_ip) << 1)) >> 1) ^
               (std::hash<uint16_t>()(k.remote_port) << 1) ^ (std::hash<uint16_t>()(k.local_port));
    }
};

struct eTran_tcp_flow_tuple_equal
{
    bool operator()(const eTran_tcp_flow_tuple &lhs, const eTran_tcp_flow_tuple &rhs) const
    {
        return lhs.remote_ip == rhs.remote_ip && lhs.remote_port == rhs.remote_port && lhs.local_ip == rhs.local_ip &&
               lhs.local_port == rhs.local_port;
    }
};

struct eTrantcp_listener {
    std::list<struct tcp_connection *> conns;

    uint16_t local_port;

    struct eTran_socket_t *s;
};
enum conn_status {
  CONN_CLOSED,
  CONN_OPEN_REQUESTED,
  CONN_ACCEPT_REQUESTED,
  CONN_BIND_REQUESTED,
  CONN_OPEN,
  CONN_CLOSE_REQUESTED,
};

struct eTrantcp_connection {
    
    /** pointer to next new byte to be received */
    uint32_t rxb_head;
    /** number of received but not yet freed bytes (behind head). */
    uint32_t rxb_used;
    /** pending rx bump to fast path */
    uint32_t rxb_bump;
    
    /** pointer to next byte to be sent */
    uint32_t txb_head;
    /** number of sent but not yet acked bytes (behind head) */
    uint32_t txb_sent;
    /** number of allocated but not yet sent bytes (after head) */
    uint32_t txb_allocated;

    uint32_t local_ip;
    uint32_t remote_ip;
    uint16_t local_port;
    uint16_t remote_port;

    uint32_t rx_buf_size;
    uint32_t tx_buf_size;

    // how many bytes can we submit to AF_XDP
    uint32_t xsk_budget;

    uint32_t qid;

    enum conn_status status;

    uint32_t pending_free_bytes;

    std::list<std::pair<uint64_t, uint32_t> > unack_tx_addrs;

    std::list<std::pair<uint64_t, char *> > rx_addrs;
    std::list<std::pair<uint64_t, char *> > ooo_rx_addrs;

    bool in_rx_bump_pending;

    bool force_rx_bump;

    // socket
    struct eTran_socket_t *s;

    bool try_accept_done;

    eTrantcp_connection() {
        rxb_head = 0;
        rxb_used = 0;
        rxb_bump = 0;
        txb_head = 0;
        txb_sent = 0;
        txb_allocated = 0;
        local_ip = 0;
        remote_ip = 0;
        local_port = 0;
        remote_port = 0;
        rx_buf_size = 0;
        tx_buf_size = 0;
        xsk_budget = 0;
        qid = 0;
        status = CONN_CLOSED;
        in_rx_bump_pending = false;
        force_rx_bump = false;
        s = nullptr;
        try_accept_done = false;
    }
    
    ~eTrantcp_connection() {  
    }
};

enum eTrantcp_event_type {
    ETRANTCP_EV_LISTEN_OPEN,
    ETRANTCP_EV_LISTEN_NEWCONN,
    ETRANTCP_EV_LISTEN_ACCEPT,

    ETRANTCP_EV_CONN_OPEN,
    ETRANTCP_EV_CONN_BIND,
    ETRANTCP_EV_CONN_CLOSE,
    ETRANTCP_EV_CONN_RECVED,
    ETRANTCP_EV_CONN_SENDBUF,
};

struct eTrantcp_event {
    enum eTrantcp_event_type type;

    union {
        struct {
            int16_t status;
            struct eTrantcp_listener *listener;
            int fd;
        } listen;

        struct {
            uint16_t remote_port;
            uint32_t remote_ip;
            struct eTrantcp_listener *listener;
            int fd;
        } newconn;

        struct {
            int16_t status;
            struct eTrantcp_connection *conn;
            bool backlog;
            int fd;
            int newfd;
        } accept;

        struct {
            int16_t status;
            struct eTrantcp_connection *conn;
            int fd;
        } open;

        struct {
            int16_t status;
            struct eTrantcp_connection *conn;
            int fd;
        } bind;
        
        struct {
            int16_t status;
            struct eTrantcp_connection *conn;
            int fd;
        } close;

        struct {
            struct eTrantcp_connection *conn;
            int fd;
        } recv;

        struct {
            struct eTrantcp_connection *conn;
            int fd;
        } send;

    } ev;
};