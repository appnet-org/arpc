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

## Example Run

```
(base) xzhu@h1:~/arpc/benchmark/serialization/online-boutique$ go test -bench=. -benchmem -benchtime=2s
Loaded 99848 payload entries
Pre-serialized 99848 messages
goos: linux
goarch: amd64
pkg: github.com/appnet-org/arpc/benchmark/serialization/online-boutique
cpu: Intel(R) Xeon(R) Gold 6142 CPU @ 2.60GHz
BenchmarkProtobuf_Write-64                727612              3205 ns/op            312026 msg/s             421 B/op          0 allocs/op
BenchmarkProtobuf_Read-64                 340344              5913 ns/op            169130 msg/s            1517 B/op         32 allocs/op
BenchmarkFlatBuffers_Write-64             380313              5839 ns/op            171259 msg/s            3236 B/op         14 allocs/op
BenchmarkFlatBuffers_Read-64              921793              2577 ns/op            387978 msg/s             209 B/op          4 allocs/op
BenchmarkCapnp_Write-64                   182682             11867 ns/op             84270 msg/s            2520 B/op          5 allocs/op
BenchmarkCapnp_Read-64                    262912              7924 ns/op            126198 msg/s             630 B/op         29 allocs/op
BenchmarkSymphony_Write-64               1000000              2079 ns/op            481109 msg/s            1220 B/op          4 allocs/op
BenchmarkSymphony_Read-64                 696314              2939 ns/op            340225 msg/s            1105 B/op         31 allocs/op
BenchmarkSymphonyHybrid_Write-64          582748              3796 ns/op            263412 msg/s             928 B/op          2 allocs/op
BenchmarkSymphonyHybrid_Read-64           369120              6090 ns/op            164213 msg/s            1692 B/op         35 allocs/op
PASS
ok      github.com/appnet-org/arpc/benchmark/serialization/online-boutique      85.421s
```