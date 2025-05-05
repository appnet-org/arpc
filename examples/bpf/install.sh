#!/bin/bash

set -ex

# Install dependencies
sudo apt update
sudo apt install -y zip bison build-essential cmake flex git libedit-dev   libllvm12 llvm-12-dev libclang-12-dev python zlib1g-dev libelf-dev libfl-dev python3-setuptools   liblzma-dev arping iperf

# Clone and build BCC
git clone https://github.com/iovisor/bcc.git
mkdir bcc/build; cd bcc/build
cmake ..
make -j $(nproc)
sudo make install

# Build and install Python bindings
cmake -DPYTHON_CMD=python3 ..
pushd src/python/
make -j $(nproc)
sudo make install
popd

set +ex