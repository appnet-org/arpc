/*
 * Copyright 2019 University of Washington, Max Planck Institute for
 * Software Systems, and The University of Texas at Austin
 *
 * Permission is hereby granted, free of charge, to any person obtaining
 * a copy of this software and associated documentation files (the
 * "Software"), to deal in the Software without restriction, including
 * without limitation the rights to use, copy, modify, merge, publish,
 * distribute, sublicense, and/or sell copies of the Software, and to
 * permit persons to whom the Software is furnished to do so, subject to
 * the following conditions:
 *
 * The above copyright notice and this permission notice shall be
 * included in all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
 * EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
 * MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
 * IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
 * CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
 * TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
 * SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 */

#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <stdint.h>
#include <sys/types.h>
#include <sys/socket.h>
#include <netinet/in.h>
#include <netinet/tcp.h>
#include <arpa/inet.h>
#include <fcntl.h>
#include <string.h>
#include <pthread.h>
#include <signal.h>
#include <netdb.h>
#include <errno.h>
#include <assert.h>
#include <locale.h>
#include <inttypes.h>

#include <algorithm>

#include "flexkvs/protocol_binary.h"

extern "C" {
#include "flexkvs/benchmark.h"
#include "flexkvs/socket_shim.h"
}

#ifdef USE_MTCP
# include <mtcp_api.h>
# include <mtcp_epoll.h>
#else
# include <sys/epoll.h>
#endif

// #define MIN(a,b) ((b) < (a) ? (b) : (a))

#define CONN_DEBUG(c, co, x...) do { } while (0)
/*#define CONN_DEBUG(c, co, x...) \
    do { printf("%d.%d: ", (int) c->id, co->fd); \
         printf(x); } while (0)*/

#define PRINT_STATS
#ifdef PRINT_STATS
#   define STATS_ADD(c, f, n) c->f += n
#else
#   define STATS_ADD(c, f, n) do { } while (0)
#endif

#define HIST_START_US 0
#define HIST_BUCKET_US 1
#define HIST_BUCKETS 65536
#define BUFSIZE 8192


enum conn_state {
    APP_CONN_CLOSED = 0,
    APP_CONN_CONNECTING = 1,
    APP_CONN_OPEN = 2,
};

enum benchmark_phase {
    BENCHMARK_INIT,
    BENCHMARK_PRELOAD,
    BENCHMARK_WARMUP,
    BENCHMARK_RUNNING,
    BENCHMARK_COOLDOWN,
    BENCHMARK_DONE,
};

struct bench_settings bench_settings;
static struct workload workload;
static volatile enum benchmark_phase phase;
static volatile uint16_t init_count = 0;
/*static uint32_t max_pending = 64;*/

struct connection {
    enum conn_state state;
    int fd;
    int ep_wr;
    uint32_t pending;
    uint32_t tx_len;
    uint32_t tx_off;
    uint32_t rx_len;
    void *rx_buf;
    void *tx_buf;
    struct connection *next;
#ifdef PRINT_STATS
    uint64_t cnt;
#endif
};

struct core {
    struct connection *conns;
#ifdef PRINT_STATS
    uint64_t tx_get;
    uint64_t tx_set;
    uint64_t rx_get;
    uint64_t rx_set;
    uint64_t rx_success;
    uint64_t rx_fail;
    uint64_t rx_calls;
    uint64_t rx_nanos;
    uint64_t tx_calls;
    uint64_t tx_nanos;
    uint64_t epupd;
    uint32_t *hist;
#endif
    int ep;
    ssctx_t sc;
#ifdef USE_MTCP
    mctx_t mc;
#endif
    struct workload_core wlc;
    uint64_t id;
    pthread_t pthread;
    uint16_t conn_pending;

    uint32_t msgs_pending;
    struct connection *q_first;
    struct connection *q_last;
    uint32_t rate_tokens;
    uint32_t rate_batch;
    uint64_t next_refill_ns;
} __attribute__((aligned(64)));

static inline uint64_t get_nanos(void)
{
    struct timespec ts;
    clock_gettime(CLOCK_MONOTONIC_RAW, &ts);
    return (uint64_t) ts.tv_sec * 1000 * 1000 * 1000 + ts.tv_nsec;
}

static inline uint64_t get_rate_nanos(void)
{
    struct timespec ts;
    clock_gettime(CLOCK_MONOTONIC, &ts);
    return (uint64_t) ts.tv_sec * 1000 * 1000 * 1000 + ts.tv_nsec;
}

