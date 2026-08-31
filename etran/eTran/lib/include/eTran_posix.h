#pragma once

#include <app_if.h>
#include <xsk_if.h>
#include <tcp_if.h>
#include <homa_if.h>
#include <intf/intf.h>

#include "eTran_common.h"

#define MAX_FD 1024 * 1024

#define CONN_AFFINITY

#define BATCH_IO_THRESHOLD 16384 // bytes

// socket.cc
extern int eTran_socket(int domain, int type, int protocol);
extern int eTran_connect(int sockfd, const struct sockaddr *addr, socklen_t addrlen);
extern int eTran_bind(int sockfd, const struct sockaddr *addr, socklen_t addrlen);
extern int eTran_listen(int sockfd, int backlog);
extern int eTran_accept(int sockfd, struct sockaddr *addr, socklen_t *addrlen);
extern int eTran_close(int sockfd);
extern ssize_t eTran_read(int fd, void *buf, size_t count);
extern ssize_t eTran_write(int fd, const void *buf, size_t count);
extern int eTran_setsockopt(int sockfd, int level, int optname, const void *optval, socklen_t optlen);
extern int eTran_getsockopt(int sockfd, int level, int optname, void *optval, socklen_t *optlen);
extern int eTran_fcntl(int fd, int cmd, int flags);
extern int eTran_epoll_create1(int flags);
extern int eTran_epoll_ctl(int epfd, int op, int fd,
    struct epoll_event *event);
extern int eTran_epoll_wait(int epfd, struct epoll_event *events,
    int maxevents, int timeout);
extern int eTran_select(int nfds, fd_set *readfds, fd_set *writefds,
    fd_set *exceptfds, struct timeval *timeout);

// TCP Control path API
int eTran_tcp_poll_events(struct app_ctx_per_thread *tctx, struct eTrantcp_event *events, int maxevents, int timeout);
int eTran_tcp_open(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, int fd, uint32_t remote_ip, uint16_t remote_port);
int eTran_tcp_bind(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, int fd, uint32_t local_ip, uint32_t local_port, bool reuseport);
int eTran_tcp_listen(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, struct eTrantcp_listener *listener, int fd, uint16_t port, uint32_t backlog);
int eTran_tcp_accept(struct app_ctx_per_thread *tctx, struct eTrantcp_listener *listener, struct eTrantcp_connection *conn, int fd, int newfd);
int eTran_tcp_close(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, int fd);

// Homa Control path API
int eTran_homa_poll_events(struct app_ctx_per_thread *tctx, struct eTranhoma_event *events, int maxevents, int timeout);
int eTran_homa_bind(struct app_ctx_per_thread *tctx, struct eTranhoma_socket *socket, int fd, uint32_t local_ip, uint32_t local_port);
int eTran_homa_close(struct app_ctx_per_thread *tctx, struct eTranhoma_socket *socket, int fd);

void tcp_flow_tx_segmentation_zc(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, const void *buf, size_t len);
size_t tcp_flow_tx_segmentation_zc_retransmission(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, size_t len);

static inline void dma(void *dst, void *src, size_t len)
{
    memcpy(dst, src, len);
}

/**
 * @brief return how many bytes can be submitted to AF_XDP
 */
static inline unsigned int xsk_bytes_avail(struct eTrantcp_connection *conn)
{
    return conn->xsk_budget > conn->txb_sent ? conn->xsk_budget - conn->txb_sent : 0;
}

/**
 * @brief return how many available bytes in the transmit buffer
 */
static inline size_t txb_bytes_avail(struct eTrantcp_connection *conn)
{
    return std::min(conn->tx_buf_size - conn->txb_sent - conn->txb_allocated, xsk_bytes_avail(conn));
}

/**
 * @brief submit data in the connection's tx buffer to AF_XDP for segmentation and transmission
 * @param tctx the per-thread context
 * @param conn the connection
 * @param buf the data to be submitted
 * @param len the length of data to be submitted
 * @return 0 on success, -EINVAL on failure
 */
