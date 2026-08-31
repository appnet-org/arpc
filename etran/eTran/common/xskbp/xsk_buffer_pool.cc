#include "xsk_buffer_pool.h"

static void init_bp_params(struct buffer_pool_params *bp_params)
{
    bp_params->bp_id = 0;
    bp_params->mmap_flags = 0;
    bp_params->frame_headroom = frame_headroom;
	bp_params->nr_buffers = umem_num_frames;
	bp_params->buffer_size = umem_frame_size;
	bp_params->nr_users_max = users_max;
	bp_params->nr_buffers_per_slab = buffers_per_slab;
}

static int xsk_configure_umem(struct buffer_pool *bp, struct buffer_pool_params *bp_params, void *addr, size_t size)
{
    struct xsk_umem_config cfg = {
        .fill_size = XSK_RING_PROD__DEFAULT_NUM_DESCS * 2,
        .comp_size = XSK_RING_CONS__DEFAULT_NUM_DESCS,
        .frame_size = (__u32)bp_params->buffer_size,
        .frame_headroom = (__u32)bp_params->frame_headroom,
        .flags = (__u32)bp_params->mmap_flags
    };

    if (xsk_umem__create(&bp->umem, addr, size, &bp->fake_fq, &bp->fake_cq, &cfg)) {
        fprintf(stderr, "xsk_configure_umem: xsk_umem__create failed\n");
        return -1;
    }

    return 0;
}

