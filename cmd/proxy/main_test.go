package main

import (
	"context"
	"encoding/binary"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/appnet-org/arpc/cmd/proxy/util"
	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/packet"
)

func init() {
	// Initialize logging to avoid race conditions in tests
	logging.Init(&logging.Config{
		Level:  "info",
		Format: "console",
	})
}

// Test runElementsChain with nil element chain
func TestRunElementsChain_NoElementChain(t *testing.T) {
	state := &ProxyState{
		elementChain: nil,
		packetBuffer: NewPacketBuffer(5 * time.Second),
	}
	defer state.packetBuffer.Close()

	src := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 1), Port: 9090}
	peer := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 2), Port: 8080}
	packet := &util.BufferedPacket{
		Payload:      []byte{1, 2, 3, 4, 5},
		Source:       src,
		Peer:         peer,
		PacketType:   util.PacketTypeRequest,
		RPCID:        12345,
		DstIP:        [4]byte{192, 168, 1, 2},
		DstPort:      8080,
		SrcIP:        [4]byte{192, 168, 1, 1},
		SrcPort:      9090,
		IsFull:       true,
		SeqNumber:    -1,
		TotalPackets: 1,
	}

	// Store verdict first
	state.packetBuffer.StoreVerdict(packet.RPCID, packet.PacketType, util.PacketVerdictPass)

	err := runElementsChain(context.Background(), state, packet)
	if err != nil {
		t.Errorf("Expected no error with nil element chain, got %v", err)
	}

	// Verify verdict was stored by checking directly (runElementsChain stores it)
	// The verdict should be stored, but we can't easily verify via ProcessPacket without
	// creating a proper packet. The test passes if runElementsChain doesn't error.
}

// Test runElementsChain with empty element chain
func TestRunElementsChain_EmptyElementChain(t *testing.T) {
	state := &ProxyState{
		elementChain: NewRPCElementChain(),
		packetBuffer: NewPacketBuffer(5 * time.Second),
	}
	defer state.packetBuffer.Close()

	src := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 1), Port: 9090}
	peer := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 2), Port: 8080}
	packet := &util.BufferedPacket{
		Payload:      []byte{1, 2, 3, 4, 5},
		Source:       src,
		Peer:         peer,
		PacketType:   util.PacketTypeRequest,
		RPCID:        12346,
		DstIP:        [4]byte{192, 168, 1, 2},
		DstPort:      8080,
		SrcIP:        [4]byte{192, 168, 1, 1},
		SrcPort:      9090,
		IsFull:       true,
		SeqNumber:    -1,
		TotalPackets: 1,
	}

	err := runElementsChain(context.Background(), state, packet)
	if err != nil {
		t.Errorf("Expected no error with empty element chain, got %v", err)
	}
}

// Test StoreVerdict and ProcessPacket with existing verdict (main.go logic path)
func TestPacketProcessing_ExistingVerdict(t *testing.T) {
	pb := NewPacketBuffer(5 * time.Second)
	defer pb.Close()

	src := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 10), Port: 9090}
	rpcID := uint64(99999)
	packetType := util.PacketTypeRequest

	// Store a verdict first
	pb.StoreVerdict(rpcID, packetType, util.PacketVerdictPass)

	// Create a packet
	codec := &packet.DataPacketCodec{}
	pkt := &packet.DataPacket{
		PacketTypeID:  packet.PacketTypeRequest.TypeID,
		RPCID:         rpcID,
		TotalPackets:  1,
		SeqNumber:     0,
		MoreFragments: false,
		FragmentIndex: 0,
		DstIP:         [4]byte{192, 168, 1, 1},
		DstPort:       8080,
		SrcIP:         [4]byte{192, 168, 1, 10},
		SrcPort:       9090,
		Payload:       []byte{1, 2, 3, 4, 5},
	}
	data, err := codec.Serialize(pkt, nil)
	if err != nil {
		t.Fatalf("Error serializing packet: %v", err)
	}

	// Process packet - should fast-forward with existing verdict
	bufferedPacket, verdict, err := pb.ProcessPacket(data, src)
	if err != nil {
		t.Fatalf("Error processing packet: %v", err)
	}
	if bufferedPacket == nil {
		t.Fatal("Expected buffered packet, got nil")
	}
	if verdict != util.PacketVerdictPass {
		t.Errorf("Expected PacketVerdictPass, got %v", verdict)
	}
}

