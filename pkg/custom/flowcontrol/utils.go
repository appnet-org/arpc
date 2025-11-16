package flowcontrol

import (
	"fmt"
	"net"
	"sync"
	"time"

	flowcontrol "github.com/appnet-org/arpc/pkg/custom/flowcontrol/quic-flowcontrol"
	"github.com/appnet-org/arpc/pkg/custom/flowcontrol/quic-flowcontrol/monotime"
	"github.com/appnet-org/arpc/pkg/custom/flowcontrol/quic-flowcontrol/protocol"
	"github.com/appnet-org/arpc/pkg/custom/flowcontrol/quic-flowcontrol/utils"
	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/packet"
	"github.com/appnet-org/arpc/pkg/transport"
	"go.uber.org/zap"
)

const (
	defaultInitialReceiveWindow = 15 * 1024 * 1024 // 15 MB
	defaultMaxReceiveWindow     = 25 * 1024 * 1024 // 25 MB
	defaultConnectionTimeout    = 30 * time.Second
)

// Predefined timer key constants for flow control timers
const (
	TimerKeyFCClientCleanup transport.TimerKey = 20
	TimerKeyFCServerCleanup transport.TimerKey = 21
)

// ConnectionID uniquely identifies a connection
type ConnectionID struct {
	IP   [4]byte
	Port uint16
}

// Key returns a binary uint64 representation for efficient map key usage
// Format: IP (4 bytes in high 48 bits) | Port (2 bytes in low 16 bits)
func (c ConnectionID) Key() uint64 {
	return uint64(c.IP[0])<<40 | uint64(c.IP[1])<<32 |
		uint64(c.IP[2])<<24 | uint64(c.IP[3])<<16 | uint64(c.Port)
}

// String returns a string representation of the connection ID for logging
func (c ConnectionID) String() string {
	return fmt.Sprintf("%d.%d.%d.%d:%d", c.IP[0], c.IP[1], c.IP[2], c.IP[3], c.Port)
}

// FCConnectionState tracks flow control state for a single connection
type FCConnectionState struct {
	ConnID         ConnectionID
	LastActivity   time.Time
	FlowController flowcontrol.ConnectionFlowController // Connection-level flow control
}

// newFCConnectionState creates a new connection state
func newFCConnectionState(connID ConnectionID, initialReceiveWindow, maxReceiveWindow protocol.ByteCount) *FCConnectionState {

	// allowWindowIncrease callback - for now, always allow window increases
	allowWindowIncrease := func(size protocol.ByteCount) bool {
		return true
	}

	flowController := flowcontrol.NewConnectionFlowController(
		initialReceiveWindow,
		maxReceiveWindow,
		allowWindowIncrease,
		utils.NewRTTStats(),
	)

	return &FCConnectionState{
		ConnID:         connID,
		LastActivity:   time.Now(),
		FlowController: flowController,
	}
}

// TransportSender interface for sending packets (avoid circular dependency)
type TransportSender interface {
	Send(addr string, rpcID uint64, data []byte, pktType packet.PacketType) error
	GetPacketRegistry() *packet.PacketRegistry
	GetConn() *net.UDPConn
}

// TimerScheduler interface for managing timers
type TimerScheduler interface {
	Schedule(id transport.TimerKey, duration time.Duration, callback transport.TimerCallback)
	SchedulePeriodic(id transport.TimerKey, interval time.Duration, callback transport.TimerCallback)
	StopTimer(id transport.TimerKey) bool
}

// FCHandler is the base handler containing common state and logic
type FCHandler struct {
	connections          map[uint64]*FCConnectionState
	initialReceiveWindow protocol.ByteCount
	maxReceiveWindow     protocol.ByteCount
	defaultTimeout       time.Duration
	mu                   sync.RWMutex
	transport            TransportSender
	timerMgr             TimerScheduler
	fcFeedbackPktType    *packet.PacketType // Cached FCFeedback packet type
}

// newFCHandler creates a new base flow control handler
func newFCHandler(
	initialReceiveWindow protocol.ByteCount,
	maxReceiveWindow protocol.ByteCount,
	transportSender TransportSender,
	timerMgr TimerScheduler,
) *FCHandler {
	return &FCHandler{
		connections:          make(map[uint64]*FCConnectionState),
		initialReceiveWindow: initialReceiveWindow,
		maxReceiveWindow:     maxReceiveWindow,
		defaultTimeout:       defaultConnectionTimeout,
		transport:            transportSender,
		timerMgr:             timerMgr,
	}
}

// getOrCreateConnection gets or creates a connection state, updating LastActivity
func (h *FCHandler) getOrCreateConnection(key uint64, connID ConnectionID) *FCConnectionState {
	h.mu.Lock()
	defer h.mu.Unlock()

	if conn, exists := h.connections[key]; exists {
		conn.LastActivity = time.Now()
		return conn
	}

	conn := newFCConnectionState(connID, h.initialReceiveWindow, h.maxReceiveWindow)
	h.connections[key] = conn

	logging.Debug("Created new FC connection state",
		zap.Uint64("key", key),
		zap.String("connID", connID.String()),
		zap.Int64("initialReceiveWindow", int64(h.initialReceiveWindow)),
		zap.Int64("maxReceiveWindow", int64(h.maxReceiveWindow)))

	return conn
}

