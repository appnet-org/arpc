package reliable

import (
	"testing"
	"time"

	"github.com/appnet-org/arpc/pkg/packet"
	"github.com/stretchr/testify/require"
)

// ==================== Basic Flow Tests ====================
//
// These tests verify the fundamental request/response tracking and message-level
// ACK behavior. Key principle: ACKs are sent only after receiving ALL segments
// of a message, not individual packets.

// TestReliableClientHandler_BasicRequestResponse verifies basic request tracking
// and cleanup after receiving an ACK.
func TestReliableClientHandler_BasicRequestResponse(t *testing.T) {
	helper := newReliableTestHelper()
	defer helper.Cleanup()

	// Send a request with 3 segments
	rpcID := helper.SendRequest(3)

	// Verify request state is tracked
	require.True(t, helper.HasPendingRequest(rpcID), "Request state should be tracked")

	helper.handler.mu.RLock()
	txState := helper.handler.txReq[rpcID]
	helper.handler.mu.RUnlock()

	require.Equal(t, uint32(3), txState.Count)

	// Receive ACK for request
	err := helper.ReceiveACK(rpcID, 0) // kind=0 for request
	require.NoError(t, err)

	// Verify request state is cleaned up
	require.False(t, helper.HasPendingRequest(rpcID), "Request state should be cleaned up after ACK")
	require.Greater(t, helper.GetBytesAcked(), uint64(0), "Bytes should be counted")
}

// TestReliableClientHandler_ResponseTrackingInOrder verifies that when all
// response segments arrive in order, the handler tracks them and sends an ACK
// only after receiving the complete message.
func TestReliableClientHandler_ResponseTrackingInOrder(t *testing.T) {
	helper := newReliableTestHelper()
	defer helper.Cleanup()

	rpcID := uint64(100)
	totalPackets := uint16(5)

	// Receive all segments in order
	for i := uint16(0); i < totalPackets; i++ {
		require.NoError(t, helper.ReceiveResponseSegment(rpcID, i, totalPackets))
	}

	// ACK should be sent after receiving all segments
	ack := helper.transport.GetLastACK()
	require.NotNil(t, ack, "ACK should be sent after complete response")
	require.Equal(t, rpcID, ack.RPCID)
	require.Equal(t, uint8(1), ack.Kind) // kind=1 for response

	// Verify response tracking is cleaned up
	require.False(t, helper.IsResponseTracked(rpcID), "Response tracking should be cleaned up")
}

// TestReliableClientHandler_ResponseTrackingOutOfOrder verifies that the handler
// correctly tracks out-of-order segments using a bitset and only sends an ACK
// when all segments have been received, regardless of arrival order.
func TestReliableClientHandler_ResponseTrackingOutOfOrder(t *testing.T) {
	helper := newReliableTestHelper()
	defer helper.Cleanup()

	rpcID := uint64(100)
	totalPackets := uint16(5)

	// Receive segments out of order: 0, 2, 4, 1, 3
	require.NoError(t, helper.ReceiveResponseSegment(rpcID, 0, totalPackets))
	require.NoError(t, helper.ReceiveResponseSegment(rpcID, 2, totalPackets))
	require.NoError(t, helper.ReceiveResponseSegment(rpcID, 4, totalPackets))

	// No ACK should be sent yet (missing segments 1 and 3)
	helper.transport.ClearSentPackets()
	require.NoError(t, helper.ReceiveResponseSegment(rpcID, 1, totalPackets))
	require.Nil(t, helper.transport.GetLastACK(), "No ACK yet, still missing segment 3")

	// Receive last segment
	require.NoError(t, helper.ReceiveResponseSegment(rpcID, 3, totalPackets))

	// Now ACK should be sent
	ack := helper.transport.GetLastACK()
	require.NotNil(t, ack, "ACK should be sent after complete response")
	require.Equal(t, rpcID, ack.RPCID)
	require.Equal(t, uint8(1), ack.Kind) // kind=1 for response

	// Verify response tracking is cleaned up
	require.False(t, helper.IsResponseTracked(rpcID), "Response tracking should be cleaned up")
}

