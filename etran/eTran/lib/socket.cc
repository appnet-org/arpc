#include <unistd.h>
#include <unordered_map>
#include <mutex>

#include <sys/eventfd.h>
#include <sys/types.h>
#include <sys/socket.h>

#include <eTran_posix.h>
#include <eTran_socket.h>

extern int (*libc_socket)(int, int, int);
extern int (*libc_close)(int sockfd);
extern int (*libc_bind)(int sockfd, const struct sockaddr *addr,
                        socklen_t addrlen);
extern int (*libc_connect)(int sockfd, const struct sockaddr *addr,
                           socklen_t addrlen);
extern int (*libc_listen)(int sockfd, int backlog);
extern int (*libc_accept)(int sockfd, struct sockaddr *addr,
                          socklen_t *addrlen);
extern ssize_t (*libc_read)(int fd, void *buf, size_t count);
extern ssize_t (*libc_write)(int fd, const void *buf, size_t count);
extern int (*libc_setsockopt)(int socket, int level, int option_name,
                              const void *option_value, socklen_t option_len);
extern int (*libc_getsockopt)(int socket, int level, int option_name,
                              void *option_value, socklen_t *option_len);
extern int (*libc_fcntl)(int fd, int cmd, ...);
extern int (*libc_epoll_create1)(int flags);
extern int (*libc_epoll_ctl)(int epfd, int op, int fd,
                             struct epoll_event *event);
extern int (*libc_epoll_wait)(int epfd, struct epoll_event *events,
                              int maxevents, int timeout);
extern int (*libc_select)(int nfds, fd_set *readfds, fd_set *writefds,
                          fd_set *exceptfds, struct timeval *timeout);

static struct eTran_file_handle fds[MAX_FD] = {};
static __thread struct eTrantcp_event events[256] = {};
static __thread struct eTranhoma_event homa_events[256] = {};

extern struct app_ctx_per_thread *eTran_get_tctx(void);

static inline struct eTran_epoll *lookup_epoll_with_fd(int fd)
{
    if (unlikely(fd < 0 || fd >= MAX_FD || fds[fd].type != FH_EPOLL))
    {
        return nullptr;
    }
    return fds[fd].data.epoll;
}

static inline struct eTran_socket_t *lookup_socket_with_fd(int fd)
{
    if (unlikely(fd < 0 || fd >= MAX_FD || fds[fd].type != FH_SOCKET))
    {
        return nullptr;
    }
    return fds[fd].data.socket;
}

static inline void clear_socket_with_fd(int fd)
{
    fds[fd].type = FH_UNUSED;
    fds[fd].data = {};
}

static inline void epoll_lock(struct eTran_epoll *ep)
{
#ifdef SOCKET_MIGRATION
    spin_lock(&ep->lock);
#endif
}

static inline void epoll_unlock(struct eTran_epoll *ep)
{
#ifdef SOCKET_MIGRATION
    spin_unlock(&ep->lock);
#endif
}

static inline void socket_lock(struct eTran_socket_t *s)
{
#ifdef SOCKET_MIGRATION
    spin_lock(&s->lock);
#endif
}

static inline void socket_unlock(struct eTran_socket_t *s)
{
#ifdef SOCKET_MIGRATION
    spin_unlock(&s->lock);
#endif
}

static inline void socket_get(struct eTran_socket_t *s)
{
    kref_get(&s->ref);
}

static inline void socket_put(struct eTran_socket_t *s)
{
    kref_put(&s->ref, s->release);
}

static inline void clear_epoll_events(struct eTran_socket_t *s, uint32_t events)
{
    socket_lock(s);
    s->epoll_events &= ~events;
    socket_unlock(s);
}

static inline void set_epoll_events(struct eTran_socket_t *s, uint32_t events)
{
    struct eTran_epoll_item *item;
    struct eTran_epoll *ep;
    uint32_t new_events;


    new_events = (~s->epoll_events) & events;
    if (!new_events)
    {
        return;
    }

    socket_lock(s);
    s->epoll_events |= events;
    /* append all epoll items of this socket to active list of its epoll if needed */
    for (auto it = s->epoll_items->begin(); it != s->epoll_items->end(); it++)
    {
        item = it->second;
        assert(item);
        if (item->interest_events & new_events)
        {
            ep = item->epoll;
            epoll_lock(ep);
            if (ep->active_list.find(s->fd) == ep->active_list.end())
            {
                ep->active_list.emplace(std::make_pair(s->fd, item));
                ep->inactive_list.erase(s->fd);
            }
            epoll_unlock(ep);
        }
    }
    socket_unlock(s);
}

static void homa_socket_release(struct kref *kref)
{
    struct eTran_socket_t *s = container_of(kref, struct eTran_socket_t, ref);
    if (s->hs)
        delete s->hs;

    /* no one owns this socket, free the fd to OS */
    clear_socket_with_fd(s->fd);
    libc_close(s->fd);

    delete s;
}

