package flowcontrol

import (
	"net"

	"github.com/appnet-org/arpc/pkg/custom/flowcontrol/quic-flowcontrol/protocol"
	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/packet"
	"go.uber.org/zap"
)

// FCClientHandler implements the client-side flow control logic
type FCClientHandler struct {
	*FCHandler // Embed base handler
}

// NewFCClientHandler creates a new flow control client handler with default configuration
func NewFCClientHandler(
	transportSender TransportSender,
	timerMgr TimerScheduler,
) *FCClientHandler {
	return NewFCClientHandlerWithConfig(
		transportSender,
		timerMgr,
		defaultInitialReceiveWindow,
		defaultMaxReceiveWindow,
	)
}

// NewFCClientHandlerWithConfig creates a new flow control client handler with custom configuration
func NewFCClientHandlerWithConfig(
	transportSender TransportSender,
	timerMgr TimerScheduler,
	initialReceiveWindow protocol.ByteCount,
	maxReceiveWindow protocol.ByteCount,
) *FCClientHandler {
	handler := &FCClientHandler{
		FCHandler: newFCHandler(
			initialReceiveWindow,
			maxReceiveWindow,
			transportSender,
			timerMgr,
		),
	}

	// Cache FCFeedback packet type
	feedbackType, exists := transportSender.GetPacketRegistry().GetPacketTypeByName(FCFeedbackPacketName)
	if !exists {
		logging.Fatal("FCFeedback packet type not registered in transport - ensure FCFeedback packet type is registered before creating FC handler")
	}
	handler.fcFeedbackPktType = &feedbackType

	// Start periodic timers
	handler.startCleanupTimer(TimerKeyFCClientCleanup)

	logging.Debug("FC client handler created",
		zap.Int64("initialReceiveWindow", int64(initialReceiveWindow)),
		zap.Int64("maxReceiveWindow", int64(maxReceiveWindow)))

	return handler
}

// OnSend handles outgoing packets (client side)
// Tracks REQUEST packets and ignores FCFeedback packets (just updates activity)
func (h *FCClientHandler) OnSend(pkt any, addr *net.UDPAddr) error {
	switch p := pkt.(type) {
	case *packet.DataPacket:
		// Only handle REQUEST packets (TypeID 1)
		if p.PacketTypeID == packet.PacketTypeRequest.TypeID {
			// Extract server connection ID from destination
			connID := ConnectionID{IP: p.DstIP, Port: p.DstPort}
			key := connID.Key()
			h.getOrCreateConnection(key, connID)
			return h.trackSentPacket(p, key)
		}
	case *FCFeedbackPacket:
		// FCFeedback packets - just update activity
		// Extract connection ID from address
		var connID ConnectionID
		if ip4 := addr.IP.To4(); ip4 != nil {
			copy(connID.IP[:], ip4)
		}
		connID.Port = uint16(addr.Port)
		key := connID.Key()
		h.getOrCreateConnection(key, connID)
		return nil
	}
	return nil
}

// OnReceive handles incoming packets (client side)
// Tracks RESPONSE packets and processes FCFeedback packets
func (h *FCClientHandler) OnReceive(pkt any, addr *net.UDPAddr) error {
	switch p := pkt.(type) {
	case *packet.DataPacket:
		// Handle RESPONSE packets (TypeID 2)
		if p.PacketTypeID == packet.PacketTypeResponse.TypeID {
			// Extract server connection ID from source
			connID := ConnectionID{IP: p.SrcIP, Port: p.SrcPort}
			key := connID.Key()
			h.getOrCreateConnection(key, connID)
			return h.trackReceivedPacket(p, key)
		}
	case *FCFeedbackPacket:
		// Process FCFeedback from server
		// Extract connection ID from address
		var connID ConnectionID
		if ip4 := addr.IP.To4(); ip4 != nil {
			copy(connID.IP[:], ip4)
		}
		connID.Port = uint16(addr.Port)
		key := connID.Key()
		h.getOrCreateConnection(key, connID)
		return h.processFeedback(p, key)
	}
	return nil
}

// Cleanup cleans up resources
func (h *FCClientHandler) Cleanup() {
	h.FCHandler.Cleanup(TimerKeyFCClientCleanup)
}
