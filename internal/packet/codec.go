package packet

import (
	"errors"
	"strconv"
)

const MaxUDPPayloadSize = 1400 // Adjust based on MTU considerations

// PacketCodec is the base interface that all codecs must implement
type PacketCodec interface {
	// Serialize converts a packet to its binary representation
	Serialize(packet any) ([]byte, error)

	// Deserialize converts binary data back to a packet
	Deserialize(data []byte) (any, error)
}

// Generic packet serialization/deserialization functions
func SerializePacket(packet any, packetType PacketType) ([]byte, error) {
	registry := DefaultRegistry
	codec, exists := registry.GetCodec(packetType.ID)
	if !exists {
		return nil, errors.New("codec not found for packet type " + packetType.Name)
	}

	return codec.Serialize(packet)
}

// DeserializePacketAny deserializes a packet by first reading its type from the data
// and then using the appropriate codec
func DeserializePacketAny(data []byte) (any, PacketType, error) {
	if len(data) < 1 {
		return nil, PacketTypeUnknown, errors.New("data too short to read packet type")
	}

	// Packet type (uint8) is the first byte of the data
	packetType := uint8(data[0])

	// Get the appropriate codec for this packet type
	registry := DefaultRegistry
	codec, exists := registry.GetCodec(PacketTypeID(packetType))
	if !exists {
		return nil, PacketTypeUnknown, errors.New("codec not found for packet type " + strconv.Itoa(int(packetType)))
	}

	// Deserialize using the codec
	packet, err := codec.Deserialize(data)
	if err != nil {
		return nil, PacketTypeUnknown, err
	}

	return packet, PacketType{ID: PacketTypeID(packetType), Name: registry.types[PacketTypeID(packetType)].Name}, nil
}
