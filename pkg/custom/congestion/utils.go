package congestion

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/appnet-org/arpc/pkg/custom/congestion/cubic"
	"github.com/appnet-org/arpc/pkg/custom/congestion/cubic/monotime"
	"github.com/appnet-org/arpc/pkg/custom/congestion/cubic/protocol"
	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/packet"
	"github.com/appnet-org/arpc/pkg/transport"
	"go.uber.org/zap"
)

const (
	defaultFeedbackInterval  = 10
	defaultMTU               = 1400
	defaultConnectionTimeout = 30 * time.Second
	defaultPacketTimeout     = 200 * time.Millisecond // Base timeout per packet
)

// Predefined timer key constants for congestion control timers
const (
	TimerKeyCCClientCleanup     transport.TimerKey = 10
	TimerKeyCCServerCleanup     transport.TimerKey = 11
	TimerKeyCCPacketTimeoutBase transport.TimerKey = 10000
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

// makePacketID creates a monotonic packet ID from RPCID and SeqNum
// Formula: (RPCID << 16) | SeqNum
// This works because RPCID is monotonically increasing (timestamp-based)
// and SeqNum is 0-indexed within each RPC (0-65535)
func makePacketID(rpcID uint64, seqNum uint16) uint64 {
	return (rpcID << 16) | uint64(seqNum)
}

// SentPacketInfo tracks a sent packet for bytes-in-flight calculation
type SentPacketInfo struct {
	packetID uint64    // Monotonic packet ID from (RPCID, SeqNum)
	bytes    uint64    // Packet size in bytes
	sendTime time.Time // When packet was sent
}

// CCConnectionState tracks congestion control state for a single connection
type CCConnectionState struct {
	ConnID       ConnectionID
	LastActivity time.Time

	// Tx side (sent packets)
	SentPackets   map[uint64]*SentPacketInfo // packetID → info
	bytesInFlight uint64                     // Running total of bytes in flight (optimization to avoid O(n) calculation)

	// Rx side (received packets)
	ReceivedPackets map[uint64]uint64 // packetID → bytes
	feedbackCount   uint32            // Count for feedback packets
}

// newCCConnectionState creates a new connection state
func newCCConnectionState(connID ConnectionID) *CCConnectionState {
	return &CCConnectionState{
		ConnID:          connID,
		LastActivity:    time.Now(),
		SentPackets:     make(map[uint64]*SentPacketInfo),
		ReceivedPackets: make(map[uint64]uint64),
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

// CCHandler is the base handler containing common state and logic
type CCHandler struct {
	connections       map[uint64]*CCConnectionState
	feedbackInterval  uint32
	defaultTimeout    time.Duration
	packetTimeout     time.Duration // timeout * feedbackInterval
	mu                sync.RWMutex
	transport         TransportSender
	timerMgr          TimerScheduler
	ccAlgorithm       cubic.SendAlgorithm
	ccFeedbackPktType *packet.PacketType // Cached CCFeedback packet type
}

// newCCHandler creates a new base congestion control handler
func newCCHandler(
	feedbackInterval uint32,
	ccAlgorithm cubic.SendAlgorithm,
	transportSender TransportSender,
	timerMgr TimerScheduler,
) *CCHandler {
	return &CCHandler{
		connections:      make(map[uint64]*CCConnectionState),
		feedbackInterval: feedbackInterval,
		defaultTimeout:   defaultConnectionTimeout,
		packetTimeout:    defaultPacketTimeout * time.Duration(feedbackInterval),
		transport:        transportSender,
		timerMgr:         timerMgr,
		ccAlgorithm:      ccAlgorithm,
	}
}

// getOrCreateConnection gets or creates a connection state, updating LastActivity
func (h *CCHandler) getOrCreateConnection(key uint64, connID ConnectionID) *CCConnectionState {
	h.mu.Lock()
	defer h.mu.Unlock()

	if conn, exists := h.connections[key]; exists {
		conn.LastActivity = time.Now()
		return conn
	}

	conn := newCCConnectionState(connID)
	h.connections[key] = conn

	logging.Debug("Created new CC connection state",
		zap.Uint64("key", key),
		zap.String("connID", connID.String()))

	return conn
}

// trackSentPacket tracks an outgoing packet (sender side)
func (h *CCHandler) trackSentPacket(dataPkt *packet.DataPacket, connKey uint64) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	conn := h.connections[connKey]
	if conn == nil {
		return nil
	}

	// Create monotonic packet ID
	packetID := makePacketID(dataPkt.RPCID, dataPkt.SeqNumber)
	bytes := uint64(len(dataPkt.Payload))
	now := time.Now()
	nowMonotime := monotime.FromTime(now)

	// Get current bytes in flight (maintained as running total)
	bytesInFlight := conn.bytesInFlight

	// Check HasPacingBudget first
	if !h.ccAlgorithm.HasPacingBudget(nowMonotime) {
		timeUntilSend := h.ccAlgorithm.TimeUntilSend(protocol.ByteCount(bytesInFlight))
		timeUntilSendDuration := timeUntilSend.Sub(nowMonotime)
		logging.Debug("HasPacingBudget(): Does not have pacing budget, can send after duration",
			zap.Duration("timeUntilSend", timeUntilSendDuration))
		// return fmt.Errorf("HasPacingBudget(): Does not have pacing budget, can send after %v from now", timeUntilSendDuration)
	}

	// Check CanSend
	if !h.ccAlgorithm.CanSend(protocol.ByteCount(bytesInFlight)) {
		logging.Debug("CanSend(): Does not have send budget", zap.Uint64("bytesInFlight", bytesInFlight))
		// return fmt.Errorf("CanSend(): Does not have send budget, bytesInFlight=%d", bytesInFlight)
	}

	// Track packet
	packetInfo := &SentPacketInfo{
		packetID: packetID,
		bytes:    bytes,
		sendTime: now,
	}
	conn.SentPackets[packetID] = packetInfo
	// Update running total
	conn.bytesInFlight += bytes

	// Schedule timeout for this packet (use uint64 arithmetic for timer key)
	timerKey := transport.TimerKey(uint64(TimerKeyCCPacketTimeoutBase) + packetID)
	h.timerMgr.Schedule(
		timerKey,
		h.packetTimeout,
		transport.TimerCallback(func() {
			h.checkTimeoutPacket(connKey, packetID)
		}),
	)

	// Call CUBIC: OnPacketSent
	h.ccAlgorithm.OnPacketSent(
		nowMonotime,
		protocol.ByteCount(bytesInFlight),
		protocol.PacketNumber(packetID),
		protocol.ByteCount(bytes),
		false, // No reliable guarantees, so we don't consider it retransmittable
	)

	return nil
}

// trackReceivedPacket tracks an incoming packet (receiver side)
func (h *CCHandler) trackReceivedPacket(dataPkt *packet.DataPacket, connKey uint64) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	conn := h.connections[connKey]
	if conn == nil {
		return nil
	}

	// Create monotonic packet ID (same formula as sender!)
	packetID := makePacketID(dataPkt.RPCID, dataPkt.SeqNumber)
	bytes := uint64(len(dataPkt.Payload))

	// Track received packet
	conn.ReceivedPackets[packetID] = bytes
	conn.feedbackCount++

	// Check if we should send feedback (only based on count)
	if conn.feedbackCount >= h.feedbackInterval && len(conn.ReceivedPackets) > 0 {
		// Send feedback without holding lock
		h.mu.Unlock()
		h.sendFeedback(conn, connKey)
		h.mu.Lock()
	}

	return nil
}

// sendFeedback sends a CCFeedback packet (receiver side)
// Address is derived from ConnID
func (h *CCHandler) sendFeedback(conn *CCConnectionState, connKey uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(conn.ReceivedPackets) == 0 {
		return
	}

	// Build feedback packet
	var totalBytes uint64
	var packetIDs []uint64

	// Collect packet IDs and calculate total bytes
	for packetID, bytes := range conn.ReceivedPackets {
		totalBytes += bytes
		packetIDs = append(packetIDs, packetID)
	}

	feedback := &CCFeedbackPacket{
		PacketTypeID: h.ccFeedbackPktType.TypeID,
		AckedCount:   uint32(len(conn.ReceivedPackets)),
		AckedBytes:   totalBytes,
		PacketIDs:    packetIDs,
	}

	// Serialize feedback
	feedbackData, err := (&CCFeedbackCodec{}).Serialize(feedback, nil)
	if err != nil {
		logging.Error("Failed to serialize CCFeedback packet", zap.Error(err))
		return
	}

	// Derive address from ConnID
	addr := &net.UDPAddr{
		IP:   net.IP(conn.ConnID.IP[:]),
		Port: int(conn.ConnID.Port),
	}

	logging.Debug("Sending CCFeedback packet",
		zap.Uint64("connKey", connKey),
		zap.String("addr", addr.String()),
		zap.Uint32("ackedCount", feedback.AckedCount),
		zap.Uint64("ackedBytes", feedback.AckedBytes))

	// Send CCFeedback packet directly via UDP (bypass handler chain)
	// CCFeedback packets are small control packets that should never be fragmented
	_, err = h.transport.GetConn().WriteToUDP(feedbackData, addr)
	if err != nil {
		logging.Error("Failed to send CCFeedback packet", zap.Error(err))
		return
	}

	logging.Debug("Sent CCFeedback",
		zap.Uint64("connKey", connKey),
		zap.Uint32("ackedCount", feedback.AckedCount),
		zap.Uint64("ackedBytes", feedback.AckedBytes))

	// Reset aggregation state
	conn.ReceivedPackets = make(map[uint64]uint64)
	conn.feedbackCount = 0
}

// processFeedback processes an incoming CCFeedback packet (sender side)
func (h *CCHandler) processFeedback(feedback *CCFeedbackPacket, connKey uint64) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	conn := h.connections[connKey]
	if conn == nil {
		return nil
	}

	// Create a set of acked packet IDs for fast lookup
	ackedSet := make(map[uint64]bool)
	for _, packetID := range feedback.PacketIDs {
		ackedSet[packetID] = true
	}

	// Find smallest acked packet ID for loss detection
	var smallestAcked uint64
	if len(feedback.PacketIDs) > 0 {
		smallestAcked = feedback.PacketIDs[0]
		for _, packetID := range feedback.PacketIDs {
			if packetID < smallestAcked {
				smallestAcked = packetID
			}
		}
	}

	// Process ACKs and detect losses
	priorInFlight := conn.bytesInFlight
	var ackedPackets []uint64
	var lostPackets []uint64

	for packetID := range conn.SentPackets {
		if ackedSet[packetID] {
			// Packet was ACKed
			ackedPackets = append(ackedPackets, packetID)
		} else if packetID < smallestAcked {
			// Packet is smaller than smallest acked → lost
			lostPackets = append(lostPackets, packetID)
		}
	}

	// Call CUBIC OnPacketAcked for each acked packet
	for _, packetID := range ackedPackets {
		if info := conn.SentPackets[packetID]; info != nil {
			h.ccAlgorithm.OnPacketAcked(
				protocol.PacketNumber(packetID),
				protocol.ByteCount(info.bytes),
				protocol.ByteCount(priorInFlight),
				monotime.FromTime(time.Now()),
			)
			h.ccAlgorithm.MaybeExitSlowStart()
			priorInFlight -= info.bytes
			// Stop the timeout timer for this packet (use uint64 arithmetic)
			timerKey := transport.TimerKey(uint64(TimerKeyCCPacketTimeoutBase) + packetID)
			h.timerMgr.StopTimer(timerKey)

			// Remove acked packet and update running total
			delete(conn.SentPackets, packetID)
			// Safety check: only subtract if we have enough to avoid underflow
			if conn.bytesInFlight >= info.bytes {
				conn.bytesInFlight -= info.bytes
			} else {
				conn.bytesInFlight = 0
			}
		}
	}

	// Call CUBIC OnCongestionEvent for each lost packet
	for _, packetID := range lostPackets {
		if info := conn.SentPackets[packetID]; info != nil {
			// Stop the timeout timer for this packet (use uint64 arithmetic)
			timerKey := transport.TimerKey(uint64(TimerKeyCCPacketTimeoutBase) + packetID)
			h.timerMgr.StopTimer(timerKey)

			h.ccAlgorithm.OnCongestionEvent(
				protocol.PacketNumber(packetID),
				protocol.ByteCount(info.bytes),
				protocol.ByteCount(priorInFlight),
			)
			h.ccAlgorithm.MaybeExitSlowStart()
			priorInFlight -= info.bytes
			// Remove lost packet and update running total
			delete(conn.SentPackets, packetID)
			// Safety check: only subtract if we have enough to avoid underflow
			if conn.bytesInFlight >= info.bytes {
				conn.bytesInFlight -= info.bytes
			} else {
				conn.bytesInFlight = 0
			}
		}
	}

	logging.Debug("Processed feedback",
		zap.Uint64("connKey", connKey),
		zap.Int("ackedCount", len(ackedPackets)),
		zap.Int("lostCount", len(lostPackets)))

	return nil
}

