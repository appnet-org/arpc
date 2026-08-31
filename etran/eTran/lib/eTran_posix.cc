#include "app_if.h"
#include "eTran_common.h"
#include "tcp_if.h"
#include "xsk_if.h"
#include <eTran_posix.h>

#include <unistd.h>
#include <unordered_map>

#include <base/ipc.h>
#include <base/compiler.h>
#include <base/lrpc.h>
#include <shm/shm_wrapper.h>

#include <sys/un.h>
#include <sys/epoll.h>
#include <linux/if_ether.h>
#include <net/if.h>
#include <netinet/ip.h>
#include <netinet/tcp.h>
#include <xskbp/xsk_buffer_pool.h>

#define XSK_INFO(qidx) (tctx->txrx_xsk_info[qidx])
#define ADD_FQ_WORK(qidx, work) atomic32_add(&tctx->actx->uring[qidx].fq_work, work)
#define DEL_FQ_WORK(qidx, work) atomic32_sub(&tctx->actx->uring[qidx].fq_work, work)
#define GET_FQ_WORK(qidx) atomic32_read(&tctx->actx->uring[qidx].fq_work)
#define FQ(qidx) (tctx->actx->uring[qidx].fq)
#define FQ_LOCK(qidx) spin_lock(tctx->actx->uring[qidx].fq_lock)
#define FQ_LOCK_TRY(qidx) spin_lock_try(tctx->actx->uring[qidx].fq_lock)
#define FQ_UNLOCK(qidx) spin_unlock(tctx->actx->uring[qidx].fq_lock)
#define ADD_CQ_WORK(qidx, work) atomic32_add(&tctx->actx->uring[qidx].cq_work, work)
#define DEL_CQ_WORK(qidx, work) atomic32_sub(&tctx->actx->uring[qidx].cq_work, work)
#define GET_CQ_WORK(qidx) atomic32_read(&tctx->actx->uring[qidx].cq_work)
#define CQ(qidx) (tctx->actx->uring[qidx].cq)
#define CQ_LOCK(qidx) spin_lock(tctx->actx->uring[qidx].cq_lock)
#define CQ_LOCK_TRY(qidx) spin_lock_try(tctx->actx->uring[qidx].cq_lock)
#define CQ_UNLOCK(qidx) spin_unlock(tctx->actx->uring[qidx].cq_lock)

// interpose.cc
extern int (*libc_epoll_create1)(int flags);
extern int (*libc_epoll_ctl)(int epfd, int op, int fd,
                             struct epoll_event *event);
extern int (*libc_epoll_wait)(int epfd, struct epoll_event *events,
                              int maxevents, int timeout);

extern inline ssize_t eTran_tcp_tx_reserve_zc(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, size_t reserve_len);
extern inline int eTran_tcp_tx_submit_zc(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, const void *buf, size_t len);
extern inline ssize_t eTran_tcp_rx_peek(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, void **buffer_1, size_t *len_1, void **buffer_2, size_t *len_2);
extern inline ssize_t eTran_tcp_rx_peek_count_zc(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, size_t count, void *buf);
extern inline int eTran_tcp_rx_release(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, size_t len);

/**
 * @brief The basic idea is to reduce the number of events by lazy updating previous connection
 * @param cached_conn cached connection
 * @param ret_events return events
 */
static inline void lazy_update_prev_conn_txev(struct eTrantcp_connection *cached_conn, struct eTrantcp_event *ret_events,
                                              int *nr_event)
{
    ret_events[*nr_event].type = ETRANTCP_EV_CONN_SENDBUF;
    ret_events[*nr_event].ev.send.conn = cached_conn;
    (*nr_event)++;
}
/**
 * @brief The basic idea is to reduce the number of events by lazy updating previous connection
 * @param cached_conn cached connection
 * @param cached_rx_bump cached received bump
 * @param ret_events return events
 * @param nr_event number of events
 */
static inline void lazy_update_prev_conn_rxev(struct eTrantcp_connection *cached_conn, size_t cached_rx_bump,
                                              struct eTrantcp_event *ret_events, int *nr_event)
{
    if (!cached_rx_bump)
        return;
    ret_events[*nr_event].type = ETRANTCP_EV_CONN_RECVED;
    ret_events[*nr_event].ev.recv.conn = cached_conn;
    (*nr_event)++;

    // update connection receive buffer
    cached_conn->rxb_used += cached_rx_bump;
}

static inline void in_order_receive(struct eTrantcp_connection *conn, uint64_t addr, char *pkt)
{
    conn->rx_addrs.push_back({addr, pkt});
}

static inline void out_of_order_receive(struct eTrantcp_connection *conn, uint64_t addr, char *pkt)
{
    conn->ooo_rx_addrs.push_back({addr, pkt});
}

/**
 * @brief synchronize transmission state with eBPF
 *        this function serves three purposes:
 *        1) reset buffer pointers when retransmission happens and enqueue to retransmission queue
 *        2) update xsk budget if it's changed
 *        3) ack bytes
 * @param tctx thread context
 * @param conn connection
 * @param pkt packet
 * @param to signal from slowpath
 * @return true if we should generate a send event
 */
