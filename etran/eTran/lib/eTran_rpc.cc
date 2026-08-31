#include <eTran_rpc.h>

#include <linux/if_ether.h>
#include <linux/if_xdp.h>
#include <linux/ip.h>
#include <net/ethernet.h>

#define XSK_INFO(qidx) (_tctx->txrx_xsk_info[qidx])
#define ADD_FQ_WORK(qidx, work) atomic32_add(&_tctx->actx->uring[qidx].fq_work, work)
#define DEL_FQ_WORK(qidx, work) atomic32_sub(&_tctx->actx->uring[qidx].fq_work, work)
#define GET_FQ_WORK(qidx) atomic32_read(&_tctx->actx->uring[qidx].fq_work)
#define FQ(qidx) (_tctx->actx->uring[qidx].fq)
#define FQ_LOCK(qidx) spin_lock(_tctx->actx->uring[qidx].fq_lock)
#define FQ_LOCK_TRY(qidx) spin_lock_try(_tctx->actx->uring[qidx].fq_lock)
#define FQ_UNLOCK(qidx) spin_unlock(_tctx->actx->uring[qidx].fq_lock)
#define ADD_CQ_WORK(qidx, work) atomic32_add(&_tctx->actx->uring[qidx].cq_work, work)
#define DEL_CQ_WORK(qidx, work) atomic32_sub(&_tctx->actx->uring[qidx].cq_work, work)
#define GET_CQ_WORK(qidx) atomic32_read(&_tctx->actx->uring[qidx].cq_work)
#define CQ(qidx) (_tctx->actx->uring[qidx].cq)
#define CQ_LOCK(qidx) spin_lock(_tctx->actx->uring[qidx].cq_lock)
#define CQ_LOCK_TRY(qidx) spin_lock_try(_tctx->actx->uring[qidx].cq_lock)
#define CQ_UNLOCK(qidx) spin_unlock(_tctx->actx->uring[qidx].cq_lock)

#define LOG_ERR(fmt, ...) fprintf(stderr, fmt, ##__VA_ARGS__)

void RpcSocket::run_event_loop(void) 
{    
    /* transmit RPC requests */
    flush_rpc_request_queue();
    
    /* receive packets from NIC and handle RPC requests/responses */
    poll_nic_rx();
    
    /* transmit RPC responses */
    flush_rpc_response_queue();

    /* free AF_XDP UMEM buffers used by transmitted messages */
    flush_reap_backlog();
    
    /* retransmit messages */
    flush_rpc_retransmission_queue();
}

void RpcSocket::run_event_loop_block(int timeout) 
{   
    if (timeout < 0) {
        LOG_ERR("Invalid timeout value: %d\n", timeout);
        return;
    }

    /* transmit RPC requests */
    flush_rpc_request_queue();
    
    /* receive packets from NIC and handle RPC requests/responses,
     * if there is no other work to do, block until timeout
    */
    poll_nic_rx_block(timeout);
    
    /* transmit RPC responses */
    flush_rpc_response_queue();

    /* free AF_XDP UMEM buffers used by transmitted messages */
    flush_reap_backlog();
    
    /* retransmit messages */
    flush_rpc_retransmission_queue();
}

void RpcSocket::server_request(uint8_t qidx, struct data_header *d, uint32_t remote_ip, uint64_t rpcid) {
    auto remote_port = __be16_to_cpu(d->common.sport);
    auto msg_len = __be32_to_cpu(d->message_length);
    auto offset = __be32_to_cpu(d->seg.offset);
    auto seg_len = __be32_to_cpu(d->seg.segment_length);
    auto slot_idx = d->unused1;
    char *payload = d->seg.data;
    struct sockaddr_in dest_addr = {
        .sin_family = AF_INET,
        .sin_port = htons(remote_port),
        .sin_addr = {
            .s_addr = htonl(remote_ip)
        }
    };
    
    /* single-packet message */
    if (likely(msg_len <= HOMA_MSS)) {
        /* zero-copy optimization for single-packet message: 
         * directly pass the buffer to the application
        */
        Buffer buffer(reinterpret_cast<uint8_t *>(payload), msg_len, msg_len);
        
        single_pkt_req_complete(buffer, &dest_addr, rpcid, slot_idx, qidx);
    
    } else { /* multi-packet message */
        auto it = _req_handle_map.find(eTran_homa_rpc_tuple(remote_ip, remote_port, rpcid));
        
        if (unlikely(it == _req_handle_map.end())) {
            /* first packet of this message, create a new message buffer for it */
            Buffer buffer = _mempool->alloc_buffer(msg_len);
            _req_handle_map.insert(
                {
                eTran_homa_rpc_tuple(remote_ip, remote_port, rpcid), 
                InternalReqHandle(buffer, dest_addr, rpcid, d->unused1, qidx, (msg_len + HOMA_MSS - 1) / HOMA_MSS - 1)
                }
                );
            memcpy(buffer._buf + offset, payload, seg_len);
        } else {
            memcpy(it->second.buffer._buf + offset, payload, seg_len);
            it->second.count--;
            
            if (unlikely(it->second.count == 0)) {
                multi_pkt_req_complete(it);
                _req_handle_map.erase(it);
            }
        }
    }
}

