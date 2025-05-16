# Echo Examples

This directory contains two example implementations of a simple echo service using different IDL formats:

## Prerequisites

- Go 1.20 or later
    - For installation instructions, see Go’s [Getting Started](https://go.dev/doc/install) guide.
- For Cap'n Proto example setup, see [capnp-gen-arpc documentation](cmd/capnp-gen-arpc/README.md)
- For Protocol Buffers example setup, see [protoc-gen-arpc documentation](cmd/protoc-gen-arpc/README.md)


## Echo Cap'n Proto Example (`echo_capnp/`)

A simple echo service implementation using Cap'n Proto as the IDL format. The service accepts text content and echoes it back.

### Quick Start
1. Define service in `echo.capnp`
2. Generate Go code using `capnp-gen-arpc`
3. Run server: `go run server/server.go`
4. Run client: `go run frontend/frontend.go`
5. Test: `curl http://localhost:8080?key=hello`

## Echo Protocol Buffers Example (`echo_proto/`)

A simple echo service implementation using Protocol Buffers as the IDL format. The service accepts a message and echoes it back.

### Quick Start
1. Define service in `echo.proto`
2. Generate Go code: `protoc --go_out=. --arpc_out=. echo/proto/echo.proto`
3. Run server: `go run server/server.go`
4. Run client: `go run frontend/frontend.go`
5. Test: `curl http://localhost:8080?key=hello`

Both examples demonstrate how to:
- Define service interfaces using different IDL formats
- Generate client/server code
- Implement and run a basic RPC service
- Test the service using HTTP endpoints
