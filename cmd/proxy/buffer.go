package main

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/packet"
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

// BufferedPacket represents a complete packet ready for processing
type BufferedPacket struct {
	Data       []byte
	Source     *net.UDPAddr
	Peer       *net.UDPAddr
	IsRequest  bool
	RPCID      uint64
	PacketType uint8
	// Routing information extracted from the packet
	DstIP   [4]byte
	DstPort uint16
	SrcIP   [4]byte
	SrcPort uint16
}

// NewPacketBuffer creates a new packet buffer
func NewPacketBuffer(enabled bool, timeout time.Duration) *PacketBuffer {
	pb := &PacketBuffer{
		enabled:  enabled,
		timeout:  timeout,
		incoming: make(map[string]map[uint64]map[uint16][]byte),
		timeouts: make(map[string]map[uint64]time.Time),
		done:     make(chan struct{}),
	}

	if enabled {
		// Start cleanup routine
		pb.cleanupTicker = time.NewTicker(timeout / 2)
		go pb.cleanupRoutine()
	}

	return pb
}

// Close stops the packet buffer and cleans up resources
func (pb *PacketBuffer) Close() {
	if pb.cleanupTicker != nil {
		pb.cleanupTicker.Stop()
	}
	close(pb.done)
}

// ProcessPacket processes a packet fragment and returns a complete packet if ready
func (pb *PacketBuffer) ProcessPacket(data []byte, src *net.UDPAddr, peer *net.UDPAddr, isRequest bool) (*BufferedPacket, error) {
	if !pb.enabled {
		// Buffering disabled, return packet immediately
		// Try to extract routing info
		routingInfo := pb.extractRoutingInfoFromData(data)
		return &BufferedPacket{
			Data:      data,
			Source:    src,
			Peer:      peer,
			IsRequest: isRequest,
			DstIP:     routingInfo.DstIP,
			DstPort:   routingInfo.DstPort,
			SrcIP:     routingInfo.SrcIP,
			SrcPort:   routingInfo.SrcPort,
		}, nil
	}

	// Parse packet using the existing packet codec
	dataPacket, err := pb.deserializePacket(data)
	if err != nil {
		logging.Debug("Failed to parse packet header, treating as single packet", zap.Error(err))
		// If we can't parse the header, treat it as a complete packet
		// Try to extract routing info from raw data
		routingInfo := pb.extractRoutingInfoFromData(data)
		return &BufferedPacket{
			Data:       data,
			Source:     src,
			Peer:       peer,
			IsRequest:  isRequest,
			PacketType: uint8(data[0]),
			DstIP:      routingInfo.DstIP,
			DstPort:    routingInfo.DstPort,
			SrcIP:      routingInfo.SrcIP,
			SrcPort:    routingInfo.SrcPort,
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
		// Reassemble the complete message
		completeData, err := pb.reassemblePacket(dataPacket, pb.incoming[connKey][dataPacket.RPCID])
		if err != nil {
			logging.Error("Failed to reassemble packet", zap.Error(err))
			// Clean up and return original data
			pb.cleanupFragments(connKey, dataPacket.RPCID)
			return &BufferedPacket{
				Data:       data,
				Source:     src,
				Peer:       peer,
				IsRequest:  isRequest,
				PacketType: uint8(dataPacket.PacketTypeID),
				DstIP:      dataPacket.DstIP,
				DstPort:    dataPacket.DstPort,
				SrcIP:      dataPacket.SrcIP,
				SrcPort:    dataPacket.SrcPort,
			}, nil
		}

		// Clean up fragment storage
		pb.cleanupFragments(connKey, dataPacket.RPCID)

		logging.Debug("Complete packet reassembled",
			zap.String("connKey", connKey),
			zap.Uint64("rpcID", dataPacket.RPCID),
			zap.Int("totalSize", len(completeData)))

		return &BufferedPacket{
			Data:       completeData,
			Source:     src,
			Peer:       peer,
			IsRequest:  isRequest,
			RPCID:      dataPacket.RPCID,
			PacketType: uint8(dataPacket.PacketTypeID),
			DstIP:      dataPacket.DstIP,
			DstPort:    dataPacket.DstPort,
			SrcIP:      dataPacket.SrcIP,
			SrcPort:    dataPacket.SrcPort,
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

// extractRoutingInfoFromData extracts routing information from raw packet data
func (pb *PacketBuffer) extractRoutingInfoFromData(data []byte) *BufferedPacket {
	routingInfo := &BufferedPacket{}

	// Try to deserialize and get routing info
	dataPacket, err := pb.deserializePacket(data)
	if err == nil {
		routingInfo.RPCID = dataPacket.RPCID
		routingInfo.PacketType = uint8(dataPacket.PacketTypeID)
		routingInfo.DstIP = dataPacket.DstIP
		routingInfo.DstPort = dataPacket.DstPort
		routingInfo.SrcIP = dataPacket.SrcIP
		routingInfo.SrcPort = dataPacket.SrcPort
	}

	return routingInfo
}

// reassemblePacket reconstructs the complete packet from fragments using the codec
func (pb *PacketBuffer) reassemblePacket(originalPacket *packet.DataPacket, fragments map[uint16][]byte) ([]byte, error) {
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

	// Create a new DataPacket representing the complete, reassembled packet
	completePacket := &packet.DataPacket{
		PacketTypeID: originalPacket.PacketTypeID,
		RPCID:        originalPacket.RPCID,
		TotalPackets: 1, // Now it's a single complete packet
		SeqNumber:    0, // Reassembled packet has sequence 0
		DstIP:        originalPacket.DstIP,
		DstPort:      originalPacket.DstPort,
		SrcIP:        originalPacket.SrcIP,
		SrcPort:      originalPacket.SrcPort,
		Payload:      completePayload,
	}

	// Use the codec to serialize the complete packet
	codec := &packet.DataPacketCodec{}
	return codec.Serialize(completePacket)
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
	Data      []byte
	Peer      *net.UDPAddr
	IsRequest bool
}

// FragmentPacketForForward fragments a complete packet if needed
// Returns a slice of fragmented packets to send
func (pb *PacketBuffer) FragmentPacketForForward(bufferedPacket *BufferedPacket) ([]FragmentedPacket, error) {
	// Deserialize the packet to extract payload
	dataPacket, err := pb.deserializePacket(bufferedPacket.Data)
	if err != nil {
		// If we can't parse it, treat as single packet and don't fragment
		return []FragmentedPacket{
			{
				Data:      bufferedPacket.Data,
				Peer:      bufferedPacket.Peer,
				IsRequest: bufferedPacket.IsRequest,
			},
		}, nil
	}

	// Extract the complete payload from the packet
	// Note: This could be either:
	// 1. The original payload of a non-fragmented packet
	// 2. The reassembled payload of fragments that were combined
	completePayload := dataPacket.Payload
	chunkSize := packet.MaxUDPPayloadSize - 29 // 29 bytes for header
	totalPackets := uint16((len(completePayload) + chunkSize - 1) / chunkSize)

	// If only one packet is needed, return as-is
	if totalPackets <= 1 {
		return []FragmentedPacket{
			{
				Data:      bufferedPacket.Data,
				Peer:      bufferedPacket.Peer,
				IsRequest: bufferedPacket.IsRequest,
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
			PacketTypeID: packet.PacketTypeID(bufferedPacket.PacketType),
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
			Data:      serialized,
			Peer:      bufferedPacket.Peer,
			IsRequest: bufferedPacket.IsRequest,
		})
	}

	logging.Debug("Fragmented packet for forwarding",
		zap.Uint64("rpcID", bufferedPacket.RPCID),
		zap.Uint16("totalFragments", totalPackets),
		zap.Int("originalSize", len(completePayload)))

	return fragments, nil
}
