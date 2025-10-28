package reliable

import (
	"net"
	"time"

	"github.com/appnet-org/arpc/pkg/packet"
	"github.com/appnet-org/arpc/pkg/transport"
)

// ==================== Mock Clock ====================

// mockClock allows controlling time in tests
type mockClock struct {
	now time.Time
}

func newMockClock() *mockClock {
	return &mockClock{now: time.Now()}
}

func (c *mockClock) Now() time.Time {
	return c.now
}

func (c *mockClock) Advance(d time.Duration) {
	c.now = c.now.Add(d)
}

// ==================== Mock Timer Manager ====================

// mockTimerManager simulates the timer without actual delays
type mockTimerManager struct {
	timers map[transport.TimerKey]*mockTimer
	clock  *mockClock
}

type mockTimer struct {
	callback transport.TimerCallback
	interval time.Duration
	periodic bool
	nextFire time.Time
}

func newMockTimerManager(clock *mockClock) *mockTimerManager {
	return &mockTimerManager{
		timers: make(map[transport.TimerKey]*mockTimer),
		clock:  clock,
	}
}

func (m *mockTimerManager) SchedulePeriodic(id transport.TimerKey, interval time.Duration, callback transport.TimerCallback) {
	m.timers[id] = &mockTimer{
		callback: callback,
		interval: interval,
		periodic: true,
		nextFire: m.clock.Now().Add(interval),
	}
}

func (m *mockTimerManager) StopTimer(id transport.TimerKey) bool {
	_, exists := m.timers[id]
	delete(m.timers, id)
	return exists
}

// TriggerTimers manually fires timers that should have fired by current clock time
func (m *mockTimerManager) TriggerTimers() {
	for _, timer := range m.timers {
		if m.clock.Now().After(timer.nextFire) || m.clock.Now().Equal(timer.nextFire) {
			timer.callback()
			if timer.periodic {
				timer.nextFire = m.clock.Now().Add(timer.interval)
			}
		}
	}
}

// TriggerTimer manually fires a specific timer by name
func (m *mockTimerManager) TriggerTimer(id transport.TimerKey) {
	if timer, exists := m.timers[id]; exists {
		timer.callback()
		if timer.periodic {
			timer.nextFire = m.clock.Now().Add(timer.interval)
		}
	}
}

// ==================== Mock Transport ====================

// mockTransport captures packets sent by the handler
type mockTransport struct {
	sentPackets []sentPacket
	registry    *packet.PacketRegistry
}

type sentPacket struct {
	addr    string
	rpcID   uint64
	data    []byte
	pktType packet.PacketType
}

func newMockTransport() *mockTransport {
	registry := packet.NewPacketRegistry()
	// Register ACK packet type
	registry.RegisterPacketType(AckPacketName, &ACKPacketCodec{})

	return &mockTransport{
		sentPackets: make([]sentPacket, 0),
		registry:    registry,
	}
}

func (m *mockTransport) Send(addr string, rpcID uint64, data []byte, pktType packet.PacketType) error {
	m.sentPackets = append(m.sentPackets, sentPacket{
		addr:    addr,
		rpcID:   rpcID,
		data:    data,
		pktType: pktType,
	})
	return nil
}

func (m *mockTransport) GetPacketRegistry() *packet.PacketRegistry {
	return m.registry
}

// GetLastACK returns the last ACK packet sent
func (m *mockTransport) GetLastACK() *ACKPacket {
	for i := len(m.sentPackets) - 1; i >= 0; i-- {
		if m.sentPackets[i].pktType.Name == AckPacketName {
			ack, _ := (&ACKPacketCodec{}).Deserialize(m.sentPackets[i].data)
			return ack.(*ACKPacket)
		}
	}
	return nil
}

// GetAllACKs returns all ACK packets sent
func (m *mockTransport) GetAllACKs() []*ACKPacket {
	acks := make([]*ACKPacket, 0)
	for _, pkt := range m.sentPackets {
		if pkt.pktType.Name == AckPacketName {
			ack, _ := (&ACKPacketCodec{}).Deserialize(pkt.data)
			acks = append(acks, ack.(*ACKPacket))
		}
	}
	return acks
}

// ClearSentPackets clears the sent packets buffer
func (m *mockTransport) ClearSentPackets() {
	m.sentPackets = m.sentPackets[:0]
}

// GetSentPacketCount returns the number of packets sent
func (m *mockTransport) GetSentPacketCount() int {
	return len(m.sentPackets)
}

// ==================== Reliable Test Helper ====================

type reliableTestHelper struct {
	handler      *ReliableClientHandler
	transport    *mockTransport
	timerMgr     *mockTimerManager
	clock        *mockClock
	addr         *net.UDPAddr
	nextRPCID    uint64
	sentRequests map[uint64]*packet.DataPacket
}