static inline bool sync_state(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, char *pkt, bool to)
{
    // bool can_continue_send = false;
    bool send_event = txb_bytes_avail(conn) == 0;
    uint32_t xsk_budget_avail = rxmeta_xskbudget(pkt);
    uint32_t ack_bytes = rxmeta_ackbytes(pkt);
    uint32_t go_back_pos = rxmeta_go_back_pos(pkt);
    uint32_t old_txb_sent = conn->txb_sent;
    uint32_t go_back_bytes;

    if (to)
    {
        // printf("sync_state: xsk_budget_avail = %u, go_back_pos = %u(%d)\n",
        //     xsk_budget_avail, (go_back_pos & ~RECOVERY_MASK), (go_back_pos & RECOVERY_MASK) == RECOVERY_MASK);
    }

    // Retransmission
    // reset conn->txb_head and conn->txb_sent
    if (unlikely(go_back_pos != POISON_32 && go_back_pos & RECOVERY_MASK))
    {
        go_back_pos &= ~RECOVERY_MASK;
        if (likely(go_back_pos != conn->txb_head))
        {
            if (conn->txb_head > go_back_pos)
            {
                go_back_bytes = conn->txb_head - go_back_pos;
            }
            else
            {
                go_back_bytes = conn->tx_buf_size - (go_back_pos - conn->txb_head);
            }
            conn->txb_head = go_back_pos;
            conn->txb_allocated += go_back_bytes;
            conn->txb_sent -= go_back_bytes;
#ifdef DEBUG
            printf("sync_state: go_back_bytes = %u\n", go_back_bytes);
#endif
            // enqueue to retransmission queue
            // fprintf(stdout, "Reset pointer to %u for conn(%p)\n", go_back_pos, conn);
            tctx->retransmission_conns.push_back(std::make_pair(conn, go_back_bytes));
        }
    }

    // xsk budget update
    if (xsk_budget_avail != POISON_32 && conn->xsk_budget != xsk_budget_avail)
    {
        conn->xsk_budget = xsk_budget_avail;
#ifdef DEBUG
        printf("sync_state: xsk_budget_avail = %u\n", xsk_budget_avail);
#endif
        // we can continue to send even if retransmission happens

        // option1: as long as we have budget, we can continue to send
        // can_continue_send = txb_bytes_avail(conn) > 0;

        // option2: as long as we have at least some budget, we can continue to send
        // this can help to reduce the number of events
        // can_continue_send |= txb_bytes_avail(conn) >= (TCP_MSS_W_TS << 2);
    }

    if (unlikely(old_txb_sent != conn->txb_sent))
    {
        // retransmission happens, it's impossible to ack bytes
        return send_event;
    }

    // ack bytes
    if (ack_bytes != POISON_32 && ack_bytes)
    {
        conn->txb_sent -= ack_bytes;
#ifdef DEBUG
        printf("sync_state: ack_bytes = %u\n", ack_bytes);
#endif
        // option1: as long as we have budget, we can continue to send
        // can_continue_send = txb_bytes_avail(conn) > 0;
        // option2: as long as we have at least some budget, we can continue to send
        // this can help to reduce the number of events
        // can_continue_send |= txb_bytes_avail(conn) >= (TCP_MSS_W_TS << 2);

        if (!conn->pending_free_bytes)
            tctx->free_pending_conns.push_back(conn);
        conn->pending_free_bytes += ack_bytes;
    }

    return send_event;
}

static inline void handle_rx(struct app_ctx_per_thread *tctx, struct eTrantcp_connection **cached_conn_ptr, size_t *cached_rx_bump, bool *cached_sendbuf_event,
    struct eTrantcp_event *ret_events, int *nr_event, uint64_t addr, char *pkt, bool last)
{
    struct thread_bcache *bc = &tctx->iobuffer;
    struct eTrantcp_connection *cached_conn = *cached_conn_ptr;
    struct eTrantcp_connection *conn =
            reinterpret_cast<struct eTrantcp_connection *>(rxmeta_conn(pkt));
    uint32_t qid;
    uint16_t py_len;
    uint32_t ooo_bump;

    if (unlikely((uint64_t)conn == POISON_64 || conn == NULL))
    {
        thread_bcache_prod(bc, addr);
        goto out;
    }

    // it's likely that we are processing the same connection
    if (unlikely(cached_conn != NULL && conn != cached_conn))
    {
        lazy_update_prev_conn_rxev(cached_conn, *cached_rx_bump, ret_events, nr_event);
        if (*cached_sendbuf_event)
            lazy_update_prev_conn_txev(cached_conn, ret_events, nr_event);
        *cached_conn_ptr = conn;
        cached_conn = conn;
        *cached_rx_bump = 0;
        *cached_sendbuf_event = false;
    }
    else if (unlikely(cached_conn == NULL))
    {
        *cached_conn_ptr = conn;
        cached_conn = conn;
        *cached_rx_bump = 0;
    }

    qid = rxmeta_qid(pkt);
    if (unlikely(qid == POISON_32))
    {
        // printf("Receive TO signal from slowpath\n");
        if (sync_state(tctx, conn, pkt, true))
        {
            *cached_sendbuf_event = true;
        }
        // Timeout packet from slowpath, don't update need_fill!!!
        thread_bcache_prod(bc, addr);
        goto out;
    }

    if (unlikely(qid & FORCE_RX_BUMP_MASK))
    {
        conn->force_rx_bump = true;
        qid &= ~FORCE_RX_BUMP_MASK;
    }

    if (!(tctx->txrx_xsk_info[tctx->actx->qid2idx[qid]]->cached_needfill++))
        tctx->cached_fqidx.push(tctx->actx->qid2idx[qid]);

    if (sync_state(tctx, conn, pkt, false))
    {
        *cached_sendbuf_event = true;
    }

    // XDP has prepared the position in received buffer, payload offset and payload length for us
    py_len = rxmeta_plen(pkt);
    ooo_bump = rxmeta_ooo_bump(pkt);
    if (py_len == POISON_16)
    {
        if (unlikely(ooo_bump & OOO_CLEAR_MASK)) {
            /* clear out-of-order segments */
            conn->ooo_rx_addrs.clear();
        }

        thread_bcache_prod(bc, addr);
        goto out;
    }

    if (ooo_bump != POISON_32)
    {
        /* An out-of-order interval is finished, this packet is the last acked part 
            * (ooo_bump & ~OOO_FIN_MASK) equals to all acked bytes including this packet
            */
        if (ooo_bump & OOO_FIN_MASK)
        {
            ooo_bump &= ~OOO_FIN_MASK;
            *cached_rx_bump += ooo_bump; // ooo_bump has already included py_len
            /* append ooo_rx_addrs to the tail of rx_addrs */
            conn->rx_addrs.insert(conn->rx_addrs.end(), conn->ooo_rx_addrs.begin(), conn->ooo_rx_addrs.end());
            conn->ooo_rx_addrs.clear();
            in_order_receive(conn, addr, pkt);
            // printf("out_of_order_receive fin: rx_bump = %ld\n", *cached_rx_bump);
        }
        else if (ooo_bump & OOO_SEGMENT_MASK)
        {
            /* out-of-order segment, don't update cached_rx_bump */
            out_of_order_receive(conn, addr, pkt);
            // printf("out_of_order_receive: rx_bump = %ld\n", *cached_rx_bump);
        }
    }
    else
    {
        /* This packet is in order */
        *cached_rx_bump += py_len;
        in_order_receive(conn, addr, pkt);
        // printf("in_order_receive: rx_bump = %ld\n", *cached_rx_bump);
    }

out:
    if (unlikely(last)) {
        if (likely(cached_conn && *cached_rx_bump))
            lazy_update_prev_conn_rxev(cached_conn, *cached_rx_bump, ret_events, nr_event);

        if (*cached_sendbuf_event)
            lazy_update_prev_conn_txev(cached_conn, ret_events, nr_event);
    }
}

/**
 * @brief Polling events from NIC
 * @param tctx thread context
 * @param ret_events return events
 * @param max_events max number of events
 */
