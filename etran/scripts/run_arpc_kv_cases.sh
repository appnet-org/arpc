#!/bin/bash
set -euo pipefail

server=${SERVER_HOST:-chenzj@hp053.utah.cloudlab.us}
client=${CLIENT_HOST:-chenzj@hp050.utah.cloudlab.us}
server_ip=${SERVER_IP:-192.168.6.1}
client_ip=${CLIENT_IP:-192.168.6.2}
warmup=${WARMUP_SECONDS:-2}
duration=${RUN_SECONDS:-5}
cooldown=${COOLDOWN_SECONDS:-1}
request_gap=${REQUEST_GAP_NS:-50000}
value_size=${VALUE_SIZE:-1024}
key_count=${KEY_COUNT:-1000}
pending=${PENDING_MESSAGES:-1}
connections=${KV_CONNECTIONS:-1}
nic_queues=${NIC_QUEUES:-1}
app_threads=${APP_THREADS:-1}
target_mops=${TARGET_MOPS:-0}
microkernel_extra_args=${MICROKERNEL_EXTRA_ARGS:-}
server_epoll_timeout=${SERVER_EPOLL_TIMEOUT_MS:-0}
cases=${CASE_ORDER:-"1 2 3 4"}

cleanup()
{
    ssh "$server" '
        if test -s /tmp/arpc-kv-server.pid; then
            pid=$(cat /tmp/arpc-kv-server.pid)
            if kill -0 "$pid" 2>/dev/null; then kill "$pid"; fi
        fi
        sleep 1
        sudo systemctl stop etran-microkernel.service 2>/dev/null || true
        sudo systemctl reset-failed etran-microkernel.service 2>/dev/null || true
        sudo ip link set dev ens1f1np1 xdp off 2>/dev/null || true
        for rule in $(sudo ethtool -n ens1f1np1 2>/dev/null |
                sed -n "s/^Filter: \([0-9][0-9]*\)$/\1/p"); do
            sudo ethtool -U ens1f1np1 delete "$rule" 2>/dev/null || true
        done
        for context in $(sudo grep -h "Created RSS context:" \
                /tmp/arpc-case*-microkernel.log 2>/dev/null |
                sed -n "s/.*Created RSS context: \([0-9][0-9]*\).*/\1/p" |
                sort -nu); do
            sudo ethtool -X ens1f1np1 context "$context" delete 2>/dev/null || true
        done
    ' >/dev/null 2>&1 || true
    ssh "$client" '
        if test -s /tmp/arpc-kv-client.pid; then
            pid=$(cat /tmp/arpc-kv-client.pid)
            if kill -0 "$pid" 2>/dev/null; then kill "$pid"; fi
        fi
        sleep 1
        sudo systemctl stop etran-microkernel.service 2>/dev/null || true
        sudo systemctl reset-failed etran-microkernel.service 2>/dev/null || true
        sudo ip link set dev ens1f1np1 xdp off 2>/dev/null || true
        for rule in $(sudo ethtool -n ens1f1np1 2>/dev/null |
                sed -n "s/^Filter: \([0-9][0-9]*\)$/\1/p"); do
            sudo ethtool -U ens1f1np1 delete "$rule" 2>/dev/null || true
        done
        for context in $(sudo grep -h "Created RSS context:" \
                /tmp/arpc-case*-microkernel.log 2>/dev/null |
                sed -n "s/.*Created RSS context: \([0-9][0-9]*\).*/\1/p" |
                sort -nu); do
            sudo ethtool -X ens1f1np1 context "$context" delete 2>/dev/null || true
        done
    ' >/dev/null 2>&1 || true
}

trap cleanup EXIT

for host in "$server" "$client"; do
    ssh "$host" '
        if command -v cpufreq-set >/dev/null; then
            for cpu in /sys/devices/system/cpu/cpu[0-9]*; do
                sudo cpufreq-set -c "${cpu##*cpu}" -g performance 2>/dev/null || true
            done
        fi
    '
done

for arpc_case in $cases; do
    echo "===== ARPC case $arpc_case ====="
    cleanup

    ssh "$server" "ping -c 3 -W 2 $client_ip >/dev/null"
    ssh "$client" "ping -c 3 -W 2 $server_ip >/dev/null"

    microkernel_dir=/users/chenzj/eTran/eTran/micro_kernel
    microkernel_args="-q ${nic_queues} -i ens1f1np1 -a ${arpc_case} ${microkernel_extra_args}"
    if [ "$arpc_case" = 5 ]; then
        microkernel_dir=/users/chenzj/eTran-original/eTran/micro_kernel
        microkernel_args="-q ${nic_queues} -i ens1f1np1 ${microkernel_extra_args}"
    fi

    for host in "$server" "$client"; do
        ssh "$host" "
            sudo rm -f /tmp/arpc-case${arpc_case}-microkernel.log
            sudo systemd-run --unit=etran-microkernel \
                --property=WorkingDirectory=${microkernel_dir} \
                --property=StandardOutput=append:/tmp/arpc-case${arpc_case}-microkernel.log \
                --property=StandardError=append:/tmp/arpc-case${arpc_case}-microkernel.log \
                bash -lc 'tail -f /dev/null | ./micro_kernel ${microkernel_args}'
        " >/dev/null
    done

    sleep 4

    ssh "$server" "
        cd ~/eTran/eTran/tcp_app
        rm -f /tmp/arpc-case${arpc_case}-server.log
        nohup env ETRAN_PROTO=tcp ETRAN_NR_APP_THREADS=$app_threads \
            ETRAN_NR_NIC_QUEUES=$nic_queues \
            ETRAN_EPOLL_TIMEOUT_MS=$server_epoll_timeout \
            LD_PRELOAD=../shared_lib/libetran.so \
            ./flexkvs_server unused.conf $app_threads $nic_queues \
            >/tmp/arpc-case${arpc_case}-server.log 2>&1 </dev/null &
        echo \$! >/tmp/arpc-kv-server.pid
    "

    sleep 3

    target_rate_arg=
    if awk -v rate="$target_mops" 'BEGIN {exit !(rate > 0)}'; then
        target_rate_arg="-R $target_mops"
    fi

    ssh "$client" "
        cd ~/eTran/eTran/tcp_app
        rm -f /tmp/arpc-case${arpc_case}-client.log \
            /tmp/arpc-case${arpc_case}-measuring /tmp/arpc-kv-client.pid
        env ETRAN_PROTO=tcp ETRAN_NR_APP_THREADS=$app_threads \
            ETRAN_NR_NIC_QUEUES=$nic_queues \
            ETRAN_MEASURE_MARKER=/tmp/arpc-case${arpc_case}-measuring \
            LD_PRELOAD=../shared_lib/libetran.so \
            ./flexkvs_bench -q $nic_queues -t $app_threads \
            -C $connections -p $pending \
            -n $key_count -g 1 $target_rate_arg \
            -v $value_size -d $request_gap -w $warmup -T $duration -c $cooldown \
            $server_ip:11211 \
            >/tmp/arpc-case${arpc_case}-client.log 2>&1 &
        pid=\$!
        echo \$pid >/tmp/arpc-kv-client.pid
        timeout_seconds=\$(( $warmup + $duration + $cooldown + 20 ))
        if timeout \$timeout_seconds tail --pid=\$pid -f /dev/null; then
            wait \$pid
        else
            kill \$pid 2>/dev/null || true
            wait \$pid 2>/dev/null || true
            cat /tmp/arpc-case${arpc_case}-client.log
            exit 124
        fi
        cat /tmp/arpc-case${arpc_case}-client.log
    "
done
