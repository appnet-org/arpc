package reliable

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/packet"
	"github.com/appnet-org/arpc/pkg/transport"
	"go.uber.org/zap"
)

// Bitset represents a simple bitset for tracking received segments
type Bitset struct {
	bits []uint64
}

// NewBitset creates a new bitset with the given size
func NewBitset(size uint32) *Bitset {
	return &Bitset{
		bits: make([]uint64, (size+63)/64), // Round up to nearest 64-bit boundary
	}
}

// Set sets the bit at the given index
func (b *Bitset) Set(index uint32, value bool) {
	if index >= uint32(len(b.bits)*64) {
		return // Out of bounds
	}
	wordIndex := index / 64
	bitIndex := index % 64

	if value {
		b.bits[wordIndex] |= 1 << bitIndex
	} else {
		b.bits[wordIndex] &^= 1 << bitIndex
	}
}

// Get gets the bit at the given index
func (b *Bitset) Get(index uint32) bool {
	if index >= uint32(len(b.bits)*64) {
		return false // Out of bounds
	}
	wordIndex := index / 64
	bitIndex := index % 64
	return (b.bits[wordIndex] & (1 << bitIndex)) != 0
}

// PopCount returns the number of set bits
func (b *Bitset) PopCount() uint32 {
	count := uint32(0)
	for _, word := range b.bits {
		count += popCount64(word)
	}
	return count
}

// popCount64 counts the number of set bits in a 64-bit word
func popCount64(x uint64) uint32 {
	x = x - ((x >> 1) & 0x5555555555555555)
	x = (x & 0x3333333333333333) + ((x >> 2) & 0x3333333333333333)
	x = (x + (x >> 4)) & 0x0f0f0f0f0f0f0f0f
	x = x + (x >> 8)
	x = x + (x >> 16)
	x = x + (x >> 32)
	return uint32(x & 0x7f)
}

// ConnectionID uniquely identifies a connection
type ConnectionID struct {
	IP   [4]byte
	Port uint16
}

// String returns a string representation of the connection ID for use as map key
func (c ConnectionID) String() string {
	return fmt.Sprintf("%d.%d.%d.%d:%d", c.IP[0], c.IP[1], c.IP[2], c.IP[3], c.Port)
}

// MsgTx represents a transmitted message state
type MsgTx struct {
	Count      uint32
	SendTs     time.Time
	DstAddr    string            // Destination address for retransmission
	PacketType packet.PacketType // Packet type for retransmission
	Segments   map[uint16][]byte // Buffered packet data by segment number
}

// ConnectionState tracks the state of a single connection
type ConnectionState struct {
	ConnID       ConnectionID
	LastActivity time.Time

	// Tx tracking (REQUEST for client, RESPONSE for server)
	TxMsg map[uint64]*MsgTx

	// Rx tracking (RESPONSE for client, REQUEST for server)
	RxMsgSeen     map[uint64]*Bitset
	RxMsgCount    map[uint64]uint32
	RxMsgComplete map[uint64]bool
}

// newConnectionState creates a new connection state
func newConnectionState(connID ConnectionID) *ConnectionState {
	return &ConnectionState{
		ConnID:        connID,
		LastActivity:  time.Now(),
		TxMsg:         make(map[uint64]*MsgTx),
		RxMsgSeen:     make(map[uint64]*Bitset),
		RxMsgCount:    make(map[uint64]uint32),
		RxMsgComplete: make(map[uint64]bool),
	}
}

// TransportSender interface for sending packets
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

// ReliableHandler is the base handler containing common state and logic
type ReliableHandler struct {
	connections    map[string]*ConnectionState
	defaultTimeout time.Duration
	mu             sync.RWMutex
	transport      TransportSender
	timerMgr       TimerScheduler
}

// newReliableHandler creates a new base reliable handler
func newReliableHandler(transportSender TransportSender, timerMgr TimerScheduler, timeout time.Duration) *ReliableHandler {
	return &ReliableHandler{
		connections:    make(map[string]*ConnectionState),
		defaultTimeout: timeout,
		transport:      transportSender,
		timerMgr:       timerMgr,
	}
}

// getOrCreateConnection gets or creates a connection state, updating LastActivity
// This is the public version that acquires its own lock
func (h *ReliableHandler) getOrCreateConnection(key string, connID ConnectionID) *ConnectionState {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.getOrCreateConnectionLocked(key, connID)
}

