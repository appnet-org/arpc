package rpc

import (
	"bytes"
	"encoding/binary"
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

func frameRequest(service, method string, payload []byte) ([]byte, error) {
	buf := new(bytes.Buffer)

	// Write service name length and value
	if err := binary.Write(buf, binary.LittleEndian, uint16(len(service))); err != nil {
		return nil, err
	}
	if _, err := buf.Write([]byte(service)); err != nil {
		return nil, err
	}

	// Write method name length and value
	if err := binary.Write(buf, binary.LittleEndian, uint16(len(method))); err != nil {
		return nil, err
	}
	if _, err := buf.Write([]byte(method)); err != nil {
		return nil, err
	}

	// Write payload
	if _, err := buf.Write(payload); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// Call sends a request to the specified service + method and unmarshals the response into resp.
func (c *Client) Call(service, method string, req any, resp any) error {
	rpcID := transport.GenerateRPCID()

	// Serialize the request payload
	payload, err := c.serializer.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Frame the request: [serviceLen][service][methodLen][method][payload]
	framed, err := frameRequest(service, method, payload)
	if err != nil {
		return fmt.Errorf("failed to frame request: %w", err)
	}

	log.Printf("Sending request to %s.%s (RPC ID: %d) -> %s\n", service, method, rpcID, c.defaultAddr)

	// Send fragmented request
	if err := c.transport.Send(c.defaultAddr, rpcID, framed); err != nil {
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
			continue
		}
		if respID != rpcID {
			log.Printf("Ignoring response with mismatched RPC ID: %d (expected %d)", respID, rpcID)
			continue
		}

		if err := c.serializer.Unmarshal(data, resp); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}

		log.Printf("Successfully received response for RPC ID %d\n", rpcID)
		return nil
	}
}
