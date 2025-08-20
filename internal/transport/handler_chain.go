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

// AddHandlers adds multiple handlers to the end of the chain
func (hc *HandlerChain) AddHandlers(handlers ...PacketHandler) *HandlerChain {
	hc.handlers = append(hc.handlers, handlers...)
	return hc
}

// InsertHandler inserts a handler at a specific position in the chain
func (hc *HandlerChain) InsertHandler(position int, handler PacketHandler) *HandlerChain {
	if position < 0 || position > len(hc.handlers) {
		// If position is invalid, append to the end
		hc.handlers = append(hc.handlers, handler)
		return hc
	}

	hc.handlers = append(hc.handlers[:position+1], hc.handlers[position:]...)
	hc.handlers[position] = handler
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

// GetHandler returns the handler at the specified position
func (hc *HandlerChain) GetHandler(position int) (PacketHandler, error) {
	if position < 0 || position >= len(hc.handlers) {
		return nil, fmt.Errorf("handler position %d out of bounds", position)
	}
	return hc.handlers[position], nil
}

// Length returns the number of handlers in the chain
func (hc *HandlerChain) Length() int {
	return len(hc.handlers)
}

// Name returns the name of the handler chain
func (hc *HandlerChain) Name() string {
	return hc.name
}

// Handle executes all handlers in the chain in sequence
func (hc *HandlerChain) Handle(pkt any, addr *net.UDPAddr) ([]byte, *net.UDPAddr, uint64, error) {
	var result []byte
	var resultAddr *net.UDPAddr
	var resultRPCID uint64
	var err error

	// Execute handlers in sequence
	log.Printf("Executing handler chain %s", hc.name)
	for i, handler := range hc.handlers {
		result, resultAddr, resultRPCID, err = handler.Handle(pkt, addr)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("handler %d (%T) failed: %w", i, handler, err)
		}

		// Update packet for next handler if we have a result
		if result != nil {
			// Create a new packet context for the next handler
			// This allows handlers to modify the packet as it flows through the chain
			pkt = result
		}

		// Update address for next handler if we have a result address
		if resultAddr != nil {
			addr = resultAddr
		}
	}

	return result, resultAddr, resultRPCID, err
}

// Clone creates a deep copy of the handler chain
func (hc *HandlerChain) Clone() *HandlerChain {
	clone := NewHandlerChain(hc.name)
	for _, handler := range hc.handlers {
		clone.handlers = append(clone.handlers, handler)
	}
	return clone
}
