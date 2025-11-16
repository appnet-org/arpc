package flowcontrol

import (
	"encoding/binary"
	"errors"

	"github.com/appnet-org/arpc/pkg/packet"
)

const FCFeedbackPacketName = "FCFeedback"

// FCFeedbackPacket provides flow control window updates
// This packet is sent when the receive window needs to be updated (threshold-based),
// allowing the sender to continue sending data without being flow-control blocked.
type FCFeedbackPacket struct {
	PacketTypeID packet.PacketTypeID // 1 byte
	SendWindow   uint64              // 8 bytes - new send window offset
}

// FCFeedbackCodec implements PacketCodec for FCFeedback packets
type FCFeedbackCodec struct{}

// Serialize encodes a FCFeedbackPacket into binary format:
// [PacketTypeID(1B)][SendWindow(8B)]
// Total: 9 bytes fixed size
func (c *FCFeedbackCodec) Serialize(pkt any) ([]byte, error) {
	p, ok := pkt.(*FCFeedbackPacket)
	if !ok {
		return nil, errors.New("invalid packet type for FCFeedback codec")
	}

	buf := make([]byte, 9)
	offset := 0

	// PacketTypeID
	buf[offset] = byte(p.PacketTypeID)
	offset++

	// SendWindow
	binary.LittleEndian.PutUint64(buf[offset:offset+8], p.SendWindow)
	offset += 8

	return buf, nil
}

// Deserialize decodes binary data into a FCFeedbackPacket
func (c *FCFeedbackCodec) Deserialize(data []byte) (any, error) {
	if len(data) < 9 {
		return nil, errors.New("data too short for FCFeedbackPacket (need 9 bytes)")
	}

	pkt := &FCFeedbackPacket{}
	offset := 0

	// PacketTypeID
	pkt.PacketTypeID = packet.PacketTypeID(data[offset])
	offset++

	// SendWindow
	pkt.SendWindow = binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8

	return pkt, nil
}

var _ packet.PacketCodec = (*FCFeedbackCodec)(nil)
