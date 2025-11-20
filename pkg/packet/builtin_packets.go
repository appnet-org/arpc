// This file defines the builtin packets (Request, Response, Error) and their corresponding
// serialization/deserialization codecs.
package packet

import (
	"encoding/binary"
	"errors"

	"github.com/appnet-org/arpc/pkg/common"
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
	RPCID        uint64  // Unique RPC ID
	TotalPackets uint16  // Total number of packets in this RPC
	SeqNumber    uint16  // Sequence number of this packet
	DstIP        [4]byte // Destination IP address (4 bytes)
	DstPort      uint16  // Destination port
	SrcIP        [4]byte // Source IP address (4 bytes)
	SrcPort      uint16  // Source port
	Payload      []byte  // Partial application data
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
// [PacketTypeID(1B)][RPCID(8B)][TotalPackets(2B)][SeqNumber(2B)][DstIP(4B)][DstPort(2B)][SrcIP(4B)][SrcPort(2B)][PayloadLen(4B)][Payload]
func (c *DataPacketCodec) Serialize(packet any, pool *common.BufferPool) ([]byte, error) {
	p, ok := packet.(*DataPacket)
	if !ok {
		return nil, errors.New("invalid packet type for DataPacket codec")
	}

	payloadLen := len(p.Payload)
	totalSize := 29 + payloadLen // 1+8+2+2+4+2+4+2+4 = 29 bytes for header

	var buf []byte
	if pool != nil {
		buf = pool.GetSize(totalSize)
	} else {
		buf = make([]byte, totalSize)
	}

	// Write fields directly into the buffer
	buf[0] = byte(p.PacketTypeID)
	binary.LittleEndian.PutUint64(buf[1:9], p.RPCID)
	binary.LittleEndian.PutUint16(buf[9:11], p.TotalPackets)
	binary.LittleEndian.PutUint16(buf[11:13], p.SeqNumber)

	// Copy destination IP (4 bytes)
	copy(buf[13:17], p.DstIP[:])

	// Write destination port
	binary.LittleEndian.PutUint16(buf[17:19], p.DstPort)

	// Copy source IP (4 bytes)
	copy(buf[19:23], p.SrcIP[:])

	// Write source port
	binary.LittleEndian.PutUint16(buf[23:25], p.SrcPort)

	// Write payload length
	binary.LittleEndian.PutUint32(buf[25:29], uint32(payloadLen))

	// Copy payload
	copy(buf[29:], p.Payload)

	// Note: We don't return the buffer to the pool here because it's returned to the caller
	// The caller (transport.Send) is responsible for returning it after WriteToUDP
	return buf, nil
}

// Deserialize decodes binary data into a DataPacket
// Format: [PacketTypeID(1B)][RPCID(8B)][TotalPackets(2B)][SeqNumber(2B)][DstIP(4B)][DstPort(2B)][SrcIP(4B)][SrcPort(2B)][PayloadLen(4B)][Payload]
func (c *DataPacketCodec) Deserialize(data []byte) (any, error) {
	if len(data) < 29 {
		return nil, errors.New("data too short for DataPacket header")
	}

	p := &DataPacket{}
	p.PacketTypeID = PacketTypeID(data[0])
	p.RPCID = binary.LittleEndian.Uint64(data[1:9])
	p.TotalPackets = binary.LittleEndian.Uint16(data[9:11])
	p.SeqNumber = binary.LittleEndian.Uint16(data[11:13])

	// Copy destination IP (4 bytes)
	copy(p.DstIP[:], data[13:17])

	// Read destination port
	p.DstPort = binary.LittleEndian.Uint16(data[17:19])

	// Copy source IP (4 bytes)
	copy(p.SrcIP[:], data[19:23])

	// Read source port
	p.SrcPort = binary.LittleEndian.Uint16(data[23:25])

	// Read payload length
	payloadLen := binary.LittleEndian.Uint32(data[25:29])

	// Validate length
	if len(data) < 29+int(payloadLen) {
		return nil, errors.New("data too short for declared payload length")
	}

	// Use zero-copy slice for payload - caller must keep buffer alive until payload is no longer needed
	payloadLenInt := int(payloadLen)
	p.Payload = data[29 : 29+payloadLenInt]

	return p, nil
}

// ErrorPacketCodec implements Error packet serialization
type ErrorPacketCodec struct{}

// Serialize encodes an ErrorPacket into binary format:
// [PacketTypeID(1B)][RPCID(8B)][MsgLen(4B)][Msg]
func (c *ErrorPacketCodec) Serialize(packet any, pool *common.BufferPool) ([]byte, error) {
	p, ok := packet.(*ErrorPacket)
	if !ok {
		return nil, errors.New("invalid packet type for Error codec")
	}

	msgBytes := []byte(p.ErrorMsg)
	if len(msgBytes) > MaxUDPPayloadSize-13 { // 1+8+4 header = 13B
		return nil, errors.New("error message too long, must fit in one MTU")
	}

	totalSize := 13 + len(msgBytes)

	var buf []byte
	if pool != nil {
		buf = pool.GetSize(totalSize)
	} else {
		buf = make([]byte, totalSize)
	}

	// Write fields
	buf[0] = byte(p.PacketTypeID)
	binary.LittleEndian.PutUint64(buf[1:9], p.RPCID)
	binary.LittleEndian.PutUint32(buf[9:13], uint32(len(msgBytes)))
	copy(buf[13:], msgBytes)

	// Note: We don't return the buffer to the pool here because it's returned to the caller
	// The caller (transport.Send) is responsible for returning it after WriteToUDP
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