static int socket_homa_poll(struct app_ctx_per_thread *tctx, int budget, int timeout)
{
    int fd;
    struct eTran_socket_t *s;
    if (budget > 64)
        budget = 64;

    int nr_events = eTran_homa_poll_events(tctx, homa_events, budget, timeout);
    if (nr_events < 0)
    {
        fprintf(stderr, "socket_info:Failed to eTran_homa_poll_events\n");
        return -1;
    }

    for (int i = 0; i < nr_events; i++)
    {
        switch (homa_events[i].type)
        {
        case ETRANHOMA_EV_SOCKET_BIND:
            fd = homa_events[i].ev.bind.fd;
            s = lookup_socket_with_fd(fd);
            if (!s)
                break;
            socket_lock(s);
            if (s->hs != homa_events[i].ev.bind.hs)
            {
                socket_unlock(s);
                break;
            }
            if (homa_events[i].ev.bind.status == -1)
            {
                s->status = S_HOMA_CLOSED;
                fprintf(stderr, "socket_info:Failed to bind socket\n");
            }
            else if (homa_events[i].ev.bind.status == 0)
            {
                s->status = S_HOMA_BOUND;
            }
            socket_unlock(s);
            break;
        case ETRANHOMA_EV_SOCKET_CLOSE:
            fd = homa_events[i].ev.bind.fd;
            s = lookup_socket_with_fd(fd);
            if (!s)
                break;
            socket_lock(s);
            if (s->hs != homa_events[i].ev.bind.hs)
            {
                socket_unlock(s);
                break;
            }
            if (homa_events[i].ev.close.status == -2)
            {
                s->status = S_HOMA_CLOSED;
                fprintf(stderr, "socket_info:Failed to close socket\n");
            }
            else
            {
                s->status = S_HOMA_CLOSED;
            }
            socket_unlock(s);
            break;
        default:
            fprintf(stderr, "socket_info:Unknown event type %d\n", homa_events[i].type);
            break;
        }
    }

    return 0;
}

static void tcp_socket_release(struct kref *kref)
{
    struct eTran_socket_t *s = container_of(kref, struct eTran_socket_t, ref);
    if (s->conn)
    {
        delete s->conn;
    }

    /* no one owns this socket, free the fd to OS */
    clear_socket_with_fd(s->fd);
    libc_close(s->fd);

    delete s;
}

static int socket_tcp_poll(struct app_ctx_per_thread *tctx, int budget, int timeout)
{
    int fd;
    struct eTran_socket_t *s;
    if (budget > 64)
        budget = 64;

    int nr_events = eTran_tcp_poll_events(tctx, events, budget, timeout);
    if (nr_events < 0)
    {
        fprintf(stderr, "socket_info:Failed to eTran_tcp_poll_events\n");
        return -1;
    }

    for (int i = 0; i < nr_events; i++)
    {
        if (likely(events[i].type == ETRANTCP_EV_CONN_RECVED ||
                   events[i].type == ETRANTCP_EV_CONN_SENDBUF))
        {
            s = events[i].type == ETRANTCP_EV_CONN_RECVED ? events[i].ev.recv.conn->s : events[i].ev.send.conn->s;
            set_epoll_events(s, events[i].type == ETRANTCP_EV_CONN_RECVED ? EPOLLIN : EPOLLOUT);
            continue;
        }
        /* control path events */
        switch (events[i].type)
        {
        case ETRANTCP_EV_CONN_OPEN:
            fd = events[i].ev.open.fd;
            s = lookup_socket_with_fd(fd);
            if (!s)
                break;
            socket_lock(s);
            if (s->conn != events[i].ev.open.conn)
            {
                socket_unlock(s);
                break;
            }
            if (events[i].ev.open.status == -1)
            {
                fprintf(stderr, "socket_info:Failed to open connection\n");
                s->status = S_CONN_CLOSED;
            }
            else if (events[i].ev.open.status == 0)
            {
                // fprintf(stdout,"socket_info:Connection opened, waiting for connection (%p), %d, %p\n", events[i].ev.open.conn, i, &events[i]);
                s->status = S_CONN_CONNECTING;
            }
            else if (events[i].ev.open.status == 1)
            {
                // fprintf(stdout,"socket_info:Connection(%p) is established, NAPI ID is %u\n", events[i].ev.open.conn, events[i].ev.open.conn->qid);
                s->status = S_CONN_CONNECTED;
            }
            socket_unlock(s);
            break;
        case ETRANTCP_EV_CONN_BIND:
            fd = events[i].ev.bind.fd;
            s = lookup_socket_with_fd(fd);
            if (!s)
                break;
            socket_lock(s);
            if (s->conn != events[i].ev.bind.conn)
            {
                socket_unlock(s);
                break;
            }
            if (events[i].ev.bind.status == -1)
            {
                fprintf(stderr, "socket_info:Failed to bind connection\n");
                s->status = S_CONN_CLOSED;
            }
            else if (events[i].ev.bind.status == 0)
            {
                // fprintf(stdout,"socket_info:Connection is bound\n");
                s->status = S_CONN_BOUND;
            }
            socket_unlock(s);
            break;
        case ETRANTCP_EV_LISTEN_OPEN:
            fd = events[i].ev.listen.fd;
            s = lookup_socket_with_fd(fd);
            if (!s)
                break;
            socket_lock(s);
            if (s->listener != events[i].ev.listen.listener)
            {
                socket_unlock(s);
                break;
            }
            if (events[i].ev.listen.status)
            {
                fprintf(stderr, "socket_info:Failed to open listener\n");
                s->status = S_CONN_CLOSED;
            }
            else
            {
                // fprintf(stdout, "socket_info:Listener opened\n");
                s->status = S_CONN_LISTENING;
            }
            socket_unlock(s);
            break;
        case ETRANTCP_EV_LISTEN_NEWCONN:
            fd = events[i].ev.newconn.fd;
            s = lookup_socket_with_fd(fd);
            if (!s)
                break;
            socket_lock(s);
            if (s->listener != events[i].ev.newconn.listener)
            {
                printf("ETRANTCP_EV_LISTEN_NEWCONN\n");
                socket_unlock(s);
                break;
            }
            // fprintf(stdout,"socket_info:New connection arrives\n");
            socket_unlock(s);
            set_epoll_events(s, EPOLLIN);
            break;
        case ETRANTCP_EV_LISTEN_ACCEPT:
            fd = events[i].ev.accept.fd;
            s = lookup_socket_with_fd(fd);
            if (!s)
                break;
            socket_lock(s);
            if (!events[i].ev.accept.conn || s != events[i].ev.accept.conn->s)
            {
                socket_unlock(s);
                break;
            }
            socket_unlock(s);
            if (events[i].ev.accept.status == 0)
            {
                // fprintf(stdout,"socket_info:Connection accepted, conn(%p)\n", events[i].ev.accept.conn);
            }
            else
            {
                fprintf(stderr, "socket_info:No connection to accept\n");
            }
            if (!events[i].ev.accept.backlog)
            {
                clear_epoll_events(s, EPOLLIN);
            }
            break;
        case ETRANTCP_EV_CONN_CLOSE:
            fd = events[i].ev.close.fd;
            s = lookup_socket_with_fd(fd);
            if (!s)
                break;
            socket_lock(s);
            if (s->conn != events[i].ev.close.conn)
            {
                socket_unlock(s);
                break;
            }
            s->status = S_CONN_CLOSED;
            socket_unlock(s);
            if (events[i].ev.close.status == -1)
            {
                fprintf(stderr, "socket_info:Failed to close connection\n");
            }
            else if (events[i].ev.close.status == 0)
            {
                fprintf(stdout, "socket_info:Connection is closed by microkernel\n");
            }
            else
            {
                fprintf(stdout, "socket_info:Connection closed\n");
            }
            set_epoll_events(s, EPOLLERR | EPOLLHUP);
            break;
        default:
            fprintf(stderr, "socket_info:Unknown event type %d\n", events[i].type);
            break;
        }
    }

    return 0;
}

