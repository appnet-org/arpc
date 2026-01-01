package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/packet"
	"github.com/appnet-org/proxy/util"
	"go.uber.org/zap"
)

// PacketBuffer handles the buffering and reassembly of fragmented RPC packets
// Similar to DataReassembler but adapted for proxy use
type PacketBuffer struct {
	mu            sync.RWMutex
	incoming      map[string]map[uint64]map[uint16][]byte // connectionKey -> rpcID -> seqNumber -> payload
	timeouts      map[string]map[uint64]time.Time         // connectionKey -> rpcID -> lastSeen
	enabled       bool
	timeout       time.Duration
	cleanupTicker *time.Ticker
	done          chan struct{}
	verdicts      map[uint64]util.PacketVerdict
	verdictsMu    sync.RWMutex
}

// NewPacketBuffer creates a new packet buffer
func NewPacketBuffer(timeout time.Duration) *PacketBuffer {
	pb := &PacketBuffer{
		timeout:  timeout,
		incoming: make(map[string]map[uint64]map[uint16][]byte),
		timeouts: make(map[string]map[uint64]time.Time),
		done:     make(chan struct{}),
		verdicts: make(map[uint64]util.PacketVerdict),
	}

	// Start cleanup routine
	pb.cleanupTicker = time.NewTicker(timeout / 2)
	go pb.cleanupRoutine()

	return pb
}

// Close stops the packet buffer and cleans up resources
func (pb *PacketBuffer) Close() {
	if pb.cleanupTicker != nil {
		pb.cleanupTicker.Stop()
	}
	close(pb.done)
}

// ProcessPacket processes a packet fragment. It buffers fragments and returns a BufferedPacket
// when we have enough data to cover the public segment, or when a verdict already exists for this RPC ID.
// Returns (nil, PacketVerdictUnknown, nil) if still waiting for more fragments.
// Returns (BufferedPacket, verdict, nil) when a packet is ready for processing, where verdict indicates
// if a preexisting verdict exists (PacketVerdictPass/Drop) or PacketVerdictUnknown if this is the first time.
func (pb *PacketBuffer) ProcessPacket(data []byte, src *net.UDPAddr) (*util.BufferedPacket, util.PacketVerdict, error) {
	// Parse packet using the packet codec
	dataPacket, err := pb.deserializePacket(data)
	if err != nil {
		// Try to print packet type
		logging.Error("Failed to deserialize packet", zap.String("packetType", string(data[0])))
		return nil, util.PacketVerdictUnknown, err
	}

	peer := &net.UDPAddr{IP: net.IP(dataPacket.DstIP[:]), Port: int(dataPacket.DstPort)}
	packetType := util.PacketType(dataPacket.PacketTypeID)

	// Check if a verdict exists for this RPC ID and retrieve it
	pb.verdictsMu.RLock()
	verdict, verdictExists := pb.verdicts[dataPacket.RPCID]
	pb.verdictsMu.RUnlock()

	if verdictExists {
		logging.Debug("Verdict exists for RPC ID", zap.Uint64("rpcID", dataPacket.RPCID), zap.String("verdict", verdict.String()))
		// Verdict exists, forward the packet immediately without buffering or element chain processing
		// Preserve the original fragment's sequence number and TotalPackets for correct forwarding
		isFull := dataPacket.TotalPackets == 1
		seqNumber := uint16(0)
		if !isFull {
			seqNumber = dataPacket.SeqNumber
		}
		return &util.BufferedPacket{
			Payload:      dataPacket.Payload,
			Source:       src,
			Peer:         peer,
			PacketType:   packetType,
			RPCID:        dataPacket.RPCID,
			DstIP:        dataPacket.DstIP,
			DstPort:      dataPacket.DstPort,
			SrcIP:        dataPacket.SrcIP,
			SrcPort:      dataPacket.SrcPort,
			IsFull:       isFull,
			SeqNumber:    int16(seqNumber),
			TotalPackets: dataPacket.TotalPackets,
		}, verdict, nil
	}

	// If this is the first packet and the entire pubic segment fits in MTU (offset_private < MTU),
	// we can process it immediately without buffering
	if dataPacket.SeqNumber == 0 && isOffsetPrivateLessThanMTU(dataPacket.Payload) {
		logging.Debug("First packet and entire public segment fits in MTU", zap.Uint64("rpcID", dataPacket.RPCID))
		return &util.BufferedPacket{
			Payload:      dataPacket.Payload,
			Source:       src,
			Peer:         peer,
			PacketType:   packetType,
			RPCID:        dataPacket.RPCID,
			DstIP:        dataPacket.DstIP,
			DstPort:      dataPacket.DstPort,
			SrcIP:        dataPacket.SrcIP,
			SrcPort:      dataPacket.SrcPort,
			IsFull:       dataPacket.TotalPackets == 1,
			SeqNumber:    int16(dataPacket.SeqNumber),
			TotalPackets: dataPacket.TotalPackets,
		}, util.PacketVerdictUnknown, nil
	}

	logging.Debug("Adding fragment to buffer", zap.Uint64("rpcID", dataPacket.RPCID), zap.Int("seqNumber", int(dataPacket.SeqNumber)))
	// Otherwise, add fragment to buffer and check if we have enough data
	publicSegment := pb.addFragmentToBuffer(src, dataPacket)
	if publicSegment != nil {
		// We have enough contiguous data to cover the public segment (bytes 0 to offsetPrivate)
		// SeqNumber is set to -1 to indicate this is a new fragmented message (the public segment)
		// that will be re-fragmented if needed when forwarding
		return &util.BufferedPacket{
			Payload:      publicSegment,
			Source:       src,
			Peer:         peer,
			PacketType:   packetType,
			RPCID:        dataPacket.RPCID,
			DstIP:        dataPacket.DstIP,
			DstPort:      dataPacket.DstPort,
			SrcIP:        dataPacket.SrcIP,
			SrcPort:      dataPacket.SrcPort,
			IsFull:       false,                   // This is only the public segment, not the full packet
			SeqNumber:    -1,                      // -1 indicates a new message that will be fragmented if needed
			TotalPackets: dataPacket.TotalPackets, // Original message's total (may be recalculated when fragmenting)
		}, util.PacketVerdictUnknown, nil
	}

	// Still waiting for more fragments
	return nil, util.PacketVerdictUnknown, nil
}

