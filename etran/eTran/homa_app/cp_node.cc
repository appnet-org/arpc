#include <stdio.h>
#include <stdlib.h>
#include <signal.h>
#include <unistd.h>
#include <string.h>
#include <poll.h>

#include <linux/if_ether.h>
#include <linux/ip.h>
#include <arpa/inet.h>
#include <net/ethernet.h>
#include <net/if.h>

#include <sys/socket.h>
#include <sys/ioctl.h>
#include <sys/un.h>
#include <errno.h>

#include <vector>
#include <cmath>
#include <numeric>
#include <algorithm>

#include <execinfo.h>
#include <fcntl.h>
#include <netdb.h>
#include <netinet/tcp.h>
#include <stdarg.h>
#include <netinet/in.h>
#include <sys/epoll.h>
#include <sys/mman.h>
#include <sys/stat.h>
#include <sys/resource.h>
#include <sys/syscall.h>
#include <sys/time.h>
#include <sys/types.h>

#include <atomic>
#include <deque>
#include <functional>
#include <iostream>
#include <mutex>
#include <optional>
#include <random>
#include <thread>

#include <eTran_rpc.h>

#include "dist.h"
#include "homa.h"
#include "test_utils.h"
#include "time_trace.h"

#define IF_NAME "ens1f1np1"
static uint32_t local_ip;

using std::string;

/* Command-line parameter values (note: changes to default values must
 * also be reflected in client and server constructors): */
int nr_nic_queues = 1;
uint32_t client_max = 1;
uint32_t client_port_max = 1;
int client_ports = 0;
int first_port = 4000;
int client_first_port = 5000;
int first_server = 1;
bool is_server = false;
int id = -1;
double net_gbps = 0.0;
bool one_way = false;
int port_receivers = 1;
const char *protocol;
int server_nodes = 1;
int server_ports = 1;
bool verbose = false;
std::string workload_string;
const char *workload = "100";
int unloaded = 0;
int pin_core_start = -1;
bool register_done = true;
int both = 0;
int self[1024]; // self[i] == 1 indicates the server id is ourself

int inet_family = AF_INET;

// ip_to_string() - Convert an IP address to a string.
std::string ip_to_string(uint32_t ip)
{
    struct in_addr addr;
    addr.s_addr = htonl(ip);
    return std::string(inet_ntoa(addr));
}

/** @rand_gen: random number generator. */
std::mt19937 rand_gen(
		std::chrono::system_clock::now().time_since_epoch().count());

/**
 * struct conn_id - A 32-bit value that encodes a unique connection
 * between a TCP client and server.
 */
struct conn_id {
	/**
	 * @client_port: the index (starting at 0) of the port within
	 * the client (corresponds to a particular sending thread).
	 * This will be the low byte returned by int().
	 */
	uint8_t client_port;

	/** @client: the node index for the client (starts from zero). */
	uint8_t client;

	/**
	 * @server_port: the index (starting at 0) of a particular port
	 * within the server.
	 */
	uint8_t server_port;

	/** @server: the node index for the server (starts from 0). */
	uint8_t server;

	conn_id(uint8_t server, uint8_t server_port, uint8_t client,
			uint8_t client_port)
		: client_port(client_port), client(client),
		server_port(server_port), server(server)
	{}

	conn_id()
		: client_port(0), client(0), server_port(0), server(0)
	{}

	inline operator int()
	{
		return *(reinterpret_cast<int *>(this));
	}
};

/**
 * @server_addrs: Internet addresses for each of the server threads available
 * to receive a Homa RPC.
 */
std::vector<sockaddr_in_union> server_addrs;

/**
 * @server_ids: for each entry in @server_addrs, a connection identifier
 * with all fields filled in except client_port, which will be 0.
 */
std::vector<conn_id> server_ids;

/**
 * @freeze: one entry for each node index; 1 means messages to that
 * node should contain a flag telling the node to freeze its time trace.
 */
std::vector<int> freeze;

/**
 * @first_id: entry i contains the index in server_addrs of the first
 * entry for the server ports on node i. Used to map from node+port to
 * server id.
 */
std::vector<int> first_id;

/** @message_id: used to generate unique identifiers for outgoing messages.*/
std::atomic<uint32_t> message_id;

/**
 * @last_stats_time: time (in rdtsc cycles) when we last printed
 * staticsics. Zero means that none of the statistics below are valid.
 */
uint64_t last_stats_time = 0;

/**
 * @last_client_rpcs: total number of client RPCS completed by this
 * application as of the last time we printed statistics.
 */
uint64_t last_client_rpcs = 0;

/**
 * @last_client_bytes_out: total amount of data in request messages for
 * client RPCS completed by this application as of the last time we printed
 * statistics.
 */
uint64_t last_client_bytes_out = 0;

/**
 * @last_client_bytes_in: total amount of data in response messages for
 * client RPCS completed by this application as of the last time we printed
 * statistics.
 */
uint64_t last_client_bytes_in = 0;

/**
 * @last_total_elapsed: total amount of elapsed time for all client RPCs
 * issued by this application (in units of rdtsc cycles), as of the last
 * time we printed statistics.
 */
uint64_t last_total_rtt = 0;

/**
 * @last_lag: total lag across all clients (measured in rdtsc cycles)
 * as of the last time we printed statistics.
 */
uint64_t last_lag = 0;

/**
 * @last_backups: total # of backed-up sends as of the last time we
 * printed statistics.
 */
uint64_t last_backups = 0;

/**
 * @last_server_rpcs: total number of server RPCS handled by this
 * application as of the last time we printed statistics.
 */
uint64_t last_server_rpcs = 0;

/**
 * @last_server_bytes_in: total amount of data in incoming requests handled by
 * this application as of the last time we printed statistics.
 */
uint64_t last_server_bytes_in = 0;

/**
 * @last_server_bytes_out: total amount of data in responses sent by
 * this application as of the last time we printed statistics.
 */
uint64_t last_server_bytes_out = 0;

/**
 * @last_per_server_rpcs: server->requests for each individual server,
 * as of the last time we printed statistics.
 */
std::vector<uint64_t> last_per_server_rpcs;

/** @log_file: where log messages get printed. */
FILE* log_file = stdout;

enum Msg_Type {NORMAL, VERBOSE};

/** @log_level: only print log messages if they have a level <= this value. */
Msg_Type log_level = NORMAL;

extern void log(Msg_Type type, const char *format, ...)
	__attribute__((format(printf, 2, 3)));

/**
 * @cmd_lock: held whenever a command is executing.  Used to ensure that
 * operations such as statistics printing don't run when commands such
 * as "stop" are changing the client or server structure.
 */
std::mutex cmd_lock;

/**
 * @fd_locks: used to synchronize concurrent accesses to the same fd
 * (indexed by fd).
 */
#define MAX_FDS 10000
std::atomic_bool fd_locks[MAX_FDS];

/**
 * @kfreeze_count: number of times that kfreeze has been evoked since
 * the last time a client was created; used to eliminate redundant
 * freezes that waste time.
 */
int kfreeze_count = 0;

/**
 * @debug: values set with the "debug" command; typically used to
 * trigger various debugging behaviors.
 */
int64_t debug[5];

/**
 * struct message_header - The first few bytes of each message (request or
 * response) have the structure defined here. The client initially specifies
 * this information in the request, and the server returns the information
 * in the response.
 */
struct message_header {
	/**
	 * @length: total number of bytes in the message, including this
	 * header.
	 */
	int length:30;

	// **********NOTE**********
	// We use this filed to determine if the message is a request or response
	/** @freeze: true means the recipient should freeze its time trace. */
	unsigned int freeze:1;

	/**
	 * @short_response: true means responses should only be 100 bytes,
	 * regardless of the request length.
	 */
	unsigned int short_response:1;

	/**
	 * @cid: uniquely identifies the connection between a client
	 * and a server.
	 */
	conn_id cid;

	/**
	 * @msg_id: unique identifier for this message among all those
	 * from a given client machine.
	 */
	uint32_t msg_id;
};

/**
 * print_help() - Print out usage information for this program.
 * @name:   Name of the program (argv[0])
 */
