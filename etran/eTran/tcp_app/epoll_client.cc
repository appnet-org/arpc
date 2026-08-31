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
#define SHORT_RESPONSE_SIZE 100

unsigned int max_buf_size = 4096;
int wait_seconds = 0;
bool multiport = false;
bool dump_io_stats = false;
bool short_response = true;
unsigned int max_outstanding = 1;
unsigned int nr_flows = 1;
unsigned int nr_threads = 1;
unsigned int nr_queues = 1;
unsigned int message_bytes = 100;
std::string server_ip_str = "192.168.6.2";
uint16_t server_port = 50000;

std::list<std::thread> threads;
std::mutex conn_fds_mtx;
std::list<int> conn_fds;

std::mutex mtx;
static unsigned int ready_threads = 0;

uint64_t total_out = 0;
uint64_t total_in = 0;
static std::atomic<uint64_t> total_req_bytes[MAX_THREADS] = {};
uint64_t prev_total_req_bytes[MAX_THREADS] = {};
static std::atomic<uint64_t> total_resp_bytes[MAX_THREADS] = {};
uint64_t prev_total_resp_bytes[MAX_THREADS] = {};
static std::atomic<uint32_t> avg_nr_events(0);

struct connection {
    int fd;
    unsigned int recv_len;
    unsigned int pending_bytes;
    unsigned int total_bytes;
    unsigned int message_bytes;
    unsigned int max_outstanding;
    char *buf;
    bool no_epoll_out;
    
    connection(int fd, unsigned int message_bytes, unsigned int max_outstanding) : fd(fd), message_bytes(message_bytes), max_outstanding(max_outstanding) {
        no_epoll_out = false;
        recv_len = 0;
        total_bytes = message_bytes * max_outstanding;
        pending_bytes = total_bytes;
        buf = (char *)calloc(1, total_bytes);
    }
};

static inline int connection_send(unsigned int tid, struct connection *c)
{
    ssize_t ret;
    uint32_t target_bytes;
    int need_epoll_out = 0;
    // Transmit messages as much as possible through this connection until we reach max_outstanding or no buffer space
    while (c->pending_bytes) {
        target_bytes = std::min(c->pending_bytes, c->message_bytes);
        ret = write(c->fd, c->buf + (c->total_bytes - c->pending_bytes), std::min(target_bytes, (unsigned int)DATA_BLOCK_SIZE));
        if (ret > 0) {
            c->pending_bytes -= ret;
            total_req_bytes[tid].fetch_add(ret);
        }
        else {
            // no buffer space
            need_epoll_out = 1;
            break;
        }
    }
    return need_epoll_out;
}

