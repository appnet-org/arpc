/**
 * Common interface for application and microkernel
 */

#pragma once

#include <netinet/in.h>

#include <xskbp/xsk_buffer_pool.h>

// FIXME 
#define TCP_WND_SCALE 3

#define OPAQUE(x) reinterpret_cast<uintptr_t>(x)
#define OPAQUE_PTR(type, x) reinterpret_cast<type *>(x)
#define opaque_ptr uintptr_t

#define SHM_BP_PREFIX "BufferPool_"
#define SHM_UMEM_PREFIX "UMEM_"
#define SHM_LRPC_PREFIX "LRPC_"

struct eTran_netaddr
{
    uint32_t ip;
    uint16_t port;
} __attribute__((packed));

enum req_type {
    REG,
    UNREG,
};

/* request format used for application request */
struct register_request {
    /* request type */
    enum req_type type;
    /* transport protocol application wants to use */
    int proto;
    /* the maximum number of nic queues application wants to use */
    unsigned int nr_nic_queues;
    /* the maximum number of threads application wants to use */
    unsigned int nr_app_threads;
    /* networ address consists of IPv4 address and port number */
    struct eTran_netaddr addr;
} __attribute__((packed));

enum resp_type {
    REG_XSK_FD,
    REG_EVENT_FD,
    REG_FAIL,
    REG_DONE,
    UNKNWON,
};

/* response format used for control path response */
struct register_response {
    enum resp_type type;

    size_t shm_bp_size;

    size_t shm_umem_size;

    size_t shm_lrpc_size;

    struct buffer_pool_params bp_params;

    int ifindex;

    unsigned int nic_qid[MAX_NIC_QUEUES];
} __attribute__((packed));

#define MAX_MSG_SIZE 56
/********** lrpc message types for TCP **********/
/*** Application ------> Kernel ***/
enum LRPC_APPOUT_TCP_TYPE
{
    // bind()
    APPOUT_TCP_BIND,
    // connect()
    APPOUT_TCP_OPEN,
    // listen()
    APPOUT_TCP_LISTEN,
    // accept()
    APPOUT_TCP_ACCEPT,
    // close()
    APPOUT_TCP_CLOSE,
    
    APPOUT_TCP_MAX,
};

// APPOUT_TCP_BIND
struct appout_tcp_bind_t
{
    opaque_ptr opaque_connection;
    int fd;
    bool reuseport;
    uint32_t local_ip;
    uint16_t local_port;
} __attribute__((packed));
static_assert(sizeof(struct appout_tcp_bind_t) <= MAX_MSG_SIZE, "struct appout_tcp_bind_t too large");

// APPOUT_TCP_OPEN
struct appout_tcp_open_t {
    opaque_ptr opaque_connection;
    int fd;
    uint32_t remote_ip;
    uint16_t remote_port;
} __attribute__((packed));
static_assert(sizeof(struct appout_tcp_open_t) <= MAX_MSG_SIZE, "struct appout_tcp_open_t too large");

// APPOUT_TCP_LISTEN
struct appout_tcp_listen_t {
    opaque_ptr opaque_connection;
    opaque_ptr opaque_listener;
    int fd;
    unsigned int backlog;
} __attribute__((packed));
static_assert(sizeof(struct appout_tcp_listen_t) <= MAX_MSG_SIZE, "struct appout_tcp_listen_t too large");

// APPOUT_TCP_ACCEPT
struct appout_tcp_accept_t {
    unsigned int tid;
    opaque_ptr opaque_listener;
    opaque_ptr opaque_connection;
    int fd;
    int newfd;
    uint16_t local_port;
} __attribute__((packed));
static_assert(sizeof(struct appout_tcp_accept_t) <= MAX_MSG_SIZE, "struct appout_tcp_accept_t too large");

// APPOUT_TCP_CLOSE
struct appout_tcp_close_t {
    opaque_ptr opaque_connection;
    int fd;
    uint32_t remote_ip;
    uint32_t local_ip;
    uint16_t remote_port;
    uint16_t local_port;
} __attribute__((packed));
static_assert(sizeof(struct appout_tcp_close_t) <= MAX_MSG_SIZE, "struct appout_tcp_close_t too large");

