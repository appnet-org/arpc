package transport

import (
	"net"

	"github.com/appnet-org/arpc/internal/protocol"
)

// PacketHandler defines the interface for handling specific packet types
type PacketHandler interface {
	Handle(pkt any, addr *net.UDPAddr) ([]byte, *net.UDPAddr, uint64, error)
}

// HandlerRegistry manages packet handlers for different packet types
type HandlerRegistry struct {
	handlers map[protocol.PacketType]PacketHandler
}

// NewHandlerRegistry creates a new handler registry with default handlers
func NewHandlerRegistry(transport *UDPTransport) *HandlerRegistry {
	registry := &HandlerRegistry{
		handlers: make(map[protocol.PacketType]PacketHandler),
	}

	// Register default handlers with the transport
	registry.RegisterHandler(protocol.PacketTypeRequest, NewDefaultRequestHandler(NewDefaultDataHandler(transport)))
	registry.RegisterHandler(protocol.PacketTypeResponse, NewDefaultResponseHandler(NewDefaultDataHandler(transport)))
	registry.RegisterHandler(protocol.PacketTypeError, &DefaultErrorHandler{})

	return registry
}

// RegisterHandler registers a new packet handler
func (hr *HandlerRegistry) RegisterHandler(packetType protocol.PacketType, handler PacketHandler) {
	hr.handlers[packetType] = handler
}

// GetHandler retrieves the handler for a specific packet type
func (hr *HandlerRegistry) GetHandler(packetType protocol.PacketType) (PacketHandler, bool) {
	handler, exists := hr.handlers[packetType]
	return handler, exists
}
