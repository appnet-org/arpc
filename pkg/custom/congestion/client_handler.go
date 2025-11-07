package congestion

import (
	"net"

	"github.com/appnet-org/arpc/pkg/custom/congestion/cubic"
	"github.com/appnet-org/arpc/pkg/custom/congestion/cubic/protocol"
	"github.com/appnet-org/arpc/pkg/custom/congestion/cubic/utils"
	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/packet"
	"github.com/appnet-org/arpc/pkg/transport"
	"go.uber.org/zap"
)

// CCClientHandler implements the client-side congestion control logic
type CCClientHandler struct {
	*CCHandler // Embed base handler
}

// NewCCClientHandler creates a new congestion control client handler with default configuration
func NewCCClientHandler(
	transportSender TransportSender,
	timerMgr TimerScheduler,
) *CCClientHandler {
	return NewCCClientHandlerWithConfig(
		transportSender,
		timerMgr,
		defaultFeedbackInterval,
	)
}

// NewCCClientHandlerWithConfig creates a new congestion control client handler with custom configuration
func NewCCClientHandlerWithConfig(
	transportSender TransportSender,
	timerMgr TimerScheduler,
	feedbackInterval uint32,
) *CCClientHandler {
	// Create CUBIC algorithm with defaults
	ccAlgorithm := cubic.NewCubicSender(
		cubic.DefaultClock{},
		utils.NewRTTStats(),
		&utils.ConnectionStats{},
		protocol.ByteCount(defaultMTU),
		false, // reno=false, use CUBIC
	)

	handler := &CCClientHandler{
		CCHandler: newCCHandler(
			feedbackInterval,
			ccAlgorithm,
			transportSender,
			timerMgr,
		),
	}

	// Start periodic timers
	handler.startCleanupTimer(transport.TimerKey("cc_client_cleanup"))
	handler.startTimeoutCheckTimer(transport.TimerKey("cc_client_timeout_check"))

	logging.Debug("CC client handler created",
		zap.Uint32("feedbackInterval", feedbackInterval))

	return handler
}

// OnSend handles outgoing packets (client side)
// Tracks REQUEST packets and ignores CCFeedback packets (just updates activity)
func (h *CCClientHandler) OnSend(pkt any, addr *net.UDPAddr) error {
	switch p := pkt.(type) {
	case *packet.DataPacket:
		// Only handle REQUEST packets (TypeID 1)
		if p.PacketTypeID == packet.PacketTypeRequest.TypeID {
			// Extract server connection ID from destination
			connID := ConnectionID{IP: p.DstIP, Port: p.DstPort}
			key := connID.String()
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
		key := connID.String()
		h.getOrCreateConnection(key, connID)
		return nil
	}
	return nil
}

// OnReceive handles incoming packets (client side)
// Tracks RESPONSE packets and processes CCFeedback packets
func (h *CCClientHandler) OnReceive(pkt any, addr *net.UDPAddr) error {
	switch p := pkt.(type) {
	case *packet.DataPacket:
		// Handle RESPONSE packets (TypeID 2)
		if p.PacketTypeID == packet.PacketTypeResponse.TypeID {
			// Extract server connection ID from source
			connID := ConnectionID{IP: p.SrcIP, Port: p.SrcPort}
			key := connID.String()
			h.getOrCreateConnection(key, connID)
			return h.trackReceivedPacket(p, key)
		}
	case *CCFeedbackPacket:
		// Process CCFeedback from server
		// Extract connection ID from address
		var connID ConnectionID
		if ip4 := addr.IP.To4(); ip4 != nil {
			copy(connID.IP[:], ip4)
		}
		connID.Port = uint16(addr.Port)
		key := connID.String()
		h.getOrCreateConnection(key, connID)
		return h.processFeedback(p, key)
	}
	return nil
}

// Cleanup cleans up resources
func (h *CCClientHandler) Cleanup() {
	h.CCHandler.Cleanup(
		transport.TimerKey("cc_client_cleanup"),
		transport.TimerKey("cc_client_timeout_check"),
	)
}