int tcp_nic_poll(struct app_ctx_per_thread *tctx, struct eTrantcp_event *ret_events, int budget)
{
    int nr_event = 0;
    unsigned int qidx;
    unsigned int i;
    unsigned int total_rcvd = 0;
    unsigned int rcvd;
    unsigned int nr_nic_queues = tctx->actx->nr_nic_queues;
    struct thread_bcache *bc = &tctx->iobuffer;
    struct eTrantcp_connection *cached_conn = NULL;
    struct xsk_ring_prod *fq;
    struct xsk_ring_cons *cq;
    unsigned int idx_fq = 0, idx_cq = 0;
    size_t cached_rx_bump = 0;
    bool cached_sendbuf_event = false;
    uint64_t addrs[64];
    char *pkts[64];

    for (qidx = 0; qidx < nr_nic_queues; qidx++)
    {
        uint32_t sp = 0;
        if (tctx->txrx_xsk_info[qidx]->outstanding)
        {
            CQ_LOCK(qidx);
            cq = CQ(qidx);
            rcvd = eTran_cq__peek(cq, std::min(tctx->txrx_xsk_info[qidx]->outstanding, TX_BATCH_SIZE), &idx_cq, tctx->actx->uring[qidx].comp_offset);
            for (unsigned int i = 0; i < rcvd; i++) {
                uint64_t addr = *eTran_cq__comp_addr(cq, idx_cq + i, tctx->actx->uring[qidx].comp_offset);
                if (unlikely(tcp_txmeta_get_from_slowpath(tctx->txrx_xsk_info[qidx]->umem_area, addr) || 
                    tcp_txmeta_get_flag(tctx->txrx_xsk_info[qidx]->umem_area, addr) == FLAG_SYNC)) {
                    thread_bcache_prod(bc, addr);
                    if (unlikely(tcp_txmeta_get_from_slowpath(tctx->txrx_xsk_info[qidx]->umem_area, addr))) {
                        sp++;
                    }
                }
            }
            if (rcvd)
            {
                eTran_cq__release(cq, rcvd, tctx->actx->uring[qidx].comp_offset);
                tctx->txrx_xsk_info[qidx]->outstanding -= rcvd - sp;
            }
            CQ_UNLOCK(qidx);
        }
    }

    qidx = tctx->next_rcv_qidx;
    while (1)
    {
        if (budget <= 0)
            break;
        struct xsk_socket_info *xsk_info = tctx->txrx_xsk_info[qidx];
        struct xsk_ring_cons *rx = &xsk_info->rx;
        if (xsk_rxring_empty(rx))
        {
            qidx = (qidx + 1) % nr_nic_queues;
            if (qidx == tctx->next_rcv_qidx)
                break;
            continue;
        }
        unsigned int idx_rx = 0;
        rcvd = eTran_cq__peek(rx, std::min(RX_BATCH_SIZE, (unsigned int)budget), &idx_rx, tctx->actx->uring[qidx].comp_offset);
        budget -= rcvd;

        for (i = 0; i < rcvd; i++)
        {
            addrs[total_rcvd] = xsk_umem__add_offset_to_addr(xsk_ring_cons__rx_desc(rx, idx_rx + i)->addr);
            pkts[total_rcvd] = reinterpret_cast<char *>(xsk_umem__get_data(xsk_info->umem_area, addrs[total_rcvd]));
            total_rcvd++;
        }
        xsk_ring_cons__release(rx, rcvd);
        qidx = (qidx + 1) % nr_nic_queues;
        if (qidx == tctx->next_rcv_qidx)
            break;
    }

    tctx->next_rcv_qidx = (tctx->next_rcv_qidx + 1) % nr_nic_queues;

    for (i = 0; i < total_rcvd; i++)
    {
        if (i + 2 < total_rcvd)
            prefetch(pkts[i + 2] - 32);
        uint64_t addr = addrs[i];
        char *pkt = pkts[i];

        handle_rx(tctx, &cached_conn, &cached_rx_bump, &cached_sendbuf_event, ret_events, &nr_event, addr, pkt, i == total_rcvd - 1);
    }

    while (tctx->cached_fqidx.pop(&qidx) == 0)
    {
        unsigned int need_fill = tctx->txrx_xsk_info[qidx]->cached_needfill;
        fq = FQ(qidx);
        FQ_LOCK(qidx);
        while (eTran_fq__reserve(fq, need_fill, &idx_fq, tctx->actx->uring[qidx].fill_offset) < need_fill)
        {
            kick_fq(tctx->txrx_xsk_fd[qidx], fq, tctx->actx->uring[qidx].fill_offset);
        }

        for (unsigned int j = 0; j < need_fill; j++)
        {
            assert(thread_bcache_check(bc, 1) == 1);
            *eTran_fq__fill_addr(fq, idx_fq++, tctx->actx->uring[qidx].fill_offset) = thread_bcache_cons(bc);
        }
        eTran_fq__submit(fq, need_fill, tctx->actx->uring[qidx].fill_offset);
        FQ_UNLOCK(qidx);
        tctx->txrx_xsk_info[qidx]->cached_needfill = 0;
    }

    return nr_event;
}

/**
 * @brief prepare send and receive buffer with a fixed-size for a new connection
 * TODO: buffer resizing
 */
static inline int connection_prepare_buffer(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *c)
{
    /* initialize rx buffer pointers */
    c->rxb_head = 0;
    c->rxb_used = 0;
    c->rxb_bump = 0;

    /* initialize tx buffer pointers */
    c->txb_head = 0;
    c->txb_sent = 0;
    c->txb_allocated = 0;

    c->xsk_budget = 0xFFFF << TCP_WND_SCALE;

    return 0;
}

