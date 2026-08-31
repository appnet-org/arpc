#ifndef TRANS_EBPF_H
#define TRANS_EBPF_H

#include "ebpf.h"

struct homa_params_t
{
    int workload_type;
};

struct tcp_params_t
{
    unsigned int rx_buf_size;
    unsigned int tx_buf_size;
    unsigned int arpc_case;
};

struct trans_params_t
{
    union
    {
        struct homa_params_t homa;
        struct tcp_params_t tcp;
    };
};

class eTranTransport
{
public:
    struct ebpf_info_t _ebpf;

    eTranNIC *_etran_nic;

    struct trans_params_t _trans_params;

    eTranTransport(eTranNIC *etran_nic, enum xdp_attach_mode attach_mode, unsigned int xdp_bind_flags, bool busy_poll, unsigned int busy_poll_batch_size, struct trans_params_t *trans_params)
    {
        _etran_nic = etran_nic;
        _ebpf.if_name = _etran_nic->_if_name;
        _ebpf.ifindex = if_nametoindex(_etran_nic->_if_name.c_str());
        _ebpf.busy_poll = busy_poll;
        _ebpf.busy_poll_batch_size = busy_poll_batch_size;
        _ebpf.xdp_fd = -1;
        _ebpf.xdp_egress_fd = -1;
        _ebpf.xdp_gen_fd = -1;
        _ebpf.cpumap_prog_fd = -1;
        _ebpf.xdp_prog = nullptr;
        _ebpf.attach_mode = attach_mode;
        _ebpf.xdp_bind_flags = xdp_bind_flags;
        memcpy(&_trans_params, trans_params, sizeof(struct trans_params_t));
    }

    virtual int load_ebpf_programs(void) = 0;

    virtual int init_ebpf_maps(void) = 0;

    virtual ~eTranTransport() {}
};

class eTranEntrance : public eTranTransport
{
public:
    static constexpr unsigned int ENTRANCE_XDP_IDX = 0;
    static constexpr unsigned int ENTRANCE_XDP_CPUMAP_IDX = 1;
    static constexpr unsigned int ENTRANCE_XDP_EGRESS_IDX = 2;
    static constexpr unsigned int ENTRANCE_XDP_GEN_IDX = 3;

    int _umem_id_tran_map_fd;
    int _tran_xdp_map_fd;
    int _tran_xdp_egress_map_fd;
    int _tran_xdp_gen_map_fd;
    int _key = 0;

    struct bpf_prog_desc _entrance_bpf_progs[4] = {
        {
            .name = "xdp_sock_prog",
            .prog_type = BPF_PROG_TYPE_XDP,
            .bpf_prog = nullptr,
        },
        {
            .name = "xdp_cpumap_prog",
            .prog_type = BPF_PROG_TYPE_XDP,
            .bpf_prog = nullptr,
        },
        {
            .name = "xdp_egress_prog",
            .prog_type = BPF_PROG_TYPE_XDP_EGRESS,
            .bpf_prog = nullptr,
        },
        {
            .name = "xdp_gen_prog",
            .prog_type = BPF_PROG_TYPE_XDP_GEN,
            .bpf_prog = nullptr,
        },
    };

    eTranEntrance(eTranNIC *etran_nic, enum xdp_attach_mode attach_mode, unsigned int xdp_bind_flags, bool busy_poll, unsigned int busy_poll_batch_size, struct trans_params_t *trans_params) : eTranTransport(etran_nic, attach_mode, xdp_bind_flags, busy_poll, busy_poll_batch_size, trans_params)
    {
    }

    int load_ebpf_programs(void) override;

    int init_ebpf_maps(void) override;

    ~eTranEntrance() {}
};

class eTranHoma : public eTranTransport
{
public:
    static constexpr unsigned int HOMA_XDP_IDX = 0;
    static constexpr unsigned int HOMA_XDP_CPUMAP_IDX = 1;
    static constexpr unsigned int HOMA_XDP_EGRESS_IDX = 2;
    static constexpr unsigned int HOMA_XDP_GEN_IDX = 3;
    static constexpr unsigned int HOMA_TAIL_CALL_IDX = 4;
    
    /* RPC state map */
    int _homa_rpc_fd;
    /* port map */
    int _homa_port_tbl_fd;
    /* cpumap map */
    int _homa_cpumap_fd;
    /* tail call map */
    int _homa_tail_call_fd;
    /* tail call program fds */
    int _tail_call_fd[9];

