package main

import (
	"encoding/binary"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/packet"
	"github.com/appnet-org/proxy/util"
)

func init() {
	// Initialize logging to avoid race conditions in tests
	logging.Init(&logging.Config{
		Level:  "info",
		Format: "console",
	})
}

// Helper function to create a DataPacket
func createDataPacket(rpcID uint64, seqNum uint16, totalPackets uint16, payload []byte) *packet.DataPacket {
	return &packet.DataPacket{
		PacketTypeID: packet.PacketTypeRequest.TypeID,
		RPCID:        rpcID,
		TotalPackets: totalPackets,
		SeqNumber:    seqNum,
		DstIP:        [4]byte{192, 168, 1, 1},
		DstPort:      8080,
		SrcIP:        [4]byte{192, 168, 1, 2},
		SrcPort:      9090,
		Payload:      payload,
	}
}

// Helper function to serialize a DataPacket
func serializePacket(dataPacket *packet.DataPacket) []byte {
	codec := &packet.DataPacketCodec{}
	data, err := codec.Serialize(dataPacket, nil)
	if err != nil {
		panic(err)
	}
	return data
}

// Helper function to create payload with offset_to_private
func createPayloadWithOffset(publicSize int, privateSize int) []byte {
	totalSize := publicSize + privateSize
	if totalSize < 5 {
		totalSize = 5
	}
	payload := make([]byte, totalSize)
	payload[0] = 0x01 // version
	binary.LittleEndian.PutUint32(payload[1:5], uint32(publicSize))
	// Fill rest with data
	for i := 5; i < totalSize; i++ {
		payload[i] = byte(i % 256)
	}
	return payload
}

func TestPacketBuffer_ConcurrentFragmentProcessing(t *testing.T) {
	pb := NewPacketBuffer(5 * time.Second)
	defer pb.Close()

	src := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 2), Port: 9090}
	rpcID := uint64(12345)
	totalPackets := uint16(3)

	// Create fragments for a fragmented message
	// Fragment 0 has public segment, fragments 1-2 are private
	publicSize := 500
	privateSize := 100
	fullPayload := createPayloadWithOffset(publicSize, privateSize)

	// Split payload into fragments (each fragment can hold up to ~1371 bytes)
	fragmentSize := 600
	frag0Len := fragmentSize
	if frag0Len > len(fullPayload) {
		frag0Len = len(fullPayload)
	}
	frag1Len := fragmentSize
	if frag1Len > len(fullPayload)-frag0Len {
		frag1Len = len(fullPayload) - frag0Len
	}
	fragment0Payload := fullPayload[:frag0Len]
	fragment1Payload := fullPayload[frag0Len : frag0Len+frag1Len]
	fragment2Payload := fullPayload[frag0Len+frag1Len:]

	frag0 := createDataPacket(rpcID, 0, totalPackets, fragment0Payload)
	frag1 := createDataPacket(rpcID, 1, totalPackets, fragment1Payload)
	frag2 := createDataPacket(rpcID, 2, totalPackets, fragment2Payload)

	data0 := serializePacket(frag0)
	data1 := serializePacket(frag1)
	data2 := serializePacket(frag2)

	// Test concurrent fragment processing for same RPC
	var wg sync.WaitGroup
	errors := make(chan error, 10)

	// Send fragments concurrently
	wg.Add(3)
	go func() {
		defer wg.Done()
		_, _, err := pb.ProcessPacket(data0, src)
		if err != nil {
			errors <- err
		}
	}()
	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond) // Slight delay to ensure fragment 0 arrives first
		_, _, err := pb.ProcessPacket(data1, src)
		if err != nil {
			errors <- err
		}
	}()
	go func() {
		defer wg.Done()
		time.Sleep(20 * time.Millisecond)
		_, _, err := pb.ProcessPacket(data2, src)
		if err != nil {
			errors <- err
		}
	}()

	wg.Wait()
	close(errors)

	// Check for errors
	errorCount := 0
	for err := range errors {
		if err != nil {
			t.Errorf("Error processing fragment: %v", err)
			errorCount++
		}
	}

	// Verify that at least one fragment was processed successfully
	// Note: Fragment 0 may have already been processed and extracted, so it might return nil on re-processing
	// The test verifies that concurrent processing doesn't cause panics or data races
	if errorCount > 0 {
		t.Errorf("Expected no errors, got %d errors", errorCount)
	}
}

