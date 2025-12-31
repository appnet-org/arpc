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

### Go
Make sure you have Go installed (1.20 or later recommended).

### Protocol Buffers Compiler (`protoc`)
You need `protoc` version 29.0 or later to ensure generated code uses modern `google.golang.org/protobuf` imports instead of the deprecated `github.com/golang/protobuf` package.

**Install on Linux:**
```bash
# Download the latest protoc
PROTOC_VERSION=29.3
wget https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-linux-x86_64.zip

# Extract and install
unzip protoc-${PROTOC_VERSION}-linux-x86_64.zip -d protoc-${PROTOC_VERSION}
sudo cp protoc-${PROTOC_VERSION}/bin/protoc /usr/local/bin/
sudo cp -r protoc-${PROTOC_VERSION}/include/google /usr/local/include/
sudo chmod +x /usr/local/bin/protoc

# Verify installation
protoc --version  # Should show libprotoc 29.3 or later
```

### Go Protocol Buffers Plugin (`protoc-gen-go`)
Install the latest `protoc-gen-go` plugin:

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
```

Verify installation:
```bash
protoc-gen-go --version  # Should show v1.36 or later
```

Make sure `$GOBIN` or `$GOPATH/bin` is in your `PATH` so `protoc` can find the plugin 
