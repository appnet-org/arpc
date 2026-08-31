#include "trans_ebpf.h"

/***************** Entrance *****************/
int eTranEntrance::load_ebpf_programs(void)
{
    int err = 0;
    struct bpf_object *obj;

    obj = bpf_object__open_file(ENTRANCE_BPF_OBJ_PATH.c_str(), nullptr);
    err = libbpf_get_error(obj);
    if (err)
    {
        fprintf(stderr, "ERROR: opening BPF object file failed (%d): %s\n", err, strerror(-err));
        return err;
    }
    for (size_t i = 0; i < sizeof(_entrance_bpf_progs) / sizeof(_entrance_bpf_progs[0]); i++)
    {
        _entrance_bpf_progs[i].bpf_prog = bpf_object__find_program_by_name(obj, _entrance_bpf_progs[i].name.c_str());
        if (!_entrance_bpf_progs[i].bpf_prog)
        {
            fprintf(stderr, "bpf_object__find_program_by_name:%s", _entrance_bpf_progs[i].name.c_str());
            return -ENOENT;
        }
        err = bpf_program__set_type(_entrance_bpf_progs[i].bpf_prog, _entrance_bpf_progs[i].prog_type);
        if (err)
        {
            fprintf(stderr, "bpf_program__set_type:%d", _entrance_bpf_progs[i].prog_type);
            return err;
        }
    }

    err = bpf_object__load(obj);
    if (err)
    {
        fprintf(stderr, "ERROR: loading BPF object file failed (%d): %s\n", err, strerror(-err));
        return err;
    }

    err = bpf_program__fd(_entrance_bpf_progs[ENTRANCE_XDP_IDX].bpf_prog);
    if (err < 0)
    {
        perror("bpf_program__fd");
        return err;
    }

    _ebpf.xdp_fd = err;

    err = bpf_program__fd(_entrance_bpf_progs[ENTRANCE_XDP_CPUMAP_IDX].bpf_prog);
    if (err < 0)
    {
        perror("bpf_program__fd");
        return err;
    }

    _ebpf.cpumap_prog_fd = err;

    err = bpf_program__fd(_entrance_bpf_progs[ENTRANCE_XDP_EGRESS_IDX].bpf_prog);
    if (err < 0)
    {
        perror("bpf_program__fd");
        return err;
    }

    _ebpf.xdp_egress_fd = err;

    err = bpf_program__fd(_entrance_bpf_progs[ENTRANCE_XDP_GEN_IDX].bpf_prog);
    if (err < 0)
    {
        perror("bpf_program__fd");
        return err;
    }
    _ebpf.xdp_gen_fd = err;

    err = bpf_xdp_attach(_ebpf.ifindex, _ebpf.xdp_fd, _ebpf.attach_mode, nullptr);
    if (err < 0)
    {
        fprintf(stderr, "Error: bpf_set_link_xdp_fd failed for interface %d\n", _ebpf.ifindex);
        return err;
    }

    _ebpf.xdp_prog = xdp_program__open_file(ENTRANCE_BPF_OBJ_PATH.c_str(), "xdp_sock", nullptr);
    err = libxdp_get_error(_ebpf.xdp_prog);
    if (err)
    {
        fprintf(stderr, "ERROR: program loading failed: %s\n", strerror(err));
        return err;
    }

    _ebpf.xdp_prog->prog_fd = _ebpf.xdp_fd;
    _ebpf.xdp_prog->bpf_obj = obj;

    return 0;
}

int eTranEntrance::init_ebpf_maps(void)
{
    /* Add tcp and homa programs to entrance */
    _umem_id_tran_map_fd =
        bpf_map__fd(bpf_object__find_map_by_name(xdp_program__bpf_obj(_ebpf.xdp_prog), "umem_id_tran_map"));
    _tran_xdp_map_fd =
        bpf_map__fd(bpf_object__find_map_by_name(xdp_program__bpf_obj(_ebpf.xdp_prog), "tran_xdp_map"));
    _tran_xdp_egress_map_fd =
        bpf_map__fd(bpf_object__find_map_by_name(xdp_program__bpf_obj(_ebpf.xdp_prog), "tran_xdp_egress_map"));
    _tran_xdp_gen_map_fd =
        bpf_map__fd(bpf_object__find_map_by_name(xdp_program__bpf_obj(_ebpf.xdp_prog), "tran_xdp_gen_map"));

    if (_umem_id_tran_map_fd < 0 || _tran_xdp_map_fd < 0 ||
        _tran_xdp_egress_map_fd < 0 || _tran_xdp_gen_map_fd < 0)
    {
        fprintf(stderr, "ERROR: bpf_map__fd failed for tran_xdp_map\n");
        return -1;
    }
    return 0;
}