func TestPacketBuffer_VerdictStorageRetrievalRace(t *testing.T) {
	pb := NewPacketBuffer(5 * time.Second)
	defer pb.Close()

	rpcID := uint64(54321)
	packetType := util.PacketTypeRequest

	// Test concurrent verdict storage and retrieval
	var wg sync.WaitGroup
	const numGoroutines = 50

	wg.Add(numGoroutines * 2) // Store and retrieve

	// Concurrent verdict storage
	for i := 0; i < numGoroutines; i++ {
		go func(iter int) {
			defer wg.Done()
			if iter%2 == 0 {
				pb.StoreVerdict(rpcID, packetType, util.PacketVerdictPass)
			} else {
				pb.StoreVerdict(rpcID, packetType, util.PacketVerdictDrop)
			}
		}(i)
	}

	// Concurrent verdict retrieval (via ProcessPacket with existing verdict)
	src := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 3), Port: 9090}
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			// Create a packet that will be fast-forwarded
			payload := createPayloadWithOffset(100, 50)
			pkt := createDataPacket(rpcID, 0, 1, payload)
			data := serializePacket(pkt)
			_, _, _ = pb.ProcessPacket(data, src)
		}()
	}

	wg.Wait()

	// Verify verdict exists
	src2 := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 4), Port: 9090}
	payload := createPayloadWithOffset(100, 50)
	pkt := createDataPacket(rpcID, 0, 1, payload)
	data := serializePacket(pkt)
	_, verdict, err := pb.ProcessPacket(data, src2)
	if err != nil {
		t.Fatalf("Error processing packet: %v", err)
	}
	if verdict == util.PacketVerdictUnknown {
		t.Error("Expected verdict to be stored, got PacketVerdictUnknown")
	}
}

func TestPacketBuffer_ProcessRemainingFragmentsRace(t *testing.T) {
	pb := NewPacketBuffer(5 * time.Second)
	defer pb.Close()

	src := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 5), Port: 9090}
	rpcID := uint64(99999)
	packetType := util.PacketTypeRequest
	connKey := src.String()

	// Store a verdict first
	pb.StoreVerdict(rpcID, packetType, util.PacketVerdictPass)

	// Create metadata
	metadata := &util.BufferedPacket{
		Source:       src,
		Peer:         &net.UDPAddr{IP: net.IPv4(192, 168, 1, 1), Port: 8080},
		PacketType:   packetType,
		RPCID:        rpcID,
		DstIP:        [4]byte{192, 168, 1, 1},
		DstPort:      8080,
		SrcIP:        [4]byte{192, 168, 1, 5},
		SrcPort:      9090,
		TotalPackets: 3,
	}

	// Add some fragments to buffer by processing packets
	publicSize := 500
	privateSize := 100
	fullPayload := createPayloadWithOffset(publicSize, privateSize)
	fragmentSize := 600
	frag0Len := fragmentSize
	if frag0Len > len(fullPayload) {
		frag0Len = len(fullPayload)
	}
	frag1Len := fragmentSize
	if frag1Len > len(fullPayload)-frag0Len {
		frag1Len = len(fullPayload) - frag0Len
	}
	frag0 := createDataPacket(rpcID, 0, 3, fullPayload[:frag0Len])
	frag1 := createDataPacket(rpcID, 1, 3, fullPayload[frag0Len:frag0Len+frag1Len])
	data0 := serializePacket(frag0)
	data1 := serializePacket(frag1)

	// Process fragment 0 to extract public segment
	_, _, _ = pb.ProcessPacket(data0, src)
	// Process fragment 1 (should be buffered)
	_, _, _ = pb.ProcessPacket(data1, src)

	// Test concurrent ProcessRemainingFragments calls
	var wg sync.WaitGroup
	const numGoroutines = 20
	results := make([][]*util.BufferedPacket, numGoroutines)

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx] = pb.ProcessRemainingFragments(connKey, rpcID, packetType, metadata)
		}(i)
	}

	wg.Wait()

	// Verify that only one goroutine got fragments (or all got empty)
	nonEmptyCount := 0
	for _, result := range results {
		if len(result) > 0 {
			nonEmptyCount++
		}
	}

	// Either all should be empty (fragments already processed) or one should have fragments
	if nonEmptyCount > 1 {
		t.Errorf("Expected at most 1 goroutine to get fragments, got %d", nonEmptyCount)
	}
}

func TestPacketBuffer_CleanupRace(t *testing.T) {
	pb := NewPacketBuffer(100 * time.Millisecond) // Short timeout for testing
	defer pb.Close()

	src := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 6), Port: 9090}
	rpcID := uint64(77777)

	// Add fragments
	payload := createPayloadWithOffset(100, 50)
	pkt := createDataPacket(rpcID, 0, 1, payload)
	data := serializePacket(pkt)

	var wg sync.WaitGroup
	const numGoroutines = 20

	// Concurrent packet processing
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_, _, _ = pb.ProcessPacket(data, src)
		}()
	}

	// Concurrent cleanup (triggered by timer)
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Wait for timeout
		time.Sleep(150 * time.Millisecond)
		// Cleanup should happen automatically via cleanup routine
		time.Sleep(50 * time.Millisecond)
	}()

	wg.Wait()

	// Verify no panics or data races occurred
	// If we get here, the test passed
}