// trackSentPacket tracks an outgoing packet (sender side)
func (h *FCHandler) trackSentPacket(dataPkt *packet.DataPacket, connKey uint64) error {
	h.mu.RLock()
	conn := h.connections[connKey]
	h.mu.RUnlock()

	if conn == nil {
		return nil
	}

	bytes := protocol.ByteCount(len(dataPkt.Payload))

	// Check send window before sending
	sendWindow := conn.FlowController.SendWindowSize()
	if sendWindow == 0 {
		logging.Debug("Flow control blocked: send window is 0",
			zap.Uint64("connKey", connKey),
			zap.String("connID", conn.ConnID.String()))
		// return fmt.Errorf("flow control blocked: send window is 0")
	}

	if sendWindow < bytes {
		logging.Debug("Flow control warning: send window smaller than packet",
			zap.Uint64("connKey", connKey),
			zap.Int64("sendWindow", int64(sendWindow)),
			zap.Int64("packetSize", int64(bytes)))
		// return fmt.Errorf("flow control warning: send window smaller than packet")
	}

	// Add bytes sent
	conn.FlowController.AddBytesSent(bytes)

	logging.Debug("Tracked sent packet",
		zap.Uint64("connKey", connKey),
		zap.Int64("bytes", int64(bytes)),
		zap.Int64("sendWindow", int64(conn.FlowController.SendWindowSize())))

	return nil
}

// trackReceivedPacket tracks an incoming packet (receiver side)
func (h *FCHandler) trackReceivedPacket(dataPkt *packet.DataPacket, connKey uint64) error {
	h.mu.RLock()
	conn := h.connections[connKey]
	h.mu.RUnlock()

	if conn == nil {
		return nil
	}

	bytes := protocol.ByteCount(len(dataPkt.Payload))

	// Add bytes read (application has consumed the data)
	// This internally increments highest received and checks for flow control violations
	hasWindowUpdate := conn.FlowController.AddBytesRead(bytes)

	logging.Debug("Tracked received packet",
		zap.Uint64("connKey", connKey),
		zap.Int64("bytes", int64(bytes)),
		zap.Bool("hasWindowUpdate", hasWindowUpdate))

	// If window update needed, send feedback
	if hasWindowUpdate {
		h.sendFeedback(conn, connKey)
	}

	return nil
}

// sendFeedback sends a FCFeedback packet (receiver side)
// Address is derived from ConnID
func (h *FCHandler) sendFeedback(conn *FCConnectionState, connKey uint64) {
	now := monotime.FromTime(time.Now())

	// Get window update
	newWindow := conn.FlowController.GetWindowUpdate(now)
	if newWindow == 0 {
		// No update needed
		return
	}

	// Build feedback packet
	feedback := &FCFeedbackPacket{
		PacketTypeID: h.fcFeedbackPktType.TypeID,
		SendWindow:   uint64(newWindow),
	}

	// Serialize feedback
	feedbackData, err := (&FCFeedbackCodec{}).Serialize(feedback)
	if err != nil {
		logging.Error("Failed to serialize FCFeedback packet", zap.Error(err))
		return
	}

	// Derive address from ConnID
	addr := &net.UDPAddr{
		IP:   net.IP(conn.ConnID.IP[:]),
		Port: int(conn.ConnID.Port),
	}

	logging.Debug("Sending FCFeedback packet",
		zap.Uint64("connKey", connKey),
		zap.String("addr", addr.String()),
		zap.Uint64("sendWindow", feedback.SendWindow))

	// Send FCFeedback packet directly via UDP (bypass handler chain)
	_, err = h.transport.GetConn().WriteToUDP(feedbackData, addr)
	if err != nil {
		logging.Error("Failed to send FCFeedback packet", zap.Error(err))
		return
	}

	logging.Debug("Sent FCFeedback",
		zap.Uint64("connKey", connKey),
		zap.Uint64("sendWindow", feedback.SendWindow))
}

// processFeedback processes an incoming FCFeedback packet (sender side)
func (h *FCHandler) processFeedback(feedback *FCFeedbackPacket, connKey uint64) error {
	h.mu.RLock()
	conn := h.connections[connKey]
	h.mu.RUnlock()

	if conn == nil {
		return nil
	}

	// Update send window
	updated := conn.FlowController.UpdateSendWindow(protocol.ByteCount(feedback.SendWindow))

	if updated {
		logging.Debug("Updated send window from feedback",
			zap.Uint64("connKey", connKey),
			zap.Uint64("newSendWindow", feedback.SendWindow),
			zap.Int64("sendWindowSize", int64(conn.FlowController.SendWindowSize())))
	}

	return nil
}

// cleanupExpiredConnections removes connections that have timed out
func (h *FCHandler) cleanupExpiredConnections() {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	for key, conn := range h.connections {
		if now.Sub(conn.LastActivity) > h.defaultTimeout {
			delete(h.connections, key)
			logging.Debug("FC connection timeout, removed state",
				zap.Uint64("key", key),
				zap.Duration("timeout", h.defaultTimeout),
				zap.Duration("elapsed", now.Sub(conn.LastActivity)))
		}
	}
}

// startCleanupTimer starts the periodic cleanup timer
func (h *FCHandler) startCleanupTimer(timerKey transport.TimerKey) {
	h.timerMgr.SchedulePeriodic(
		timerKey,
		1*time.Second, // Check every second
		transport.TimerCallback(func() {
			h.cleanupExpiredConnections()
		}),
	)
}

// Cleanup cleans up resources
func (h *FCHandler) Cleanup(cleanupTimerKey transport.TimerKey) {
	h.timerMgr.StopTimer(cleanupTimerKey)
}

// GetConnectionInfo returns flow control info for debugging (optional)
func (h *FCHandler) GetConnectionInfo(connID ConnectionID) (sendWindow, receiveWindow protocol.ByteCount, exists bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	key := connID.Key()
	conn, exists := h.connections[key]
	if !exists {
		return 0, 0, false
	}

	return conn.FlowController.SendWindowSize(), 0, true // receiveWindow is internal to flow controller
}
