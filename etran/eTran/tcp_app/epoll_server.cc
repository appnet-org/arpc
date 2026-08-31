#include <arpa/inet.h>
#include <sys/epoll.h>
#include <sys/socket.h>
#include <sys/types.h>
#include <unistd.h>
#include <fcntl.h>
#include <netinet/in.h>
#include <assert.h>
#include <mutex>

#include <iostream>
#include <thread>
#include <list>
#include <string>
#include <unordered_map>
#include <atomic>

#define MAX_THREADS 16

#define DATA_BLOCK_SIZE 65536
#define LISTEN_BACKLOG 512
#define SHORT_RESPONSE_SIZE 100

// parameters
unsigned int max_buf_size = 4096;
bool dump_io_stats = false;
bool short_response = true;
unsigned int nr_threads = 1;
unsigned int nr_queues = 1;
unsigned int message_bytes = 100;
std::string server_ip_str = "192.168.6.2"; // FXIME: this is not used in this test
uint16_t server_port = 50000;

std::list<std::thread> threads;

uint64_t total_in = 0;
uint64_t total_out = 0;
static std::atomic<uint64_t> total_recv_bytes[MAX_THREADS] = {};
uint64_t prev_total_recv_bytes[MAX_THREADS] = {};
static std::atomic<uint64_t> total_resp_bytes[MAX_THREADS] = {};
uint64_t prev_total_resp_bytes[MAX_THREADS] = {};
static std::atomic<uint32_t> avg_nr_events(0);

struct connection {
    int fd;
    unsigned int pending_bytes;
    unsigned int message_bytes;
    unsigned int unsent_bytes;
    unsigned int response_bytes;
    uint32_t recv_len;
    bool has_epoll_out;
    char *buf;

    connection(int fd, unsigned int message_bytes, bool short_response) : fd(fd), message_bytes(message_bytes) {
        has_epoll_out = 0;
        pending_bytes = 0;
        recv_len = 0;
        unsent_bytes = 0;
        response_bytes = short_response ? SHORT_RESPONSE_SIZE : message_bytes;
        buf = (char *)calloc(1, max_buf_size);
    }
};

std::list<int> conn_fds;

static inline int listen_socket(int fd, int epfd, uint32_t events)
{
    struct epoll_event ev;
    if (events & EPOLLERR) {
        fprintf(stderr, "Error on listen socket\n");
        close(fd);
        return -1;
    }
    if (events & EPOLLIN) {
        int newfd = accept(fd, NULL, NULL);
        if (newfd > 0) {
            conn_fds.push_back(newfd);
            // set this socket to NOBLOCK
            if (fcntl(newfd, F_SETFL, fcntl(newfd, F_GETFL, 0) | O_NONBLOCK)) {
                fprintf(stderr, "Failed to set non-blocking\n");
                close(newfd);
                return 0;
            }
            ev.events = EPOLLIN | EPOLLERR | EPOLLHUP;
            ev.data.ptr = new connection(newfd, message_bytes, short_response);
            if (epoll_ctl(epfd, EPOLL_CTL_ADD, newfd, &ev)) {
                fprintf(stderr, "Failed to add newfd to epoll\n");
                return 0;
            }
        }
    }
    return 0;
}

static inline int connection_send(unsigned int tid, struct connection *c)
{
    int need_epoll_out = 0;
    ssize_t ret;
    uint32_t target_bytes;
    while (c->pending_bytes) {
        target_bytes = std::min(c->pending_bytes, c->response_bytes + c->unsent_bytes);
        ret = write(c->fd, c->buf, std::min(target_bytes, (unsigned int)DATA_BLOCK_SIZE));
        if (ret > 0) {
            c->pending_bytes -= ret;
            total_resp_bytes[tid].fetch_add(ret);
            // accumulate unsent_bytes
            c->unsent_bytes += target_bytes - ret;
        } else {
            need_epoll_out = 1;
            break;
        }
    }
    return need_epoll_out;
}

static inline void connection_recv(unsigned int tid, struct connection *c)
{
    ssize_t ret;
    while (1) {
        ret = read(c->fd, c->buf + c->recv_len, c->message_bytes - c->recv_len);
        if (ret > 0) {
            c->recv_len += ret;
            total_recv_bytes[tid].fetch_add(ret);
        } else {
            break;
        }
        if (c->recv_len == c->message_bytes) {
            c->recv_len = 0;
            c->pending_bytes += c->response_bytes;
        }
    }
}

static inline int connection_events(unsigned int tid, struct connection *c, uint32_t events)
{
    if (events & EPOLLIN) {
        connection_recv(tid, c);
    }

    return connection_send(tid, c);
}

