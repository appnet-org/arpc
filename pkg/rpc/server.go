package rpc

import (
	"context"
	"encoding/binary"
	"log"
	"strings"

	"github.com/appnet-org/aprc/internal/protocol"
	"github.com/appnet-org/aprc/internal/serializer"
	"github.com/appnet-org/aprc/internal/transport"
)

type MethodHandler func(srv any, ctx context.Context, dec func(any) error) (any, error)

// MethodDesc represents an RPC service's method specification.
type MethodDesc struct {
	MethodName string
	Handler    MethodHandler
}

// ServiceDesc represents an RPC service's specification.
type ServiceDesc struct {
	ServiceImpl any
	ServiceName string
	Methods     map[string]*MethodDesc
}

type Server struct {
	transport  *transport.UDPTransport
	serializer serializer.Serializer
	services   map[string]*ServiceDesc
}

// NewServer initializes a new server
func NewServer(addr string, serializer serializer.Serializer) (*Server, error) {
	udpTransport, err := transport.NewUDPTransport(addr)
	if err != nil {
		return nil, err
	}
	return &Server{
		transport:  udpTransport,
		serializer: serializer,
		services:   make(map[string]*ServiceDesc),
	}, nil
}

// RegisterService registers a service and its methods with the server.
func (s *Server) RegisterService(desc *ServiceDesc, impl any) {
	s.services[strings.ToLower(desc.ServiceName)] = desc
	log.Printf("Registered service: %s\n", desc.ServiceName)
}

type RequestHeader struct {
	Service string
	Method  string
}

func parseFramedRequest(data []byte) (RequestHeader, []byte) {
	serviceLen := int(binary.LittleEndian.Uint16(data[0:2]))
	service := string(data[2 : 2+serviceLen])

	offset := 2 + serviceLen
	methodLen := int(binary.LittleEndian.Uint16(data[offset : offset+2]))
	method := string(data[offset+2 : offset+2+methodLen])

	payload := data[offset+2+methodLen:]
	return RequestHeader{Service: service, Method: method}, payload
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

		header, payload := parseFramedRequest(data)
		serviceName := strings.ToLower(header.Service)
		methodName := strings.ToLower(header.Method)

		svcDesc, ok := s.services[serviceName]
		if !ok {
			log.Printf("Unknown service: %s", serviceName)
			continue
		}
		methodDesc, ok := svcDesc.Methods[methodName]
		if !ok {
			log.Printf("Unknown method: %s.%s", serviceName, methodName)
			continue
		}

		// Now delegate to the method's handler
		// TODO: fix context
		respBytes, err := methodDesc.Handler(svcDesc.ServiceImpl, context.Background(), func(v any) error {
			return s.serializer.Unmarshal(payload, v)
		})
		if err != nil {
			log.Printf("Handler error: %v", err)
			continue
		}

		resp, ok := respBytes.([]byte)
		if !ok {
			log.Printf("Handler returned non-byte response: %T", respBytes)
			continue
		}
		err = s.transport.Send(addr.String(), rpcID, resp)
		if err != nil {
			log.Printf("Error sending response: %v", err)
		}
	}
}
