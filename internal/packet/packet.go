package packet

import (
	"errors"
)

// PacketType is the type of packet. 0 is reserved for errors.
type PacketTypeID uint8

type PacketType struct {
	ID   PacketTypeID
	Name string
}

// PacketRegistry allows registering custom packet types and their codecs
type PacketRegistry struct {
	types  map[PacketTypeID]PacketType  // map of packet type ID to packet type
	codecs map[PacketTypeID]PacketCodec // map of packet type ID to codec
	nextID PacketTypeID                 // Track the next available packet type ID
}

// NewPacketRegistry creates a new packet registry
func NewPacketRegistry() *PacketRegistry {
	return &PacketRegistry{
		types:  make(map[PacketTypeID]PacketType),
		codecs: make(map[PacketTypeID]PacketCodec),
		nextID: 1, // Start from 1 since 0 are reserved for invalid types
	}
}

// RegisterPacketType registers a custom packet type with its codec and returns the assigned packet type ID
func (pr *PacketRegistry) RegisterPacketType(packetType string, codec PacketCodec) (PacketType, error) {
	if pr.nextID == 255 {
		return PacketTypeUnknown, errors.New("no more available packet type IDs")
	}

	pt := PacketType{
		ID:   pr.nextID,
		Name: packetType,
	}
	pr.types[pt.ID] = pt
	pr.codecs[pt.ID] = codec
	pr.nextID++ // Increment for next registration

	return pt, nil
}
func (pr *PacketRegistry) RegisterPacketTypeWithID(packetType string, id PacketTypeID, codec PacketCodec) (PacketType, error) {
	if _, exists := pr.types[id]; exists {
		return PacketTypeUnknown, errors.New("packet type with this ID already exists")
	}

	pt := PacketType{
		ID:   id,
		Name: packetType,
	}
	pr.types[pt.ID] = pt
	pr.codecs[pt.ID] = codec

	return pt, nil
}

// GetPacketType retrieves a packet type by ID
func (pr *PacketRegistry) GetPacketType(id PacketTypeID) (PacketType, bool) {
	pt, exists := pr.types[id]
	return pt, exists
}

// GetCodec retrieves the codec for a packet type
func (pr *PacketRegistry) GetCodec(packet_id PacketTypeID) (PacketCodec, bool) {
	codec, exists := pr.codecs[packet_id]
	return codec, exists
}

// DefaultRegistry is the default packet registry with predefined types
var DefaultRegistry = func() *PacketRegistry {
	pr := NewPacketRegistry()

	// Register default packet types with their codecs
	pr.RegisterPacketTypeWithID(PacketTypeRequest.Name, PacketTypeRequest.ID, &DataPacketCodec{})
	pr.RegisterPacketTypeWithID(PacketTypeResponse.Name, PacketTypeResponse.ID, &DataPacketCodec{})
	pr.RegisterPacketTypeWithID(PacketTypeError.Name, PacketTypeError.ID, &ErrorPacketCodec{})
	pr.RegisterPacketTypeWithID(PacketTypeUnknown.Name, PacketTypeUnknown.ID, &ErrorPacketCodec{})

	return pr
}()

// Errors
var (
	ErrInvalidPacketTypeID     = errors.New("invalid packet type ID: 0 is reserved")
	ErrPacketTypeAlreadyExists = errors.New("packet type with this ID already exists")
)