static int alloc_epoll_fd(struct eTran_epoll **ep, struct app_ctx_per_thread *tctx)
{
    int fd;

    fd = libc_epoll_create1(0);
    if (unlikely(fd < 0))
    {
        return fd;
    }

    if (unlikely(fd >= MAX_FD))
    {
        libc_close(fd);
        return -ENOMEM;
    }

    *ep = new eTran_epoll(fd, tctx);
    if (unlikely(!*ep))
    {
        libc_close(fd);
        return -1;
    }

    return fd;
}

static int alloc_socket_fd(void)
{
    struct app_ctx_per_thread *tctx;
    int fd = eventfd(0, 0);
    if (fd < 0)
    {
        fprintf(stderr, "alloc_socket_fd: failed to create eventfd\n");
        return fd;
    }

    if (fd >= MAX_FD)
    {
        fprintf(stderr, "alloc_socket_fd: fd is too large\n");
        libc_close(fd);
        return -ENOMEM;
    }

    struct eTran_socket_t *s = new eTran_socket_t();
    if (!s)
    {
        libc_close(fd);
        return -ENOMEM;
    }

    tctx = eTran_get_tctx();
    if (!tctx)
    {
        fprintf(stderr, "alloc_socket_fd: failed to get tctx\n");
        delete s;
        libc_close(fd);
        return -ENOMEM;
    }

    s->fd = fd;
    s->protocol = 0;
    s->tctx = tctx;
    s->conn = nullptr;
    s->type = SOCKET_TYPE_SOCKET;
    s->listener = nullptr;
    s->flags = {};
    s->addr = {};
    s->epoll_events = 0;

    assert(fd < MAX_FD);
    fds[fd].data.socket = s;
    fds[fd].type = FH_SOCKET;

    return fd;
}

int eTran_socket(int domain, int type, int protocol)
{
    struct eTran_socket_t *s;
    int fd;
    int flag = 0;

    if ((type & SOCK_NONBLOCK) == SOCK_NONBLOCK)
    {
        flag |= SOF_NONBLOCK;
    }

    fd = alloc_socket_fd();
    if (fd < 0)
    {
        errno = -fd;
        return fd;
    }

    s = lookup_socket_with_fd(fd);

    s->flags = static_cast<enum socket_flags>(s->flags | flag);
    s->protocol = protocol;
    if (s->protocol == IPPROTO_HOMA)
    {
        s->status = S_HOMA_INIT;
        s->release = homa_socket_release;
    }
    else if (s->protocol == IPPROTO_TCP)
    {
        s->status = S_CONN_INIT;
        s->release = tcp_socket_release;
    }

    return fd;
}

