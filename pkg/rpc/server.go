package rpc

import (
	"github.com/appnet-org/aprc/internal/transport"
	"github.com/appnet-org/aprc/internal/protocol"
)

// Server represents a UDP-based RPC server that listens for incoming requests,
// processes them using a user-defined handler, and sends responses.
type Server struct {
	transport *transport.UDPTransport                // UDP transport for receiving and sending messages
	handler   func(*protocol.RPCMessage) *protocol.RPCMessage // Function to process incoming RPC messages
}

// NewServer initializes a new RPC server that listens on the specified address.
// The handler function is responsible for processing incoming requests and returning responses.
func NewServer(addr string, handler func(*protocol.RPCMessage) *protocol.RPCMessage) (*Server, error) {
	// Create a new UDP transport to listen on the given address.
	udpTransport, err := transport.NewUDPTransport(addr)
	if err != nil {
		return nil, err
	}

	// Return the initialized server instance with the provided handler.
	return &Server{transport: udpTransport, handler: handler}, nil
}

// Start begins listening for incoming UDP messages and processes them using the handler function.
// The server runs in an infinite loop, continuously handling requests.
func (s *Server) Start() {
	for {
		// Receive a UDP message (up to 1024 bytes).
		data, addr, err := s.transport.Receive(1024)
		if err != nil {
			continue // Ignore errors and continue listening.
		}

		// Decode the received message into an RPCMessage struct.
		msg, err := protocol.DecodeMessage(data)
		if err != nil {
			continue // Ignore malformed messages.
		}

		// Process the message using the handler function.
		response := s.handler(msg)

		// Encode the response message and send it back to the client.
		respData, _ := protocol.EncodeMessage(response)
		s.transport.Send(addr.String(), respData)
	}
}