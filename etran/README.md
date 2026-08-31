# eTran: Extensible Kernel Transport with eBPF

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

eTran (extensible kernel Transport) is a system for agilely customizing kernel transports. 

eTran achieves agile customizability and kernel safety by (1) leveraging existing eBPF infrastructure such as built-in data structures (eBPF maps), BPF timer, and XDP for fast packet IO, and (2) extending it with new eBPF hooks and maps to support complex transport functionalities while conforming to the strict eBPF verifier for safety. 

eTran allows safely offloading full transport states and performance-critical operations into the kernel, achieving strong protection. For example, it supports packet acknowledging, flow pacing, fast retransmission, and more in eBPF. We implement TCP (with DCTCP congestion control) and Homa under eTran, and achieve up to 4.8×/1.8× higher throughput with 3.7×/7.5× lower latency compared to existing kernel implementation.

## Transport protocols supported
- [x] Homa
- [x] TCP

## Getting Started

### Build eTran-linux

```bash
git clone https://github.com/eTran-NSDI25/eTran-linux

# Install dependencies
sudo apt update && sudo apt-get install git fakeroot build-essential ncurses-dev xz-utils libssl-dev bc flex libelf-dev bison clang llvm libclang-dev libbpf-dev libelf-dev dwarves libmnl-dev libc6-dev-i386 libcap-dev libgoogle-perftools-dev libdwarf-dev cpufrequtils libpcap-dev automake libtool pkg-config -y

cd ~/linux

make menuconfig

scripts/config --disable SYSTEM_TRUSTED_KEYS
scripts/config --disable SYSTEM_REVOCATION_KEYS

# Compile kernel
make -j`nproc`

# Install kernel modules and kernel
sudo make modules_install -j`nproc` && sudo make install -j`nproc`

# Install kernel headers
sudo make headers_install INSTALL_HDR_PATH=/usr

# One-shot command to compile, install kernel and reboot
make -j`nproc` && sudo make modules_install -j`nproc` && sudo make install -j`nproc` && sudo make headers_install INSTALL_HDR_PATH=/usr && sudo shutdown -r now
```

### Build eTran
```bash
cd ~/eTran
# Install bpftool and llvm
sudo bash install.sh
# Compile eTran
./configure && make -C eTran
```

### Run application examples
1. Warm up systems to make sure routing tables are set up correctly in kernel. E.g., ping between servers and clients.

2. Launch microkernel:
```bash
cd eTran/micro_kernel
sudo ./micro_kernel
```
3. Run application:
```bash
# Homa server
ETRAN_PROTO=homa ./cp_node server
# Homa client
ETRAN_PROTO=homa ./cp_node client --first-server 0 --workload 100 --client-max 1 --one-way
# TCP server
ETRAN_PROTO=tcp ETRAN_NR_APP_THREADS=1 ETRAN_NR_NIC_QUEUES=1 LD_PRELOAD=../shared_lib/libetran.so ./epoll_server -i 192.168.6.1 -l 100000 -b 100000
# TCP client
ETRAN_PROTO=tcp ETRAN_NR_APP_THREADS=1 ETRAN_NR_NIC_QUEUES=1 LD_PRELOAD=../shared_lib/libetran.so ./epoll_client -i 192.168.6.1 -l 100000 -b 100000
```

### ARPC TCP decomposition

The `arpc` branch adds `micro_kernel -a CASE` to select a TCP feature set:

- `1`: basic in-order packet processing and ACKs; no reliability, congestion control, or dynamic flow control.
- `2`: case 1 plus duplicate-ACK and timeout-based retransmission.
- `3`: case 2 plus congestion control and pacing.
- `4`: case 3 plus dynamic receive-window and remote-window flow control, with per-packet ACK generation.

For comparable KV latency measurements, run `flexkvs_bench` with one thread,
one connection, and one outstanding request:

```bash
./flexkvs_bench -q 1 -t 1 -C 1 -p 1 -n 1000 -w 2 -T 10 -c 1 192.168.6.1:11211
```

The benchmark now honors warmup, measurement, and cooldown durations and emits
one final `RESULT` line.

Run all four cases on the two CloudLab nodes with:

