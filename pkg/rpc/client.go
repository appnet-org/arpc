package rpc

import (
	"github.com/appnet-org/aprc/internal/transport"
	"github.com/appnet-org/aprc/internal/protocol"
)

// Client represents an RPC client that communicates with a remote server over UDP.
type Client struct {
	transport *transport.UDPTransport // UDP transport for sending and receiving messages
}

// NewClient creates a new RPC client instance.
// Currently, this function does not initialize the transport since it is created per request.
func NewClient() *Client {
	return &Client{}
}

// Call sends an RPC request to the given address and waits for a response.
// It performs the following steps:
// 1. Establishes a temporary UDP transport.
// 2. Encodes the RPC message into a byte slice.
// 3. Sends the encoded message to the server.
// 4. Waits for a response from the server.
// 5. Decodes the response and returns it.
func (c *Client) Call(addr string, msg *protocol.RPCMessage) (*protocol.RPCMessage, error) {
	// Create a temporary UDP transport for sending and receiving messages.
	transport, err := transport.NewUDPTransport("")
	if err != nil {
		return nil, err
	}
	defer transport.Close() // Ensure transport is closed after the request completes.

	// Encode the message into a byte slice.
	data, err := protocol.EncodeMessage(msg)
	if err != nil {
		return nil, err
	}

	// Send the message to the server at the specified address.
	if err := transport.Send(addr, data); err != nil {
		return nil, err
	}

	// Wait for a response from the server (up to 1024 bytes).
	respData, _, err := transport.Receive(1024)
	if err != nil {
		return nil, err
	}

	// Decode the received response into an RPCMessage and return it.
	return protocol.DecodeMessage(respData)
}