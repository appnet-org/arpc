package reliable

import (
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

// MsgTx represents a transmitted message state
type MsgTx struct {
	Count      uint32
	SendTs     time.Time
	TotalBytes uint64
}

// TransportSender interface for sending packets
type TransportSender interface {
	Send(addr string, rpcID uint64, data []byte, pktType packet.PacketType) error
	GetPacketRegistry() *packet.PacketRegistry
}

// TimerScheduler interface for managing timers
type TimerScheduler interface {
	SchedulePeriodic(id transport.TimerKey, interval time.Duration, callback transport.TimerCallback)
	StopTimer(id transport.TimerKey) bool
}

// ReliableClientHandler implements the client-side reliable transport logic
type ReliableClientHandler struct {
	// Client state
	txReq           map[uint64]*MsgTx
	rttMin          int64 // in microseconds
	rxRespSeen      map[uint64]*Bitset
	rxRespCount     map[uint64]uint32
	bytesAckedTotal uint64
	msgsLost        int

	// Thread safety
	mu sync.RWMutex

	// Transport reference for sending ACKs and retransmissions
	transport TransportSender
	timerMgr  TimerScheduler
}

// NewReliableClientHandler creates a new reliable client handler
func NewReliableClientHandler(transport TransportSender, timerMgr TimerScheduler) *ReliableClientHandler {
	handler := &ReliableClientHandler{
		txReq:           make(map[uint64]*MsgTx),
		rttMin:          1000000, // 1 second in microseconds
		rxRespSeen:      make(map[uint64]*Bitset),
		rxRespCount:     make(map[uint64]uint32),
		bytesAckedTotal: 0,
		msgsLost:        0,
		transport:       transport,
		timerMgr:        timerMgr,
	}

	// Start the retransmission timer
	handler.startRetransmitTimer()

	logging.Debug("Reliable client handler created", zap.Any("handler", handler))

	return handler
}

// OnSend handles outgoing REQUEST packets
func (h *ReliableClientHandler) OnSend(pkt any, addr *net.UDPAddr) error {
	dataPkt, ok := pkt.(*packet.DataPacket)
	if !ok {
		return nil // Not a data packet, ignore
	}

	// Only handle REQUEST packets (TypeID 1)
	if dataPkt.PacketTypeID != packet.PacketTypeRequest.TypeID {
		return nil
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Store request state
	h.txReq[dataPkt.RPCID] = &MsgTx{
		Count:      uint32(dataPkt.TotalPackets),
		SendTs:     time.Now(),
		TotalBytes: uint64(len(dataPkt.Payload)),
	}

	logging.Debug("Stored request state for reliable transport",
		zap.Uint64("rpcID", dataPkt.RPCID),
		zap.Uint16("totalPackets", dataPkt.TotalPackets),
		zap.Time("sendTime", time.Now()))

	return nil
}

// OnReceive handles incoming RESPONSE and ACK packets
func (h *ReliableClientHandler) OnReceive(pkt any, addr *net.UDPAddr) error {
	switch p := pkt.(type) {
	case *packet.DataPacket:
		return h.handleResponse(p, addr)
	case *ACKPacket:
		return h.handleACK(p)
	default:
		return nil // Not a packet we handle
	}
}

// handleResponse processes RESPONSE segments
func (h *ReliableClientHandler) handleResponse(resp *packet.DataPacket, addr *net.UDPAddr) error {
	// Only handle RESPONSE packets (TypeID 2)
	if resp.PacketTypeID != packet.PacketTypeResponse.TypeID {
		return nil
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	rpcID := resp.RPCID

	// Initialize response tracking if not exists
	if _, exists := h.rxRespCount[rpcID]; !exists {
		h.rxRespCount[rpcID] = uint32(resp.TotalPackets)
		h.rxRespSeen[rpcID] = NewBitset(uint32(resp.TotalPackets))
	}

	// Mark this segment as received
	h.rxRespSeen[rpcID].Set(uint32(resp.SeqNumber), true)

	logging.Debug("Received response segment",
		zap.Uint64("rpcID", rpcID),
		zap.Uint16("seqNumber", resp.SeqNumber),
		zap.Uint16("totalPackets", resp.TotalPackets),
		zap.Uint32("receivedCount", h.rxRespSeen[rpcID].PopCount()))

	// Check if entire response is complete
	if h.rxRespSeen[rpcID].PopCount() == h.rxRespCount[rpcID] {
		// Entire RESPONSE arrived → send ACK (kind = 1)
		rttSample := h.measureRTTSample()
		h.sendACK(rpcID, 1, rttSample, addr)

		// Clean up response tracking
		delete(h.rxRespSeen, rpcID)
		delete(h.rxRespCount, rpcID)

		logging.Debug("Complete response received, sent ACK",
			zap.Uint64("rpcID", rpcID),
			zap.Int64("rttSample", rttSample))
	}

	return nil
}

// handleACK processes ACK packets
func (h *ReliableClientHandler) handleACK(ack *ACKPacket) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if ack.Kind == 0 { // ACK for REQUEST
		// Update RTT minimum
		if ack.Timestamp > 0 {
			rttSample := time.Now().UnixMicro() - ack.Timestamp
			if rttSample < h.rttMin {
				h.rttMin = rttSample
			}
		}

		// Update statistics and clean up
		if txState, exists := h.txReq[ack.RPCID]; exists {
			h.bytesAckedTotal += txState.TotalBytes
			delete(h.txReq, ack.RPCID)

			logging.Debug("Request ACK received",
				zap.Uint64("rpcID", ack.RPCID),
				zap.Uint64("bytesAcked", txState.TotalBytes),
				zap.Int64("rttMin", h.rttMin))
		}
	}

	return nil
}

// sendACK sends an ACK packet
func (h *ReliableClientHandler) sendACK(rpcID uint64, kind uint8, rttSample int64, addr *net.UDPAddr) {
	ackPkt := &ACKPacket{
		RPCID:     rpcID,
		Kind:      kind,
		Status:    0, // Success
		Timestamp: time.Now().UnixMicro(),
		Message:   "",
	}

	// Send ACK packet
	logging.Debug("Sending ACK packet",
		zap.Uint64("rpcID", rpcID),
		zap.Uint8("kind", kind),
		zap.Int64("rttSample", rttSample))

	// Get ACK packet type from transport's registry
	ackPacketType, exists := h.transport.GetPacketRegistry().GetPacketTypeByName(AckPacketName)
	if !exists {
		logging.Error("ACK packet type not registered in transport")
		return
	}

	ackData, err := (&ACKPacketCodec{}).Serialize(ackPkt)
	if err != nil {
		logging.Error("Failed to serialize ACK packet", zap.Error(err))
		return
	}

	h.transport.Send(addr.String(), rpcID, ackData, ackPacketType)
}

// measureRTTSample measures RTT sample (simplified implementation)
func (h *ReliableClientHandler) measureRTTSample() int64 {
	// In a real implementation, this would measure the actual RTT
	// For now, return a placeholder value
	return h.rttMin
}

// rto calculates the retransmission timeout
func (h *ReliableClientHandler) rto() time.Duration {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Simple RTO calculation: 4 * RTT_min
	rtoMicros := h.rttMin * 4
	if rtoMicros < 100000 { // Minimum 100ms
		rtoMicros = 100000
	}

	return time.Duration(rtoMicros) * time.Microsecond
}

// retransmitMessage retransmits a message
func (h *ReliableClientHandler) retransmitMessage(kind uint8, rpcID uint64) {
	logging.Debug("Retransmitting message",
		zap.Uint8("kind", kind),
		zap.Uint64("rpcID", rpcID))

	// In a real implementation, this would trigger the transport layer
	// to retransmit the message segments
	// For now, we just log the retransmission
}

// startRetransmitTimer starts the periodic retransmission timer
func (h *ReliableClientHandler) startRetransmitTimer() {
	h.timerMgr.SchedulePeriodic(
		transport.TimerKey("retransmit_req"),
		1*time.Millisecond,
		transport.TimerCallback(func() {
			h.checkRetransmissions()
		}),
	)
}

// checkRetransmissions checks for messages that need retransmission
func (h *ReliableClientHandler) checkRetransmissions() {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	rto := h.rto()

	for rpcID, txState := range h.txReq {
		elapsed := now.Sub(txState.SendTs)
		if elapsed > rto {
			// Message has timed out, retransmit
			h.retransmitMessage(0, rpcID) // 0 = REQUEST
			h.msgsLost++
			txState.SendTs = now // Update send time

			logging.Debug("Message retransmitted due to timeout",
				zap.Uint64("rpcID", rpcID),
				zap.Duration("rto", rto),
				zap.Duration("elapsed", elapsed),
				zap.Int("msgsLost", h.msgsLost))
		}
	}
}

// GetStats returns current statistics
func (h *ReliableClientHandler) GetStats() (bytesAckedTotal uint64, msgsLost int, rttMin int64) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.bytesAckedTotal, h.msgsLost, h.rttMin
}

// Cleanup cleans up resources
func (h *ReliableClientHandler) Cleanup() {
	h.timerMgr.StopTimer(transport.TimerKey("retransmit_req"))
}