void print_help(const char *name)
{
	printf("Usage: cp_node [command]\n\n"
		"If there are command-line options, they constitute a single command\n"
		"to execute, after which cp_node will print statistics every second.\n\n"
		"If there are no command-line options, then cp_node enters a loop reading\n"
		"lines from standard input and executing them as commands. The following\n"
		"commands are supported, each followed by a list of options supported\n"
		"by that command:\n\n"
		"client [options]      Start one or more client threads\n"
        "    --both            Serve as both client and server, client starts after a few seconds (default: 5)\n"
		"    --client-max      Maximum number of outstanding requests from a single\n"
		"                      client machine (divided equally among client ports)\n"
		"                      (default: %d)\n"
        "    --queues          Number of NIC queues to use (Only for eTran, default: %d)\n"
        "    --client-first-port Lowest port number to use for each client (default: %d)\n"
		"    --first-port      Lowest port number to use for each server (default: %d)\n"
		"    --pin             All client threads will be restricted to run only on givevn cores start from it\n"
		"    --first-server    Id of first server node (default: %d, meaning node%d)\n"
		"    --gbps            Target network utilization, including only message data,\n"
		"                      Gbps; 0 means send continuously (default: %.1f)\n"
		"    --id              Id of this node; a value of I >= 0 means requests will\n"
		"                      not be sent to nodeI (default: -1)\n"
		"    --one-way         Make all response messages 100 B, instead of the same\n"\
		"                      size as request messages\n"
		"    --ports           Number of ports on which to send requests (one\n"
		"                      sending thread per port (default: %d)\n"
		"    --server-nodes    Number of nodes running server threads (default: %d)\n"
		"    --server-ports    Number of server ports on each server node\n"
		"                      (default: %d)\n"
		"    --unloaded        Nonzero means run test in special mode for collecting\n"
		"                      baseline data, with the given number of measurements\n"
		"                      per length in the distribution (Homa only, default: 0)\n"
		"    --workload        Name of distribution for request lengths (e.g., 'w1')\n"
		"                      or integer for fixed length (default: %s)\n\n"                "debug value value ... Set one or more int64_t values that may be used for\n"
		"                      various debugging purposes\n\n"
		"dump_times file       Log RTT times (and lengths) to file\n\n"
		"exit                  Exit the application\n\n"
		"log [options] [msg]   Configure logging as determined by the options. If\n"
		"                      there is an \"option\" that doesn't start with \"--\",\n"
		"                      then it and all of the remaining words are printed to\n"
		"                      the log as a message.\n"
		"    --file            Name of log file to use for future messages (\"-\"\n"
		"                      means use standard output)\n"
		"    --level           Log level: either normal or verbose\n\n"
		"server [options]      Start serving requests on one or more ports\n"
		"    --pin             All client threads will be restricted to run only on givevn cores start from it\n"
		"    --first-port      Lowest port number to use (default: %d)\n"
		"    --ports           Number of ports to listen on (default: %d)\n\n"
		"stop [options]        Stop existing client and/or server threads; each\n"
		"                      option must be either 'clients' or 'servers'\n\n"
		" tt [options]         Manage time tracing:\n"
		"     freeze           Stop recording time trace information until\n"
		"                      print has been invoked\n"
		"     kfreeze          Freeze the kernel's internal timetrace\n"
		"     print file       Dump timetrace information to file\n",
		client_max, nr_nic_queues, client_first_port, first_port, first_server, first_server, net_gbps,
		client_ports, server_nodes, server_ports, workload,
		first_port, server_ports);
}

/**
 * log() - Print a message to the current log file
 * @type:   Kind of message (NORMAL or VERBOSE); used to control degree of
 *          log verbosity
 * @format: printf-style format string, followed by printf-style arguments.
 */
void log(Msg_Type type, const char *format, ...)
{
	char buffer[1000];
	struct timespec now;
	va_list args;

	if (type > log_level)
		return;
	va_start(args, format);
	clock_gettime(CLOCK_REALTIME, &now);

	vsnprintf(buffer, sizeof(buffer), format, args);
	fprintf(log_file, "%010lu.%09lu %s", now.tv_sec, now.tv_nsec, buffer);
}

inline void parse_type(const char *s, char **end, int *value)
{
	*value = strtol(s, end, 0);
}

inline void parse_type(const char *s, char **end, int64_t *value)
{
	*value = strtoll(s, end, 0);
}

inline void parse_type(const char *s, char **end, double *value)
{
	*value = strtod(s, end);
}

/**
 * parse() - Parse a value of a particular type from an argument word.
 * @words:     Words of a command being parsed.
 * @i:         Index within words of a word expected to contain an integer
 *             value (may be outside the range of words, in which case an
 *             error message is printed).
 * @value:     The parsed value corresponding to @words[i] is stored here,
 *             if the function completes successfully.
 * @format:    Name of option being parsed (for use in error messages).
 * @type_name: Human-readable name for ValueType (for use in error messages).
 * Return:     Nonzero means success, zero means an error occurred (and a
 *             message was printed).
 */
template<typename ValueType>
int parse(std::vector<string> &words, unsigned i, ValueType *value,
		const char *option, const char *type_name)
{
	ValueType num;
	char *end;

	if (i >= words.size()) {
		printf("No value provided for %s\n", option);
		return 0;
	}
	parse_type(words[i].c_str(), &end, &num);
	if (*end != 0) {
		printf("Bad value '%s' for %s; must be %s\n",
				words[i].c_str(), option, type_name);
		return 0;
	}
	*value = num;
	return 1;
}

/**
 * log_affinity() - Log a message listing the core affinity for the
 * current thread.
 */
void log_affinity()
{
	cpu_set_t cores;
	if (sched_getaffinity(0, sizeof(cores), &cores) != 0) {
		log(NORMAL, "ERROR: couldn't read core affinities: %s",
				strerror(errno));
		return;
	}
	int total = CPU_COUNT(&cores);
	std::string list = "";
	for (int i = 0; total > 0; i++) {
		if (!CPU_ISSET(i, &cores))
			continue;
		total--;
		if (!list.empty())
			list.append(" ");
		list.append(std::to_string(i));
	}
	log(NORMAL, "Core affinities: %s\n", list.c_str());
}

/**
 * init_server_addrs() - Set up the server_addrs table (addresses of the
 * server/port combinations that clients will communicate with), based on
 * current configuration parameters. Any previous contents of the table
 * are discarded. This also initializes related arrays @server_ids and
 * @freeze.
 */
void init_server_addrs(void)
{
	server_addrs.clear();
	server_ids.clear();
	freeze.clear();
	first_id.clear();
	for (int node = first_server; node < first_server + server_nodes;
			node++) {
		char host[100];
		struct addrinfo hints;
		struct addrinfo *matching_addresses;
		sockaddr_in_union *dest;

		if (node == id)
			continue;

		snprintf(host, sizeof(host), "node%d", node);
		memset(&hints, 0, sizeof(struct addrinfo));
		hints.ai_family = inet_family;
		hints.ai_socktype = SOCK_DGRAM;
		int status = getaddrinfo(host, NULL, &hints,
				&matching_addresses);
		if (status != 0) {
			log(NORMAL, "FATAL: couldn't look up address "
					"for %s: %s\n",
					host, gai_strerror(status));
			exit(1);
		}
		dest = reinterpret_cast<sockaddr_in_union *>
				(matching_addresses->ai_addr);
		while (((int) first_id.size()) < node)
			first_id.push_back(-1);
		first_id.push_back((int) server_addrs.size());

		for (int thread = 0; thread < server_ports; thread++) {
			dest->in4.sin_port = htons(first_port + thread);
			server_addrs.push_back(*dest);
			server_ids.emplace_back(node, thread, id, 0);
		}

		while (((int) freeze.size()) <= node)
			freeze.push_back(0);
		freeaddrinfo(matching_addresses);
	}
}

/**
 * class server_metrics - Keeps statistics for a single server thread
 * (i.e. all the requests arriving via one Homa port or one TCP listen
 * socket).
 */
class server_metrics {
public:
	/** @requests: Total number of requests handled so far. */
	uint64_t requests;

	/**
	 * @bytes_in: Total number of bytes of message data received by this
	 * server in requests.
	 */
	uint64_t bytes_in;

	/**
	 * @bytes_out: Total number of bytes of message data sent by this
	 * server in responses.
	 */
	uint64_t bytes_out;

    RpcSocket *rpcsocket = nullptr;

	server_metrics() :requests(0), bytes_in(0), bytes_out(0) {}
};

/**
 * @metrics: keeps track of metrics for all servers (whether Homa or TCP).
 * These are malloc-ed and must be freed eventually.
 */
std::vector<server_metrics *> metrics;

/**
 * class homa_server - Holds information about a single port used
 * to receive incoming requests, including one or more threads that
 * handle requests arriving via the port.
 */
class homa_server {
public:
	homa_server(int id, uint32_t local_ip, uint16_t local_port, int num_threads);
	~homa_server();
	void server(int thread_id, server_metrics *thread_metrics, uint32_t local_ip, uint16_t local_port);

	/** @id: Unique identifier for this server among all Homa servers. */
	int id;

