# CPU Performance Testing

This directory contains minimal performance test files for each serialization format, designed for use with `perf` and other low-level performance analysis tools.

## Test Files

- `protobuf_perf.go` - Minimal Protobuf marshal/unmarshal test
- `symphony_perf.go` - Minimal Symphony marshal/unmarshal test (with optimized getters)
- `capnp_perf.go` - Minimal Cap'n Proto marshal/unmarshal test  
- `flatbuffers_perf.go` - Minimal FlatBuffers marshal/unmarshal test

## Usage with perf

Each test file performs a single marshal and unmarshal operation. Use `perf` to run multiple iterations and collect statistics:

```bash
cd cpu-perf

# Test Protobuf
sudo taskset -c 3 perf stat -r 1000 -e cycles,instructions,branches,branch-misses,cache-misses -- go run protobuf_perf.go

# Test Symphony (with optimized direct getters)
sudo taskset -c 3 perf stat -r 1000 -e cycles,instructions,branches,branch-misses,cache-misses -- go run symphony_perf.go

# Test Cap'n Proto
sudo taskset -c 3 perf stat -r 1000 -e cycles,instructions,branches,branch-misses,cache-misses -- go run capnp_perf.go

# Test FlatBuffers
sudo taskset -c 3 perf stat -r 1000 -e cycles,instructions,branches,branch-misses,cache-misses -- go run flatbuffers_perf.go
```

## Features

- **Minimal overhead**: Each test contains only the essential marshal/unmarshal code
- **Single iteration**: Perfect for `perf` statistical analysis
- **Isolated testing**: No cross-format interference
- **Optimized Symphony**: Uses hardcoded direct getter functions for maximum performance
- **Consistent test data**: All formats use identical test data for fair comparison

## Test Data

All tests use the same data:
- ID: 42
- Score: 300
- Username: "alice"
- Content: "hello world"