static inline int eTran_tcp_tx_submit_zc(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, const void *buf, size_t len)
{
    if (unlikely(conn->status != CONN_OPEN))
    {
        fprintf(stderr, "eTran_tcp_tx_submit_zc(): conn->status != CONN_OPEN\n");
        return -EINVAL;
    }

    if (unlikely(conn->txb_allocated < len))
    {
        fprintf(stderr, "eTran_tcp_tx_submit_zc(): (%p), txb_allocated(%u) < len(%lu)\n", conn, conn->txb_allocated, len);
        return -EINVAL;
    }

    tcp_flow_tx_segmentation_zc(tctx, conn, buf, len);

    conn->txb_allocated -= len;
    conn->txb_sent += len;

    return 0;
}

static inline int eTran_tcp_tx_submit_zc_retransmission(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, size_t len)
{
    if (unlikely(conn->status != CONN_OPEN))
    {
        fprintf(stderr, "eTran_tcp_tx_submit_zc_retransmission(): conn->status != CONN_OPEN\n");
        return -EINVAL;
    }

    len = tcp_flow_tx_segmentation_zc_retransmission(tctx, conn, len);

    conn->txb_allocated -= len;
    conn->txb_sent += len;

    return 0;
}

static inline ssize_t eTran_tcp_rx_peek_count_zc(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, size_t count, void *buf)
{
    uint32_t copy_offset = 0;
    if (conn->rxb_used == 0) {
        return 0;
    }

    if (count > conn->rxb_used) {
        count = conn->rxb_used;
    }

    if (conn->rxb_head + count > conn->rx_buf_size) {
        // valid range [conn->rxb_head, conn->rx_buf_size), [0, conn->rxb_head + count - conn->rx_buf_size)
        for (auto it = conn->rx_addrs.begin(); it != conn->rx_addrs.end();) {
            auto [addr, pkt] = *it;
            uint16_t py_len = rxmeta_plen(pkt);
            uint16_t py_off = rxmeta_poff(pkt);
            uint32_t rx_pos = rxmeta_pos(pkt);
            if (rx_pos >= conn->rxb_head || rx_pos < conn->rxb_head + count - conn->rx_buf_size) {
                auto append_len = std::min((size_t)py_len, count - copy_offset);
                dma((uint8_t *)buf + copy_offset, pkt + py_off, append_len);
                copy_offset += append_len;

                if (likely(append_len == py_len)) {
                    thread_bcache_prod(&tctx->iobuffer, addr);
                    // remove from rx_addrs
                    it = conn->rx_addrs.erase(it);
                } else {
                    /* truncate packet */
                    rxmeta_set_poff(pkt, py_off + append_len);
                    rxmeta_set_plen(pkt, py_len - append_len);
                    rxmeta_set_pos(pkt, rx_pos + append_len > conn->rx_buf_size ? rx_pos + append_len - conn->rx_buf_size : rx_pos + append_len);
                    // printf("truncate packet: copy_offset(%u), append_len(%ld)\n", copy_offset, append_len);
                    break;
                }

            } else {
                // printf("mismatch: rx_pos(%u), conn->rxb_head(%u), count(%ld)\n", rx_pos, conn->rxb_head, count);
                it++;
            }
            if (copy_offset == count)
                break;
        }
        
    } else {
        // valid range [conn->rxb_head, conn->rxb_head + count)
        for (auto it = conn->rx_addrs.begin(); it != conn->rx_addrs.end();) {
            auto [addr, pkt] = *it;
            uint16_t py_len = rxmeta_plen(pkt);
            uint16_t py_off = rxmeta_poff(pkt);
            uint32_t rx_pos = rxmeta_pos(pkt);
            if (rx_pos >= conn->rxb_head && rx_pos < conn->rxb_head + count) {
                auto append_len = std::min((size_t)py_len, count - copy_offset);
                dma((uint8_t *)buf + copy_offset, pkt + py_off, append_len);
                copy_offset += append_len;

                if (likely(append_len == py_len)) {
                    thread_bcache_prod(&tctx->iobuffer, addr);
                    // remove from rx_addrs
                    it = conn->rx_addrs.erase(it);
                } else {
                    /* truncate packet */
                    rxmeta_set_poff(pkt, py_off + append_len);
                    rxmeta_set_plen(pkt, py_len - append_len);
                    rxmeta_set_pos(pkt, rx_pos + append_len > conn->rx_buf_size ? rx_pos + append_len - conn->rx_buf_size : rx_pos + append_len);
                    // printf("truncate packet: copy_offset(%u), append_len(%ld)\n", copy_offset, append_len);
                    break;
                }

            } else {
                // printf("mismatch: rx_pos(%u), conn->rxb_head(%u), count(%ld)\n", rx_pos, conn->rxb_head, count);
                it++;
            }
            if (copy_offset == count)
                break;
        }
    }

    return copy_offset;
}

