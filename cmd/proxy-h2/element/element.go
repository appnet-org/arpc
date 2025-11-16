package element

import (
	"context"
	"strings"
)

// Headers represents HTTP/2 headers as a map from header name to values
// Header names are stored in lowercase (HTTP/2 requirement)
type Headers map[string][]string

// HTTP2RPCContext contains the full context for an HTTP/2 RPC request/response
// Similar to Envoy WASM's context, providing access to headers, payload, and metadata
type HTTP2RPCContext struct {
	// Headers contains all HTTP/2 headers for this stream
	// Can be modified by elements (modifications will be re-encoded when forwarding)
	Headers Headers

	// Payload contains the gRPC message payload (data from DATA frames)
	// Can be modified by elements
	Payload []byte

	// StreamID is the HTTP/2 stream ID
	StreamID uint32

	// IsRequest indicates if this is a request (true) or response (false)
	IsRequest bool

	// RemoteAddr is the client's remote address
	RemoteAddr string

	// Path is extracted from the :path pseudo-header (if available)
	Path string

	// Method is extracted from the :method pseudo-header (if available)
	Method string

	// Authority is extracted from the :authority pseudo-header (if available)
	Authority string
}

// GetHeader returns the first value of a header, or empty string if not found
func (c *HTTP2RPCContext) GetHeader(name string) string {
	values := c.Headers[strings.ToLower(name)]
	if len(values) > 0 {
		return values[0]
	}
	return ""
}

// GetHeaderValues returns all values for a header
func (c *HTTP2RPCContext) GetHeaderValues(name string) []string {
	return c.Headers[strings.ToLower(name)]
}

// SetHeader sets a header value (replaces existing values)
func (c *HTTP2RPCContext) SetHeader(name, value string) {
	c.Headers[strings.ToLower(name)] = []string{value}
}

// AddHeader adds a header value (appends to existing values)
func (c *HTTP2RPCContext) AddHeader(name, value string) {
	name = strings.ToLower(name)
	c.Headers[name] = append(c.Headers[name], value)
}

// RemoveHeader removes a header
func (c *HTTP2RPCContext) RemoveHeader(name string) {
	delete(c.Headers, strings.ToLower(name))
}

// HasHeader checks if a header exists
func (c *HTTP2RPCContext) HasHeader(name string) bool {
	_, exists := c.Headers[strings.ToLower(name)]
	return exists
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

// RPCElement defines the interface for RPC elements (Enhancement of original interface)
// Similar to Envoy WASM filters, elements can access and modify both headers and payload
type RPCElement interface {
	// ProcessRequest processes the request before it's sent to the server
	// The context can be modified in place (headers, payload)
	// Returns a verdict indicating whether to pass or drop the request
	ProcessRequest(ctx context.Context, rpcCtx *HTTP2RPCContext) (Verdict, context.Context, error)

	// ProcessResponse processes the response after it's received from the server
	// The context can be modified in place (headers, payload)
	// Returns a verdict indicating whether to pass or drop the response
	ProcessResponse(ctx context.Context, rpcCtx *HTTP2RPCContext) (Verdict, context.Context, error)

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
func (c *RPCElementChain) ProcessRequest(ctx context.Context, rpcCtx *HTTP2RPCContext) (Verdict, context.Context, error) {
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
func (c *RPCElementChain) ProcessResponse(ctx context.Context, rpcCtx *HTTP2RPCContext) (Verdict, context.Context, error) {
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
