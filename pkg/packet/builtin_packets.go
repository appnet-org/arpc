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
	PacketTypeID  PacketTypeID
	RPCID         uint64  // Unique RPC ID
	TotalPackets  uint16  // Total number of packets in this RPC
	SeqNumber     uint16  // Sequence number of this packet
	MoreFragments bool    // Indicates if there are more fragments with the same sequence number
	FragmentIndex uint8   // Indexes fragments that share the same sequence number (0-255)
	DstIP         [4]byte // Destination IP address (4 bytes)
	DstPort       uint16  // Destination port
	SrcIP         [4]byte // Source IP address (4 bytes)
	SrcPort       uint16  // Source port
	Payload       []byte  // Partial application data
}

// RequestPacket extends DataPacket for request packets
type RequestPacket struct {
	DataPacket
}

// ResponsePacket extends DataPacket for response packets
type ResponsePacket struct {
	DataPacket
}

// ErrorPacket has routing information similar to DataPacket
type ErrorPacket struct {
	PacketTypeID PacketTypeID
	RPCID        uint64  // RPC ID that caused the error
	DstIP        [4]byte // Destination IP address (4 bytes)
	DstPort      uint16  // Destination port
	SrcIP        [4]byte // Source IP address (4 bytes)
	SrcPort      uint16  // Source port
	ErrorMsg     string  // Error message string (must fit in one MTU)
}

// DataPacketCodec implements DataPacket serialization for both Request and Response packets
type DataPacketCodec struct{}

// Serialize encodes a DataPacket into binary format:
// [PacketTypeID(1B)][RPCID(8B)][TotalPackets(2B)][SeqNumber(2B)][MoreFragments(1B)][FragmentIndex(1B)][DstIP(4B)][DstPort(2B)][SrcIP(4B)][SrcPort(2B)][PayloadLen(4B)][Payload]
func (c *DataPacketCodec) Serialize(packet any, pool *common.BufferPool) ([]byte, error) {
	p, ok := packet.(*DataPacket)
	if !ok {
		return nil, errors.New("invalid packet type for DataPacket codec")
	}

	payloadLen := len(p.Payload)
	totalSize := 31 + payloadLen // 1+8+2+2+1+1+4+2+4+2+4 = 31 bytes for header

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

	// Write MoreFragments as a byte (0 or 1)
	if p.MoreFragments {
		buf[13] = 1
	} else {
		buf[13] = 0
	}

	// Write FragmentIndex
	buf[14] = p.FragmentIndex

	// Copy destination IP (4 bytes)
	copy(buf[15:19], p.DstIP[:])

	// Write destination port
	binary.LittleEndian.PutUint16(buf[19:21], p.DstPort)

	// Copy source IP (4 bytes)
	copy(buf[21:25], p.SrcIP[:])

	// Write source port
	binary.LittleEndian.PutUint16(buf[25:27], p.SrcPort)

	// Write payload length
	binary.LittleEndian.PutUint32(buf[27:31], uint32(payloadLen))

	// Copy payload
	copy(buf[31:], p.Payload)

	// Note: We don't return the buffer to the pool here because it's returned to the caller
	// The caller (transport.Send) is responsible for returning it after WriteToUDP
	return buf, nil
}

// Deserialize decodes binary data into a DataPacket
// Format: [PacketTypeID(1B)][RPCID(8B)][TotalPackets(2B)][SeqNumber(2B)][MoreFragments(1B)][FragmentIndex(1B)][DstIP(4B)][DstPort(2B)][SrcIP(4B)][SrcPort(2B)][PayloadLen(4B)][Payload]
func (c *DataPacketCodec) Deserialize(data []byte) (any, error) {
	if len(data) < 31 {
		return nil, errors.New("data too short for DataPacket header")
	}

	p := &DataPacket{}
	p.PacketTypeID = PacketTypeID(data[0])
	p.RPCID = binary.LittleEndian.Uint64(data[1:9])
	p.TotalPackets = binary.LittleEndian.Uint16(data[9:11])
	p.SeqNumber = binary.LittleEndian.Uint16(data[11:13])

	// Read MoreFragments (convert byte to bool)
	p.MoreFragments = data[13] != 0

	// Read FragmentIndex
	p.FragmentIndex = data[14]

	// Copy destination IP (4 bytes)
	copy(p.DstIP[:], data[15:19])

	// Read destination port
	p.DstPort = binary.LittleEndian.Uint16(data[19:21])

	// Copy source IP (4 bytes)
	copy(p.SrcIP[:], data[21:25])

	// Read source port
	p.SrcPort = binary.LittleEndian.Uint16(data[25:27])

	// Read payload length
	payloadLen := binary.LittleEndian.Uint32(data[27:31])

	// Validate length
	if len(data) < 31+int(payloadLen) {
		return nil, errors.New("data too short for declared payload length")
	}

	// Use zero-copy slice for payload - caller must keep buffer alive until payload is no longer needed
	payloadLenInt := int(payloadLen)
	p.Payload = data[31 : 31+payloadLenInt]

	return p, nil
}