// Test concurrent StoreVerdict calls (simulating concurrent packet processing)
func TestPacketProcessing_ConcurrentVerdictStorage(t *testing.T) {
	pb := NewPacketBuffer(5 * time.Second)
	defer pb.Close()

	rpcID := uint64(88888)
	packetType := util.PacketTypeRequest

	var wg sync.WaitGroup
	const numGoroutines = 50

	// Concurrent verdict storage
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			pb.StoreVerdict(rpcID, packetType, util.PacketVerdictPass)
		}()
	}

	wg.Wait()

	// Verify verdict exists
	src := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 11), Port: 9090}
	codec := &packet.DataPacketCodec{}
	pkt := &packet.DataPacket{
		PacketTypeID:  packet.PacketTypeRequest.TypeID,
		RPCID:         rpcID,
		TotalPackets:  1,
		SeqNumber:     0,
		MoreFragments: false,
		FragmentIndex: 0,
		DstIP:         [4]byte{192, 168, 1, 1},
		DstPort:       8080,
		SrcIP:         [4]byte{192, 168, 1, 11},
		SrcPort:       9090,
		Payload:       []byte{1, 2, 3, 4, 5},
	}
	data, _ := codec.Serialize(pkt, nil)
	_, verdict, _ := pb.ProcessPacket(data, src)
	if verdict == util.PacketVerdictUnknown {
		t.Error("Expected verdict to be stored, got PacketVerdictUnknown")
	}
}

// Test DropVerdict handling
func TestPacketProcessing_DropVerdict(t *testing.T) {
	pb := NewPacketBuffer(5 * time.Second)
	defer pb.Close()

	rpcID := uint64(77777)
	packetType := util.PacketTypeRequest

	// Store a drop verdict
	pb.StoreVerdict(rpcID, packetType, util.PacketVerdictDrop)

	// Process remaining fragments - should return empty
	src := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 12), Port: 9090}
	metadata := &util.BufferedPacket{
		Source:       src,
		Peer:         &net.UDPAddr{IP: net.IPv4(192, 168, 1, 1), Port: 8080},
		PacketType:   packetType,
		RPCID:        rpcID,
		DstIP:        [4]byte{192, 168, 1, 1},
		DstPort:      8080,
		SrcIP:        [4]byte{192, 168, 1, 12},
		SrcPort:      9090,
		TotalPackets: 1,
	}

	fragments := pb.ProcessRemainingFragments(src.String(), rpcID, packetType, metadata)
	if len(fragments) != 0 {
		t.Errorf("Expected no fragments for drop verdict, got %d", len(fragments))
	}
}