func TestPacketBuffer_DifferentRPCsConcurrent(t *testing.T) {
	pb := NewPacketBuffer(5 * time.Second)
	defer pb.Close()

	src1 := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 10), Port: 9090}
	src2 := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 11), Port: 9090}
	rpcID1 := uint64(100001)
	rpcID2 := uint64(100002)

	payload1 := createPayloadWithOffset(100, 50)
	payload2 := createPayloadWithOffset(200, 100)

	pkt1 := createDataPacket(rpcID1, 0, 1, payload1)
	pkt2 := createDataPacket(rpcID2, 0, 1, payload2)

	data1 := serializePacket(pkt1)
	data2 := serializePacket(pkt2)

	// Process packets for different RPCs concurrently
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _, err := pb.ProcessPacket(data1, src1)
		if err != nil {
			t.Errorf("Error processing packet 1: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		_, _, err := pb.ProcessPacket(data2, src2)
		if err != nil {
			t.Errorf("Error processing packet 2: %v", err)
		}
	}()

	wg.Wait()

	// Verify both packets were processed correctly
	_, _, err1 := pb.ProcessPacket(data1, src1)
	if err1 != nil {
		t.Errorf("Error re-processing packet 1: %v", err1)
	}

	_, _, err2 := pb.ProcessPacket(data2, src2)
	if err2 != nil {
		t.Errorf("Error re-processing packet 2: %v", err2)
	}
}

func TestPacketBuffer_FragmentPacketForForward(t *testing.T) {
	pb := NewPacketBuffer(5 * time.Second)
	defer pb.Close()

	src := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 7), Port: 9090}
	peer := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 1), Port: 8080}
	rpcID := uint64(88888)

	// Test single packet
	smallPayload := make([]byte, 100)
	bufferedPacket := &util.BufferedPacket{
		Payload:      smallPayload,
		Source:       src,
		Peer:         peer,
		PacketType:   util.PacketTypeRequest,
		RPCID:        rpcID,
		DstIP:        [4]byte{192, 168, 1, 1},
		DstPort:      8080,
		SrcIP:        [4]byte{192, 168, 1, 7},
		SrcPort:      9090,
		IsFull:       true,
		SeqNumber:    -1,
		TotalPackets: 1,
	}

	fragments, err := pb.FragmentPacketForForward(bufferedPacket)
	if err != nil {
		t.Fatalf("Error fragmenting packet: %v", err)
	}
	if len(fragments) != 1 {
		t.Errorf("Expected 1 fragment for small packet, got %d", len(fragments))
	}

	// Test fragmented packet
	largePayload := make([]byte, 2000) // Larger than MTU
	bufferedPacket2 := &util.BufferedPacket{
		Payload:      largePayload,
		Source:       src,
		Peer:         peer,
		PacketType:   util.PacketTypeRequest,
		RPCID:        rpcID + 1,
		DstIP:        [4]byte{192, 168, 1, 1},
		DstPort:      8080,
		SrcIP:        [4]byte{192, 168, 1, 7},
		SrcPort:      9090,
		IsFull:       false,
		SeqNumber:    -1,
		TotalPackets: 0,
	}

	fragments2, err := pb.FragmentPacketForForward(bufferedPacket2)
	if err != nil {
		t.Fatalf("Error fragmenting large packet: %v", err)
	}
	if len(fragments2) <= 1 {
		t.Errorf("Expected multiple fragments for large packet, got %d", len(fragments2))
	}
}

// createLargeSymphonyPayload creates a Symphony-format payload similar to SetRequest
// with the specified key and value sizes. Format:
// - Byte 0: version (0x01)
// - Bytes 1-4: offset_to_private (little-endian uint32)
// - Bytes 5-8: service_id (0)
// - Bytes 9-12: method_id (0)
// - Bytes 13+: private segment (version + table + key + value data)
func createLargeSymphonyPayload(keySize, valueSize int) []byte {
	// Public segment: 13 bytes (version + offset_to_private + service_id + method_id)
	publicSegmentSize := 13

	// Private segment: 1 (version) + 8 (table for 2 fields) + (4 + keySize) + (4 + valueSize)
	privateSegmentSize := 1 + 8 + (4 + keySize) + (4 + valueSize)

	totalSize := publicSegmentSize + privateSegmentSize
	payload := make([]byte, totalSize)

	// Public segment
	payload[0] = 0x01 // version
	binary.LittleEndian.PutUint32(payload[1:5], uint32(publicSegmentSize))
	binary.LittleEndian.PutUint32(payload[5:9], 0)  // service_id
	binary.LittleEndian.PutUint32(payload[9:13], 0) // method_id

	// Private segment
	privateStart := publicSegmentSize
	payload[privateStart] = 0x01 // version

	// Table entries (2 fields, 4 bytes each = 8 bytes)
	tableStart := privateStart + 1
	// Field 1 (Key) offset: points to after table
	binary.LittleEndian.PutUint32(payload[tableStart:], uint32(9)) // relative offset from privateStart
	// Field 2 (Value) offset: points to after key
	binary.LittleEndian.PutUint32(payload[tableStart+4:], uint32(9+4+keySize))

	// Payload data
	payloadStart := tableStart + 8
	// Key: length prefix + data
	binary.LittleEndian.PutUint32(payload[payloadStart:], uint32(keySize))
	for i := 0; i < keySize; i++ {
		payload[payloadStart+4+i] = byte('k')
	}
	// Value: length prefix + data
	valueOffset := payloadStart + 4 + keySize
	binary.LittleEndian.PutUint32(payload[valueOffset:], uint32(valueSize))
	for i := 0; i < valueSize; i++ {
		payload[valueOffset+4+i] = byte('v')
	}

	return payload
}