static inline void refill_rate_tokens(struct core *c)
{
    if (bench_settings.target_mops <= 0)
        return;

    const uint64_t interval_ns = 100000;
    uint64_t now = get_rate_nanos();
    if (now < c->next_refill_ns)
        return;

    uint64_t intervals = 1 + (now - c->next_refill_ns) / interval_ns;
    uint64_t tokens = c->rate_tokens + intervals * c->rate_batch;
    c->rate_tokens = std::min(tokens, (uint64_t)c->rate_batch * 2);
    c->next_refill_ns += intervals * interval_ns;
}

static inline void wait_for_rate_tokens(struct core *c)
{
    if (bench_settings.target_mops <= 0 || c->rate_tokens ||
            c->msgs_pending)
        return;

    uint64_t now = get_rate_nanos();
    if (now >= c->next_refill_ns)
        return;

    uint64_t delay = c->next_refill_ns - now;
    struct timespec ts = {
        .tv_sec = (time_t)(delay / 1000000000),
        .tv_nsec = (long)(delay % 1000000000),
    };
    nanosleep(&ts, NULL);
}



static uint64_t ns_lat_list[500000];
static int ns_lat_list_idx = 0;
static uint64_t ns_lat_total = 0;
static uint64_t ns_lat_sum = 0;

static inline void record_latency(struct core *c, uint64_t nanos)
{
    size_t bucket = ((nanos / 1000) - HIST_START_US) / HIST_BUCKET_US;
    if (bucket >= HIST_BUCKETS) {
        bucket = HIST_BUCKETS - 1;
    }
    c->hist[bucket]++;

    if (c->id == 0) {
        if (ns_lat_list_idx >= 500000) {
            ns_lat_list_idx = 0;
        }
        ns_lat_list[ns_lat_list_idx++] = nanos;
        ns_lat_total++;
        ns_lat_sum += nanos;
    }

    /*__sync_fetch_and_add(&c->hist[bucket], 1)*/;
}

#ifdef PRINT_STATS
static inline uint64_t read_cnt(uint64_t *p)
{
  uint64_t v = *p;
  __sync_fetch_and_sub(p, v);
  return v;
}
#endif

static inline void conn_epupdate(struct core *c, struct connection *co, int wr)
{
    ss_epev_t ev;
    int cn = c->id;

    if (co->ep_wr == wr) {
        return;
    }
    /*printf("conn_epupdate(%p, %d)\n", co, wr);*/

    ev.data.ptr = co;
    ev.events = SS_EPOLLHUP | SS_EPOLLERR | (wr ? SS_EPOLLOUT : 0) | SS_EPOLLIN;
    if (ss_epoll_ctl(c->sc, c->ep, SS_EPOLL_CTL_MOD, co->fd, &ev) != 0) {
        perror("epoll_ctl failed");
        fprintf(stderr, "[%d] epoll_ctl failed\n", cn);
        abort();
    }
    co->ep_wr = wr;

    STATS_ADD(c, epupd, 1);
}