void RpcSocket::client_response(uint8_t qidx, struct data_header *d)
{
    auto msg_len = __be32_to_cpu(d->message_length);
    auto offset = __be32_to_cpu(d->seg.offset);
    auto seg_len = __be32_to_cpu(d->seg.segment_length);
    char *payload = d->seg.data;
    auto slot_idx = d->unused1;
    InternalReqMeta *req_meta = &_reqmeta_slots[slot_idx];

    /* single-packet message */
    if (likely(msg_len <= HOMA_MSS)) {
        /* zero-copy optimization for single-packet message: 
         * directly pass the buffer to the application
        */
        Buffer buffer(reinterpret_cast<uint8_t *>(payload), msg_len, msg_len);
        req_meta->buffer = buffer;
        
        single_pkt_resp_complete(req_meta);
        
        /* for affinity */
        req_meta->qidx = qidx;
        
        /* return the reqmeta slot to the available slots */
        _available_slots.push(req_meta - _reqmeta_slots);
    
    } else { /* multi-packet message */
        if (req_meta->buffer.actual_size == 0) {
            /* first packet of this message, create a new message buffer for it */
            Buffer buffer = _mempool->alloc_buffer(msg_len);
            req_meta->buffer = buffer;
            memcpy(buffer._buf + offset, payload, seg_len);
            req_meta->recv_count = (msg_len + HOMA_MSS - 1) / HOMA_MSS - 1;
        } else {
            memcpy(req_meta->buffer._buf + offset, payload, seg_len);
            req_meta->recv_count--;
           
            if (unlikely(req_meta->recv_count == 0)) {
                
                multi_pkt_resp_complete(req_meta);
                
                /* return the reqmeta slot to the available slots */
                _available_slots.push(req_meta - _reqmeta_slots);
            }
        }
    }
}

// return 0 on success, -1 on failure
int RpcSocket::message_tx_retransmission(struct InternalResendMeta rm)
{
    /* it doesn't matter which queue to use */
    unsigned int qidx = rand() % _tctx->actx->nr_nic_queues;
    struct xsk_socket_info *xsk_info = _tctx->txrx_xsk_info[qidx];
    unsigned int idx_tx = 0;
    uint64_t orig_addr = rm.orig_addr;
    uint64_t addr = rm.addr;

    if (unlikely(homa_txmeta_get_flag(xsk_info->umem_area, orig_addr) & FLAG_UNDER_REAP)) {
        /* this RPC has completed after we enqueue it to retransmission queue, 
         * don't retransmit it and enqueue orig_addr to reap backlog
         */
        /* clear FLAG_UNDER_RETRANSMISSION */
        homa_txmeta_set_flag(xsk_info->umem_area, orig_addr, (~FLAG_UNDER_RETRANSMISSION) & homa_txmeta_get_flag(xsk_info->umem_area, orig_addr));
        
        enqueue_reap_backlog(orig_addr);
        
        return 0;
    }

    if (xsk_ring_prod__reserve(&xsk_info->tx, rm.nr_pkt, &idx_tx) < rm.nr_pkt) {
        LOG_ERR("Cannot reserve space in tx ring for retransmission.\n");
        return -1;
    }
    
    for (int i = 0; i < rm.nr_pkt; i++) {
        struct xdp_desc *tx_desc = xsk_ring_prod__tx_desc(&xsk_info->tx, idx_tx + i);
        tx_desc->addr = addr;
        tx_desc->len = sizeof(struct ethhdr) + sizeof(struct iphdr) + (i == rm.nr_pkt - 1 ? rm.len : HOMA_MSS);
        tx_desc->options = XDP_EGRESS_NO_COMP;
        addr = homa_txmeta_get_buffer_next(xsk_info->umem_area, addr);
    }
    xsk_info->outstanding += rm.nr_pkt;
    
    xsk_ring_prod__submit(&xsk_info->tx, rm.nr_pkt);
    
    kick_tx(xsk_info->fd, &xsk_info->tx);

    /* clear FLAG_UNDER_RETRANSMISSION */
    homa_txmeta_set_flag(xsk_info->umem_area, orig_addr, (~FLAG_UNDER_RETRANSMISSION) & homa_txmeta_get_flag(xsk_info->umem_area, orig_addr));

    return 0;
}