static inline void connection_recv(unsigned int tid, struct connection *c)
{
    ssize_t ret;
    bool wait_response = c->pending_bytes + c->message_bytes <= c->total_bytes;
    // Receive messages as much as possible through this connection if there are outstanding messages
    while (wait_response) {
        uint32_t target_bytes = short_response ? SHORT_RESPONSE_SIZE : message_bytes;
        ret = read(c->fd, c->buf + c->recv_len, std::min(target_bytes, (unsigned int)DATA_BLOCK_SIZE));
        if (ret > 0) {
            c->recv_len += ret;
            total_resp_bytes[tid].fetch_add(ret);
        } else {
            // no more data
            break;
        }
        if (c->recv_len >= target_bytes) {
            c->recv_len -= target_bytes;
            c->pending_bytes += message_bytes;
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
    struct connection *c;
    int epfd;
    struct epoll_event ev, events[256];
    struct in_addr server_ip_addr;
    uint16_t t_server_port;
    if (multiport)
        t_server_port = server_port + tid;
    else
        t_server_port = server_port;
    unsigned int t_nr_flows = nr_flows / nr_threads;
    if (t_nr_flows == 0) {
        t_nr_flows = 1;
    }
    
    assert(inet_pton(AF_INET, server_ip_str.c_str(), &server_ip_addr) == 1);

    while (1) {
        mtx.lock();
        if (tid == ready_threads) {
            mtx.unlock();
            break;
        }
        mtx.unlock();
    }

    epfd = epoll_create1(0);

    if (epfd < 0) {
        fprintf(stderr, "Failed to create epoll\n");
        return;
    }

    for (unsigned int i = 0; i < t_nr_flows; i++) {
        int fd = socket(AF_INET, SOCK_STREAM, 0);
        if (fd < 0) {
            fprintf(stderr, "Failed to create socket\n");
            perror("\n");
            return;
        }

        struct sockaddr_in server_addr;
        server_addr.sin_family = AF_INET;
        server_addr.sin_port = htons(t_server_port);
        server_addr.sin_addr.s_addr = server_ip_addr.s_addr;
        if (connect(fd, (struct sockaddr *)&server_addr, sizeof(server_addr)) < 0) {
            fprintf(stderr, "Failed to connect to server\n");
            perror("connect");
            close(fd);
            return;
        }

        // close(fd);

        if (fcntl(fd, F_SETFL, fcntl(fd, F_GETFL, 0) | O_NONBLOCK) < 0) {
            fprintf(stderr, "Failed to set non-blocking\n");
            close(fd);
            return;
        }

        ev.events = EPOLLIN | EPOLLOUT | EPOLLERR;
        ev.data.ptr = new connection(fd, message_bytes, max_outstanding);

        if (epoll_ctl(epfd, EPOLL_CTL_ADD, fd, &ev) < 0) {
            fprintf(stderr, "Failed to add fd to epoll\n");
            close(fd);
            close(epfd);
            return;
        }
        conn_fds_mtx.lock();
        conn_fds.push_back(fd);
        conn_fds_mtx.unlock();
    }

    printf("Connected to %s:%d successfully, total connections (%u) on Thread#%u\n", server_ip_str.c_str(), t_server_port, t_nr_flows, tid);
    
    mtx.lock();
    ready_threads++;
    mtx.unlock();
    
    while (ready_threads < nr_threads) {
        usleep(1000);
    }

    sleep(wait_seconds);

    while (1) {

        int nfds = epoll_wait(epfd, events, 128, -1);
        if (nfds) avg_nr_events.store((avg_nr_events.load() + nfds) / 2);
        for (int i = 0; i < nfds; i++) {
            c = (connection *)events[i].data.ptr;
            
            if (events[i].events & EPOLLERR || events[i].events & EPOLLHUP) {
                fprintf(stderr, "EPOLLERR\n");
                conn_fds.remove(c->fd);
                // remove from epoll
                epoll_ctl(epfd, EPOLL_CTL_DEL, c->fd, NULL);
                close(c->fd);
                continue;
            }
            
            int ret = connection_events(tid, c, events[i].events);
            if (ret == 0 && !c->no_epoll_out) {
                ev.events = EPOLLIN | EPOLLERR;
                ev.data.ptr = c;
                if (epoll_ctl(epfd, EPOLL_CTL_MOD, c->fd, &ev) < 0) {
                    fprintf(stderr, "Failed to add fd to epoll\n");
                    return;
                }
                c->no_epoll_out = 1;
            } else if (ret == 1 && c->no_epoll_out) {
                ev.events = EPOLLIN | EPOLLERR | EPOLLOUT;
                ev.data.ptr = c;
                if (epoll_ctl(epfd, EPOLL_CTL_MOD, c->fd, &ev) < 0) {
                    fprintf(stderr, "Failed to add fd to epoll\n");
                    return;
                }
                c->no_epoll_out = 0;
            }
        }
    }

    close(epfd);
    for (auto &fd : conn_fds) {
        close(fd);
    }
}

int parse_args(int argc, char *argv[])
{
    int opt;
    while ((opt = getopt(argc, argv, "t:q:f:b:i:p:so:mw:l:")) != -1) {
        switch (opt) {
            case 'b':
                message_bytes = std::stoi(optarg);
                break;
            case 'i':
                server_ip_str = optarg;
                break;
            case 'f':
                nr_flows = std::stoi(optarg);
                break;
            case 't':
                nr_threads = std::stoi(optarg);
                break;
            case 'l':
                max_buf_size = std::stoi(optarg);
                break;
            case 'q':
                nr_queues = std::stoi(optarg);
                break;
            case 'p':
                server_port = std::stoi(optarg);
                break;
            case 'w':
                wait_seconds = std::stoi(optarg);
                break;
            case 'd':
                dump_io_stats = true;
                break;
            case 's':
                short_response = false;
                break;
            case 'm':
                multiport = true;
                break;
            case 'o':
                max_outstanding = std::stoi(optarg);
                break;
            default:
                std::cout << "Usage: " << argv[0] << 
                    " [-t nr_threads, default:1]" <<
                    " [-l max_buf_size, default:4096]" << 
                    " [-q nr_queues, default:1]" <<
                    " [-b bytes, default:100]" << 
                    " [-i server_ip, default:192.168.6.2]" <<
                    " [-f nr_flows, default:1]"
                    " [-p server_port, default:50000]" << 
                    " [-w wait_seconds, default:0]" <<
                    " [-s enable short_response, default:true]" << 
                    " [-o max_outstanding, default:1]" <<
                    " [-m multiport, default:false]" << 
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
            unsigned int _out = 0;
            unsigned int _in = 0;

            for (unsigned int i = 0; i < nr_threads; i++) {
                _out += total_req_bytes[i].load() - prev_total_req_bytes[i];
                _in += total_resp_bytes[i].load() - prev_total_resp_bytes[i];
                prev_total_req_bytes[i] = total_req_bytes[i].load();
                prev_total_resp_bytes[i] = total_resp_bytes[i].load();
            }
            total_out += _out;
            total_in += _in;

            printf("Throughput In/Out(%.2f/%.2f Gbps)(%.2f Kops) conn#(%lu), avg_nr_events(%u), total_out(%luB), total_in(%luB)\n", 
                _out * 8.0 / 1e9, _in * 8.0 / 1e9, _out / message_bytes / 1e3,
                conn_fds.size(), avg_nr_events.load(), total_out, total_in);
        }
    }).detach();

    for (auto &t : threads) {
        t.join();
    }

    return 0;
}