/* Open connection */
static inline void conn_connect(struct core *c, struct connection *co)
{
    int fd, cn, ret;
    ssctx_t sc;
    ss_epev_t ev;
    struct sockaddr_in addr;

    cn = c->id;
    sc = c->sc;
    CONN_DEBUG(c, co, "Opening new connection\n");

    /* create socket */
    if ((fd = ss_socket(sc, AF_INET, SOCK_STREAM, IPPROTO_TCP))
        < 0)
    {
        perror("creating socket failed");
        fprintf(stderr, "[%d] socket failed\n", cn);
        abort();
    }

    // /* disable nagling */
    // if (ss_set_nonagle(sc, fd) != 0) {
    //     fprintf(stderr, "[%d] setsockopt TCP_NODELAY failed\n", cn);
    //     abort();
    // }

    /* add to epoll */
    ev.data.ptr = co;
    ev.events = SS_EPOLLIN | SS_EPOLLOUT | SS_EPOLLHUP | SS_EPOLLERR;
    if (ss_epoll_ctl(sc, c->ep, SS_EPOLL_CTL_ADD, fd, &ev) < 0) {
      fprintf(stderr, "[%d] adding to epoll failed\n", cn);
    }

    /* initialize address of socket */
    addr.sin_family = AF_INET;
    addr.sin_addr.s_addr = htonl(bench_settings.dstip);
    addr.sin_port = htons(bench_settings.dstport);

    /* initiate blocking connect*/
    ret = ss_connect(sc, fd, (struct sockaddr *) &addr, sizeof(addr));
    if (ret == 0) {
        /* success */
        CONN_DEBUG(c, co, "Connection succeeded\n");
        co->state = APP_CONN_OPEN;
    } else if (ret < 0 && errno == EINPROGRESS) {
        /* still going on */
        CONN_DEBUG(c, co, "Connection pending: %d\n", fd);
        co->state = APP_CONN_CONNECTING;
    } else {
        /* opening connection failed */
        fprintf(stderr, "[%d] connect failed: %d\n", cn, ret);
        abort();
    }

    /* make socket non-blocking */
    if ((ret = ss_set_nonblock(sc, fd)) != 0) {
        fprintf(stderr, "[%d] set_nonblock failed: %d\n", cn, ret);
        abort();
    }

    co->fd = fd;
    co->ep_wr = 1;
    co->pending = 0;
    co->rx_len = 0;
    co->tx_len = 0;
    co->tx_off = 0;

    conn_epupdate(c, co, 0);

    co->next = NULL;
    if (c->q_last == NULL) {
      c->q_last = co;
      c->q_first = co;
    } else {
      c->q_last->next = co;
      c->q_last = co;
    }

#ifdef PRINT_STATS
    co->cnt = 0;
#endif
}

/* receive response on connection */
static inline int conn_recv(struct core *c, struct connection *co,
        enum workload_op *op, uint16_t *err, uint32_t *opaque)
{
    protocol_binary_response_header *res =
        (protocol_binary_response_header *) co->rx_buf;
    int cn = c->id;
    size_t reqlen;
    ssize_t ret;
    //uint64_t tsc;

    /* while response incomplete: receive more */
    while (co->rx_len < sizeof(*res) ||
            co->rx_len < sizeof(*res) + ntohl(res->response.bodylen))
    {
        //tsc = get_nanos();
        ret = ss_read(c->sc, co->fd, (uint8_t *) co->rx_buf + co->rx_len,
            BUFSIZE - co->rx_len);
        /*STATS_ADD(c, rx_nanos, get_nanos() - tsc);
        STATS_ADD(c, rx_calls, 1);*/

        if (ret > 0) {
            co->rx_len += ret;
        } else if (ret < 0 && errno == EAGAIN) {
            /* nothing to receive */
            return 1;
        } else {
            /* errror, close connection */
            fprintf(stderr, "[%d] read failed: %d\n", cn, (int) ret);
            abort();
            return -1;
        }
    }

    /* response is complete now */
    if (res->response.magic != PROTOCOL_BINARY_RES) {
        fprintf(stderr, "[%d] invalid magic on response: %x\n", cn,
                res->response.magic);
        abort();
    }

    reqlen = sizeof(*res) + ntohl(res->response.bodylen);

    if (res->response.opcode == PROTOCOL_BINARY_CMD_GET) {
        *op = WL_OP_GET;
    } else if (res->response.opcode == PROTOCOL_BINARY_CMD_SET) {
        *op = WL_OP_SET;
    } else {
        fprintf(stderr, "[%d] conn_recv: unknown opcode=%x\n", cn,
                res->response.opcode);
    }
    *err = ntohs(res->response.status);
    *opaque = res->response.opaque;

    if (co->rx_len > reqlen) {
        memmove(co->rx_buf, (uint8_t *) co->rx_buf + reqlen,
                co->rx_len - reqlen);
    }
    co->rx_len -= reqlen;
    return 0;
}

/* send out request on connection */
static inline int conn_send(struct core *c, struct connection *co)
{
    int cn;
    ssize_t ret;
    //uint64_t tsc;

    assert(co->tx_len > 0);
    assert(co->tx_off < co->tx_len);

    cn = c->id;

    //tsc = get_nanos();
    ret = ss_write(c->sc, co->fd, (uint8_t *) co->tx_buf + co->tx_off,
            co->tx_len - co->tx_off);

    //STATS_ADD(c, tx_nanos, get_nanos() - tsc);
    //STATS_ADD(c, tx_calls, 1);
    if (ret > 0) {
        co->tx_off += ret;
        if (co->tx_off == co->tx_len) {
            /* sent whole message */
            co->tx_off = 0;
            co->tx_len = 0;
            return 0;
        } else {
            return 1;
        }
    } else if (ret < 0 && errno != EAGAIN) {
        /* send failed */
        fprintf(stderr, "[%d] write failed: %d\n", cn, (int) ret);
        abort();
        return -1;
    } else if (ret < 0 && errno == EAGAIN) {
        /* send would block */
        return 1;
    }

    return -1;
}