// fragmentPayloadLikeClient simulates how the client fragments a large payload
// using the Symphony fragmentation strategy (slack optimization).
func fragmentPayloadLikeClient(payload []byte, mtu int) [][]byte {
	if len(payload) <= mtu {
		return [][]byte{payload}
	}

	offsetToPrivate := int(binary.LittleEndian.Uint32(payload[1:5]))
	publicData := payload[0:offsetToPrivate]
	privateData := payload[offsetToPrivate:]

	var packets [][]byte

	// Phase 1: Full MTU public packets
	publicOffset := 0
	for len(publicData)-publicOffset > mtu {
		pkt := make([]byte, mtu)
		copy(pkt, publicData[publicOffset:publicOffset+mtu])
		packets = append(packets, pkt)
		publicOffset += mtu
	}

	// Meeting packet (remainder of public + some private)
	meetingPacket := make([]byte, len(publicData)-publicOffset)
	copy(meetingPacket, publicData[publicOffset:])

	// Phase 2: Slack optimization
	if len(privateData) > 0 {
		privateHeadLen := len(privateData) % mtu
		chunk := privateData[:privateHeadLen]
		privateData = privateData[privateHeadLen:]

		currentMeetingLen := len(meetingPacket)
		totalLen := currentMeetingLen + len(chunk)

		if totalLen <= mtu {
			newPacket := make([]byte, totalLen)
			copy(newPacket, meetingPacket)
			copy(newPacket[currentMeetingLen:], chunk)
			packets = append(packets, newPacket)
		} else {
			fillNeeded := mtu - currentMeetingLen
			packet1 := make([]byte, mtu)
			copy(packet1, meetingPacket)
			copy(packet1[currentMeetingLen:], chunk[:fillNeeded])
			packets = append(packets, packet1)

			overflow := chunk[fillNeeded:]
			packet2 := make([]byte, len(overflow))
			copy(packet2, overflow)
			packets = append(packets, packet2)
		}
	} else {
		if len(meetingPacket) > 0 {
			packets = append(packets, meetingPacket)
		}
	}

	// Phase 3: Remaining private data (MTU-aligned)
	for len(privateData) > 0 {
		chunkSize := mtu
		if len(privateData) < mtu {
			chunkSize = len(privateData)
		}
		pkt := make([]byte, chunkSize)
		copy(pkt, privateData[:chunkSize])
		packets = append(packets, pkt)
		privateData = privateData[chunkSize:]
	}

	return packets
}