int eTran_close(int fd)
{
    struct eTran_socket_t *s;
    struct eTran_epoll *ep;
    struct app_ctx_per_thread *tctx;

    if (fd < 0 || fd >= MAX_FD || fds[fd].type == FH_UNUSED)
    {
        errno = EBADF;
        return -EBADF;
    }

    tctx = eTran_get_tctx();
    if (!tctx)
    {
        errno = EIO;
        return -EIO;
    }

    if (fds[fd].type == FH_EPOLL)
    {
        ep = fds[fd].data.epoll;

#ifndef SOCKET_MIGRATION
        if (ep->tctx != tctx)
        {
            errno = EPERM;
            return -EPERM;
        }
#endif

        /* clear epoll active and inactive lists */
        epoll_lock(ep);
        for (auto it = ep->inactive_list.begin(); it != ep->inactive_list.end(); it++)
        {
            delete it->second;
        }
        ep->inactive_list.clear();

        for (auto it = ep->active_list.begin(); it != ep->active_list.end(); it++)
        {
            delete it->second;
        }
        ep->active_list.clear();
        epoll_unlock(ep);

        delete ep;
        fds[fd].data.epoll = nullptr;
        fds[fd].type = FH_UNUSED;
    }
    else
    {
        s = fds[fd].data.socket;
#ifndef SOCKET_MIGRATION
        if (s->tctx != tctx)
        {
            errno = EPERM;
            return -EPERM;
        }
#endif

        if (s->conn || s->hs)
        {
            if (s->protocol == IPPROTO_TCP)
            {
                /* notify control path */
                eTran_tcp_close(s->tctx, s->conn, s->fd);
                do
                {
                    /* poll control path for completion */
                    if (socket_tcp_poll(s->tctx, 64, -1))
                        break;
                } while (s->status != S_CONN_CLOSED);
            }
            else if (s->protocol == IPPROTO_HOMA)
            {
                /* notify control path */
                eTran_homa_close(s->tctx, s->hs, s->fd);
                do
                {
                    /* poll control path for completion */
                    if (socket_homa_poll(s->tctx, 64, -1))
                        break;
                } while (s->status != S_HOMA_CLOSED);
            }
        }
        socket_put(s);
    }

    return 0;
}

int eTran_connect(int fd, const struct sockaddr *addr, socklen_t addrlen)
{
    int ret = 0;
    struct eTran_socket_t *s;
    struct app_ctx_per_thread *tctx;
    struct sockaddr_in *sin = (struct sockaddr_in *)addr;

    s = lookup_socket_with_fd(fd);
    if (!s)
    {
        errno = EBADF;
        return -EBADF;
    }

    tctx = eTran_get_tctx();
    if (!tctx)
    {
        errno = EIO;
        return -EIO;
    }

    /* check parameters */
    if (addrlen != sizeof(s->addr) || addr->sa_family != AF_INET)
    {
        errno = EINVAL;
        return -EINVAL;
    }

    /* check socket */
    if (s->protocol != IPPROTO_TCP)
    {
        errno = EINVAL;
        return -EINVAL;
    }

    if (s->type == SOCKET_TYPE_LISTENER)
    {
        errno = EINVAL;
        return -EINVAL;
    }

    if (s->status != S_CONN_INIT)
    {
        errno = EINVAL;
        return -EINVAL;
    }

    socket_lock(s);
    /* update the owner thread */
    s->tctx = tctx;

    s->conn = new eTrantcp_connection();
    if (!s->conn)
    {
        errno = ENOMEM;
        ret = -ENOMEM;
        goto out;
    }
    s->conn->s = s;

    /* notify control path to connect */
    if (eTran_tcp_open(tctx, s->conn, s->fd, ntohl(sin->sin_addr.s_addr), ntohs(sin->sin_port)))
    {
        errno = EIO;
        ret = -EIO;
        goto out;
    }

    socket_unlock(s);

    do
    {
        /* poll control path for completion */
        if (socket_tcp_poll(tctx, 64, -1))
        {
            errno = EIO;
            ret = -EIO;
            socket_lock(s);
            goto out;
        }

        if (s->status == S_CONN_CLOSED)
        {
            errno = EIO;
            ret = -EIO;
            socket_lock(s);
            goto out;
        }

    } while (s->status != S_CONN_CONNECTED);

    socket_lock(s);

    /* SOCKET_TYPE_SOCKET --> SOCKET_TYPE_CONNECTION */
    s->type = SOCKET_TYPE_CONNECTION;
    s->addr = *sin;

    socket_unlock(s);

    return 0;

out:
    if (s->conn)
    {
        delete s->conn;
        s->conn = nullptr;
    }

    socket_unlock(s);

    if (s->status == S_CONN_CLOSED)
        socket_put(s);

    return ret;
}

static int _homa_bind(struct app_ctx_per_thread *tctx, struct eTran_socket_t *s, const struct sockaddr *addr)
{
    int ret = 0;

    socket_lock(s);

    /* update the owner thread */
    s->tctx = tctx;

    if (s->status != S_HOMA_INIT)
    {
        ret = -EINVAL;
        errno = EINVAL;
        goto out;
    }

    s->hs = new eTranhoma_socket();
    if (!s->hs)
    {
        ret = -ENOMEM;
        errno = ENOMEM;
        goto out;
    }
    s->hs->s = s;

    s->addr = *(struct sockaddr_in *)addr;

    /* notify control path to bind the port */
    if (eTran_homa_bind(tctx, s->hs, s->fd, ntohl(s->addr.sin_addr.s_addr), ntohs(s->addr.sin_port)))
    {
        ret = -EIO;
        errno = EIO;
        goto out;
    }

    socket_unlock(s);
    do
    {
        /* poll control path for completion */
        if (socket_homa_poll(tctx, 64, -1))
        {
            ret = -EIO;
            errno = EIO;
            socket_lock(s);
            goto out;
        }

    } while (s->status == S_HOMA_INIT);

    socket_lock(s);
    if (s->status == S_HOMA_CLOSED)
    {
        ret = -EIO;
        errno = EIO;
        goto out;
    }

    s->flags = static_cast<enum socket_flags>(s->flags | SOF_BOUND);

    socket_unlock(s);

    return 0;
out:
    if (s->hs)
    {
        delete s->hs;
        s->hs = nullptr;
    }

    socket_unlock(s);

    if (s->status == S_HOMA_CLOSED)
        socket_put(s);

    return ret;
}

