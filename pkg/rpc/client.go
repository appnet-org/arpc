package rpc

import (
	"fmt"
	"log"

	"github.com/appnet-org/aprc/internal/protocol"
	"github.com/appnet-org/aprc/internal/transport"
)

type Client struct {
	transport *transport.UDPTransport
}

// NewClient initializes and returns an RPC client with a UDP transport instance
func NewClient() (*Client, error) {
	udpTransport, err := transport.NewUDPTransport("")
	if err != nil {
		return nil, err
	}

	return &Client{transport: udpTransport}, nil
}

// Call sends a request to the specified address and waits for a response
func (c *Client) Call(addr string, data []byte) ([]byte, error) {
	rpcID := transport.GenerateRPCID()

	log.Printf("Sending request (RPC ID: %d) to %s\n", rpcID, addr)

	err := c.transport.Send(addr, rpcID, data)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	response, _, _, err := c.transport.Receive(protocol.MaxUDPPayloadSize)
	if err != nil {
		return nil, fmt.Errorf("failed to receive response: %w", err)
	}

	log.Printf("Received response: %s\n", string(response))
	return response, nil
}