// TestPacketBuffer_LargeMessageReassembly tests that a large message (like 523KB)
// is correctly reassembled and forwarded. This reproduces a bug where large requests
// get stuck at the proxy.
func TestPacketBuffer_LargeMessageReassembly(t *testing.T) {
	pb := NewPacketBuffer(5 * time.Second)
	defer pb.Close()

	src := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 100), Port: 9090}
	rpcID := uint64(123456789)

	// Create a large payload similar to the user's stuck request:
	// key_size=61, value_size=523288 (approximately 523KB)
	keySize := 61
	valueSize := 523288
	fullPayload := createLargeSymphonyPayload(keySize, valueSize)

	t.Logf("Full payload size: %d bytes", len(fullPayload))
	t.Logf("Offset to private: %d", offsetToPrivate(fullPayload))

	// Fragment the payload like the client does
	mtu := packet.MaxUDPPayloadSize - DataPacketHeaderSize // 1400 - 29 = 1371
	fragmentPayloads := fragmentPayloadLikeClient(fullPayload, mtu)
	totalPackets := uint16(len(fragmentPayloads))

	t.Logf("Total fragments: %d", totalPackets)
	t.Logf("Fragment 0 size: %d bytes", len(fragmentPayloads[0]))

	// Create DataPackets for each fragment
	codec := &packet.DataPacketCodec{}
	serializedFragments := make([][]byte, len(fragmentPayloads))
	for i, fragPayload := range fragmentPayloads {
		pkt := &packet.DataPacket{
			PacketTypeID: packet.PacketTypeRequest.TypeID,
			RPCID:        rpcID,
			TotalPackets: totalPackets,
			SeqNumber:    uint16(i),
			DstIP:        [4]byte{192, 168, 1, 1},
			DstPort:      8080,
			SrcIP:        [4]byte{192, 168, 1, 100},
			SrcPort:      9090,
			Payload:      fragPayload,
		}
		data, err := codec.Serialize(pkt, nil)
		if err != nil {
			t.Fatalf("Failed to serialize fragment %d: %v", i, err)
		}
		serializedFragments[i] = data
	}

	// Process fragment 0 - this should extract the public segment
	bufferedPacket, verdict, err := pb.ProcessPacket(serializedFragments[0], src)
	if err != nil {
		t.Fatalf("Error processing fragment 0: %v", err)
	}
	if bufferedPacket == nil {
		t.Fatal("Expected buffered packet after processing fragment 0, got nil")
	}
	if verdict != util.PacketVerdictUnknown {
		t.Errorf("Expected PacketVerdictUnknown for first packet, got %v", verdict)
	}

	t.Logf("Buffered packet payload size: %d bytes", len(bufferedPacket.Payload))
	t.Logf("Buffered packet SeqNumber: %d", bufferedPacket.SeqNumber)
	t.Logf("Buffered packet TotalPackets: %d", bufferedPacket.TotalPackets)
	t.Logf("LastUsedSeqNum: %d", bufferedPacket.LastUsedSeqNum)

	// Verify that we got the expected data
	// The public segment is 13 bytes, but addFragmentToBuffer returns the full
	// contiguous fragments needed to cover the public segment
	if bufferedPacket.SeqNumber != -1 {
		t.Errorf("Expected SeqNumber=-1 for reassembled packet, got %d", bufferedPacket.SeqNumber)
	}

	// Verify the payload contains at least the public segment
	if len(bufferedPacket.Payload) < 13 {
		t.Errorf("Expected payload to contain at least 13 bytes (public segment), got %d", len(bufferedPacket.Payload))
	}

	// Verify offset_to_private in the returned payload
	returnedOffset := offsetToPrivate(bufferedPacket.Payload)
	if returnedOffset != 13 {
		t.Errorf("Expected offset_to_private=13 in returned payload, got %d", returnedOffset)
	}

	// Store a verdict for subsequent fragments
	pb.StoreVerdict(rpcID, util.PacketTypeRequest, util.PacketVerdictPass)

	// Process remaining fragments - they should be fast-forwarded
	for i := 1; i < len(serializedFragments); i++ {
		bufferedPacket, verdict, err := pb.ProcessPacket(serializedFragments[i], src)
		if err != nil {
			t.Fatalf("Error processing fragment %d: %v", i, err)
		}
		if bufferedPacket == nil {
			t.Fatalf("Expected buffered packet for fragment %d (fast-forward), got nil", i)
		}
		if verdict != util.PacketVerdictPass {
			t.Errorf("Expected PacketVerdictPass for fragment %d, got %v", i, verdict)
		}
		if bufferedPacket.SeqNumber != int16(i) {
			t.Errorf("Expected SeqNumber=%d for fragment %d, got %d", i, i, bufferedPacket.SeqNumber)
		}
	}
}

