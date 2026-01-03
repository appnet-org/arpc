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

// verdictKey is a composite key for storing verdicts that distinguishes requests from responses
type verdictKey struct {
	RPCID      uint64
	PacketType util.PacketType
}

// PacketBuffer handles the buffering and reassembly of fragmented RPC packets
// Similar to DataReassembler but adapted for proxy use
type PacketBuffer struct {
	mu                     sync.RWMutex
	incoming               map[string]map[uint64]map[uint16][]byte // connectionKey -> rpcID -> seqNumber -> payload
	timeouts               map[string]map[uint64]time.Time         // connectionKey -> rpcID -> lastSeen
	publicSegmentExtracted map[string]map[uint64]bool              // connectionKey -> rpcID -> bool (track if public segment was already extracted)
	enabled                bool
	timeout                time.Duration
	cleanupTicker          *time.Ticker
	done                   chan struct{}
	verdicts               map[verdictKey]util.PacketVerdict
	verdictTimes           map[verdictKey]time.Time // verdictKey -> lastSeen
	verdictsMu             sync.RWMutex
}

// NewPacketBuffer creates a new packet buffer
func NewPacketBuffer(timeout time.Duration) *PacketBuffer {
	pb := &PacketBuffer{
		timeout:                timeout,
		incoming:               make(map[string]map[uint64]map[uint16][]byte),
		timeouts:               make(map[string]map[uint64]time.Time),
		publicSegmentExtracted: make(map[string]map[uint64]bool),
		done:                   make(chan struct{}),
		verdicts:               make(map[verdictKey]util.PacketVerdict),
		verdictTimes:           make(map[verdictKey]time.Time),
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

	// Check if a verdict exists for this RPC ID and packet type
	key := verdictKey{
		RPCID:      dataPacket.RPCID,
		PacketType: packetType,
	}
	pb.verdictsMu.RLock()
	verdict, verdictExists := pb.verdicts[key]
	pb.verdictsMu.RUnlock()

	if verdictExists {
		// Update last access time for verdict cleanup
		pb.verdictsMu.Lock()
		pb.verdictTimes[key] = time.Now()
		pb.verdictsMu.Unlock()
		logging.Debug("Verdict exists for RPC ID", zap.Uint64("rpcID", dataPacket.RPCID), zap.String("packetType", packetType.String()), zap.String("verdict", verdict.String()))
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

	logging.Debug("Adding fragment to buffer", zap.Uint64("rpcID", dataPacket.RPCID), zap.Int("seqNumber", int(dataPacket.SeqNumber)), zap.Int("size", len(dataPacket.Payload)))
	// Otherwise, add fragment to buffer and check if we have enough data
	publicSegment, lastUsedSeqNum := pb.addFragmentToBuffer(src, dataPacket)
	if publicSegment != nil {
		// We have enough contiguous data to cover the public segment (bytes 0 to offsetPrivate)
		// SeqNumber is set to -1 to indicate this is a new fragmented message (the public segment)
		// that will be re-fragmented if needed when forwarding
		return &util.BufferedPacket{
			Payload:        publicSegment,
			Source:         src,
			Peer:           peer,
			PacketType:     packetType,
			RPCID:          dataPacket.RPCID,
			DstIP:          dataPacket.DstIP,
			DstPort:        dataPacket.DstPort,
			SrcIP:          dataPacket.SrcIP,
			SrcPort:        dataPacket.SrcPort,
			IsFull:         false,                   // This is only the public segment, not the full packet
			SeqNumber:      -1,                      // -1 indicates a new message that will be fragmented if needed
			TotalPackets:   dataPacket.TotalPackets, // Original message's total (may be recalculated when fragmenting)
			LastUsedSeqNum: lastUsedSeqNum,          // Store last used seq num for cleanup
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
// Also returns the last sequence number used in the public segment (if public segment is ready)
func (pb *PacketBuffer) addFragmentToBuffer(src *net.UDPAddr, dataPacket *packet.DataPacket) ([]byte, uint16) {
	// Create connection key for this source
	connKey := src.String()

	pb.mu.Lock()
	defer pb.mu.Unlock()

	// Initialize maps if they don't exist
	if _, exists := pb.incoming[connKey]; !exists {
		pb.incoming[connKey] = make(map[uint64]map[uint16][]byte)
		pb.timeouts[connKey] = make(map[uint64]time.Time)
		pb.publicSegmentExtracted[connKey] = make(map[uint64]bool)
	}
	if _, exists := pb.incoming[connKey][dataPacket.RPCID]; !exists {
		pb.incoming[connKey][dataPacket.RPCID] = make(map[uint16][]byte)
	}

	// If public segment was already extracted, just buffer this fragment and return nil
	// This prevents duplicate extraction when private segment fragments arrive after public segment extraction
	// TODO: Related to out-of-order fragment bug - fragments buffered here after public segment
	// extraction need to be processed once a verdict exists (see CleanupUsedFragments TODO)
	if pb.publicSegmentExtracted[connKey][dataPacket.RPCID] {
		pb.incoming[connKey][dataPacket.RPCID][dataPacket.SeqNumber] = dataPacket.Payload
		pb.timeouts[connKey][dataPacket.RPCID] = time.Now()
		return nil, 0
	}

	// Store the fragment
	pb.incoming[connKey][dataPacket.RPCID][dataPacket.SeqNumber] = dataPacket.Payload
	pb.timeouts[connKey][dataPacket.RPCID] = time.Now()

	// Now, check if we have enough data to cover the public segment
	fragments := pb.incoming[connKey][dataPacket.RPCID]

	// Check if first packet exists and get offsetToPrivate
	firstPacketPayload, hasFirstPacket := fragments[0]
	if !hasFirstPacket {
		return nil, 0
	}

	offsetPrivate := offsetToPrivate(firstPacketPayload)

	// Check if we have contiguous data from fragment 0 up to the offset
	cumulativeSize := 0
	seqNum := uint16(0)
	for cumulativeSize < offsetPrivate {
		fragmentPayload, hasFragment := fragments[seqNum]
		if !hasFragment {
			// Missing fragment means we don't have contiguous data yet
			return nil, 0
		}
		cumulativeSize += len(fragmentPayload)
		seqNum++
	}

	// lastUsedSeqNum is the last sequence number used (seqNum-1 because seqNum was incremented after the last fragment)
	lastUsedSeqNum := seqNum - 1

	// We have enough contiguous data - reassemble the public segment
	publicSegment := make([]byte, 0, offsetPrivate)
	cumulativeSize = 0
	seqNum = 0
	for cumulativeSize < offsetPrivate {
		fragmentPayload := fragments[seqNum]
		// Only take the portion we need to reach offsetPrivate
		publicSegment = append(publicSegment, fragmentPayload...)
		cumulativeSize += len(fragmentPayload)
		seqNum++
	}
	// Mark that public segment has been extracted for this RPC ID
	pb.publicSegmentExtracted[connKey][dataPacket.RPCID] = true

	logging.Debug("Reassembled public segment", zap.Uint64("rpcID", dataPacket.RPCID), zap.Int("size", len(publicSegment)), zap.Uint16("lastUsedSeqNum", lastUsedSeqNum))
	return publicSegment, lastUsedSeqNum
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
	pb.mu.Lock()
	defer pb.mu.Unlock()

	if pb.incoming[connKey] != nil {
		delete(pb.incoming[connKey], rpcID)
	}
	if pb.timeouts[connKey] != nil {
		delete(pb.timeouts[connKey], rpcID)
	}

	// Clean up empty connection maps
	if len(pb.incoming[connKey]) == 0 {
		delete(pb.incoming, connKey)
		delete(pb.timeouts, connKey)
	}
}

// CleanupUsedFragments removes fragments up to and including the lastUsedSeqNum
// This should be called after the public segment has been successfully forwarded
func (pb *PacketBuffer) CleanupUsedFragments(connKey string, rpcID uint64, lastUsedSeqNum uint16) {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	fragments, exists := pb.incoming[connKey]
	if !exists {
		return
	}
	rpcFragments, exists := fragments[rpcID]
	if !exists {
		return
	}

	// Remove fragments from sequence number 0 up to and including lastUsedSeqNum
	for seqNum := uint16(0); seqNum <= lastUsedSeqNum; seqNum++ {
		delete(rpcFragments, seqNum)
	}

	// If all fragments are cleaned up, remove the RPC entry
	if len(rpcFragments) == 0 {
		delete(fragments, rpcID)
		if pb.timeouts[connKey] != nil {
			delete(pb.timeouts[connKey], rpcID)
		}
		// Clear the public segment extracted flag
		if pb.publicSegmentExtracted[connKey] != nil {
			delete(pb.publicSegmentExtracted[connKey], rpcID)
		}

		// Clean up empty connection maps
		if len(fragments) == 0 {
			delete(pb.incoming, connKey)
			if pb.timeouts[connKey] != nil {
				delete(pb.timeouts, connKey)
			}
			if pb.publicSegmentExtracted[connKey] != nil {
				delete(pb.publicSegmentExtracted, connKey)
			}
		}
	}

	logging.Debug("Cleaned up used fragments",
		zap.String("connKey", connKey),
		zap.Uint64("rpcID", rpcID),
		zap.Uint16("lastUsedSeqNum", lastUsedSeqNum))

	// TODO: Process remaining buffered fragments if verdict exists
	// Bug: If fragments arrive out-of-order (e.g., fragments 0, 2, then 1),
	// and the public segment is extracted from fragments 0+1, fragment 2
	// remains in the buffer but is never processed. After a verdict is stored,
	// we should check for remaining fragments and process them via fast-forward
	// path. This requires storing metadata (RPCID, PacketType, addresses) to
	// reconstruct BufferedPackets for remaining fragments.
}

// cleanupRoutine periodically cleans up expired fragments and verdicts
func (pb *PacketBuffer) cleanupRoutine() {
	for {
		select {
		case <-pb.cleanupTicker.C:
			pb.cleanupExpiredFragments()
			pb.cleanupExpiredVerdicts()
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
				if pb.publicSegmentExtracted[connKey] != nil {
					delete(pb.publicSegmentExtracted[connKey], rpcID)
				}
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
			if pb.publicSegmentExtracted[connKey] != nil {
				delete(pb.publicSegmentExtracted, connKey)
			}
		}
	}

	if expiredCount > 0 {
		logging.Debug("Cleanup completed", zap.Int("expiredRPCs", expiredCount))
	}
}

// cleanupExpiredVerdicts removes verdicts that have timed out
func (pb *PacketBuffer) cleanupExpiredVerdicts() {
	pb.verdictsMu.Lock()
	defer pb.verdictsMu.Unlock()

	now := time.Now()
	expiredCount := 0

	for key, lastSeen := range pb.verdictTimes {
		if now.Sub(lastSeen) > pb.timeout {
			// This verdict has timed out
			delete(pb.verdicts, key)
			delete(pb.verdictTimes, key)
			expiredCount++

			logging.Debug("Cleaned up expired verdict",
				zap.Uint64("rpcID", key.RPCID),
				zap.String("packetType", key.PacketType.String()),
				zap.Duration("age", now.Sub(lastSeen)))
		}
	}

	if expiredCount > 0 {
		logging.Debug("Verdict cleanup completed", zap.Int("expiredVerdicts", expiredCount))
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
