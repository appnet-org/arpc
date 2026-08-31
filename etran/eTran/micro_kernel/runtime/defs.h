#pragma once

#include <string>

#include <tran_def/homa.h>
#include <tran_def/tcp.h>

#define INIT_CHECK(call) do { \
    int ret = (call); \
    if (ret) { \
        fprintf(stderr, "ERROR: %s failed (%d): %s\n", #call, ret, strerror(-ret)); \
        return ret; \
    } \
} while (0)

#define SUPPORT_PROTO(proto) ((proto) == IPPROTO_TCP || (proto) == IPPROTO_HOMA)

// #define DEBUG_TCP

const std::string ENTRANCE_BPF_OBJ_PATH = "eBPF/entrance/entrance.o";
const std::string TCP_BPF_OBJ_PATH = "eBPF/tcp/main.o";
const std::string HOMA_BPF_OBJ_PATH = "eBPF/homa/main.o";
const std::string MICRO_KERNEL_SOCK_PATH = "/tmp/micro_kernel_socket";
const unsigned int MAX_APP_THREADS = 20;
const unsigned int MAX_SUPPORT_APP = 32;
const unsigned int CP_CPU = 19;

const int enrollment_to_ms = 0;
const int network_to_ms = 0;
// interval between processing slowpath
const int sp_interval_ms = 1;

const unsigned int IO_BATCH_SIZE = 32;

const uint16_t PORT_MIN = 1000;
const uint16_t PORT_MAX = 60000;

// ebpf.cc
extern int ebpf_init(void);
extern void ebpf_exit();

// control_plance.cc
extern int ctx_init(void);
extern int thread_init(void);
extern void wait_thread(void);
extern void shutdown_thread(void);

extern void dump_bpf_maps();
extern void dump_ring_stats(void);

extern void kick_napi(void);