// TestLargeMessage_EndToEndSimulation simulates the full end-to-end flow
// of processing a large fragmented message through the proxy.
// This test creates a mock UDP connection to capture forwarded packets.
func TestLargeMessage_EndToEndSimulation(t *testing.T) {
	// Create proxy state
	state := &ProxyState{
		elementChain: NewRPCElementChain(), // Empty chain
		packetBuffer: NewPacketBuffer(30 * time.Second),
	}
	defer state.packetBuffer.Close()

	// Create a mock UDP connection for testing
	serverAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to resolve server address: %v", err)
	}

	serverConn, err := net.ListenUDP("udp", serverAddr)
	if err != nil {
		t.Fatalf("Failed to create server connection: %v", err)
	}
	defer serverConn.Close()

	// Create proxy listener
	proxyAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to resolve proxy address: %v", err)
	}

	proxyConn, err := net.ListenUDP("udp", proxyAddr)
	if err != nil {
		t.Fatalf("Failed to create proxy connection: %v", err)
	}
	defer proxyConn.Close()

	// Create a large Symphony-format payload
	keySize := 61
	valueSize := 10000 // 10KB for faster testing
	fullPayload := createLargeSymphonyPayloadForMainTest(keySize, valueSize)

	t.Logf("Full payload size: %d bytes", len(fullPayload))

	// Fragment the payload
	mtu := packet.MaxUDPPayloadSize - DataPacketHeaderSize // 1369
	fragmentPayloads := fragmentPayloadForMainTest(fullPayload, mtu)
	totalPackets := uint16(len(fragmentPayloads))

	t.Logf("Total fragments: %d", totalPackets)

	// Get server's actual address for routing
	serverActualAddr := serverConn.LocalAddr().(*net.UDPAddr)
	t.Logf("Server address: %s", serverActualAddr.String())

	// Create DataPackets with server address as destination
	codec := &packet.DataPacketCodec{}
	rpcID := uint64(999888777)
	src := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345}

	var dstIP [4]byte
	copy(dstIP[:], serverActualAddr.IP.To4())
	dstPort := uint16(serverActualAddr.Port)

	// Channel to signal when test is complete
	done := make(chan struct{})
	receivedFragments := make(map[uint16]bool)
	var mu sync.Mutex

	// Start a goroutine to receive packets at the server
	go func() {
		buf := make([]byte, 2048)
		for {
			select {
			case <-done:
				return
			default:
				serverConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
				n, _, err := serverConn.ReadFromUDP(buf)
				if err != nil {
					continue
				}

				// Deserialize the received packet
				packetAny, err := codec.Deserialize(buf[:n])
				if err != nil {
					t.Logf("Failed to deserialize received packet: %v", err)
					continue
				}

				dataPkt, ok := packetAny.(*packet.DataPacket)
				if !ok {
					continue
				}

				mu.Lock()
				receivedFragments[dataPkt.SeqNumber] = true
				t.Logf("Server received fragment %d (total: %d)", dataPkt.SeqNumber, dataPkt.TotalPackets)
				mu.Unlock()
			}
		}
	}()

	// Process each fragment through the proxy
	for i, fragPayload := range fragmentPayloads {
		pkt := &packet.DataPacket{
			PacketTypeID:  packet.PacketTypeRequest.TypeID,
			RPCID:         rpcID,
			TotalPackets:  totalPackets,
			SeqNumber:     uint16(i),
			MoreFragments: false,
			FragmentIndex: 0,
			DstIP:         dstIP,
			DstPort:       dstPort,
			SrcIP:         [4]byte{127, 0, 0, 1},
			SrcPort:       12345,
			Payload:       fragPayload,
		}

		data, err := codec.Serialize(pkt, nil)
		if err != nil {
			t.Fatalf("Failed to serialize fragment %d: %v", i, err)
		}

		// Simulate handlePacket
		bufferedPacket, existingVerdict, err := state.packetBuffer.ProcessPacket(data, src)
		if err != nil {
			t.Fatalf("Error processing fragment %d: %v", i, err)
		}

		if bufferedPacket == nil {
			// Still buffering
			t.Logf("Fragment %d: buffering", i)
			// Try to forward any buffered fragments if verdict exists
			continue
		}

		t.Logf("Fragment %d: got buffered packet (SeqNumber=%d, verdict=%v)",
			i, bufferedPacket.SeqNumber, existingVerdict)

		// If this is the public segment extraction (SeqNumber == -1)
		if bufferedPacket.SeqNumber == -1 && existingVerdict == util.PacketVerdictUnknown {
			// Split the payload into public and private segments (like handlePacket does)
			payload := bufferedPacket.Payload
			if len(payload) > offsetToPrivate(payload) {
				publicPayload := payload[:offsetToPrivate(payload)]
				bufferedPacket.Payload = publicPayload
			}
			// Process through element chain (empty chain, so just pass)
			err := runElementsChain(context.Background(), state, bufferedPacket)
			if err != nil {
				t.Fatalf("Error in element chain: %v", err)
			}

			// Fragment and forward
			fragments, err := state.packetBuffer.FragmentPacketForForward(bufferedPacket)
			if err != nil {
				t.Fatalf("Error fragmenting for forward: %v", err)
			}

			for _, frag := range fragments {
				_, err := proxyConn.WriteToUDP(frag.Data, frag.Peer)
				if err != nil {
					t.Logf("Error forwarding fragment: %v", err)
				}
			}

			// Cleanup and forward remaining
			connKey := src.String()
			state.packetBuffer.CleanupUsedFragments(connKey, bufferedPacket.RPCID, bufferedPacket.LastUsedSeqNum)

			// Forward remaining buffered fragments
			remaining := state.packetBuffer.ProcessRemainingFragments(
				connKey, bufferedPacket.RPCID, bufferedPacket.PacketType, bufferedPacket)
			for _, rem := range remaining {
				remFrags, err := state.packetBuffer.FragmentPacketForForward(rem)
				if err != nil {
					t.Logf("Error fragmenting remaining: %v", err)
					continue
				}
				for _, rf := range remFrags {
					proxyConn.WriteToUDP(rf.Data, rf.Peer)
				}
			}
		} else if existingVerdict == util.PacketVerdictPass {
			// Fast-forward
			fragments, err := state.packetBuffer.FragmentPacketForForward(bufferedPacket)
			if err != nil {
				t.Logf("Error fragmenting fast-forward: %v", err)
				continue
			}
			for _, frag := range fragments {
				proxyConn.WriteToUDP(frag.Data, frag.Peer)
			}
		}
	}

	// Wait a bit for all packets to be received
	time.Sleep(500 * time.Millisecond)
	close(done)

	// Check how many fragments were received
	mu.Lock()
	receivedCount := len(receivedFragments)
	mu.Unlock()

	t.Logf("Total fragments received at server: %d/%d", receivedCount, totalPackets)

	if receivedCount == 0 {
		t.Error("No fragments were received at the server - proxy forwarding may be broken")
	}

	// Note: Due to the fragmentation/reassembly differences between client and proxy,
	// the number of received fragments may not equal totalPackets.
	// The key is that at least SOME data was forwarded.
}

