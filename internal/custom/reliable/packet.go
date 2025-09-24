package reliable

import (
	"bytes"
	"encoding/binary"
	"errors"

	"github.com/appnet-org/arpc/internal/packet"
)

// ACKPacket represents an acknowledgment packet
type ACKPacket struct {
	RPCID     uint64 // RPC ID being acknowledged
	Kind      uint8  // Kind of packet (0=request, 1=response, 2=error)
	Status    uint8  // Status code (0=success, 1=error, etc.)
	Timestamp int64  // Timestamp when ACK was generated
	Message   string // Optional message
}

// ACKPacketCodec implements PacketCodec for ACK packets
type ACKPacketCodec struct{}

func (c *ACKPacketCodec) Serialize(packet any) ([]byte, error) {
	p, ok := packet.(*ACKPacket)
	if !ok {
		return nil, errors.New("invalid packet type for ACK codec")
	}

	buf := new(bytes.Buffer)

	// Write RPC ID
	if err := binary.Write(buf, binary.LittleEndian, p.RPCID); err != nil {
		return nil, err
	}

	if err := binary.Write(buf, binary.LittleEndian, p.Kind); err != nil {
		return nil, err
	}

	// Write status
	if err := binary.Write(buf, binary.LittleEndian, p.Status); err != nil {
		return nil, err
	}

	// Write timestamp
	if err := binary.Write(buf, binary.LittleEndian, p.Timestamp); err != nil {
		return nil, err
	}

	// Write message length and string
	msgBytes := []byte(p.Message)
	if err := binary.Write(buf, binary.LittleEndian, uint32(len(msgBytes))); err != nil {
		return nil, err
	}
	if _, err := buf.Write(msgBytes); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (c *ACKPacketCodec) Deserialize(data []byte) (any, error) {
	buf := bytes.NewReader(data)
	pkt := &ACKPacket{}

	// Read RPC ID
	if err := binary.Read(buf, binary.LittleEndian, &pkt.RPCID); err != nil {
		return nil, err
	}

	// Read Kind
	if err := binary.Read(buf, binary.LittleEndian, &pkt.Kind); err != nil {
		return nil, err
	}

	// Read status
	if err := binary.Read(buf, binary.LittleEndian, &pkt.Status); err != nil {
		return nil, err
	}

	// Read timestamp
	if err := binary.Read(buf, binary.LittleEndian, &pkt.Timestamp); err != nil {
		return nil, err
	}

	// Read message length and string
	var msgLen uint32
	if err := binary.Read(buf, binary.LittleEndian, &msgLen); err != nil {
		return nil, err
	}

	msgBytes := make([]byte, msgLen)
	if _, err := buf.Read(msgBytes); err != nil {
		return nil, err
	}
	pkt.Message = string(msgBytes)

	return pkt, nil
}

var _ packet.PacketCodec = (*ACKPacketCodec)(nil)