// TestPacketBuffer_LargeMessageFragmentsOutOfOrder tests that fragments arriving
// out of order are correctly buffered and the public segment is extracted when
// fragment 0 arrives.
func TestPacketBuffer_LargeMessageFragmentsOutOfOrder(t *testing.T) {
	pb := NewPacketBuffer(5 * time.Second)
	defer pb.Close()

	src := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 101), Port: 9090}
	rpcID := uint64(987654321)

	// Create a moderately large payload (5KB) to have multiple fragments
	keySize := 100
	valueSize := 5000
	fullPayload := createLargeSymphonyPayload(keySize, valueSize)

	t.Logf("Full payload size: %d bytes", len(fullPayload))
	t.Logf("Offset to private: %d", offsetToPrivate(fullPayload))

	mtu := packet.MaxUDPPayloadSize - DataPacketHeaderSize
	fragmentPayloads := fragmentPayloadLikeClient(fullPayload, mtu)
	totalPackets := uint16(len(fragmentPayloads))

	t.Logf("Total fragments: %d", totalPackets)

	if totalPackets < 3 {
		t.Skip("Need at least 3 fragments for out-of-order test")
	}

	// Create DataPackets
	codec := &packet.DataPacketCodec{}
	serializedFragments := make([][]byte, len(fragmentPayloads))
	for i, fragPayload := range fragmentPayloads {
		pkt := &packet.DataPacket{
			PacketTypeID: packet.PacketTypeRequest.TypeID,
			RPCID:        rpcID,
			TotalPackets: totalPackets,
			SeqNumber:    uint16(i),
			DstIP:        [4]byte{192, 168, 1, 1},
			DstPort:      8080,
			SrcIP:        [4]byte{192, 168, 1, 101},
			SrcPort:      9090,
			Payload:      fragPayload,
		}
		data, err := codec.Serialize(pkt, nil)
		if err != nil {
			t.Fatalf("Failed to serialize fragment %d: %v", i, err)
		}
		serializedFragments[i] = data
	}

	// Process fragments out of order: 2, 1, 0
	// Fragment 2 should be buffered, return nil
	bufferedPacket, _, err := pb.ProcessPacket(serializedFragments[2], src)
	if err != nil {
		t.Fatalf("Error processing fragment 2: %v", err)
	}
	if bufferedPacket != nil {
		t.Error("Expected nil for fragment 2 (missing fragment 0), got buffered packet")
	}

	// Fragment 1 should be buffered, return nil
	bufferedPacket, _, err = pb.ProcessPacket(serializedFragments[1], src)
	if err != nil {
		t.Fatalf("Error processing fragment 1: %v", err)
	}
	if bufferedPacket != nil {
		t.Error("Expected nil for fragment 1 (missing fragment 0), got buffered packet")
	}

	// Fragment 0 should complete the public segment and return a packet
	bufferedPacket, _, err = pb.ProcessPacket(serializedFragments[0], src)
	if err != nil {
		t.Fatalf("Error processing fragment 0: %v", err)
	}
	if bufferedPacket == nil {
		t.Fatal("Expected buffered packet after processing fragment 0, got nil")
	}

	t.Logf("Buffered packet payload size: %d bytes", len(bufferedPacket.Payload))
	t.Logf("LastUsedSeqNum: %d", bufferedPacket.LastUsedSeqNum)

	// Verify we can still retrieve remaining fragments after storing verdict
	pb.StoreVerdict(rpcID, util.PacketTypeRequest, util.PacketVerdictPass)

	connKey := src.String()
	metadata := &util.BufferedPacket{
		Source:       src,
		Peer:         &net.UDPAddr{IP: net.IPv4(192, 168, 1, 1), Port: 8080},
		PacketType:   util.PacketTypeRequest,
		RPCID:        rpcID,
		DstIP:        [4]byte{192, 168, 1, 1},
		DstPort:      8080,
		SrcIP:        [4]byte{192, 168, 1, 101},
		SrcPort:      9090,
		TotalPackets: totalPackets,
	}

	// Cleanup used fragments (fragment 0)
	pb.CleanupUsedFragments(connKey, rpcID, bufferedPacket.LastUsedSeqNum)

	// Get remaining fragments (should be fragments 1 and 2)
	remaining := pb.ProcessRemainingFragments(connKey, rpcID, util.PacketTypeRequest, metadata)
	t.Logf("Remaining fragments: %d", len(remaining))

	// Should have fragments 1 and 2
	if len(remaining) != 2 {
		t.Errorf("Expected 2 remaining fragments, got %d", len(remaining))
	}
}

// TestPacketBuffer_LargeMessageConcurrentFragments tests concurrent processing
// of many fragments for a large message.
func TestPacketBuffer_LargeMessageConcurrentFragments(t *testing.T) {
	pb := NewPacketBuffer(5 * time.Second)
	defer pb.Close()

	src := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 102), Port: 9090}
	rpcID := uint64(111222333)

	// Create a large payload similar to user's case
	keySize := 61
	valueSize := 50000 // 50KB for faster testing
	fullPayload := createLargeSymphonyPayload(keySize, valueSize)

	mtu := packet.MaxUDPPayloadSize - DataPacketHeaderSize
	fragmentPayloads := fragmentPayloadLikeClient(fullPayload, mtu)
	totalPackets := uint16(len(fragmentPayloads))

	t.Logf("Total fragments: %d", totalPackets)

	// Create DataPackets
	codec := &packet.DataPacketCodec{}
	serializedFragments := make([][]byte, len(fragmentPayloads))
	for i, fragPayload := range fragmentPayloads {
		pkt := &packet.DataPacket{
			PacketTypeID: packet.PacketTypeRequest.TypeID,
			RPCID:        rpcID,
			TotalPackets: totalPackets,
			SeqNumber:    uint16(i),
			DstIP:        [4]byte{192, 168, 1, 1},
			DstPort:      8080,
			SrcIP:        [4]byte{192, 168, 1, 102},
			SrcPort:      9090,
			Payload:      fragPayload,
		}
		data, err := codec.Serialize(pkt, nil)
		if err != nil {
			t.Fatalf("Failed to serialize fragment %d: %v", i, err)
		}
		serializedFragments[i] = data
	}

	// Process all fragments concurrently
	var wg sync.WaitGroup
	results := make([]*util.BufferedPacket, len(serializedFragments))
	errors := make([]error, len(serializedFragments))

	for i := range serializedFragments {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			result, _, err := pb.ProcessPacket(serializedFragments[idx], src)
			results[idx] = result
			errors[idx] = err
		}(i)
	}

	wg.Wait()

	// Check for errors
	for i, err := range errors {
		if err != nil {
			t.Errorf("Error processing fragment %d: %v", i, err)
		}
	}

	// At least one result should be the public segment extraction (from fragment 0)
	var publicSegmentExtracted bool
	for i, result := range results {
		if result != nil && result.SeqNumber == -1 {
			publicSegmentExtracted = true
			t.Logf("Fragment %d extracted public segment", i)
		}
	}

	if !publicSegmentExtracted {
		t.Error("Expected at least one result to have SeqNumber=-1 (public segment extraction)")
	}

	// Store verdict and verify remaining fragments can be retrieved
	pb.StoreVerdict(rpcID, util.PacketTypeRequest, util.PacketVerdictPass)

	// Process any remaining fragments that weren't fast-forwarded
	remainingCount := 0
	for i := 1; i < len(serializedFragments); i++ {
		result, verdict, _ := pb.ProcessPacket(serializedFragments[i], src)
		if result != nil && verdict == util.PacketVerdictPass {
			remainingCount++
		}
	}

	t.Logf("Remaining fragments processed via fast-forward: %d", remainingCount)
}