// getOrCreateConnectionLocked gets or creates a connection state, updating LastActivity
// This is the internal version that assumes the lock is already held
func (h *ReliableHandler) getOrCreateConnectionLocked(key string, connID ConnectionID) *ConnectionState {
	if conn, exists := h.connections[key]; exists {
		conn.LastActivity = time.Now()
		return conn
	}

	conn := newConnectionState(connID)
	h.connections[key] = conn

	logging.Debug("Created new connection state",
		zap.String("key", key),
		zap.String("connID", connID.String()))

	return conn
}

// serializeDataPacket serializes a DataPacket for buffering/retransmission
func (h *ReliableHandler) serializeDataPacket(pkt *packet.DataPacket) ([]byte, error) {
	// Get the codec for DataPacket from the registry
	registry := h.transport.GetPacketRegistry()
	codec, exists := registry.GetCodec(pkt.PacketTypeID)
	if !exists {
		return nil, errors.New("codec not found for packet type")
	}
	return codec.Serialize(pkt)
}

// sendACK sends an ACK packet
func (h *ReliableHandler) sendACK(rpcID uint64, kind uint8, addr *net.UDPAddr) error {
	// Get ACK packet type from registry
	ackPacketType, exists := h.transport.GetPacketRegistry().GetPacketTypeByName(AckPacketName)
	if !exists {
		logging.Error("ACK packet type not registered in transport")
		return errors.New("ACK packet type not registered")
	}

	ackPkt := &ACKPacket{
		PacketTypeID: ackPacketType.TypeID,
		RPCID:        rpcID,
		Kind:         kind,
		Status:       0, // Success
		Timestamp:    time.Now().UnixMicro(),
		Message:      "",
	}

	// Serialize the ACK packet
	ackData, err := (&ACKPacketCodec{}).Serialize(ackPkt)
	if err != nil {
		logging.Error("Failed to serialize ACK packet", zap.Error(err))
		return err
	}

	logging.Debug("Sending ACK packet",
		zap.Uint64("rpcID", rpcID),
		zap.Uint8("kind", kind),
		zap.String("addr", addr.String()))

	// Send ACK packet directly via UDP (bypass fragmentation)
	// ACK packets are small control packets that should never be fragmented
	_, err = h.transport.GetConn().WriteToUDP(ackData, addr)
	if err != nil {
		logging.Error("Failed to send ACK packet", zap.Error(err))
		return err
	}

	return nil
}

// cleanupExpiredConnections removes connections that have timed out
func (h *ReliableHandler) cleanupExpiredConnections() {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	for key, conn := range h.connections {
		if now.Sub(conn.LastActivity) > h.defaultTimeout {
			delete(h.connections, key)
			logging.Debug("Connection timeout, removed state",
				zap.String("key", key),
				zap.Duration("timeout", h.defaultTimeout),
				zap.Duration("elapsed", now.Sub(conn.LastActivity)))
		}
	}
}

// startCleanupTimer starts the periodic cleanup timer
func (h *ReliableHandler) startCleanupTimer(timerKey transport.TimerKey) {
	h.timerMgr.SchedulePeriodic(
		timerKey,
		1*time.Second, // Check every second
		transport.TimerCallback(func() {
			h.cleanupExpiredConnections()
		}),
	)
}

// checkRetransmission checks if a specific message needs retransmission
func (h *ReliableHandler) checkRetransmission(connKey string, rpcID uint64, timeout time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()

	conn := h.connections[connKey]
	if conn == nil {
		return
	}

	msgTx, exists := conn.TxMsg[rpcID]
	if !exists {
		// Message was already ACKed or removed
		return
	}

	// SendTs.IsZero() means message has been ACKed
	if msgTx.SendTs.IsZero() {
		return
	}

	// Check if message has timed out
	now := time.Now()
	if now.Sub(msgTx.SendTs) > timeout {
		logging.Debug("Message retransmission timeout detected",
			zap.Uint64("rpcID", rpcID),
			zap.String("connection", connKey),
			zap.Duration("elapsed", now.Sub(msgTx.SendTs)),
			zap.Duration("timeout", timeout),
			zap.Int("segments", len(msgTx.Segments)))

		// Retransmit all segments
		for seqNum, segmentData := range msgTx.Segments {
			err := h.transport.Send(msgTx.DstAddr, rpcID, segmentData, msgTx.PacketType)
			if err != nil {
				logging.Error("Failed to retransmit segment",
					zap.Uint64("rpcID", rpcID),
					zap.Uint16("seqNumber", seqNum),
					zap.String("connection", connKey),
					zap.Error(err))
			} else {
				logging.Debug("Retransmitted segment",
					zap.Uint64("rpcID", rpcID),
					zap.Uint16("seqNumber", seqNum),
					zap.String("connection", connKey))
			}
		}

		// Update SendTs to current time and reschedule timeout for next retry
		msgTx.SendTs = now

		// Reschedule timeout for next retry
		timerKey := transport.TimerKey(fmt.Sprintf("reliable_msg_timeout_%d", rpcID))
		h.timerMgr.Schedule(
			timerKey,
			timeout,
			transport.TimerCallback(func() {
				h.checkRetransmission(connKey, rpcID, timeout)
			}),
		)
	}
}