static int _tcp_bind(struct app_ctx_per_thread *tctx, struct eTran_socket_t *s, const struct sockaddr *addr)
{
    int ret = 0;

    socket_lock(s);

    /* update the owner thread */
    s->tctx = tctx;

    if (s->status != S_CONN_INIT)
    {
        ret = -EINVAL;
        errno = EINVAL;
        goto out;
    }

    s->conn = new eTrantcp_connection();
    if (!s->conn)
    {
        ret = -ENOMEM;
        errno = ENOMEM;
        goto out;
    }
    s->conn->s = s;

    s->addr = *(struct sockaddr_in *)addr;

    /* notify control path to bind the port */
    if (eTran_tcp_bind(tctx, s->conn, s->fd, ntohl(s->addr.sin_addr.s_addr), ntohs(s->addr.sin_port), s->flags & SOF_REUSEPORT))
    {
        ret = -EIO;
        errno = EIO;
        goto out;
    }

    socket_unlock(s);
    do
    {
        /* poll control path for completion */
        if (socket_tcp_poll(tctx, 64, -1))
        {
            ret = -EIO;
            errno = EIO;
            socket_lock(s);
            goto out;
        }

        if (s->status == S_CONN_CLOSED)
        {
            ret = -EIO;
            errno = EIO;
            socket_lock(s);
            goto out;
        }
    } while (s->status == S_CONN_INIT);

    socket_lock(s);

    if (s->status == S_CONN_CLOSED)
    {
        ret = -EIO;
        errno = EIO;
        goto out;
    }

    s->flags = static_cast<enum socket_flags>(s->flags | SOF_BOUND);

    socket_unlock(s);

    return 0;
out:

    if (s->conn)
    {
        delete s->conn;
        s->conn = nullptr;
    }

    socket_unlock(s);

    if (s->status == S_CONN_CLOSED)
        socket_put(s);

    return ret;
}

int eTran_bind(int sockfd, const struct sockaddr *addr, socklen_t addrlen)
{
    struct eTran_socket_t *s;
    struct app_ctx_per_thread *tctx;

    s = lookup_socket_with_fd(sockfd);
    if (!s)
    {
        errno = EBADF;
        return -EBADF;
    }

    tctx = eTran_get_tctx();
    if (!tctx)
    {
        errno = EIO;
        return -EIO;
    }

    /* check parameters */
    if (addrlen != sizeof(s->addr) || addr->sa_family != AF_INET)
    {
        errno = EINVAL;
        return -EINVAL;
    }

    if (s->protocol == IPPROTO_TCP)
    {
        return _tcp_bind(tctx, s, addr);
    }
    else if (s->protocol == IPPROTO_HOMA)
    {
        return _homa_bind(tctx, s, addr);
    }
    else
    {
        errno = EINVAL;
        return -EINVAL;
    }

    return 0;
}

int eTran_listen(int sockfd, int backlog)
{
    struct app_ctx_per_thread *tctx;
    int ret = 0;
    struct eTran_socket_t *s = lookup_socket_with_fd(sockfd);

    if (!s)
    {
        ret = -EBADF;
        errno = EBADF;
        return ret;
    }

    tctx = eTran_get_tctx();
    if (!tctx)
    {
        ret = -EIO;
        errno = EIO;
        return ret;
    }

#ifndef SOCKET_MIGRATION
    if (s->tctx != tctx)
    {
        ret = -EPERM;
        errno = EPERM;
        return ret;
    }
#endif

    socket_lock(s);

    if (s->protocol != IPPROTO_TCP)
    {
        socket_unlock(s);
        errno = EINVAL;
        ret = -EINVAL;
        return ret;
    }

    if (s->listener || !s->conn)
    {
        socket_unlock(s);
        ret = -EINVAL;
        errno = EINVAL;
        return ret;
    }

    if (!(s->flags & SOF_BOUND))
    {
        socket_unlock(s);
        ret = -EINVAL;
        errno = EINVAL;
        return ret;
    }

    if (s->status != S_CONN_BOUND)
    {
        socket_unlock(s);
        ret = -EINVAL;
        errno = EINVAL;
        return ret;
    }

    s->listener = (struct eTrantcp_listener *)calloc(1, sizeof(struct eTrantcp_listener));
    if (!s->listener)
    {
        ret = -ENOMEM;
        errno = ENOMEM;
        goto out;
    }

    s->listener->s = s;

    if (eTran_tcp_listen(s->tctx, s->conn, s->listener, s->fd, ntohs(s->addr.sin_port), backlog))
    {
        ret = -EIO;
        errno = EIO;
        goto out;
    }

    socket_unlock(s);
    do
    {
        /* poll control path for completion */
        if (socket_tcp_poll(s->tctx, 64, -1))
        {
            errno = EIO;
            ret = -EIO;
            socket_lock(s);
            goto out;
        }

        if (s->status == S_CONN_CLOSED)
        {
            errno = EIO;
            ret = -EIO;
            socket_lock(s);
            goto out;
        }
    } while (s->status == S_CONN_BOUND);

    return 0;
out:

    if (s->listener)
    {
        free(s->listener);
        s->listener = nullptr;
    }

    socket_unlock(s);
    return ret;
}

