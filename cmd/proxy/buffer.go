package main

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/packet"
	"github.com/appnet-org/proxy/types"
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
}

// NewPacketBuffer creates a new packet buffer
func NewPacketBuffer(timeout time.Duration) *PacketBuffer {
	pb := &PacketBuffer{
		timeout:  timeout,
		incoming: make(map[string]map[uint64]map[uint16][]byte),
		timeouts: make(map[string]map[uint64]time.Time),
		done:     make(chan struct{}),
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

// ProcessPacket processes a packet fragment. If buffering is enabled, it buffers fragments
// and returns a complete packet when all fragments are received. If buffering is disabled
// or the packet is already complete, it returns immediately. Returns nil, nil if still
// waiting for more fragments.
func (pb *PacketBuffer) ProcessPacket(data []byte, src *net.UDPAddr, requestMode, responseMode types.ExecutionMode) (*types.BufferedPacket, error) {
	logging.Debug("Processing packet with execution modes",
		zap.String("requestMode", requestMode.String()),
		zap.String("responseMode", responseMode.String()))

	// Parse packet using the packet codec
	dataPacket, err := pb.deserializePacket(data)
	if err != nil {
		// Try to print packet type
		logging.Error("Failed to deserialize packet", zap.String("packetType", string(data[0])))
		return nil, err
	}

	peer := &net.UDPAddr{IP: net.IP(dataPacket.DstIP[:]), Port: int(dataPacket.DstPort)}

	// Determine which execution mode to check based on packet type
	packetType := types.PacketType(dataPacket.PacketTypeID)
	var executionMode types.ExecutionMode

	switch packetType {
	case types.PacketTypeRequest:
		executionMode = requestMode
	case types.PacketTypeResponse, types.PacketTypeError:
		executionMode = responseMode
	default:
		// For unknown/other packet types, default to requestMode
		executionMode = requestMode
	}

	// If the appropriate mode is StreamingMode, return the packet as is
	if executionMode == types.StreamingMode {
		isFull := true
		seqNumber := uint16(0)
		totalPackets := uint16(1)
		// Check if this is a fragment
		if dataPacket.TotalPackets > 1 {
			isFull = false
			seqNumber = dataPacket.SeqNumber
			totalPackets = dataPacket.TotalPackets
		}
		return &types.BufferedPacket{
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
			SeqNumber:    seqNumber,
			TotalPackets: totalPackets,
		}, nil
	}

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

	logging.Debug("Buffered packet fragment",
		zap.String("connKey", connKey),
		zap.Uint64("rpcID", dataPacket.RPCID),
		zap.Uint16("seqNumber", dataPacket.SeqNumber),
		zap.Uint16("totalPackets", dataPacket.TotalPackets),
		zap.Int("fragmentsReceived", len(pb.incoming[connKey][dataPacket.RPCID])))

	// Check if we have all fragments
	if len(pb.incoming[connKey][dataPacket.RPCID]) == int(dataPacket.TotalPackets) {
		// Reassemble the complete message payload
		completePayload, err := pb.reassemblePayload(dataPacket, pb.incoming[connKey][dataPacket.RPCID])
		if err != nil {
			logging.Error("Failed to reassemble packet", zap.Error(err))
			// Clean up and return original payload
			pb.cleanupFragments(connKey, dataPacket.RPCID)
			payload := dataPacket.Payload
			return &types.BufferedPacket{
				Payload:      payload,
				Source:       src,
				Peer:         peer,
				PacketType:   types.PacketType(dataPacket.PacketTypeID),
				RPCID:        dataPacket.RPCID,
				DstIP:        dataPacket.DstIP,
				DstPort:      dataPacket.DstPort,
				SrcIP:        dataPacket.SrcIP,
				SrcPort:      dataPacket.SrcPort,
				IsFull:       false,
				SeqNumber:    dataPacket.SeqNumber,
				TotalPackets: dataPacket.TotalPackets,
			}, nil
		}

		// Clean up fragment storage
		pb.cleanupFragments(connKey, dataPacket.RPCID)

		logging.Debug("Complete packet reassembled",
			zap.String("connKey", connKey),
			zap.Uint64("rpcID", dataPacket.RPCID),
			zap.Int("totalSize", len(completePayload)))

		return &types.BufferedPacket{
			Payload:      completePayload,
			Source:       src,
			Peer:         peer,
			PacketType:   types.PacketType(dataPacket.PacketTypeID),
			RPCID:        dataPacket.RPCID,
			DstIP:        dataPacket.DstIP,
			DstPort:      dataPacket.DstPort,
			SrcIP:        dataPacket.SrcIP,
			SrcPort:      dataPacket.SrcPort,
			IsFull:       true,
			SeqNumber:    0,
			TotalPackets: dataPacket.TotalPackets, // Actual number of packets that were reassembled
		}, nil
	}

	// Still waiting for more fragments
	return nil, nil
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
	PacketType types.PacketType
}

// FragmentPacketForForward fragments a complete packet if needed
// Returns a slice of fragmented packets to send
func (pb *PacketBuffer) FragmentPacketForForward(bufferedPacket *types.BufferedPacket) ([]FragmentedPacket, error) {
	// Use the payload directly from bufferedPacket
	completePayload := bufferedPacket.Payload
	chunkSize := packet.MaxUDPPayloadSize - 29 // 29 bytes for header
	totalPackets := uint16((len(completePayload) + chunkSize - 1) / chunkSize)

	// If only one packet is needed, create a single packet and return it
	if totalPackets <= 1 {
		// Reconstruct the full packet from payload
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

		serialized, err := codec.Serialize(singlePacket)
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

	// Need to fragment the packet
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
			SeqNumber:    uint16(i),
			DstIP:        bufferedPacket.DstIP,
			DstPort:      bufferedPacket.DstPort,
			SrcIP:        bufferedPacket.SrcIP,
			SrcPort:      bufferedPacket.SrcPort,
			Payload:      completePayload[start:end],
		}

		// Serialize the fragment
		serialized, err := codec.Serialize(fragment)
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