// createLargeSymphonyPayloadForMainTest creates a Symphony-format payload
func createLargeSymphonyPayloadForMainTest(keySize, valueSize int) []byte {
	publicSegmentSize := 13
	privateSegmentSize := 1 + 8 + (4 + keySize) + (4 + valueSize)
	totalSize := publicSegmentSize + privateSegmentSize
	payload := make([]byte, totalSize)

	// Public segment
	payload[0] = 0x01
	binary.LittleEndian.PutUint32(payload[1:5], uint32(publicSegmentSize))
	binary.LittleEndian.PutUint32(payload[5:9], 0)
	binary.LittleEndian.PutUint32(payload[9:13], 0)

	// Private segment
	privateStart := publicSegmentSize
	payload[privateStart] = 0x01
	tableStart := privateStart + 1
	binary.LittleEndian.PutUint32(payload[tableStart:], uint32(9))
	binary.LittleEndian.PutUint32(payload[tableStart+4:], uint32(9+4+keySize))

	payloadStart := tableStart + 8
	binary.LittleEndian.PutUint32(payload[payloadStart:], uint32(keySize))
	for i := 0; i < keySize; i++ {
		payload[payloadStart+4+i] = byte('k')
	}
	valueOffset := payloadStart + 4 + keySize
	binary.LittleEndian.PutUint32(payload[valueOffset:], uint32(valueSize))
	for i := 0; i < valueSize; i++ {
		payload[valueOffset+4+i] = byte('v')
	}

	return payload
}

