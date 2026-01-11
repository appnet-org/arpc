package main

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"net"
	"sync"
	"time"

	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/packet"
	"github.com/appnet-org/proxy/util"
	"go.uber.org/zap"
)

// DataPacketHeaderSize is the size of the DataPacket header in bytes
// Total: 1+8+2+2+4+2+4+2+4 = 29 bytes
const DataPacketHeaderSize = 29

const (
	// numShards is the number of shards for partitioning fragment storage
	numShards = 256
)

// verdictKey is a composite key for storing verdicts that distinguishes requests from responses
type verdictKey struct {
	RPCID      uint64
	PacketType util.PacketType
}

// verdictEntry stores verdict and timestamp for cleanup
type verdictEntry struct {
	Verdict    util.PacketVerdict
	LastAccess time.Time
}

// fragmentKey is a composite key for fragment storage
type fragmentKey struct {
	ConnKey string
	RPCID   uint64
	SeqNum  uint16
}

// rpcState tracks the state of an RPC's fragment reassembly
type rpcState struct {
	mu                     sync.Mutex // Per-RPC mutex for serialization
	Fragments              map[uint16][]byte
	TotalPackets           uint16
	PublicSegmentExtracted bool
	LastSeen               time.Time
}

// shard manages fragments for a subset of connections
type shard struct {
	mu        sync.RWMutex
	rpcStates map[string]map[uint64]*rpcState // connKey -> rpcID -> state
}

// PacketBuffer handles the buffering and reassembly of fragmented RPC packets
type PacketBuffer struct {
	shards        [numShards]*shard
	verdicts      sync.Map // map[verdictKey]*verdictEntry
	timeout       time.Duration
	cleanupTicker *time.Ticker
	done          chan struct{}
}

