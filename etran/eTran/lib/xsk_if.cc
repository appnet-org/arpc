#include <xsk_if.h>

#include <errno.h>
#include <stdlib.h>

#include <xskbp/xsk_buffer_pool.h>

/**
 * @brief Emulate xsk_socket__create_shared() in libxdp.
 * TODO: There is no way for microkernel to track the status of tehese sockets
 *       since we bypass the struct xsk_umem. Problems may arise when the microkernel
 *       crashes and frees the umem before the application exits.
 */
struct xsk_socket_info *xsk_configure_socket(int xsk_fd, unsigned int qid, int umem_fd, int ifindex,
                                             struct buffer_pool_wrapper *bpw)
{
    struct xsk_socket_info *xsk_info = NULL;
    /* To avoid packet drop after XDP, let RX RING SIZE equals FILL RING SIZE */
    __u32 rx_ring_size = XSK_RING_CONS__DEFAULT_NUM_DESCS * 2;
    __u32 tx_ring_size = XSK_RING_PROD__DEFAULT_NUM_DESCS;
    struct xsk_ring_cons *rx;
    struct xsk_ring_prod *tx;
    struct xdp_mmap_offsets off;
    struct sockaddr_xdp sxdp = {0};

    xsk_info = reinterpret_cast<struct xsk_socket_info *>(calloc(1, sizeof(struct xsk_socket_info)));
    if (!xsk_info)
        goto err;

    xsk_info->fd = xsk_fd;

    if (setsockopt(xsk_info->fd, SOL_XDP, XDP_RX_RING, &rx_ring_size, sizeof(rx_ring_size)))
    {
        fprintf(stderr, "setsockopt XDP_RX_RING failed, %s\n", strerror(errno));
        goto err;
    }

    if (setsockopt(xsk_info->fd, SOL_XDP, XDP_TX_RING, &tx_ring_size, sizeof(tx_ring_size)))
    {
        fprintf(stderr, "setsockopt XDP_TX_RING failed, %s\n", strerror(errno));
        goto err;
    }

    if (xsk_get_mmap_offsets(xsk_info->fd, &off))
    {
        fprintf(stderr, "xsk_get_mmap_offsets failed\n");
        goto err;
    }
    // RX Ring
    xsk_info->rx_map = mmap(NULL, off.rx.desc + rx_ring_size * sizeof(struct xdp_desc), PROT_READ | PROT_WRITE,
                            MAP_SHARED | MAP_POPULATE, xsk_info->fd, XDP_PGOFF_RX_RING);
    if (xsk_info->rx_map == MAP_FAILED)
    {
        perror("rx mmap failed");
        goto err;
    }
    xsk_info->rx_map_size = off.rx.desc + rx_ring_size * sizeof(struct xdp_desc);
    rx = &xsk_info->rx;
    rx->mask = rx_ring_size - 1;
    rx->size = rx_ring_size;
    rx->producer = reinterpret_cast<unsigned int *>(xsk_info->rx_map + off.rx.producer);
    rx->consumer = reinterpret_cast<unsigned int *>(xsk_info->rx_map + off.rx.consumer);
    rx->flags = reinterpret_cast<unsigned int *>(xsk_info->rx_map + off.rx.flags);
    rx->ring = xsk_info->rx_map + off.rx.desc;
    rx->cached_prod = *rx->producer;
    rx->cached_cons = *rx->consumer;
    // TX Ring
    xsk_info->tx_map = mmap(NULL, off.tx.desc + tx_ring_size * sizeof(struct xdp_desc), PROT_READ | PROT_WRITE,
                            MAP_SHARED | MAP_POPULATE, xsk_info->fd, XDP_PGOFF_TX_RING);
    if (xsk_info->tx_map == MAP_FAILED)
    {
        perror("tx mmap failed");
        goto err;
    }
    xsk_info->tx_map_size = off.tx.desc + tx_ring_size * sizeof(struct xdp_desc);
    tx = &xsk_info->tx;
    tx->mask = tx_ring_size - 1;
    tx->size = tx_ring_size;
    tx->producer = reinterpret_cast<unsigned int *>(xsk_info->tx_map + off.tx.producer);
    tx->consumer = reinterpret_cast<unsigned int *>(xsk_info->tx_map + off.tx.consumer);
    tx->flags = reinterpret_cast<unsigned int *>(xsk_info->tx_map + off.tx.flags);
    tx->ring = xsk_info->tx_map + off.tx.desc;
    tx->cached_prod = *tx->producer;
    tx->cached_cons = *tx->consumer + tx_ring_size;