// handleSendDataPacket tracks outgoing data packets (REQUEST or RESPONSE)
// This is a common function used by both client and server handlers
func (h *ReliableHandler) handleSendDataPacket(pkt *packet.DataPacket, packetTypeName string) error {

	// Extract connection ID from destination
	connID := ConnectionID{IP: pkt.DstIP, Port: pkt.DstPort}
	key := connID.String()

	// Serialize packet for buffering (do this before acquiring lock to minimize lock time)
	serializedData, err := h.serializeDataPacket(pkt)
	if err != nil {
		logging.Error("Failed to serialize packet for buffering",
			zap.String("packetType", packetTypeName),
			zap.Uint64("rpcID", pkt.RPCID),
			zap.Error(err))
		return err
	}

	// Get packet type for retransmission (do this before acquiring lock)
	packetType, exists := h.transport.GetPacketRegistry().GetPacketType(pkt.PacketTypeID)
	if !exists {
		logging.Error("Packet type not found in registry",
			zap.Uint8("packetTypeID", uint8(pkt.PacketTypeID)))
		return nil // Continue even if we can't buffer for retransmission
	}

	// Acquire lock once for all operations
	h.mu.Lock()
	defer h.mu.Unlock()

	// Get or create connection (lock already held)
	conn := h.getOrCreateConnectionLocked(key, connID)

	// Track this packet in TxMsg
	isNewMessage := false
	if _, exists := conn.TxMsg[pkt.RPCID]; !exists {
		conn.TxMsg[pkt.RPCID] = &MsgTx{
			Count:      uint32(pkt.TotalPackets),
			SendTs:     time.Now(),
			DstAddr:    key,
			PacketType: packetType,
			Segments:   make(map[uint16][]byte),
		}
		isNewMessage = true
	}

	// Buffer this segment
	conn.TxMsg[pkt.RPCID].Segments[pkt.SeqNumber] = serializedData

	// Schedule timeout for this message (only for first segment)
	if isNewMessage {
		retransmitTimeout := 1 * time.Second // Default retransmit timeout
		timerKey := transport.TimerKey(fmt.Sprintf("reliable_msg_timeout_%d", pkt.RPCID))
		h.timerMgr.Schedule(
			timerKey,
			retransmitTimeout,
			transport.TimerCallback(func() {
				h.checkRetransmission(key, pkt.RPCID, retransmitTimeout)
			}),
		)
	}

	logging.Debug("Tracking sent packet",
		zap.String("packetType", packetTypeName),
		zap.Uint64("rpcID", pkt.RPCID),
		zap.Uint16("seqNumber", pkt.SeqNumber),
		zap.Uint16("totalPackets", pkt.TotalPackets),
		zap.String("connection", key))

	return nil
}

// handleSendACK handles sending ACK packets (already formed, just update activity)
// This is a common function used by both client and server handlers
func (h *ReliableHandler) handleSendACK(ack *ACKPacket, addr *net.UDPAddr, packetTypeName string, peerType string) error {
	// Extract connection ID from destination
	// Note: ACK packet doesn't have DstIP/Port, so we use addr
	var connID ConnectionID
	if ip4 := addr.IP.To4(); ip4 != nil {
		copy(connID.IP[:], ip4)
	}
	connID.Port = uint16(addr.Port)
	key := connID.String()

	// Just update activity timestamp
	h.getOrCreateConnection(key, connID)

	logging.Debug("Sending ACK",
		zap.String("packetType", packetTypeName),
		zap.Uint64("rpcID", ack.RPCID),
		zap.String("peer", peerType),
		zap.String("connection", key))

	return nil
}

