#pragma once

/**
 * @file xsk_buffer_pool.h
 * @brief Buffer pool for each AF_XDP socket UMEM
 */
#include <sys/resource.h>
#include <sys/mman.h>
#include <sys/types.h>

#include <stdint.h>
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#include <errno.h>
#include <pthread.h>
#include <unistd.h>
#include <fcntl.h>
#include <sys/stat.h>
#include <unistd.h>

#include <bpf/libbpf.h>
#include <bpf/btf.h>
#include <xdp/xsk.h>

#include <base/compiler.h>
#include <base/lock.h>
#include <shm/shm_wrapper.h>

/* Maximum NIC queues per buffer pool */
#define MAX_NIC_QUEUES 20
/* Number of frames in each UMEM */
const unsigned int umem_num_frames = 64 * XSK_RING_PROD__DEFAULT_NUM_DESCS;
/* Size of each frame in UMEM */
const unsigned int umem_frame_size = XSK_UMEM__DEFAULT_FRAME_SIZE;
/* Headroom for each frame in UMEM */
const unsigned int frame_headroom = 0;
/* Number of buffers in each slab */
const unsigned int buffers_per_slab = XSK_RING_PROD__DEFAULT_NUM_DESCS * 2;
/* Maximum number of users for each buffer pool */
const unsigned int users_max = 32;

/********************** Buffer Pool ***********************/
struct buffer_pool_params {
    unsigned int bp_id;
    unsigned int frame_headroom;
    int mmap_flags;

    uint64_t nr_buffers;
    uint64_t buffer_size;
    uint32_t nr_users_max;
    uint32_t nr_buffers_per_slab;
};

struct buffer_pool {
    struct xsk_ring_prod fq[MAX_NIC_QUEUES];
    spinlock_t fq_lock[MAX_NIC_QUEUES];
    
    struct xsk_ring_cons cq[MAX_NIC_QUEUES];
    spinlock_t cq_lock[MAX_NIC_QUEUES];

    spinlock_t spinlock;

    shmptr_t slabs;
    shmptr_t slabs_swap;

    shmptr_t buffers;
    shmptr_t buffers_swap;

    uint64_t nr_slabs;
    uint64_t nr_slabs_swap;

    uint64_t nr_slabs_avail;
    uint64_t nr_slabs_swap_avail;

    uint64_t nr_buffers;
    uint64_t total_size;
    uint64_t total_umem_size;

    struct xsk_umem *umem;
    /* These two rings are only used for xsk_umem__create() */
    struct xsk_ring_prod fake_fq;
    struct xsk_ring_cons fake_cq;
};

struct buffer_pool_wrapper {
    struct buffer_pool_params bp_params;
    struct buffer_pool *bp; // points to shm_bp->addr, for convenience
    struct shm_wrapper *shm_bp;
    struct shm_wrapper *shm_umem;
};

int bp_init(struct buffer_pool_wrapper *bpw, unsigned int app_id, const char *bp_prefix, const char *umem_prefix);
void bp_free(struct buffer_pool_wrapper *bpw);
/********************** Buffer Pool ***********************/

/**************** Thread-local buffer cache ****************/
struct thread_bcache {
    // read-only fields
    struct buffer_pool_wrapper *bpw;

    // read-write fields
    uint64_t *slabs_cons;
    uint64_t *slabs_prod;

    uint64_t nr_buffers_cons;
    uint64_t nr_buffers_prod;

};
int thread_bcache_create(struct buffer_pool_wrapper *bpw, struct thread_bcache *bc);
void thread_bcache_free(struct thread_bcache *bc);

void thread_bcache_prod(struct thread_bcache *bc, uint64_t buffer);
uint64_t thread_bcache_check(struct thread_bcache *bc, uint64_t nr_req_buffers);
uint64_t thread_bcache_cons(struct thread_bcache *bc);
/**************** Thread-local buffer cache ****************/

static inline uint64_t add_offset_tx_frame(uint64_t frame_addr)
{
    return frame_addr + XDP_PACKET_HEADROOM + frame_headroom;
}