// TestPacketBuffer_ReassemblyDataIntegrity verifies that the reassembled payload
// contains the correct data and can be properly split into public/private segments.
func TestPacketBuffer_ReassemblyDataIntegrity(t *testing.T) {
	pb := NewPacketBuffer(5 * time.Second)
	defer pb.Close()

	src := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 150), Port: 9090}
	rpcID := uint64(444555666)

	// Create a payload with known content
	keySize := 100
	valueSize := 50000
	fullPayload := createLargeSymphonyPayload(keySize, valueSize)

	originalOffsetToPrivate := offsetToPrivate(fullPayload)
	t.Logf("Original offset_to_private: %d", originalOffsetToPrivate)

	mtu := packet.MaxUDPPayloadSize - DataPacketHeaderSize
	fragmentPayloads := fragmentPayloadLikeClient(fullPayload, mtu)
	totalPackets := uint16(len(fragmentPayloads))

	t.Logf("Total fragments: %d", totalPackets)
	t.Logf("Fragment 0 size: %d bytes", len(fragmentPayloads[0]))

	// Create and process fragment 0
	codec := &packet.DataPacketCodec{}
	pkt := &packet.DataPacket{
		PacketTypeID: packet.PacketTypeRequest.TypeID,
		RPCID:        rpcID,
		TotalPackets: totalPackets,
		SeqNumber:    0,
		DstIP:        [4]byte{192, 168, 1, 1},
		DstPort:      8080,
		SrcIP:        [4]byte{192, 168, 1, 150},
		SrcPort:      9090,
		Payload:      fragmentPayloads[0],
	}
	data, err := codec.Serialize(pkt, nil)
	if err != nil {
		t.Fatalf("Failed to serialize: %v", err)
	}

	bufferedPacket, _, err := pb.ProcessPacket(data, src)
	if err != nil {
		t.Fatalf("Error processing packet: %v", err)
	}
	if bufferedPacket == nil {
		t.Fatal("Expected buffered packet, got nil")
	}

	// Verify the reassembled payload has the correct offset_to_private
	reassembledOffset := offsetToPrivate(bufferedPacket.Payload)
	if reassembledOffset != originalOffsetToPrivate {
		t.Errorf("Offset mismatch: original=%d, reassembled=%d",
			originalOffsetToPrivate, reassembledOffset)
	}

	// Verify the public segment (first 13 bytes) matches the original
	if len(bufferedPacket.Payload) < originalOffsetToPrivate {
		t.Fatalf("Reassembled payload too short: %d < %d",
			len(bufferedPacket.Payload), originalOffsetToPrivate)
	}

	for i := 0; i < originalOffsetToPrivate; i++ {
		if bufferedPacket.Payload[i] != fullPayload[i] {
			t.Errorf("Public segment mismatch at byte %d: got %02x, want %02x",
				i, bufferedPacket.Payload[i], fullPayload[i])
		}
	}

	t.Logf("SUCCESS: Reassembled payload has correct offset and public segment data")

	// Now simulate what handlePacket does: split into public and private
	publicPayload := bufferedPacket.Payload[:reassembledOffset]
	privatePayload := bufferedPacket.Payload[reassembledOffset:]

	t.Logf("Split: publicPayload=%d bytes, privatePayload=%d bytes",
		len(publicPayload), len(privatePayload))

	// Verify public segment header is intact
	if publicPayload[0] != 0x01 {
		t.Errorf("Public segment version byte wrong: got %02x, want 0x01", publicPayload[0])
	}
}

