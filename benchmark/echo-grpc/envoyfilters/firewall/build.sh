#!/usr/bin/env bash

WORKDIR=`dirname $(realpath $0)`
cd $WORKDIR

cargo build --target=wasm32-wasip1 --release
cp target/wasm32-wasip1/release/firewall.wasm /tmp/appnet