    sxdp.sxdp_family = PF_XDP;
    sxdp.sxdp_ifindex = ifindex;
    sxdp.sxdp_queue_id = qid;
    sxdp.sxdp_flags |= XDP_SHARED_UMEM;
    sxdp.sxdp_shared_umem_fd = umem_fd;

    if (bind(xsk_info->fd, (struct sockaddr *)&sxdp, sizeof(sxdp)))
    {
        perror("bind xsk_fd failed");
        goto err;
    }

    xsk_info->umem_area = reinterpret_cast<char *>(bpw->shm_umem->addr);

    return xsk_info;
err:
    if (xsk_info->rx_map_size)
        munmap(xsk_info->rx_map, xsk_info->rx_map_size);
    if (xsk_info->tx_map_size)
        munmap(xsk_info->tx_map, xsk_info->tx_map_size);
    if (xsk_info)
        free(xsk_info);
    return NULL;
}

void xsk_delete_socket(struct xsk_socket_info *xsk_info)
{
    if (!xsk_info)
        return;

    if (xsk_info->rx_map_size)
        munmap(xsk_info->rx_map, xsk_info->rx_map_size);

    if (xsk_info->tx_map_size)
        munmap(xsk_info->tx_map, xsk_info->tx_map_size);
    
    free(xsk_info);
}

void xsk_mmap_offsets_v1(struct xdp_mmap_offsets *off)
{
    struct xdp_mmap_offsets_v1 off_v1;

    /* getsockopt on a kernel <= 5.3 has no flags fields.
     * Copy over the offsets to the correct places in the >=5.4 format
     * and put the flags where they would have been on that kernel.
     */
    memcpy(&off_v1, off, sizeof(off_v1));

    off->rx.producer = off_v1.rx.producer;
    off->rx.consumer = off_v1.rx.consumer;
    off->rx.desc = off_v1.rx.desc;
    off->rx.flags = off_v1.rx.consumer + sizeof(__u32);

    off->tx.producer = off_v1.tx.producer;
    off->tx.consumer = off_v1.tx.consumer;
    off->tx.desc = off_v1.tx.desc;
    off->tx.flags = off_v1.tx.consumer + sizeof(__u32);

    off->fr.producer = off_v1.fr.producer;
    off->fr.consumer = off_v1.fr.consumer;
    off->fr.desc = off_v1.fr.desc;
    off->fr.flags = off_v1.fr.consumer + sizeof(__u32);

    off->cr.producer = off_v1.cr.producer;
    off->cr.consumer = off_v1.cr.consumer;
    off->cr.desc = off_v1.cr.desc;
    off->cr.flags = off_v1.cr.consumer + sizeof(__u32);
}

int xsk_get_mmap_offsets(int fd, struct xdp_mmap_offsets *off)
{
    socklen_t optlen;
    int err;

    optlen = sizeof(*off);
    err = getsockopt(fd, SOL_XDP, XDP_MMAP_OFFSETS, off, &optlen);
    if (err)
        return err;

    if (optlen == sizeof(*off))
        return 0;

    if (optlen == sizeof(struct xdp_mmap_offsets_v1))
    {
        xsk_mmap_offsets_v1(off);
        return 0;
    }

    return -EINVAL;
}