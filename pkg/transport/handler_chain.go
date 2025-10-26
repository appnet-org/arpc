package transport

import (
	"fmt"
	"net"

	"github.com/appnet-org/arpc/pkg/logging"
	"go.uber.org/zap"
)

// Handler defines the interface for packet handlers
type Handler interface {
	OnReceive(pkt any, addr *net.UDPAddr) error
	OnSend(pkt any, addr *net.UDPAddr) error
}

// HandlerChain represents a chain of handlers for a specific packet type
type HandlerChain struct {
	name     string
	handlers []Handler
}

// NewHandlerChain creates a new handler chain
func NewHandlerChain(name string, handlers ...Handler) *HandlerChain {
	return &HandlerChain{
		name:     name,
		handlers: handlers,
	}
}

func (hc *HandlerChain) AddHandler(handler Handler) {
	hc.handlers = append(hc.handlers, handler)
}

// RemoveHandler removes a handler from the chain
func (hc *HandlerChain) RemoveHandler(handler Handler) bool {
	for i, h := range hc.handlers {
		if h == handler {
			// Remove the handler by slicing around it
			hc.handlers = append(hc.handlers[:i], hc.handlers[i+1:]...)
			return true
		}
	}
	return false
}

// GetHandlers returns a copy of the handlers slice
func (hc *HandlerChain) GetHandlers() []Handler {
	handlers := make([]Handler, len(hc.handlers))
	copy(handlers, hc.handlers)
	return handlers
}

// OnReceive processes a packet through the receive chain
func (hc *HandlerChain) OnReceive(pkt any, addr *net.UDPAddr) error {
	logging.Debug("Executing on_receive handler chain", zap.String("chainName", hc.name))

	for i, handler := range hc.handlers {
		if err := handler.OnReceive(pkt, addr); err != nil {
			logging.Error("Handler error in receive chain",
				zap.String("chainName", hc.name),
				zap.Int("handlerIndex", i),
				zap.Error(err))
			return fmt.Errorf("handler %d in chain %s failed: %w", i, hc.name, err)
		}
	}
	return nil
}

// OnSend processes a packet through the send chain
func (hc *HandlerChain) OnSend(pkt any, addr *net.UDPAddr) error {
	logging.Debug("Executing on_send handler chain", zap.String("chainName", hc.name))

	for i, handler := range hc.handlers {
		if err := handler.OnSend(pkt, addr); err != nil {
			logging.Error("Handler error in send chain",
				zap.String("chainName", hc.name),
				zap.Int("handlerIndex", i),
				zap.Error(err))
			return fmt.Errorf("handler %d in chain %s failed: %w", i, hc.name, err)
		}
	}
	return nil
}
