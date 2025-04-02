# AppNet RPC (aRPC)

**AppNet RPC (arpc)** is a minimal, fast, and pluggable Remote Procedure Call framework built on top of **UDP**, with support for customizable serialization formats.

## Installation

Make sure you have `protoc` installed.

Then install the Go protobuf compiler and aRPC plugin:

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install ./cmd/protoc-gen-aprc  # From this repo
```

Ensure your `$PATH` includes `$GOPATH/bin` or `$HOME/go/bin` so `protoc` can find the plugins.

## Example 

See [examples/README.md](examples/README.md)

## Repo Structure

```
.
├── cmd
│   └── protoc-gen-aprc       # aRPC plugin for protoc
├── examples
│   └── echo                  # Echo service with client + server
├── go.mod
├── go.sum
├── internal
│   ├── protocol              # Fragmentation and framing
│   ├── serializer            # Serializer interface + impls
│   └── transport             # UDP socket logic
├── pkg
│   └── rpc                   # Core aRPC client/server framework
├── proto
│   └── echo.proto            # Sample protobuf definition
├── README.md
```


## Contact

If you have any questions or comments, please get in touch with Xiangfeng Zhu (xfzhu@cs.washington.edu).