    RpcSocket *rpcsocket = nullptr;

	/** @port: Homa port number managed by this object. */
	int port;

	/** @threads: One or more threads that service incoming requests*/
	std::vector<std::thread> threads;
};

/** @homa_servers: keeps track of all existing Homa servers. */
std::vector<homa_server *> homa_servers;

/**
 * homa_server::homa_server() - Constructor for homa_servers. Sets up the
 * Homa socket and starts up the threads to service the port.
 * @port:         Homa port number for this port.
 * @id:           Unique identifier for this port; used in thread identifiers
 *                for time traces.
 * @inet_family:  AF_INET or AF_INET6: determines whether we use IPv4 or IPv6.
 * @num_threads:  How many threads should collctively service requests on
 *                @port.
 */
homa_server::homa_server(int id, uint32_t local_ip, uint16_t local_port, int num_threads)
	: id(id)
        , threads()
{
    // TODO: allow multiple threads to service the same port
    num_threads = 1;

    for (int i = 0; i < num_threads; i++) {
		server_metrics *thread_metrics = new server_metrics;
		metrics.push_back(thread_metrics);
		threads.emplace_back([this, i, thread_metrics, local_ip, local_port] () {
			server(i, thread_metrics, local_ip, local_port);
		});
	}
}

/**
 * homa_server::~homa_server() - Destructor for homa_servers.
 */
homa_server::~homa_server()
{
	for (std::thread &thread: threads)
		thread.join();
}

/**
 * homa_server::server() - Handles incoming requests arriving on a Homa
 * socket. Normally invoked as top-level method in a thread.
 * @thread_id:   Unique identifier for this thread among all those for the port.
 * @metrics:     Used to record statistics for this thread.
 */
void homa_server::server(int thread_id, server_metrics *thread_metrics, uint32_t local_ip, uint16_t local_port)
{
	char thread_name[50];

	snprintf(thread_name, sizeof(thread_name), "S%d.%d", id, thread_id);
	time_trace::thread_buffer thread_buffer(thread_name);

    // create rpcsocket
    std::string local_ip_str = ip_to_string(local_ip);
    RpcSocket rpcsocket(thread_metrics, local_ip_str, local_port);
    thread_metrics->rpcsocket = &rpcsocket;
    log(NORMAL, "Server#%d binds to Homa port %d\n", thread_id, local_port);

    // set request handler for rpcsocket
    rpcsocket.set_req_handler([](ReqHandle *req_handle, void *context) {
        server_metrics *metrics = (server_metrics *)context;
        message_header *header;

        header = reinterpret_cast<message_header *>(req_handle->buffer._buf);
        uint32_t resp_len = header->short_response ? 100 : header->length;
        header->freeze = 1;

        tt("Received Homa request, cid 0x%08x, id %u, length %d",
            header->cid, header->msg_id, header->length);

        // allocate response buffer and submit response
        Buffer buffer = metrics->rpcsocket->alloc_buffer(resp_len);
        if (unlikely(buffer._buf == nullptr)) {
            log(NORMAL, "ERROR: couldn't allocate buffer for response\n");
            abort();
        }

        memcpy(buffer._buf, req_handle->buffer._buf, resp_len);

        metrics->rpcsocket->enqueue_response(req_handle, buffer);
        
        // collect metrics
        metrics->requests++;
        metrics->bytes_in += header->length;
        metrics->bytes_out += resp_len;
    });

	if (pin_core_start >= 0) {
		printf("Pinning server thread %s to core %d.\n", thread_name,
				10 + pin_core_start + id);
		pin_thread(10 + pin_core_start + id);
	} else {
		printf("Server thread %s started.\n", thread_name);
	}

    while (1) {
        rpcsocket.run_event_loop();
        // rpcsocket.run_event_loop_block(0);
    }
}

/**
 * class client - Holds information that is common to both Homa clients
 * and TCP clients.
 */
class client {
public:
	/**
	 * struct rinfo - Holds information about a request that we will
	 * want when we get the response.
	 */
	struct rinfo {
		/** @start_time: rdtsc time when the request was sent. */
		uint64_t start_time;

		/** @request_length: number of bytes in the request message. */
		int request_length;

		/**
		 * @active: true means the request has been sent but
		 * a response hasn't yet been received.
		 */
		bool active;

		rinfo() : start_time(0), request_length(0), active(false) {}
	};

	client(int id);
	virtual ~client();
	void check_completion(const char *protocol);
	int get_rinfo();
	void record(uint64_t end_time, message_header *header);
	virtual void stop_sender(void) {}

	/**
	 * @id: unique identifier for this client (index starting at
	 * 0 for the first client.
	 */
	int id;

	/**
	 * @rinfos: storage for more than enough rinfos to handle all of the
	 * outstanding requests.
	 */
	std::vector<rinfo> rinfos;

	/** @last_rinfo: index into rinfos of last slot that was allocated. */
	int last_rinfo;

	/**
	 * @receivers_running: number of receiving threads that have
	 * initialized and are ready to receive responses.
	 */
	std::atomic<size_t> receivers_running;

	/**
	 * @request_servers: a randomly chosen collection of indexes into
	 * server_addrs; used to select the server for each outgoing request.
	 */
	std::vector<int16_t> request_servers;

	/**
	 * @next_server: index into request_servers of the server to use for
	 * the next outgoing RPC.
	 */
	uint32_t next_server;

	/**
	 * @request_lengths: a randomly chosen collection of lengths to
	 * use for outgoing RPCs. Precomputed to save time during the
	 * actual measurements, and based on a given distribution.
	 * Note: lengths are always at least 4 (this is needed in order
	 * to include a 32-bit timestamp in the request).
	 */
	std::vector<int> request_lengths;

	/**
	 * @cnext_length: index into request_lengths of the length to use for
	 * the next outgoing RPC.
	 */
	uint32_t next_length;

	/**
	 * @request_intervals: a randomly chosen collection of inter-request
	 * intervals, measured in rdtsc cycles. Precomputed to save time
	 * during the actual measurements, and chosen to achieve a given
	 * network utilization, assuming a given distribution of request
	 * lengths.
	 */
	std::vector<int> request_intervals;

	/**
	 * @next_interval: index into request_intervals of the value to use
	 * for the next outgoing RPC.
	 */
	std::atomic<uint32_t> next_interval;

	/**
	 * @actual_lengths: a circular buffer that holds the actual payload
	 * sizes used for the most recent RPCs.
	 */
	std::vector<int> actual_lengths;

	/**
	 * @actual_rtts: a circular buffer that holds the actual round trip
	 * times (measured in rdtsc cycles) for the most recent RPCs. Entries
	 * in this array correspond to those in @actual_lengths.
	 */
	std::vector<uint64_t> actual_rtts;

	/**
	 * define NUM_CLENT_STATS: number of records in actual_lengths
	 * and actual_rtts.
	 */
#define NUM_CLIENT_STATS 500000

	/** @requests: total number of RPCs issued so far for each server. */
	std::vector<uint64_t> requests;

	/** @responses: total number of responses received so far from
	 * each server. Dynamically allocated (as of 3/2020, can't use
	 * vector with std::atomic).
	 */
	std::atomic<uint64_t> *responses;

	/** @num_servers: Number of entries in @responses. */
	size_t num_servers;

	/**
	 * @total_requests: total number of RPCs issued so far across all
	 * servers.
	 */
	uint64_t total_requests;

	/**
	 * @total_responses: total number of responses received so far from all
	 * servers.
	 */
	std::atomic<uint64_t> total_responses;

	/**
	 * @request_bytes: total amount of data sent in all requests for
	 * which responses have been received.
	 */
	std::atomic<uint64_t> request_bytes;

	/**
	 * @response_bytes: total amount of data in all response messages
	 * received so far.
	 */
	std::atomic<uint64_t> response_bytes;

	/**
	 * @total_rtt: sum of round-trip times (in rdtsc cycles) for
	 * all responses received so far.
	 */
	std::atomic<uint64_t> total_rtt;

	/**
	 * @lag: time in rdtsc cycles by which we are running behind
	 * because client_port_max was exceeded (i.e., the request
	 * we just sent should have been sent @lag cycles ago).
	 */
	uint64_t lag;
};
/** @clients: keeps track of all existing clients. */
std::vector<client *> clients;

/**
 * client::client() - Constructor for client objects.
 *
 * @id: Unique identifier for this client (index starting at 0?)
 */