int bp_init(struct buffer_pool_wrapper *bpw, unsigned int app_id, const char *bp_prefix, const char *umem_prefix)
{
    uint64_t nr_slabs, nr_slabs_swap, nr_buffers, nr_buffers_swap;
    uint64_t slab_size, slab_swap_size, buffer_size, buffer_swap_size;
    uint64_t total_size;
    uint8_t *p;
    uint64_t *bp_buffers_tmp;
    uint64_t **bp_slabs_tmp;
    uint64_t *bp_buffers_swap_tmp;
    uint64_t **bp_slabs_swap_tmp;
    struct buffer_pool *bp = NULL;
    struct shm_wrapper *shm_bp = NULL;
    struct shm_wrapper *shm_umem = NULL;

    if (!bpw)
        return -1;
    
    struct buffer_pool_params *bp_params = &bpw->bp_params;

    init_bp_params(bp_params);
    bp_params->bp_id = app_id;
    
    std::string bp_name = bp_prefix + std::to_string(bp_params->bp_id);
    std::string umem_name = umem_prefix + std::to_string(bp_params->bp_id);

    assert(bp_name.length() < 32);
    assert(umem_name.length() < 32);

    nr_slabs = (bp_params->nr_buffers + bp_params->nr_buffers_per_slab - 1) / bp_params->nr_buffers_per_slab;

    nr_buffers = nr_slabs * bp_params->nr_buffers_per_slab;
    bp_params->nr_buffers = nr_buffers; // update new nr_buffers

    nr_slabs_swap = bp_params->nr_users_max * 2; // cons,prod

    nr_buffers_swap = nr_slabs_swap * bp_params->nr_buffers_per_slab;

    slab_size = nr_slabs * sizeof(uint64_t *); // typeof slabs[i] is uint64_t *

    slab_swap_size = nr_slabs_swap * sizeof(uint64_t *);

    buffer_size = nr_buffers * sizeof(uint64_t); // typeof buffers[i] is uint64_t

    buffer_swap_size = nr_buffers_swap * sizeof(uint64_t);

    total_size = sizeof(struct buffer_pool) + slab_size + slab_swap_size + buffer_size + buffer_swap_size;

    /* Create shared memory for buffer pool */
    bpw->shm_bp = shm_wrapper_create(bp_name, total_size);
    if (!bpw->shm_bp) {
        fprintf(stderr, "bp_init: shm_wrapper_create failed\n");
        return -1;
    }
    printf("Create shared memory for buffer pool success.\n");
    shm_bp = bpw->shm_bp;
    
    memset(shm_bp->addr, 0, shm_bp->size);
    
    bpw->bp = (struct buffer_pool *)shm_bp->addr;

    bp = bpw->bp;
    
    /* Create shared memory for UMEM */
    bpw->shm_umem = shm_wrapper_create(umem_name, bp_params->nr_buffers * bp_params->buffer_size);
    if (!bpw->shm_umem) {
        fprintf(stderr, "bp_init: shm_wrapper_create failed\n");
        goto err;
    }
    printf("Create shared memory for umem success.\n");
    shm_umem = bpw->shm_umem;
    memset(shm_umem->addr, 0, shm_umem->size);

    bp->total_size = total_size;
    bp->total_umem_size = bp_params->nr_buffers * bp_params->buffer_size;
    
    /* Use libxdp to configure UMEM */
    if (xsk_configure_umem(bp, bp_params, shm_umem->addr, bp->total_umem_size)) {
        fprintf(stderr, "bp_init: xsk_configure_umem failed\n");
        goto err;
    }

    p = (uint8_t *)bp;

    bp->slabs = (shmptr_t)ptr_to_shmptr(&p[sizeof(struct buffer_pool)], shm_bp);
    bp->slabs_swap = (shmptr_t)ptr_to_shmptr(&p[sizeof(struct buffer_pool) + slab_size], shm_bp);
    bp->buffers = (shmptr_t)ptr_to_shmptr(&p[sizeof(struct buffer_pool) + slab_size + slab_swap_size], shm_bp);
    bp->buffers_swap = (shmptr_t)ptr_to_shmptr(&p[sizeof(struct buffer_pool) + slab_size + slab_swap_size + buffer_size], shm_bp);

    bp->nr_slabs = nr_slabs;
    bp->nr_slabs_swap = nr_slabs_swap;
    bp->nr_buffers = nr_buffers;

    bp_buffers_tmp = (uint64_t *)shmptr_to_ptr(bp->buffers, shm_bp);
    bp_slabs_tmp = (uint64_t **)shmptr_to_ptr(bp->slabs, shm_bp);
    for (size_t i = 0; i < nr_slabs; i++)
        bp_slabs_tmp[i] = (uint64_t *)ptr_to_shmptr(&bp_buffers_tmp[i * bp_params->nr_buffers_per_slab], shm_bp);
    bp->nr_slabs_avail = nr_slabs;

    bp_buffers_swap_tmp = (uint64_t *)shmptr_to_ptr(bp->buffers_swap, shm_bp);
    bp_slabs_swap_tmp = (uint64_t **)shmptr_to_ptr(bp->slabs_swap, shm_bp);
    for (size_t i = 0; i < nr_slabs_swap; i++)
        bp_slabs_swap_tmp[i] = (uint64_t *)ptr_to_shmptr(&bp_buffers_swap_tmp[i * bp_params->nr_buffers_per_slab], shm_bp);
    bp->nr_slabs_swap_avail = nr_slabs_swap;

    for (size_t i = 0; i < nr_buffers; i++)
        bp_buffers_tmp[i] = i * bp_params->buffer_size;

    spin_lock_init(&bp->spinlock);

    for(unsigned int i = 0; i < MAX_NIC_QUEUES; i++) {
        memset(&bp->fq[i], 0, sizeof(struct xsk_ring_prod));
        memset(&bp->cq[i], 0, sizeof(struct xsk_ring_cons));
        
        spin_lock_init(&bp->fq_lock[i]);
        spin_lock_init(&bp->cq_lock[i]);
    }

    printf("-----------------------BufferPool info-----------------------\n");
    printf("BufferPool total size: \t\t%u\n", shm_bp->size);
    printf("bp->nr_slabs: \t\t\t%lu\n", bp->nr_slabs);
    printf("bp->nr_slabs_swap: \t\t%lu\n", bp->nr_slabs_swap);
    printf("bp->nr_buffers: \t\t%lu\n", bp->nr_buffers);
    printf("bp->nr_slabs_avail: \t\t%lu\n", bp->nr_slabs_avail);
    printf("bp->nr_slabs_swap_avail: \t%lu\n", bp->nr_slabs_swap_avail);
    printf("-----------------------BufferPool info-----------------------\n");

    return 0;

err:
    if (shm_umem)
        shm_wrapper_destroy(shm_umem);
    if (shm_bp)
        shm_wrapper_destroy(shm_bp);
    return -1;
}

void bp_free(struct buffer_pool_wrapper *bpw)
{
    if (!bpw)
        return;
    
    if (bpw->shm_umem)
        shm_wrapper_destroy(bpw->shm_umem);
    if (bpw->shm_bp)
        shm_wrapper_destroy(bpw->shm_bp);
}

int thread_bcache_create(struct buffer_pool_wrapper *bpw, struct thread_bcache *bc)
{
    uint64_t nr_slabs_swap_avail;
    struct buffer_pool *bp;
    
    if (!bpw || !bc)
        return -1;

    bp = bpw->bp;
    if (!bp)
        return -1;
    
    memset(bc, 0, sizeof(struct thread_bcache));
    bc->bpw = bpw;

    spin_lock(&bp->spinlock);
    nr_slabs_swap_avail = bp->nr_slabs_swap_avail;

    if (nr_slabs_swap_avail < 2)
    {
        spin_unlock(&bp->spinlock);
        return -1;
    }

    uint64_t **tmp = (uint64_t **)shmptr_to_ptr(bp->slabs_swap, bpw->shm_bp);
    bc->slabs_cons = (uint64_t *)shmptr_to_ptr((shmptr_t)tmp[nr_slabs_swap_avail - 1], bpw->shm_bp);
    bc->slabs_prod = (uint64_t *)shmptr_to_ptr((shmptr_t)tmp[nr_slabs_swap_avail - 2], bpw->shm_bp);

    bp->nr_slabs_swap_avail -= 2;
    spin_unlock(&bp->spinlock);

    return 0;
}