/******************* TCP *******************/
int eTranTCP::load_ebpf_programs(void)
{
    int err = 0;
    struct bpf_object *obj;
    std::string object_path = "eBPF/tcp/main_case" +
        std::to_string(_trans_params.tcp.arpc_case) + ".o";

    obj = bpf_object__open_file(object_path.c_str(), nullptr);
    err = libbpf_get_error(obj);
    if (err)
    {
        fprintf(stderr, "ERROR: opening BPF object file failed (%d): %s\n", err, strerror(-err));
        return err;
    }

    for (size_t i = 0; i < sizeof(_tcp_bpf_progs) / sizeof(_tcp_bpf_progs[0]); i++)
    {
        _tcp_bpf_progs[i].bpf_prog = bpf_object__find_program_by_name(obj, _tcp_bpf_progs[i].name.c_str());
        if (!_tcp_bpf_progs[i].bpf_prog)
        {
            fprintf(stderr, "bpf_object__find_program_by_name:%s", _tcp_bpf_progs[i].name.c_str());
            return -ENOENT;
        }

        err = bpf_program__set_type(_tcp_bpf_progs[i].bpf_prog, _tcp_bpf_progs[i].prog_type);
        if (err)
        {
            fprintf(stderr, "bpf_program__set_type:%d", _tcp_bpf_progs[i].prog_type);
            return err;
        }
    }

    err = bpf_object__load(obj);
    if (err)
    {
        fprintf(stderr, "ERROR: loading BPF object file failed (%d): %s\n", err, strerror(-err));
        return err;
    }

    err = bpf_program__fd(_tcp_bpf_progs[TCP_XDP_IDX].bpf_prog);
    if (err < 0)
    {
        perror("bpf_program__fd");
        return err;
    }

    _ebpf.xdp_fd = err;

    err = bpf_program__fd(_tcp_bpf_progs[TCP_XDP_CPUMAP_IDX].bpf_prog);
    if (err < 0)
    {
        perror("bpf_program__fd");
        return err;
    }

    _ebpf.cpumap_prog_fd = err;

    err = bpf_program__fd(_tcp_bpf_progs[TCP_XDP_EGRESS_IDX].bpf_prog);
    if (err < 0)
    {
        perror("bpf_program__fd");
        return err;
    }

    _ebpf.xdp_egress_fd = err;

    err = bpf_program__fd(_tcp_bpf_progs[TCP_XDP_GEN_IDX].bpf_prog);
    if (err < 0)
    {
        perror("bpf_program__fd");
        return err;
    }

    _ebpf.xdp_gen_fd = err;

    _ebpf.xdp_prog = xdp_program__open_file(object_path.c_str(), "xdp_sock", nullptr);
    err = libxdp_get_error(_ebpf.xdp_prog);
    if (err)
    {
        fprintf(stderr, "ERROR: program loading failed: %s\n", strerror(err));
        return err;
    }

    _ebpf.xdp_prog->prog_fd = _ebpf.xdp_fd;
    _ebpf.xdp_prog->bpf_obj = obj;

    return 0;
}

static inline size_t roundup_page(size_t sz)
{
    long page_size = sysconf(_SC_PAGE_SIZE);
    return (sz + page_size - 1) / page_size * page_size;
}

int eTranTCP::init_ebpf_maps(void)
{
    _tcp_connection_map_fd = bpf_map__fd(bpf_object__find_map_by_name(xdp_program__bpf_obj(_ebpf.xdp_prog), "bpf_tcp_conn_map"));
    if (_tcp_connection_map_fd < 0)
    {
        fprintf(stderr, "ERROR: bpf_map__fd failed for bpf_tcp_conn_map\n");
        return -1;
    }

    _tcp_cc_map_fd = bpf_map__fd(bpf_object__find_map_by_name(xdp_program__bpf_obj(_ebpf.xdp_prog), "bpf_cc_map"));
    if (_tcp_cc_map_fd < 0)
    {
        fprintf(stderr, "ERROR: bpf_map__fd failed for bpf_cc_map\n");
        return -1;
    }

    // mmap cc map
    const size_t map_sz = roundup_page(sizeof(struct bpf_cc_map_user));
    _tcp_cc_map_mmap = (struct bpf_cc_map_user *)mmap(nullptr, map_sz, PROT_READ | PROT_WRITE, MAP_SHARED, _tcp_cc_map_fd, 0);
    if (_tcp_cc_map_mmap == MAP_FAILED)
    {
        fprintf(stderr, "ERROR: mmap failed for bpf_cc_map\n");
        return -1;
    }

    for (unsigned int i = 0; i < MAX_TCP_FLOWS; i++)
        _avail_cc_idxs.push_back(i);

    _tw_outer_map_fd = bpf_map__fd(bpf_object__find_map_by_name(xdp_program__bpf_obj(_ebpf.xdp_prog), "tw_outer_map"));
    if (_tw_outer_map_fd < 0)
    {
        fprintf(stderr, "ERROR: bpf_map__fd failed for tw_outer_map\n");
        return -1;
    }
    /* install timingwheel slots */
    for (int i = 0; i < MAX_CPU; i++)
    {
        struct bpf_map_create_opts map_opts = {
            .sz = sizeof(map_opts),
            .map_extra = MAX_BUCKETS,
        };
        int fd;
        std::string tw_name = "tw_" + std::to_string(i);
        fd = bpf_map_create(BPF_MAP_TYPE_PKT_QUEUE, tw_name.c_str(), sizeof(__u32), sizeof(__u32), NR_SLOT_PER_BKT, &map_opts);
        if (fd < 0)
        {
            fprintf(stderr, "ERROR: bpf_map_create failed for %s\n", tw_name.c_str());
            perror("\n");
            return -1;
        }
        if (bpf_map_update_elem(_tw_outer_map_fd, &i, &fd, BPF_ANY))
        {
            fprintf(stderr, "ERROR: bpf_map_update_elem failed for %s\n", tw_name.c_str());
            return -1;
        }
        _tw_fds.push_back(fd);
    }
    fprintf(stdout, "\tTCP: Timing wheel init done.\n");

    return 0;
}