// TestReliableClientHandler_DuplicateResponseSegments verifies that duplicate
// segments are handled gracefully (bitset naturally handles this by setting the
// same bit multiple times).
func TestReliableClientHandler_DuplicateResponseSegments(t *testing.T) {
	helper := newReliableTestHelper()
	defer helper.Cleanup()

	rpcID := uint64(200)
	totalPackets := uint16(3)

	// Receive segment 0 twice
	require.NoError(t, helper.ReceiveResponseSegment(rpcID, 0, totalPackets))
	require.NoError(t, helper.ReceiveResponseSegment(rpcID, 0, totalPackets))

	// Count should still be 1
	require.Equal(t, uint32(1), helper.GetResponseReceivedCount(rpcID))

	// Receive remaining segments
	require.NoError(t, helper.ReceiveResponseSegment(rpcID, 1, totalPackets))
	require.NoError(t, helper.ReceiveResponseSegment(rpcID, 2, totalPackets))

	// ACK should be sent
	ack := helper.transport.GetLastACK()
	require.NotNil(t, ack)
	require.Equal(t, rpcID, ack.RPCID)
}

// ==================== Retransmission Tests ====================
//
// These tests verify ACK-based cleanup behavior. In the current implementation,
// retransmission logic is triggered by periodic timer checks comparing elapsed
// time against RTO (Retransmission Timeout = 4×RTT_min, minimum 100ms).
//
// Note: Actual retransmission timer tests are omitted because they require
// integration with real timers, which would slow down the test suite. The focus
// here is on the cleanup behavior when ACKs arrive.

// TestReliableClientHandler_RetransmissionStopsAfterACK verifies that when an
// ACK is received, the request state is cleaned up and no retransmission occurs.
func TestReliableClientHandler_RetransmissionStopsAfterACK(t *testing.T) {
	helper := newReliableTestHelper()
	defer helper.Cleanup()

	// Send a request
	rpcID := helper.SendRequest(1)

	// Receive ACK immediately
	err := helper.ReceiveACK(rpcID, 0)
	require.NoError(t, err)

	// Verify request was cleared
	require.False(t, helper.HasPendingRequest(rpcID), "Request should be cleared after ACK")
	require.Equal(t, 0, helper.GetMsgsLost(), "No losses should be recorded")
}

// ==================== RTT and RTO Tests ====================
//
// RTT (Round Trip Time) is measured from ACK timestamps. The handler tracks
// the minimum RTT seen and uses it to calculate RTO (Retransmission Timeout).
//
// RTO Calculation: RTO = 4 × RTT_min (with a minimum of 100ms)
//
// This is a simpler approach compared to QUIC's full RTT tracking with smoothed
// RTT and mean deviation, suitable for the current reliability requirements.

// TestReliableClientHandler_RTTUpdate verifies that RTT measurements are updated
// when ACKs arrive with timestamp information.
func TestReliableClientHandler_RTTUpdate(t *testing.T) {
	helper := newReliableTestHelper()
	defer helper.Cleanup()

	// Send request
	rpcID := helper.SendRequest(1)

	// Get initial RTT (1 second)
	initialRTT := helper.GetRTTMin()
	require.Equal(t, int64(1000000), initialRTT) // 1 second in microseconds

	// Simulate 50ms RTT by creating ACK with earlier timestamp
	timestampDiff := int64(50000) // 50ms in microseconds
	ack := &ACKPacket{
		RPCID:     rpcID,
		Kind:      0,
		Status:    0,
		Timestamp: helper.clock.Now().UnixMicro() - timestampDiff,
	}
	err := helper.handler.OnReceive(ack, helper.addr)
	require.NoError(t, err)

	// Verify RTT was updated
	newRTT := helper.GetRTTMin()
	require.Less(t, newRTT, initialRTT, "RTT should decrease")
	require.Less(t, newRTT, int64(100000), "RTT should be less than 100ms")
}

// TestReliableClientHandler_RTOCalculation verifies the RTO calculation logic
// and that RTT_min only updates when a lower RTT is observed.
func TestReliableClientHandler_RTOCalculation(t *testing.T) {
	helper := newReliableTestHelper()
	defer helper.Cleanup()

	// Initial RTO should be 4 * RTT_min, but clamped to minimum 100ms
	rto := helper.handler.rto()
	require.GreaterOrEqual(t, rto, 100*time.Millisecond)

	// Update RTT to 25ms
	rpcID := helper.SendRequest(1)
	ack := &ACKPacket{
		RPCID:     rpcID,
		Kind:      0,
		Status:    0,
		Timestamp: helper.clock.Now().UnixMicro() - 25000, // 25ms ago
	}
	helper.handler.OnReceive(ack, helper.addr)

	// RTO should now be 4 * 25ms = 100ms (with some tolerance)
	newRTO := helper.handler.rto()
	require.GreaterOrEqual(t, newRTO, 100*time.Millisecond)
	require.LessOrEqual(t, newRTO, 110*time.Millisecond)

	// Update RTT to 50ms
	rpcID2 := helper.SendRequest(1)
	ack2 := &ACKPacket{
		RPCID:     rpcID2,
		Kind:      0,
		Status:    0,
		Timestamp: helper.clock.Now().UnixMicro() - 50000, // 50ms ago
	}
	helper.handler.OnReceive(ack2, helper.addr)

	// RTO should still be ~100ms (4 * 25ms, not updated because 50ms > 25ms)
	newRTO2 := helper.handler.rto()
	require.GreaterOrEqual(t, newRTO2, 100*time.Millisecond)
	require.LessOrEqual(t, newRTO2, 110*time.Millisecond)
}