// offsetToPrivate extracts the offset to private segment from the payload
// The offset is stored as a little-endian uint32 at bytes 1-5
func offsetToPrivate(payload []byte) int {
	if len(payload) < 5 {
		// Returns a large value if payload is too short to prevent incorrect processing
		return int(packet.MaxUDPPayloadSize) + 1
	}
	return int(binary.LittleEndian.Uint32(payload[1:5]))
}

// isOffsetPrivateLessThanMTU checks if the offset_private is less than the MTU
// If it is, we have the entire public partition and can process the packet immediately.
func isOffsetPrivateLessThanMTU(payload []byte) bool {
	return offsetToPrivate(payload) < packet.MaxUDPPayloadSize
}

// addFragmentToBuffer adds a packet fragment to the buffer for reassembly
// Returns the assembled public segment if enough data is available, nil otherwise
func (pb *PacketBuffer) addFragmentToBuffer(src *net.UDPAddr, dataPacket *packet.DataPacket) []byte {
	// Create connection key for this source
	connKey := src.String()

	pb.mu.Lock()
	defer pb.mu.Unlock()

	// Initialize maps if they don't exist
	if _, exists := pb.incoming[connKey]; !exists {
		pb.incoming[connKey] = make(map[uint64]map[uint16][]byte)
		pb.timeouts[connKey] = make(map[uint64]time.Time)
	}
	if _, exists := pb.incoming[connKey][dataPacket.RPCID]; !exists {
		pb.incoming[connKey][dataPacket.RPCID] = make(map[uint16][]byte)
	}

	// Store the fragment
	pb.incoming[connKey][dataPacket.RPCID][dataPacket.SeqNumber] = dataPacket.Payload
	pb.timeouts[connKey][dataPacket.RPCID] = time.Now()

	// Now, check if we have enough data to cover the public segment
	fragments := pb.incoming[connKey][dataPacket.RPCID]

	// Check if first packet exists and get offsetToPrivate
	firstPacketPayload, hasFirstPacket := fragments[0]
	if !hasFirstPacket {
		return nil
	}

	offsetPrivate := offsetToPrivate(firstPacketPayload)

	// Check if we have contiguous data from fragment 0 up to the offset
	cumulativeSize := 0
	seqNum := uint16(0)
	for cumulativeSize < offsetPrivate {
		fragmentPayload, hasFragment := fragments[seqNum]
		if !hasFragment {
			// Missing fragment means we don't have contiguous data yet
			return nil
		}
		cumulativeSize += len(fragmentPayload)
		seqNum++
	}

	// We have enough contiguous data - reassemble the public segment
	publicSegment := make([]byte, 0, offsetPrivate)
	cumulativeSize = 0
	seqNum = 0
	for cumulativeSize < offsetPrivate {
		fragmentPayload := fragments[seqNum]
		// Only take the portion we need to reach offsetPrivate
		remaining := offsetPrivate - cumulativeSize
		if len(fragmentPayload) <= remaining {
			publicSegment = append(publicSegment, fragmentPayload...)
			cumulativeSize += len(fragmentPayload)
		} else {
			publicSegment = append(publicSegment, fragmentPayload[:remaining]...)
			cumulativeSize = offsetPrivate
		}
		seqNum++
	}
	logging.Debug("Reassembled public segment", zap.Uint64("rpcID", dataPacket.RPCID), zap.Int("size", len(publicSegment)))
	return publicSegment
}

