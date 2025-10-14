package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/appnet-org/arpc/pkg/logging"
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
		return &BufferedPacket{
			Data:      data,
			Source:    src,
			Peer:      peer,
			IsRequest: isRequest,
		}, nil
	}

	// Parse packet header to extract RPC information
	packetType, rpcID, totalPackets, seqNumber, payload, err := pb.parsePacketHeader(data)
	if err != nil {
		logging.Debug("Failed to parse packet header, treating as single packet", zap.Error(err))
		// If we can't parse the header, treat it as a complete packet
		return &BufferedPacket{
			Data:       data,
			Source:     src,
			Peer:       peer,
			IsRequest:  isRequest,
			PacketType: uint8(data[0]),
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
	if _, exists := pb.incoming[connKey][rpcID]; !exists {
		pb.incoming[connKey][rpcID] = make(map[uint16][]byte)
	}

	// Store the fragment
	pb.incoming[connKey][rpcID][seqNumber] = payload
	pb.timeouts[connKey][rpcID] = time.Now()

	logging.Debug("Buffered packet fragment",
		zap.String("connKey", connKey),
		zap.Uint64("rpcID", rpcID),
		zap.Uint16("seqNumber", seqNumber),
		zap.Uint16("totalPackets", totalPackets),
		zap.Int("fragmentsReceived", len(pb.incoming[connKey][rpcID])))

	// Check if we have all fragments
	if len(pb.incoming[connKey][rpcID]) == int(totalPackets) {
		// Reassemble the complete message
		completeData, err := pb.reassemblePacket(packetType, rpcID, totalPackets, pb.incoming[connKey][rpcID])
		if err != nil {
			logging.Error("Failed to reassemble packet", zap.Error(err))
			// Clean up and return original data
			pb.cleanupFragments(connKey, rpcID)
			return &BufferedPacket{
				Data:       data,
				Source:     src,
				Peer:       peer,
				IsRequest:  isRequest,
				PacketType: packetType,
			}, nil
		}

		// Clean up fragment storage
		pb.cleanupFragments(connKey, rpcID)

		logging.Debug("Complete packet reassembled",
			zap.String("connKey", connKey),
			zap.Uint64("rpcID", rpcID),
			zap.Int("totalSize", len(completeData)))

		return &BufferedPacket{
			Data:       completeData,
			Source:     src,
			Peer:       peer,
			IsRequest:  isRequest,
			RPCID:      rpcID,
			PacketType: packetType,
		}, nil
	}

	// Still waiting for more fragments
	return nil, nil
}

// parsePacketHeader extracts packet information from the binary data
func (pb *PacketBuffer) parsePacketHeader(data []byte) (uint8, uint64, uint16, uint16, []byte, error) {
	if len(data) < 29 {
		return 0, 0, 0, 0, nil, fmt.Errorf("data too short for packet header: %d bytes", len(data))
	}

	packetType := uint8(data[0])
	rpcID := binary.LittleEndian.Uint64(data[1:9])
	totalPackets := binary.LittleEndian.Uint16(data[9:11])
	seqNumber := binary.LittleEndian.Uint16(data[11:13])
	// Skip DstIP(4B), DstPort(2B), SrcIP(4B), SrcPort(2B) to get to PayloadLen
	payloadLen := binary.LittleEndian.Uint32(data[25:29])

	// Validate payload length
	if len(data) < 29+int(payloadLen) {
		return 0, 0, 0, 0, nil, fmt.Errorf("data too short for declared payload length")
	}

	payload := data[29 : 29+payloadLen]

	return packetType, rpcID, totalPackets, seqNumber, payload, nil
}

// reassemblePacket reconstructs the complete packet from fragments
func (pb *PacketBuffer) reassemblePacket(packetType uint8, rpcID uint64, totalPackets uint16, fragments map[uint16][]byte) ([]byte, error) {
	// Calculate total payload size
	var totalPayloadSize int
	for i := range int(totalPackets) {
		if fragment, exists := fragments[uint16(i)]; exists {
			totalPayloadSize += len(fragment)
		} else {
			return nil, fmt.Errorf("missing fragment %d for RPC %d", i, rpcID)
		}
	}

	// Create the complete packet buffer
	completeData := make([]byte, 17+totalPayloadSize)

	// Write packet header
	completeData[0] = packetType
	binary.LittleEndian.PutUint64(completeData[1:9], rpcID)
	binary.LittleEndian.PutUint16(completeData[9:11], totalPackets)
	binary.LittleEndian.PutUint16(completeData[11:13], 0) // seqNumber is 0 for complete packet
	binary.LittleEndian.PutUint32(completeData[13:17], uint32(totalPayloadSize))

	// Concatenate fragments in order
	offset := 17
	for i := range int(totalPackets) {
		fragment := fragments[uint16(i)]
		copy(completeData[offset:], fragment)
		offset += len(fragment)
	}

	return completeData, nil
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
