#pragma once

#include <linux/types.h>

#include <tran_def/tcp.h>

#define DEFAULT_MTU 1500
#define ETH_HDR_LEN 14
#define IP_HDR_LEN 20

#define XDP_GEN_PKT_SIZE 128
#define NAPI_BATCH_SIZE 64

// #define XDP_DEBUG
// #define XDP_EGRESS_DEBUG
// #define XDP_GEN_DEBUG