/******************* Homa *******************/

int eTranHoma::load_ebpf_programs(void)
{
    int err = 0;
    struct bpf_object *obj;

    obj = bpf_object__open_file(HOMA_BPF_OBJ_PATH.c_str(), nullptr);
    err = libbpf_get_error(obj);
    if (err)
    {
        fprintf(stderr, "ERROR: opening BPF object file failed (%d): %s\n", err, strerror(-err));
        return err;
    }

    for (size_t i = 0; i < sizeof(_homa_bpf_progs) / sizeof(_homa_bpf_progs[0]); i++)
    {
        _homa_bpf_progs[i].bpf_prog = bpf_object__find_program_by_name(obj, _homa_bpf_progs[i].name.c_str());
        if (!_homa_bpf_progs[i].bpf_prog)
        {
            fprintf(stderr, "bpf_object__find_program_by_name:%s", _homa_bpf_progs[i].name.c_str());
            return -ENOENT;
        }

        err = bpf_program__set_type(_homa_bpf_progs[i].bpf_prog, _homa_bpf_progs[i].prog_type);
        if (err)
        {
            fprintf(stderr, "bpf_program__set_type:%d", _homa_bpf_progs[i].prog_type);
            return err;
        }
    }

    err = bpf_object__load(obj);
    if (err)
    {
        fprintf(stderr, "ERROR: loading BPF object file failed (%d): %s\n", err, strerror(-err));
        return err;
    }

    err = bpf_program__fd(_homa_bpf_progs[HOMA_XDP_IDX].bpf_prog);
    if (err < 0)
    {
        perror("bpf_program__fd");
        return err;
    }

    _ebpf.xdp_fd = err;

    err = bpf_program__fd(_homa_bpf_progs[HOMA_XDP_CPUMAP_IDX].bpf_prog);
    if (err < 0)
    {
        perror("bpf_program__fd");
        return err;
    }

    _ebpf.cpumap_prog_fd = err;

    err = bpf_program__fd(_homa_bpf_progs[HOMA_XDP_EGRESS_IDX].bpf_prog);
    if (err < 0)
    {
        perror("bpf_program__fd");
        return err;
    }

    _ebpf.xdp_egress_fd = err;

    err = bpf_program__fd(_homa_bpf_progs[HOMA_XDP_GEN_IDX].bpf_prog);
    if (err < 0)
    {
        perror("bpf_program__fd");
        return err;
    }

    _ebpf.xdp_gen_fd = err;

    for (int i = 0; i < 9; i++)
    {
        err = bpf_program__fd(_homa_bpf_progs[HOMA_TAIL_CALL_IDX + i].bpf_prog);
        if (err < 0)
        {
            perror("bpf_program__fd");
            return err;
        }
        _tail_call_fd[i] = err;
    }

    _ebpf.xdp_prog = xdp_program__open_file(HOMA_BPF_OBJ_PATH.c_str(), "xdp_sock", nullptr);
    err = libxdp_get_error(_ebpf.xdp_prog);
    if (err)
    {
        fprintf(stderr, "ERROR: program loading failed: %s\n", strerror(err));
        return err;
    }

    _ebpf.xdp_prog->prog_fd = _ebpf.xdp_fd;
    _ebpf.xdp_prog->bpf_obj = obj;

    return 0;
}