void RpcSocket::flush_rpc_retransmission_queue(void)
{
    while (unlikely(!_retransmission_queue.empty()))
    {
        struct InternalResendMeta r = _retransmission_queue.front();
        if (message_tx_retransmission(r) == 0) {
            _retransmission_queue.pop();
            break;
        }
    }
}

void RpcSocket::prepare_retransmission(struct resend_header *r, uint64_t buffer_addr)
{
    bool first = true;
    auto offset = __be32_to_cpu(r->offset);
    auto length = __be32_to_cpu(r->length);
    auto priority = r->priority;

    uint32_t first_seq = offset / HOMA_MSS;
    uint32_t last_seq = (offset + length - 1) / HOMA_MSS;

    uint64_t buffer_next;
    uint64_t new_buffer;
    uint32_t seq = 0;
    uint64_t orig_buffer_tail = POISON_64;
    uint64_t buffer_tail = POISON_64;
    
    struct InternalResendMeta rm = {0};
    rm.orig_addr = buffer_addr;
    rm.prio = priority;

    // find packets starting from first_seq to last_seq, allocate new buffers and copy the data, and enqueue them to _retransmission_queue
    while (buffer_addr != POISON_64) {
        buffer_next = homa_txmeta_get_buffer_next(_tctx->txrx_xsk_info[0]->umem_area, buffer_addr);
        if (buffer_next == POISON_64)
            orig_buffer_tail = buffer_addr;
        if (seq >= first_seq && seq <= last_seq) {
            if (thread_bcache_check(&_tctx->iobuffer, 1) != 1) {
                break;
            }
            new_buffer = thread_bcache_cons(&_tctx->iobuffer);
            new_buffer = add_offset_tx_frame(new_buffer);

            // put new_buffer to the tail of the buffer chain
            if (buffer_tail != POISON_64)
                homa_txmeta_set_buffer_next(_tctx->txrx_xsk_info[0]->umem_area, buffer_tail, new_buffer);
            homa_txmeta_set_buffer_next(_tctx->txrx_xsk_info[0]->umem_area, new_buffer, POISON_64);
            buffer_tail = new_buffer;

            char *pkt = reinterpret_cast<char *>(xsk_umem__get_data(_tctx->txrx_xsk_info[0]->umem_area, new_buffer));
            char *old_pkt = reinterpret_cast<char *>(xsk_umem__get_data(_tctx->txrx_xsk_info[0]->umem_area, buffer_addr));
            memcpy(pkt, old_pkt, std::min((uint32_t)HOMA_MSS, length));
            struct iphdr *iph = reinterpret_cast<struct iphdr *>(pkt + sizeof(struct ethhdr));
            iph->tos = priority << 5;
            struct data_header *d = reinterpret_cast<struct data_header *>(pkt + sizeof(struct ethhdr) + sizeof(struct iphdr));
            d->retransmit = 1;            
            if (first) {
                rm.addr = new_buffer;
                first = false;
            }
            rm.len = std::min((uint32_t)HOMA_MSS, length);
            rm.nr_pkt++;

            if (length > HOMA_MSS)
                length -= HOMA_MSS;
        }
        buffer_addr = buffer_next;
        seq++;
    }

    if (!first) {
        homa_txmeta_set_buffer_next(_tctx->txrx_xsk_info[0]->umem_area, orig_buffer_tail, rm.addr);
        _retransmission_queue.push(rm);
    }
}

