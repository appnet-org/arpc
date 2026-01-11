# KV-Store Serialization Benchmark

Benchmark comparing Protobuf, FlatBuffers, and Cap'n Proto serialization performance for KV-store operations using real trace data.

## Prerequisites

- Go 1.24+
- `protoc` (for Protobuf)
- `flatc` (for FlatBuffers)
- `capnp` and `capnpc-go` plugin (for Cap'n Proto)
- `protoc-gen-symphony` (for Symphony)

## Setup

Generate code for all serialization formats:

```bash
# Install protoc-gen-symphony (if not already installed)
go install github.com/appnet-org/arpc/cmd/symphony-gen-arpc/protoc-gen-symphony

# Protobuf and Symphony
protoc --symphony_out=paths=source_relative:. \
       --symphony-hybrid_out=paths=source_relative:. \
       --go_out=paths=source_relative:. \ 
       proto/onlineboutique.proto

# FlatBuffers
cd flatbuffers && flatc --go onlineboutique.fbs && cd ..

# Cap'n Proto (ensure capnpc-go is in PATH)
export PATH=$PATH:$GOPATH/bin
capnp compile -I$(go list -f '{{.Dir}}' capnproto.org/go/capnp/v3)/std -ogo capnp/onlineboutique.capnp
```

## Running Benchmarks

Run all benchmarks:

```bash
go test -bench=. -benchmem -benchtime=2s
```

Run specific benchmark:

```bash
go test -bench=BenchmarkProtobuf_Write -benchmem
go test -bench=BenchmarkCapnp_Read -benchmem
```

## Benchmarks

- `BenchmarkProtobuf_Write` / `BenchmarkProtobuf_Read`
- `BenchmarkSymphony_Write` / `BenchmarkSymphony_Read` (non-zero-copy)
- `BenchmarkSymphony_Write_ZeroCopy` / `BenchmarkSymphony_Read_ZeroCopy` (zero-copy using Raw types)
- `BenchmarkFlatBuffers_Write` / `BenchmarkFlatBuffers_Read`
- `BenchmarkCapnp_Write` / `BenchmarkCapnp_Read`

