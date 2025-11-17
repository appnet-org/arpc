package element

import (
	"context"
)

// TCPRPCContext contains the full context for a TCP RPC request/response
// Similar to HTTP2RPCContext but for raw TCP data
type TCPRPCContext struct {
	// Data contains the TCP data chunk
	// Can be modified by elements
	Data []byte

	// ConnectionID is a unique identifier for the connection
	ConnectionID string

	// IsRequest indicates if this is a request (true) or response (false)
	IsRequest bool

	// RemoteAddr is the client's remote address
	RemoteAddr string

	// TargetAddr is the target server address
	TargetAddr string
}

// Verdict determines how the proxy should handle the RPC after processing
type Verdict int

const (
	// VerdictPass allows the RPC to continue processing (forward normally)
	VerdictPass Verdict = iota

	// VerdictDrop drops the RPC (do not forward)
	VerdictDrop
)

// String returns the string representation of Verdict
func (v Verdict) String() string {
	switch v {
	case VerdictPass:
		return "pass"
	case VerdictDrop:
		return "drop"
	}
	return "unknown"
}

// RPCElement defines the interface for RPC elements
// Similar to Envoy WASM filters, elements can access and modify data
type RPCElement interface {
	// ProcessRequest processes the request before it's sent to the server
	// The context can be modified in place (data)
	// Returns a verdict indicating whether to pass or drop the request
	ProcessRequest(ctx context.Context, rpcCtx *TCPRPCContext) (Verdict, context.Context, error)

	// ProcessResponse processes the response after it's received from the server
	// The context can be modified in place (data)
	// Returns a verdict indicating whether to pass or drop the response
	ProcessResponse(ctx context.Context, rpcCtx *TCPRPCContext) (Verdict, context.Context, error)

	// Name returns the name of the RPC element
	Name() string
}

// RPCElementChain represents a chain of RPC elements
type RPCElementChain struct {
	elements []RPCElement
}

// NewRPCElementChain creates a new chain of RPC elements
func NewRPCElementChain(elements ...RPCElement) *RPCElementChain {
	return &RPCElementChain{
		elements: elements,
	}
}

// ProcessRequest processes the request through all RPC elements in the chain
// Returns the verdict and modified context
func (c *RPCElementChain) ProcessRequest(ctx context.Context, rpcCtx *TCPRPCContext) (Verdict, context.Context, error) {
	var err error
	var verdict Verdict

	for _, element := range c.elements {
		verdict, ctx, err = element.ProcessRequest(ctx, rpcCtx)
		if err != nil {
			return VerdictPass, ctx, err
		}
		if verdict == VerdictDrop {
			return VerdictDrop, ctx, nil
		}
	}

	return VerdictPass, ctx, nil
}

// ProcessResponse processes the response through all RPC elements in reverse order
// Returns the verdict and modified context
func (c *RPCElementChain) ProcessResponse(ctx context.Context, rpcCtx *TCPRPCContext) (Verdict, context.Context, error) {
	var err error
	var verdict Verdict

	for i := len(c.elements) - 1; i >= 0; i-- {
		verdict, ctx, err = c.elements[i].ProcessResponse(ctx, rpcCtx)
		if err != nil {
			return VerdictPass, ctx, err
		}
		if verdict == VerdictDrop {
			return VerdictDrop, ctx, nil
		}
	}

	return VerdictPass, ctx, nil
}

