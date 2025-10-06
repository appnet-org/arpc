# RPC Elements Documentation

## Overview
RPC Elements are a middleware mechanism in the aRPC framework that allows for request and response processing at different stages of the RPC lifecycle. They provide a way to implement cross-cutting concerns like logging, metrics, authentication, and other middleware functionality.

## Core Concepts

### RPCElement Interface
The `RPCElement` interface defines the contract that all RPC elements must implement:

```go
type RPCElement interface {
    // ProcessRequest processes the request before it's sent to the server
    ProcessRequest(ctx context.Context, req *RPCRequest) (*RPCRequest, error)

    // ProcessResponse processes the response after it's received from the server
    ProcessResponse(ctx context.Context, resp *RPCResponse) (*RPCResponse, error)

    // Name returns the name of the RPC element
    Name() string
}
```

### Request and Response Types

#### RPCRequest
```go
type RPCRequest struct {
    ID          uint64 // Unique identifier for the request
    ServiceName string // Name of the service being called
    Method      string // Name of the method being called
    Payload     any    // RPC payload
}
```

#### RPCResponse
```go
type RPCResponse struct {
    ID     uint64
    Result any
    Error  error
}
```

## Element Chain

The `RPCElementChain` provides a way to compose multiple RPC elements into a processing pipeline:

```go
type RPCElementChain struct {
    elements []RPCElement
}
```

### Processing Flow

1. **Request Processing**:
   - Requests are processed through elements in forward order
   - Each element can modify the request before passing it to the next element
   - If any element returns an error, processing stops

2. **Response Processing**:
   - Responses are processed through elements in reverse order
   - Each element can modify the response before passing it to the next element
   - If any element returns an error, processing stops

## Example Implementation

Here's an example of a metrics element implementation:

```go
type MetricsElement struct {
    requestCount uint64
    ctx          context.Context
    cancel       context.CancelFunc
}

func (m *MetricsElement) ProcessRequest(ctx context.Context, req *RPCRequest) (*RPCRequest, error) {
    atomic.AddUint64(&m.requestCount, 1)
    return req, nil
}

func (m *MetricsElement) ProcessResponse(ctx context.Context, resp *RPCResponse) (*RPCResponse, error) {
    if resp.Error != nil {
        m.cancel() // Stop metrics on error
    }
    return resp, nil
}

func (m *MetricsElement) Name() string {
    return "metrics"
}
```

## Usage

### Creating an Element Chain

```go
// Create individual elements
metrics := NewMetricsElement()
logging := NewLoggingElement(transport.RoleClient, log.New(os.Stdout, "aRPC: ", log.LstdFlags))

// Create a chain with multiple elements
rpcElements := []element.RPCElement{
    metrics,
    logging,
}

// Create a new element chain
chain := element.NewRPCElementChain(rpcElements...)
```

### Using with Client

```go
// Create client with RPC elements
client, err := rpc.NewClient(serializer, ":9000", transportElements, rpcElements)
```