// ErrorPacketCodec implements Error packet serialization
type ErrorPacketCodec struct{}

// Serialize encodes an ErrorPacket into binary format:
// [PacketTypeID(1B)][RPCID(8B)][DstIP(4B)][DstPort(2B)][SrcIP(4B)][SrcPort(2B)][MsgLen(4B)][Msg]
func (c *ErrorPacketCodec) Serialize(packet any, pool *common.BufferPool) ([]byte, error) {
	p, ok := packet.(*ErrorPacket)
	if !ok {
		return nil, errors.New("invalid packet type for Error codec")
	}

	msgBytes := []byte(p.ErrorMsg)
	if len(msgBytes) > MaxUDPPayloadSize-29 { // 1+8+4+2+4+2+4 header = 29B
		return nil, errors.New("error message too long, must fit in one MTU")
	}

	totalSize := 29 + len(msgBytes)

	var buf []byte
	if pool != nil {
		buf = pool.GetSize(totalSize)
	} else {
		buf = make([]byte, totalSize)
	}

	// Write fields
	buf[0] = byte(p.PacketTypeID)
	binary.LittleEndian.PutUint64(buf[1:9], p.RPCID)

	// Copy destination IP (4 bytes)
	copy(buf[9:13], p.DstIP[:])

	// Write destination port
	binary.LittleEndian.PutUint16(buf[13:15], p.DstPort)

	// Copy source IP (4 bytes)
	copy(buf[15:19], p.SrcIP[:])

	// Write source port
	binary.LittleEndian.PutUint16(buf[19:21], p.SrcPort)

	// Write message length
	binary.LittleEndian.PutUint32(buf[21:25], uint32(len(msgBytes)))

	// Copy message
	copy(buf[25:], msgBytes)

	// Note: We don't return the buffer to the pool here because it's returned to the caller
	// The caller (transport.Send) is responsible for returning it after WriteToUDP
	return buf, nil
}

// Deserialize decodes binary data into an ErrorPacket
// Format: [PacketTypeID(1B)][RPCID(8B)][DstIP(4B)][DstPort(2B)][SrcIP(4B)][SrcPort(2B)][MsgLen(4B)][Msg]
func (c *ErrorPacketCodec) Deserialize(data []byte) (any, error) {
	if len(data) < 29 {
		return nil, errors.New("data too short for ErrorPacket header")
	}

	pkt := &ErrorPacket{}
	pkt.PacketTypeID = PacketTypeID(data[0])
	pkt.RPCID = binary.LittleEndian.Uint64(data[1:9])

	// Copy destination IP (4 bytes)
	copy(pkt.DstIP[:], data[9:13])

	// Read destination port
	pkt.DstPort = binary.LittleEndian.Uint16(data[13:15])

	// Copy source IP (4 bytes)
	copy(pkt.SrcIP[:], data[15:19])

	// Read source port
	pkt.SrcPort = binary.LittleEndian.Uint16(data[19:21])

	// Read message length
	msgLen := binary.LittleEndian.Uint32(data[21:25])

	if len(data) < 29+int(msgLen) {
		return nil, errors.New("data too short for declared error message length")
	}

	pkt.ErrorMsg = string(data[25 : 25+msgLen])
	return pkt, nil
}