static inline int process_tcp_kernel_events(struct app_ctx_per_thread *tctx, struct eTrantcp_event *ret_events, int max_events)
{
    int nr_eventfd_work = 0;
    int nr_event = 0;
    lrpc_msg msg;
    struct appin_tcp_event_newconn_t *newconn_msg;
    struct appin_tcp_conn_accept_t *accept_msg;
    struct appin_tcp_conn_open_t *open_msg;
    struct appin_tcp_status_t *status_bind_msg;
    struct appin_tcp_status_t *status_listen_msg;
    struct appin_tcp_status_t *status_close_msg;

    while (!lrpc_empty(&tctx->app_in))
    {
        assert(lrpc_recv(&tctx->app_in, &msg) == 0);
        nr_eventfd_work++;
        if (tctx->pending_eventfd_work)
            tctx->pending_eventfd_work--;

        switch (msg.cmd)
        {
        case APPIN_TCP_EVENT_NEWCONN:
            newconn_msg = (struct appin_tcp_event_newconn_t *)msg.data;
            ret_events[nr_event].type = ETRANTCP_EV_LISTEN_NEWCONN;
            ret_events[nr_event].ev.newconn.listener = OPAQUE_PTR(struct eTrantcp_listener, newconn_msg->opaque_listener);
            ret_events[nr_event].ev.newconn.fd = newconn_msg->fd;
            ret_events[nr_event].ev.newconn.remote_ip = newconn_msg->remote_ip;
            ret_events[nr_event].ev.newconn.remote_port = newconn_msg->remote_port;
            nr_event++;
            break;
        case APPIN_TCP_CONN_ACCEPT:
            accept_msg = (struct appin_tcp_conn_accept_t *)msg.data;
            ret_events[nr_event].type = ETRANTCP_EV_LISTEN_ACCEPT;
            ret_events[nr_event].ev.accept.conn = OPAQUE_PTR(struct eTrantcp_connection, accept_msg->opaque_connection);
            ret_events[nr_event].ev.accept.fd = accept_msg->fd;
            ret_events[nr_event].ev.accept.newfd = accept_msg->newfd;
            ret_events[nr_event].ev.accept.status = accept_msg->status;
            ret_events[nr_event].ev.accept.backlog = accept_msg->backlog;
            if (accept_msg->status == 0)
            {
                ret_events[nr_event].ev.accept.conn->local_ip = accept_msg->local_ip;
                ret_events[nr_event].ev.accept.conn->remote_ip = accept_msg->remote_ip;
                ret_events[nr_event].ev.accept.conn->remote_port = accept_msg->remote_port;
                ret_events[nr_event].ev.accept.conn->qid = accept_msg->qid;
                ret_events[nr_event].ev.accept.conn->rx_buf_size = accept_msg->rx_buf_size;
                ret_events[nr_event].ev.accept.conn->tx_buf_size = accept_msg->tx_buf_size;
                ret_events[nr_event].ev.accept.conn->status = CONN_OPEN;
                assert(connection_prepare_buffer(tctx, ret_events[nr_event].ev.accept.conn) == 0);
                // insert this connection to per thread context
                tctx->open_conns.insert(
                    std::make_pair(eTran_tcp_flow_tuple(accept_msg->remote_ip, accept_msg->remote_port, accept_msg->local_ip,
                                                        ret_events[nr_event].ev.accept.conn->local_port),
                                   ret_events[nr_event].ev.accept.conn));
            }
            else
            {
                ret_events[nr_event].ev.accept.conn->try_accept_done = true;
            }
            nr_event++;
            break;
        case APPIN_TCP_CONN_OPEN:
            open_msg = (struct appin_tcp_conn_open_t *)msg.data;
            ret_events[nr_event].type = ETRANTCP_EV_CONN_OPEN;
            ret_events[nr_event].ev.open.conn = OPAQUE_PTR(struct eTrantcp_connection, open_msg->opaque_connection);
            ret_events[nr_event].ev.open.fd = open_msg->fd;
            ret_events[nr_event].ev.open.status = open_msg->status;
            if (open_msg->status == 0)
            {
                ret_events[nr_event].ev.open.conn->local_ip = open_msg->local_ip;
            }
            else if (open_msg->status == 1)
            {
                ret_events[nr_event].ev.open.conn->qid = open_msg->qid;
                ret_events[nr_event].ev.open.conn->local_port = open_msg->local_port;
                ret_events[nr_event].ev.open.conn->rx_buf_size = open_msg->rx_buf_size;
                ret_events[nr_event].ev.open.conn->tx_buf_size = open_msg->tx_buf_size;
                ret_events[nr_event].ev.open.conn->status = CONN_OPEN;
                assert(connection_prepare_buffer(tctx, ret_events[nr_event].ev.open.conn) == 0);
                // insert this connection to per thread context
                tctx->open_conns.insert(std::make_pair(eTran_tcp_flow_tuple(ret_events[nr_event].ev.open.conn->remote_ip,
                                                                            ret_events[nr_event].ev.open.conn->remote_port,
                                                                            ret_events[nr_event].ev.open.conn->local_ip,
                                                                            ret_events[nr_event].ev.open.conn->local_port),
                                                       ret_events[nr_event].ev.open.conn));

                // generate a ETRANTCP_EV_CONN_SENDBUF event
                nr_event++;
                ret_events[nr_event].type = ETRANTCP_EV_CONN_SENDBUF;
                ret_events[nr_event].ev.send.conn = ret_events[nr_event - 1].ev.open.conn;
                // printf("Generate a ETRANTCP_EV_CONN_SENDBUF event for conn(%p)\n", ret_events[nr_event].ev.send.conn);
            }
            nr_event++;
            break;
        case APPIN_TCP_STATUS_LISTEN:
            status_listen_msg = (struct appin_tcp_status_t *)msg.data;
            ret_events[nr_event].type = ETRANTCP_EV_LISTEN_OPEN;
            ret_events[nr_event].ev.listen.listener =
                OPAQUE_PTR(struct eTrantcp_listener, status_listen_msg->opaque_listener);
            ret_events[nr_event].ev.listen.fd = status_listen_msg->fd;
            ret_events[nr_event].ev.listen.status = status_listen_msg->status;
            nr_event++;
            break;
        case APPIN_TCP_STATUS_BIND:
            status_bind_msg = (struct appin_tcp_status_t *)msg.data;
            ret_events[nr_event].type = ETRANTCP_EV_CONN_BIND;
            ret_events[nr_event].ev.bind.conn = OPAQUE_PTR(struct eTrantcp_connection, status_bind_msg->opaque_connection);
            ret_events[nr_event].ev.bind.fd = status_bind_msg->fd;
            ret_events[nr_event].ev.bind.status = status_bind_msg->status;
            nr_event++;
            break;
        case APPIN_TCP_STATUS_CLOSE:
            status_close_msg = (struct appin_tcp_status_t *)msg.data;
            ret_events[nr_event].type = ETRANTCP_EV_CONN_CLOSE;
            ret_events[nr_event].ev.close.conn =
                OPAQUE_PTR(struct eTrantcp_connection, status_close_msg->opaque_connection);
            ret_events[nr_event].ev.close.fd = status_close_msg->fd;
            ret_events[nr_event].ev.close.status = status_close_msg->status;
            nr_event++;
            break;
        default:
            fprintf(stderr, "tcp_kernel_poll(): unknown msg.cmd: %ld\n", msg.cmd);
            break;
        }
        if (nr_event >= max_events)
            break;
    }
    uint64_t ret = consume_evfd(tctx->evfd);
    if (ret > (uint64_t)nr_eventfd_work)
    {
        // we have pending events not process
        tctx->pending_eventfd_work += (ret - nr_eventfd_work);
        // printf("tcp_kernel_poll()-2: pending_eventfd_work = %d\n", tctx->pending_eventfd_work);
    }

    return nr_event;
}

