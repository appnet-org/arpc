#!/bin/bash
set -euo pipefail

app_pid=$(cat "$1")
test -r "/proc/$app_pid/stat"

process_ticks=$(awk '{print $14 + $15}' "/proc/$app_pid/stat")
cgroup=$(systemctl show etran-microkernel.service --property=ControlGroup --value)
for unit_pid in $(cat "/sys/fs/cgroup${cgroup}/cgroup.procs"); do
    if test -r "/proc/$unit_pid/stat"; then
        ticks=$(awk '{print $14 + $15}' "/proc/$unit_pid/stat")
        process_ticks=$((process_ticks + ticks))
    fi
done

irq_ticks=$(awk '/^cpu / {print $7 + $8; exit}' /proc/stat)
printf "%s %s %s %s %s\n" \
    "$process_ticks" "$irq_ticks" "$(getconf CLK_TCK)" "$(nproc)" \
    "$(date +%s%N)"