// deserializePacket extracts packet information using the existing packet codec
func (pb *PacketBuffer) deserializePacket(data []byte) (*packet.DataPacket, error) {
	// Use the existing DataPacketCodec to deserialize
	codec := &packet.DataPacketCodec{}
	packetAny, err := codec.Deserialize(data)
	if err != nil {
		return nil, err
	}

	dataPacket, ok := packetAny.(*packet.DataPacket)
	if !ok {
		return nil, fmt.Errorf("unexpected packet type")
	}

	return dataPacket, nil
}

// reassemblePayload reconstructs the complete payload from fragments
func (pb *PacketBuffer) reassemblePayload(originalPacket *packet.DataPacket, fragments map[uint16][]byte) ([]byte, error) {
	// Calculate total payload size
	var totalPayloadSize int
	for i := range int(originalPacket.TotalPackets) {
		if fragment, exists := fragments[uint16(i)]; exists {
			totalPayloadSize += len(fragment)
		} else {
			return nil, fmt.Errorf("missing fragment %d for RPC %d", i, originalPacket.RPCID)
		}
	}

	// Concatenate fragments in order to create complete payload
	completePayload := make([]byte, 0, totalPayloadSize)
	for i := range int(originalPacket.TotalPackets) {
		fragment := fragments[uint16(i)]
		completePayload = append(completePayload, fragment...)
	}

	return completePayload, nil
}

// cleanupFragments removes fragment storage for a completed RPC
func (pb *PacketBuffer) cleanupFragments(connKey string, rpcID uint64) {
	delete(pb.incoming[connKey], rpcID)
	delete(pb.timeouts[connKey], rpcID)

	// Clean up empty connection maps
	if len(pb.incoming[connKey]) == 0 {
		delete(pb.incoming, connKey)
		delete(pb.timeouts, connKey)
	}
}

// cleanupRoutine periodically cleans up expired fragments
func (pb *PacketBuffer) cleanupRoutine() {
	for {
		select {
		case <-pb.cleanupTicker.C:
			pb.cleanupExpiredFragments()
		case <-pb.done:
			return
		}
	}
}

// cleanupExpiredFragments removes fragments that have timed out
func (pb *PacketBuffer) cleanupExpiredFragments() {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	now := time.Now()
	expiredCount := 0

	for connKey, timeouts := range pb.timeouts {
		for rpcID, lastSeen := range timeouts {
			if now.Sub(lastSeen) > pb.timeout {
				// This RPC has timed out
				delete(pb.incoming[connKey], rpcID)
				delete(pb.timeouts[connKey], rpcID)
				expiredCount++

				logging.Debug("Cleaned up expired fragments",
					zap.String("connKey", connKey),
					zap.Uint64("rpcID", rpcID),
					zap.Duration("age", now.Sub(lastSeen)))
			}
		}

		// Clean up empty connection maps
		if len(pb.incoming[connKey]) == 0 {
			delete(pb.incoming, connKey)
			delete(pb.timeouts, connKey)
		}
	}

	if expiredCount > 0 {
		logging.Debug("Cleanup completed", zap.Int("expiredRPCs", expiredCount))
	}
}

// GetStats returns buffer statistics for monitoring
func (pb *PacketBuffer) GetStats() map[string]any {
	pb.mu.RLock()
	defer pb.mu.RUnlock()

	stats := map[string]any{
		"enabled":           pb.enabled,
		"timeout":           pb.timeout.String(),
		"activeConnections": len(pb.incoming),
		"totalFragments":    0,
	}

	for _, fragments := range pb.incoming {
		stats["totalFragments"] = stats["totalFragments"].(int) + len(fragments)
	}

	return stats
}

