#ifndef NIC_H
#define NIC_H

#include <string>
#include <vector>
#include <iostream>
#include <sstream>
#include <iomanip>

#include <intf/intf_ebpf.h>
#include <runtime/ebpf_if.h>
#include <utils/utils.h>

class eTranNIC
{
public:
    /* interface name */
    std::string _if_name;
    /* local IPv4 address */
    uint32_t _local_ip;
    /* number of NIC queues */
    unsigned int _num_queues;
    /* NIC queue length */
    unsigned int _queue_len;
    /* enable NAPI polling */
    bool _napi_polling;
    /* enable socket busy poll */
    bool _socket_busy_poll;
    /* enable interrupt affinity */
    bool _intr_affinity;
    /* enable coalescing */
    bool _coalescing;

    /* NIC queue information */
    struct nic_queue_info _nic_queues[MAX_NIC_QUEUES];

    /* available queue index, starts from zero */
    std::vector<unsigned int> _available_qids;
    
    /* available keys in BPF_MAP_TYPE_XSKMAP, starts from zero */
    std::vector<int> _available_xsk_keys;

    eTranNIC(std::string if_name, unsigned int num_queues, unsigned int queue_len,
             bool napi_polling, bool socket_busy_poll, bool intr_affinity, bool coalescing) : _if_name(if_name), _num_queues(num_queues), _queue_len(queue_len),
                                                                     _napi_polling(napi_polling), _socket_busy_poll(socket_busy_poll), _intr_affinity(intr_affinity), _coalescing(coalescing)
    {
        if (create_nic()) {
            throw std::runtime_error("Failed to create NIC");
        }

        memset(_nic_queues, 0, sizeof(_nic_queues));
        _available_qids.resize(_num_queues);
        for (unsigned int i = 0; i < _num_queues; i++) {
            _nic_queues[i].qid = i;
            _available_qids[i] = i;
        }

        _available_xsk_keys.resize(MAX_XSK_FD);
        for (int i = 0; i < MAX_XSK_FD; i++) {
            _available_xsk_keys[i] = i;
        }


    }

    ~eTranNIC()
    {
        destroy_nic();
    }

private:
    int create_nic(void);
    void destroy_nic(void);
};

#endif // NIC_H