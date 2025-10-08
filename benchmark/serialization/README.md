# Serialization Benchmark

This benchmark compares the performance of different serialization formats: Protobuf, Symphony, Cap'n Proto, and FlatBuffers.

## Schema

All serialization formats use the same message structure:

```proto
message BenchmarkMessage {
  int32 id = 1;
  int32 score = 2;
  string username = 3;
  string content = 4;
}
```

## Metrics Tested

The benchmark measures the following performance metrics for each serialization format:

### 1. Marshal (Serialization) Performance
- **Time per operation**: Nanoseconds per marshal operation
- **Memory allocations**: Number of allocations per operation
- **Memory usage**: Bytes allocated per operation
- **CPU profiling**: CPU usage patterns during marshaling

### 2. Unmarshal (Deserialization) Performance
- **Time per operation**: Nanoseconds per unmarshal operation
- **Memory allocations**: Number of allocations per operation
- **Memory usage**: Bytes allocated per operation
- **CPU profiling**: CPU usage patterns during unmarshaling

### 3. Serialized Data Size
- **Bytes**: Size of the serialized message in bytes

## Serialization Formats

1. **Protobuf** - Google's Protocol Buffers
2. **Symphony** - Custom serialization format (part of aRPC)
3. **Cap'n Proto** - Infinitely fast serialization
4. **FlatBuffers** - Google's zero-copy serialization library

## Test Data

The benchmark uses the following test data:
- ID: 42
- Score: 300
- Username: "alice"
- Content: "hello world"

## Running the Benchmark

Execute the benchmark script from inside a particular testcase directory:

```bash
./benchmark.sh
```

This script will:
1. Generate code for all serialization formats
2. Build the Go module dependencies
3. Run the benchmark tests
4. Generate performance measurements and CPU profiles

## Output

The benchmark generates several output files in the `results/` directory:

### Timing Results
- `marshal_timing_results.log` - Marshal performance measurements
- `unmarshal_timing_results.log` - Unmarshal performance measurements

### CPU Profiles
- `protobuf_marshal.prof` - Protobuf marshal CPU profile
- `symphony_marshal.prof` - Symphony marshal CPU profile
- `capnproto_marshal.prof` - Cap'n Proto marshal CPU profile
- `flatbuffers_marshal.prof` - FlatBuffers marshal CPU profile
- `protobuf_unmarshal.prof` - Protobuf unmarshal CPU profile
- `symphony_unmarshal.prof` - Symphony unmarshal CPU profile
- `capnproto_unmarshal.prof` - Cap'n Proto unmarshal CPU profile
- `flatbuffers_unmarshal.prof` - FlatBuffers unmarshal CPU profile

## Analyzing CPU Profiles

Use Go's pprof tool to analyze the CPU profiles:

```bash
go tool pprof results/protobuf_marshal.prof
go tool pprof results/symphony_marshal.prof
go tool pprof results/capnproto_marshal.prof
go tool pprof results/flatbuffers_marshal.prof
```

Common pprof commands:
- `top` - Show top functions by CPU usage
- `list <function>` - Show source code for a function
- `web` - Open web interface
- `png` - Generate PNG visualization

## Prerequisites

- Go 1.24 or later
- flatc (FlatBuffers compiler)
- capnp (Cap'n Proto compiler)
- protoc (Protocol Buffers compiler)
- protoc-gen-go (Go plugin for protoc)
- protoc-gen-symphony (Symphony plugin for protoc)
