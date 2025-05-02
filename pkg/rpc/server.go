package rpc

import (
	"bytes"
	"context"
	"encoding/binary"
	"log"

	"github.com/appnet-org/arpc/internal/metadata"
	"github.com/appnet-org/arpc/internal/protocol"
	"github.com/appnet-org/arpc/internal/serializer"
	"github.com/appnet-org/arpc/internal/transport"
)

// MethodHandler defines the function signature for handling an RPC method.
type MethodHandler func(srv any, ctx context.Context, dec func(any) error) (resp any, newCtx context.Context, err error)

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
	transport     *transport.UDPTransport
	serializer    serializer.Serializer
	metadataCodec metadata.MetadataCodec
	services      map[string]*ServiceDesc
}

// NewServer initializes a new Server instance with the given address and serializer.
func NewServer(addr string, serializer serializer.Serializer) (*Server, error) {
	udpTransport, err := transport.NewUDPTransport(addr)
	if err != nil {
		return nil, err
	}
	return &Server{
		transport:     udpTransport,
		serializer:    serializer,
		metadataCodec: metadata.MetadataCodec{},
		services:      make(map[string]*ServiceDesc),
	}, nil
}

// RegisterService registers a service and its methods with the server.
func (s *Server) RegisterService(desc *ServiceDesc, impl any) {
	s.services[desc.ServiceName] = desc
	log.Printf("Registered service: %s\n", desc.ServiceName)
}

// parseFramedRequest extracts service, method, header, and payload segments from a request frame.
func parseFramedRequest(data []byte) (string, string, []byte, []byte, error) {
	offset := 0

	// Service
	serviceLen := int(binary.LittleEndian.Uint16(data[offset:]))
	offset += 2
	service := string(data[offset : offset+serviceLen])
	offset += serviceLen

	// Method
	methodLen := int(binary.LittleEndian.Uint16(data[offset:]))
	offset += 2
	method := string(data[offset : offset+methodLen])
	offset += methodLen

	// Headers
	headerLen := int(binary.LittleEndian.Uint16(data[offset:]))
	offset += 2
	headers := data[offset : offset+headerLen]
	offset += headerLen

	// Payload
	payload := data[offset:]

	return service, method, headers, payload, nil
}

func frameResponse(service, method string, headers []byte, payload []byte) ([]byte, error) {
	buf := new(bytes.Buffer)

	// Write service name
	if err := binary.Write(buf, binary.LittleEndian, uint16(len(service))); err != nil {
		return nil, err
	}
	if _, err := buf.Write([]byte(service)); err != nil {
		return nil, err
	}

	// Write method name
	if err := binary.Write(buf, binary.LittleEndian, uint16(len(method))); err != nil {
		return nil, err
	}
	if _, err := buf.Write([]byte(method)); err != nil {
		return nil, err
	}

	// Write header bytes
	if err := binary.Write(buf, binary.LittleEndian, uint16(len(headers))); err != nil {
		return nil, err
	}
	if _, err := buf.Write(headers); err != nil {
		return nil, err
	}

	// Write payload
	if _, err := buf.Write(payload); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
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
		serviceName, methodName, reqHeaderBytes, reqPayloadBytes, err := parseFramedRequest(data)
		if err != nil {
			log.Printf("Failed to parse framed request: %v", err)
			continue
		}

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

		// Decode headers
		md, err := s.metadataCodec.DecodeHeaders(reqHeaderBytes)
		if err != nil {
			log.Printf("Failed to decode headers: %v", err)
			continue
		}
		ctx := metadata.NewIncomingContext(context.Background(), md)

		// Log all headers
		log.Printf("Received headers for %s.%s:", serviceName, methodName)
		for k, v := range md {
			log.Printf("  %s: %s", k, v)
		}

		// Invoke method handler
		resp, ctx, err := methodDesc.Handler(svcDesc.ServiceImpl, ctx, func(v any) error {
			return s.serializer.Unmarshal(reqPayloadBytes, v)
		})
		if err != nil {
			log.Printf("Handler error: %v", err)
			continue
		}

		// Serialize response
		respPayloadBytes, err := s.serializer.Marshal(resp)
		if err != nil {
			log.Printf("Error marshaling response: %v", err)
			continue
		}

		// Extract response headers from context
		respMD := metadata.FromOutgoingContext(ctx) // default to empty for now
		respHeaderBytes, err := s.metadataCodec.EncodeHeaders(respMD)
		if err != nil {
			log.Printf("Failed to encode response headers: %v", err)
			continue
		}

		// Frame response
		framedResp, err := frameResponse(serviceName, methodName, respHeaderBytes, respPayloadBytes)
		if err != nil {
			log.Printf("Failed to frame response: %v", err)
			continue
		}

		err = s.transport.Send(addr.String(), rpcID, framedResp)
		if err != nil {
			log.Printf("Error sending response: %v", err)
		}
	}
}