// CanSend checks if we can send more data based on congestion window
// Returns true if bytesInFlight < congestionWindow
func (h *CCHandler) CanSend() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Calculate total bytes in flight across all connections
	var totalBytesInFlight uint64
	for _, conn := range h.connections {
		totalBytesInFlight += conn.bytesInFlight
	}

	return h.ccAlgorithm.CanSend(protocol.ByteCount(totalBytesInFlight))
}

// SetFeedbackInterval updates the feedback interval
func (h *CCHandler) SetFeedbackInterval(interval uint32) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.feedbackInterval = interval
	h.packetTimeout = defaultPacketTimeout * time.Duration(interval)
}

// cleanupExpiredConnections removes connections that have timed out
func (h *CCHandler) cleanupExpiredConnections() {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	for key, conn := range h.connections {
		if now.Sub(conn.LastActivity) > h.defaultTimeout {
			delete(h.connections, key)
			logging.Debug("CC connection timeout, removed state",
				zap.Uint64("key", key),
				zap.Duration("timeout", h.defaultTimeout),
				zap.Duration("elapsed", now.Sub(conn.LastActivity)))
		}
	}
}

// checkTimeoutPacket checks if a specific packet has timed out and assumes loss
func (h *CCHandler) checkTimeoutPacket(connKey uint64, packetID uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	conn := h.connections[connKey]
	if conn == nil {
		return
	}

	info, exists := conn.SentPackets[packetID]
	if !exists {
		// Packet was already ACKed or removed
		return
	}

	// Check if packet has timed out
	now := time.Now()
	if now.Sub(info.sendTime) > h.packetTimeout {
		// Packet timeout - assume loss
		logging.Debug("Packet timeout - assuming loss",
			zap.Uint64("connKey", connKey),
			zap.Uint64("packetID", packetID),
			zap.Uint64("timeoutBytes", info.bytes),
			zap.Duration("elapsed", now.Sub(info.sendTime)))

		// Call CUBIC: OnRetransmissionTimeout
		h.ccAlgorithm.OnRetransmissionTimeout(
			false, // packetsRetransmitted is false because we are not retransmitting packets
		)
		h.ccAlgorithm.MaybeExitSlowStart()

		// Remove timeout packet and update running total
		delete(conn.SentPackets, packetID)
		// Safety check: only subtract if we have enough to avoid underflow
		if conn.bytesInFlight >= info.bytes {
			conn.bytesInFlight -= info.bytes
		} else {
			conn.bytesInFlight = 0
		}
	}
}

// startCleanupTimer starts the periodic cleanup timer
func (h *CCHandler) startCleanupTimer(timerKey transport.TimerKey) {
	h.timerMgr.SchedulePeriodic(
		timerKey,
		1*time.Second, // Check every second
		transport.TimerCallback(func() {
			h.cleanupExpiredConnections()
		}),
	)
}

// Cleanup cleans up resources
func (h *CCHandler) Cleanup(cleanupTimerKey transport.TimerKey) {
	h.timerMgr.StopTimer(cleanupTimerKey)
	// Note: Individual packet timeout timers are automatically cleaned up when they fire
	// or when packets are ACKed. We don't need to stop them here.
}
