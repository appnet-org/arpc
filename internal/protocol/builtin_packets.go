// This file defines the builtin packets (Request, Response, Error) and their corresponding
// serialization/deserialization codecs.
package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// Builtin packet types
var (
	PacketTypeUnknown  = PacketType{ID: 0, Name: "Unknown"}
	PacketTypeRequest  = PacketType{ID: 1, Name: "Request"}
	PacketTypeResponse = PacketType{ID: 2, Name: "Response"}
	PacketTypeError    = PacketType{ID: 3, Name: "Error"}
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

func (c *DataPacketCodec) Serialize(packet any) ([]byte, error) {
	p, ok := packet.(*DataPacket)
	if !ok {
		return nil, errors.New("invalid packet type for DataPacket codec")
	}

	buf := new(bytes.Buffer)

	// Write standard fields
	if err := binary.Write(buf, binary.LittleEndian, p.PacketTypeID); err != nil {
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
	if err := binary.Read(buf, binary.LittleEndian, &p.PacketTypeID); err != nil {
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

	return &p, nil
}

// ErrorPacketCodec implements Error packet serialization
type ErrorPacketCodec struct{}

func (c *ErrorPacketCodec) Serialize(packet any) ([]byte, error) {
	p, ok := packet.(*ErrorPacket)
	if !ok {
		return nil, errors.New("invalid packet type for Error codec")
	}

	buf := new(bytes.Buffer)

	// Write error packet type ID
	if err := binary.Write(buf, binary.LittleEndian, PacketTypeError.ID); err != nil {
		return nil, err
	}

	// Write RPC ID
	if err := binary.Write(buf, binary.LittleEndian, p.RPCID); err != nil {
		return nil, err
	}

	// Error message must fit in one MTU
	if len(p.ErrorMsg) > MaxUDPPayloadSize-8 {
		return nil, errors.New("error message too long, must fit in one MTU")
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

	// Read error packet type ID
	if err := binary.Read(buf, binary.LittleEndian, &pkt.PacketTypeID); err != nil {
		return nil, err
	}

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
