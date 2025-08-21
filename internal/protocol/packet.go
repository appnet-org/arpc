package protocol

import (
	"errors"
)

// PacketType is the type of packet. 0 is reserved for errors.
type PacketType uint8

const (
	PacketTypeUnknown  PacketType = 0
	PacketTypeRequest  PacketType = 1
	PacketTypeResponse PacketType = 2
	PacketTypeError    PacketType = 3
)

// DataPacket represents the common structure for Request and Response packets
type DataPacket struct {
	PacketType   PacketType
	RPCID        uint64 // Unique RPC ID
	TotalPackets uint16 // Total number of packets in this RPC
	SeqNumber    uint16 // Sequence number of this packet
	Payload      []byte // Partial application data
}

// RequestPacket extends DataPacket for request packets
type RequestPacket struct {
	DataPacket
	// Additional user-defined fields can be added here if needed
}

// ResponsePacket extends DataPacket for response packets
type ResponsePacket struct {
	DataPacket
	// Additional user-defined fields can be added here if needed
}

// AckPacket has exactly two fields as specified
type AckPacket struct {
	RPCID      uint64 // RPC ID being acknowledged
	BytesAcked uint32 // Number of bytes acknowledged
}

// ErrorPacket has exactly two fields as specified
type ErrorPacket struct {
	RPCID    uint64 // RPC ID that caused the error
	ErrorMsg string // Error message string
}

// PacketRegistry allows registering custom packet types and their codecs
type PacketRegistry struct {
	types  map[uint8]PacketType
	codecs map[PacketType]PacketCodec
}

// PacketCodec defines how to serialize/deserialize a specific packet type
type PacketCodec interface {
	Serialize(packet any) ([]byte, error)
	Deserialize(data []byte) (any, error)
	NewPacket() any
}

// NewPacketRegistry creates a new packet registry
func NewPacketRegistry() *PacketRegistry {
	return &PacketRegistry{
		types:  make(map[uint8]PacketType),
		codecs: make(map[PacketType]PacketCodec),
	}
}

// RegisterPacketType registers a custom packet type with its codec
func (pr *PacketRegistry) RegisterPacketType(pt PacketType, codec PacketCodec) error {
	if pt == 0 {
		return ErrInvalidPacketTypeID
	}
	if _, exists := pr.types[uint8(pt)]; exists {
		return ErrPacketTypeAlreadyExists
	}
	pr.types[uint8(pt)] = pt
	pr.codecs[pt] = codec
	return nil
}

// GetPacketType retrieves a packet type by ID
func (pr *PacketRegistry) GetPacketType(id uint8) (PacketType, bool) {
	pt, exists := pr.types[id]
	return pt, exists
}

// GetCodec retrieves the codec for a packet type
func (pr *PacketRegistry) GetCodec(pt PacketType) (PacketCodec, bool) {
	codec, exists := pr.codecs[pt]
	return codec, exists
}

// DefaultRegistry is the default packet registry with predefined types
var DefaultRegistry = func() *PacketRegistry {
	pr := NewPacketRegistry()

	// Register default packet types with their codecs
	pr.RegisterPacketType(PacketTypeRequest, &DataPacketCodec{})
	pr.RegisterPacketType(PacketTypeResponse, &DataPacketCodec{})
	pr.RegisterPacketType(PacketTypeError, &ErrorPacketCodec{})

	return pr
}()

// Errors
var (
	ErrInvalidPacketTypeID     = errors.New("invalid packet type ID: 0 is reserved")
	ErrPacketTypeAlreadyExists = errors.New("packet type with this ID already exists")
	ErrCodecNotFound           = errors.New("codec not found for packet type")
)