// ==================== Multiple Request Tests ====================
//
// These tests verify that the handler can correctly manage multiple concurrent
// requests, tracking each independently and handling ACKs in any order.

// TestReliableClientHandler_MultipleInFlightRequests verifies that multiple
// concurrent requests are tracked independently and ACKs can arrive in any order.
func TestReliableClientHandler_MultipleInFlightRequests(t *testing.T) {
	helper := newReliableTestHelper()
	defer helper.Cleanup()

	// Send multiple requests
	rpcID1 := helper.SendRequest(1)
	rpcID2 := helper.SendRequest(2)
	rpcID3 := helper.SendRequest(3)

	// Verify all are tracked
	require.Equal(t, 3, helper.GetPendingRequestCount())

	// ACK middle request
	err := helper.ReceiveACK(rpcID2, 0)
	require.NoError(t, err)

	// Verify only one is removed
	require.Equal(t, 2, helper.GetPendingRequestCount())
	require.True(t, helper.HasPendingRequest(rpcID1))
	require.False(t, helper.HasPendingRequest(rpcID2))
	require.True(t, helper.HasPendingRequest(rpcID3))

	// ACK remaining requests
	helper.ReceiveACK(rpcID1, 0)
	helper.ReceiveACK(rpcID3, 0)

	// Verify all are cleared
	require.Equal(t, 0, helper.GetPendingRequestCount())
}

// TestReliableClientHandler_MultipleRequestsSequential verifies sequential
// request handling with ACKs arriving in a different order than requests were sent.
func TestReliableClientHandler_MultipleRequestsSequential(t *testing.T) {
	helper := newReliableTestHelper()
	defer helper.Cleanup()

	// Send 3 requests
	rpcID1 := helper.SendRequest(1)
	rpcID2 := helper.SendRequest(1)
	rpcID3 := helper.SendRequest(1)

	// All should be pending
	require.Equal(t, 3, helper.GetPendingRequestCount())

	// ACK them in different order
	helper.ReceiveACK(rpcID2, 0)
	require.Equal(t, 2, helper.GetPendingRequestCount())

	helper.ReceiveACK(rpcID1, 0)
	require.Equal(t, 1, helper.GetPendingRequestCount())

	helper.ReceiveACK(rpcID3, 0)
	require.Equal(t, 0, helper.GetPendingRequestCount())
}

// ==================== Statistics Tests ====================
//
// These tests verify the statistics tracking functionality. The handler maintains:
// - bytesAckedTotal: Total bytes acknowledged across all completed RPCs
// - msgsLost: Counter for retransmission events
// - rttMin: Minimum RTT observed

// TestReliableClientHandler_Statistics verifies basic statistics tracking
// for successful request/response cycles.
func TestReliableClientHandler_Statistics(t *testing.T) {
	helper := newReliableTestHelper()
	defer helper.Cleanup()

	// Send and ack multiple requests
	totalBytes := uint64(0)
	for i := 0; i < 5; i++ {
		rpcID := helper.SendRequest(2)
		totalBytes += uint64(len("test payload"))
		err := helper.ReceiveACK(rpcID, 0)
		require.NoError(t, err)
	}

	// Check statistics
	bytesAcked, msgsLost, rttMin := helper.handler.GetStats()
	require.Greater(t, bytesAcked, uint64(0))
	require.Equal(t, 0, msgsLost)
	require.Greater(t, rttMin, int64(0))
}

