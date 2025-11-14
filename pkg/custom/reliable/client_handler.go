package reliable

import (
	"net"
	"time"

	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/packet"
	"go.uber.org/zap"
)

// ReliableClientHandler implements the client-side reliable transport logic
type ReliableClientHandler struct {
	*ReliableHandler // Embed base handler
}

// NewReliableClientHandler creates a new reliable client handler with default 30s timeout
func NewReliableClientHandler(transportSender TransportSender, timerMgr TimerScheduler) *ReliableClientHandler {
	return NewReliableClientHandlerWithTimeout(transportSender, timerMgr, 30*time.Second)
}

// NewReliableClientHandlerWithTimeout creates a new reliable client handler with custom timeout
func NewReliableClientHandlerWithTimeout(transportSender TransportSender, timerMgr TimerScheduler, timeout time.Duration) *ReliableClientHandler {
	handler := &ReliableClientHandler{
		ReliableHandler: newReliableHandler(transportSender, timerMgr, timeout),
	}

	// Cache ACK packet type during initialization
	ackPacketType, exists := transportSender.GetPacketRegistry().GetPacketTypeByName(AckPacketName)
	if !exists {
		logging.Fatal("ACK packet type not registered in transport - ensure ACK packet type is registered before creating reliable handler")
	}
	handler.ackPacketType = &ackPacketType

	handler.startCleanupTimer(TimerKeyReliableClientCleanup)

	logging.Debug("Reliable client handler created",
		zap.Duration("timeout", timeout))

	return handler
}

// OnSend handles outgoing REQUEST packets and ACK(kind=1) packets
func (h *ReliableClientHandler) OnSend(pkt any, addr *net.UDPAddr) error {
	switch p := pkt.(type) {
	case *packet.DataPacket:
		if p.PacketTypeID == packet.PacketTypeRequest.TypeID {
			return h.handleSendRequest(p)
		}
	case *ACKPacket:
		if p.Kind == 1 { // ACK for RESPONSE
			return h.handleSendACK(p, addr)
		}
	}
	return nil
}

// OnReceive handles incoming RESPONSE and ACK(kind=0) packets
func (h *ReliableClientHandler) OnReceive(pkt any, addr *net.UDPAddr) error {
	switch p := pkt.(type) {
	case *packet.DataPacket:
		if p.PacketTypeID == packet.PacketTypeResponse.TypeID {
			return h.handleReceiveResponse(p, addr)
		}
	case *ACKPacket:
		if p.Kind == 0 { // ACK for REQUEST
			return h.handleReceiveACK(p, addr)
		}
	}
	return nil
}

// handleSendRequest tracks outgoing REQUEST packets
func (h *ReliableClientHandler) handleSendRequest(pkt *packet.DataPacket) error {
	return h.handleSendDataPacket(pkt, "REQUEST")
}

// handleSendACK handles sending ACK packets (already formed, just update activity)
func (h *ReliableClientHandler) handleSendACK(ack *ACKPacket, addr *net.UDPAddr) error {
	return h.ReliableHandler.handleSendACK(ack, addr, "RESPONSE", "server")
}

// handleReceiveResponse processes incoming RESPONSE packets
func (h *ReliableClientHandler) handleReceiveResponse(pkt *packet.DataPacket, addr *net.UDPAddr) error {
	return h.handleReceiveDataPacket(pkt, addr, 1, "RESPONSE", "server")
}

// handleReceiveACK processes ACK packets for REQUESTs
func (h *ReliableClientHandler) handleReceiveACK(ack *ACKPacket, addr *net.UDPAddr) error {
	return h.ReliableHandler.handleReceiveACK(ack, addr, "REQUEST", "server")
}

// Cleanup cleans up resources
func (h *ReliableClientHandler) Cleanup() {
	h.ReliableHandler.Cleanup(TimerKeyReliableClientCleanup)
}
