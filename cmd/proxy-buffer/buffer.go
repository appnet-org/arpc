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
	"github.com/appnet-org/proxy-buffer/util"
	"go.uber.org/zap"
)

// DataPacketHeaderSize is the size of the DataPacket header in bytes
// Total: 1+8+2+2+4+2+4+2+4 = 29 bytes
const DataPacketHeaderSize = 29

const (
	// numShards is the number of shards for partitioning fragment storage
	numShards = 256
)

// rpcKey is a composite key for RPC storage
type rpcKey struct {
	ConnKey string
	RPCID   uint64
}

// rpcState tracks the state of an RPC's fragment reassembly
type rpcState struct {
	mu           sync.Mutex // Per-RPC mutex for serialization
	Fragments    map[uint16][]byte
	TotalPackets uint16
	LastSeen     time.Time
	// Metadata from first fragment
	Source     *net.UDPAddr
	Peer       *net.UDPAddr
	PacketType util.PacketType
	DstIP      [4]byte
	DstPort    uint16
	SrcIP      [4]byte
	SrcPort    uint16
}

// shard manages fragments for a subset of connections
type shard struct {
	mu        sync.RWMutex
	rpcStates map[string]map[uint64]*rpcState // connKey -> rpcID -> state
}

// PacketBuffer handles the buffering and reassembly of fragmented RPC packets
// This simplified version buffers ALL fragments for an RPC before returning
type PacketBuffer struct {
	shards        [numShards]*shard
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
func (s *shard) getOrCreateRPCState(connKey string, rpcID uint64, totalPackets uint16, src, peer *net.UDPAddr, packetType util.PacketType, dataPacket *packet.DataPacket) *rpcState {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.rpcStates[connKey] == nil {
		s.rpcStates[connKey] = make(map[uint64]*rpcState)
	}

	state, exists := s.rpcStates[connKey][rpcID]
	if !exists {
		state = &rpcState{
			Fragments:    make(map[uint16][]byte),
			TotalPackets: totalPackets,
			LastSeen:     time.Now(),
			Source:       src,
			Peer:         peer,
			PacketType:   packetType,
			DstIP:        dataPacket.DstIP,
			DstPort:      dataPacket.DstPort,
			SrcIP:        dataPacket.SrcIP,
			SrcPort:      dataPacket.SrcPort,
		}
		s.rpcStates[connKey][rpcID] = state
	} else {
		// Update TotalPackets if not set
		if state.TotalPackets == 0 {
			state.TotalPackets = totalPackets
		}
	}

	return state
}

// ProcessPacket processes a packet fragment. It buffers fragments and returns a BufferedPacket
// only when ALL fragments for the RPC have been received and reassembled.
// Returns (nil, nil) if still waiting for more fragments.
// Returns (BufferedPacket, nil) when all fragments are received and reassembled.
func (pb *PacketBuffer) ProcessPacket(data []byte, src *net.UDPAddr) (*util.BufferedPacket, error) {
	// Parse packet using the packet codec
	dataPacket, err := pb.deserializePacket(data)
	if err != nil {
		logging.Error("Failed to deserialize packet", zap.String("packetType", string(data[0])))
		return nil, err
	}

	peer := &net.UDPAddr{IP: net.IP(dataPacket.DstIP[:]), Port: int(dataPacket.DstPort)}
	packetType := util.PacketType(dataPacket.PacketTypeID)

	// If this is a single packet (no fragmentation), process immediately
	if dataPacket.TotalPackets == 1 {
		logging.Debug("Single packet RPC, no buffering needed", zap.Uint64("rpcID", dataPacket.RPCID))
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
			TotalPackets: dataPacket.TotalPackets,
		}, nil
	}

	logging.Debug("Adding fragment to buffer",
		zap.Uint64("rpcID", dataPacket.RPCID),
		zap.Uint16("seqNumber", dataPacket.SeqNumber),
		zap.Uint16("totalPackets", dataPacket.TotalPackets),
		zap.Int("size", len(dataPacket.Payload)))

	// Add fragment to buffer and check if we have all fragments
	completePayload := pb.addFragmentToBuffer(src, peer, packetType, dataPacket)
	if completePayload != nil {
		// All fragments received - return the complete message
		return &util.BufferedPacket{
			Payload:      completePayload,
			Source:       src,
			Peer:         peer,
			PacketType:   packetType,
			RPCID:        dataPacket.RPCID,
			DstIP:        dataPacket.DstIP,
			DstPort:      dataPacket.DstPort,
			SrcIP:        dataPacket.SrcIP,
			SrcPort:      dataPacket.SrcPort,
			TotalPackets: dataPacket.TotalPackets,
		}, nil
	}

	// Still waiting for more fragments
	return nil, nil
}

