package reliable

import (
	"encoding/binary"
	"errors"

	"github.com/appnet-org/arpc/pkg/common"
	"github.com/appnet-org/arpc/pkg/packet"
)

const AckPacketName = "Acknowledgement"

// ACKPacket represents an acknowledgment packet
type ACKPacket struct {
	PacketTypeID packet.PacketTypeID
	RPCID        uint64 // RPC ID being acknowledged
	Kind         uint8  // Kind of packet (0=request, 1=response, 2=error)
	Status       uint8  // Status code (0=success, 1=error, etc.)
	Timestamp    int64  // Timestamp when ACK was generated
	Message      string // Optional message
}

// ACKPacketCodec implements PacketCodec for ACK packets
type ACKPacketCodec struct{}

// Serialize encodes an ACKPacket into binary format:
// [PacketTypeID(1B)][RPCID(8B)][Kind(1B)][Status(1B)][Timestamp(8B)][MsgLen(4B)][Msg]
func (c *ACKPacketCodec) Serialize(packet any, pool *common.BufferPool) ([]byte, error) {
	p, ok := packet.(*ACKPacket)
	if !ok {
		return nil, errors.New("invalid packet type for ACK codec")
	}

	msgBytes := []byte(p.Message)
	totalSize := 23 + len(msgBytes) // header + message

	var buf []byte
	if pool != nil {
		buf = pool.GetSize(totalSize)
	} else {
		buf = make([]byte, totalSize)
	}

	// PacketTypeID
	buf[0] = byte(p.PacketTypeID)

	// RPCID
	binary.LittleEndian.PutUint64(buf[1:9], p.RPCID)

	// Kind
	buf[9] = p.Kind

	// Status
	buf[10] = p.Status

	// Timestamp
	binary.LittleEndian.PutUint64(buf[11:19], uint64(p.Timestamp))

	// Message length
	binary.LittleEndian.PutUint32(buf[19:23], uint32(len(msgBytes)))

	// Message
	copy(buf[23:], msgBytes)

	return buf, nil
}

// Deserialize decodes binary data into an ACKPacket
func (c *ACKPacketCodec) Deserialize(data []byte) (any, error) {
	if len(data) < 23 {
		return nil, errors.New("data too short for ACKPacket header")
	}

	pkt := &ACKPacket{}
	pkt.PacketTypeID = packet.PacketTypeID(data[0])
	pkt.RPCID = binary.LittleEndian.Uint64(data[1:9])
	pkt.Kind = data[9]
	pkt.Status = data[10]
	pkt.Timestamp = int64(binary.LittleEndian.Uint64(data[11:19]))

	msgLen := binary.LittleEndian.Uint32(data[19:23])
	if len(data) < 23+int(msgLen) {
		return nil, errors.New("data too short for declared message length")
	}

	pkt.Message = string(data[23 : 23+msgLen])
	return pkt, nil
}

var _ packet.PacketCodec = (*ACKPacketCodec)(nil)
