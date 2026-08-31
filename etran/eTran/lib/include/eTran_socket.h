#pragma once

#include <base/lock.h>
#include <base/kref.h>

#include <eTran_posix.h>

#include <tcp_if.h>
#include <homa_if.h>

#include <map>

enum socket_type {
    SOCKET_TYPE_SOCKET = 1,
    SOCKET_TYPE_CONNECTION,
    SOCKET_TYPE_LISTENER,
    SOCKET_TYPE_EPOLL,
};

enum socket_status {
    S_UNUSED = 0,
    S_CONN_INIT,
    S_CONN_CONNECTING,
    S_CONN_CONNECTED,
    S_CONN_BOUND,
    S_CONN_LISTENING,
    S_CONN_CLOSED,
    S_HOMA_INIT,
    S_HOMA_BOUND,
    S_HOMA_CLOSED,
};

enum socket_flags {
    SOF_NONBLOCK = 1,
    SOF_BOUND = 2,
    SOF_REUSEPORT = 4,
    SOF_REUSEADDR = 8,
};

struct eTran_socket_t {
    /* File descriptor, created by libc's eventfd() */
    int fd;
    /* The protocol of the socket */
    int protocol;
    /* Belongs to which eTran thread */
    struct app_ctx_per_thread *tctx;
    
    union {
        struct eTrantcp_connection *conn;
        struct eTranhoma_socket *hs;
    };
    struct eTrantcp_listener *listener;
    enum socket_status status;
    enum socket_type type;
    enum socket_flags flags;
    struct sockaddr_in addr;

    uint32_t epoll_events;
    
    spinlock_t lock;
    struct kref ref;
    
    void (*release)(struct kref *kref);

    std::map<int, struct eTran_epoll_item *> *epoll_items;

    eTran_socket_t() {
        fd = 0;
        protocol = 0;
        tctx = nullptr;
        conn = nullptr;
        listener = nullptr;
        status = S_UNUSED;
        type = SOCKET_TYPE_SOCKET;
        flags = {};
        addr = {};
        epoll_events = 0;
        spin_lock_init(&lock);
        kref_init(&ref);
        epoll_items = new std::map<int, struct eTran_epoll_item *>();
    }
    
    ~eTran_socket_t() {
        delete epoll_items;
    }
};

struct eTran_epoll {
    int epfd;

    struct app_ctx_per_thread *tctx;

    std::map<int, struct eTran_epoll_item *> inactive_list;
    std::map<int, struct eTran_epoll_item *> active_list;
    spinlock_t lock;

    int num_linux;

    int num_eTran;

    bool linux_first;

    eTran_epoll(int fd, struct app_ctx_per_thread *tctx) {
        spin_lock_init(&lock);
        this->epfd = fd;
        this->tctx = tctx;
        this->num_linux = 0;
        this->num_eTran = 0;
    }

};

struct eTran_epoll_item {
    struct eTran_epoll *epoll;
    struct eTran_socket_t *socket;
    uint32_t interest_events;

    epoll_data_t data;

    eTran_epoll_item(struct eTran_epoll *epoll, struct eTran_socket_t *socket, epoll_data_t data, uint32_t events) {
        this->epoll = epoll;
        this->socket = socket;
        this->data = data;
        this->interest_events = events;
    }
};


enum fh_type{
    FH_UNUSED = 0,
    FH_SOCKET,
    FH_EPOLL,
};

struct eTran_file_handle {
    union {
        struct eTran_socket_t *socket;
        struct eTran_epoll *epoll;
    } data;

    enum fh_type type;
    
    eTran_file_handle() {
        this->data = {0};
        this->type = FH_UNUSED;
    }
};