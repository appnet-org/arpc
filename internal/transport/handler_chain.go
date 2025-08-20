package transport

import (
	"fmt"
	"log"
	"net"
)

// HandlerChain represents a chain of handlers that can be executed in sequence
type HandlerChain struct {
	handlers []PacketHandler
	name     string
}

// NewHandlerChain creates a new handler chain with the given name
func NewHandlerChain(name string) *HandlerChain {
	return &HandlerChain{
		handlers: make([]PacketHandler, 0),
		name:     name,
	}
}

// AddHandler adds a handler to the end of the chain
func (hc *HandlerChain) AddHandler(handler PacketHandler) *HandlerChain {
	hc.handlers = append(hc.handlers, handler)
	return hc
}

// PrependHandler adds a handler to the beginning of the chain
func (hc *HandlerChain) PrependHandler(handler PacketHandler) *HandlerChain {
	hc.handlers = append([]PacketHandler{handler}, hc.handlers...)
	return hc
}

// RemoveHandler removes a handler at the specified position
func (hc *HandlerChain) RemoveHandler(position int) *HandlerChain {
	if position < 0 || position >= len(hc.handlers) {
		return hc
	}

	hc.handlers = append(hc.handlers[:position], hc.handlers[position+1:]...)
	return hc
}

// Clear removes all handlers from the chain
func (hc *HandlerChain) Clear() *HandlerChain {
	hc.handlers = make([]PacketHandler, 0)
	return hc
}

// GetHandlers returns a copy of the handlers slice
func (hc *HandlerChain) GetHandlers() []PacketHandler {
	handlers := make([]PacketHandler, len(hc.handlers))
	copy(handlers, hc.handlers)
	return handlers
}

// Length returns the number of handlers in the chain
func (hc *HandlerChain) Length() int {
	return len(hc.handlers)
}

// Name returns the name of the handler chain
func (hc *HandlerChain) Name() string {
	return hc.name
}

// OnReceive executes all handlers in the chain in sequence for receiving packets
func (hc *HandlerChain) OnReceive(pkt any, addr *net.UDPAddr) error {
	log.Printf("Executing receive handler chain %s", hc.name)

	// Execute handlers in sequence
	for i, handler := range hc.handlers {
		if err := handler.OnReceive(pkt, addr); err != nil {
			return fmt.Errorf("handler %d (%T) failed: %w", i, handler, err)
		}
	}

	return nil
}

// OnSend executes all handlers in the chain in sequence for sending packets
func (hc *HandlerChain) OnSend(pkt any, addr *net.UDPAddr) error {
	log.Printf("Executing send handler chain %s", hc.name)

	// Execute handlers in sequence
	for i, handler := range hc.handlers {
		if err := handler.OnSend(pkt, addr); err != nil {
			return fmt.Errorf("handler %d (%T) failed: %w", i, handler, err)
		}
	}

	return nil
}