void RpcSocket::poll_nic_rx(void) 
{
    struct thread_bcache *bc = &_tctx->iobuffer;
    unsigned int nr_nic_queues = _tctx->actx->nr_nic_queues;
    unsigned int quantum;
    unsigned int active_qidx[nr_nic_queues];
    int nfds = 0;
    unsigned int qidx, i, rcvd;
    unsigned int idx_rx = 0, idx_fq = 0;
    struct xsk_socket_info *xsk_info;
    struct xsk_ring_cons *rx;
    struct xsk_ring_prod *fq;

    /* find queues from which packets are received */
    for (qidx = 0; qidx < nr_nic_queues; qidx++) {
        xsk_info = XSK_INFO(qidx);
        rx = &xsk_info->rx;
        if (!xsk_rxring_empty(rx)) {
            active_qidx[nfds++] = qidx;
        }
    }

    if (nfds == 0) {
        return;
    }
    
    quantum = RX_BATCH_SIZE / nfds;

    /* handle received packets */
    for (int j = 0; j < nfds; j++) {
        qidx = active_qidx[j];
        xsk_info = XSK_INFO(qidx);
        rx = &xsk_info->rx;

        xsk_info->deficit += quantum;
        rcvd = xsk_ring_cons__peek(rx, std::min(xsk_info->deficit, RX_BATCH_SIZE), &idx_rx);
        xsk_info->deficit -= rcvd;

        for (i = 0; i < rcvd; i++) {
            const struct xdp_desc *desc = xsk_ring_cons__rx_desc(rx, idx_rx + i);
            uint64_t addr = xsk_umem__add_offset_to_addr(desc->addr);
            char *pkt = reinterpret_cast<char *>(xsk_umem__get_data(xsk_info->umem_area, addr));

            /* extract remote ip, rpcid and type */
            struct iphdr *iph = reinterpret_cast<struct iphdr *>(pkt + sizeof(struct ethhdr));
            struct data_header *d = reinterpret_cast<struct data_header *>(pkt + sizeof(struct ethhdr) + sizeof(struct iphdr));
            auto remote_ip = __be32_to_cpu(iph->saddr);
            auto rpcid = local_id(__be64_to_cpu(d->common.sender_id));
            auto type = d->common.type;
            
            /* read rx metadata */
            uint32_t qid = homa_rxmeta_qid(pkt);
            uint64_t reap_client_buffer_addr = homa_rxmeta_reap_client_buffer(pkt);
            uint64_t reap_server_buffer_addr = homa_rxmeta_reap_server_buffer(pkt);

            /* record queues whose fill ring needs to be replenished */
            if (!(_tctx->txrx_xsk_info[_tctx->actx->qid2idx[qid]]->cached_needfill++))
                _tctx->cached_fqidx.push(_tctx->actx->qid2idx[qid]);

            /* filter non-DATA packets */
            if (unlikely(type != DATA)) {
                if (unlikely(type == RESEND)) {
                    if (reap_client_buffer_addr != POISON_64)
                        enqueue_reap_backlog(reap_client_buffer_addr);
                    if (reap_server_buffer_addr != POISON_64) {
                        if (!(homa_txmeta_get_flag(xsk_info->umem_area, reap_server_buffer_addr) & (FLAG_UNDER_REAP | FLAG_UNDER_RETRANSMISSION))) {
                            /* enqueue an RPC to retransmission queue only when it is not under reaping or retransmission */
                            homa_txmeta_set_flag(xsk_info->umem_area, reap_server_buffer_addr, FLAG_UNDER_RETRANSMISSION);
                            prepare_retransmission(reinterpret_cast<struct resend_header *>(d), reap_server_buffer_addr);
                        }
                    }
                }
                thread_bcache_prod(bc, addr);
                continue;
            }
            
            if (reap_client_buffer_addr != POISON_64)
                enqueue_reap_backlog(reap_client_buffer_addr);
            if (reap_server_buffer_addr != POISON_64)    
                enqueue_reap_backlog(reap_server_buffer_addr);

            if (client_rpc(rpcid))
                client_response(qidx, d);
            else if (_req_handler)
                server_request(qidx, d, remote_ip, rpcid);

            /* free the AF_XDP UMEM buffer */
            thread_bcache_prod(bc, addr);
        }

        /* finish receving packets from this queue */
        xsk_ring_cons__release(rx, rcvd);

        if (xsk_rxring_empty(rx))
            xsk_info->deficit = 0;
    }

    /* replenish fill rings */
    while (_tctx->cached_fqidx.pop(&qidx) == 0) {
        unsigned int need_fill = _tctx->txrx_xsk_info[qidx]->cached_needfill;
        auto fill_offset = _tctx->actx->uring[qidx].fill_offset;
        fq = FQ(qidx);
        FQ_LOCK(qidx);
        
        while (eTran_fq__reserve(fq, need_fill, &idx_fq, fill_offset) < need_fill) {
            /* this should not happen */
            kick_fq(_tctx->txrx_xsk_fd[qidx], fq, fill_offset);
            LOG_ERR("Can't reserve space in fill ring.\n");
        }
        unsigned int j;
        for (j = 0; j < need_fill; j++) {
            if (unlikely(thread_bcache_check(bc, 1) < 1)) {
                /* try again */
                if (thread_bcache_check(bc, 1) < 1) {
                    LOG_ERR("No buffer available for filling fill ring.\n");
                    break;
                }
            }
            *eTran_fq__fill_addr(fq, idx_fq++, fill_offset) = thread_bcache_cons(bc);
        }

        if (unlikely(j < need_fill)) {
            /* rollback */
            fq->cached_prod -= (need_fill - j);
        }

        eTran_fq__submit(fq, need_fill, fill_offset);
        
        FQ_UNLOCK(qidx);
        _tctx->txrx_xsk_info[qidx]->cached_needfill -= j;
        
        if (unlikely(_tctx->txrx_xsk_info[qidx]->cached_needfill)) {
            _tctx->cached_fqidx.cancel_pop();
            break;
        }
    }
}

