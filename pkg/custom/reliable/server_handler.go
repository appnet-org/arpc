package reliable

import (
	"net"
	"time"

	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/packet"
	"go.uber.org/zap"
)

// ReliableServerHandler implements the server-side reliable transport logic
type ReliableServerHandler struct {
	*ReliableHandler // Embed base handler
}

// NewReliableServerHandler creates a new reliable server handler with default 30s timeout
func NewReliableServerHandler(transportSender TransportSender, timerMgr TimerScheduler) *ReliableServerHandler {
	return NewReliableServerHandlerWithTimeout(transportSender, timerMgr, 30*time.Second)
}

// NewReliableServerHandlerWithTimeout creates a new reliable server handler with custom timeout
func NewReliableServerHandlerWithTimeout(transportSender TransportSender, timerMgr TimerScheduler, timeout time.Duration) *ReliableServerHandler {
	handler := &ReliableServerHandler{
		ReliableHandler: newReliableHandler(transportSender, timerMgr, timeout),
	}

	// Cache ACK packet type during initialization
	ackPacketType, exists := transportSender.GetPacketRegistry().GetPacketTypeByName(AckPacketName)
	if !exists {
		logging.Fatal("ACK packet type not registered in transport - ensure ACK packet type is registered before creating reliable handler")
	}
	handler.ackPacketType = &ackPacketType

	handler.startCleanupTimer(TimerKeyReliableServerCleanup)

	logging.Debug("Reliable server handler created",
		zap.Duration("timeout", timeout))

	return handler
}

// OnReceive handles incoming REQUEST and ACK(kind=1) packets
func (h *ReliableServerHandler) OnReceive(pkt any, addr *net.UDPAddr) error {
	switch p := pkt.(type) {
	case *packet.DataPacket:
		if p.PacketTypeID == packet.PacketTypeRequest.TypeID {
			return h.handleReceiveRequest(p, addr)
		}
	case *ACKPacket:
		if p.Kind == 1 { // ACK for RESPONSE
			return h.handleReceiveACK(p, addr)
		}
	}
	return nil
}

// OnSend handles outgoing RESPONSE packets and ACK(kind=0) packets
func (h *ReliableServerHandler) OnSend(pkt any, addr *net.UDPAddr) error {
	switch p := pkt.(type) {
	case *packet.DataPacket:
		if p.PacketTypeID == packet.PacketTypeResponse.TypeID {
			return h.handleSendResponse(p)
		}
	case *ACKPacket:
		if p.Kind == 0 { // ACK for REQUEST
			return h.handleSendACK(p, addr)
		}
	}
	return nil
}

// handleReceiveRequest processes incoming REQUEST packets
func (h *ReliableServerHandler) handleReceiveRequest(pkt *packet.DataPacket, addr *net.UDPAddr) error {
	return h.handleReceiveDataPacket(pkt, addr, 0, "REQUEST", "client")
}

// handleReceiveACK processes ACK packets for RESPONSEs
func (h *ReliableServerHandler) handleReceiveACK(ack *ACKPacket, addr *net.UDPAddr) error {
	return h.ReliableHandler.handleReceiveACK(ack, addr, "RESPONSE", "client")
}

// handleSendResponse tracks outgoing RESPONSE packets
func (h *ReliableServerHandler) handleSendResponse(pkt *packet.DataPacket) error {
	return h.handleSendDataPacket(pkt, "RESPONSE")
}

// handleSendACK handles sending ACK packets (already formed, just update activity)
func (h *ReliableServerHandler) handleSendACK(ack *ACKPacket, addr *net.UDPAddr) error {
	return h.ReliableHandler.handleSendACK(ack, addr, "REQUEST", "client")
}

// Cleanup cleans up resources
func (h *ReliableServerHandler) Cleanup() {
	h.ReliableHandler.Cleanup(TimerKeyReliableServerCleanup)
}