int tcp_nic_poll_epoll(struct app_ctx_per_thread *tctx, struct eTrantcp_event *ret_events, int budget, int timeout)
{
    int nr_event = 0;
    unsigned int qidx;
    unsigned int i;
    int j;
    unsigned int nr_nic_queues = tctx->actx->nr_nic_queues;
    struct thread_bcache *bc = &tctx->iobuffer;
    struct xsk_ring_cons *cq;
    struct xsk_ring_prod *fq;
    int nfds;
    struct epoll_event events[nr_nic_queues + 1];
    unsigned int quantum = budget;
    uint64_t addrs[64];
    char *pkts[64];

    struct eTrantcp_connection *cached_conn = NULL;
    size_t cached_rx_bump = 0;
    bool cached_sendbuf_event = false;
    unsigned int rcvd;
    unsigned int idx_fq = 0, idx_cq = 0, idx_rx = 0;

    for (qidx = 0; qidx < nr_nic_queues; qidx++)
    {
        uint32_t sp = 0;
        if (tctx->txrx_xsk_info[qidx]->outstanding)
        {
            CQ_LOCK(qidx);
            cq = CQ(qidx);
            rcvd = eTran_cq__peek(cq, std::min(tctx->txrx_xsk_info[qidx]->outstanding, TX_BATCH_SIZE), &idx_cq, tctx->actx->uring[qidx].comp_offset);
            for (unsigned int i = 0; i < rcvd; i++) {
                uint64_t addr = *eTran_cq__comp_addr(cq, idx_cq + i, tctx->actx->uring[qidx].comp_offset);
                if (unlikely(tcp_txmeta_get_from_slowpath(tctx->txrx_xsk_info[qidx]->umem_area, addr) || 
                    tcp_txmeta_get_flag(tctx->txrx_xsk_info[qidx]->umem_area, addr) == FLAG_SYNC)) {
                    thread_bcache_prod(bc, addr);
                    if (unlikely(tcp_txmeta_get_from_slowpath(tctx->txrx_xsk_info[qidx]->umem_area, addr))) {
                        sp++;
                    }
                }
            }
            if (rcvd)
            {
                eTran_cq__release(cq, rcvd, tctx->actx->uring[qidx].comp_offset);
                tctx->txrx_xsk_info[qidx]->outstanding -= rcvd - sp;
            }
            CQ_UNLOCK(qidx);
        }
    }

    nr_event += process_tcp_kernel_events(tctx, ret_events + nr_event, budget - nr_event);
    timeout = 0;

    // step2: receive packets (<= budget) from **all** NIC queues in a deficit round-robin manner
    nfds = libc_epoll_wait(tctx->epfd, events, nr_nic_queues + 1, timeout);
    if (unlikely(nfds < 0))
    {
        perror("epoll_wait");
        return -1;
    }

    if (unlikely(nfds == 0))
    {

        return nr_event;
    }

    quantum = budget / nfds;

    for (j = 0; j < nfds; j++)
    {
        if (unlikely(events[j].data.fd == tctx->evfd))
        {
            nr_event += process_tcp_kernel_events(tctx, ret_events + nr_event, budget - nr_event);
            // a bit werid
            if (nfds > 1)
                quantum = budget / (nfds - 1);
            continue;
        }

        qidx = tctx->txrx_xsk_fd_to_idx[events[j].data.fd];

        struct xsk_socket_info *xsk_info = tctx->txrx_xsk_info[qidx];
        struct xsk_ring_cons *rx = &xsk_info->rx;
        xsk_info->deficit += quantum;
        rcvd = xsk_ring_cons__peek(rx, std::min(xsk_info->deficit, RX_BATCH_SIZE), &idx_rx);
        if (!rcvd)
        {
            fprintf(stderr, "Werid case: no packet received\n");
            continue;
        }
        xsk_info->deficit -= rcvd;

        for (i = 0; i < rcvd; i++)
        {
            addrs[i] = xsk_umem__add_offset_to_addr(xsk_ring_cons__rx_desc(rx, idx_rx + i)->addr);
            pkts[i] = reinterpret_cast<char *>(xsk_umem__get_data(xsk_info->umem_area, addrs[i]));
        }

        xsk_ring_cons__release(rx, rcvd);

        cached_conn = NULL;
        cached_rx_bump = 0;
        cached_sendbuf_event = false;
        for (i = 0; i < rcvd; i++)
        {
            if (i + 2 < rcvd)
                prefetch(pkts[i + 2] - 32);
            uint64_t addr = addrs[i];
            char *pkt = pkts[i];

            handle_rx(tctx, &cached_conn, &cached_rx_bump, &cached_sendbuf_event, ret_events, &nr_event, addr, pkt, i == rcvd - 1); 
        }

        if (xsk_rxring_empty(rx))
        {
            xsk_info->deficit = 0;
        }
    }

    while (tctx->cached_fqidx.pop(&qidx) == 0)
    {
        unsigned int need_fill = tctx->txrx_xsk_info[qidx]->cached_needfill;
        fq = FQ(qidx);
        FQ_LOCK(qidx);
        while (eTran_fq__reserve(fq, need_fill, &idx_fq, tctx->actx->uring[qidx].fill_offset) < need_fill)
        {
            kick_fq(tctx->txrx_xsk_fd[qidx], fq, tctx->actx->uring[qidx].fill_offset);
        }

        for (unsigned int j = 0; j < need_fill; j++)
        {
            assert(thread_bcache_check(bc, 1) == 1);
            *eTran_fq__fill_addr(fq, idx_fq++, tctx->actx->uring[qidx].fill_offset) = thread_bcache_cons(bc);
        }
        eTran_fq__submit(fq, need_fill, tctx->actx->uring[qidx].fill_offset);
        FQ_UNLOCK(qidx);
        tctx->txrx_xsk_info[qidx]->cached_needfill = 0;
    }

    return nr_event;
}

