package transport

import (
	"net"

	"github.com/appnet-org/arpc/internal/packet"
)

// PacketHandler defines the interface for handling specific packet types
type PacketHandler interface {
	OnReceive(pkt any, addr *net.UDPAddr) error
	OnSend(pkt any, addr *net.UDPAddr) error
}

// HandlerRegistry manages packet handlers for different packet types
type HandlerRegistry struct {
	handlers map[packet.PacketTypeID]*HandlerChain // map of packet type ID to handler chain
}

// NewHandlerRegistry creates a new handler registry with default handlers
func NewHandlerRegistry(transport *UDPTransport) *HandlerRegistry {
	registry := &HandlerRegistry{
		handlers: make(map[packet.PacketTypeID]*HandlerChain),
	}

	// Create default handler chains (by default, we don't have any handlers registered.)
	requestChain := NewHandlerChain("RequestHandlerChain")
	responseChain := NewHandlerChain("ResponseHandlerChain")
	errorChain := NewHandlerChain("ErrorHandlerChain")

	// Register default handler chains
	registry.RegisterHandlerChain(packet.PacketTypeRequest.ID, requestChain)
	registry.RegisterHandlerChain(packet.PacketTypeResponse.ID, responseChain)
	registry.RegisterHandlerChain(packet.PacketTypeError.ID, errorChain)

	return registry
}

// RegisterHandlerChain registers a handler chain for a packet type
func (hr *HandlerRegistry) RegisterHandlerChain(packetTypeID packet.PacketTypeID, chain *HandlerChain) {
	hr.handlers[packetTypeID] = chain
}

// GetHandlerChain is an alias for GetHandler for clarity
func (hr *HandlerRegistry) GetHandlerChain(packetTypeID packet.PacketTypeID) (*HandlerChain, bool) {
	chain, exists := hr.handlers[packetTypeID]
	return chain, exists
}