client::client(int id)
	: id(id)
        , rinfos()
        , last_rinfo(0)
	, receivers_running(0)
	, request_servers()
	, next_server(0)
	, request_lengths()
	, next_length(0)
	, request_intervals()
	, next_interval(0)
	, actual_lengths(NUM_CLIENT_STATS, 0)
	, actual_rtts(NUM_CLIENT_STATS, 0)
	, requests()
	, responses()
        , num_servers(server_addrs.size())
	, total_requests(0)
	, total_responses(0)
	, request_bytes(0)
	, response_bytes(0)
        , total_rtt(0)
        , lag(0)
{
	rinfos.resize(2*client_port_max + 5);

	/* Precompute information about the requests this client will
	 * generate. Pick a different prime number for the size of each
	 * vector, so that they will wrap at different times, giving
	 * different combinations of values over time.
	 */
#define NUM_SERVERS 4729
#define NUM_LENGTHS 7207
#define NUM_INTERVALS 8783
	std::uniform_int_distribution<int> server_dist(0,
			static_cast<int>(num_servers - 1));
	for (int i = 0; i < NUM_SERVERS; i++) {
		int server = server_dist(rand_gen);
		// generate a server that is not ourself
		while (self[server]) {
			server = server_dist(rand_gen);
		}
		request_servers.push_back(server);
	}
	std::vector<dist_point> points = dist_get(workload,
			HOMA_MAX_MESSAGE_LENGTH);
	if (points.empty()) {
		printf("FATAL: invalid workload '%s'\n", workload);
		exit(1);
	}
	dist_sample(points, &rand_gen, NUM_LENGTHS, request_lengths);
	if (net_gbps == 0.0)
		request_intervals.push_back(0);
	else {
		double lambda = 1e09*(net_gbps/8.0)
				/(dist_mean(points)*client_ports);
		double cycles_per_second = get_cycles_per_sec();
		std::exponential_distribution<double> interval_dist(lambda);
		for (int i = 0; i < NUM_INTERVALS; i++) {
			double seconds = interval_dist(rand_gen);
			int cycles = int(seconds*cycles_per_second);
			request_intervals.push_back(cycles);
		}
	}
	requests.resize(server_addrs.size());
	responses = new std::atomic<uint64_t>[num_servers];
	for (size_t i = 0; i < num_servers; i++)
		responses[i] = 0;
	double avg_length = 0;
	for (size_t i = 0; i < request_lengths.size(); i++)
		avg_length += request_lengths[i];
	avg_length /= NUM_LENGTHS;
	uint64_t interval_sum = 0;
	for (size_t i = 0; i < request_intervals.size(); i++)
		interval_sum += request_intervals[i];
	double rate = ((double) NUM_INTERVALS)/to_seconds(interval_sum);
	log(NORMAL, "Average message length %.1f KB (expected %.1fKB), "
			"rate %.2f K/sec, expected BW %.1f Gbps\n",
			avg_length*1e-3, dist_mean(points)*1e-3, rate*1e-3,
			avg_length*rate*8e-9);
	kfreeze_count = 0;
}

/**
 * Destructor for clients.
 */
client::~client()
{
	delete[] responses;
}

/**
 * check_completion() - Make sure that all outstanding requests have
 * completed; if not, generate a log message.
 * @protocol:  String that identifies the current protocol for the log
 *             message, if any.
 */
void client::check_completion(const char *protocol)
{
	string server_info;
	int incomplete = total_requests - total_responses;
	for (size_t i = 0; i < requests.size(); i++) {
		char buffer[100];
		int diff = requests[i] - responses[i];
		if (diff == 0)
			continue;
		if (!server_info.empty())
			server_info.append(", ");
		snprintf(buffer, sizeof(buffer), "s%lu: %d", i, diff);
		server_info.append(buffer);
	}
	if ((incomplete != 0) || !server_info.empty())
		log(NORMAL, "ERROR: %d incomplete %s requests (%s)\n",
				incomplete, protocol, server_info.c_str());
}

/**
 * get_rinfo() - Find an available rinfo slot and return its index in
 * rinfos.
 */
int client::get_rinfo()
{
	int next = last_rinfo;

	while (true) {
		next++;
		if (next >= static_cast<int>(rinfos.size()))
			next = 0;
		if (!rinfos[next].active) {
			rinfos[next].active = true;
			last_rinfo = next;
			return next;
		}
		if (next == last_rinfo) {
			log(NORMAL, "FATAL: ran out of rinfos (%lu in use, "
					"total_requests %ld, "
					"total_responses %ld, last_rinfo %d)\n",
					rinfos.size(), total_requests,
				        total_responses.load(), last_rinfo);
			exit(1);
		}
	}
}

void kfreeze()
{

}

/**
 * record() - Records statistics about a particular request.
 * @end_time:   Completion time for the request, in rdtsc cycles.
 * @header:     The header from the response.
 */
void client::record(uint64_t end_time, message_header *header)
{
	int server_id;
	int slot = total_responses.fetch_add(1) % NUM_CLIENT_STATS;
	int64_t rtt;

	if (header->msg_id >= rinfos.size()) {
		log(NORMAL, "ERROR: msg_id (%u) exceed rinfos.size (%lu)\n",
			header->msg_id, rinfos.size());
		return;
	}
	rinfo *r = &rinfos[header->msg_id];
	if (!r->active) {
		log(NORMAL, "ERROR: response arrived for inactive msg_id %u\n",
			header->msg_id);
		return;
	}
	rtt = end_time - r->start_time;
	r->active = false;

	int kcycles = rtt>>10;
	tt("Received response, cid 0x%08x, id %u, length %d, "
			"rtt %d kcycles",
			header->cid, header->msg_id,
			header->length, kcycles);
	if ((kcycles > debug[0]) && (kcycles < debug[1])
			&& (header->length < 1500) && !time_trace::frozen) {
		freeze[header->cid.server] = 1;
		tt("Freezing timetrace because of long RTT for "
				"cid 0x%08x, id %u, length %d, kcycles %d",
				header->cid, header->msg_id, header->length,
				kcycles);
		log(NORMAL, "Freezing timetrace because of long RTT for "
				"cid 0x%08x, id %u",
				int(header->cid), header->msg_id);
		time_trace::freeze();
		kfreeze();
	}

	server_id = first_id[header->cid.server];
	if (server_id == -1) {
		log(NORMAL, "WARNING: response received from unknown "
				"cid 0x%08x\n", (int) header->cid);
		return;
	}
	server_id += header->cid.server_port;
	responses[server_id].fetch_add(1);
	request_bytes += r->request_length;
	response_bytes += header->length;
	total_rtt += rtt;
	actual_lengths[slot] = header->length;
	actual_rtts[slot] = rtt;
}

/**
 * class homa_client - Holds information about a single Homa client,
 * which consists of one thread issuing requests and one or more threads
 * receiving responses.
 */
class homa_client : public client {
public:
	homa_client(int id, uint32_t local_ip, uint16_t local_port);
	virtual ~homa_client();
	void measure_unloaded(int count);
	uint64_t measure_rtt(int server, int length, char *buffer);

	void sender(uint32_t local_ip, uint16_t local_port);
    virtual void stop_sender(void);

    RpcSocket *rpcsocket = nullptr;

    bool warmup_done = false;

    uint64_t warmup_cycles;

	/** @exit_sender: true means the working thread should exit ASAP. */
	bool exit_sender;

	/** @sender_exited:  just what you'd guess from the name. */
	bool sender_exited;

    /**
	 * @sender_buffer: used by the sender to send requests, and also
	 * by measure_unloaded; malloced, size HOMA_MAX_MESSAGE_LENGTH.
	 */
	char *sender_buffer;

	/**
	 * @sender_thread: thread that sends requests (may also receive
	 * responses if port_receivers is 0).
	 */
	std::optional<std::thread> sender_thread;
};

/**
 * homa_client::homa_client() - Constructor for homa_client objects.
 *
 * @id: Unique identifier for this client (index starting at 0?)
 */
homa_client::homa_client(int id, uint32_t local_ip, uint16_t local_port)
	: client(id)
        , exit_sender(false)
        , sender_exited(false)
        , sender_buffer(new char[HOMA_MAX_MESSAGE_LENGTH])
        , sender_thread()
{
    log(NORMAL, "Client#%d binds to Homa port %d\n", id, local_port);

	if (unloaded) {

        std::string local_ip_str = ip_to_string(local_ip);
        RpcSocket rpcsocket(this, local_ip_str, local_port);
        this->rpcsocket = &rpcsocket;

		measure_unloaded(unloaded);
		sender_exited = true;
	} else {
		sender_thread.emplace(&homa_client::sender, this, local_ip, local_port);
	}
}

/**
 * homa_client::~homa_client() - Destructor for homa_client objects;
 * will terminate threads created for this client.
 */
