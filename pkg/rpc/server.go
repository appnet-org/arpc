package rpc

import (
	"bytes"
	"context"
	"encoding/binary"
	"log"

	"github.com/appnet-org/arpc/internal/packet"
	"github.com/appnet-org/arpc/internal/transport"
	"github.com/appnet-org/arpc/pkg/rpc/element"
	"github.com/appnet-org/arpc/pkg/serializer"
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
	transport       *transport.UDPTransport
	serializer      serializer.Serializer
	services        map[string]*ServiceDesc
	rpcElementChain *element.RPCElementChain
}

// NewServer initializes a new Server instance with the given address and serializer.
func NewServer(addr string, serializer serializer.Serializer, rpcElements []element.RPCElement) (*Server, error) {
	udpTransport, err := transport.NewUDPTransport(addr)
	if err != nil {
		return nil, err
	}
	return &Server{
		transport:       udpTransport,
		serializer:      serializer,
		services:        make(map[string]*ServiceDesc),
		rpcElementChain: element.NewRPCElementChain(rpcElements...),
	}, nil
}

// RegisterService registers a service and its methods with the server.
func (s *Server) RegisterService(desc *ServiceDesc, impl any) {
	s.services[desc.ServiceName] = desc
	log.Printf("Registered service: %s\n", desc.ServiceName)
}

// parseFramedRequest extracts service, method, header, and payload segments from a request frame.
func parseFramedRequest(data []byte) (string, string, []byte, error) {
	// TODO(XZ): this is a temporary solution fix issue #5 (the first 6 bytes are the original IP address and port)
	offset := 6

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

	// Payload
	payload := data[offset:]

	return service, method, payload, nil
}

func frameResponse(service, method string, payload []byte) ([]byte, error) {
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
		// Receive a packet from a client
		data, addr, rpcID, _, err := s.transport.Receive(packet.MaxUDPPayloadSize)
		if err != nil {
			log.Println("Error receiving data:", err)
			continue
		}

		if data == nil {
			continue // Either still waiting for fragments or we received an non-data packet
		}

		// Parse request payload
		serviceName, methodName, reqPayloadBytes, err := parseFramedRequest(data)
		if err != nil {
			log.Printf("Failed to parse framed request: %v", err)
			continue
		}

		// Create RPC request for element processing
		rpcReq := &element.RPCRequest{
			ID:          rpcID,
			ServiceName: serviceName,
			Method:      methodName,
			Payload:     reqPayloadBytes,
		}

		// Process request through RPC elements
		rpcReq, err = s.rpcElementChain.ProcessRequest(context.Background(), rpcReq)
		if err != nil {
			log.Printf("RPC element processing error: %v", err)
			continue
		}

		// Lookup service and method
		svcDesc, ok := s.services[rpcReq.ServiceName]
		if !ok {
			log.Printf("Unknown service: %s", rpcReq.ServiceName)
			continue
		}
		methodDesc, ok := svcDesc.Methods[rpcReq.Method]
		if !ok {
			log.Printf("Unknown method: %s.%s", rpcReq.ServiceName, rpcReq.Method)
			continue
		}

		// Invoke method handler
		resp, _, err := methodDesc.Handler(svcDesc.ServiceImpl, context.Background(), func(v any) error {
			return s.serializer.Unmarshal(rpcReq.Payload.([]byte), v)
		})
		if err != nil {
			log.Printf("Handler error: %v", err)
			continue
		}

		// Create RPC response for element processing
		rpcResp := &element.RPCResponse{
			Result: resp,
			Error:  err,
		}

		// Process response through RPC elements
		rpcResp, err = s.rpcElementChain.ProcessResponse(context.Background(), rpcResp)
		if err != nil {
			log.Printf("RPC element response processing error: %v", err)
			continue
		}

		// Serialize response
		respPayloadBytes, err := s.serializer.Marshal(rpcResp.Result)
		if err != nil {
			log.Printf("Error marshaling response: %v", err)
			continue
		}

		// Frame response
		framedResp, err := frameResponse(rpcReq.ServiceName, rpcReq.Method, respPayloadBytes)
		if err != nil {
			log.Printf("Failed to frame response: %v", err)
			continue
		}

		// Send the response
		err = s.transport.Send(addr.String(), rpcID, framedResp, packet.PacketTypeResponse)
		if err != nil {
			log.Printf("Error sending response: %v", err)
		}
	}
}