// addFragmentToBuffer adds a packet fragment to the buffer for reassembly
// Returns the complete reassembled payload if all fragments are received, nil otherwise
func (pb *PacketBuffer) addFragmentToBuffer(src, peer *net.UDPAddr, packetType util.PacketType, dataPacket *packet.DataPacket) []byte {
	connKey := src.String()
	shard := pb.getShard(connKey)
	state := shard.getOrCreateRPCState(connKey, dataPacket.RPCID, dataPacket.TotalPackets, src, peer, packetType, dataPacket)

	// Serialize fragment processing per RPC
	state.mu.Lock()
	defer state.mu.Unlock()

	// Make a copy of the payload
	payloadCopy := make([]byte, len(dataPacket.Payload))
	copy(payloadCopy, dataPacket.Payload)
	state.Fragments[dataPacket.SeqNumber] = payloadCopy
	state.LastSeen = time.Now()

	// Check if we have all fragments
	if uint16(len(state.Fragments)) < state.TotalPackets {
		logging.Debug("Still waiting for fragments",
			zap.Uint64("rpcID", dataPacket.RPCID),
			zap.Int("received", len(state.Fragments)),
			zap.Uint16("total", state.TotalPackets))
		return nil
	}

	// All fragments received - reassemble in order
	var completePayload []byte
	for i := uint16(0); i < state.TotalPackets; i++ {
		fragment, exists := state.Fragments[i]
		if !exists {
			// This shouldn't happen if our count is correct, but be safe
			logging.Error("Missing fragment during reassembly",
				zap.Uint64("rpcID", dataPacket.RPCID),
				zap.Uint16("seqNum", i))
			return nil
		}
		completePayload = append(completePayload, fragment...)
	}

	logging.Debug("All fragments received, reassembled complete message",
		zap.Uint64("rpcID", dataPacket.RPCID),
		zap.Uint16("totalPackets", state.TotalPackets),
		zap.Int("size", len(completePayload)))

	// Clean up the RPC state after successful reassembly
	pb.cleanupRPCState(connKey, dataPacket.RPCID)

	return completePayload
}

// cleanupRPCState removes the RPC state for a completed RPC
func (pb *PacketBuffer) cleanupRPCState(connKey string, rpcID uint64) {
	shard := pb.getShard(connKey)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	if rpcStates, exists := shard.rpcStates[connKey]; exists {
		delete(rpcStates, rpcID)
		if len(rpcStates) == 0 {
			delete(shard.rpcStates, connKey)
		}
	}
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

// offsetToPrivate extracts the offset to private segment from the payload
// The offset is stored as a little-endian uint32 at bytes 1-5
func offsetToPrivate(payload []byte) int {
	if len(payload) < 5 {
		// Returns a large value if payload is too short to prevent incorrect processing
		return int(packet.MaxUDPPayloadSize) + 1
	}
	return int(binary.LittleEndian.Uint32(payload[1:5]))
}

// FragmentedPacket represents a fragment ready to be sent
type FragmentedPacket struct {
	Data       []byte
	Peer       *net.UDPAddr
	PacketType util.PacketType
}

// FragmentPacketForForward fragments a packet payload for transmission if needed.
// Returns a slice of fragmented packets ready to send.
func (pb *PacketBuffer) FragmentPacketForForward(bufferedPacket *util.BufferedPacket) ([]FragmentedPacket, error) {
	completePayload := bufferedPacket.Payload
	chunkSize := packet.MaxUDPPayloadSize - DataPacketHeaderSize

	// Check if payload fits in a single packet
	if len(completePayload) <= chunkSize {
		codec := &packet.DataPacketCodec{}
		singlePacket := &packet.DataPacket{
			PacketTypeID: packet.PacketTypeID(uint8(bufferedPacket.PacketType)),
			RPCID:        bufferedPacket.RPCID,
			TotalPackets: 1,
			SeqNumber:    0,
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
	totalFragments := uint16((len(completePayload) + chunkSize - 1) / chunkSize)
	codec := &packet.DataPacketCodec{}
	fragments := make([]FragmentedPacket, 0, totalFragments)
	for i := range int(totalFragments) {
		start := i * chunkSize
		end := min(start+chunkSize, len(completePayload))

		fragment := &packet.DataPacket{
			PacketTypeID: packet.PacketTypeID(uint8(bufferedPacket.PacketType)),
			RPCID:        bufferedPacket.RPCID,
			TotalPackets: totalFragments,
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
		zap.Uint16("total packets", totalFragments),
		zap.Int("payload size", len(completePayload)))

	return fragments, nil
}