    struct bpf_prog_desc _homa_bpf_progs[13] = {
        {
            .name = "xdp_sock_prog",
            .prog_type = BPF_PROG_TYPE_XDP,
            .bpf_prog = nullptr,
        },
        {
            .name = "xdp_cpumap_prog",
            .prog_type = BPF_PROG_TYPE_XDP,
            .bpf_prog = nullptr,
        },
        {
            .name = "xdp_egress_prog",
            .prog_type = BPF_PROG_TYPE_XDP_EGRESS,
            .bpf_prog = nullptr,
        },
        {
            .name = "xdp_gen_prog",
            .prog_type = BPF_PROG_TYPE_XDP_GEN,
            .bpf_prog = nullptr,
        },
        {
            .name = "choose_rpc_to_grant_prog",
            .prog_type = BPF_PROG_TYPE_XDP_GEN,
            .bpf_prog = nullptr,
        },
        {
            .name = "complete_grant_1_prog",
            .prog_type = BPF_PROG_TYPE_XDP_GEN,
            .bpf_prog = nullptr,
        },
        {
            .name = "complete_grant_2_prog",
            .prog_type = BPF_PROG_TYPE_XDP_GEN,
            .bpf_prog = nullptr,
        },
        {
            .name = "complete_grant_3_prog",
            .prog_type = BPF_PROG_TYPE_XDP_GEN,
            .bpf_prog = nullptr,
        },
        {
            .name = "complete_grant_4_prog",
            .prog_type = BPF_PROG_TYPE_XDP_GEN,
            .bpf_prog = nullptr,
        },
        {
            .name = "complete_grant_5_prog",
            .prog_type = BPF_PROG_TYPE_XDP_GEN,
            .bpf_prog = nullptr,
        },
        {
            .name = "complete_grant_6_prog",
            .prog_type = BPF_PROG_TYPE_XDP_GEN,
            .bpf_prog = nullptr,
        },
        {
            .name = "complete_grant_7_prog",
            .prog_type = BPF_PROG_TYPE_XDP_GEN,
            .bpf_prog = nullptr,
        },
        {
            .name = "complete_grant_8_prog",
            .prog_type = BPF_PROG_TYPE_XDP_GEN,
            .bpf_prog = nullptr,
        },
    };

    eTranHoma(eTranNIC *etran_nic, enum xdp_attach_mode attach_mode, unsigned int xdp_bind_flags, bool busy_poll, unsigned int busy_poll_batch_size, struct trans_params_t *trans_params) : eTranTransport(etran_nic, attach_mode, xdp_bind_flags, busy_poll, busy_poll_batch_size, trans_params)
    {
    }

    int load_ebpf_programs(void) override;

    int init_ebpf_maps(void) override;

    ~eTranHoma() {}
};

class eTranTCP : public eTranTransport
{
public:
    static constexpr unsigned int TCP_XDP_IDX = 0;
    static constexpr unsigned int TCP_XDP_CPUMAP_IDX = 1;
    static constexpr unsigned int TCP_XDP_EGRESS_IDX = 2;
    static constexpr unsigned int TCP_XDP_GEN_IDX = 3;

    /* connection map */
    int _tcp_connection_map_fd;
    /* congestion control map */
    int _tcp_cc_map_fd;
    /* mapping congestion control map */
    struct bpf_cc_map_user *_tcp_cc_map_mmap;
    /* Timing Wheel map */
    int _tw_outer_map_fd;   // outer map
    std::list<int> _tw_fds; // inner maps
    /* available congestion control index */
    std::list<uint32_t> _avail_cc_idxs;

    struct bpf_prog_desc _tcp_bpf_progs[4] = {
        {
            .name = "xdp_sock_prog",
            .prog_type = BPF_PROG_TYPE_XDP,
            .bpf_prog = nullptr,
        },
        {
            .name = "xdp_cpumap_prog",
            .prog_type = BPF_PROG_TYPE_XDP,
            .bpf_prog = nullptr,
        },
        {
            .name = "xdp_egress_prog",
            .prog_type = BPF_PROG_TYPE_XDP_EGRESS,
            .bpf_prog = nullptr,
        },
        {
            .name = "xdp_gen_prog",
            .prog_type = BPF_PROG_TYPE_XDP_GEN,
            .bpf_prog = nullptr,
        },
    };

    eTranTCP(eTranNIC *etran_nic, enum xdp_attach_mode attach_mode, unsigned int xdp_bind_flags, bool busy_poll, unsigned int busy_poll_batch_size, struct trans_params_t *trans_params) : eTranTransport(etran_nic, attach_mode, xdp_bind_flags, busy_poll, busy_poll_batch_size, trans_params)
    {
    }

    int load_ebpf_programs(void) override;

    int init_ebpf_maps(void) override;

    ~eTranTCP() {}
};

#endif // TRANS_EBPF_H