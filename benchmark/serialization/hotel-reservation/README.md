# Hotel Reservation Serialization Benchmark

Benchmark comparing Protobuf, FlatBuffers, and Cap'n Proto serialization performance for hotel reservation operations using generated payloads.

## Prerequisites

- Go 1.24+
- `protoc` (for Protobuf)
- `flatc` (for FlatBuffers)
- `capnp` and `capnpc-go` plugin (for Cap'n Proto)
- `protoc-gen-symphony` and `protoc-gen-symphony-hybrid` (for Symphony)

## Setup

Generate code for all serialization formats:

```bash
# Install Symphony protoc plugins (if not already installed)
go install github.com/appnet-org/arpc/cmd/symphony-gen-arpc/protoc-gen-symphony
go install github.com/appnet-org/arpc/cmd/symphony-gen-arpc/protoc-gen-symphony-hybrid

# Protobuf and Symphony (from benchmark/serialization/hotel-reservation)
protoc --symphony_out=paths=source_relative:. \
       --symphony-hybrid_out=paths=source_relative:. \
       --go_out=paths=source_relative:. \
       proto/hotel_reservation.proto

# FlatBuffers
cd flatbuffers && flatc --go hotel_reservation.fbs && cd ..

# Cap'n Proto (ensure capnpc-go is in PATH)
export PATH=$PATH:$(go env GOPATH)/bin
capnp compile -I$(go list -f '{{.Dir}}' capnproto.org/go/capnp/v3)/std -ogo capnp/hotel_reservation.capnp
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
- `BenchmarkSymphony_Write` / `BenchmarkSymphony_Read`
- `BenchmarkSymphonyHybrid_Write` / `BenchmarkSymphonyHybrid_Read`
- `BenchmarkFlatBuffers_Write` / `BenchmarkFlatBuffers_Read`
- `BenchmarkCapnp_Write` / `BenchmarkCapnp_Read`

## Payloads

Generate JSONL payloads (100k total) with the payload generator:

```bash
cd payload_generator && python main.py && cd ..
```

## Plot CDF

After running benchmarks, plot latency CDFs:

```bash
python plot_latency_cdf.py
python plot_latency_cdf.py --include-hybrid
```

Output: `hotel_reservation_serialization_latency_cdf.pdf` (and `_hybrid.pdf` when using `--include-hybrid`).

## Example Run

```
[aruj@h1 hotel-reservation]$ go test -bench=. -benchmem -benchtime=2s
Loaded 100000 payload entries
Pre-serialized 100000 messages
goos: linux
goarch: amd64
pkg: github.com/appnet-org/arpc/benchmark/serialization/hotel-reservation
cpu: Intel(R) Xeon(R) Gold 6142 CPU @ 2.60GHz
BenchmarkProtobuf_Write-64               1000000              2465 ns/op            405677 msg/s             338 B/op          0 allocs/op
BenchmarkProtobuf_Read-64                1000000              4021 ns/op            248667 msg/s             932 B/op         21 allocs/op
BenchmarkFlatBuffers_Write-64            1000000              3584 ns/op            279019 msg/s            2332 B/op         10 allocs/op
BenchmarkFlatBuffers_Read-64             1604374              1370 ns/op            730144 msg/s             216 B/op          3 allocs/op
BenchmarkCapnp_Write-64                   416602              8026 ns/op            124592 msg/s            2078 B/op          5 allocs/op
BenchmarkCapnp_Read-64                    752450              5246 ns/op            190613 msg/s             576 B/op         17 allocs/op
BenchmarkSymphony_Write-64               1686945              1377 ns/op            726430 msg/s            1134 B/op          4 allocs/op
BenchmarkSymphony_Read-64                1000000              2077 ns/op            481407 msg/s             821 B/op         18 allocs/op
BenchmarkSymphonyHybrid_Write-64         1000000              2845 ns/op            351523 msg/s             773 B/op          2 allocs/op
BenchmarkSymphonyHybrid_Read-64          1000000              4131 ns/op            242051 msg/s            1016 B/op         22 allocs/op
PASS
ok      github.com/appnet-org/arpc/benchmark/serialization/hotel-reservation    114.865s
```
