package element

import (
	"context"
)

// Request represents an RPC request
type RPCRequest struct {
	ID          uint64 // Unique identifier for the request
	ServiceName string // Name of the service being called
	Method      string // Name of the method being called
	Payload     any    // RPC payload
}

// Response represents an RPC response
type RPCResponse struct {
	Result any
	Error  error
}

// RPCElement defines the interface for RPC elements
type RPCElement interface {
	// ProcessRequest processes the request before it's sent to the server
	ProcessRequest(ctx context.Context, req *RPCRequest) (*RPCRequest, error)

	// ProcessResponse processes the response after it's received from the server
	ProcessResponse(ctx context.Context, resp *RPCResponse) (*RPCResponse, error)

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
func (c *RPCElementChain) ProcessRequest(ctx context.Context, req *RPCRequest) (*RPCRequest, error) {
	var err error
	for _, element := range c.elements {
		req, err = element.ProcessRequest(ctx, req)
		if err != nil {
			return nil, err
		}
	}
	return req, nil
}

// ProcessResponse processes the response through all RPC elements in reverse order
func (c *RPCElementChain) ProcessResponse(ctx context.Context, resp *RPCResponse) (*RPCResponse, error) {
	var err error
	for i := len(c.elements) - 1; i >= 0; i-- {
		resp, err = c.elements[i].ProcessResponse(ctx, resp)
		if err != nil {
			return nil, err
		}
	}
	return resp, nil
}