homa_client::~homa_client()
{
	uint64_t start = rdtsc();
	exit_sender = true;
	while (!sender_exited || (total_responses != total_requests)) {
		if (to_seconds(rdtsc() - start) > 2.0)
			break;
	}

	if (sender_thread)
		sender_thread->join();

	check_completion("homa");

    delete rpcsocket;
}

/**
 * homa_client::stop_sender() - Ask the sending thread to stop sending,
 * and wait until it exits (but give up if that takes too long).
 */
void homa_client::stop_sender(void)
{
	uint64_t start = rdtsc();
	exit_sender = true;
	while (1) {
		if (sender_exited) {
			if (sender_thread) {
				sender_thread->join();
				sender_thread.reset();
			}
		}
		if (to_seconds(rdtsc() - start) > 0.5)
			break;
	}
}

// cont_handler
void cont_handler(ContHandle *cont_handle, void *context)
{
    homa_client *homa_client = (class homa_client *)context;
    uint64_t end_time = rdtsc();
    message_header *header = reinterpret_cast<message_header *>(cont_handle->buffer._buf);
    tt("Received response, cid 0x%08x, id %x, %d bytes",
            header->cid, header->msg_id, header->length);
    homa_client->record(end_time, header);
}

void homa_client::sender(uint32_t local_ip, uint16_t local_port)
{
    std::string local_ip_str = ip_to_string(local_ip);
    RpcSocket rpcsocket(this, local_ip_str, local_port);
    this->rpcsocket = &rpcsocket;

    message_header *header = reinterpret_cast<message_header *>(sender_buffer);

	uint64_t next_start = rdtsc();
    char thread_name[50];
	snprintf(thread_name, sizeof(thread_name), "W%d", id);

	if (pin_core_start >= 0) {
		printf("Pinning client thread %s to core %d.\n", thread_name,
				pin_core_start + id);
		pin_thread(pin_core_start + id);
	} else {
		printf("Client thread %s started.\n", thread_name);
	}

	time_trace::thread_buffer thread_buffer(thread_name);

    struct sockaddr_in dest_addr;

    if (both) {
        // set request handler for rpcsocket
        rpcsocket.set_req_handler([](ReqHandle *req_handle, void *context) {
            homa_client *homa_client = (class homa_client *)context;
            message_header *header;

            header = reinterpret_cast<message_header *>(req_handle->buffer._buf);
            uint32_t resp_len = header->short_response ? 100 : header->length;
            header->freeze = 1;

            // allocate response buffer and submit response
            Buffer buffer = homa_client->rpcsocket->alloc_buffer(resp_len);
            if (unlikely(buffer._buf == nullptr)) {
                log(NORMAL, "ERROR: couldn't allocate buffer for response\n");
                abort();
            }
            memcpy(buffer._buf, req_handle->buffer._buf, resp_len);
            homa_client->rpcsocket->enqueue_response(req_handle, buffer);
        });
    }

    if (both) {
        // wait for all nodes to be ready after a few seconds
        uint64_t now = rdtsc();
        uint64_t target = now + get_cycles_per_sec()*both;
        while (now < target) {
            rpcsocket.run_event_loop();
            now = rdtsc();
        }
    }

	int first_run = 1;
    while (1) {
		uint64_t now;
		int server;
		int slot = get_rinfo();

		/* Wait until (a) we have reached the next start time
		 * and (b) there aren't too many requests outstanding.
		 */
		while (1) {
			if (exit_sender) {
				sender_exited = true;
				rinfos[slot].active = false;
				return;
			}
			if (likely(!first_run))
            	rpcsocket.run_event_loop();
			else
				first_run = 0;

			now = rdtsc();
			if (now < next_start)
				continue;
			if ((total_requests - total_responses) < client_port_max)
				break;
		}

		rinfos[slot].start_time = now;
		server = request_servers[next_server];
		next_server++;
		if (next_server >= request_servers.size())
			next_server = 0;

        int msg_len = request_lengths[next_length];
        if (msg_len > HOMA_MAX_MESSAGE_LENGTH)
			msg_len = HOMA_MAX_MESSAGE_LENGTH;
		if (msg_len < sizeof32(*header))
			msg_len = sizeof32(*header);

        Buffer buffer = rpcsocket.alloc_buffer(msg_len);
        if (unlikely(buffer._buf == nullptr)) {
            log(NORMAL, "ERROR: couldn't allocate buffer for response\n");
            abort();
        }

        header = reinterpret_cast<message_header *>(buffer._buf);
		header->length = msg_len;
		rinfos[slot].request_length = header->length;
		header->cid = server_ids[server];
		header->cid.client_port = id;
        // printf("header->cid = 0x%08x\n", (int)header->cid);
		// header->freeze = freeze[header->cid.server];
		header->freeze = 0;
		header->short_response = one_way;
		header->msg_id = slot;
        
        dest_addr.sin_family = AF_INET;
        dest_addr.sin_port = server_addrs[server].in4.sin_port;
        memcpy(&dest_addr.sin_addr, &server_addrs[server].in4.sin_addr, sizeof(dest_addr.sin_addr));

		tt("sending request, cid 0x%08x, id %u, length %d",
				header->cid, header->msg_id, header->length);
        rpcsocket.enqueue_request(buffer, &dest_addr, cont_handler);

        rpcsocket.run_event_loop();

		requests[server]++;
		total_requests++;
		next_length++;
		if (next_length >= request_lengths.size())
			next_length = 0;
		lag = now - next_start;
		next_start = next_start + request_intervals[next_interval];
		next_interval++;
		if (next_interval >= request_intervals.size())
			next_interval = 0;
	}
}

void warmup_handler(ContHandle *cont_handle, void *context)
{
    homa_client *homa_client = (class homa_client *)context;
    homa_client->warmup_done = true;
    homa_client->warmup_cycles = get_cycles();
}

/**
 * homa_client::measure_rtt() - Make a single request to a given server and
 * return the RTT.
 * @server:      Identifier of server to use for the request.
 * @length:      Number of message bytes in the request.
 * @buffer:      Block of memory to use for request; must
 *               contain HOMA_MAX_MESSAGE_LENGTH bytes.
 * @receiver:    Use this to receive responses.
 *
 * Return:       Round-trip time to service the request, in rdtsc cycles.
 */
uint64_t homa_client::measure_rtt(int server, int length, char *buffer)
{
	message_header *header = reinterpret_cast<message_header *>(buffer);
	uint64_t start;

	if (length > HOMA_MAX_MESSAGE_LENGTH)
		length = HOMA_MAX_MESSAGE_LENGTH;
	if (length < sizeof32(*header))
		length = sizeof32(*header);
    
    Buffer b = rpcsocket->alloc_buffer(length);
    if (unlikely(b._buf == nullptr)) {
        log(NORMAL, "ERROR: couldn't allocate buffer for response\n");
        abort();
    }

    header = reinterpret_cast<message_header *>(b._buf);
	
    header->length = length;
	header->cid = server_ids[server];
	header->cid.client_port = id;

	start = rdtsc();

    // send warm request
    struct sockaddr_in dest_addr = {
        .sin_family = AF_INET,
        .sin_port = server_addrs[server].in4.sin_port, 
        .sin_addr = server_addrs[server].in4.sin_addr.s_addr,
    };

    rpcsocket->enqueue_request(b, &dest_addr, warmup_handler);

    while (!warmup_done) {
        rpcsocket->run_event_loop();
    }

    warmup_done = false;

	return warmup_cycles - start;
}

/**
 * homa_client::measure_unloaded() - Gather baseline measurements of Homa
 * under best-case conditions. This method will fill in the actual_lengths
 * and actual_rtts arrays with several measurements for each message length
 * in the current workload.
 * @count:    How many samples to measure for each length in the distribution.
 */
void homa_client::measure_unloaded(int count)
{
	std::vector<dist_point> dist = dist_get(workload,
			HOMA_MAX_MESSAGE_LENGTH);
	int server = request_servers[0];
	int slot;
	uint64_t ms100 = get_cycles_per_sec()/10;
	uint64_t end;
	
	printf("Warmup\n");
	/* Make one request for each size in the distribution, just to warm
	 * up the system.
	 */
	for (dist_point &point: dist)
		measure_rtt(server, point.length, sender_buffer);
	
	printf("Real measurements\n");
	/* Now do the real measurements. Stop with each size after 10
	 * measurements if more than 0.1 second has elapsed (otherwise
	 * this takes too long).
	 */
	slot = 0;
	for (dist_point &point: dist) {
		end = rdtsc() + ms100;
		for (int i = 0; i < count; i++) {
			if ((rdtsc() >= end) && (i >= 10))
				break;
			actual_lengths[slot] = point.length;
			actual_rtts[slot] = measure_rtt(server, point.length,
					sender_buffer);
			slot++;
			if (slot >= NUM_CLIENT_STATS) {
				log(NORMAL, "WARNING: not enough space to "
						"record all unloaded RTTs\n");
				slot = 0;
			}
		}
	}
	printf("Real measurements done\n");
}