size_t tcp_flow_tx_segmentation_zc_retransmission(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, size_t len)
{
    unsigned int qidx;
    struct xsk_ring_prod *tx;
    unsigned int idx_tx = 0;
    size_t actual_len = len;

#ifdef CONN_AFFINITY
    qidx = tctx->actx->qid2idx[conn->qid];
#else
#ifdef THREAD_AFFINITY
    qidx = tctx->tid % tctx->actx->nr_nic_queues;
#else
    qidx = get_qidx_with_hash(conn->remote_ip, conn->remote_port, conn->local_ip, conn->local_port,
                              tctx->actx->nr_nic_queues);
#endif
#endif

    char *umem_area = tctx->txrx_xsk_info[qidx]->umem_area;
    tx = &tctx->txrx_xsk_info[qidx]->tx;

    unsigned int total_submit_pkts = 0;
    auto it = conn->unack_tx_addrs.begin();
    uint64_t first_pkt_addr = POISON_64;

    uint32_t skip_len = 0;
    
    while (len)
    {
        if (total_submit_pkts)
            it++;
        if (it == conn->unack_tx_addrs.end())
            break;
        uint64_t addr = it->first;
        uint32_t plen = it->second;

        skip_len += plen;
        if (skip_len < conn->txb_sent) {
            it++;
            continue;
        }

        while (xsk_ring_prod__reserve(tx, 1, &idx_tx) < 1)
        {
            kick_tx(tctx->txrx_xsk_fd[qidx], tx);
        }

        tcp_txmeta_clear_all(umem_area, addr);
        tcp_txmeta_pos(umem_area, addr, conn->txb_head);
        tcp_txmeta_plen(umem_area, addr, plen);

        if (unlikely(first_pkt_addr == POISON_64))
            first_pkt_addr = addr;

        struct xdp_desc *desc = xsk_ring_prod__tx_desc(tx, idx_tx);
        desc->addr = addr;
        desc->len = sizeof(struct ethhdr) + sizeof(struct iphdr) + sizeof(struct tcphdr) + TS_OPT_SIZE + plen;
        desc->options = 0;

        conn->txb_head += plen;
        if (conn->txb_head >= conn->tx_buf_size)
        {
            conn->txb_head -= conn->tx_buf_size;
        }

        if (len > plen)
            len -= plen;
        else
            len = 0;
        total_submit_pkts++;
    }

    if (first_pkt_addr != POISON_64) {
        // set metadata for the first packet to indicate how many bytes are submitted
        tcp_txmeta_pending(umem_area, first_pkt_addr, actual_len - len);
    }

    xsk_ring_prod__submit(tx, total_submit_pkts);
    kick_tx(tctx->txrx_xsk_fd[qidx], tx);
    tctx->txrx_xsk_info[qidx]->outstanding += total_submit_pkts;

    return actual_len - len;
}

/**
 * @brief segment and transmit data in the connection's tx buffer starting from conn->txb_head
 * @param tctx the per-thread context
 * @param conn the connection
 * @param buf the data to be transmitted
 * @param len the length of data to be transmitted
 * conn->txb_head is updated in this function
 */
void tcp_flow_tx_segmentation_zc(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, const void *buf, size_t len)
{
    unsigned int qidx;
    char *umem_area;
    struct xsk_ring_prod *tx;
    unsigned int idx_tx = 0;
    unsigned int nr_buffers = 0;
    unsigned int batch_nr_buffers = 0;
    uint32_t remaining_bytes = len;
    struct thread_bcache *bc = &tctx->iobuffer;
    bool first_pkt = true;
    int copy_offset = 0;

#ifdef CONN_AFFINITY
    qidx = tctx->actx->qid2idx[conn->qid];
#else
#ifdef THREAD_AFFINITY
    qidx = tctx->tid % tctx->actx->nr_nic_queues;
#else
    qidx = get_qidx_with_hash(conn->remote_ip, conn->remote_port, conn->local_ip, conn->local_port,
                              tctx->actx->nr_nic_queues);
#endif
#endif

    tx = &tctx->txrx_xsk_info[qidx]->tx;
    umem_area = tctx->txrx_xsk_info[qidx]->umem_area;

    // How many MTU-sized IO buffers are needed
    nr_buffers = tcp_get_nr_buffers_from_len(len);

    // printf("Batch transmit %u packets, %lu\n", nr_buffers, len);
    unsigned int total_submit_pkts = 0;
    while (nr_buffers)
    {
        batch_nr_buffers = std::min(nr_buffers, TX_BATCH_SIZE);
        nr_buffers -= batch_nr_buffers;

        while (xsk_ring_prod__reserve(tx, batch_nr_buffers, &idx_tx) < batch_nr_buffers)
        {
            kick_tx(tctx->txrx_xsk_fd[qidx], tx);
        }

        for (unsigned int i = 0; i < batch_nr_buffers; i++)
        {
            struct xdp_desc *desc = xsk_ring_prod__tx_desc(tx, idx_tx + i);
            assert(thread_bcache_check(bc, 1) == 1);
            uint64_t addr = thread_bcache_cons(bc);
            addr = add_offset_tx_frame(addr);

            char *pkt = reinterpret_cast<char *>(xsk_umem__get_data(umem_area, addr));

            struct iphdr *iph = reinterpret_cast<struct iphdr *>(pkt + sizeof(struct ethhdr));
            // IP address
            iph->saddr = htonl(conn->local_ip);
            iph->daddr = htonl(conn->remote_ip);
            iph->protocol = IPPROTO_TCP;
            // TCP port
            struct tcphdr *tcph = reinterpret_cast<struct tcphdr *>(pkt + sizeof(struct ethhdr) + sizeof(struct iphdr));
            tcph->source = htons(conn->local_port);
            tcph->dest = htons(conn->remote_port);
            // DMA payload
            pkt += sizeof(struct ethhdr) + sizeof(struct iphdr) + sizeof(struct tcphdr) + TS_OPT_SIZE;
            uint32_t plen = std::min((uint32_t)TCP_MSS_W_TS, remaining_bytes);

            // set metadata
            tcp_txmeta_clear_all(umem_area, addr);
            tcp_txmeta_pos(umem_area, addr, conn->txb_head);
            tcp_txmeta_plen(umem_area, addr, plen);

            if (unlikely(first_pkt))
            {
                // set metadata for the first packet to indicate how many bytes are submitted
                tcp_txmeta_pending(umem_area, addr, len);
                first_pkt = false;
            }

            dma(pkt, (uint8_t *)buf + copy_offset, plen);

            conn->unack_tx_addrs.push_back({addr, plen});

            desc->addr = addr;
            desc->len = sizeof(struct ethhdr) + sizeof(struct iphdr) + sizeof(struct tcphdr) + TS_OPT_SIZE + plen;
            desc->options = 0;

            remaining_bytes -= plen;

            copy_offset += plen;
            
            conn->txb_head += plen;
            if (conn->txb_head >= conn->tx_buf_size)
            {
                conn->txb_head -= conn->tx_buf_size;
            }
        }
        xsk_ring_prod__submit(tx, batch_nr_buffers);
        total_submit_pkts += batch_nr_buffers;
        if (!total_submit_pkts || total_submit_pkts >= TX_BATCH_SIZE)
            kick_tx(tctx->txrx_xsk_fd[qidx], tx);
    }
    tctx->txrx_xsk_info[qidx]->outstanding += total_submit_pkts;
    kick_tx(tctx->txrx_xsk_fd[qidx], tx);
}

