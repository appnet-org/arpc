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

// func (c *RPCElementChain) parseMetadata(ctx context.Context, data []byte) (uint64, string, string, uint16, error) {
// 	offset := uint16(1)
// 	rpcID := binary.LittleEndian.Uint64(data[offset : offset+8])
// 	offset += 12

// 	serviceLen := binary.LittleEndian.Uint16(data[offset : offset+2])
// 	offset += 2
// 	if offset+serviceLen > uint16(len(data)) {
// 		log.Printf("Invalid packet: service length %d is too large", serviceLen)
// 		return 0, "", "", 0, nil
// 	}
// 	service := data[offset : offset+serviceLen]
// 	offset += serviceLen
// 	methodLen := binary.LittleEndian.Uint16(data[offset : offset+2])
// 	offset += 2
// 	if offset+methodLen > uint16(len(data)) {
// 		log.Printf("Invalid packet: method length %d is too large", methodLen)
// 		return 0, "", "", 0, nil
// 	}
// 	method := data[offset : offset+methodLen]
// 	offset += methodLen
// 	return rpcID, string(service), string(method), offset, nil
// }
