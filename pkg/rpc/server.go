package rpc

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/metadata"
	"github.com/appnet-org/arpc/pkg/packet"
	"github.com/appnet-org/arpc/pkg/rpc/element"
	"github.com/appnet-org/arpc/pkg/serializer"
	"github.com/appnet-org/arpc/pkg/transport"
	"go.uber.org/zap"
)

// MethodHandler defines the function signature for handling an RPC method.
type MethodHandler func(srv any, ctx context.Context, dec func(any) error, req *element.RPCRequest, chain *element.RPCElementChain) (resp *element.RPCResponse, newCtx context.Context, err error)

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
	metadataCodec   metadata.MetadataCodec
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
		metadataCodec:   metadata.MetadataCodec{},
		services:        make(map[string]*ServiceDesc),
		rpcElementChain: element.NewRPCElementChain(rpcElements...),
	}, nil
}

// RegisterService registers a service and its methods with the server.
func (s *Server) RegisterService(desc *ServiceDesc, impl any) {
	s.services[desc.ServiceName] = desc
	logging.Info("Registered service", zap.String("serviceName", desc.ServiceName))
}

// parseFramedRequest extracts service, method, metadata, and payload segments from a request frame.
// Wire format: [dst ip(4B)][dst port(2B)][src port(2B)][serviceLen(2B)][service][methodLen(2B)][method][metadataLen(2B)][metadata][payload]
func (s *Server) parseFramedRequest(data []byte) (string, string, metadata.Metadata, []byte, error) {
	offset := 0

	// Service
	if offset+2 > len(data) {
		return "", "", nil, nil, fmt.Errorf("data too short for service length")
	}
	serviceLen := int(binary.LittleEndian.Uint16(data[offset:]))
	offset += 2
	if offset+serviceLen > len(data) {
		return "", "", nil, nil, fmt.Errorf("service length %d exceeds data length", serviceLen)
	}
	service := string(data[offset : offset+serviceLen])
	offset += serviceLen

	// Method
	if offset+2 > len(data) {
		return "", "", nil, nil, fmt.Errorf("data too short for method length")
	}
	methodLen := int(binary.LittleEndian.Uint16(data[offset:]))
	offset += 2
	if offset+methodLen > len(data) {
		return "", "", nil, nil, fmt.Errorf("method length %d exceeds data length", methodLen)
	}
	method := string(data[offset : offset+methodLen])
	offset += methodLen

	// Metadata
	var md metadata.Metadata
	if offset+2 > len(data) {
		return "", "", nil, nil, fmt.Errorf("data too short for metadata length")
	}
	metadataLen := int(binary.LittleEndian.Uint16(data[offset:]))
	offset += 2

	if metadataLen > 0 {
		if offset+metadataLen > len(data) {
			return "", "", nil, nil, fmt.Errorf("metadata length %d exceeds data length", metadataLen)
		}
		metadataBytes := data[offset : offset+metadataLen]
		offset += metadataLen

		// Decode metadata
		var err error
		md, err = s.metadataCodec.DecodeHeaders(metadataBytes)
		if err != nil {
			return "", "", nil, nil, fmt.Errorf("failed to decode metadata: %w", err)
		}
		logging.Debug("Decoded metadata", zap.Any("metadata", md))
	}

	// Payload
	payload := data[offset:]

	return service, method, md, payload, nil
}

// frameResponse constructs a binary message with
// [serviceLen(2B)][service][methodLen(2B)][method][payload]
func (s *Server) frameResponse(service, method string, payload []byte) ([]byte, error) {
	// total size = 2 + len(service) + 2 + len(method) + len(payload)
	totalSize := 4 + len(service) + len(method) + len(payload)
	buf := make([]byte, totalSize)

	// service length
	binary.LittleEndian.PutUint16(buf[0:2], uint16(len(service)))
	copy(buf[2:], service)

	// method length
	methodStart := 2 + len(service)
	binary.LittleEndian.PutUint16(buf[methodStart:methodStart+2], uint16(len(method)))
	copy(buf[methodStart+2:], method)

	// payload
	payloadStart := methodStart + 2 + len(method)
	copy(buf[payloadStart:], payload)

	return buf, nil
}

