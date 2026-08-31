#include <arpa/inet.h>

#include <iostream>
#include <thread>
#include <list>
#include <string>
#include <unordered_map>
#include <algorithm>
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

#define MAX_FLOWS 32

bool dump_io_stats = false;
const unsigned int nr_threads = 1;
const unsigned int nr_queues = 1;
unsigned int data_bytes = 100;
// FXIME: this is not used
std::string server_ip_str = "192.168.6.2";
uint16_t server_port = 50000;

void thread_func(unsigned int tid)
{
    struct in_addr server_ip_addr;
    int newfd;
    
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

    if (bind(fd, (struct sockaddr *)&server_addr, sizeof(server_addr)) < 0) {
        fprintf(stderr, "Failed to bind to server\n");
        close(fd);
        return;
    }

    if (listen(fd, 5)) {
        fprintf(stderr, "Failed to listen\n");
        close(fd);
        return;
    }
    printf("listen on %s:%d\n", server_ip_str.c_str(), server_port);

    newfd = accept(fd, NULL, NULL);

    if (newfd < 0) {
        fprintf(stderr, "Failed to accept\n");
        close(fd);
        return;
    }

    if (fcntl(newfd, F_SETFL, fcntl(newfd, F_GETFL, 0) | O_NONBLOCK) < 0) {
        fprintf(stderr, "Failed to set non-blocking\n");
        close(newfd);
        close(fd);
        return;
    }

    printf("accept new connection\n");

    char recv_buf[data_bytes];
    char send_buf[data_bytes];
    // struct eTrantcp_event events[256] = {};
    // struct app_ctx_per_thread *tctx = eTran_get_tctx();
    // bool pending = false;

    while (1) {
        while (read(newfd, recv_buf, data_bytes) < data_bytes)
            ;
        while (write(newfd, send_buf, data_bytes) < data_bytes)
            ;

        // int nr_events = eTran_tcp_poll_events(tctx, events, 256, 0);
        // for (int i = 0; i < nr_events; i++) {
        //     switch (events[i].type)
        //     {
        //     case ETRANTCP_EV_CONN_RECVED:
        //         ret = conn_recv(tctx, events[i].ev.recv.conn, recv_buf, data_bytes);
        //         if (ret < data_bytes) {
        //             printf("ret: %lu\n", ret);
        //             break;
        //         }
        //         if (conn_send(tctx, events[i].ev.recv.conn, send_buf, data_bytes) < data_bytes) {
        //             printf("Failed to send\n");
        //             pending = true;
        //         }
        //         break;
        //     case ETRANTCP_EV_CONN_SENDBUF:
        //         if (!pending) break;
        //         if (conn_send(tctx, events[i].ev.send.conn, send_buf, data_bytes) == data_bytes)
        //             pending = false;
        //         break;
        //     default:
        //         break;
        //     }
        // }
    }

    printf("close all connections\n");
    close(newfd);
    close(fd);
}

int parse_args(int argc, char *argv[])
{
    int opt;
    while ((opt = getopt(argc, argv, "t:q:f:b:i:p:")) != -1) {
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
            default:
                std::cout << "Usage: " << argv[0] << 
                    " [-b bytes, default:100, <= 1448]" << 
                    " [-i server_ip, default:192.168.6.2]" <<
                    " [-p server_port, default:50000]" << 
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

    std::thread thread;
    thread = std::thread(thread_func, 0);

    std::thread([]() {
        while (1) {
            sleep(1);
        }
    }).detach();

    if (thread.joinable())
        thread.join();

    return 0;
}