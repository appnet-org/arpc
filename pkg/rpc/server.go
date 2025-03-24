package rpc

import (
	"github.com/appnet-org/aprc/internal/transport"
	"github.com/appnet-org/aprc/internal/protocol"
)

type Server struct {
	transport *transport.UDPTransport
	handler   func(*protocol.RPCMessage) *protocol.RPCMessage
}

func NewServer(addr string, handler func(*protocol.RPCMessage) *protocol.RPCMessage) (*Server, error) {
	udpTransport, err := transport.NewUDPTransport(addr)
	if err != nil {
		return nil, err
	}

	return &Server{transport: udpTransport, handler: handler}, nil
}

func (s *Server) Start() {
	for {
		data, addr, err := s.transport.Receive(1024)
		if err != nil {
			continue
		}
		msg, err := protocol.DecodeMessage(data)
		if err != nil {
			continue
		}
		response := s.handler(msg)
		respData, _ := protocol.EncodeMessage(response)
		s.transport.Send(addr.String(), respData)
	}
}