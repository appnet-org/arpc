#pragma once

#include <sys/epoll.h>

#include <base/lrpc.h>
#include <intf/intf.h>
#include <tcp_if.h>
#include <homa_if.h>
#include <xsk_if.h>
#include <stddef.h>
#include <xskbp/xsk_buffer_pool.h>
#include <base/mem_pool.h>

#include <unordered_map>
#include <list>

constexpr unsigned int MAX_APP_THREADS = 20;

// size must be power of 2
template <typename T>
class fixed_size_vector_64k {
public:
    fixed_size_vector_64k() : head(0), tail(0) {
        slot_mask = 0;
    }
    fixed_size_vector_64k(unsigned int max_slot) : head(0), tail(0) {
        slot_mask = max_slot - 1;
        for (unsigned i = 0; i < (slot_mask + 1); i++) {
            _vector[i] = i;
        }
    }

    inline int push(T e) {
        if (head == ((tail + 1) & slot_mask)) {
            return -1;
        }
        _vector[tail] = e;
        tail = (tail + 1) & slot_mask;
        return 0;
    }

    inline int pop(T *e) {
        if (head == tail) {
            return -1;
        }
        *e = _vector[head];
        head = (head + 1) & slot_mask;
        return 0;
    }

    inline unsigned int dump_head(void) {
        return head;
    }

    inline unsigned int dump_tail(void) {
        return tail;
    }

    inline int size(void) {
        return (tail - head) & slot_mask;
    }

    inline void cancel_pop(void) {
        head = (head - 1) & slot_mask;
    }

private:
    T _vector[65536];
    unsigned int head;
    unsigned int tail;
    unsigned int slot_mask;
};

template <typename T>
class fixed_size_vector_32 {
public:
    fixed_size_vector_32() : head(0), tail(0) {
        slot_mask = 0;
    }
    fixed_size_vector_32(unsigned int max_slot) : head(0), tail(0) {
        slot_mask = max_slot - 1;
    }

    inline int push(T e) {
        if (head == ((tail + 1) & slot_mask)) {
            return -1;
        }
        _vector[tail] = e;
        tail = (tail + 1) & slot_mask;
        return 0;
    }

    inline int pop(T *e) {
        if (head == tail) {
            return -1;
        }
        *e = _vector[head];
        head = (head + 1) & slot_mask;
        return 0;
    }

    inline unsigned int dump_head(void) {
        return head;
    }

    inline unsigned int dump_tail(void) {
        return tail;
    }

    inline int size(void) {
        return (tail - head) & slot_mask;
    }

    inline void cancel_pop(void) {
        head = (head - 1) & slot_mask;
    }

private:
    T _vector[32];
    unsigned int head;
    unsigned int tail;
    unsigned int slot_mask;
};

struct eTran_cfg
{
    int proto;
    unsigned int nr_nic_queues;
    unsigned int nr_app_threads;
};

struct app_ctx_per_thread {
    uint32_t tid;
    
    struct app_ctx *actx;

    // FIXME
    unsigned int txrx_xsk_fd_to_idx[1024 * 1024];

    // TXRX-only XSK fd
    int txrx_xsk_fd[MAX_NIC_QUEUES];

    struct xsk_socket_info *txrx_xsk_info[MAX_NIC_QUEUES];

    struct thread_bcache iobuffer;

    unsigned int cqidx[MAX_NIC_QUEUES];
    unsigned int cqk;
    unsigned int fqidx[MAX_NIC_QUEUES];
    unsigned int fqk;
    fixed_size_vector_32<unsigned int> cached_fqidx;

    unsigned int next_rcv_qidx;

    // per application thread lrpc channel
    // kernel recv_head_wb for channel_2
    // app recv_head_wb for channel_1
    // channel_1:    kernel --> app     
    // channel_2:    app    --> kernel
    struct shm_wrapper *lrpc;

    struct lrpc_chan_in app_in;
    struct lrpc_chan_out app_out;

    int evfd;

    unsigned int pending_eventfd_work;

    int epfd;

    std::unordered_map<struct eTran_tcp_flow_tuple, struct eTrantcp_connection *, eTran_tcp_flow_tuple_hash, eTran_tcp_flow_tuple_equal> open_conns;

    Mempool *mp;

    // connections that need to free packets
    std::list<struct eTrantcp_connection *> free_pending_conns;

    // connections that need to synchronize with eBPF
    // send a dummy packet
    std::list<struct eTrantcp_connection *> rx_bump_pending_conns;

    // connections that need to retransmit
    std::list<std::pair<struct eTrantcp_connection *, uint32_t> > retransmission_conns;
};

// application context in application
struct app_ctx {
    // the fd of Unix domain socket to communicate with the kernel
    int fd;

    // if we have allocated resources for this application
    bool done;

    // microkernel's pid
    int pid;

    // application's per-thread context in kernel
    struct app_ctx_per_thread tctx[MAX_APP_THREADS];

    // The name is a bit ambiguous, because we only have one UMEM,
    // it actually refers to the first XSK fd bound to that UMEM
    int umem_fd[MAX_NIC_QUEUES];

    struct xsk_umem_rings uring[MAX_NIC_QUEUES];

    // the real nic queue ids the application is using
    unsigned int nic_qid[MAX_NIC_QUEUES];

    // convert nic queue id to index
    unsigned int qid2idx[MAX_NIC_QUEUES];

    // the NIC interface index
    int ifindex;

    // the protocol the application is using
    int proto;

    // the number of queues the application is using
    unsigned int nr_nic_queues;

    // the number of threads the application is using
    unsigned int nr_app_threads;

    struct buffer_pool_wrapper bpw;

    // constructor
    app_ctx() : fd(-1), done(false), nr_nic_queues(0), nr_app_threads(0)
    {
        for (unsigned int i = 0; i < MAX_NIC_QUEUES; i++)
        {
            uring[i] = {0};
            nic_qid[i] = -1;
        }
        for (unsigned int i = 0; i < MAX_APP_THREADS; i++)
        {
            for (unsigned int j = 0; j < MAX_NIC_QUEUES; j++)
            {
                tctx[i].txrx_xsk_fd[j] = -1;
                tctx[i].txrx_xsk_info[j] = NULL;
            }
            tctx[i].cqk = 0;
            tctx[i].fqk = 0;
            tctx[i].cached_fqidx = fixed_size_vector_32<unsigned int>(32);
            tctx[i].mp = NULL;
            tctx[i].next_rcv_qidx = 0;
            tctx[i].lrpc = NULL;
            tctx[i].app_in = {0};
            tctx[i].app_out = {0};
            tctx[i].actx = this;
            tctx[i].evfd = -1;
        }
        bpw = {0};
    }

    // destructor
    ~app_ctx()
    {
        if (fd >= 0)
            close(fd);
    }
};