int eTran_accept(int sockfd, struct sockaddr *addr, socklen_t *addrlen)
{
    int newfd;
    int ret = 0;
    struct eTran_socket_t *ns;
    struct eTran_socket_t *s = lookup_socket_with_fd(sockfd);
    if (!s)
    {
        errno = EBADF;
        return -EBADF;
    }

    if (s->protocol != IPPROTO_TCP)
    {
        errno = EINVAL;
        return -EINVAL;
    }

    struct app_ctx_per_thread *tctx = eTran_get_tctx();
    if (!tctx)
    {
        errno = EIO;
        return -EIO;
    }

#ifndef SOCKET_MIGRATION
    if (s->tctx != tctx)
    {
        errno = EPERM;
        return -EPERM;
    }
#endif

    newfd = alloc_socket_fd();
    if (newfd < 0)
    {
        return -EIO;
    }

    ns = lookup_socket_with_fd(newfd);
    assert(ns);

    ns->release = tcp_socket_release;

    struct eTrantcp_connection *new_conn = new eTrantcp_connection();
    if (!new_conn)
    {
        close(newfd);
        errno = ENOMEM;
        return -ENOMEM;
    }

    socket_lock(s);

    if (s->status != S_CONN_LISTENING)
    {
        close(newfd);
        delete new_conn;
        errno = EINVAL;
        ret = -EINVAL;
        goto out;
    }

    if (eTran_tcp_accept(tctx, s->listener, new_conn, s->fd, newfd))
    {
        close(newfd);
        delete new_conn;
        errno = EIO;
        ret = -EIO;
        goto out;
    }
    new_conn->s = s;

    socket_unlock(s);
    do
    {
        if (socket_tcp_poll(tctx, 64, -1))
        {
            close(newfd);
            delete new_conn;
            errno = EIO;
            ret = -EIO;
            socket_lock(s);
            goto out;
        }

        if (new_conn->try_accept_done && s->flags & SOF_NONBLOCK && new_conn->status == CONN_ACCEPT_REQUESTED)
        {
            close(newfd);
            delete new_conn;
            errno = EIO;
            ret = -EIO;
            socket_lock(s);
            goto out;
        }

    } while (new_conn->status == CONN_ACCEPT_REQUESTED);

    ns->conn = new_conn;
    ns->protocol = s->protocol;
    ns->status = S_CONN_CONNECTED;
    ns->type = SOCKET_TYPE_CONNECTION;
    // new established connection should be writable, right?
    ns->epoll_events = EPOLLOUT;
    new_conn->s = ns;

    return newfd;
out:
    socket_unlock(s);
    return ret;
}

ssize_t eTran_read(int fd, void *buf, size_t count)
{
    struct eTran_socket_t *s;
    ssize_t ret;
    bool polled = false;

    s = lookup_socket_with_fd(fd);

    if (unlikely(!s))
    {
        return -EBADF;
    }

    if (unlikely(s->protocol != IPPROTO_TCP))
    {
        errno = EINVAL;
        return -EINVAL;
    }

    struct app_ctx_per_thread *tctx = eTran_get_tctx();
    if (unlikely(!tctx))
    {
        return -EINVAL;
    }

#ifndef SOCKET_MIGRATION
    if (s->tctx != tctx)
    {
        return -EPERM;
    }
#endif

    if (unlikely(s->type != SOCKET_TYPE_CONNECTION || s->status != S_CONN_CONNECTED))
    {
        return -EINVAL;
    }

    ret = conn_recv(tctx, s->conn, buf, count);

    while (ret == 0 && !(s->flags & SOF_NONBLOCK) && s->status == S_CONN_CONNECTED)
    {
        socket_tcp_poll(tctx, 64, -1);
        ret = conn_recv(tctx, s->conn, buf, count);
        polled = true;
    }

    if (s->conn->rxb_used == 0)
    {
        clear_epoll_events(s, EPOLLIN);
    }

    if (!polled)
        socket_tcp_poll(tctx, 64, 0);

    return ret;
}

ssize_t eTran_write(int fd, const void *buf, size_t count)
{
    struct eTran_socket_t *s;
    ssize_t ret;
    bool polled = false;

    s = lookup_socket_with_fd(fd);
    if (unlikely(!s))
    {
        return -EBADF;
    }

    if (unlikely(s->protocol != IPPROTO_TCP))
    {
        errno = EINVAL;
        return -EINVAL;
    }

    struct app_ctx_per_thread *tctx = eTran_get_tctx();
    if (unlikely(!tctx))
    {
        return -EINVAL;
    }

#ifndef SOCKET_MIGRATION
    if (s->tctx != tctx)
    {
        return -EPERM;
    }
#endif

    if (unlikely(s->type != SOCKET_TYPE_CONNECTION || s->status != S_CONN_CONNECTED))
    {
        return -EINVAL;
    }

    ret = conn_send(tctx, s->conn, buf, count);

    while (ret == 0 && !(s->flags & SOF_NONBLOCK) && s->status == S_CONN_CONNECTED)
    {
        socket_tcp_poll(tctx, 64, -1);
        ret = conn_send(tctx, s->conn, buf, count);
        polled = true;
    }

    if (txb_bytes_avail(s->conn) == 0)
        clear_epoll_events(s, EPOLLOUT);

    if (!polled)
        socket_tcp_poll(tctx, 64, 0);

    return ret;
}

int eTran_setsockopt(int socket, int level, int option_name,
                     const void *option_value, socklen_t option_len)
{
    if (level != SOL_SOCKET || (option_name != SO_REUSEPORT && option_name != SO_REUSEADDR))
    {
        return -EINVAL;
    }

    struct eTran_socket_t *s = lookup_socket_with_fd(socket);
    if (!s)
    {
        return -EBADF;
    }

    if (s->protocol != IPPROTO_TCP)
    {
        return -EINVAL;
    }

    if (option_len != sizeof(int))
    {
        return -EINVAL;
    }

    int *optval = (int *)option_value;
    if (*optval != 0 && *optval != 1)
    {
        return -EINVAL;
    }
    socket_lock(s);
    s->flags = static_cast<enum socket_flags>(s->flags | (option_name & SO_REUSEPORT ? SOF_REUSEPORT : 0 | (option_name & SO_REUSEADDR) ? SOF_REUSEADDR : 0));
    socket_unlock(s);

    return 0;
}