void thread_func(unsigned int tid)
{
    int epfd;
    struct epoll_event ev, events[256];
    struct in_addr server_ip_addr;
    uint16_t t_server_port = server_port + tid;
    // uint16_t t_server_port = server_port;
    struct connection *c;
    
    assert(inet_pton(AF_INET, server_ip_str.c_str(), &server_ip_addr) == 1);

    int fd = socket(AF_INET, SOCK_STREAM | SOCK_NONBLOCK, 0);
    if (fd < 0) {
        fprintf(stderr, "Failed to create socket\n");
        return;
    }

    struct sockaddr_in server_addr;
    server_addr.sin_family = AF_INET;
    server_addr.sin_port = htons(t_server_port);
    server_addr.sin_addr.s_addr = server_ip_addr.s_addr;

    if (bind(fd, (struct sockaddr *)&server_addr, sizeof(server_addr)) < 0) {
        fprintf(stderr, "Failed to bind to server\n");
        close(fd);
        return;
    }

    if (listen(fd, LISTEN_BACKLOG)) {
        fprintf(stderr, "Failed to listen\n");
        close(fd);
        return;
    }
    printf("Server thread#%u listen on %s:%d\n", tid, server_ip_str.c_str(), t_server_port);

    epfd = epoll_create1(0);

    if (epfd < 0) {
        fprintf(stderr, "Failed to create epoll\n");
        close(fd);
        return;
    }

    ev.events = EPOLLIN;
    ev.data.ptr = NULL;

    if (epoll_ctl(epfd, EPOLL_CTL_ADD, fd, &ev) < 0) {
        fprintf(stderr, "Failed to add fd to epoll\n");
        close(fd);
        close(epfd);
        return;
    }

    while (1) {

        int nfds = epoll_wait(epfd, events, 128, -1);
        if (nfds) avg_nr_events.store((avg_nr_events.load() + nfds) / 2);
        for (int i = 0; i < nfds; i++) {
            c = (struct connection *)events[i].data.ptr;

            if (c == NULL) {
                if (listen_socket(fd, epfd, events[i].events))
                    abort();
                continue;
            }

            if (events[i].events & EPOLLERR || events[i].events & EPOLLHUP) {
                conn_fds.remove(c->fd);
                // remove from epoll
                epoll_ctl(epfd, EPOLL_CTL_DEL, c->fd, NULL);
                close(c->fd);
                delete c;
                continue;
            }

            int ret = connection_events(tid, c, events[i].events);

            if (ret == 1 && !c->has_epoll_out) {
                ev.events = EPOLLIN | EPOLLOUT;
                ev.data.ptr = c;
                if (epoll_ctl(epfd, EPOLL_CTL_MOD, c->fd, &ev) < 0) {
                    fprintf(stderr, "Failed to add fd to epoll\n");
                    close(c->fd);
                    close(epfd);
                    return;
                }
                c->has_epoll_out = true;
            } else if (ret == 0 && c->has_epoll_out)  {
                ev.events = EPOLLIN;
                ev.data.ptr = c;
                if (epoll_ctl(epfd, EPOLL_CTL_MOD, c->fd, &ev) < 0) {
                    fprintf(stderr, "Failed to add fd to epoll\n");
                    close(c->fd);
                    close(epfd);
                    return;
                }
                c->has_epoll_out = false;
            }
        }

    }

    close(epfd);
    close(fd);
    for (auto &fd : conn_fds) {
        close(fd);
    }
}

int parse_args(int argc, char *argv[])
{
    int opt;
    while ((opt = getopt(argc, argv, "t:q:f:b:i:p:sl:")) != -1) {
        switch (opt) {
            case 'b':
                message_bytes = std::stoi(optarg);
                break;
            case 'i':
                server_ip_str = optarg;
                break;
            case 'p':
                server_port = std::stoi(optarg);
                break;
            case 'd':
                dump_io_stats = true;
                break;
            case 'q':
                nr_queues = std::stoi(optarg);
                break;
            case 't':
                nr_threads = std::stoi(optarg);
                break;
            case 'l':
                max_buf_size = std::stoi(optarg);
                break;
            case 's':
                short_response = false;
                break;
            default:
                std::cout << "Usage: " << argv[0] << 
                    " [-t nr_threads, default:1]" <<
                    " [-l max_buf_size, default:4096]"
                    " [-q nr_queues, default:1]" <<
                    " [-b bytes, default:100]" << 
                    " [-i server_ip, default:192.168.6.2]" <<
                    " [-p server_port, default:50000]" << 
                    " [-s enable short_response, default:true]" << 
                    " [-d dump_io_stats]" << std::endl;
                return -1;
        }
    }
    return 0;
}

int main(int argc, char *argv[])
{
    if (parse_args(argc, argv))
    {
        std::cout << "Failed to parse arguments." << std::endl;
        exit(EXIT_FAILURE);
    }
    
    for (unsigned int i = 0; i < nr_threads; i++) {
        threads.push_back(std::thread(thread_func, i));
    }

    std::thread([]() {
        while (1) {
            sleep(1);

            unsigned _in = 0;
            unsigned _out = 0;

            for (unsigned int i = 0; i < nr_threads; i++) {
                _in += total_recv_bytes[i].load() - prev_total_recv_bytes[i];
                _out += total_resp_bytes[i].load() - prev_total_resp_bytes[i];
                prev_total_recv_bytes[i] = total_recv_bytes[i].load();
                prev_total_resp_bytes[i] = total_resp_bytes[i].load();
            }
            total_in += _in;
            total_out += _out;

            printf("Throughput In/Out(%.2f/%.2f Gbps)(%.2f Kops) conn#(%lu), avg_nr_events(%u), total_recv(%luB), total_resp(%luB)\n", 
                _in * 8.0 / 1e9, _out * 8.0 / 1e9, _in / message_bytes / 1e3,
                conn_fds.size(), avg_nr_events.load(), total_in, total_out);
        }
    }).detach();

    for (auto &t : threads) {
        t.join();
    }

    return 0;
}