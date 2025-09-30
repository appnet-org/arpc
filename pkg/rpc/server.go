package rpc

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"

	"github.com/appnet-org/arpc/internal/packet"
	"github.com/appnet-org/arpc/internal/transport"
	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/rpc/element"
	"github.com/appnet-org/arpc/pkg/serializer"
	"go.uber.org/zap"
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
	logging.Info("Registered service", zap.String("serviceName", desc.ServiceName))
}

// parseFramedRequest extracts service, method, header, and payload segments from a request frame.
func (s *Server) parseFramedRequest(data []byte) (string, string, []byte, error) {
	// TODO(XZ): this is a temporary solution fix issue #5 (the first 8 bytes are the original IP address and port)
	offset := 8

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

func (s *Server) frameResponse(service, method string, payload []byte) ([]byte, error) {
	// TOOD(xz): we should pre-calculate the buffer to avoid multiple allocations (issue #14).
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
	logging.Info("Server started... Waiting for messages.")

	for {
		// Receive a packet from a client
		data, addr, rpcID, _, err := s.transport.Receive(packet.MaxUDPPayloadSize, transport.RoleServer)
		if err != nil {
			logging.Error("Error receiving data", zap.Error(err))
			continue
		}

		if data == nil {
			continue // Either still waiting for fragments or we received an non-data packet
		}

		// Parse request payload
		serviceName, methodName, reqPayloadBytes, err := s.parseFramedRequest(data)
		if err != nil {
			logging.Error("Failed to parse framed request", zap.Error(err))
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
			logging.Error("RPC element processing error", zap.Error(err))
			continue
		}

		// Lookup service and method
		svcDesc, ok := s.services[rpcReq.ServiceName]
		if !ok {
			logging.Warn("Unknown service", zap.String("serviceName", rpcReq.ServiceName))
			continue
		}
		methodDesc, ok := svcDesc.Methods[rpcReq.Method]
		if !ok {
			logging.Warn("Unknown method",
				zap.String("serviceName", rpcReq.ServiceName),
				zap.String("methodName", rpcReq.Method))
			continue
		}

		// Invoke method handler
		resp, _, err := methodDesc.Handler(svcDesc.ServiceImpl, context.Background(), func(v any) error {
			return s.serializer.Unmarshal(rpcReq.Payload.([]byte), v)
		})
		if err != nil {
			logging.Error("Handler error", zap.Error(err))
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
			logging.Error("RPC element response processing error", zap.Error(err))
			continue
		}

		// Serialize response
		respPayloadBytes, err := s.serializer.Marshal(rpcResp.Result)
		if err != nil {
			logging.Error("Error marshaling response", zap.Error(err))
			continue
		}

		// Frame response
		framedResp, err := s.frameResponse(rpcReq.ServiceName, rpcReq.Method, respPayloadBytes)
		if err != nil {
			logging.Error("Failed to frame response", zap.Error(err))
			continue
		}

		// Send the response
		err = s.transport.Send(addr.String(), rpcID, framedResp, packet.PacketTypeResponse)
		if err != nil {
			logging.Error("Error sending response", zap.Error(err))
		}
	}
}

// Temporary functions to register packet types and handlers.
// TODO(XZ): remove these once the transport can be dynamically configured.

// RegisterPacketType registers a custom packet type with the server's transport
func (s *Server) RegisterPacketType(packetType string, codec packet.PacketCodec) (packet.PacketType, error) {
	return s.transport.RegisterPacketType(packetType, codec)
}

// RegisterPacketTypeWithID registers a custom packet type with a specific ID
func (s *Server) RegisterPacketTypeWithID(packetType string, id packet.PacketTypeID, codec packet.PacketCodec) (packet.PacketType, error) {
	return s.transport.RegisterPacketTypeWithID(packetType, id, codec)
}

// RegisterHandler registers a handler for a specific packet type and role
func (s *Server) RegisterHandler(packetTypeID packet.PacketTypeID, handler transport.Handler, role transport.Role) {
	handlerChain := transport.NewHandlerChain(fmt.Sprintf("ServerHandler_%d", packetTypeID), handler)
	s.transport.RegisterHandlerChain(packetTypeID, handlerChain, role)
}

// RegisterHandlerChain registers a complete handler chain for a packet type and role
func (s *Server) RegisterHandlerChain(packetTypeID packet.PacketTypeID, chain *transport.HandlerChain, role transport.Role) {
	s.transport.RegisterHandlerChain(packetTypeID, chain, role)
}

// GetRegisteredPackets returns all registered packet types
func (s *Server) GetRegisteredPackets() []packet.PacketType {
	return s.transport.ListRegisteredPackets()
}

// GetTransport returns the underlying transport for advanced operations
func (s *Server) GetTransport() *transport.UDPTransport {
	return s.transport
}