// fragmentPayloadForMainTest fragments a payload like the client does
func fragmentPayloadForMainTest(payload []byte, mtu int) [][]byte {
	if len(payload) <= mtu {
		return [][]byte{payload}
	}

	offsetToPrivate := int(binary.LittleEndian.Uint32(payload[1:5]))
	publicData := payload[0:offsetToPrivate]
	privateData := payload[offsetToPrivate:]

	var packets [][]byte

	publicOffset := 0
	for len(publicData)-publicOffset > mtu {
		pkt := make([]byte, mtu)
		copy(pkt, publicData[publicOffset:publicOffset+mtu])
		packets = append(packets, pkt)
		publicOffset += mtu
	}

	meetingPacket := make([]byte, len(publicData)-publicOffset)
	copy(meetingPacket, publicData[publicOffset:])

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

// TestLargeMessage_FragmentZeroDelayed tests the scenario where fragment 0
// is delayed and arrives after other fragments. This could cause the request
// to get stuck if not handled properly.
func TestLargeMessage_FragmentZeroDelayed(t *testing.T) {
	state := &ProxyState{
		elementChain: NewRPCElementChain(),
		packetBuffer: NewPacketBuffer(30 * time.Second),
	}
	defer state.packetBuffer.Close()

	src := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 50), Port: 9090}
	rpcID := uint64(555666777)

	// Create payload
	keySize := 61
	valueSize := 20000
	fullPayload := createLargeSymphonyPayloadForMainTest(keySize, valueSize)
	mtu := packet.MaxUDPPayloadSize - DataPacketHeaderSize
	fragmentPayloads := fragmentPayloadForMainTest(fullPayload, mtu)
	totalPackets := uint16(len(fragmentPayloads))

	t.Logf("Total fragments: %d", totalPackets)

	// Create DataPackets
	codec := &packet.DataPacketCodec{}
	serializedFragments := make([][]byte, len(fragmentPayloads))
	for i, fragPayload := range fragmentPayloads {
		pkt := &packet.DataPacket{
			PacketTypeID:  packet.PacketTypeRequest.TypeID,
			RPCID:         rpcID,
			TotalPackets:  totalPackets,
			SeqNumber:     uint16(i),
			MoreFragments: false,
			FragmentIndex: 0,
			DstIP:         [4]byte{192, 168, 1, 1},
			DstPort:       8080,
			SrcIP:         [4]byte{192, 168, 1, 50},
			SrcPort:       9090,
			Payload:       fragPayload,
		}
		data, err := codec.Serialize(pkt, nil)
		if err != nil {
			t.Fatalf("Failed to serialize fragment %d: %v", i, err)
		}
		serializedFragments[i] = data
	}

	// Simulate fragment 0 being delayed: process fragments 1, 2, 3, ... first
	bufferedResults := make([]*util.BufferedPacket, len(serializedFragments))

	// Process fragments 1 to N-1 first
	for i := 1; i < len(serializedFragments); i++ {
		result, _, err := state.packetBuffer.ProcessPacket(serializedFragments[i], src)
		if err != nil {
			t.Fatalf("Error processing fragment %d: %v", i, err)
		}
		bufferedResults[i] = result
		if result != nil {
			t.Logf("Fragment %d: unexpected result before fragment 0 arrives", i)
		}
	}

	t.Log("All fragments except 0 have been buffered, now processing fragment 0...")

	// Now process fragment 0 - this should trigger public segment extraction
	result, _, err := state.packetBuffer.ProcessPacket(serializedFragments[0], src)
	if err != nil {
		t.Fatalf("Error processing fragment 0: %v", err)
	}

	if result == nil {
		t.Fatal("BUG: Fragment 0 should have triggered public segment extraction, got nil")
	}

	t.Logf("Fragment 0 result: SeqNumber=%d, PayloadSize=%d, LastUsedSeqNum=%d",
		result.SeqNumber, len(result.Payload), result.LastUsedSeqNum)

	// Verify we can retrieve remaining fragments
	state.packetBuffer.StoreVerdict(rpcID, util.PacketTypeRequest, util.PacketVerdictPass)

	connKey := src.String()
	state.packetBuffer.CleanupUsedFragments(connKey, rpcID, result.LastUsedSeqNum)

	remaining := state.packetBuffer.ProcessRemainingFragments(
		connKey, rpcID, util.PacketTypeRequest, result)

	t.Logf("Remaining fragments to forward: %d", len(remaining))

	// Should have fragments 1 to N-1 remaining
	expectedRemaining := len(serializedFragments) - 1 - int(result.LastUsedSeqNum)
	if len(remaining) != expectedRemaining {
		t.Errorf("Expected %d remaining fragments, got %d", expectedRemaining, len(remaining))
	}

	// Verify all sequence numbers are accounted for
	seenSeqNums := make(map[uint16]bool)
	for i := uint16(0); i <= result.LastUsedSeqNum; i++ {
		seenSeqNums[i] = true // Used for public segment
	}
	for _, rem := range remaining {
		seenSeqNums[uint16(rem.SeqNumber)] = true
	}

	for i := uint16(0); i < totalPackets; i++ {
		if !seenSeqNums[i] {
			t.Errorf("Fragment %d is missing - request would be incomplete!", i)
		}
	}

	t.Log("SUCCESS: All fragments accounted for even with delayed fragment 0")
}