// NewPacketBuffer creates a new packet buffer
func NewPacketBuffer(timeout time.Duration) *PacketBuffer {
	pb := &PacketBuffer{
		timeout: timeout,
		done:    make(chan struct{}),
	}

	// Initialize shards
	for i := range pb.shards {
		pb.shards[i] = &shard{
			rpcStates: make(map[string]map[uint64]*rpcState),
		}
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

// getShard returns the shard for a given connection key
func (pb *PacketBuffer) getShard(connKey string) *shard {
	h := fnv.New32a()
	h.Write([]byte(connKey))
	return pb.shards[h.Sum32()%numShards]
}

// getOrCreateRPCState gets or creates the RPC state for a connection and RPC ID
func (s *shard) getOrCreateRPCState(connKey string, rpcID uint64, totalPackets uint16) *rpcState {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.rpcStates[connKey] == nil {
		s.rpcStates[connKey] = make(map[uint64]*rpcState)
	}

	state, exists := s.rpcStates[connKey][rpcID]
	if !exists {
		state = &rpcState{
			Fragments:              make(map[uint16][]byte),
			TotalPackets:           totalPackets,
			PublicSegmentExtracted: false,
			LastSeen:               time.Now(),
		}
		s.rpcStates[connKey][rpcID] = state
	} else {
		// Update TotalPackets if not set (don't update LastSeen here - that's done in addFragmentToBuffer)
		if state.TotalPackets == 0 {
			state.TotalPackets = totalPackets
		}
	}

	return state
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

	if val, ok := pb.verdicts.Load(key); ok {
		entry := val.(*verdictEntry)
		// Update last access time atomically
		pb.verdicts.Store(key, &verdictEntry{
			Verdict:    entry.Verdict,
			LastAccess: time.Now(),
		})

		logging.Debug("Verdict exists for RPC ID", zap.Uint64("rpcID", dataPacket.RPCID), zap.String("packetType", packetType.String()), zap.String("verdict", entry.Verdict.String()))
		// Verdict exists, forward the packet immediately without buffering or element chain processing
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
		}, entry.Verdict, nil
	}

	// If this is the first packet, the entire public segment fits in MTU (offset_private < MTU),
	// AND this is the only packet (no fragmentation), we can process it immediately without buffering.
	if dataPacket.SeqNumber == 0 && dataPacket.TotalPackets == 1 && isOffsetPrivateLessThanMTU(dataPacket.Payload) {
		logging.Debug("Single packet and entire public segment fits in MTU", zap.Uint64("rpcID", dataPacket.RPCID))
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
		// We have enough contiguous data to cover the public segment
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
			IsFull:         false,
			SeqNumber:      -1,
			TotalPackets:   dataPacket.TotalPackets,
			LastUsedSeqNum: lastUsedSeqNum,
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
	connKey := src.String()
	shard := pb.getShard(connKey)
	state := shard.getOrCreateRPCState(connKey, dataPacket.RPCID, dataPacket.TotalPackets)

	// Serialize fragment processing per RPC
	state.mu.Lock()
	defer state.mu.Unlock()

	// If public segment was already extracted, just buffer this fragment and return nil
	if state.PublicSegmentExtracted {
		// Make a copy of the payload
		payloadCopy := make([]byte, len(dataPacket.Payload))
		copy(payloadCopy, dataPacket.Payload)
		state.Fragments[dataPacket.SeqNumber] = payloadCopy
		state.LastSeen = time.Now()
		return nil, 0
	}

	// Make a copy of the payload
	payloadCopy := make([]byte, len(dataPacket.Payload))
	copy(payloadCopy, dataPacket.Payload)
	state.Fragments[dataPacket.SeqNumber] = payloadCopy
	state.LastSeen = time.Now()

	// Check if we have enough data to cover the public segment
	fragments := state.Fragments

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

	// lastUsedSeqNum is the last sequence number used
	lastUsedSeqNum := seqNum - 1

	// We have enough contiguous data - reassemble the public segment
	// Use buffer pool if available, otherwise allocate
	publicSegment := make([]byte, 0, offsetPrivate)
	cumulativeSize = 0
	seqNum = 0
	for cumulativeSize < offsetPrivate {
		fragmentPayload := fragments[seqNum]
		publicSegment = append(publicSegment, fragmentPayload...)
		cumulativeSize += len(fragmentPayload)
		seqNum++
	}

	// Mark that public segment has been extracted
	state.PublicSegmentExtracted = true

	logging.Debug("Reassembled public segment", zap.Uint64("rpcID", dataPacket.RPCID), zap.Int("size", len(publicSegment)), zap.Uint16("lastUsedSeqNum", lastUsedSeqNum))
	return publicSegment, lastUsedSeqNum
}

// deserializePacket extracts packet information using the existing packet codec
func (pb *PacketBuffer) deserializePacket(data []byte) (*packet.DataPacket, error) {
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

// CleanupUsedFragments removes fragments up to and including the lastUsedSeqNum
// This should be called after the public segment has been successfully forwarded
func (pb *PacketBuffer) CleanupUsedFragments(connKey string, rpcID uint64, lastUsedSeqNum uint16) {
	shard := pb.getShard(connKey)
	shard.mu.RLock()
	rpcStates, exists := shard.rpcStates[connKey]
	if !exists {
		shard.mu.RUnlock()
		return
	}
	state, exists := rpcStates[rpcID]
	if !exists {
		shard.mu.RUnlock()
		return
	}
	shard.mu.RUnlock()

	state.mu.Lock()
	defer state.mu.Unlock()

	// Remove fragments from sequence number 0 up to and including lastUsedSeqNum
	for seqNum := uint16(0); seqNum <= lastUsedSeqNum; seqNum++ {
		delete(state.Fragments, seqNum)
	}

	logging.Debug("Cleaned up used fragments",
		zap.String("connKey", connKey),
		zap.Uint64("rpcID", rpcID),
		zap.Uint16("lastUsedSeqNum", lastUsedSeqNum))
}

// ProcessRemainingFragments processes remaining buffered fragments after a verdict has been stored.
// It checks if a verdict exists for the RPC ID and packet type, then reconstructs BufferedPackets
// for all remaining fragments using the provided metadata. Returns an empty slice if no verdict
// exists or no remaining fragments are found.
func (pb *PacketBuffer) ProcessRemainingFragments(connKey string, rpcID uint64, packetType util.PacketType, metadata *util.BufferedPacket) []*util.BufferedPacket {
	// Check if a verdict exists for this RPC ID and packet type
	key := verdictKey{
		RPCID:      rpcID,
		PacketType: packetType,
	}

	val, verdictExists := pb.verdicts.Load(key)
	if !verdictExists {
		logging.Debug("No verdict exists for remaining fragments", zap.Uint64("rpcID", rpcID), zap.String("packetType", packetType.String()))
		return nil
	}

	entry := val.(*verdictEntry)
	if entry.Verdict == util.PacketVerdictDrop {
		logging.Debug("Verdict is drop for remaining fragments, skipping processing", zap.Uint64("rpcID", rpcID))
		return nil
	}

	shard := pb.getShard(connKey)
	shard.mu.RLock()
	rpcStates, exists := shard.rpcStates[connKey]
	if !exists {
		shard.mu.RUnlock()
		return nil
	}
	state, exists := rpcStates[rpcID]
	if !exists {
		shard.mu.RUnlock()
		return nil
	}
	shard.mu.RUnlock()

	state.mu.Lock()
	defer state.mu.Unlock()

	if len(state.Fragments) == 0 {
		return nil
	}

	totalPackets := metadata.TotalPackets
	if state.TotalPackets > 0 {
		totalPackets = state.TotalPackets
	}

	// Create BufferedPacket for each remaining fragment
	result := make([]*util.BufferedPacket, 0, len(state.Fragments))
	seqNumsToDelete := make([]uint16, 0, len(state.Fragments))

	for seqNum, payload := range state.Fragments {
		// Make a copy of the payload
		payloadCopy := make([]byte, len(payload))
		copy(payloadCopy, payload)

		bufferedPacket := &util.BufferedPacket{
			Payload:      payloadCopy,
			Source:       metadata.Source,
			Peer:         metadata.Peer,
			PacketType:   packetType,
			RPCID:        rpcID,
			DstIP:        metadata.DstIP,
			DstPort:      metadata.DstPort,
			SrcIP:        metadata.SrcIP,
			SrcPort:      metadata.SrcPort,
			IsFull:       false,
			SeqNumber:    int16(seqNum),
			TotalPackets: totalPackets,
		}
		result = append(result, bufferedPacket)
		seqNumsToDelete = append(seqNumsToDelete, seqNum)

		logging.Debug("Processing remaining fragment",
			zap.String("connKey", connKey),
			zap.Uint64("rpcID", rpcID),
			zap.Uint16("seqNum", seqNum),
			zap.String("verdict", entry.Verdict.String()))
	}

	// Remove processed fragments from buffer
	for _, seqNum := range seqNumsToDelete {
		delete(state.Fragments, seqNum)
	}

	logging.Debug("Processed remaining fragments",
		zap.String("connKey", connKey),
		zap.Uint64("rpcID", rpcID),
		zap.Int("count", len(result)))

	return result
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
	now := time.Now()
	expiredCount := 0

	for _, shard := range pb.shards {
		shard.mu.Lock()
		for connKey, rpcStates := range shard.rpcStates {
			var toDelete []uint64
			for rpcID, state := range rpcStates {
				state.mu.Lock()
				lastSeen := state.LastSeen
				state.mu.Unlock()

				if now.Sub(lastSeen) > pb.timeout {
					// This RPC has timed out - mark for deletion
					toDelete = append(toDelete, rpcID)
					expiredCount++

					logging.Debug("Cleaned up expired fragments",
						zap.String("connKey", connKey),
						zap.Uint64("rpcID", rpcID),
						zap.Duration("age", now.Sub(lastSeen)))
				}
			}
			// Delete expired RPCs
			for _, rpcID := range toDelete {
				delete(rpcStates, rpcID)
			}
			// Clean up empty connection maps
			if len(rpcStates) == 0 {
				delete(shard.rpcStates, connKey)
			}
		}
		shard.mu.Unlock()
	}

	if expiredCount > 0 {
		logging.Debug("Cleanup completed", zap.Int("expiredRPCs", expiredCount))
	}
}

// cleanupExpiredVerdicts removes verdicts that have timed out
func (pb *PacketBuffer) cleanupExpiredVerdicts() {
	now := time.Now()
	expiredCount := 0

	pb.verdicts.Range(func(key, value interface{}) bool {
		entry := value.(*verdictEntry)
		if now.Sub(entry.LastAccess) > pb.timeout {
			pb.verdicts.Delete(key)
			expiredCount++

			verdictKey := key.(verdictKey)
			logging.Debug("Cleaned up expired verdict",
				zap.Uint64("rpcID", verdictKey.RPCID),
				zap.String("packetType", verdictKey.PacketType.String()),
				zap.Duration("age", now.Sub(entry.LastAccess)))
		}
		return true
	})

	if expiredCount > 0 {
		logging.Debug("Verdict cleanup completed", zap.Int("expiredVerdicts", expiredCount))
	}
}

// GetStats returns buffer statistics for monitoring
func (pb *PacketBuffer) GetStats() map[string]any {
	stats := map[string]any{
		"timeout":           pb.timeout.String(),
		"activeConnections": 0,
		"totalFragments":    0,
	}

	for _, shard := range pb.shards {
		shard.mu.RLock()
		stats["activeConnections"] = stats["activeConnections"].(int) + len(shard.rpcStates)
		for _, rpcStates := range shard.rpcStates {
			for _, state := range rpcStates {
				state.mu.Lock()
				stats["totalFragments"] = stats["totalFragments"].(int) + len(state.Fragments)
				state.mu.Unlock()
			}
		}
		shard.mu.RUnlock()
	}

	return stats
}

// StoreVerdict stores a verdict for an RPC ID and packet type
func (pb *PacketBuffer) StoreVerdict(rpcID uint64, packetType util.PacketType, verdict util.PacketVerdict) {
	key := verdictKey{
		RPCID:      rpcID,
		PacketType: packetType,
	}
	pb.verdicts.Store(key, &verdictEntry{
		Verdict:    verdict,
		LastAccess: time.Now(),
	})
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
	completePayload := bufferedPacket.Payload
	chunkSize := packet.MaxUDPPayloadSize - DataPacketHeaderSize

	// Check if payload fits in a single packet
	if len(completePayload) <= chunkSize {
		seqNum := uint16(0)
		if bufferedPacket.SeqNumber >= 0 {
			seqNum = uint16(bufferedPacket.SeqNumber)
		}

		codec := &packet.DataPacketCodec{}
		singlePacket := &packet.DataPacket{
			PacketTypeID: packet.PacketTypeID(uint8(bufferedPacket.PacketType)),
			RPCID:        bufferedPacket.RPCID,
			TotalPackets: bufferedPacket.TotalPackets,
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

	// The complete payload exceeds MTU, need to fragment it for transmission
	totalfragments := uint16((len(completePayload) + chunkSize - 1) / chunkSize)
	codec := &packet.DataPacketCodec{}
	fragments := make([]FragmentedPacket, 0, totalfragments)
	for i := range int(totalfragments) {
		start := i * chunkSize
		end := min(start+chunkSize, len(completePayload))

		fragment := &packet.DataPacket{
			PacketTypeID: packet.PacketTypeID(uint8(bufferedPacket.PacketType)),
			RPCID:        bufferedPacket.RPCID,
			TotalPackets: totalfragments,
			SeqNumber:    uint16(i),
			DstIP:        bufferedPacket.DstIP,
			DstPort:      bufferedPacket.DstPort,
			SrcIP:        bufferedPacket.SrcIP,
			SrcPort:      bufferedPacket.SrcPort,
			Payload:      completePayload[start:end],
		}

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
		zap.Uint16("total packets", totalfragments),
		zap.Int("payload size", len(completePayload)))

	return fragments, nil
}