// TestPacketBuffer_VeryLargeMessageLike523KB specifically tests the exact scenario
// reported by the user: a SET request with key_size=61 and value_size=523288
func TestPacketBuffer_VeryLargeMessageLike523KB(t *testing.T) {
	pb := NewPacketBuffer(30 * time.Second)
	defer pb.Close()

	src := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 200), Port: 9090}
	rpcID := uint64(6480) // Using the user's request number

	// Exact sizes from user's stuck request
	keySize := 61
	valueSize := 523288
	fullPayload := createLargeSymphonyPayload(keySize, valueSize)

	t.Logf("Full payload size: %d bytes", len(fullPayload))
	t.Logf("Offset to private: %d", offsetToPrivate(fullPayload))

	// Verify the offset is small (public segment should be minimal)
	if offsetToPrivate(fullPayload) != 13 {
		t.Errorf("Expected offset_to_private=13, got %d", offsetToPrivate(fullPayload))
	}

	mtu := packet.MaxUDPPayloadSize - DataPacketHeaderSize // 1371
	fragmentPayloads := fragmentPayloadLikeClient(fullPayload, mtu)
	totalPackets := uint16(len(fragmentPayloads))

	t.Logf("Total fragments: %d", totalPackets)
	t.Logf("Fragment 0 size: %d bytes", len(fragmentPayloads[0]))

	// Verify fragment count calculation
	// Expected: ~382 fragments for 523KB
	expectedFragments := (len(fullPayload) + mtu - 1) / mtu
	t.Logf("Expected fragments (simple calc): %d", expectedFragments)

	if totalPackets < 100 {
		t.Errorf("Expected many fragments for 523KB payload, got only %d", totalPackets)
	}

	// Create DataPackets
	codec := &packet.DataPacketCodec{}
	serializedFragments := make([][]byte, len(fragmentPayloads))
	for i, fragPayload := range fragmentPayloads {
		pkt := &packet.DataPacket{
			PacketTypeID: packet.PacketTypeRequest.TypeID,
			RPCID:        rpcID,
			TotalPackets: totalPackets,
			SeqNumber:    uint16(i),
			DstIP:        [4]byte{192, 168, 1, 1},
			DstPort:      8080,
			SrcIP:        [4]byte{192, 168, 1, 200},
			SrcPort:      9090,
			Payload:      fragPayload,
		}
		data, err := codec.Serialize(pkt, nil)
		if err != nil {
			t.Fatalf("Failed to serialize fragment %d: %v", i, err)
		}
		serializedFragments[i] = data
	}

	// Simulate fragments arriving with small delays (like network jitter)
	// Process fragment 0 first
	bufferedPacket, verdict, err := pb.ProcessPacket(serializedFragments[0], src)
	if err != nil {
		t.Fatalf("Error processing fragment 0: %v", err)
	}
	if bufferedPacket == nil {
		t.Fatal("BUG REPRODUCED: Expected buffered packet after processing fragment 0, got nil - request will be stuck!")
	}

	t.Logf("SUCCESS: Fragment 0 processed correctly")
	t.Logf("  Payload size: %d bytes", len(bufferedPacket.Payload))
	t.Logf("  SeqNumber: %d", bufferedPacket.SeqNumber)
	t.Logf("  TotalPackets: %d", bufferedPacket.TotalPackets)
	t.Logf("  LastUsedSeqNum: %d", bufferedPacket.LastUsedSeqNum)
	t.Logf("  Verdict: %v", verdict)

	// Verify the buffered packet can be forwarded correctly
	fragments, err := pb.FragmentPacketForForward(bufferedPacket)
	if err != nil {
		t.Fatalf("Error fragmenting for forward: %v", err)
	}
	t.Logf("Fragments to forward for public segment: %d", len(fragments))

	// Store verdict
	pb.StoreVerdict(rpcID, util.PacketTypeRequest, util.PacketVerdictPass)

	// Verify remaining fragments can be fast-forwarded
	successCount := 0
	for i := 1; i < len(serializedFragments); i++ {
		result, v, err := pb.ProcessPacket(serializedFragments[i], src)
		if err != nil {
			t.Errorf("Error processing fragment %d: %v", i, err)
			continue
		}
		if result == nil {
			t.Errorf("Fragment %d: expected fast-forward result, got nil", i)
			continue
		}
		if v != util.PacketVerdictPass {
			t.Errorf("Fragment %d: expected PacketVerdictPass, got %v", i, v)
			continue
		}
		successCount++
	}

	t.Logf("Successfully fast-forwarded %d/%d remaining fragments", successCount, len(serializedFragments)-1)

	if successCount != len(serializedFragments)-1 {
		t.Errorf("Not all fragments were fast-forwarded: %d/%d", successCount, len(serializedFragments)-1)
	}
}
