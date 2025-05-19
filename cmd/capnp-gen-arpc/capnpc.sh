#!/bin/bash

set -ex

if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <argument>"
    exit 1
fi

CAPNP_FILE=$1
CAPNP_DIR=$(dirname "$CAPNP_FILE")
CAPNP_FILENAME=$(basename "$CAPNP_FILE")

# Always build the capnpc_arpc binary for now
go build -o capnpc_arpc .

./capnpc_arpc $1

cd $CAPNP_DIR
capnp compile -I$(go list -f '{{.Dir}}' capnproto.org/go/capnp/v3)/std -ogo $CAPNP_FILENAME

set +ex