void RpcSocket::poll_nic_rx_block(int timeout) 
{
    struct thread_bcache *bc = &_tctx->iobuffer;
    unsigned int nr_nic_queues = _tctx->actx->nr_nic_queues;
    unsigned int quantum;
    int nfds = 0;
    unsigned int qidx, i, rcvd;
    unsigned int idx_rx = 0, idx_fq = 0;
    struct xsk_socket_info *xsk_info;
    struct xsk_ring_cons *rx;
    struct xsk_ring_prod *fq;

    /* find queues from which packets are received */
    struct epoll_event events[nr_nic_queues];
    timeout = (_pending_request_queue.empty() && 
                    _retransmission_queue.empty() && 
                    _pending_response_queue.empty() && 
                    _request_queue.size() == 0 &&
                    _response_queue.size() == 0 && 
                    _reap_backlog.size() == 0) ? timeout : 0;
    nfds = epoll_wait(_tctx->epfd, events, nr_nic_queues, timeout);
    if (nfds <= 0)
        return;
    
    quantum = RX_BATCH_SIZE / nfds;

    /* handle received packets */
    for (int j = 0; j < nfds; j++) {
        if (unlikely(events[j].data.fd == _tctx->evfd)) {
            LOG_ERR("epoll_wait() returned event on eventfd.\n");
            continue;
        }
        qidx = _tctx->txrx_xsk_fd_to_idx[events[j].data.fd];
        xsk_info = XSK_INFO(qidx);
        rx = &xsk_info->rx;

        xsk_info->deficit += quantum;
        rcvd = xsk_ring_cons__peek(rx, std::min(xsk_info->deficit, RX_BATCH_SIZE), &idx_rx);
        xsk_info->deficit -= rcvd;

        for (i = 0; i < rcvd; i++) {
            const struct xdp_desc *desc = xsk_ring_cons__rx_desc(rx, idx_rx + i);
            uint64_t addr = xsk_umem__add_offset_to_addr(desc->addr);
            char *pkt = reinterpret_cast<char *>(xsk_umem__get_data(xsk_info->umem_area, addr));

            /* extract remote ip, rpcid and type */
            struct iphdr *iph = reinterpret_cast<struct iphdr *>(pkt + sizeof(struct ethhdr));
            struct data_header *d = reinterpret_cast<struct data_header *>(pkt + sizeof(struct ethhdr) + sizeof(struct iphdr));
            auto remote_ip = __be32_to_cpu(iph->saddr);
            auto rpcid = local_id(__be64_to_cpu(d->common.sender_id));
            auto type = d->common.type;
            
            /* read rx metadata */
            uint32_t qid = homa_rxmeta_qid(pkt);
            uint64_t reap_client_buffer_addr = homa_rxmeta_reap_client_buffer(pkt);
            uint64_t reap_server_buffer_addr = homa_rxmeta_reap_server_buffer(pkt);

            /* record queues whose fill ring needs to be replenished */
            if (!(_tctx->txrx_xsk_info[_tctx->actx->qid2idx[qid]]->cached_needfill++))
                _tctx->cached_fqidx.push(_tctx->actx->qid2idx[qid]);

            /* filter non-DATA packets */
            if (unlikely(type != DATA)) {
                if (unlikely(type == RESEND)) {
                    if (reap_client_buffer_addr != POISON_64)
                        enqueue_reap_backlog(reap_client_buffer_addr);
                    if (reap_server_buffer_addr != POISON_64)
                        prepare_retransmission(reinterpret_cast<struct resend_header *>(d), reap_server_buffer_addr);
                }
                thread_bcache_prod(bc, addr);
                continue;
            }
            
            if (reap_client_buffer_addr != POISON_64)
                enqueue_reap_backlog(reap_client_buffer_addr);
            if (reap_server_buffer_addr != POISON_64)    
                enqueue_reap_backlog(reap_server_buffer_addr);

            if (client_rpc(rpcid))
                client_response(qidx, d);
            else if (_req_handler)
                server_request(qidx, d, remote_ip, rpcid);

            /* free the AF_XDP UMEM buffer */
            thread_bcache_prod(bc, addr);
        }

        /* finish receving packets from this queue */
        xsk_ring_cons__release(rx, rcvd);

        if (xsk_rxring_empty(rx))
            xsk_info->deficit = 0;
    }

    /* replenish fill rings */
    while (_tctx->cached_fqidx.pop(&qidx) == 0) {
        unsigned int need_fill = _tctx->txrx_xsk_info[qidx]->cached_needfill;
        auto fill_offset = _tctx->actx->uring[qidx].fill_offset;
        fq = FQ(qidx);
        FQ_LOCK(qidx);
        
        while (eTran_fq__reserve(fq, need_fill, &idx_fq, fill_offset) < need_fill) {
            /* this should not happen */
            kick_fq(_tctx->txrx_xsk_fd[qidx], fq, fill_offset);
            LOG_ERR("Can't reserve space in fill ring.\n");
        }
        unsigned int j;
        for (j = 0; j < need_fill; j++) {
            if (unlikely(thread_bcache_check(bc, 1) < 1)) {
                /* try again */
                if (thread_bcache_check(bc, 1) < 1) {
                    LOG_ERR("No buffer available for filling fill ring.\n");
                    break;
                }
            }
            *eTran_fq__fill_addr(fq, idx_fq++, fill_offset) = thread_bcache_cons(bc);
        }

        if (unlikely(j < need_fill)) {
            /* rollback */
            fq->cached_prod -= (need_fill - j);
        }

        eTran_fq__submit(fq, need_fill, fill_offset);
        
        FQ_UNLOCK(qidx);
        _tctx->txrx_xsk_info[qidx]->cached_needfill -= j;
        
        if (unlikely(_tctx->txrx_xsk_info[qidx]->cached_needfill)) {
            _tctx->cached_fqidx.cancel_pop();
            break;
        }
    }
}

