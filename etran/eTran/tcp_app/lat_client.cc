#include <arpa/inet.h>

#include <iostream>
#include <thread>
#include <list>
#include <string>
#include <unordered_map>
#include <algorithm>
#include <vector>
#include <cassert>
#include <unistd.h>
#include <fcntl.h>
#include <sys/time.h>

/* time related */
static inline uint64_t rdtsc(void)
{
	uint32_t lo, hi;
	__asm__ __volatile__("rdtsc" : "=a"(lo), "=d"(hi));
	return (((uint64_t)hi << 32) | lo);
}
#define get_cycles rdtsc

static double get_cycles_per_sec(void)
{
	static double cps = 0;
	if (cps != 0)
	{
		return cps;
	}

	// Take parallel time readings using both rdtsc and gettimeofday.
	// After 10ms have elapsed, take the ratio between these readings.

	struct timeval start_time, stop_time;
	uint64_t start_cycles, stop_cycles, micros;
	double old_cps;

	// There is one tricky aspect, which is that we could get interrupted
	// between calling gettimeofday and reading the cycle counter, in which
	// case we won't have corresponding readings.  To handle this (unlikely)
	// case, compute the overall result repeatedly, and wait until we get
	// two successive calculations that are within 0.1% of each other.
	old_cps = 0;
	while (1)
	{
		if (gettimeofday(&start_time, NULL) != 0)
		{
			exit(1);
		}
		start_cycles = rdtsc();
		while (1)
		{
			if (gettimeofday(&stop_time, NULL) != 0)
			{
				exit(1);
			}
			stop_cycles = rdtsc();
			micros = (stop_time.tv_usec - start_time.tv_usec) +
					 (stop_time.tv_sec - start_time.tv_sec) * 1000000;
			if (micros > 10000)
			{
				cps = (double)(stop_cycles - start_cycles);
				cps = 1000000.0 * cps / (double)(micros);
				break;
			}
		}
		double delta = cps / 1000.0;
		if ((old_cps > (cps - delta)) && (old_cps < (cps + delta)))
		{
			return cps;
		}
		old_cps = cps;
	}
}

static inline double cycles_to_us(uint64_t cycles)
{
	return (double)cycles * 1e6 / get_cycles_per_sec();
}

#define NR_RT 500000
unsigned int nr_rt = 0;

bool pin_core = false;
bool dump_io_stats = false;
const unsigned int nr_threads = 1;
const unsigned int nr_queues = 1;
const unsigned int nr_flows = 1;
unsigned int data_bytes = 100;
std::string server_ip_str = "192.168.6.2";
uint16_t server_port = 50000;

std::vector<uint64_t> recv_tsc;
std::vector<uint64_t> send_tsc;
std::vector<uint64_t> rtt_tsc;
std::vector<std::thread> threads;

