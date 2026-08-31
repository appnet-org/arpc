#pragma once
#include <arpa/inet.h>

#include <tran_def/homa.h>

#include <xsk_if.h>
#include <app_if.h>
#include <eTran_common.h>
#include <eTran_socket.h>

#include <queue>
#include <functional>
#include <unordered_map>

/* we limit that one socket can send at most MAX_OUTSTANDING_RPC outstanding RPCs */
constexpr unsigned int MAX_OUTSTANDING_RPC = 256;

/* forward declarations */
class RpcSocket;
class MsgBufferPool;
struct ReqHandle;
struct InternalReqHandle;
struct ContHandle;
struct InternalReqMeta;

using ContHandlerType = std::function<void(ContHandle *cont_handle, void *context)>;
using ReqHandlerType = std::function<void(ReqHandle *req_handle, void *context)>;

/* input context for RPC request handler, the content can not be modified */
struct ReqHandle {
    const Buffer buffer;
    const struct sockaddr_in dest_addr;
    const uint64_t rpcid;
    const uint8_t slot_idx;
    const uint8_t qidx;

    ReqHandle(const Buffer buffer, const struct sockaddr_in dest_addr, const uint64_t rpcid, const uint8_t slot_idx, const uint8_t qidx) : buffer(buffer), dest_addr(dest_addr), rpcid(rpcid), slot_idx(slot_idx), qidx(qidx) {}
};

/* used by internal, which is opaque to application */
struct InternalReqHandle {
    Buffer buffer;
    struct sockaddr_in dest_addr;
    uint64_t rpcid;
    uint8_t slot_idx;
    uint8_t qidx;
    uint32_t count;

    InternalReqHandle(Buffer buffer, struct sockaddr_in dest_addr, uint64_t rpcid, uint8_t slot_idx, uint8_t qidx, uint32_t count) : buffer(buffer), dest_addr(dest_addr), rpcid(rpcid), slot_idx(slot_idx), qidx(qidx), count(count) {}
};

/* input context for RPC continue handler, the content can not be modified */
struct ContHandle {
    const Buffer buffer;
    const struct sockaddr_in dest_addr;
    const uint64_t rpcid;

    ContHandle(const Buffer buffer, const struct sockaddr_in dest_addr, const uint64_t rpcid) : buffer(buffer), dest_addr(dest_addr), rpcid(rpcid) {}
};

/* used by internal for transmission, which is opaque to application */
struct InternalReqMeta {
    Buffer buffer;
    struct sockaddr_in dest_addr;
    uint64_t rpcid;
    uint16_t seq;
    uint64_t prev_buffer_addr;
    ContHandlerType cont_handler;
    bool receiving;
    uint8_t qidx;
    uint8_t slot_idx;
    uint32_t recv_count;
};

/* used by internal for enqueuing request, which is opaque to application */
struct InternalReqArgs {
    Buffer buffer;
    struct sockaddr_in dest_addr;
    ContHandlerType cont_handler;

    InternalReqArgs(Buffer buffer, struct sockaddr_in dest_addr, ContHandlerType cont_handler) : buffer(buffer), dest_addr(dest_addr), cont_handler(cont_handler) {}
};

/* used by internal for retransmission, which is opaque to application */
struct InternalResendMeta {
    // the original first buffer address of message
    uint64_t orig_addr;
    // the first retransmission buffer address
    uint64_t addr;
    // last packet's len
    uint32_t len;
    // how many packets
    uint16_t nr_pkt;
    // priority
    uint8_t prio;
};

/**
 * @brief RPC socket class
 * @note This class is not thread-safe.
 * @param app_context Application context passed to RPC handler
 * @param local_ip_str Local IPv4 address, format: "xxx.xxx.xxx.xxx"
 * @param local_port Local port
 */