// TODO: this function cannot handle the case when some buffers still exists in the cache
void thread_bcache_free(struct thread_bcache *bc)
{
    struct buffer_pool_wrapper *bpw;
    struct buffer_pool *bp;

    if (!bc) return;

    bpw = bc->bpw;

    if (!bpw) return;

    bp = bpw->bp;
    
    if (!bpw->bp) return;

    spin_lock(&bp->spinlock);
    uint64_t **tmp = (uint64_t **)shmptr_to_ptr(bp->slabs_swap, bpw->shm_bp);
    tmp[bp->nr_slabs_swap_avail] = (uint64_t *)ptr_to_shmptr(bc->slabs_cons, bpw->shm_bp);
    tmp[bp->nr_slabs_swap_avail + 1] = (uint64_t *)ptr_to_shmptr(bc->slabs_prod, bpw->shm_bp);

    bp->nr_slabs_swap_avail += 2;
    spin_unlock(&bp->spinlock);
}

uint64_t
thread_bcache_check(struct thread_bcache *bc, uint64_t nr_req_buffers)
{
    struct buffer_pool_wrapper *bpw;
    struct buffer_pool *bp;
    uint64_t *slab_full;

    /* fast-path */
    if (likely(bc->nr_buffers_cons))
    {
        // as long as we have available buffers in the cache,
        // we allocate buffers from the cache
        return (bc->nr_buffers_cons < nr_req_buffers) ? bc->nr_buffers_cons : nr_req_buffers;
    }

    /* slow-path */
    bpw = bc->bpw;
    bp = bpw->bp;
    spin_lock(&bp->spinlock);
    if (bp->nr_slabs_avail == 0)
    {
        spin_unlock(&bp->spinlock);
        return 0;
    }

    bp->nr_slabs_avail--;

    uint64_t **tmp = (uint64_t **)shmptr_to_ptr(bp->slabs, bpw->shm_bp);
    slab_full = (uint64_t *)shmptr_to_ptr((shmptr_t)tmp[bp->nr_slabs_avail], bpw->shm_bp);
    tmp[bp->nr_slabs_avail] = (uint64_t *)ptr_to_shmptr(bc->slabs_cons, bpw->shm_bp);

    spin_unlock(&bp->spinlock);
    bc->slabs_cons = slab_full;
    bc->nr_buffers_cons = bpw->bp_params.nr_buffers_per_slab;

    return nr_req_buffers;
}

static inline uint64_t fix_umem_offset(uint64_t addr) {
  return addr & ~(XSK_UMEM__DEFAULT_FRAME_SIZE - 1);
}

uint64_t thread_bcache_cons(struct thread_bcache *bc)
{
    assert(bc->nr_buffers_cons > 0);
    uint64_t addr = bc->slabs_cons[--bc->nr_buffers_cons];
    return addr;
}

void thread_bcache_prod(struct thread_bcache *bc, uint64_t buffer)
{
    struct buffer_pool_wrapper *bpw;
    struct buffer_pool *bp;
    uint64_t *slab_empty;
    
    bpw = bc->bpw;
    bp = bpw->bp;

    buffer = fix_umem_offset(buffer);

    assert(bp->nr_slabs_avail <= bp->nr_slabs);
    
    /* fast-path */
    if (likely(bc->nr_buffers_prod < bpw->bp_params.nr_buffers_per_slab))
    {
        bc->slabs_prod[bc->nr_buffers_prod++] = buffer;
        return;
    }

    /* slow-path */
    spin_lock(&bp->spinlock);
    
    uint64_t **tmp = (uint64_t **)shmptr_to_ptr(bp->slabs, bpw->shm_bp);
    slab_empty = (uint64_t *)shmptr_to_ptr((shmptr_t)tmp[bp->nr_slabs_avail], bpw->shm_bp);
    tmp[bp->nr_slabs_avail] = (uint64_t *)ptr_to_shmptr(bc->slabs_prod, bpw->shm_bp);
    bp->nr_slabs_avail++;

    spin_unlock(&bp->spinlock);
    assert(slab_empty != NULL);
    slab_empty[0] = buffer;
    bc->slabs_prod = slab_empty;
    bc->nr_buffers_prod = 1;
}