void RpcSocket::flush_rpc_response_queue(void)
{
    unsigned int budget_use = 0;

    /* if we have work in flush _pending_response_queue, process them first */
    while (!_pending_response_queue.empty()) {
        InternalReqMeta &req_meta = _pending_response_queue.front();
        if (message_tx_segmentation(&req_meta, req_meta.slot_idx, &budget_use) == 0) {
            /* segmentation is not finished for this message due to budget limit, 
             * re-enqueue the message to the head and return
             */
            return;
        }
        /* this rpc has been successfully segmented and transmitted, free bounce message buffer */
        free_buffer(req_meta.buffer);

        _pending_response_queue.pop();
        
        if (budget_use >= TX_BATCH_SIZE)
            return;
    }

    while (!_response_queue.empty()) {
        ReqHandle req_handle = _response_queue.front();
        _response_queue.pop();
        InternalReqMeta req_meta = {
            .buffer = req_handle.buffer,
            .dest_addr = req_handle.dest_addr,
            .rpcid = req_handle.rpcid,
            .qidx = req_handle.qidx,
        };
        req_meta.seq = 0;
        req_meta.prev_buffer_addr = POISON_64;
        req_meta.slot_idx = req_handle.slot_idx;
        if (message_tx_segmentation(&req_meta, req_meta.slot_idx, &budget_use) == 0) {
            /* segmentation is not finished for this message due to budget limit, 
             * re-enqueue the message to the head and return
             */
            _pending_response_queue.push(req_meta);
            break;
        }
        /* this rpc has been successfully segmented and transmitted, free bounce message buffer */
        free_buffer(req_meta.buffer);

        if (budget_use >= TX_BATCH_SIZE)
            break;
    }
}

