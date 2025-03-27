package rpc

import (
	"log"

	"github.com/appnet-org/aprc/internal/protocol"
	"github.com/appnet-org/aprc/internal/transport"
)

type Server struct {
	transport *transport.UDPTransport
	handler   func([]byte) []byte
}

// NewServer initializes and returns a new UDP-based RPC server
func NewServer(addr string, handler func([]byte) []byte) (*Server, error) {
	udpTransport, err := transport.NewUDPTransport(addr)
	if err != nil {
		return nil, err
	}

	return &Server{transport: udpTransport, handler: handler}, nil
}

// Start listens for incoming requests, processes them, and sends responses
func (s *Server) Start() {
	log.Println("Server started... Waiting for messages.")

	for {
		// Receive a message from a client
		data, addr, rpcID, err := s.transport.Receive(protocol.MaxUDPPayloadSize)
		if err != nil {
			log.Println("Error receiving data:", err)
			continue
		}

		if data == nil {
			continue // Still waiting for fragments
		}

		log.Printf("Received a message from %s, Message Length %d\n", addr.String(), len(data))

		// Process request and get response
		response := s.handler(data)

		// Send the response back
		err = s.transport.Send(addr.String(), rpcID, response)
		if err != nil {
			log.Println("Error sending response:", err)
		}
	}
}
