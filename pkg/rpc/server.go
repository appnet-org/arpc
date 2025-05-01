package rpc

import (
	"context"
	"encoding/binary"
	"log"

	"github.com/appnet-org/arpc/internal/protocol"
	"github.com/appnet-org/arpc/internal/serializer"
	"github.com/appnet-org/arpc/internal/transport"
)

// MethodHandler defines the function signature for handling an RPC method.
type MethodHandler func(srv any, ctx context.Context, dec func(any) error) (any, error)

// MethodDesc represents an RPC service's method specification.
type MethodDesc struct {
	MethodName string
	Handler    MethodHandler
}

// ServiceDesc describes an RPC service, including its implementation and methods.
type ServiceDesc struct {
	ServiceImpl any
	ServiceName string
	Methods     map[string]*MethodDesc
}

// Server is the core RPC server handling transport, serialization, and registered services.
type Server struct {
	transport  *transport.UDPTransport
	serializer serializer.Serializer
	services   map[string]*ServiceDesc
}

// NewServer initializes a new Server instance with the given address and serializer.
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
	s.services[desc.ServiceName] = desc
	log.Printf("Registered service: %s\n", desc.ServiceName)
}

// RequestHeader holds metadata about an incoming RPC request.
type RequestHeader struct {
	Service string
	Method  string
}

// parseFramedRequest extracts service, method, and payload from a framed request.
func parseFramedRequest(data []byte) (RequestHeader, []byte) {
	serviceLen := int(binary.LittleEndian.Uint16(data[0:2]))
	service := string(data[2 : 2+serviceLen])

	offset := 2 + serviceLen
	methodLen := int(binary.LittleEndian.Uint16(data[offset : offset+2]))
	method := string(data[offset+2 : offset+2+methodLen])

	payload := data[offset+2+methodLen:]
	return RequestHeader{Service: service, Method: method}, payload
}

// Start begins listening for incoming RPC requests, dispatching to the appropriate service/method handler.
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

		// Parse request header and payload
		header, payload := parseFramedRequest(data)
		serviceName := header.Service
		methodName := header.Method

		// Lookup service and method
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

		// Invoke method handler
		// TODO: improve context usage
		resp, err := methodDesc.Handler(svcDesc.ServiceImpl, context.Background(), func(v any) error {
			return s.serializer.Unmarshal(payload, v)
		})
		if err != nil {
			log.Printf("Handler error: %v", err)
			continue
		}

		// Serialize and send response
		respBytes, err := s.serializer.Marshal(resp)
		if err != nil {
			log.Printf("Error marshaling response: %v", err)
			continue
		}

		err = s.transport.Send(addr.String(), rpcID, respBytes)
		if err != nil {
			log.Printf("Error sending response: %v", err)
		}
	}
}