// handleReceiveDataPacket processes incoming data packets (REQUEST or RESPONSE)
// This is a common function used by both client and server handlers
func (h *ReliableHandler) handleReceiveDataPacket(pkt *packet.DataPacket, addr *net.UDPAddr, ackKind uint8, packetTypeName string, peerType string) error {
	// Extract connection ID from source
	connID := ConnectionID{IP: pkt.SrcIP, Port: pkt.SrcPort}
	key := connID.String()

	// Acquire lock once for all operations
	h.mu.Lock()

	// Get or create connection (lock already held)
	conn := h.getOrCreateConnectionLocked(key, connID)

	// Check for duplicate (message already complete)
	if conn.RxMsgComplete[pkt.RPCID] {
		logging.Debug("Received duplicate packet, resending ACK",
			zap.String("packetType", packetTypeName),
			zap.Uint64("rpcID", pkt.RPCID),
			zap.String("peer", peerType),
			zap.String("connection", key))

		// Release lock before sending ACK
		h.mu.Unlock()
		err := h.sendACK(pkt.RPCID, ackKind, addr)
		if err != nil {
			logging.Error("Failed to resend ACK for duplicate", zap.Error(err))
		}
		return err
	}

	// Initialize tracking if first segment
	if _, exists := conn.RxMsgSeen[pkt.RPCID]; !exists {
		conn.RxMsgSeen[pkt.RPCID] = NewBitset(uint32(pkt.TotalPackets))
		conn.RxMsgCount[pkt.RPCID] = uint32(pkt.TotalPackets)
	}

	// Mark segment received
	conn.RxMsgSeen[pkt.RPCID].Set(uint32(pkt.SeqNumber), true)

	// Check if complete (calculate count once)
	receivedCount := conn.RxMsgSeen[pkt.RPCID].PopCount()
	isComplete := receivedCount == conn.RxMsgCount[pkt.RPCID]

	if isComplete {
		conn.RxMsgComplete[pkt.RPCID] = true
	}

	logging.Debug("Received packet segment",
		zap.String("packetType", packetTypeName),
		zap.Uint64("rpcID", pkt.RPCID),
		zap.Uint16("seqNumber", pkt.SeqNumber),
		zap.Uint16("totalPackets", pkt.TotalPackets),
		zap.Uint32("receivedCount", receivedCount),
		zap.String("peer", peerType),
		zap.String("connection", key))

	// Release lock before sending ACK (if complete)
	if isComplete {
		logging.Debug("Received complete packet, sending ACK",
			zap.String("packetType", packetTypeName),
			zap.Uint64("rpcID", pkt.RPCID),
			zap.String("peer", peerType),
			zap.String("connection", key))

		h.mu.Unlock()
		err := h.sendACK(pkt.RPCID, ackKind, addr)
		if err != nil {
			logging.Error("Failed to send ACK", zap.Error(err))
			return err
		}
	} else {
		h.mu.Unlock()
	}

	return nil
}

// handleReceiveACK processes ACK packets
// This is a common function used by both client and server handlers
func (h *ReliableHandler) handleReceiveACK(ack *ACKPacket, addr *net.UDPAddr, packetTypeName string, peerType string) error {
	// Extract connection ID from source
	var connID ConnectionID
	if ip4 := addr.IP.To4(); ip4 != nil {
		copy(connID.IP[:], ip4)
	}
	connID.Port = uint16(addr.Port)
	key := connID.String()

	h.mu.Lock()
	defer h.mu.Unlock()

	// Check if we're tracking this connection
	if conn, exists := h.connections[key]; exists {
		// Mark message as ACKed by zeroing SendTs and clear buffered segments
		if msgTx, msgExists := conn.TxMsg[ack.RPCID]; msgExists {
			msgTx.SendTs = time.Time{} // Zero time indicates ACKed
			msgTx.Segments = nil       // Clear buffered data to save memory

			// Stop the timeout timer for this message
			timerKey := transport.TimerKey(fmt.Sprintf("reliable_msg_timeout_%d", ack.RPCID))
			h.timerMgr.StopTimer(timerKey)

			logging.Debug("Received ACK",
				zap.String("packetType", packetTypeName),
				zap.Uint64("rpcID", ack.RPCID),
				zap.String("peer", peerType),
				zap.String("connection", key))
		}
	}

	return nil
}

// Cleanup stops all timers and cleans up resources
func (h *ReliableHandler) Cleanup(cleanupTimerKey transport.TimerKey) {
	h.timerMgr.StopTimer(cleanupTimerKey)
	// Note: Individual message timeout timers are automatically cleaned up when they fire
	// or when messages are ACKed. We don't need to stop them here.
}