int eTranHoma::init_ebpf_maps(void)
{
    int key, val;

    _homa_rpc_fd = bpf_map__fd(bpf_object__find_map_by_name(xdp_program__bpf_obj(_ebpf.xdp_prog), "rpc_tbl"));
    if (_homa_rpc_fd < 0)
    {
        fprintf(stderr, "ERROR: bpf_map__fd failed for rpc_tbl\n");
        return -1;
    }

    _homa_port_tbl_fd = bpf_map__fd(bpf_object__find_map_by_name(xdp_program__bpf_obj(_ebpf.xdp_prog), "port_tbl"));
    if (_homa_port_tbl_fd < 0)
    {
        fprintf(stderr, "ERROR: bpf_map__fd failed for port_tbl\n");
        return -1;
    }

    _homa_cpumap_fd = bpf_map__fd(bpf_object__find_map_by_name(xdp_program__bpf_obj(_ebpf.xdp_prog), "cpumap"));
    if (_homa_cpumap_fd < 0)
    {
        fprintf(stderr, "ERROR: bpf_map__fd failed for cpumap\n");
        return -1;
    }

    /* Install cpumap XDP programs */
    for (int i = 0; i < MAX_CPU; i++)
    {
        __u32 key = i;
        struct bpf_cpumap_val val = {0};
        val.qsize = 2048;
        val.bpf_prog.fd = _ebpf.cpumap_prog_fd;
        if (bpf_map_update_elem(_homa_cpumap_fd, &key, &val, BPF_ANY))
        {
            fprintf(stderr, "ERROR: bpf_map_update_elem cpumap_fd\n");
            return -1;
        }
    }

    _homa_tail_call_fd = bpf_map__fd(bpf_object__find_map_by_name(xdp_program__bpf_obj(_ebpf.xdp_prog), "xdp_gen_tail_call_map"));
    if (_homa_tail_call_fd < 0)
    {
        fprintf(stderr, "ERROR: bpf_map__fd failed for xdp_gen_tail_call_map\n");
        return -1;
    }

    /* Install XDP_GEN_CHOOSE_RPC_TO_GRANT and 8 XDP_GEN_COMPLETE_GRANT */
    for (int i = 0; i < 9; i++)
    {
        key = i;
        val = _tail_call_fd[i];
        if (bpf_map_update_elem(_homa_tail_call_fd, &key, &val, BPF_ANY))
        {
            fprintf(stderr, "ERROR: bpf_map_update_elem XDP_GEN_COMPLETE_GRANT\n");
            return -1;
        }
    }

    /* Initialize local_ip */
    struct bpf_map *data_map;
    data_map = bpf_object__find_map_by_name(xdp_program__bpf_obj(_ebpf.xdp_prog), ".bss.local_ip");
    if (!data_map || !bpf_map__is_internal(data_map))
    {
        fprintf(stderr, "ERROR: bss map found!\n");
        return -1;
    }

    key = 0;
    uint32_t local_ip = _etran_nic->_local_ip;
    if (bpf_map_update_elem(bpf_map__fd(data_map), &key, &local_ip, BPF_ANY))
    {
        fprintf(stderr, "ERROR: bpf_map_update_elem local_ip %d!\n", local_ip);
        return -1;
    }

    /* Initialize workload_type */
    data_map = bpf_object__find_map_by_name(xdp_program__bpf_obj(_ebpf.xdp_prog), ".data.workload_type");
    if (!data_map || !bpf_map__is_internal(data_map))
    {
        fprintf(stderr, "ERROR: bss map found!\n");
        return -1;
    }
    key = 0;
    if (bpf_map_update_elem(bpf_map__fd(data_map), &key, &_trans_params.homa.workload_type, BPF_ANY))
    {
        fprintf(stderr, "ERROR: bpf_map_update_elem workload_type %d!\n", _trans_params.homa.workload_type);
        return -1;
    }
    printf("\tHoma: Default workload type w%d.\n", _trans_params.homa.workload_type);

    key = 0;
    if (bpf_map_update_elem(bpf_map__fd(data_map), &key, &_etran_nic->_num_queues, BPF_ANY))
    {
        fprintf(stderr, "ERROR: bpf_map_update_elem _etran_nic->_num_queues %d!\n", _etran_nic->_num_queues);
        return -1;
    }
    // Initialize avail_pacing_idx
    data_map = bpf_object__find_map_by_name(xdp_program__bpf_obj(_ebpf.xdp_prog), "avail_pacing_idx");
    if (!data_map)
    {
        fprintf(stderr, "ERROR: bpf_map__fd failed for avail_pacing_idx\n");
        return -1;
    }

    for (int i = 0; i < MAX_BUCKET_SIZE; i++)
    {
        __u64 qid = i;
        if (bpf_map_update_elem(bpf_map__fd(data_map), nullptr, &qid, BPF_ANY))
        {
            fprintf(stderr, "ERROR: bpf_map_update_elem qid\n");
            return -1;
        }
    }

    return 0;
}
