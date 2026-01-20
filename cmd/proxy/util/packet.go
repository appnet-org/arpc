package util

import "net"

// BufferedPacket represents a complete packet ready for processing
type BufferedPacket struct {
	Payload    []byte
	Source     *net.UDPAddr
	Peer       *net.UDPAddr
	RPCID      uint64
	PacketType PacketType
	// Routing information extracted from the packet
	DstIP   [4]byte
	DstPort uint16
	SrcIP   [4]byte
	SrcPort uint16
	// Fragmentation information
	IsFull         bool   // true for full messages, false for partial messages
	SeqNumber      int16  // sequence number (-1 for full messages or public segment)
	TotalPackets   uint16 // total number of packets (0 for full messages)
	LastUsedSeqNum uint16 // last sequence number used for public segment (only set when buffered)
}

// PacketType represents the type of packet
type PacketType uint8

const (
	PacketTypeRequest  PacketType = 1
	PacketTypeResponse PacketType = 2
	PacketTypeError    PacketType = 3
	PacketTypeOther    PacketType = 4
	PacketTypeUnknown  PacketType = 0 // For compatibility
)

// String returns the string representation of PacketType
func (p PacketType) String() string {
	switch p {
	case PacketTypeRequest:
		return "REQUEST"
	case PacketTypeResponse:
		return "RESPONSE"
	case PacketTypeError:
		return "ERROR"
	case PacketTypeOther:
		return "OTHER"
	}
	return "UNKNOWN"
}

// PacketVerdict defines how the proxy should handle packets for an element.
type PacketVerdict int

const (
	// PacketVerdictUnknown indicates no verdict exists yet (zero value).
	PacketVerdictUnknown PacketVerdict = iota

	// PacketVerdictPass allows the packet to continue processing (XDP_PASS equivalent).
	PacketVerdictPass

	// PacketVerdictDrop drops the packet (XDP_DROP equivalent).
	PacketVerdictDrop
)

// String returns the string representation of PacketVerdict
func (p PacketVerdict) String() string {
	switch p {
	case PacketVerdictUnknown:
		return "packet_verdict_unknown"
	case PacketVerdictPass:
		return "packet_verdict_pass"
	case PacketVerdictDrop:
		return "packet_verdict_drop"
	}
	return "packet_verdict_unknown"
}

// GetRPCID returns the RPC ID of the buffered packet.
func (bp *BufferedPacket) GetRPCID() uint64 {
	return bp.RPCID
}
