package transport

import (
	"net"

	"github.com/appnet-org/arpc/internal/packet"
)

type Role string

const (
	RoleClient Role = "client"
	RoleServer Role = "server"
)

// PacketHandler defines the interface for handling specific packet types
type PacketHandler interface {
	OnReceive(pkt any, addr *net.UDPAddr) error
	OnSend(pkt any, addr *net.UDPAddr) error
}

// HandlerRegistry manages packet handlers for different packet types
type HandlerRegistry struct {
	clientHandlers map[packet.PacketTypeID]*HandlerChain
	serverHandlers map[packet.PacketTypeID]*HandlerChain
}

// NewHandlerRegistry creates a new handler registry with default handlers
func NewHandlerRegistry(transport *UDPTransport) *HandlerRegistry {
	registry := &HandlerRegistry{
		clientHandlers: make(map[packet.PacketTypeID]*HandlerChain),
		serverHandlers: make(map[packet.PacketTypeID]*HandlerChain),
	}

	// Create default handler chains (by default, we don't have any handlers registered.)
	requestChain := NewHandlerChain("RequestHandlerChain")
	responseChain := NewHandlerChain("ResponseHandlerChain")
	errorChain := NewHandlerChain("ErrorHandlerChain")

	// Register default handler chains for both client and server
	registry.RegisterHandlerChain(packet.PacketTypeRequest.TypeID, requestChain, RoleClient)
	registry.RegisterHandlerChain(packet.PacketTypeResponse.TypeID, responseChain, RoleClient)
	registry.RegisterHandlerChain(packet.PacketTypeError.TypeID, errorChain, RoleClient)

	registry.RegisterHandlerChain(packet.PacketTypeRequest.TypeID, requestChain, RoleServer)
	registry.RegisterHandlerChain(packet.PacketTypeResponse.TypeID, responseChain, RoleServer)
	registry.RegisterHandlerChain(packet.PacketTypeError.TypeID, errorChain, RoleServer)

	return registry
}

func (hr *HandlerRegistry) GetHandlerChain(packetTypeID packet.PacketTypeID, role Role) (*HandlerChain, bool) {
	switch role {
	case RoleClient:
		chain, exists := hr.clientHandlers[packetTypeID]
		return chain, exists
	case RoleServer:
		chain, exists := hr.serverHandlers[packetTypeID]
		return chain, exists
	}
	return nil, false
}

// RegisterHandlerChain registers a handler chain for a packet type on the client or server side
func (hr *HandlerRegistry) RegisterHandlerChain(packetTypeID packet.PacketTypeID, chain *HandlerChain, role Role) {
	switch role {
	case RoleClient:
		hr.clientHandlers[packetTypeID] = chain
	case RoleServer:
		hr.serverHandlers[packetTypeID] = chain
	}
}
