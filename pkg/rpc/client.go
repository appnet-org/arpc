package rpc

import (
	"fmt"
	"log"
	"time"

	"github.com/appnet-org/aprc/internal/protocol"
	"github.com/appnet-org/aprc/internal/serializer"
	"github.com/appnet-org/aprc/internal/transport"
)

type Client struct {
	transport   *transport.UDPTransport
	serializer  serializer.Serializer
	defaultAddr string
}

// NewClient creates a new Client using the given serializer and target address.
func NewClient(serializer serializer.Serializer, addr string) (*Client, error) {
	t, err := transport.NewUDPTransport("")
	if err != nil {
		return nil, err
	}
	return &Client{
		transport:   t,
		serializer:  serializer,
		defaultAddr: addr,
	}, nil
}

// Call sends a request to the specified method and unmarshals the response into resp.
func (c *Client) Call(method string, req any, resp any) error {
	rpcID := transport.GenerateRPCID()

	// Serialize the request
	payload, err := c.serializer.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	log.Printf("Sending request to method '%s' (RPC ID: %d) -> %s\n", method, rpcID, c.defaultAddr)

	// Send fragmented request
	if err := c.transport.Send(c.defaultAddr, rpcID, payload); err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	// Wait for response
	for {
		data, _, respID, err := c.transport.Receive(protocol.MaxUDPPayloadSize)
		if err != nil {
			return fmt.Errorf("failed to receive response: %w", err)
		}

		if data == nil {
			time.Sleep(10 * time.Millisecond)
			continue // still waiting on full message
		}

		if respID != rpcID {
			log.Printf("Ignoring response with mismatched RPC ID: %d (expected %d)", respID, rpcID)
			continue
		}

		// Deserialize into provided response
		if err := c.serializer.Unmarshal(data, resp); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}

		log.Printf("Successfully received response for RPC ID %d\n", rpcID)
		return nil
	}
}