void RpcSocket::flush_rpc_request_queue(void)
{
    unsigned int slot_idx;
    unsigned int budget_use = 0;

    /* if we have available slots for pending RPC requests, enqueue them first */
    while (_pending_request_queue.size() && _available_slots.size()) {
        InternalReqArgs req_args = _pending_request_queue.front();
        enqueue_request(req_args.buffer, &req_args.dest_addr, req_args.cont_handler);
        _pending_request_queue.pop();
    }

    while(1) {
        
        if (dequeue_rpc_request(&slot_idx) < 0) 
            break;
        
        InternalReqMeta *req_meta = &_reqmeta_slots[slot_idx];
        
        if (message_tx_segmentation(req_meta, slot_idx, &budget_use) == 0) {
            /* segmentation is not finished for this message due to budget limit, 
             * re-enqueue the message to the head and break
             */
            cancel_dequeue_rpc_request();
            break;
        }
        /* this rpc has been successfully segmented and transmitted, free bounce message buffer */
        free_buffer(req_meta->buffer);
        /* check if we exceed the budget */
        if (budget_use >= TX_BATCH_SIZE)
            break;
    }
}

void RpcSocket::enqueue_response(ReqHandle *req_handle, Buffer buffer)
{
    ReqHandle n(buffer, req_handle->dest_addr, req_handle->rpcid, req_handle->slot_idx, req_handle->qidx);
    _response_queue.emplace(n);

    if (_response_queue.size() > (int)TX_BATCH_SIZE)
        flush_rpc_response_queue();
}

void RpcSocket::enqueue_request(Buffer buffer, struct sockaddr_in *dest_addr, ContHandlerType cont_handler) {
    unsigned int slot_idx = 0;
    /* allocate a slot of request metadata for transmission */
    if (_available_slots.pop(&slot_idx)) {
        /* unfortunately, no available slots for this request, so enqueue to backlog queue */
        _pending_request_queue.push(InternalReqArgs(buffer, *dest_addr, cont_handler));
        return;
    }
    _reqmeta_slots[slot_idx].buffer = buffer;
    _reqmeta_slots[slot_idx].dest_addr = *dest_addr;
    _reqmeta_slots[slot_idx].rpcid = _next_rpcid;
    update_rpcid();
    _reqmeta_slots[slot_idx].seq = 0;
    _reqmeta_slots[slot_idx].prev_buffer_addr = POISON_64;
    _reqmeta_slots[slot_idx].cont_handler = cont_handler;

    internal_enqueue_request(slot_idx);

    if (_request_queue.size() > (int)TX_BATCH_SIZE)
        flush_rpc_request_queue();
}