void thread_func(unsigned int tid)
{
    if (pin_core) {
        cpu_set_t cpuset;
        CPU_ZERO(&cpuset);
        CPU_SET(5+tid, &cpuset);
        if (pthread_setaffinity_np(pthread_self(), sizeof(cpu_set_t), &cpuset) < 0) {
            fprintf(stderr, "Failed to set thread affinity\n");
            return;
        }
    }
    printf("Start client thread (%u)\n", tid);
    
    /* open connections */
    struct in_addr server_ip_addr;
    
    assert(inet_pton(AF_INET, server_ip_str.c_str(), &server_ip_addr) == 1);

    int fd = socket(AF_INET, SOCK_STREAM, 0);
    if (fd < 0) {
        fprintf(stderr, "Failed to create socket\n");
        return;
    }

    struct sockaddr_in server_addr;
    server_addr.sin_family = AF_INET;
    server_addr.sin_port = htons(server_port);
    server_addr.sin_addr.s_addr = server_ip_addr.s_addr;

    if (connect(fd, (struct sockaddr *)&server_addr, sizeof(server_addr)) < 0) {
        fprintf(stderr, "Failed to connect to server\n");
        close(fd);
        return;
    }

    if (fcntl(fd, F_SETFL, fcntl(fd, F_GETFL, 0) | O_NONBLOCK) < 0) {
        fprintf(stderr, "Failed to set non-blocking\n");
        close(fd);
        return;
    }

    sleep(1);

    char recv_buf[data_bytes];
    char send_buf[data_bytes]; 
    // struct eTrantcp_event events[256] = {};
    // struct app_ctx_per_thread *tctx = eTran_get_tctx();
    // bool outstanding = false;
    // send_tsc.push_back(get_cycles());
    // size_t ret = 0;
    // if (write(fd, send_buf, data_bytes) < data_bytes) {
    //     goto out;
    // }
    // outstanding = true;
    
    while (1) {
        send_tsc.push_back(get_cycles());
        while (write(fd, send_buf, data_bytes) < data_bytes) {
            ;
        }
        while (read(fd, recv_buf, data_bytes) < data_bytes)
            ;
        recv_tsc.push_back(get_cycles());
        if (++nr_rt >= NR_RT) {
            goto out;
        }
        // int nr_events = eTran_tcp_poll_events(tctx, events, 256, 0);
        // for (int i = 0; i < nr_events; i++) {
        //     switch (events[i].type)
        //     {
        //     case ETRANTCP_EV_CONN_RECVED:
        //         ret = conn_recv(tctx, events[i].ev.recv.conn, recv_buf, data_bytes);
        //         if (ret <= 0) break;
        //         recv_tsc.push_back(get_cycles());
        //         if (++nr_rt >= NR_RT) {
        //             goto out;
        //         }
        //         send_tsc.push_back(get_cycles());
        //         ret = conn_send(tctx, events[i].ev.recv.conn, send_buf, data_bytes);
        //         if (ret == data_bytes) {
        //             outstanding = true;
        //         } else {
        //             send_tsc.pop_back();
        //         }
        //         break;
        //     case ETRANTCP_EV_CONN_SENDBUF:
        //         if (outstanding) break;
        //         send_tsc.push_back(get_cycles());
        //         ret = conn_send(tctx, events[i].ev.send.conn, send_buf, data_bytes);
        //         if (ret == data_bytes) {
        //             outstanding = true;
        //         } else {
        //             send_tsc.pop_back();
        //         }
        //         break;
        //     default:
        //         break;
        //     }
        // }
    }
out:

    /* dump rtts */
    for (auto it1 = recv_tsc.begin(), it2 = send_tsc.begin(); it1 != recv_tsc.end() && it2 != send_tsc.end(); it1++, it2++) {
        // ignore the first 1000 RTTs
        if (std::distance(recv_tsc.begin(), it1) < 1000)
            continue;
        rtt_tsc.push_back(*it1 - *it2);
    }
    // sort rtt_tsc
    std::sort(rtt_tsc.begin(), rtt_tsc.end());
    // print p50, p99, p99.9
    printf("rtt_tsc.size(): %lu\n", rtt_tsc.size());
    if (rtt_tsc.size() > 0) {
        std::cout << "min: " << cycles_to_us(rtt_tsc[0]) << " us" << std::endl;
        std::cout << "p50: " << cycles_to_us(rtt_tsc[rtt_tsc.size() / 2]) << " us" << std::endl;
        std::cout << "p99: " << cycles_to_us(rtt_tsc[rtt_tsc.size() * 99 / 100]) << " us" << std::endl;
        std::cout << "p99.9: " << cycles_to_us(rtt_tsc[rtt_tsc.size() * 999 / 1000]) << " us" << std::endl;
    }

    /* close connections */
    close(fd);
}

int parse_args(int argc, char *argv[])
{
    int opt;
    while ((opt = getopt(argc, argv, "t:q:f:b:i:p:dc")) != -1) {
        switch (opt) {
            case 'b':
                data_bytes = std::stoi(optarg);
                if (data_bytes > 1448)
                    data_bytes = 1448;
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
            case 'c':
                pin_core = true;
            default:
                std::cout << "Usage: " << argv[0] << 
                    " [-b bytes, default:100, <= 1448]" << 
                    " [-i server_ip, default:192.168.6.2]" <<
                    " [-p server_port, default:50000]" << 
                    " [-c pin_core]" << 
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

    threads.push_back(std::thread(thread_func, 0));

    std::thread([]() {
        while (1) {
            sleep(1);
            printf("nr_rt: %u\n", nr_rt);
        }
    }).detach();

    for (auto &t : threads) {
        if (t.joinable())
            t.join();
    }

    return 0;
}