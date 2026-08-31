#!/bin/bash
set -euo pipefail

export ARPC_WORKLOAD_SCRIPT=run_arpc_fixed_rate.sh
export TARGET_MOPS=${TARGET_MOPS:-0.9}
export CPU_SAMPLE_SECONDS=${CPU_SAMPLE_SECONDS:-10}

exec "$(dirname "$0")/profile_arpc_throughput_cpu.sh"