/**
 * @brief Release len bytes data from the connection
 * @param tctx the per-thread context
 * @param conn the connection
 * @param len the length of data to be released
 * @return 0 on success, -EINVAL on failure
 */
static inline int eTran_tcp_rx_release(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, size_t len)
{
    if (conn->rxb_used < len)
    {
        fprintf(stderr, "eTran_tcp_submit_rx(): conn->rxb_used < len\n");
        return -EINVAL;
    }

    conn->rxb_used -= len;
    conn->rxb_bump += len;

    conn->rxb_head += len;
    if (conn->rxb_head > conn->rx_buf_size) {
        conn->rxb_head -= conn->rx_buf_size;
    }

    if (unlikely((conn->rxb_bump > std::min(std::min(conn->rx_buf_size >> 2, ((unsigned int)0xFFFF) << TCP_WND_SCALE), (unsigned int)32768) || conn->force_rx_bump) 
        && !conn->in_rx_bump_pending))
    {
        tctx->rx_bump_pending_conns.push_back(conn);
        conn->in_rx_bump_pending = true;
    }

    conn->force_rx_bump = false;

    return 0;
}

/**
 * @brief get the amount of data that can be transmitted in the connection's tx buffer
 */
static inline ssize_t eTran_tcp_tx_avail(struct eTrantcp_connection *conn)
{
    return txb_bytes_avail(conn);
}

/**
 * @brief reserve space in the connection's tx buffer for data transmission
 * @param tctx the per-thread context
 * @param conn the connection
 * @param reserve_len the length of data to be reserved
 * @return the length of data reserved on success, -EINVAL on failure
 */
static inline ssize_t eTran_tcp_tx_reserve_zc(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, size_t reserve_len)
{
    uint32_t avail;
    uint32_t head;

    if (unlikely(conn->status != CONN_OPEN))
    {
        fprintf(stderr, "eTran_tcp_tx_reserve_zc(): conn->status != CONN_OPEN\n");
        return -EINVAL;
    }

    avail = txb_bytes_avail(conn);
    if (avail < reserve_len)
    {
        reserve_len = avail;
    }

    // get current head of tx buffer
    head = conn->txb_head + conn->txb_allocated;

    if (head >= conn->tx_buf_size)
    {
        head -= conn->tx_buf_size;
    }

    conn->txb_allocated += reserve_len;

    return reserve_len;
}

static inline int notify_kernel_tcp_conn_open(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, int fd,
                                          uint32_t remote_ip, uint16_t remote_port)
{
    lrpc_msg msg;
    struct appout_tcp_open_t *open_msg = (struct appout_tcp_open_t *)msg.data;

    msg.cmd = APPOUT_TCP_OPEN;
    open_msg->opaque_connection = OPAQUE(conn);
    open_msg->fd = fd;
    open_msg->remote_ip = remote_ip;
    open_msg->remote_port = remote_port;

    if (lrpc_send(&tctx->app_out, &msg))
    {
        fprintf(stderr, "notify_kernel_tcp_conn_open(): lrpc_send() failed\n");
        return -1;
    }
    return 0;
}

static inline int notify_kernel_tcp_conn_bind(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, int fd,
                                          uint32_t local_ip, uint16_t local_port, bool reuseport)
{
    lrpc_msg msg;
    struct appout_tcp_bind_t *bind_msg = (struct appout_tcp_bind_t *)msg.data;

    msg.cmd = APPOUT_TCP_BIND;
    bind_msg->opaque_connection = OPAQUE(conn);
    bind_msg->fd = fd;
    bind_msg->local_ip = local_ip;
    bind_msg->local_port = local_port;
    bind_msg->reuseport = reuseport;

    if (lrpc_send(&tctx->app_out, &msg))
    {
        fprintf(stderr, "notify_kernel_tcp_conn_bind(): lrpc_send() failed\n");
        return -1;
    }

    return 0;
}

