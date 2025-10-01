package element

import (
	"context"
)

// RPCElement defines the interface for RPC elements
type RPCElement interface {
	// ProcessRequest processes the request before it's sent to the server
	ProcessRequest(ctx context.Context, req []byte) ([]byte, error)

	// ProcessResponse processes the response after it's received from the server
	ProcessResponse(ctx context.Context, resp []byte) ([]byte, error)

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
func (c *RPCElementChain) ProcessRequest(ctx context.Context, req []byte) ([]byte, error) {
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
func (c *RPCElementChain) ProcessResponse(ctx context.Context, resp []byte) ([]byte, error) {
	var err error
	for i := len(c.elements) - 1; i >= 0; i-- {
		resp, err = c.elements[i].ProcessResponse(ctx, resp)
		if err != nil {
			return nil, err
		}
	}
	return resp, nil
}