// Start begins listening for incoming RPC requests, dispatching to the appropriate service/method handler.
func (s *Server) Start() {
	logging.Info("Server started... Waiting for messages.")

	for {
		// Receive a packet from a client
		data, addr, rpcID, _, err := s.transport.Receive(packet.MaxUDPPayloadSize, transport.RoleServer)
		if err != nil {
			logging.Error("Error receiving data", zap.Error(err))
			if err := s.transport.Send(addr.String(), rpcID, []byte(err.Error()), packet.PacketTypeUnknown); err != nil {
				logging.Error("Error sending error response", zap.Error(err))
			}
			continue
		}

		if data == nil {
			continue // Either still waiting for fragments or we received an non-data packet
		}

		// Parse request payload
		serviceName, methodName, md, reqPayloadBytes, err := s.parseFramedRequest(data)
		if err != nil {
			logging.Error("Failed to parse framed request", zap.Error(err))
			if err := s.transport.Send(addr.String(), rpcID, []byte(err.Error()), packet.PacketTypeUnknown); err != nil {
				logging.Error("Error sending error response", zap.Error(err))
			}
			continue
		}

		// Create context with incoming metadata
		ctx := context.Background()
		if len(md) > 0 {
			ctx = metadata.NewIncomingContext(ctx, md)
		}

		// Create RPC request for element processing
		rpcReq := &element.RPCRequest{
			ID:          rpcID,
			ServiceName: serviceName,
			Method:      methodName,
		}

		// Lookup service and method
		svcDesc, ok := s.services[rpcReq.ServiceName]
		if !ok {
			logging.Warn("Unknown service", zap.String("serviceName", rpcReq.ServiceName))
			if err := s.transport.Send(addr.String(), rpcID, []byte("unknown service"), packet.PacketTypeError); err != nil {
				logging.Error("Error sending error response", zap.Error(err))
			}
			continue
		}
		methodDesc, ok := svcDesc.Methods[rpcReq.Method]
		if !ok {
			logging.Warn("Unknown method",
				zap.String("serviceName", rpcReq.ServiceName),
				zap.String("methodName", rpcReq.Method))
			continue
		}

		// Invoke method handler with context containing metadata
		rpcResp, _, err := methodDesc.Handler(svcDesc.ServiceImpl, ctx, func(v any) error {
			return s.serializer.Unmarshal(reqPayloadBytes, v)
		}, rpcReq, s.rpcElementChain)
		if err != nil {
			var errType packet.PacketType
			if rpcErr, ok := err.(*RPCError); ok && rpcErr.Type == RPCFailError {
				errType = packet.PacketTypeError
			} else {
				errType = packet.PacketTypeUnknown
				logging.Error("Handler error", zap.Error(err))
			}
			if err := s.transport.Send(addr.String(), rpcID, []byte(err.Error()), errType); err != nil {
				logging.Error("Error sending error response", zap.Error(err))
			}
			continue
		}

		// Serialize response
		respPayloadBytes, err := s.serializer.Marshal(rpcResp.Result)
		if err != nil {
			logging.Error("Error marshaling response", zap.Error(err))
			if err := s.transport.Send(addr.String(), rpcID, []byte(err.Error()), packet.PacketTypeUnknown); err != nil {
				logging.Error("Error sending error response", zap.Error(err))
			}
			continue
		}

		// Frame response
		framedResp, err := s.frameResponse(rpcReq.ServiceName, rpcReq.Method, respPayloadBytes)
		if err != nil {
			logging.Error("Failed to frame response", zap.Error(err))
			if err := s.transport.Send(addr.String(), rpcID, []byte(err.Error()), packet.PacketTypeUnknown); err != nil {
				logging.Error("Error sending error response", zap.Error(err))
			}
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