/**
 * server_stats() -  Prints recent statistics collected from all
 * servers.
 * @now:   Current time in rdtsc cycles (used to compute rates for
 *         statistics).
 */
void server_stats(uint64_t now)
{
	char details[10000];
	int offset = 0;
	int length;
	uint64_t server_rpcs = 0;
	uint64_t server_bytes_in = 0;
	uint64_t server_bytes_out = 0;
	details[0] = 0;
	for (uint32_t i = 0; i < metrics.size(); i++) {
		server_metrics *server = metrics[i];
		server_rpcs += server->requests;
		server_bytes_in += server->bytes_in;
		server_bytes_out += server->bytes_out;
		length = snprintf(details + offset, sizeof(details) - offset,
				"%s%lu", (offset != 0) ? " " : "",
				server->requests - last_per_server_rpcs[i]);
		offset += length;
		if (i > last_per_server_rpcs.size())
			printf("last_per_server_rpcs has %lu entries, needs %lu\n",
					last_per_server_rpcs.size(),
					metrics.size());
		last_per_server_rpcs[i] = server->requests;
	}
	if ((last_stats_time != 0) && (server_bytes_in != last_server_bytes_in)) {
		double elapsed = to_seconds(now - last_stats_time);
		double rpcs = (double) (server_rpcs - last_server_rpcs);
		double in_delta = (double) (server_bytes_in
				- last_server_bytes_in);
		double out_delta = (double) (server_bytes_out
				- last_server_bytes_out);
		log(NORMAL, "Servers: %.2f Kops/sec, %.2f Gbps in, "
				"%.2f Gbps out, avg. req. length %.1f bytes\n",
				rpcs/(1000.0*elapsed),
				8.0*in_delta/(1e09*elapsed),
				8.0*out_delta/(1e09*elapsed),
				in_delta/rpcs);
		log(NORMAL, "RPCs per server: %s\n", details);
	}
	last_server_rpcs = server_rpcs;
	last_server_bytes_in = server_bytes_in;
	last_server_bytes_out = server_bytes_out;
}

/**
 * client_stats() -  Prints recent statistics collected by all existing
 * clients (either TCP or Homa).
 * @now:       Current time in rdtsc cycles (used to compute rates for
 *             statistics).
 */
void client_stats(uint64_t now)
{
#define CDF_VALUES 100000
	uint64_t client_rpcs = 0;
	uint64_t request_bytes = 0;
	uint64_t response_bytes = 0;
	uint64_t total_rtt = 0;
	uint64_t lag = 0;
	uint64_t outstanding_rpcs = 0;
	uint64_t cdf_times[CDF_VALUES];
	uint64_t backups = 0;
	int times_per_client;
	int cdf_index = 0;

	if (clients.size() == 0)
		return;

	times_per_client = CDF_VALUES/clients.size();
	if (times_per_client > NUM_CLIENT_STATS)
		times_per_client = NUM_CLIENT_STATS;
	for (client *client: clients) {
		for (size_t i = 0; i < client->num_servers; i++)
			client_rpcs += client->responses[i];
		request_bytes += client->request_bytes;
		response_bytes += client->response_bytes;
		total_rtt += client->total_rtt;
		lag += client->lag;
		outstanding_rpcs += client->total_requests
			- client->total_responses;
		for (int i = 1; i <= times_per_client; i++) {
			/* Collect the most recent RTTs from the client for
			 * computing a CDF.
			 */
			int src = (client->total_responses - i)
					% NUM_CLIENT_STATS;
			if (client->actual_rtts[src] == 0) {
				/* Client hasn't accumulated times_per_client
				 * entries yet; just use what it has. */
				break;
			}
			cdf_times[cdf_index] = client->actual_rtts[src];
			cdf_index++;
		}
	}
	std::sort(cdf_times, cdf_times + cdf_index);
	if ((last_stats_time != 0) && (request_bytes != last_client_bytes_out)) {
		double elapsed = to_seconds(now - last_stats_time);
		double rpcs = (double) (client_rpcs - last_client_rpcs);
		double delta_out = (double) (request_bytes
				- last_client_bytes_out);
		double delta_in = (double) (response_bytes
				- last_client_bytes_in);
		log(NORMAL, "Clients: %.2f Kops/sec, %.2f Gbps out, "
				"%.2f Gbps in, RTT (us) P50 %.2f P99 %.2f "
				"P99.9 %.2f, avg. req. length %.1f bytes\n",
				rpcs/(1000.0*elapsed),
				8.0*delta_out/(1e09*elapsed),
				8.0*delta_in/(1e09*elapsed),
				to_seconds(cdf_times[cdf_index/2])*1e06,
				to_seconds(cdf_times[99*cdf_index/100])*1e06,
				to_seconds(cdf_times[999*cdf_index/1000])*1e06,
			        delta_out/rpcs);
		double lag_fraction;
		if (lag > last_lag)
			lag_fraction = (to_seconds(lag - last_lag)/elapsed)
				/ clients.size();
		else
			lag_fraction = -(to_seconds(last_lag - lag)/elapsed)
				/ clients.size();
		if (lag_fraction >= .01)
			log(NORMAL, "Lag due to overload: %.1f%%\n",
					lag_fraction*100.0);
		if (backups != 0) {
			log(NORMAL, "Backed-up sends: %lu/%lu (%.1f%%)\n",
					backups - last_backups,
					client_rpcs - last_client_rpcs,
					100.0*(backups - last_backups)
					/(client_rpcs - last_client_rpcs));
		}
	}
	if (outstanding_rpcs != 0)
		log(NORMAL, "Outstanding client RPCs: %lu\n", outstanding_rpcs);
	last_client_rpcs = client_rpcs;
	last_client_bytes_out = request_bytes;
	last_client_bytes_in = response_bytes;
	last_total_rtt = total_rtt;
	last_lag = lag;
	last_backups = backups;
}

/**
 * log_stats() - Enter an infinite loop printing statistics to the
 * log every second. This function never returns.
 */
void log_stats()
{
	while (1) {
		sleep(1);
		std::lock_guard<std::mutex> lock(cmd_lock);
		uint64_t now = rdtsc();
		server_stats(now);
		client_stats(now);

		last_stats_time = now;
	}
}

static int get_local_ip(void)
{
    int fd;
    struct ifreq ifr;
    fd = socket(AF_INET, SOCK_DGRAM, 0);
    ifr.ifr_addr.sa_family = AF_INET;
    strncpy(ifr.ifr_name, IF_NAME, IFNAMSIZ-1);
    ioctl(fd, SIOCGIFADDR, &ifr);
    close(fd);
    local_ip = ntohl(((struct sockaddr_in *)&ifr.ifr_addr)->sin_addr.s_addr);
    printf("Local IP: %hhu.%hhu.%hhu.%hhu\n", (local_ip >> 24) & 0xFF, (local_ip >> 16) & 0xFF, (local_ip >> 8) & 0xFF, local_ip & 0xFF);
    return 0;
}

/**
 * client_cmd() - Parse the arguments for a "client" command and execute it.
 * @words:  Command arguments (including the command name as @words[0]).
 *
 * Return:  Nonzero means success, zero means there was an error.
 */
