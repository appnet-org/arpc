#pragma once

#include <tran_def/homa.h>
#include <arpa/inet.h>

#define HOMA_MSS (1514 - 14 - 20 - HOMA_DATA_H)
#define HOMA_PAYLOAD_OFFSET (14 + 20 + HOMA_DATA_H)

struct eTranhoma_socket {
    uint32_t local_ip;
    uint16_t local_port;

    struct eTran_socket_t *s;
    
    eTranhoma_socket() {
        local_ip = 0;
        local_port = 0;
        s = nullptr;
    }
};

enum eTranhoma_event_type {
    ETRANHOMA_EV_SOCKET_BIND,
    ETRANHOMA_EV_SOCKET_CLOSE,
};

struct eTranhoma_event {
    enum eTranhoma_event_type type;
    union {
        struct {
            int16_t status;
            struct eTranhoma_socket *hs;
            int fd;
        } bind;
        
        struct {
            int16_t status;
            struct eTranhoma_socket *hs;
            int fd;
        } close;

    } ev;
};

struct eTran_homa_rpc_tuple {
    uint32_t remote_ip;
    uint16_t remote_port;
    uint64_t rpcid;

    // constructor
    eTran_homa_rpc_tuple(uint32_t remote_ip, uint16_t remote_port, uint64_t rpcid)
        : remote_ip(remote_ip), remote_port(remote_port), rpcid(rpcid) {}
    
    uint32_t hash() const
    {
        return ((std::hash<uint32_t>()(remote_ip) ^ (std::hash<uint32_t>()(rpcid) << 1)) >> 1) ^
               (std::hash<uint16_t>()(remote_port) << 1);
    }
};

struct eTran_homa_rpc_tuple_hash
{
    std::size_t operator()(const eTran_homa_rpc_tuple &k) const
    {
        return ((std::hash<uint32_t>()(k.remote_ip) ^ (std::hash<uint32_t>()(k.rpcid) << 1)) >> 1) ^
               (std::hash<uint16_t>()(k.remote_port) << 1);
    }
};

struct eTran_homa_rpc_tuple_equal
{
    bool operator()(const eTran_homa_rpc_tuple &lhs, const eTran_homa_rpc_tuple &rhs) const
    {
        return lhs.remote_ip == rhs.remote_ip && lhs.remote_port == rhs.remote_port && lhs.rpcid == rhs.rpcid;
    }
};