// FragmentedPacket represents a fragment ready to be sent
type FragmentedPacket struct {
	Data       []byte
	Peer       *net.UDPAddr
	PacketType util.PacketType
}

// FragmentPacketForForward fragments a packet payload for transmission if needed.
// For single packets: preserves original sequence number and TotalPackets when forwarding existing fragments.
// For multi-packet: creates a new fragmented message with sequence numbers starting from 0.
// Returns a slice of fragmented packets ready to send.
func (pb *PacketBuffer) FragmentPacketForForward(bufferedPacket *util.BufferedPacket) ([]FragmentedPacket, error) {
	// Use the payload directly from bufferedPacket (may have been modified by element chain)
	completePayload := bufferedPacket.Payload
	chunkSize := packet.MaxUDPPayloadSize - 29 // 29 bytes for header (1+8+2+2+4+2+4+2+4)

	// Check if payload fits in a single packet
	if len(completePayload) <= chunkSize {
		// Single packet case
		seqNum := uint16(0)
		totalPackets := uint16(1)

		if bufferedPacket.SeqNumber >= 0 {
			// Forwarding an existing fragment - preserve its original sequence number and TotalPackets
			// to maintain the original fragmented message structure
			seqNum = uint16(bufferedPacket.SeqNumber)
			if bufferedPacket.TotalPackets > 0 {
				totalPackets = bufferedPacket.TotalPackets
			}
		}
		// If SeqNumber < 0 (i.e., -1), this is a new message/public segment, use seq 0 and TotalPackets 1

		// Create a single packet from payload
		codec := &packet.DataPacketCodec{}
		singlePacket := &packet.DataPacket{
			PacketTypeID: packet.PacketTypeID(uint8(bufferedPacket.PacketType)),
			RPCID:        bufferedPacket.RPCID,
			TotalPackets: totalPackets,
			SeqNumber:    seqNum,
			DstIP:        bufferedPacket.DstIP,
			DstPort:      bufferedPacket.DstPort,
			SrcIP:        bufferedPacket.SrcIP,
			SrcPort:      bufferedPacket.SrcPort,
			Payload:      completePayload,
		}

		serialized, err := codec.Serialize(singlePacket, nil)
		if err != nil {
			logging.Error("Failed to serialize packet", zap.Error(err))
			return nil, err
		}

		return []FragmentedPacket{
			{
				Data:       serialized,
				Peer:       bufferedPacket.Peer,
				PacketType: bufferedPacket.PacketType,
			},
		}, nil
	}

	// The complete payload exceeds MTU, need to fragment it for transmission.
	// This creates a NEW fragmented message (even if the original was fragmented),
	// so sequence numbers start from 0 and TotalPackets is recalculated based on the payload size.
	// Note: If the payload was modified by element chain, we correctly recalculate fragmentation here.
	totalPackets := uint16((len(completePayload) + chunkSize - 1) / chunkSize)
	codec := &packet.DataPacketCodec{}
	fragments := make([]FragmentedPacket, 0, totalPackets)
	for i := range int(totalPackets) {
		start := i * chunkSize
		end := min(start+chunkSize, len(completePayload))

		// Create a fragment packet using routing info from bufferedPacket
		fragment := &packet.DataPacket{
			PacketTypeID: packet.PacketTypeID(uint8(bufferedPacket.PacketType)),
			RPCID:        bufferedPacket.RPCID,
			TotalPackets: totalPackets,
			SeqNumber:    uint16(i), // New fragmented message, start from 0
			DstIP:        bufferedPacket.DstIP,
			DstPort:      bufferedPacket.DstPort,
			SrcIP:        bufferedPacket.SrcIP,
			SrcPort:      bufferedPacket.SrcPort,
			Payload:      completePayload[start:end],
		}

		// Serialize the fragment
		serialized, err := codec.Serialize(fragment, nil)
		if err != nil {
			logging.Error("Failed to serialize fragment", zap.Error(err))
			return nil, err
		}

		fragments = append(fragments, FragmentedPacket{
			Data:       serialized,
			Peer:       bufferedPacket.Peer,
			PacketType: bufferedPacket.PacketType,
		})
	}

	logging.Debug("Fragmented packet for forwarding",
		zap.Uint64("rpcID", bufferedPacket.RPCID),
		zap.Uint16("totalFragments", totalPackets),
		zap.Int("originalSize", len(completePayload)))

	return fragments, nil
}
