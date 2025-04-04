#!/bin/bash

set -ex

# Install necessary commands
sudo apt-get update
sudo apt-get install -y linux-tools-common linux-tools-generic linux-tools-`uname -r`

# Disable TurboBoost
# cat /sys/devices/system/cpu/intel_pstate/no_turbo
echo "1" | sudo tee /sys/devices/system/cpu/intel_pstate/no_turbo

# Disable CPU Frequency Scaling 
# cat /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor
echo "performance" | sudo tee /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor

# Disable CPU Idle State
# sudo cpupower frequency-info
sudo cpupower idle-set -D 0

# Disable address space randomization 
echo 0 | sudo tee /proc/sys/kernel/randomize_va_space


# Install wrk and wrk2
sudo apt-get install luarocks -y
sudo luarocks install luasocket

git clone https://github.com/wg/wrk.git
pushd wrk
make -j $(nproc)

popd

sudo apt-get install libssl-dev -y
sudo apt-get install libz-dev -y 

git clone https://github.com/giltene/wrk2.git
pushd wrk2
make -j $(nproc)

popd

set +ex