class RpcSocket
{
  public:
    RpcSocket(void *app_context, std::string local_ip_str, uint16_t local_port)
    {
        _app_context = app_context;
        
        _local_port = local_port;

        _mempool = new Mempool(MBytes(256));
        if (!_mempool) {
            throw std::runtime_error("Failed to create memory pool.");
        }

        _next_rpcid = 0;

        _available_slots = fixed_size_vector_64k<unsigned int>(MAX_OUTSTANDING_RPC);
        
        _reqmeta_slots = new InternalReqMeta[MAX_OUTSTANDING_RPC];
        for (unsigned int i = 0; i < MAX_OUTSTANDING_RPC; i++)
        {
            _available_slots.push(i);
            _reqmeta_slots[i].qidx = UINT8_MAX;
        }

        _request_queue = fixed_size_vector_64k<unsigned int>(MAX_OUTSTANDING_RPC);

        _reap_backlog = fixed_size_vector_64k<uint64_t>(65536);

        _fd = eTran_socket(AF_INET, SOCK_DGRAM, IPPROTO_HOMA);
        if (_fd < 0) {
            throw std::runtime_error("Failed to create socket.");
        }
        struct sockaddr_in addr = {0};
        addr.sin_family = AF_INET;
        addr.sin_port = htons(local_port);
        inet_pton(AF_INET, local_ip_str.c_str(), &addr.sin_addr);
        _local_addr = addr;
        if (eTran_bind(_fd, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
            throw std::runtime_error("Failed to bind socket.");
        }

        _tctx = eTran_get_tctx();
        if (!_tctx) {
            throw std::runtime_error("Failed to get thread context.");
        }
    }

    ~RpcSocket()
    {
        delete _mempool;
        eTran_close(_fd);
    }

    /* called by application to set RPC request handler */
    inline void set_req_handler(ReqHandlerType handler)
    {
        _req_handler = handler;
    }

    /* called by application to allocate a message buffer for communication */
    inline Buffer alloc_buffer(size_t size)
    {
        return _mempool->alloc_buffer(size);
    }

    /* free a message buffer */
    inline void free_buffer(Buffer &buffer)
    {
       _mempool->free_buffer(buffer);
    }

    /* called by RPC request handler to enqueue response */
    void enqueue_response(ReqHandle *req_handle, Buffer buffer);

    /* called by application to enqueue request, a continue handler must be provided to handle response */
    void enqueue_request(Buffer buffer, struct sockaddr_in *dest_addr, ContHandlerType cont_handler);

    /* application must periodically call this function to handle all events */
    void run_event_loop(void);

    /* blocked version of run_event_loop() */
    void run_event_loop_block(int timeout);

  private:
    
    /* file descriptor of socket */
    int _fd;

    /* context for each application thread */
    struct app_ctx_per_thread *_tctx;

    /* context provided by application */
    void *_app_context;
    
    /* local address */
    struct sockaddr_in _local_addr;

    /* local port */
    uint16_t _local_port;

    /* next ID used by RPC, always even, starts from zero */
    uint64_t _next_rpcid;

    /* request metadata */
    InternalReqMeta *_reqmeta_slots;
    
    /* available slots in request metadata */
    fixed_size_vector_64k<unsigned int> _available_slots;

    /* RPC request queue consists of slots of request metadata */
    fixed_size_vector_64k<unsigned int> _request_queue;

    /* reap backlog queue consists of buffer address */
    fixed_size_vector_64k<uint64_t> _reap_backlog;

    /* retransmission queue */
    std::queue<struct InternalResendMeta> _retransmission_queue;

    /* RPC response queue consists of request handle */
    std::queue<struct ReqHandle> _response_queue;

    /* pending RPC responses that can't transmit immediately */
    std::queue<struct InternalReqMeta> _pending_response_queue;

    /* pending RPC requests due to no slots are available */
    std::queue<struct InternalReqArgs> _pending_request_queue;

    /* request handler registered by application */
    ReqHandlerType _req_handler;

    /* used by server to find corresponding RPC state when receiving multi-packet RPC request */
    std::unordered_map<struct eTran_homa_rpc_tuple, struct InternalReqHandle, 
        eTran_homa_rpc_tuple_hash, eTran_homa_rpc_tuple_equal> _req_handle_map;

    /* hugepage backed memory pool for messages */
    Mempool *_mempool;

    /* called when a new RPC request is enqueued */
    inline void update_rpcid(void)
    {
        _next_rpcid += 2;
    }

    /* truly enqueue a request to internal request queue */
    inline void internal_enqueue_request(unsigned int slot_idx)
    {
        _request_queue.push(slot_idx);
    }

    /* dequeue arequest from internal request queue */
    inline int dequeue_rpc_request(unsigned int *slot_idx)
    {
        return _request_queue.pop(slot_idx);
    }

    /* cancel the last dequeue operation */
    inline void cancel_dequeue_rpc_request(void)
    {
        _request_queue.cancel_pop();
    }

    /* free buffers used by transmitted messages to buffer pool 
     * each operation can free at most budget buffers
     * return the last buffer address (not released ) in the chain
    */
    inline int reap_rpc_buffers(uint64_t buffer_addr, unsigned int budget, unsigned int *total_budget_use)
    {
        struct thread_bcache *bc = &_tctx->iobuffer;
        uint64_t buffer_next;
        unsigned int budget_use = 0;
        while (buffer_addr != POISON_64) {
            buffer_next = homa_txmeta_get_buffer_next(_tctx->txrx_xsk_info[0]->umem_area, buffer_addr);
            
            homa_txmeta_clear_all(_tctx->txrx_xsk_info[0]->umem_area, buffer_addr);
            thread_bcache_prod(bc, buffer_addr);
            
            buffer_addr = buffer_next;
            budget_use++;
            if (budget_use >= budget) {
                break;
            }
        }

        *total_budget_use += budget_use;

        return buffer_addr;
    }

    /* flush the reap backlog queue */
    inline void flush_reap_backlog(void)
    {
        unsigned int budget = TX_BATCH_SIZE << 2;
        unsigned int total_budget_use = 0;
        uint64_t buffer_addr;
        while (_reap_backlog.pop(&buffer_addr) == 0) {
            buffer_addr = reap_rpc_buffers(buffer_addr, budget, &total_budget_use);
            if (buffer_addr != POISON_64) {
                
                enqueue_reap_backlog(buffer_addr);
                
                if (_reap_backlog.size() < 20 && (total_budget_use >= TX_BATCH_SIZE << 3)) 
                    break;
                else {
                    /* it seems that there are too many buffers to reap, continue even if the budget has been exhausted */
                    total_budget_use = 0;
                }
            }
        }
    }

    /* enqueue buffer address (i.e., the first buffer address of transmitted message) to reap backlog */
    inline void enqueue_reap_backlog(uint64_t buffer_addr)
    {
        homa_txmeta_set_flag(_tctx->txrx_xsk_info[0]->umem_area, buffer_addr, FLAG_UNDER_REAP);
        if (unlikely(homa_txmeta_get_flag(_tctx->txrx_xsk_info[0]->umem_area, buffer_addr) & FLAG_UNDER_RETRANSMISSION)) {
            /* this packet is under retransmission queue, don't enqueue it here */
            return;
        }
        _reap_backlog.push(buffer_addr);
    }

    /* convert RPC id to local RPC id,
     * for client RPC, this value is even,
     * for server RPC, this value is odd
     */
    inline uint64_t local_id(uint64_t rpcid)
    {
        return rpcid ^ 1;
    }

    /* return true if this rpc is a client RPC */
    inline bool client_rpc(uint64_t rpcid)
    {
        return (rpcid & 1) == 0;
    }

    /* handler for client RPC when receiving response */
    void client_response(uint8_t qidx, struct data_header *d);

    /* handler for server RPC when receiving request */
    void server_request(uint8_t qidx, struct data_header *d, uint32_t remote_ip, uint64_t rpcid);

    /* called by server RPC when multi-packet request is completed */
    inline void multi_pkt_req_complete(std::unordered_map<struct eTran_homa_rpc_tuple, struct InternalReqHandle, 
        eTran_homa_rpc_tuple_hash, eTran_homa_rpc_tuple_equal>::iterator it) {
        InternalReqHandle internal_req_handle = it->second;
        ReqHandle req_handle(internal_req_handle.buffer, internal_req_handle.dest_addr, internal_req_handle.rpcid, internal_req_handle.slot_idx, internal_req_handle.qidx);
        _req_handler(&req_handle, _app_context);
        /* free bounce message buffer */
       free_buffer(internal_req_handle.buffer);
    }

    /* called by server RPC when multi-packet response is completed */
    inline void single_pkt_req_complete(Buffer buffer, struct sockaddr_in *dest_addr, uint64_t rpcid, uint8_t slot_idx, uint8_t qidx) {
        ReqHandle req_handle(buffer, *dest_addr, rpcid, slot_idx, qidx);
        _req_handler(&req_handle, _app_context);
        /* no need to free bounce message buffer */
    }

    /* called by client RPC when multi-packet response is completed */
    inline void multi_pkt_resp_complete(InternalReqMeta *req_meta) {
        struct ContHandle cont_handle(req_meta->buffer, req_meta->dest_addr, req_meta->rpcid);
        Buffer b = req_meta->buffer;
        req_meta->cont_handler(&cont_handle, _app_context);
        /* free bounce message buffer */
        free_buffer(b);
    }

    /* called by client RPC when single-packet response is completed */
    inline void single_pkt_resp_complete(InternalReqMeta *req_meta) {
        struct ContHandle cont_handle(req_meta->buffer, req_meta->dest_addr, req_meta->rpcid);
        req_meta->cont_handler(&cont_handle, _app_context);
        /* no need to free bounce message buffer */
    }
    
    /* construct resend metadata according to RESEND header the first buffer address of transmitted message */
    void prepare_retransmission(struct resend_header *r, uint64_t buffer_addr);

    /* called by run_event_loop_xxx() to flush all queued RPC requests */
    void flush_rpc_request_queue(void);

    /* called by run_event_loop_xxx() to flush all queued RPC responses */
    void flush_rpc_response_queue(void);

    /* called by run_event_loop_xxx() to flush all queued RPC retransmissions */
    void flush_rpc_retransmission_queue(void);

    /* called by run_event_loop_xxx() to poll packets received from NIC */
    void poll_nic_rx(void);

    /* blocked version of poll_nic_rx() */
    void poll_nic_rx_block(int timeout);

    /* retransmit packets for one RPC according to resend metadata */
    int message_tx_retransmission(struct InternalResendMeta rm);

    /* segment messages to packets and transmit them for one RPC */
    int message_tx_segmentation(InternalReqMeta *req_meta, unsigned int slot_idx, unsigned int *send_out);
};
