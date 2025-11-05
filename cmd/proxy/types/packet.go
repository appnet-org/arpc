package types

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
	IsFull       bool   // true for full messages, false for partial messages
	SeqNumber    uint16 // sequence number (0 for full messages)
	TotalPackets uint16 // total number of packets (0 for full messages)
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