static inline void set_request(struct core *c, struct connection *co,
        struct key *k, uint32_t opaque)
{
    protocol_binary_request_set *set =
        (protocol_binary_request_set *) co->tx_buf;
    protocol_binary_request_header *req = &set->message.header;

    assert(co->tx_off == 0);
    assert(co->tx_len == 0);
    assert(sizeof(*set) + k->keylen + bench_settings.valuesize <= BUFSIZE);

    req->request.magic = PROTOCOL_BINARY_REQ;
    req->request.opcode = PROTOCOL_BINARY_CMD_SET;
    req->request.keylen = htons(k->keylen);
    req->request.extlen = 8;
    req->request.datatype = 0;
    req->request.reserved = 0;
    req->request.bodylen = htonl(8 + k->keylen + bench_settings.valuesize);
    req->request.opaque = opaque;
    req->request.cas = 0;
    set->message.body.flags = 0;
    set->message.body.expiration = 0;

    memcpy(set + 1, k->key, k->keylen);

    co->tx_len = sizeof(*set) + k->keylen + bench_settings.valuesize;
}

static inline void get_request(struct core *c, struct connection *co,
        struct key *k, uint32_t opaque)
{
    protocol_binary_request_get *get =
        (protocol_binary_request_get *) co->tx_buf;
    protocol_binary_request_header *req = &get->message.header;

    assert(co->tx_off == 0);
    assert(co->tx_len == 0);
    assert(sizeof(*get) + k->keylen <= BUFSIZE);

    req->request.magic = PROTOCOL_BINARY_REQ;
    req->request.opcode = PROTOCOL_BINARY_CMD_GET;
    req->request.keylen = htons(k->keylen);
    req->request.extlen = 0;
    req->request.datatype = 0;
    req->request.reserved = 0;
    req->request.bodylen = htonl(k->keylen);
    req->request.opaque = opaque;
    req->request.cas = 0;

    memcpy(get + 1, k->key, k->keylen);

    co->tx_len = sizeof(*get) + k->keylen;
}

static void poll_conns(struct core *c, int timeout)
{
    struct connection *co;
    int ret, j, cn;
    ss_epev_t evs[32];
    // socklen_t slen;
    uint32_t events;

    cn = c->id;

    /* get events */
    if ((ret = ss_epoll_wait(c->sc, c->ep, evs, 32, timeout)) < 0) {
        fprintf(stderr, "[%d] wait_conns epoll_wait failed\n", cn);
        abort();
    }

    for (j = 0; j < ret; j++) {
        co = (connection*)evs[j].data.ptr;
        events = evs[j].events;

        /* we only exepect events for connections that are currently
         * connecting. */
        if (co->state != APP_CONN_CONNECTING) {
            fprintf(stderr, "[%d] wait_conns event on non-connecting "
                    "connection\n", cn);
            abort();
        }

        /* check for errors on the connection */
        if ((events & (SS_EPOLLERR | SS_EPOLLHUP)) != 0) {
            fprintf(stderr, "[%d] wait_conns error on conn\n", cn);
            abort();
        }

        /* check connection state */
        // slen = sizeof(status);
        // if (ss_getsockopt(c->sc, co->fd, SOL_SOCKET, SO_ERROR,
        //             &status, &slen) < 0)
        // {
        //     fprintf(stderr, "[%d] wait_conns getsockopt failed\n", cn);
        //     abort();
        // }
        // if (status != 0) {
        //     fprintf(stderr, "[%d] wait_conns conn failed\n", cn);
        //     abort();
        // }

        c->conn_pending--;
        co->state = APP_CONN_OPEN;
        conn_epupdate(c, co, 0);

        co->next = NULL;
        if (c->q_last == NULL) {
          c->q_last = co;
          c->q_first = co;
        } else {
          c->q_last->next = co;
          c->q_last = co;
        }

    }
}

