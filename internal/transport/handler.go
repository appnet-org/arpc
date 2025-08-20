package transport

import (
	"net"

	"github.com/appnet-org/arpc/internal/protocol"
)

// PacketHandler defines the interface for handling specific packet types
type PacketHandler interface {
	OnReceive(pkt any, addr *net.UDPAddr) error
	OnSend(pkt any, addr *net.UDPAddr) error
}

// HandlerRegistry manages packet handlers for different packet types
type HandlerRegistry struct {
	handlers map[protocol.PacketType]*HandlerChain
}

// NewHandlerRegistry creates a new handler registry with default handlers
func NewHandlerRegistry(transport *UDPTransport) *HandlerRegistry {
	registry := &HandlerRegistry{
		handlers: make(map[protocol.PacketType]*HandlerChain),
	}

	// Create default handler chains (by default, we don't have any handlers registered.)
	requestChain := NewHandlerChain("RequestHandlerChain")
	responseChain := NewHandlerChain("ResponseHandlerChain")
	errorChain := NewHandlerChain("ErrorHandlerChain")

	// Register default handler chains
	registry.RegisterHandlerChain(protocol.PacketTypeRequest, requestChain)
	registry.RegisterHandlerChain(protocol.PacketTypeResponse, responseChain)
	registry.RegisterHandlerChain(protocol.PacketTypeError, errorChain)

	return registry
}

// RegisterHandlerChain registers a handler chain for a packet type
func (hr *HandlerRegistry) RegisterHandlerChain(packetType protocol.PacketType, chain *HandlerChain) {
	hr.handlers[packetType] = chain
}

// GetHandlerChain is an alias for GetHandler for clarity
func (hr *HandlerRegistry) GetHandlerChain(packetType protocol.PacketType) (*HandlerChain, bool) {
	chain, exists := hr.handlers[packetType]
	return chain, exists
}