/**
 * @brief synchronize the rx_bump with the XDP_EGRESS for connections in rx_bump_pending_conns
 *        The synchronization is achieved by submitting a dummy packet (XDP_EGRESS may transmit it if needed)
 * @param tctx the per-thread context
 */
static void tcp_ebpf_sync(struct app_ctx_per_thread *tctx)
{
    struct eTrantcp_connection *conn;
    struct thread_bcache *bc;
    uint64_t buffer_addr;
    struct xsk_ring_prod *tx;
    uint32_t qidx;
    char *umem_area;
    unsigned int idx_tx = 0;
    struct xdp_desc *desc;
    char *pkt;
    struct iphdr *iph;
    struct tcphdr *tcph;

    auto it = tctx->rx_bump_pending_conns.begin();

    while (it != tctx->rx_bump_pending_conns.end())
    {
        conn = *it;
        bc = &tctx->iobuffer;
        // TODO: choose a reasonable queue to avoid HOL blocking
        qidx = tctx->actx->qid2idx[conn->qid];
        umem_area = tctx->txrx_xsk_info[qidx]->umem_area;
        tx = &tctx->txrx_xsk_info[qidx]->tx;

        if (xsk_ring_prod__reserve(tx, 1, &idx_tx) < 1)
        {
            fprintf(stdout, "#%u(idx:%u) Tx Ring is busy\n", conn->qid, qidx);
            break;
        }

        desc = xsk_ring_prod__tx_desc(tx, idx_tx);

        assert(thread_bcache_check(bc, 1) == 1);
        buffer_addr = thread_bcache_cons(bc);
        buffer_addr = add_offset_tx_frame(buffer_addr);

        pkt = reinterpret_cast<char *>(xsk_umem__get_data(umem_area, buffer_addr));
        iph = reinterpret_cast<struct iphdr *>(pkt + sizeof(struct ethhdr));
        tcph = reinterpret_cast<struct tcphdr *>(pkt + sizeof(struct ethhdr) + sizeof(struct iphdr));

        iph->saddr = htonl(conn->local_ip);
        iph->daddr = htonl(conn->remote_ip);
        iph->protocol = IPPROTO_TCP;

        tcph->source = htons(conn->local_port);
        tcph->dest = htons(conn->remote_port);

        // set metadata
        tcp_txmeta_clear_all(umem_area, buffer_addr);
        tcp_txmeta_flag(umem_area, buffer_addr, FLAG_SYNC);
        tcp_txmeta_rxbump(umem_area, buffer_addr, conn->rxb_bump);

        desc->addr = buffer_addr;
        desc->len = sizeof(struct ethhdr) + sizeof(struct iphdr) + sizeof(struct tcphdr) + TS_OPT_SIZE;
        desc->options = 0;

        xsk_ring_prod__submit(tx, 1);
        tctx->txrx_xsk_info[qidx]->outstanding++;
        // ADD_CQ_WORK(qidx, 1);

#ifdef DEBUG
        printf("Connection(%p): sync rxb_bump(%u) with ebpf\n", conn, conn->rxb_bump);
#endif
        kick_tx(tctx->txrx_xsk_fd[qidx], tx);

        conn->rxb_bump = 0;

        it = tctx->rx_bump_pending_conns.erase(it);

        conn->in_rx_bump_pending = false;
    }
}

/**
 * @brief call eTran_tcp_tx_submit_retransmission() for connections in retransmission_conns
 * @param tctx the per-thread context
 */
static inline void tcp_retransmission(struct app_ctx_per_thread *tctx)
{
    if (tctx->retransmission_conns.empty())
        return;

    auto it = tctx->retransmission_conns.begin();
    while (it != tctx->retransmission_conns.end())
    {
        struct eTrantcp_connection *conn = it->first;
        uint32_t len = it->second;

        if (!conn->unack_tx_addrs.empty()) 
            eTran_tcp_tx_submit_zc_retransmission(tctx, conn, len);

        // printf("Retransmitting %u bytes for conn (%p)\n", len, conn);

        it = tctx->retransmission_conns.erase(it);
    }
}

static inline void tcp_free_buffers(struct app_ctx_per_thread *tctx)
{
    struct thread_bcache *bc = &tctx->iobuffer;

    while (!tctx->free_pending_conns.empty()) {
        auto it = tctx->free_pending_conns.begin();
        struct eTrantcp_connection *conn = *it;
        while (conn->pending_free_bytes) {
            auto [addr, plen] = conn->unack_tx_addrs.front();
            conn->unack_tx_addrs.pop_front();
            thread_bcache_prod(bc, addr);
            conn->pending_free_bytes -= plen;
        }
        tctx->free_pending_conns.erase(it);
    }
}

/**
 * @brief poll events
 * @param tctx the per-thread context
 * @param events the event array
 * @param maxevents the maximum number of events
 * @param timeout the timeout in milliseconds
 */
int eTran_tcp_poll_events(struct app_ctx_per_thread *tctx, struct eTrantcp_event *events, int maxevents, int timeout)
{
    int nr_event = 0;
    int ret = 0;

    tcp_ebpf_sync(tctx);

    if (timeout == 0)
    {
        ret = tcp_nic_poll(tctx, events, maxevents);
        if (ret > 0)
            nr_event += ret;
        if (unlikely(!lrpc_empty(&tctx->app_in)))
        {
            while (!lrpc_empty(&tctx->app_in))
                nr_event += process_tcp_kernel_events(tctx, events + nr_event, maxevents - nr_event);
        }
    }
    else
    {
        ret = tcp_nic_poll_epoll(tctx, events, maxevents, timeout);
        if (ret > 0)
            nr_event += ret;
    }

    /* Note: this function must be called before tcp_retransmission() 
     * since it manipulates conn->unack_tx_addrs
     */
    tcp_free_buffers(tctx);
    
    tcp_retransmission(tctx);

    return nr_event;
}