// TestReliableClientHandler_StatsBasic verifies statistics when all requests
// complete successfully without losses.
func TestReliableClientHandler_StatsBasic(t *testing.T) {
	helper := newReliableTestHelper()
	defer helper.Cleanup()

	// Send 3 requests
	rpcID1 := helper.SendRequest(1)
	rpcID2 := helper.SendRequest(1)
	rpcID3 := helper.SendRequest(1)

	// ACK all requests
	helper.ReceiveACK(rpcID1, 0)
	helper.ReceiveACK(rpcID2, 0)
	helper.ReceiveACK(rpcID3, 0)

	// Check final stats
	bytesAcked, msgsLost, _ := helper.handler.GetStats()
	require.Greater(t, bytesAcked, uint64(0))
	require.Equal(t, 0, msgsLost) // No losses
}

// ==================== Edge Cases ====================
//
// These tests cover unusual but valid scenarios to ensure robustness.

// TestReliableClientHandler_LargeMessage verifies handling of messages with
// many segments (100+), ensuring the bitset and tracking scales properly.
func TestReliableClientHandler_LargeMessage(t *testing.T) {
	helper := newReliableTestHelper()
	defer helper.Cleanup()

	rpcID := uint64(500)
	totalPackets := uint16(100) // Large message with 100 segments

	// Receive all segments
	for i := uint16(0); i < totalPackets; i++ {
		require.NoError(t, helper.ReceiveResponseSegment(rpcID, i, totalPackets))
	}

	// Verify ACK is sent
	ack := helper.transport.GetLastACK()
	require.NotNil(t, ack)
	require.Equal(t, rpcID, ack.RPCID)
	require.False(t, helper.IsResponseTracked(rpcID))
}

// TestReliableClientHandler_ZeroLengthMessage verifies handling of messages
// with empty payloads (valid in RPC protocols for signaling).
func TestReliableClientHandler_ZeroLengthMessage(t *testing.T) {
	helper := newReliableTestHelper()
	defer helper.Cleanup()

	rpcID := uint64(600)
	totalPackets := uint16(1)

	// Single segment with empty payload
	resp := &packet.DataPacket{
		PacketTypeID: packet.PacketTypeResponse.TypeID,
		RPCID:        rpcID,
		SeqNumber:    0,
		TotalPackets: totalPackets,
		Payload:      []byte{},
	}
	err := helper.handler.OnReceive(resp, helper.addr)
	require.NoError(t, err)

	// ACK should still be sent
	ack := helper.transport.GetLastACK()
	require.NotNil(t, ack)
	require.Equal(t, rpcID, ack.RPCID)
}

// TestReliableClientHandler_ConcurrentResponsesAndRequests verifies that the
// handler can simultaneously track outgoing requests and incoming responses
// without interference.
func TestReliableClientHandler_ConcurrentResponsesAndRequests(t *testing.T) {
	helper := newReliableTestHelper()
	defer helper.Cleanup()

	// Send multiple requests
	reqID1 := helper.SendRequest(2)
	reqID2 := helper.SendRequest(3)

	// Start receiving responses
	respID1 := uint64(1000)
	respID2 := uint64(2000)

	// Interleave request ACKs and response segments
	helper.ReceiveACK(reqID1, 0)
	helper.ReceiveResponseSegment(respID1, 0, 5)
	helper.ReceiveResponseSegment(respID2, 0, 3)
	helper.ReceiveACK(reqID2, 0)

	// Complete responses
	for i := uint16(1); i < 5; i++ {
		helper.ReceiveResponseSegment(respID1, i, 5)
	}
	for i := uint16(1); i < 3; i++ {
		helper.ReceiveResponseSegment(respID2, i, 3)
	}

	// Verify both responses sent ACKs
	acks := helper.transport.GetAllACKs()
	responseAcks := 0
	for _, ack := range acks {
		if ack.Kind == 1 { // Response ACK
			responseAcks++
		}
	}
	require.Equal(t, 2, responseAcks)

	// Verify all requests cleared
	require.Equal(t, 0, helper.GetPendingRequestCount())
}

// TestReliableClientHandler_IgnoreNonDataPackets verifies that non-data packets
// are gracefully ignored without errors (defensive programming).
func TestReliableClientHandler_IgnoreNonDataPackets(t *testing.T) {
	helper := newReliableTestHelper()
	defer helper.Cleanup()

	// Try sending a non-DataPacket (should be ignored)
	err := helper.handler.OnSend("not a packet", helper.addr)
	require.NoError(t, err)

	// Try receiving a non-DataPacket/ACKPacket (should be ignored)
	err = helper.handler.OnReceive("not a packet", helper.addr)
	require.NoError(t, err)
}