func newReliableTestHelper() *reliableTestHelper {
	clock := newMockClock()
	timerMgr := newMockTimerManager(clock)
	mockTransport := newMockTransport()

	// Create handler with mock dependencies
	handler := &ReliableClientHandler{
		txReq:           make(map[uint64]*MsgTx),
		rttMin:          1000000, // 1 second in microseconds
		rxRespSeen:      make(map[uint64]*Bitset),
		rxRespCount:     make(map[uint64]uint32),
		bytesAckedTotal: 0,
		msgsLost:        0,
		transport:       mockTransport,
		timerMgr:        timerMgr,
	}

	// Start retransmit timer
	handler.startRetransmitTimer()

	return &reliableTestHelper{
		handler:      handler,
		transport:    mockTransport,
		timerMgr:     timerMgr,
		clock:        clock,
		addr:         &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080},
		nextRPCID:    1,
		sentRequests: make(map[uint64]*packet.DataPacket),
	}
}

// SendRequest simulates sending a REQUEST with N segments
func (h *reliableTestHelper) SendRequest(numSegments uint16) uint64 {
	rpcID := h.nextRPCID
	h.nextRPCID++

	for i := uint16(0); i < numSegments; i++ {
		pkt := &packet.DataPacket{
			PacketTypeID: packet.PacketTypeRequest.TypeID,
			RPCID:        rpcID,
			SeqNumber:    i,
			TotalPackets: numSegments,
			Payload:      []byte("test payload"),
		}
		h.handler.OnSend(pkt, h.addr)
		h.sentRequests[rpcID] = pkt
	}

	return rpcID
}

// ReceiveResponseSegment simulates receiving one response segment
func (h *reliableTestHelper) ReceiveResponseSegment(rpcID uint64, seqNum uint16, totalPackets uint16) error {
	resp := &packet.DataPacket{
		PacketTypeID: packet.PacketTypeResponse.TypeID,
		RPCID:        rpcID,
		SeqNumber:    seqNum,
		TotalPackets: totalPackets,
		Payload:      []byte("response data"),
	}
	return h.handler.OnReceive(resp, h.addr)
}

// ReceiveCompleteResponse simulates receiving all segments of a response
func (h *reliableTestHelper) ReceiveCompleteResponse(rpcID uint64, totalPackets uint16) error {
	for i := uint16(0); i < totalPackets; i++ {
		if err := h.ReceiveResponseSegment(rpcID, i, totalPackets); err != nil {
			return err
		}
	}
	return nil
}

// ReceiveACK simulates receiving an ACK for a request
func (h *reliableTestHelper) ReceiveACK(rpcID uint64, kind uint8) error {
	ack := &ACKPacket{
		RPCID:     rpcID,
		Kind:      kind,
		Status:    0,
		Timestamp: h.clock.Now().UnixMicro(),
	}
	return h.handler.OnReceive(ack, h.addr)
}

// AdvanceTime advances clock and triggers timers
func (h *reliableTestHelper) AdvanceTime(d time.Duration) {
	h.clock.Advance(d)
	h.timerMgr.TriggerTimers()
}

// GetRTTMin returns the current minimum RTT
func (h *reliableTestHelper) GetRTTMin() int64 {
	h.handler.mu.RLock()
	defer h.handler.mu.RUnlock()
	return h.handler.rttMin
}

// GetMsgsLost returns the number of lost messages
func (h *reliableTestHelper) GetMsgsLost() int {
	h.handler.mu.RLock()
	defer h.handler.mu.RUnlock()
	return h.handler.msgsLost
}

// GetBytesAcked returns the total bytes acknowledged
func (h *reliableTestHelper) GetBytesAcked() uint64 {
	h.handler.mu.RLock()
	defer h.handler.mu.RUnlock()
	return h.handler.bytesAckedTotal
}

// HasPendingRequest checks if a request is still pending
func (h *reliableTestHelper) HasPendingRequest(rpcID uint64) bool {
	h.handler.mu.RLock()
	defer h.handler.mu.RUnlock()
	_, exists := h.handler.txReq[rpcID]
	return exists
}

// GetPendingRequestCount returns the number of pending requests
func (h *reliableTestHelper) GetPendingRequestCount() int {
	h.handler.mu.RLock()
	defer h.handler.mu.RUnlock()
	return len(h.handler.txReq)
}

// IsResponseTracked checks if a response is being tracked
func (h *reliableTestHelper) IsResponseTracked(rpcID uint64) bool {
	h.handler.mu.RLock()
	defer h.handler.mu.RUnlock()
	_, exists := h.handler.rxRespSeen[rpcID]
	return exists
}

// GetResponseReceivedCount returns the number of segments received for a response
func (h *reliableTestHelper) GetResponseReceivedCount(rpcID uint64) uint32 {
	h.handler.mu.RLock()
	defer h.handler.mu.RUnlock()
	if bitset, exists := h.handler.rxRespSeen[rpcID]; exists {
		return bitset.PopCount()
	}
	return 0
}

// Cleanup cleans up test resources
func (h *reliableTestHelper) Cleanup() {
	h.handler.Cleanup()
}