int client_cmd(std::vector<string> &words)
{
	client_max = 1;
	client_ports = 1;
	first_port = 4000;
	first_server = 1;
	inet_family = AF_INET;
	net_gbps = 0.0;
	port_receivers = 1;
	protocol = "homa";
	server_nodes = 1;
	one_way = false;
	unloaded = 0;
	workload = "100";
	for (unsigned i = 1; i < words.size(); i++) {
		const char *option = words[i].c_str();

		if (strcmp(option, "--client-max") == 0) {
			if (!parse(words, i+1, (int *) &client_max,
					option, "integer"))
				return 0;
			i++;
		} else if (strcmp(option, "--first-port") == 0) {
			if (!parse(words, i+1, &first_port, option, "integer"))
				return 0;
			i++;
        } else if (strcmp(option, "--client-first-port") == 0) {
			if (!parse(words, i+1, &client_first_port, option, "integer"))
				return 0;
			i++;
		} else if (strcmp(option, "--pin") == 0) {
			if (!parse(words, i+1, &pin_core_start, option, "integer"))
				return 0;
			i++;
        } else if (strcmp(option, "--queues") == 0) {
			if (!parse(words, i+1, &nr_nic_queues, option, "integer"))
				return 0;
			i++;
		} else if (strcmp(option, "--first-server") == 0) {
			if (!parse(words, i+1, &first_server, option, "integer"))
				return 0;
			i++;
		} else if (strcmp(option, "--gbps") == 0) {
			if (!parse(words, i+1, &net_gbps, option, "float"))
				return 0;
			i++;
		} else if (strcmp(option, "--id") == 0) {
			if (!parse(words, i+1, &id, option, "integer"))
				return 0;
			i++;
		} else if (strcmp(option, "--one-way") == 0) {
			one_way = true;
        } else if (strcmp(option, "--both") == 0) {
            if (!parse(words, i+1, &both, option, "integer"))
				return 0;
			i++;
		} else if (strcmp(option, "--ports") == 0) {
			if (!parse(words, i+1, &client_ports, option, "integer"))
				return 0;
			i++;
		} else if (strcmp(option, "--server-nodes") == 0) {
			if (!parse(words, i+1, &server_nodes, option, "integer"))
				return 0;
			i++;
		} else if (strcmp(option, "--server-ports") == 0) {
			if (!parse(words, i+1, &server_ports, option, "integer"))
				return 0;
			i++;
		} else if (strcmp(option, "--unloaded") == 0) {
			if (!parse(words, i+1, &unloaded, option, "integer"))
				return 0;
			i++;
		} else if (strcmp(option, "--workload") == 0) {
			if ((i + 1) >= words.size()) {
				printf("No value provided for %s\n",
						option);
				return 0;
			}
			workload_string = words[i+1];
			workload = workload_string.c_str();
			i++;
		} else {
			printf("Unknown option '%s'\n", option);
			return 0;
		}
	}
	init_server_addrs();
	client_port_max = client_max/client_ports;
	if (client_port_max < 1)
		client_port_max = 1;

	get_local_ip();

    struct eTran_cfg cfg = {0};
    if (both)
        cfg.nr_app_threads = client_ports + server_ports;
    else
        cfg.nr_app_threads = client_ports;
    cfg.nr_nic_queues = nr_nic_queues;
    cfg.proto = IPPROTO_HOMA;
    if (eTran_init(&cfg)) {
        printf("Failed to init eTran\n");
        exit(EXIT_FAILURE);
    }

	register_done = true;

    if (both) {
        /* Create servers. */
        for (homa_server *server: homa_servers)
				delete server;
        homa_servers.clear();
        last_per_server_rpcs.clear();
        for (server_metrics *m: metrics)
            delete m;
        metrics.clear();

        for (int i = 0; i < server_ports; i++) {
            homa_server *server = new homa_server(i, local_ip, (uint16_t)(first_port + i), 1);
            homa_servers.push_back(server);
        }

        last_per_server_rpcs.resize(server_ports, 0);
        last_stats_time = 0;
    }

	/* Create clients. */
	for (int i = 0; i < client_ports; i++) {
        clients.push_back(new homa_client(i, local_ip, (uint16_t)(client_first_port + i)));
	}
	
    last_stats_time = 0;
	time_trace::cleanup();
	return 1;
}

/**
 * debug_cmd() - Parse the arguments for a "debug" command and execute it.
 * @words:  Command arguments (including the command name as @words[0]).
 *
 * Return:  Nonzero means success, zero means there was an error.
 */
int debug_cmd(std::vector<string> &words)
{
	int64_t value;
	size_t num_debug = sizeof(debug)/sizeof(*debug);

	if (words.size() > (num_debug + 1)) {
		printf("Too many debug values; at most %lu allowed\n",
			num_debug);
	}
	for (size_t i = 1; i < words.size(); i++) {
		if (!parse(words, i, &value, "debug", "64-bit integer"))
			return 0;
		debug[i-1] = value;
	}
	return 1;
}

/**
 * dump_times_cmd() - Parse the arguments for a "dump_times" command and
 * execute it.
 * @words:  Command arguments (including the command name as @words[0]).
 *
 * Return:  Nonzero means success, zero means there was an error.
 */
int dump_times_cmd(std::vector<string> &words)
{
	FILE *f;
	time_t now;
	char time_buffer[100];

	if (words.size() != 2) {
		printf("Wrong # args; must be 'dump_times file'\n");
		return 0;
	}
	f = fopen(words[1].c_str(), "w");
	if (f == NULL) {
		printf("Couldn't open file %s: %s\n", words[1].c_str(),
				strerror(errno));
		return 0;
	}

	time(&now);
	strftime(time_buffer, sizeof(time_buffer), "%Y-%m-%d %H:%M:%S",
			localtime(&now));
	fprintf(f, "# Round-trip times measured by cp_node at %s\n",
			time_buffer);
	fprintf(f, "# --protocol %s, --workload %s, --gpbs %.1f --threads %d,\n",
			protocol, workload, net_gbps, client_ports);
	fprintf(f, "# --server-nodes %d --server-ports %d, --client-max %d\n",
			server_nodes, server_ports, client_max);
	fprintf(f, "# Length   RTT (usec)\n");
	for (client *client: clients) {
		__u32 start = client->total_responses % NUM_CLIENT_STATS;
		__u32 i = start;
		printf("start:%u\n", start);
		while (1) {
			if (client->actual_rtts[i] != 0) {
				fprintf(f, "%8d %12.2f\n",
						client->actual_lengths[i],
						1e06*to_seconds(
						client->actual_rtts[i]));
				client->actual_rtts[i] = 0;
			}
			i++;
			if (i >= client->actual_rtts.size())
				i = 0;
			if (i == start)
				break;
		}
	}
	fclose(f);
	return 1;
}

/**
 * info_cmd() - Parse the arguments for an "info" command and execute it.
 * @words:  Command arguments (including the command name as @words[0]).
 *
 * Return:  Nonzero means success, zero means there was an error.
 */
int info_cmd(std::vector<string> &words)
{
	const char *workload;
	char *end;
	int mtu;

	if (words.size() != 3) {
		printf("Usage: info workload mtu\n");
		return 0;
	}
	workload = words[1].c_str();
	mtu = strtol(words[2].c_str(), &end, 0);
	if (*end != 0) {
		printf("Bad value '%s' for mtu; must be integer\n",
				words[2].c_str());
		return 0;
	}
	std::vector<dist_point> points = dist_get(workload,
			HOMA_MAX_MESSAGE_LENGTH);
	printf("Workload %s: mean %.1f bytes, overhead %.3f\n",
			workload, dist_mean(points),
			dist_overhead(points, mtu));
	return 1;
}

/**
 * log_cmd() - Parse the arguments for a "log" command and execute it.
 * @words:  Command arguments (including the command name as @words[0]).
 *
 * Return:  Nonzero means success, zero means there was an error.
 */
int log_cmd(std::vector<string> &words)
{
	for (unsigned i = 1; i < words.size(); i++) {
		const char *option = words[i].c_str();

		if (strncmp(option, "--", 2) != 0) {
			string message;
			for (unsigned j = i; j < words.size(); j++) {
				if (j != i)
					message.append(" ");
				message.append(words[j]);
			}
			message.append("\n");
			log(NORMAL, "%s", message.c_str());
			return 1;
		}

		if (strcmp(option, "--file") == 0) {
			FILE *f;
			if ((i + 1) >= words.size()) {
				printf("No value provided for %s\n",
						option);
				return 0;
			}
			const char *name = words[i+1].c_str();
			if (strcmp(name, "-") == 0)
				f = stdout;
			else {
				f = fopen(name, "w");
				if (f == NULL) {
					printf("Couldn't open %s: %s\n", name,
							strerror(errno));
					return 0;
				}
				setlinebuf(f);
			}
			if (log_file != stdout)
				fclose(log_file);
			log_file = f;
			i++;
		} else if (strcmp(option, "--level") == 0) {
			if ((i + 1) >= words.size()) {
				printf("No value provided for %s\n",
						option);
				return 0;
			}
			if (words[i+1].compare("normal") == 0)
				log_level = NORMAL;
			else if (words[i+1].compare("verbose") == 0)
				log_level = VERBOSE;
			else {
				printf("Unknown log level '%s'; must be "
						"normal or verbose\n",
						words[i+1].c_str());
				return 0;
			}
			log(NORMAL, "Log level is now %s\n",
					words[i+1].c_str());
			i++;
		} else {
			printf("Unknown option '%s'\n", option);
			return 0;
		}
	}
	return 1;
}

/**
 * server_cmd() - Parse the arguments for a "server" command and execute it.
 * @words:  Command arguments (including the command name as @words[0]).
 *
 * Return:  Nonzero means success, zero means there was an error.
 */