int eTran_tcp_bind(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, int fd, uint32_t local_ip, uint32_t local_port, bool reuseport)
{
    if (!tctx || !conn)
        return -EINVAL;

    conn->status = CONN_BIND_REQUESTED;
    conn->local_ip = local_ip;
    conn->local_port = local_port;

    return notify_kernel_tcp_conn_bind(tctx, conn, fd, local_ip, local_port, reuseport);
}

/**
 * @brief open a TCP connection
 * @param tctx the per-thread context
 * @param conn the connection
 * @param remote_ip the remote IP address
 * @param remote_port the remote port
 * @return 0 on success, -EINVAL on failure
 */
int eTran_tcp_open(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, int fd, uint32_t remote_ip,
                   uint16_t remote_port)
{
    if (!tctx || !conn)
        return -EINVAL;

    conn->status = CONN_OPEN_REQUESTED;
    conn->remote_ip = remote_ip;
    conn->remote_port = remote_port;

    return notify_kernel_tcp_conn_open(tctx, conn, fd, remote_ip, remote_port);
}

/**
 * @brief listen on a TCP port
 * @param tctx the per-thread context
 * @param listener the listener
 * @param port the local port
 * @param backlog the maximum length of the queue of pending connections
 * @param reuseport whether to enable SO_REUSEPORT
 * @return 0 on success, -EINVAL on failure
 */
int eTran_tcp_listen(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, struct eTrantcp_listener *listener, int fd, uint16_t port, uint32_t backlog)
{
    if (!tctx || !listener)
        return -EINVAL;

    listener->local_port = port;

    return notify_kernel_tcp_conn_listen(tctx, conn, listener, fd, backlog);
}

/**
 * @brief accept a connection
 * @param tctx the per-thread context
 * @param listener the listener
 * @param conn the connection
 * @return 0 on success, -EINVAL on failure
 */
int eTran_tcp_accept(struct app_ctx_per_thread *tctx, struct eTrantcp_listener *listener, struct eTrantcp_connection *conn, int fd, int newfd)
{
    if (!tctx || !listener || !conn)
        return -EINVAL;

    conn->status = CONN_ACCEPT_REQUESTED;
    conn->local_port = listener->local_port;

    return notify_kernel_tcp_conn_accept(tctx, listener, conn, fd, newfd, conn->local_port);
}

/**
 * @brief close a connection
 * @param tctx the per-thread context
 * @param conn the connection
 * @return 0 on success, -EINVAL on failure
 */
int eTran_tcp_close(struct app_ctx_per_thread *tctx, struct eTrantcp_connection *conn, int fd)
{
    if (!tctx || !conn)
        return -EINVAL;

    conn->status = CONN_CLOSE_REQUESTED;

    return notify_kernel_tcp_conn_close(tctx, conn, fd);
}

static inline int process_homa_kernel_events(struct app_ctx_per_thread *tctx, struct eTranhoma_event *ret_events, int max_events)
{
    int nr_eventfd_work = 0;
    int nr_event = 0;
    lrpc_msg msg;
    struct appin_homa_status_t *status_bind;
    struct appin_homa_status_t *status_close;

    while (!lrpc_empty(&tctx->app_in))
    {
        assert(lrpc_recv(&tctx->app_in, &msg) == 0);

        nr_eventfd_work++;
        if (tctx->pending_eventfd_work)
            tctx->pending_eventfd_work--;

        switch (msg.cmd)
        {
        case APPIN_HOMA_STATUS_BIND:
            status_bind = reinterpret_cast<struct appin_homa_status_t *>(msg.data);
            ret_events[nr_event].type = ETRANHOMA_EV_SOCKET_BIND;
            ret_events[nr_event].ev.bind.status = status_bind->status;
            ret_events[nr_event].ev.bind.hs = OPAQUE_PTR(struct eTranhoma_socket, status_bind->opaque_socket);
            ret_events[nr_event].ev.bind.fd = status_bind->fd;
            nr_event++;
            break;
        case APPIN_HOMA_STATUS_CLOSE:
            status_close = reinterpret_cast<struct appin_homa_status_t *>(msg.data);
            ret_events[nr_event].type = ETRANHOMA_EV_SOCKET_CLOSE;
            ret_events[nr_event].ev.close.status = status_close->status;
            ret_events[nr_event].ev.close.hs = OPAQUE_PTR(struct eTranhoma_socket, status_close->opaque_socket);
            ret_events[nr_event].ev.bind.fd = status_bind->fd;
            nr_event++;
            break;
        default:
            fprintf(stderr, "Unknown event type %ld\n", msg.cmd);
            break;
        }

        if (nr_event >= max_events)
            break;
    }

    uint64_t ret = consume_evfd(tctx->evfd);
    if (ret > (uint64_t)nr_eventfd_work)
    {
        /* accumulate unprocessed lrpc events */
        tctx->pending_eventfd_work += (ret - nr_eventfd_work);
    }

    return nr_event;
}

/**
 * @brief poll events
 * @param tctx the per-thread context
 * @param events the event array
 * @param maxevents the maximum number of events
 * @param timeout the timeout in milliseconds
 */
int eTran_homa_poll_events(struct app_ctx_per_thread *tctx, struct eTranhoma_event *events, int maxevents, int timeout)
{
    int nr_event = 0;
    int nfds;
    struct epoll_event ep_events[1];

    /* process pending lrpc events */
    if (tctx->pending_eventfd_work)
        goto pending;

    /* use epoll to wait for lrpc events */
    nfds = libc_epoll_wait(tctx->epfd, ep_events, 1, timeout);

    if (nfds <= 0)
        return nfds;

pending:
    /* process all lrpc events from slowpath */
    while (!lrpc_empty(&tctx->app_in))
    {
        nr_event += process_homa_kernel_events(tctx, events + nr_event, maxevents - nr_event);
        if (nr_event >= maxevents)
            break;
    }

    return nr_event;
}

int eTran_homa_bind(struct app_ctx_per_thread *tctx, struct eTranhoma_socket *socket, int fd, uint32_t local_ip, uint32_t local_port)
{
    if (!tctx || !socket)
        return -EINVAL;

    socket->local_ip = local_ip;
    socket->local_port = local_port;

    return notify_kernel_homa_bind(tctx, socket, fd);
}

int eTran_homa_close(struct app_ctx_per_thread *tctx, struct eTranhoma_socket *socket, int fd)
{
    if (!tctx || !socket)
        return -EINVAL;

    return notify_kernel_homa_close(tctx, socket, fd);
}