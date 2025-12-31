package main

import (
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

// ProcessPacket processes a packet fragment. If buffering is enabled, it buffers fragments
// and returns a complete packet when all fragments are received. If buffering is disabled
// or the packet is already complete, it returns immediately. Returns nil, PacketVerdictUnknown, nil if still
// waiting for more fragments. Returns PacketVerdictUnknown if no verdict exists for this RPC ID.
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
		// Verdict exists, forward the packet immediately without buffering
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
			SeqNumber:    seqNumber,
			TotalPackets: dataPacket.TotalPackets,
		}, verdict, nil
	}

	// Check if this is the first packet of the RPC
	if dataPacket.SeqNumber == 0 {
		// Check if offset_private < MTU?
		// if not, add to buffer
	} else {
		// Add fragment to buffer
		pb.addFragmentToBuffer(src, dataPacket)
	}
	// Check if this is the last packet of the RPC

	// Still waiting for more fragments
	return nil, util.PacketVerdictUnknown, nil
}

// addFragmentToBuffer adds a packet fragment to the buffer for reassembly
func (pb *PacketBuffer) addFragmentToBuffer(src *net.UDPAddr, dataPacket *packet.DataPacket) {
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

	// TODO: check if we have enough data
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

// FragmentPacketForForward fragments a complete packet if needed
// Returns a slice of fragmented packets to send
func (pb *PacketBuffer) FragmentPacketForForward(bufferedPacket *util.BufferedPacket) ([]FragmentedPacket, error) {
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