/* initialize core */
static void prepare_core(struct core *c)
{
    int cn = c->id;
    uint32_t i;
    uint8_t *buf;
    ssctx_t sc;
#ifdef USE_MTCP
    int ret;
    struct sockaddr_in addr;
#endif

    /* Affinitize threads */
#ifdef USE_MTCP
    if ((ret = mtcp_core_affinitize(cn)) != 0) {
        fprintf(stderr, "[%d] mtcp_core_affinitize failed: %d\n", cn, ret);
        abort();
    }

    if ((sc = mtcp_create_context(cn)) == NULL) {
        fprintf(stderr, "[%d] mtcp_create_context failed\n", cn);
        abort();
    }

    /* initialize address of socket */
    addr.sin_family = AF_INET;
    addr.sin_addr.s_addr = htonl(bench_settings.dstip);
    addr.sin_port = htons(bench_settings.dstport);

    if ((ret = mtcp_init_rss(sc, INADDR_ANY, 1, addr.sin_addr.s_addr,
                    addr.sin_port)) != 0)
    {
        fprintf(stderr, "[%d] mtcp_init_rss failed\n", cn);
        abort();
    }
#else
    sc = NULL;
#endif
    c->sc = sc;

    /* create epoll */
    if ((c->ep = ss_epoll_create(sc, 4 * bench_settings.conns)) < 0) {
        fprintf(stderr, "[%d] epoll_create failed\n", c->ep);
        abort();
    }

    /* Allocate histogram */
    if ((c->hist = (unsigned int*)calloc(HIST_BUCKETS, sizeof(*c->hist))) == NULL) {
        fprintf(stderr, "[%d] allocating histogram failed\n", cn);
        abort();
    }

    /* Allocate connection structs */
    if ((c->conns = (connection*)calloc(bench_settings.conns, sizeof(*c->conns))) == NULL) {
        fprintf(stderr, "[%d] allocating connection structs failed\n", cn);
        abort();
    }

    /* Initiate connections */
    c->conn_pending = 0;
    c->msgs_pending = 0;
    c->q_first = c->q_last = NULL;
    for (i = 0; i < bench_settings.conns; i++) {
        if ((buf = (unsigned char*)malloc(BUFSIZE * 2)) == NULL) {
            fprintf(stderr, "[%d] allocating conn buffer failed\n", cn);
        }
        c->conns[i].rx_buf = buf;
        c->conns[i].tx_buf = buf + BUFSIZE;
        c->conns[i].state = APP_CONN_CONNECTING;
        c->conns[i].fd = -1;

        conn_connect(c, &c->conns[i]);
        // while (c->conn_pending >= 16) {
        //     poll_conns(c, 0);
        // }
    }

    // while (c->conn_pending > 0) {
    //     poll_conns(c, 0);
    // }
    fprintf(stderr, "core ready: %d\n", cn);

}

/* wait for all connections to be established */
static void wait_conns(struct core *c)
{
    uint32_t i;

    for (i = 0; i < bench_settings.conns; i++) {
        while (c->conns[i].state == APP_CONN_CONNECTING) {
            poll_conns(c, 0);
        }
    }
}

/* wait for all connections to be established */
static void load_keys(struct core *c)
{
    int cn, ret, l;
    size_t i, pending;
    struct key *k;
    struct connection *co;
    ss_epev_t evs[8];
    uint32_t events, opaque;
    uint16_t err;
    enum workload_op op;

    cn = c->id;

    /* set connections to trigger epoll OUT events */
    for (i = 0; i < (size_t)(std::min((uint16_t)32, bench_settings.conns)); i++) {
        conn_epupdate(c, &c->conns[i], 1);
    }

    i = cn;
    pending = 0;
    while (i < workload.keys_num || pending > 0) {
        // printf("call epoll_wait\n");
        if ((ret = ss_epoll_wait(c->sc, c->ep, evs, 8, -1)) < 0) {
            fprintf(stderr, "[%d] load_keys epoll_wait failed\n", cn);
            abort();
        }

        for (l = 0; l < ret; l++) {
            co = (connection*)evs[l].data.ptr;
            events = evs[l].events;

            /* check for errors on the connection */
            if ((events & (SS_EPOLLERR | SS_EPOLLHUP)) != 0) {
                fprintf(stderr, "[%d] load_keys error on conn\n", cn);
                abort();
            }

            if ((events & SS_EPOLLIN) == SS_EPOLLIN) {
                /* data ready to receive */
                if (conn_recv(c, co, &op, &err, &opaque) == 0) {
                    if (err != 0) {
                        fprintf(stderr, "[%d] load_keys set failed:%x\n", cn, err);
                    }
                    pending--;
                    co->pending--;
                    conn_epupdate(c, co, 1);
                }
            } else {
                /* ready to send */
                assert((events & SS_EPOLLOUT) == SS_EPOLLOUT);
                if (co->tx_len == 0 && i < workload.keys_num) {
                    /* send out new request */
                    // printf("[%d:%p] i=%zu  keys_num=%zu\n", cn, co, i, workload.keys_num);
                    k = &workload.keys[i];
                    set_request(c, co, k, 0);
                    if (conn_send(c, co) == 0) {
                        /* fully sent out -> poll for RX */
                        conn_epupdate(c, co, 0);
                    }
                    i += bench_settings.threads;
                    pending++;
                    co->pending++;
                } else if (co->tx_len == 0) {
                    /* no more keys to initialize -> poll for RX */
                    conn_epupdate(c, co, 0);
                } else {
                    if (conn_send(c, co) == 0) {
                        /* fully sent out -> poll for RX */
                        conn_epupdate(c, co, 0);
                    }
                }
            }

        }
    }

    /* set connections to trigger epoll OUT events */
    /*for (i = 0; i < bench_settings.conns; i++) {
        conn_epupdate(c, &c->conns[i], 1);
    }*/
}