```bash
./scripts/run_arpc_kv_cases.sh
```

The script warms the routing and neighbor tables in both directions before
starting either microkernel.

All cases run over the same loss-free network path. Use `KV_CONNECTIONS` and
`PENDING_MESSAGES` together to increase the number of in-flight KV requests so
flow-control, ACK, reliability, and congestion-control paths execute under
load. ACK coalescing is disabled in case4.

The runner also removes flow-director rules and RSS contexts with `ethtool`
between cases, so repeated tests do not require rebooting the machines.

Example no-loss comparison:

```bash
KV_CONNECTIONS=128 PENDING_MESSAGES=128 KEY_COUNT=1000 REQUEST_GAP_NS=0 \
WARMUP_SECONDS=1 RUN_SECONDS=3 COOLDOWN_SECONDS=1 \
./scripts/run_arpc_kv_cases.sh
```

Use multiple NIC/softirq queues and userspace workers with:

```bash
NIC_QUEUES=4 APP_THREADS=4 KV_CONNECTIONS=128 PENDING_MESSAGES=128 \
./scripts/run_arpc_kv_cases.sh
```

The current eTran socket/eventfd allocator becomes exhausted above roughly 128
total FlexKVS connections in this configuration; 256 and 512 connection runs
fail in `alloc_socket_fd`.

The workload that produced the clearest repeatable case1-to-case4 latency
increase uses 2 queues, 2 workers, 128 in-flight requests, and 1350-byte values:

```bash
./scripts/run_arpc_latency_ladder.sh
```

Use `CASE_ORDER="1 2 3 4 5"` to include the original `f26ef18` eTran TCP
microkernel as case5. In plots, case4 is labeled `+FC` and case5 is `TCP`.

Profile aggregate CPU busy percentage on both hosts during the measurement
window with:

```bash
./scripts/profile_arpc_total_cpu.sh
```

The profiler reads aggregate `/proc/stat` counters, so results include
userspace, kernel, IRQ, and softirq CPU time.

Latency and throughput workloads are stored separately:

```bash
# Latency: 2 queues/workers, 128 in-flight, 1350-byte values.
./scripts/run_arpc_latency_ladder.sh

# Throughput: 4 queues/workers, 64 connections per worker, 64-byte values.
./scripts/run_arpc_throughput.sh

# eTran-related total CPU while the throughput workload is saturated.
./scripts/profile_arpc_throughput_cpu.sh

# eTran-related total CPU at the same 0.9 Mops offered load.
./scripts/profile_arpc_fixed_rate_cpu.sh
```

The throughput profiler sums microkernel and FlexKVS process CPU time plus
IRQ/softirq time, and reports equivalent busy cores and host percentage.
The fixed-rate workload uses interrupt-driven microkernels (`-n`) and blocking
server epoll so CPU usage reflects packet processing rather than busy polling.
It fixes offered load, but total CPU is not guaranteed to increase
monotonically: loss detection, retransmission, and out-of-order recovery remain
cold paths on a loss-free, in-order network.

Because the microkernel and FlexKVS workers busy-poll, saturated runs consume
roughly the configured worker/queue core budget for every case. At saturation,
additional protocol cost normally appears as lower throughput or higher CPU
time per operation, not as a monotonic increase in total busy cores. A
monotonic CPU-percentage comparison requires a fixed offered request rate below
the slowest case rather than running each case at its own maximum throughput.

No packet loss or synthetic bookkeeping is injected. Case-to-case differences
should be evaluated across repeated randomized trials.

### Reference

```
@inproceedings{etran,
  title={{eTran: Extensible Kernel Transport with eBPF}},
  author={Chen, Zhongjie and Meng, Qingkai and Lao, ChonLam and Liu, Yifan and Ren, Fengyuan and Yu, Minlan and Zhou, Yang},
  booktitle={22nd USENIX Symposium on Networked Systems Design and Implementation (NSDI 25)},
  year={2025}
}
```

### Contact

Feel free to raise issues or contact us if you have any questions or suggestions. 
You can reach us at: Zhongjie Chen (chenzhjthu@gmail.com), Yang Zhou (yangzhou.rpc@gmail.com).