static inline int notify_kernel_tcp_conn_listen(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, struct eTrantcp_listener *listener, int fd, unsigned int backlog)
{
    lrpc_msg msg;
    struct appout_tcp_listen_t *listen_msg = (struct appout_tcp_listen_t *)msg.data;

    msg.cmd = APPOUT_TCP_LISTEN;

    listen_msg->opaque_listener = OPAQUE(listener);
    listen_msg->opaque_connection = OPAQUE(conn);
    listen_msg->fd = fd;
    listen_msg->backlog = backlog;

    if (lrpc_send(&tctx->app_out, &msg))
    {
        fprintf(stderr, "notify_kernel_tcp_conn_listen(): lrpc_send() failed\n");
        return -1;
    }

    return 0;
}

static inline int notify_kernel_tcp_conn_accept(struct app_ctx_per_thread *tctx, struct eTrantcp_listener *listener,
                                            struct eTrantcp_connection *conn, int fd, int newfd, uint16_t local_port)
{
    lrpc_msg msg;
    struct appout_tcp_accept_t *accept_msg = (struct appout_tcp_accept_t *)msg.data;

    msg.cmd = APPOUT_TCP_ACCEPT;

    accept_msg->tid = tctx->tid;
    accept_msg->opaque_connection = OPAQUE(conn);
    accept_msg->opaque_listener = OPAQUE(listener);
    accept_msg->fd = fd;
    accept_msg->newfd = newfd;
    accept_msg->local_port = local_port;

    if (lrpc_send(&tctx->app_out, &msg))
    {
        fprintf(stderr, "notify_kernel_tcp_conn_accept(): lrpc_send() failed\n");
        return -1;
    }

    return 0;
}

static inline int notify_kernel_tcp_conn_close(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, int fd)
{
    lrpc_msg msg;
    struct appout_tcp_close_t *close_msg = (struct appout_tcp_close_t *)msg.data;
    
    msg.cmd = APPOUT_TCP_CLOSE;
    
    close_msg->opaque_connection = OPAQUE(conn);
    close_msg->fd = fd;
    close_msg->local_ip = conn->local_ip;
    close_msg->remote_ip = conn->remote_ip;
    close_msg->local_port = conn->local_port;
    close_msg->remote_port = conn->remote_port;

    if (lrpc_send(&tctx->app_out, &msg))
    {
        fprintf(stderr, "notify_kernel_tcp_conn_close(): lrpc_send() failed\n");
        return -1;
    }

    return 0;
}

static inline ssize_t conn_recv(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, void *buf, size_t count)
{
    ssize_t ret;

    ret = eTran_tcp_rx_peek_count_zc(tctx, conn, count, buf);

    if (ret <= 0)
        return ret;

    eTran_tcp_rx_release(tctx, conn, ret);

    return ret;
}

static inline ssize_t conn_send(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, const void *buf, size_t count)
{
    ssize_t ret;

    ret = eTran_tcp_tx_reserve_zc(tctx, conn, count);

    if (ret <= 0)
        return ret;    

    eTran_tcp_tx_submit_zc(tctx, conn, buf, ret);

    return ret;
}

static inline int notify_kernel_homa_bind(struct app_ctx_per_thread *tctx, struct eTranhoma_socket *socket, int fd)
{
    lrpc_msg msg;
    struct appout_homa_bind_t *bind_msg = (struct appout_homa_bind_t *)msg.data;
    
    uint32_t local_ip = socket->local_ip;
    uint16_t local_port = socket->local_port;

    msg.cmd = APPOUT_HOMA_BIND;
    bind_msg->opaque_socket = OPAQUE(socket);
    bind_msg->fd = fd;
    bind_msg->local_ip = local_ip;
    bind_msg->local_port = local_port;

    if (lrpc_send(&tctx->app_out, &msg))
    {
        fprintf(stderr, "notify_kernel_homa_bind(): lrpc_send() failed\n");
        return -1;
    }
    return 0;
}

static inline int notify_kernel_homa_close(struct app_ctx_per_thread *tctx, struct eTranhoma_socket *socket, int fd)
{
    lrpc_msg msg;
    struct appout_homa_close_t *close_msg = (struct appout_homa_close_t *)msg.data;

    msg.cmd = APPOUT_HOMA_CLOSE;
    close_msg->opaque_socket = OPAQUE(socket);
    close_msg->fd = fd;
    if (lrpc_send(&tctx->app_out, &msg))
    {
        fprintf(stderr, "notify_kernel_homa_close(): lrpc_send() failed\n");
        return -1;
    }

    return 0;
}