static inline void conn_events(struct core *c, struct connection *co,
        uint32_t events)
{
    int cn;
    uint32_t opaque;
    uint16_t err;
    enum workload_op op = {};

    cn = c->id;

    /* check for errors on the connection */
    if ((events & (SS_EPOLLERR | SS_EPOLLHUP)) != 0) {
        fprintf(stderr, "[%d] error on connection\n", cn);
        abort();
    }

    /* receive responses */
    while (co->pending > 0 && conn_recv(c, co, &op, &err, &opaque) == 0) {
        if (phase == BENCHMARK_RUNNING) {
            record_latency(c, (uint32_t) get_nanos() - opaque);

            STATS_ADD(co, cnt, 1);
            if (op == WL_OP_GET) {
                STATS_ADD(c, rx_get, 1);
            } else {
                STATS_ADD(c, rx_set, 1);
            }
            if (err == 0) {
                STATS_ADD(c, rx_success, 1);
            } else {
                STATS_ADD(c, rx_fail, 1);
                fprintf(stderr, "[%d] request failed:%x\n", cn, err);
            }
        }
        co->pending--;
        c->msgs_pending--;

        if (co->pending == 0) {
          co->next = NULL;
          if (c->q_last == NULL) {
            c->q_last = co;
            c->q_first = co;
          } else {
            c->q_last->next = co;
            c->q_last = co;
          }
        }
    }

    /* try to send out any remaining tx buffer contents */
    if (co->tx_len != 0) {
        conn_send(c, co);
    }

    /* make sure we epoll for write iff we're actually blocked on writes */
    conn_epupdate(c, co, co->tx_len != 0);

}

static inline void send_pending(struct core *c)
{
    struct connection *co;
    uint32_t opaque;
    enum workload_op op = {};
    struct key *k;

    refill_rate_tokens(c);

    /* send out new requests */
    while (c->msgs_pending < bench_settings.pending && c->q_first != NULL &&
            (bench_settings.target_mops <= 0 || c->rate_tokens)) {
        if (bench_settings.request_gap) {
            uint64_t target = get_nanos() + bench_settings.request_gap;
            while (get_nanos() < target)
                __asm__ __volatile__("pause");
        }

        co = c->q_first;
        c->q_first = co->next;
        if (c->q_first == NULL)
          c->q_last = NULL;

        /* pick a key and operation */
        workload_op_f(&workload, &c->wlc, &k, &op);

        /* assign a time stamp */
        opaque = get_nanos();

        /* assemble request */
        if (op == WL_OP_GET) {
            get_request(c, co, k, opaque);
            STATS_ADD(c, tx_get, 1);
        } else {
            set_request(c, co, k, opaque);
            STATS_ADD(c, tx_set, 1);
        }

        /* send out request */
        conn_send(c, co);
        conn_epupdate(c, co, co->tx_len != 0);

        co->pending++;
        c->msgs_pending++;
        if (bench_settings.target_mops > 0)
            c->rate_tokens--;
    }
}