// return 1: finish the message, 0: don't finish due to budget limit
int RpcSocket::message_tx_segmentation(InternalReqMeta *req_meta, unsigned int slot_idx, unsigned int *send_out)
{
    Buffer buffer = req_meta->buffer;
    struct sockaddr_in *dest_addr = &req_meta->dest_addr;
    
    size_t size = buffer.actual_size;
    unsigned int message_length = (unsigned int)size;

    // TODO: we need a reasonable way to choose qidx
    int qidx = (req_meta->qidx == UINT8_MAX || req_meta->buffer.actual_size > HOMA_MSS) ? -1 : req_meta->qidx;
    if (qidx == -1)
        qidx = get_qidx_with_hash(dest_addr->sin_addr.s_addr, dest_addr->sin_port, _local_addr.sin_addr.s_addr, 
            _local_port, _tctx->actx->nr_nic_queues);

    /* continue from the last segment */
    unsigned int copy_offset = req_meta->seq * HOMA_MSS;
    size -= copy_offset;
    
    /* figure out how many buffers are needed */
    unsigned int nr_buffers = homa_get_nr_buffers_from_len(size);

    struct xsk_socket_info *xsk_info = _tctx->txrx_xsk_info[qidx];
    struct thread_bcache *bc = &_tctx->iobuffer;
    struct xsk_ring_prod *tx = &xsk_info->tx;
    unsigned int idx_tx = 0;

    unsigned int plen;

    /* actual buffers can be sent in one call */
    nr_buffers = std::min(nr_buffers, TX_BATCH_SIZE);
    
    unsigned int i;
    for (i = 0; i < nr_buffers; i++) {

        /* check if we have buffers available */
        if (thread_bcache_check(bc, 1) < 1) {
            /* try again */
            if (thread_bcache_check(bc, 1) < 1)
                break;
        }

        if (xsk_ring_prod__reserve(tx, 1, &idx_tx) < 1) {
            /* AF_XDP tx ring is busy, kick NAPI */
            kick_tx(_tctx->txrx_xsk_fd[qidx], tx);
            /* try again */
            if (xsk_ring_prod__reserve(tx, 1, &idx_tx) < 1)
                break;
        }

        struct xdp_desc *desc = xsk_ring_prod__tx_desc(tx, idx_tx);
        uint64_t addr = thread_bcache_cons(bc);
        addr = add_offset_tx_frame(addr);
        char *pkt = reinterpret_cast<char *>(xsk_umem__get_data(xsk_info->umem_area, addr));
        
        homa_txmeta_clear_all(xsk_info->umem_area, addr);
        
        /* mark this packet is from data path */
        homa_txmeta_set_from_slowpath(xsk_info->umem_area, addr, 0);
        /* store the buffer address in packet's headroom */
        homa_txmeta_set_buffer_addr(xsk_info->umem_area, addr, addr);
        /* store the next buffer address in packet's headroom */
        homa_txmeta_set_buffer_next(xsk_info->umem_area, addr, POISON_64);
        
        if (req_meta->prev_buffer_addr != POISON_64) {
            /* link the buffer with the previous buffer */
            homa_txmeta_set_buffer_next(xsk_info->umem_area, req_meta->prev_buffer_addr, addr);
        }
        req_meta->prev_buffer_addr = addr;

        /* fill IP header */
        struct iphdr *iph = reinterpret_cast<struct iphdr *>(pkt + sizeof(struct ethhdr));
        iph->saddr = _local_addr.sin_addr.s_addr;
        iph->daddr = dest_addr->sin_addr.s_addr;
        iph->protocol = IPPROTO_HOMA;

        /* fill Homa data header */
        struct data_header *d = reinterpret_cast<struct data_header *>(pkt + sizeof(struct ethhdr) + sizeof(struct iphdr));
        d->common.sport = __cpu_to_be16(_local_port);
        d->common.dport = dest_addr->sin_port;
        d->common.doff = (sizeof(struct data_header) - sizeof(struct data_segment)) >> 2;
        d->common.type = DATA;
        d->common.seq = __cpu_to_be16(req_meta->seq);
        req_meta->seq++;
        d->common.sender_id = __cpu_to_be64(req_meta->rpcid);

        d->message_length = __cpu_to_be32(message_length);
        d->retransmit = 0;
        /* we use this unused1 field to store the slot_idx, 
         * this can help us to find the req_meta slot when we receive the response 
         */
        d->unused1 = slot_idx;

        /* the following two fileds are written by XDP_EGRESS */
        d->incoming = 0;
        d->cutoff_version = 0;
        
        plen = std::min((size_t)HOMA_MSS, size);
        
        d->seg.offset = __cpu_to_be32(copy_offset);
        d->seg.segment_length = __cpu_to_be32(plen);
        d->seg.ack.rpcid = 0;
        d->seg.ack.dport = 0;
        d->seg.ack.sport = 0;

        /* copy data */
        pkt += HOMA_PAYLOAD_OFFSET;
        memcpy(pkt, buffer._buf + copy_offset, plen);
        
        copy_offset += plen;
        size -= plen;
        
        /* fill AF_XDP descriptor */
        desc->addr = addr;
        desc->len = HOMA_PAYLOAD_OFFSET + plen;
        desc->options = XDP_EGRESS_NO_COMP;
    }

    xsk_ring_prod__submit(tx, i);
    
    kick_tx(_tctx->txrx_xsk_fd[qidx], tx);

    *send_out += i;

    return copy_offset >= buffer.actual_size ? 1 : 0;
}