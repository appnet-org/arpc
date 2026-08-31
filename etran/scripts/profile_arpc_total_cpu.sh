#!/bin/bash
set -euo pipefail

server=${SERVER_HOST:-chenzj@hp053.utah.cloudlab.us}
client=${CLIENT_HOST:-chenzj@hp050.utah.cloudlab.us}
duration=${RUN_SECONDS:-5}
output=${CPU_PROFILE_OUTPUT:-/tmp/arpc-total-cpu.tsv}
cases=${CASE_ORDER:-"1 2 3 4"}

read_cpu()
{
    ssh "$1" "head -1 /proc/stat"
}

busy_percent()
{
    awk -v before="$1" -v after="$2" '
        BEGIN {
            split(before, b);
            split(after, a);
            bt = at = 0;
            for (i = 2; i <= 11; i++) {
                bt += b[i];
                at += a[i];
            }
            bidle = b[5] + b[6];
            aidle = a[5] + a[6];
            total = at - bt;
            idle = aidle - bidle;
            printf "%.2f", 100 * (total - idle) / total;
        }
    '
}

printf "case\tserver_busy_pct\tclient_busy_pct\n" >"$output"

for arpc_case in $cases; do
    rm -f "/tmp/arpc-profile-case${arpc_case}.log"
    ssh "$client" "rm -f /tmp/arpc-case${arpc_case}-client.log"
    CASE_ORDER="$arpc_case" \
        "$(dirname "$0")/run_arpc_latency_ladder.sh" \
        >"/tmp/arpc-profile-case${arpc_case}.log" 2>&1 &
    runner_pid=$!

    for _ in $(seq 1 60); do
        if ssh "$client" \
            "grep -q 'Warmup completed; measuring' /tmp/arpc-case${arpc_case}-client.log 2>/dev/null"; then
            break
        fi
        if ! kill -0 "$runner_pid" 2>/dev/null; then
            wait "$runner_pid"
            exit 1
        fi
        sleep 1
    done

    server_before=$(read_cpu "$server")
    client_before=$(read_cpu "$client")
    sleep "$duration"
    server_after=$(read_cpu "$server")
    client_after=$(read_cpu "$client")

    wait "$runner_pid"

    server_busy=$(busy_percent "$server_before" "$server_after")
    client_busy=$(busy_percent "$client_before" "$client_after")
    printf "%s\t%s\t%s\n" "$arpc_case" "$server_busy" "$client_busy" |
        tee -a "$output"
done

cat "$output"