static void *thread_run(void *arg)
{
    struct core *c = (core*)arg;
    int i, cn, ret, ep, num_evs;
    struct connection *co;
    ssctx_t sc;
    ss_epev_t *evs;

    cpu_set_t cpuset;
    CPU_ZERO(&cpuset);
    CPU_SET(5 + c->id, &cpuset);
    pthread_setaffinity_np(pthread_self(), sizeof(cpuset), &cpuset);

    /* initiate core struct and connections */
    prepare_core(c);

    if (bench_settings.target_mops > 0) {
        double per_thread_rate =
            bench_settings.target_mops * 1000000 / bench_settings.threads;
        c->rate_batch = std::max(1U,
            (uint32_t)(per_thread_rate * 0.0001));
        c->rate_tokens = c->rate_batch;
        c->next_refill_ns = get_rate_nanos() + 100000;
    }

    cn = c->id;
    ep = c->ep;
    sc = c->sc;

    printf("[%d] Waiting for connections...\n", cn);
    fflush(stdout);

    /* wait for connections to be established */
    wait_conns(c);

     printf("[%d] Waiting for connections done\n", cn);
    fflush(stdout);

   /* wait until we start running */
    while (phase < BENCHMARK_PRELOAD) {
        sched_yield();
    }

    printf("[%d] Preloading keys...\n", cn);
    fflush(stdout);
    /* pre-load keys */
    load_keys(c);
    printf("[%d] Preloaded keys...\n", cn);
    fflush(stdout);

    __sync_fetch_and_add(&init_count, 1);

   /* wait until we start running */
    while (phase < BENCHMARK_WARMUP) {
        sched_yield();
    }

    num_evs = 32;
    if ((evs = (ss_epev_t*)calloc(num_evs, sizeof(*evs))) == NULL) {
        fprintf(stderr, "[%d] malloc failed\n", cn);
        abort();
    }

    printf("[%d] Start warmup...\n", cn);
    fflush(stdout);

    send_pending(c);

    while (phase != BENCHMARK_DONE) {
        /* epoll, wait for events */
        if ((ret = ss_epoll_wait(sc, ep, evs, num_evs, 10)) < 0) {
            fprintf(stderr, "[%d] epoll_wait failed\n", cn);
            abort();
        }

        for (i = 0; i < ret; i++) {
            co = (connection*)evs[i].data.ptr;
            conn_events(c, co, evs[i].events);
        }

        if (phase == BENCHMARK_WARMUP || phase == BENCHMARK_RUNNING)
            send_pending(c);
        wait_for_rate_tokens(c);
        if (phase == BENCHMARK_WARMUP || phase == BENCHMARK_RUNNING)
            send_pending(c);
    }

    return NULL;
}

static inline void hist_fract_buckets(uint32_t *hist, uint64_t total,
        double *fracs, size_t *idxs, size_t num)
{
    size_t i, j;
    uint64_t sum = 0, goals[num];
    for (j = 0; j < num; j++) {
        goals[j] = total * fracs[j];
    }
    for (i = 0, j = 0; i < HIST_BUCKETS && j < num; i++) {
        sum += hist[i];
        for (; j < num && sum >= goals[j]; j++) {
            idxs[j] = i;
        }
    }
}

static inline int hist_value(size_t i)
{
    if (i == HIST_BUCKETS - 1) {
        return -1;
    }

    return i * HIST_BUCKET_US + HIST_START_US;
}