int eTran_getsockopt(int socket, int level, int option_name,
                     void *option_value, socklen_t *option_len)
{
    return -EINVAL;
}

int eTran_fcntl(int fd, int cmd, int flags)
{
    if (cmd != F_GETFL && cmd != F_SETFL)
    {
        return -EINVAL;
    }

    if (flags & ~O_NONBLOCK)
    {
        return -EINVAL;
    }
    struct eTran_socket_t *s = lookup_socket_with_fd(fd);
    if (!s)
    {
        return -EBADF;
    }

    if (s->protocol != IPPROTO_TCP)
    {
        return -EINVAL;
    }

    if (cmd == F_GETFL)
    {
        return s->flags & SOF_NONBLOCK;
    }
    else if (cmd == F_SETFL)
    {
        socket_lock(s);
        if (flags & O_NONBLOCK)
        {
            s->flags = static_cast<enum socket_flags>(s->flags | SOF_NONBLOCK);
        }
        else
        {
            s->flags = static_cast<enum socket_flags>(s->flags & ~SOF_NONBLOCK);
        }
        socket_unlock(s);
        return 0;
    }
    return -EINVAL;
}

int eTran_epoll_create1(int flags)
{
    int epfd;
    struct eTran_epoll *ep;
    struct app_ctx_per_thread *tctx;

    if (flags)
    {
        errno = EINVAL;
        return -EINVAL;
    }

    tctx = eTran_get_tctx();
    if (!tctx)
    {
        return -EIO;
    }

    /* allocate fd from libc_epoll_create() */
    epfd = alloc_epoll_fd(&ep, tctx);

    if (epfd < 0)
    {
        return -1;
    }

    fds[epfd].data.epoll = ep;
    fds[epfd].type = FH_EPOLL;

    return epfd;
}

int eTran_epoll_ctl(int epfd, int op, int fd, struct epoll_event *event)
{
    struct eTran_epoll_item *item;
    struct eTran_epoll *ep;
    struct eTran_socket_t *s;
    struct app_ctx_per_thread *tctx;
    int ret;

    ep = lookup_epoll_with_fd(epfd);

    if (!ep)
    {
        return -EBADF;
    }

    s = lookup_socket_with_fd(fd);

    if (!s)
    {
        // linux fd
        ret = libc_epoll_ctl(epfd, op, fd, event);
        if (ret < 0)
            return ret;
        if (op == EPOLL_CTL_ADD)
        {
            ep->num_linux++;
        }
        else if (op == EPOLL_CTL_DEL)
        {
            ep->num_linux--;
        }
        return ret;
    }

    if (op != EPOLL_CTL_ADD && op != EPOLL_CTL_MOD && op != EPOLL_CTL_DEL)
    {
        return -EINVAL;
    }

    if (s->protocol != IPPROTO_TCP)
    {
        return -EINVAL;
    }

    tctx = eTran_get_tctx();
    if (!tctx)
    {
        return -EIO;
    }

#ifndef SOCKET_MIGRATION
    if (tctx != s->tctx || ep->tctx != tctx)
    {
        fprintf(stderr, "socket migration is not supported\n");
        return -EPERM;
    }
#endif

    if (op == EPOLL_CTL_ADD)
    {
        if (event->events & ~(EPOLLIN | EPOLLOUT | EPOLLHUP | EPOLLET | EPOLLONESHOT | EPOLLERR))
        {
            return -EINVAL;
        }

        // if this socket has been registered on this epoll before?
        socket_lock(s);
        if (s->epoll_items->find(epfd) != s->epoll_items->end())
        {
            socket_unlock(s);
            return -EEXIST;
        }
        socket_unlock(s);

        item = new eTran_epoll_item(ep, s, event->data, event->events);
        if (!item)
        {
            return -ENOMEM;
        }

        // append the item to socket
        socket_lock(s);
        s->epoll_items->emplace(std::make_pair(epfd, item));
        socket_unlock(s);

        epoll_lock(ep);
        // append the item to epoll's inactive list
        assert(ep->inactive_list.find(fd) == ep->inactive_list.end());
        ep->inactive_list.emplace(std::make_pair(fd, item));

        // check if this socket already has the events
        if (s->epoll_events & item->interest_events)
        {
            assert(ep->active_list.find(fd) == ep->active_list.end());
            ep->active_list.emplace(std::make_pair(fd, item));
            ep->inactive_list.erase(fd);
        }
        ep->num_eTran++;
        epoll_unlock(ep);
    }
    else if (op == EPOLL_CTL_MOD)
    {
        if (event->events & ~(EPOLLIN | EPOLLOUT | EPOLLHUP | EPOLLET | EPOLLONESHOT | EPOLLERR))
        {
            return -EINVAL;
        }

        socket_lock(s);
        // if this socket has been registered on this epoll before?
        if (s->epoll_items->find(epfd) == s->epoll_items->end())
        {
            socket_unlock(s);
            return -ENOENT;
        }
        item = s->epoll_items->at(epfd);
        socket_unlock(s);

        // update interest events
        item->interest_events = event->events;

        epoll_lock(ep);
        // check if this socket already has the events
        if (s->epoll_events & item->interest_events)
        {
            if (ep->active_list.find(fd) == ep->active_list.end())
            {
                ep->active_list.emplace(std::make_pair(fd, item));
                ep->inactive_list.erase(fd);
            }
        }
        epoll_unlock(ep);
    }
    else if (op == EPOLL_CTL_DEL)
    {
        socket_lock(s);
        // if this socket has been registered on this epoll before?
        if (s->epoll_items->find(epfd) == s->epoll_items->end())
        {
            socket_unlock(s);
            return -ENOENT;
        }
        // delete from socket list
        delete s->epoll_items->at(epfd);
        s->epoll_items->erase(epfd);
        socket_unlock(s);

        // delete from epoll list
        epoll_lock(ep);
        if (ep->inactive_list.find(fd) != ep->inactive_list.end())
        {
            ep->inactive_list.erase(fd);
        }
        else if (ep->active_list.find(fd) != ep->active_list.end())
        {
            ep->active_list.erase(fd);
        }

        ep->num_eTran--;
        epoll_unlock(ep);
    }

    return 0;
}

