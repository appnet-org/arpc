package transport

import (
	"log"
	"net"

	"github.com/appnet-org/arpc/internal/protocol"
)

// DefaultErrorHandler handles error packets
type DefaultErrorHandler struct{}

func (h *DefaultErrorHandler) Handle(pkt any, addr *net.UDPAddr) ([]byte, *net.UDPAddr, uint64, error) {
	errorPkt := pkt.(*protocol.ErrorPacket)
	log.Printf("Received error packet for RPC %d: %s", errorPkt.RPCID, errorPkt.ErrorMsg)
	return nil, addr, errorPkt.RPCID, nil
}

// DefaultRequestHandler handles request packets by delegating to DefaultDataHandler
type DefaultRequestHandler struct {
	dataHandler *DefaultDataHandler
}

func NewDefaultRequestHandler(dataHandler *DefaultDataHandler) *DefaultRequestHandler {
	return &DefaultRequestHandler{dataHandler: dataHandler}
}

func (h *DefaultRequestHandler) Handle(pkt any, addr *net.UDPAddr) ([]byte, *net.UDPAddr, uint64, error) {
	return h.dataHandler.Handle(pkt, addr)
}

// DefaultResponseHandler handles response packets by delegating to DefaultDataHandler
type DefaultResponseHandler struct {
	dataHandler *DefaultDataHandler
}

func NewDefaultResponseHandler(dataHandler *DefaultDataHandler) *DefaultResponseHandler {
	return &DefaultResponseHandler{dataHandler: dataHandler}
}

func (h *DefaultResponseHandler) Handle(pkt any, addr *net.UDPAddr) ([]byte, *net.UDPAddr, uint64, error) {
	return h.dataHandler.Handle(pkt, addr)
}

// TransportDataHandler is a wrapper that has access to transport state
type DefaultDataHandler struct {
	transport *UDPTransport
}

func NewDefaultDataHandler(transport *UDPTransport) *DefaultDataHandler {
	return &DefaultDataHandler{transport: transport}
}

func (h *DefaultDataHandler) Handle(pkt any, addr *net.UDPAddr) ([]byte, *net.UDPAddr, uint64, error) {
	dataPkt := pkt.(protocol.DataPacket)

	h.transport.mu.Lock()
	defer h.transport.mu.Unlock()

	if _, exists := h.transport.incoming[dataPkt.RPCID]; !exists {
		h.transport.incoming[dataPkt.RPCID] = make(map[uint16][]byte)
	}

	h.transport.incoming[dataPkt.RPCID][dataPkt.SeqNumber] = dataPkt.Payload

	if len(h.transport.incoming[dataPkt.RPCID]) == int(dataPkt.TotalPackets) {
		var fullMessage []byte
		for i := uint16(0); i < dataPkt.TotalPackets; i++ {
			fullMessage = append(fullMessage, h.transport.incoming[dataPkt.RPCID][i]...)
		}

		delete(h.transport.incoming, dataPkt.RPCID)
		return fullMessage, addr, dataPkt.RPCID, nil
	}

	return nil, nil, 0, nil
}