int main(int argc, char *argv[])
{
    // ctrl+c handler
    // signal(SIGINT, [](int) {
    //     printf("dump\n");
    //     FILE *f = fopen("eTran.txt", "w");
    //     for (int i = 0; i < 500000; i++) {
    //         if (ns_lat_list[i] == 0) {
    //             break;
    //         }
    //         fprintf(f, "%d %lu\n", i, ns_lat_list[i]);
    //     }
    //     exit(0);
    // });

#ifdef USE_MTCP
    int ret;
#endif
    int i, j, num_threads;
    struct core *cs;
    uint64_t t_prev, t_cur;
    long double *ttp, tp, tp_total;
    uint32_t *hist, hx;
    uint64_t msg_total;
    double fracs[6] = { 0.5, 0.9, 0.95, 0.99, 0.999, 0.9999 };
    size_t fracs_pos[sizeof(fracs) / sizeof(fracs[0])];

    setlocale(LC_NUMERIC, "");

    /* parse bench_settings from command line */
    init_settings(&bench_settings);
    if (parse_settings(argc, argv, &bench_settings) != 0) {
        print_usage();
        return EXIT_FAILURE;
    }
    num_threads = bench_settings.threads;

    /* initialize workload */
    workload_init(&workload);

#ifdef USE_MTCP
    if ((ret = mtcp_init("/tmp/mtcp.conf")) != 0) {
        fprintf(stderr, "mtcp_init failed: %d\n", ret);
        return EXIT_FAILURE;
    }
#endif

    /* allocate core structs */
    assert(sizeof(*cs) % 64 == 0);
    cs = (core*)calloc(num_threads, sizeof(*cs));

    /* allocate instrumentation structs */
    ttp = (long double*)calloc(num_threads, sizeof(*ttp));
    hist = (uint32_t*)calloc(HIST_BUCKETS, sizeof(*hist));

    if (cs == NULL || ttp == NULL || hist == NULL) {
        fprintf(stderr, "allocation failed failed\n");
        return EXIT_FAILURE;
    }

    phase = BENCHMARK_INIT;

    for (i = 0; i < num_threads; i++) {
        cs[i].id = i;
        workload_core_init(&workload, &cs[i].wlc);
        if (pthread_create(&cs[i].pthread, NULL, thread_run, cs + i)) {
            fprintf(stderr, "pthread_create failed\n");
            return EXIT_FAILURE;
        }
    }

    //sleep(10);
    phase = BENCHMARK_PRELOAD;
    //sleep(10);
    phase = BENCHMARK_WARMUP;

    while (init_count < num_threads) {
        sched_yield();
    }
    printf("Preloading completed\n");
    fflush(stdout);
    sleep(bench_settings.warmup_time);
    for (i = 0; i < num_threads; i++) {
        read_cnt(&cs[i].rx_success);
        memset(cs[i].hist, 0, sizeof(*cs[i].hist) * HIST_BUCKETS);
    }
    ns_lat_list_idx = 0;
    ns_lat_total = 0;
    ns_lat_sum = 0;

    printf("Warmup completed; measuring one-message latency\n");
    fflush(stdout);
    const char *marker_path = getenv("ETRAN_MEASURE_MARKER");
    if (marker_path) {
        int marker_fd = open(marker_path, O_CREAT | O_WRONLY | O_TRUNC, 0644);
        if (marker_fd >= 0)
            close(marker_fd);
    }
    phase = BENCHMARK_RUNNING;
    t_prev = get_nanos();
    sleep(bench_settings.run_time);
    t_cur = get_nanos();
    phase = BENCHMARK_COOLDOWN;
    sleep(bench_settings.cooldown_time);
    phase = BENCHMARK_DONE;
    if (marker_path)
        unlink(marker_path);

    for (i = 0; i < num_threads; i++)
        pthread_join(cs[i].pthread, NULL);

    tp_total = 0;
    msg_total = 0;
    for (i = 0; i < num_threads; i++) {
        tp = read_cnt(&cs[i].rx_success);
        tp /= (double) (t_cur - t_prev) / 1000000000.;
        ttp[i] = tp;
        tp_total += tp;

        for (j = 0; j < HIST_BUCKETS; j++) {
            hx = cs[i].hist[j];
            msg_total += hx;
            hist[j] += hx;
        }
    }

    if (msg_total == 0) {
        fprintf(stderr, "No requests completed during measurement\n");
        return EXIT_FAILURE;
    }

    hist_fract_buckets(hist, msg_total, fracs, fracs_pos,
            sizeof(fracs) / sizeof(fracs[0]));
    size_t ns_count = std::min(ns_lat_total,
            (uint64_t)(sizeof(ns_lat_list) / sizeof(ns_lat_list[0])));
    std::sort(ns_lat_list, ns_lat_list + ns_count);
    printf("RESULT requests=%" PRIu64 " throughput=%'.4Lf mops "
            "p50=%d us p90=%d us p95=%d us p99=%d us p99.9=%d us p99.99=%d us "
            "mean_ns=%" PRIu64 " p50_ns=%" PRIu64 " p99_ns=%" PRIu64 "\n",
            msg_total, tp_total / 1000000.,
            hist_value(fracs_pos[0]), hist_value(fracs_pos[1]),
            hist_value(fracs_pos[2]), hist_value(fracs_pos[3]),
            hist_value(fracs_pos[4]), hist_value(fracs_pos[5]),
            ns_lat_sum / ns_lat_total,
            ns_lat_list[ns_count / 2], ns_lat_list[ns_count * 99 / 100]);

#ifdef USE_MTCP
    mtcp_destroy();
#endif
}
