package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
)

const MaxUDPPayloadSize = 1400 // Adjust based on MTU considerations

// DataPacketCodec implements DataPacket serialization for both Request and Response packets
type DataPacketCodec struct{}

func (c *DataPacketCodec) Serialize(packet any) ([]byte, error) {
	p, ok := packet.(*DataPacket)
	if !ok {
		return nil, errors.New("invalid packet type for DataPacket codec")
	}

	buf := new(bytes.Buffer)

	// Write standard fields
	if err := binary.Write(buf, binary.LittleEndian, p.PacketType); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, p.RPCID); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, p.TotalPackets); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, p.SeqNumber); err != nil {
		return nil, err
	}

	// Write payload length and data
	if err := binary.Write(buf, binary.LittleEndian, uint32(len(p.Payload))); err != nil {
		return nil, err
	}
	if _, err := buf.Write(p.Payload); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (c *DataPacketCodec) Deserialize(data []byte) (any, error) {
	buf := bytes.NewReader(data)

	// Read into the DataPacket fields
	p := DataPacket{}

	// Read standard fields
	if err := binary.Read(buf, binary.LittleEndian, &p.PacketType); err != nil {
		return nil, err
	}
	if err := binary.Read(buf, binary.LittleEndian, &p.RPCID); err != nil {
		return nil, err
	}
	if err := binary.Read(buf, binary.LittleEndian, &p.TotalPackets); err != nil {
		return nil, err
	}
	if err := binary.Read(buf, binary.LittleEndian, &p.SeqNumber); err != nil {
		return nil, err
	}

	// Read payload length and data
	var payloadLen uint32
	if err := binary.Read(buf, binary.LittleEndian, &payloadLen); err != nil {
		return nil, err
	}

	p.Payload = make([]byte, payloadLen)
	if _, err := buf.Read(p.Payload); err != nil {
		return nil, err
	}

	return p, nil
}

func (c *DataPacketCodec) NewPacket() any {
	// This method can't determine which type to create without context
	// It's better to use the specific codecs for this purpose
	panic("DataPacketCodec.NewPacket() should not be called directly")
}

// ErrorPacketCodec implements Error packet serialization
type ErrorPacketCodec struct{}

func (c *ErrorPacketCodec) Serialize(packet any) ([]byte, error) {
	p, ok := packet.(*ErrorPacket)
	if !ok {
		return nil, errors.New("invalid packet type for Error codec")
	}

	buf := new(bytes.Buffer)

	// Write RPC ID
	if err := binary.Write(buf, binary.LittleEndian, p.RPCID); err != nil {
		return nil, err
	}

	// Write error message length and string
	msgBytes := []byte(p.ErrorMsg)
	if err := binary.Write(buf, binary.LittleEndian, uint32(len(msgBytes))); err != nil {
		return nil, err
	}
	if _, err := buf.Write(msgBytes); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (c *ErrorPacketCodec) Deserialize(data []byte) (any, error) {
	buf := bytes.NewReader(data)
	pkt := &ErrorPacket{}

	// Read RPC ID
	if err := binary.Read(buf, binary.LittleEndian, &pkt.RPCID); err != nil {
		return nil, err
	}

	// Read error message length and string
	var msgLen uint32
	if err := binary.Read(buf, binary.LittleEndian, &msgLen); err != nil {
		return nil, err
	}

	msgBytes := make([]byte, msgLen)
	if _, err := buf.Read(msgBytes); err != nil {
		return nil, err
	}
	pkt.ErrorMsg = string(msgBytes)

	return pkt, nil
}

func (c *ErrorPacketCodec) NewPacket() any {
	return &ErrorPacket{}
}

// Generic packet serialization/deserialization functions
func SerializePacket(packet any, packetType PacketType) ([]byte, error) {
	registry := DefaultRegistry
	codec, exists := registry.GetCodec(packetType)
	if !exists {
		return nil, ErrCodecNotFound
	}

	return codec.Serialize(packet)
}

// DeserializePacketAny deserializes a packet by first reading its type from the data
// and then using the appropriate codec
func DeserializePacketAny(data []byte) (any, PacketType, error) {
	if len(data) < 1 {
		return nil, 0, errors.New("data too short to read packet type")
	}

	// Packet type (uint8) is the first byte of the data
	packetType := PacketType(data[0])

	// Get the appropriate codec for this packet type
	registry := DefaultRegistry
	codec, exists := registry.GetCodec(packetType)
	if !exists {
		return nil, packetType, ErrCodecNotFound
	}

	// Deserialize using the codec
	packet, err := codec.Deserialize(data)
	if err != nil {
		return nil, packetType, err
	}

	return packet, packetType, nil
}
