package congestion

import (
	"net"

	"github.com/appnet-org/arpc/pkg/custom/congestion/cubic"
	"github.com/appnet-org/arpc/pkg/custom/congestion/cubic/protocol"
	"github.com/appnet-org/arpc/pkg/custom/congestion/cubic/utils"
	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/packet"
	"go.uber.org/zap"
)

// CCServerHandler implements the server-side congestion control logic
type CCServerHandler struct {
	*CCHandler // Embed base handler
}

// NewCCServerHandler creates a new congestion control server handler with default configuration
func NewCCServerHandler(
	transportSender TransportSender,
	timerMgr TimerScheduler,
) *CCServerHandler {
	return NewCCServerHandlerWithConfig(
		transportSender,
		timerMgr,
		defaultFeedbackInterval,
	)
}

// NewCCServerHandlerWithConfig creates a new congestion control server handler with custom configuration
func NewCCServerHandlerWithConfig(
	transportSender TransportSender,
	timerMgr TimerScheduler,
	feedbackInterval uint32,
) *CCServerHandler {
	// Create CUBIC algorithm with defaults
	ccAlgorithm := cubic.NewCubicSender(
		cubic.DefaultClock{},
		utils.NewRTTStats(),
		&utils.ConnectionStats{},
		protocol.ByteCount(defaultMTU),
		false, // reno=false, use CUBIC
	)

	handler := &CCServerHandler{
		CCHandler: newCCHandler(
			feedbackInterval,
			ccAlgorithm,
			transportSender,
			timerMgr,
		),
	}

	// Cache CCFeedback packet type
	feedbackType, exists := transportSender.GetPacketRegistry().GetPacketTypeByName(CCFeedbackPacketName)
	if !exists {
		logging.Fatal("CCFeedback packet type not registered in transport - ensure CCFeedback packet type is registered before creating CC handler")
	}
	handler.ccFeedbackPktType = &feedbackType

	// Start periodic timers
	handler.startCleanupTimer(TimerKeyCCServerCleanup)

	logging.Debug("CC server handler created",
		zap.Uint32("feedbackInterval", feedbackInterval))

	return handler
}

// OnSend handles outgoing packets (server side)
// Tracks RESPONSE packets and ignores CCFeedback packets (just updates activity)
func (h *CCServerHandler) OnSend(pkt any, addr *net.UDPAddr) error {
	switch p := pkt.(type) {
	case *packet.DataPacket:
		// Only handle RESPONSE packets (TypeID 2)
		if p.PacketTypeID == packet.PacketTypeResponse.TypeID {
			// Extract client connection ID from destination
			connID := ConnectionID{IP: p.DstIP, Port: p.DstPort}
			key := connID.Key()
			h.getOrCreateConnection(key, connID)
			return h.trackSentPacket(p, key)
		}
	case *CCFeedbackPacket:
		// CCFeedback packets - just update activity
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

// OnReceive handles incoming packets (server side)
// Tracks REQUEST packets and processes CCFeedback packets
func (h *CCServerHandler) OnReceive(pkt any, addr *net.UDPAddr) error {
	switch p := pkt.(type) {
	case *packet.DataPacket:
		// Handle REQUEST packets (TypeID 1)
		if p.PacketTypeID == packet.PacketTypeRequest.TypeID {
			// Extract client connection ID from source
			connID := ConnectionID{IP: p.SrcIP, Port: p.SrcPort}
			key := connID.Key()
			h.getOrCreateConnection(key, connID)
			return h.trackReceivedPacket(p, key)
		}
	case *CCFeedbackPacket:
		// Process CCFeedback from client
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
func (h *CCServerHandler) Cleanup() {
	h.CCHandler.Cleanup(TimerKeyCCServerCleanup)
}
