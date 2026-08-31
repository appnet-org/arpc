#!/bin/bash
set -euo pipefail

server=${SERVER_HOST:-chenzj@hp053.utah.cloudlab.us}
client=${CLIENT_HOST:-chenzj@hp050.utah.cloudlab.us}
sample_seconds=${CPU_SAMPLE_SECONDS:-10}
output=${CPU_PROFILE_OUTPUT:-/tmp/arpc-throughput-cpu.tsv}
cases=${CASE_ORDER:-"1 2 3 4"}
workload_script=${ARPC_WORKLOAD_SCRIPT:-run_arpc_throughput.sh}

read_usage()
{
    local host=$1
    local app_pid_file=$2
    ssh "$host" "~/eTran/scripts/arpc_cpu_snapshot.sh $app_pid_file"
}

calculate_usage()
{
    awk -v before="$1" -v after="$2" '
        BEGIN {
            split(before, b);
            split(after, a);
            seconds = (a[5] - b[5]) / 1000000000;
            process_cores = (a[1] - b[1]) / a[3] / seconds;
            irq_cores = (a[2] - b[2]) / a[3] / seconds;
            total_cores = process_cores + irq_cores;
            host_pct = 100 * total_cores / a[4];
            printf "%.2f %.2f\n", total_cores, host_pct;
        }
    '
}

printf "case\tserver_cores\tserver_pct\tclient_cores\tclient_pct\n" >"$output"

for arpc_case in $cases; do
    marker_seen=0
    for attempt in 1 2 3; do
        rm -f "/tmp/arpc-throughput-profile-case${arpc_case}.log"
        ssh "$client" \
            "rm -f /tmp/arpc-case${arpc_case}-client.log \
                /tmp/arpc-case${arpc_case}-measuring /tmp/arpc-kv-client.pid"

        CASE_ORDER="$arpc_case" \
            "$(dirname "$0")/$workload_script" \
            >"/tmp/arpc-throughput-profile-case${arpc_case}.log" 2>&1 &
        runner_pid=$!

        for _ in $(seq 1 60); do
            if ssh "$client" "
                test -s /tmp/arpc-kv-client.pid &&
                pid=\$(cat /tmp/arpc-kv-client.pid) &&
                kill -0 \$pid 2>/dev/null &&
                test -e /tmp/arpc-case${arpc_case}-measuring
            "; then
                marker_seen=1
                break
            fi
            if ! kill -0 "$runner_pid" 2>/dev/null; then
                wait "$runner_pid" 2>/dev/null || true
                break
            fi
            sleep 1
        done

        test "$marker_seen" = 1 && break
        kill "$runner_pid" 2>/dev/null || true
        wait "$runner_pid" 2>/dev/null || true
        sleep 2
    done

    if test "$marker_seen" != 1; then
        echo "Failed to start case $arpc_case after 3 attempts" >&2
        exit 1
    fi

    read_usage "$server" /tmp/arpc-kv-server.pid >/tmp/arpc-server-before &
    server_snapshot_pid=$!
    read_usage "$client" /tmp/arpc-kv-client.pid >/tmp/arpc-client-before &
    client_snapshot_pid=$!
    wait "$server_snapshot_pid"
    wait "$client_snapshot_pid"
    server_before=$(cat /tmp/arpc-server-before)
    client_before=$(cat /tmp/arpc-client-before)

    sleep "$sample_seconds"

    read_usage "$server" /tmp/arpc-kv-server.pid >/tmp/arpc-server-after &
    server_snapshot_pid=$!
    read_usage "$client" /tmp/arpc-kv-client.pid >/tmp/arpc-client-after &
    client_snapshot_pid=$!
    wait "$server_snapshot_pid"
    wait "$client_snapshot_pid"
    server_after=$(cat /tmp/arpc-server-after)
    client_after=$(cat /tmp/arpc-client-after)
    wait "$runner_pid" || true

    read -r server_cores server_pct < <(
        calculate_usage "$server_before" "$server_after")
    read -r client_cores client_pct < <(
        calculate_usage "$client_before" "$client_after")

    printf "%s\t%s\t%s\t%s\t%s\n" \
        "$arpc_case" "$server_cores" "$server_pct" \
        "$client_cores" "$client_pct" | tee -a "$output"
done

cat "$output"