/*** Kernel ------> Application ***/
enum LRPC_APPIN_TCP_TYPE {   
    // listen on a new connection
    APPIN_TCP_EVENT_NEWCONN,
    
    // response to accept(), success or not and extra information
    APPIN_TCP_CONN_ACCEPT,

    // response to open(), success or not and extra information
    APPIN_TCP_CONN_OPEN,

    // response to bind(), success or not
    APPIN_TCP_STATUS_BIND,

    // response to listen(), success or not
    APPIN_TCP_STATUS_LISTEN,
    
    // response to close(), success or not
    // notify application that the connection is closed
    APPIN_TCP_STATUS_CLOSE,
    
    APPIN_TCP_MAX,
};

// APPIN_TCP_EVENT_NEWCONN
struct appin_tcp_event_newconn_t {
    opaque_ptr opaque_listener;
    int fd;
    uint32_t remote_ip;
    uint16_t remote_port;
} __attribute__((packed));
static_assert(sizeof(struct appin_tcp_event_newconn_t) <= MAX_MSG_SIZE, "struct appin_tcp_event_newconn_t too large");

// APPIN_TCP_CONN_ACCEPT
struct appin_tcp_conn_accept_t {
    opaque_ptr opaque_connection;
    int fd;
    int newfd;
    int32_t  status;
    bool backlog;
    uint32_t rx_buf_size;
    uint32_t tx_buf_size;
    uint32_t local_ip;
    uint32_t remote_ip;
    uint16_t remote_port;
    uint32_t qid;
} __attribute__((packed));
static_assert(sizeof(struct appin_tcp_conn_accept_t) <= MAX_MSG_SIZE, "struct appin_tcp_conn_accept_t too large");

// APPIN_TCP_CONN_OPEN
struct appin_tcp_conn_open_t {
    opaque_ptr opaque_connection;
    int fd;
    int32_t  status;
    uint32_t rx_buf_size;
    uint32_t tx_buf_size;
    uint32_t local_ip;
    uint16_t local_port;
    uint32_t qid;
} __attribute__((packed));

// APPIN_TCP_STATUS_BIND, APPIN_TCP_STATUS_LISTEN and APPIN_TCP_STATUS_CLOSE
struct appin_tcp_status_t {
    union {
        opaque_ptr opaque_listener;
        opaque_ptr opaque_connection;
    };
    int fd;
    int32_t status;
} __attribute__((packed));
static_assert(sizeof(struct appin_tcp_status_t) <= MAX_MSG_SIZE, "struct appin_tcp_status_t too large");

/********** lrpc message types for Homa **********/
/*** Application ------> Kernel ***/
enum LRPC_APPOUT_HOMA_TYPE
{
    // bind()
    APPOUT_HOMA_BIND = APPOUT_TCP_MAX,
    // close()
    APPOUT_HOMA_CLOSE,
};

// APPOUT_HOMA_BIND
struct appout_homa_bind_t
{
    opaque_ptr opaque_socket;
    int fd;
    uint32_t local_ip;
    uint16_t local_port;
} __attribute__((packed));
static_assert(sizeof(struct appout_homa_bind_t) <= MAX_MSG_SIZE, "struct appout_homa_bind_t too large");

// APPOUT_HOMA_CLOSE
struct appout_homa_close_t {
    opaque_ptr opaque_socket;
    int fd;
} __attribute__((packed));
static_assert(sizeof(struct appout_homa_close_t) <= MAX_MSG_SIZE, "struct appout_homa_close_t too large");

/*** Kernel ------> Application ***/
enum LRPC_APPIN_HOMA_TYPE {   
    // response to bind(), success or not
    APPIN_HOMA_STATUS_BIND = APPIN_TCP_MAX,
    
    // response to close(), success or not
    // notify application that the socket is closed
    APPIN_HOMA_STATUS_CLOSE,
};

// APPIN_HOMA_STATUS_BIND, APPIN_HOMA_STATUS_CLOSE
struct appin_homa_status_t {
    opaque_ptr opaque_socket;
    int fd;
    int32_t status;
} __attribute__((packed));
static_assert(sizeof(struct appin_homa_status_t) <= MAX_MSG_SIZE, "struct appin_homa_status_t too large");