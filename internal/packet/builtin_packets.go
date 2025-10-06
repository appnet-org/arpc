// This file defines the builtin packets (Request, Response, Error) and their corresponding
// serialization/deserialization codecs.
package packet

import (
	"encoding/binary"
	"errors"
)

// Builtin packet types
var (
	PacketTypeUnknown  = PacketType{TypeID: 0, Name: "Unknown"}
	PacketTypeRequest  = PacketType{TypeID: 1, Name: "Request"}
	PacketTypeResponse = PacketType{TypeID: 2, Name: "Response"}
	PacketTypeError    = PacketType{TypeID: 3, Name: "Error"}
)

// DataPacket represents the common structure for Request and Response packets
type DataPacket struct {
	PacketTypeID PacketTypeID
	RPCID        uint64 // Unique RPC ID
	TotalPackets uint16 // Total number of packets in this RPC
	SeqNumber    uint16 // Sequence number of this packet
	Payload      []byte // Partial application data
}

// RequestPacket extends DataPacket for request packets
type RequestPacket struct {
	DataPacket
}

// ResponsePacket extends DataPacket for response packets
type ResponsePacket struct {
	DataPacket
}

// ErrorPacket has exactly two fields as specified
type ErrorPacket struct {
	PacketTypeID PacketTypeID
	RPCID        uint64 // RPC ID that caused the error
	ErrorMsg     string // Error message string (must fit in on one MTU)
}

// DataPacketCodec implements DataPacket serialization for both Request and Response packets
type DataPacketCodec struct{}

// Serialize encodes a DataPacket into binary format:
// [PacketTypeID(1B)][RPCID(8B)][TotalPackets(2B)][SeqNumber(2B)][PayloadLen(4B)][Payload]
func (c *DataPacketCodec) Serialize(packet any) ([]byte, error) {
	p, ok := packet.(*DataPacket)
	if !ok {
		return nil, errors.New("invalid packet type for DataPacket codec")
	}

	payloadLen := len(p.Payload)
	totalSize := 17 + payloadLen
	buf := make([]byte, totalSize)

	// Write fields directly into the buffer
	buf[0] = byte(p.PacketTypeID)
	binary.LittleEndian.PutUint64(buf[1:9], p.RPCID)
	binary.LittleEndian.PutUint16(buf[9:11], p.TotalPackets)
	binary.LittleEndian.PutUint16(buf[11:13], p.SeqNumber)
	binary.LittleEndian.PutUint32(buf[13:17], uint32(payloadLen))

	// Copy payload
	copy(buf[17:], p.Payload)

	return buf, nil
}

// Deserialize decodes binary data into a DataPacket
// Format: [PacketTypeID(1B)][RPCID(8B)][TotalPackets(2B)][SeqNumber(2B)][PayloadLen(4B)][Payload]
func (c *DataPacketCodec) Deserialize(data []byte) (any, error) {
	if len(data) < 17 {
		return nil, errors.New("data too short for DataPacket header")
	}

	p := &DataPacket{}
	p.PacketTypeID = PacketTypeID(data[0])
	p.RPCID = binary.LittleEndian.Uint64(data[1:9])
	p.TotalPackets = binary.LittleEndian.Uint16(data[9:11])
	p.SeqNumber = binary.LittleEndian.Uint16(data[11:13])
	payloadLen := binary.LittleEndian.Uint32(data[13:17])

	// Validate length
	if len(data) < 17+int(payloadLen) {
		return nil, errors.New("data too short for declared payload length")
	}

	// Payload — copy if you need ownership, or slice directly for zero-copy
	p.Payload = data[17 : 17+payloadLen]

	return p, nil
}

// ErrorPacketCodec implements Error packet serialization
type ErrorPacketCodec struct{}

// Serialize encodes an ErrorPacket into binary format:
// [PacketTypeID(1B)][RPCID(8B)][MsgLen(4B)][Msg]
func (c *ErrorPacketCodec) Serialize(packet any) ([]byte, error) {
	p, ok := packet.(*ErrorPacket)
	if !ok {
		return nil, errors.New("invalid packet type for Error codec")
	}

	msgBytes := []byte(p.ErrorMsg)
	if len(msgBytes) > MaxUDPPayloadSize-13 { // 1+8+4 header = 13B
		return nil, errors.New("error message too long, must fit in one MTU")
	}

	totalSize := 13 + len(msgBytes)
	buf := make([]byte, totalSize)

	// Write fields
	buf[0] = byte(p.PacketTypeID)
	binary.LittleEndian.PutUint64(buf[1:9], p.RPCID)
	binary.LittleEndian.PutUint32(buf[9:13], uint32(len(msgBytes)))
	copy(buf[13:], msgBytes)

	return buf, nil
}

// Deserialize decodes binary data into an ErrorPacket
func (c *ErrorPacketCodec) Deserialize(data []byte) (any, error) {
	if len(data) < 13 {
		return nil, errors.New("data too short for ErrorPacket header")
	}

	pkt := &ErrorPacket{}
	pkt.PacketTypeID = PacketTypeID(data[0])
	pkt.RPCID = binary.LittleEndian.Uint64(data[1:9])
	msgLen := binary.LittleEndian.Uint32(data[9:13])

	if len(data) < 13+int(msgLen) {
		return nil, errors.New("data too short for declared error message length")
	}

	pkt.ErrorMsg = string(data[13 : 13+msgLen])
	return pkt, nil
}
