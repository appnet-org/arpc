# KV-Store Serialization Benchmark

Benchmark comparing Protobuf, FlatBuffers, and Cap'n Proto serialization performance for KV-store operations using real trace data.

## Prerequisites

- Go 1.24+
- `protoc` (for Protobuf)
- `flatc` (for FlatBuffers)
- `capnp` and `capnpc-go` plugin (for Cap'n Proto)

## Setup

Generate code for all serialization formats:

```bash
# Protobuf
protoc --go_out=paths=source_relative:. proto/kv.proto

# FlatBuffers
cd flatbuffers && flatc --go kv.fbs && cd ..

# Cap'n Proto (ensure capnpc-go is in PATH)
export PATH=$PATH:$GOPATH/bin
capnp compile -I$(go list -f '{{.Dir}}' capnproto.org/go/capnp/v3)/std -ogo capnp/kv.capnp
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
- `BenchmarkFlatBuffers_Write` / `BenchmarkFlatBuffers_Read`
- `BenchmarkCapnp_Write` / `BenchmarkCapnp_Read`

Benchmarks use trace data from `benchmark/meta-kv-trace/trace_large.req` with GET and SET operations.