int eTran_epoll_wait(int epfd, struct epoll_event *events,
                     int maxevents, int timeout)
{
    struct app_ctx_per_thread *tctx;
    struct eTran_epoll *ep;
    struct eTran_socket_t *s;
    struct eTran_epoll_item *item;
    int nr_event = 0;
    int n;

    if (maxevents <= 0)
        return -EINVAL;

    ep = lookup_epoll_with_fd(epfd);

    if (!ep)
    {
        return -EBADF;
    }

    tctx = eTran_get_tctx();
    if (!tctx)
    {
        return -EIO;
    }

    // all fds are managed by linux
    if (!ep->num_eTran)
    {
        return libc_epoll_wait(epfd, events, maxevents, timeout);
    }

    if (!ep->num_linux)
    { // all fds are managed by eTran

        /* call socket_tcp_poll() at least once */
        socket_tcp_poll(tctx, maxevents, 0);
        epoll_lock(ep);
        /* traverse epoll's active list */
        for (auto it = ep->active_list.begin(); it != ep->active_list.end();)
        {
            if (nr_event >= maxevents)
            {
                break;
            }
            item = it->second;
            s = item->socket;
            if (s->epoll_events & item->interest_events)
            {
                events[nr_event].data = item->data;
                events[nr_event].events = item->interest_events & s->epoll_events;
                nr_event++;
            }
            else
            {
                // remove from active list and append to inactive list
                it = ep->active_list.erase(it);
                ep->inactive_list.emplace(std::make_pair(s->fd, item));
                continue;
            }
            it++;
        }
        epoll_unlock(ep);

        // no events, use timeout
        if (!nr_event && timeout)
            socket_tcp_poll(tctx, maxevents, timeout);
    }
    else if (ep->linux_first)
    {

        /* linux epoll first, then eTran */
        ep->linux_first = false;
        n = libc_epoll_wait(epfd, events, maxevents, timeout);
        if (n < 0)
            n = 0;
        nr_event += n;

#ifndef SOCKET_MIGRATION
        if (ep->tctx != tctx)
        {
            fprintf(stderr, "socket migration is not supported\n");
            goto out;
        }
#endif

        /* call socket_tcp_poll() at least once */
        socket_tcp_poll(tctx, maxevents, 0);

        epoll_lock(ep);
        /* traverse epoll's active list */
        for (auto it = ep->active_list.begin(); it != ep->active_list.end();)
        {
            if (nr_event >= maxevents)
            {
                break;
            }
            item = it->second;
            s = item->socket;
            if (s->epoll_events & item->interest_events)
            {
                events[nr_event].data = item->data;
                events[nr_event].events = item->interest_events & s->epoll_events;
                nr_event++;
            }
            else
            {
                // remove from active list and append to inactive list
                it = ep->active_list.erase(it);
                ep->inactive_list.emplace(std::make_pair(s->fd, item));
                continue;
            }
            it++;
        }
        epoll_unlock(ep);
    }
    else
    {

#ifndef SOCKET_MIGRATION
        if (ep->tctx != tctx)
        {
            fprintf(stderr, "socket migration is not supported\n");

            /* linux epoll */
            if (nr_event < maxevents)
            {
                n += libc_epoll_wait(epfd, events + nr_event, maxevents - nr_event, timeout);
                if (n < 0)
                    n = 0;
                nr_event += n;
            }

            goto out;
        }
#endif

        /* eTran epoll first, then linux */
        ep->linux_first = true;

        /* call socket_tcp_poll() at least once */
        socket_tcp_poll(tctx, maxevents, 0);

        epoll_lock(ep);
        /* traverse epoll's active list */
        for (auto it = ep->active_list.begin(); it != ep->active_list.end();)
        {
            if (nr_event >= maxevents)
            {
                break;
            }
            item = it->second;
            s = item->socket;
            if (s->epoll_events & item->interest_events)
            {
                events[nr_event].data = item->data;
                events[nr_event].events = item->interest_events & s->epoll_events;
                nr_event++;
            }
            else
            {
                // remove from active list and append to inactive list
                it = ep->active_list.erase(it);
                ep->inactive_list.emplace(std::make_pair(s->fd, item));
                continue;
            }
            it++;
        }
        epoll_unlock(ep);

        if (nr_event < maxevents)
        {
            n += libc_epoll_wait(epfd, events + nr_event, maxevents - nr_event, timeout);
            if (n < 0)
                n = 0;
            nr_event += n;
        }
    }
#ifndef SOCKET_MIGRATION
out:
#endif
    return nr_event;
}

int eTran_select(int nfds, fd_set *readfds, fd_set *writefds,
                 fd_set *exceptfds, struct timeval *timeout)
{
    return -EINVAL;
}