int server_cmd(std::vector<string> &words)
{
	first_port = 4000;
	inet_family = AF_INET;
    protocol = "homa";

	server_ports = 1;

	for (unsigned i = 1; i < words.size(); i++) {
		const char *option = words[i].c_str();

		if (strcmp(option, "--first-port") == 0) {
			if (!parse(words, i+1, &first_port, option, "integer"))
				return 0;
			i++;
        } else if (strcmp(option, "--queues") == 0) {
			if (!parse(words, i+1, &nr_nic_queues, option, "integer"))
				return 0;
			i++;
		} else if (strcmp(option, "--ports") == 0) {
			if (!parse(words, i+1, &server_ports, option, "integer"))
				return 0;
			i++;
		} else if (strcmp(option, "--pin") == 0) {
			if (!parse(words, i+1, &pin_core_start, option, "integer"))
				return 0;
			i++; 
		} else {
			printf("Unknown option '%s'\n", option);
			return 0;
		}
	}

	get_local_ip();

    struct eTran_cfg cfg = {0};

    cfg.nr_app_threads = server_ports;
    cfg.nr_nic_queues = nr_nic_queues;
    cfg.proto = IPPROTO_HOMA;

    if (eTran_init(&cfg)) {
        printf("Failed to init eTran\n");
        exit(EXIT_FAILURE);
    }

	register_done = true;

    for (int i = 0; i < server_ports; i++) {
        homa_server *server = new homa_server(i, local_ip, (uint16_t)(first_port + i), 1);
        homa_servers.push_back(server);
    }

	last_per_server_rpcs.resize(server_ports, 0);
	last_stats_time = 0;
	return 1;
}

/**
 * stop_cmd() - Parse the arguments for a "stop" command and execute it.
 * @words:  Command arguments (including the command name as @words[0]).
 *
 * Return:  Nonzero means success, zero means there was an error.
 */
int stop_cmd(std::vector<string> &words)
{
	for (unsigned i = 1; i < words.size(); i++) {
		const char *option = words[i].c_str();
		if (strcmp(option, "clients") == 0) {
			for (client *client: clients)
				delete client;
			clients.clear();
		} else if (strcmp(option, "senders") == 0) {
			for (client *client: clients)
				client->stop_sender();
		} else if (strcmp(option, "servers") == 0) {
			for (homa_server *server: homa_servers)
				delete server;
			homa_servers.clear();
			last_per_server_rpcs.clear();
			for (server_metrics *m: metrics)
				delete m;
			metrics.clear();
		} else {
			printf("Unknown option '%s'; must be clients, senders, "
				"or servers\n", option);
			return 0;
		}
	}
	return 1;
}

/**
 * tt_cmd() - Parse the arguments for a "tt" command and execute it.
 * @words:  Command arguments (including the command name as @words[0]).
 *
 * Return:  Nonzero means success, zero means there was an error.
 */
int tt_cmd(std::vector<string> &words)
{
	if (words.size() < 2) {
		printf("tt command requires an option\n");
		return 0;
	}
	const char *option = words[1].c_str();
	if (strcmp(option, "freeze") == 0) {
		tt("Freezing timetrace because of tt freeze command");
		time_trace::freeze();
	} else if (strcmp(option, "freezeboth") == 0) {
		tt("Freezing timetrace because of tt freezeboth command");
		time_trace::freeze();
		kfreeze();
	} else if (strcmp(option, "kfreeze") == 0) {
		kfreeze();
	} else if (strcmp(option, "print") == 0) {
		if (words.size() < 3) {
			printf("No file name provided for %s\n", option);
			return 0;
		}
		int error = time_trace::print_to_file(words[2].c_str());
		if (error) {
			printf("Couldn't open time trace file '%s': %s",
				words[2].c_str(), strerror(error));
			return 0;
		}
	} else {
		printf("Unknown option '%s'; must be freeze, freezeboth, "
				"kfreeze or print\n",
				option);
		return 0;
	}
	return 1;
}

/**
 * exec_words() - Given a command that has been parsed into words,
 * execute the command corresponding to the words.
 * @words:  Each entry represents one word of the command, like argc/argv.
 *
 * Return:  Nonzero means success, zero means there was an error.
 */
int exec_words(std::vector<string> &words)
{
	std::lock_guard<std::mutex> lock(cmd_lock);
	if (words.size() == 0)
		return 1;
	if (words[0].compare("client") == 0) {
		return client_cmd(words);
	} else if (words[0].compare("debug") == 0) {
		return debug_cmd(words);
	} else if (words[0].compare("dump_times") == 0) {
		return dump_times_cmd(words);
	} else if (words[0].compare("info") == 0) {
		return info_cmd(words);
	} else if (words[0].compare("log") == 0) {
		return log_cmd(words);
	} else if (words[0].compare("exit") == 0) {
		if (log_file != stdout)
			log(NORMAL, "cp_node exiting (exit command)\n");
		exit(0);
	} else if (words[0].compare("server") == 0) {
		return server_cmd(words);
	} else if (words[0].compare("stop") == 0) {
		return stop_cmd(words);
	} else if (words[0].compare("tt") == 0) {
		return tt_cmd(words);
	} else {
		printf("Unknown command '%s'\n", words[0].c_str());
		return 0;
	}
}

/**
 * exec_string() - Given a string, parse it into words and execute the
 * resulting command.
 * @cmd:  Command to execute.
 */
void exec_string(const char *cmd)
{
	const char *p = cmd;
	std::vector<string> words;

	if (log_file != stdout)
		log(NORMAL, "Command: %s\n", cmd);

	while (1) {
		int word_length = strcspn(p, " \t\n");
		if (word_length > 0)
			words.emplace_back(p, word_length);
		p += word_length;
		if (*p == 0)
			break;
		p++;
	}
	exec_words(words);
}

/**
 * error_handler() - This method is invoked after a terminal error such
 * as a segfault; it logs a backtrace and exits.
 * @signal    Signal number that caused this method to be invoked.
 * @info      Details about the cause of the signal; used to find the
 *            faulting address for segfaults.
 * @ucontext  CPU context at the time the signal occurred.
 */
void error_handler(int signal, siginfo_t* info, void* ucontext)
{
	ucontext_t* uc = static_cast<ucontext_t*>(ucontext);
	void* caller_address = reinterpret_cast<void*>(
			uc->uc_mcontext.gregs[REG_RIP]);

	log(NORMAL, "Signal %d (%s) at address %p from %p\n",
			signal, strsignal(signal), info->si_addr,
			caller_address);

	const int max_frames = 128;
	void* return_addresses[max_frames];
	int frames = backtrace(return_addresses, max_frames);

	// Overwrite sigaction with caller's address.
	return_addresses[1] = caller_address;

	char** symbols = backtrace_symbols(return_addresses, frames);
	if (symbols == NULL) {
		/* If the malloc failed we might be able to get the backtrace out
		 * to stderr still.
		 */
		log(NORMAL, "backtrace_symbols failed; trying "
				"backtrace_symbols_fd\n");
		backtrace_symbols_fd(return_addresses, frames, 2);
		return;
	}

	log(NORMAL, "Backtrace:\n");
	for (int i = 1; i < frames; ++i)
		log(NORMAL, "%s\n", symbols[i]);
	fflush(log_file);
	while(1) {}

	/* Use abort, rather than exit, to dump core/trap in gdb. */
	abort();
}

int main(int argc, char *argv[]) {
    
    time_trace::thread_buffer thread_buffer("main");
    setlinebuf(stdout);
    signal(SIGPIPE, SIG_IGN);

    struct sigaction action;
	action.sa_sigaction = error_handler;
	action.sa_flags = SA_RESTART | SA_SIGINFO;
	if (sigaction(SIGSEGV, &action, NULL) != 0)
		log(VERBOSE, "Couldn't set signal handler for SIGSEGV; "
				"continuing anyway\n");

	if ((argc >= 2) && (strcmp(argv[1], "--help") == 0)) {
		print_help(argv[0]);
		exit(0);
	}

    if (argc > 1) {
        std::vector<string> words;
        for (int i = 1; i < argc; i++)
            words.emplace_back(argv[i]);
        if (!exec_words(words))
            exit(1);

        /* Instead of going interactive, just print stats.
            * every second.
            */
        log_stats();
	}

	std::thread logger(log_stats);
    while (1) {
		string line;

		printf("%% ");
		fflush(stdout);
		if (!std::getline(std::cin, line)) {
			if (log_file != stdout)
				log(NORMAL, "cp_node exiting (EOF on stdin)\n");
			exit(0);
		}
		exec_string(line.c_str());
	}

	if (register_done)
    	eTran_exit();
    
    return 0;
}
