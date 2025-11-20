package congestion

import (
	"encoding/binary"
	"errors"

	"github.com/appnet-org/arpc/pkg/common"
	"github.com/appnet-org/arpc/pkg/packet"
)

const CCFeedbackPacketName = "CCFeedback"

// CCFeedbackPacket provides aggregated congestion control feedback
// This packet is sent every N packets or after timeout to provide CC information
// without requiring per-packet ACKs, allowing decoupling of reliability from congestion control.
type CCFeedbackPacket struct {
	PacketTypeID packet.PacketTypeID // 1 byte
	AckedCount   uint32              // 4 bytes - number of packets acked
	AckedBytes   uint64              // 8 bytes - total bytes acked
	PacketIDs    []uint64            // Variable length - array of packet IDs that were received
}

// CCFeedbackCodec implements PacketCodec for CCFeedback packets
type CCFeedbackCodec struct{}

// Serialize encodes a CCFeedbackPacket into binary format:
// [PacketTypeID(1B)][AckedCount(4B)][AckedBytes(8B)][PacketIDCount(4B)][PacketIDs...]
// Header: 1 + 4 + 8 + 4 = 17 bytes
// Plus 8 bytes per packet ID
func (c *CCFeedbackCodec) Serialize(pkt any, pool *common.BufferPool) ([]byte, error) {
	p, ok := pkt.(*CCFeedbackPacket)
	if !ok {
		return nil, errors.New("invalid packet type for CCFeedback codec")
	}

	// Header size: 1 + 4 + 8 + 4 = 17 bytes
	// Plus 8 bytes per packet ID
	totalSize := 17 + len(p.PacketIDs)*8

	var buf []byte
	if pool != nil {
		buf = pool.GetSize(totalSize)
	} else {
		buf = make([]byte, totalSize)
	}
	offset := 0

	// PacketTypeID
	buf[offset] = byte(p.PacketTypeID)
	offset++

	// AckedCount
	binary.LittleEndian.PutUint32(buf[offset:offset+4], p.AckedCount)
	offset += 4

	// AckedBytes
	binary.LittleEndian.PutUint64(buf[offset:offset+8], p.AckedBytes)
	offset += 8

	// PacketIDCount
	binary.LittleEndian.PutUint32(buf[offset:offset+4], uint32(len(p.PacketIDs)))
	offset += 4

	// PacketIDs
	for _, packetID := range p.PacketIDs {
		binary.LittleEndian.PutUint64(buf[offset:offset+8], packetID)
		offset += 8
	}

	return buf, nil
}

// Deserialize decodes binary data into a CCFeedbackPacket
func (c *CCFeedbackCodec) Deserialize(data []byte) (any, error) {
	if len(data) < 17 {
		return nil, errors.New("data too short for CCFeedbackPacket header (need 17 bytes)")
	}

	pkt := &CCFeedbackPacket{}
	offset := 0

	// PacketTypeID
	pkt.PacketTypeID = packet.PacketTypeID(data[offset])
	offset++

	// AckedCount
	pkt.AckedCount = binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	// AckedBytes
	pkt.AckedBytes = binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8

	// PacketIDCount
	packetIDCount := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	// Validate length
	expectedLen := 17 + int(packetIDCount)*8
	if len(data) < expectedLen {
		return nil, errors.New("data too short for declared packet ID count")
	}

	// PacketIDs
	pkt.PacketIDs = make([]uint64, packetIDCount)
	for i := uint32(0); i < packetIDCount; i++ {
		pkt.PacketIDs[i] = binary.LittleEndian.Uint64(data[offset : offset+8])
		offset += 8
	}

	return pkt, nil
}

var _ packet.PacketCodec = (*CCFeedbackCodec)(nil)
