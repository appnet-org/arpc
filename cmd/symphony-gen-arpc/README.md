# Symphony and aRPC Code Generation

This repository provides two custom `protoc` plugins for generating optimized RPC and packet layout code:
- `protoc-gen-arpc`: Generates aRPC-compatible stubs for efficient, minimal RPC communication.
- `protoc-gen-symphony`: Generates optimized packet layouts compatible with the Symphony runtime.

## Installation

To install the code generators, run:

```bash
go install ./protoc-gen-arpc
go install ./protoc-gen-symphony
````

This will build and install the plugins into your `$GOBIN` directory. Make sure `$GOBIN` is in your `PATH`.

## Code Generation

To generate code from your `.proto` file using both the Symphony and aRPC plugins:

```bash
protoc --symphony_out=paths=source_relative:. \
       --arpc_out=paths=source_relative:. \
       --go_out=paths=source_relative:. \
       kv.proto
```

This command will generate:

* `<your-proto-file>.pb.go`: The standard Protobuf struct and getter/setter methods.
* `<your-proto-file>.syn.go`: Contains the optimized field layout and serialization logic used by Symphony.
* `<your-proto-file>_arpc.syn.go`: Contains aRPC client/server stubs for RPC handling.


## Requirements

* Go 
* `protoc` (Protocol Buffers compiler) 
