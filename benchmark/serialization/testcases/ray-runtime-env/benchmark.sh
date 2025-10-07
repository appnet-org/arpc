#!/bin/bash

set -e
## First generate the proto files

# Then build the flatbuffers schema
cd flatbuffers
flatc --go benchmark.fbs
cd ..

# Then build the capnp files
capnp compile -I$(go list -f '{{.Dir}}' capnproto.org/go/capnp/v3)/std -ogo capnp/benchmark.capnp

# Then build the protobuf files
protoc --go_out=paths=source_relative:. protobuf/benchmark.proto

# Then build the symphony files
protoc --go_out=paths=source_relative:. --symphony_out=paths=source_relative:. symphony/benchmark.proto
goimports -w symphony/*.syn.go

go run main.go
