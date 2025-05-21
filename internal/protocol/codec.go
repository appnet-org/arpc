package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
)

const MaxUDPPayloadSize = 1400 // Adjust based on MTU considerations

type Packet struct {
	RPCID        uint64 // Unique RPC ID
	TotalPackets uint16 // Total number of packets in this RPC
	SeqNumber    uint16 // Sequence number of this packet
	Payload      []byte // Partial application data
}

// SerializePacket encodes a Packet into bytes
func SerializePacket(pkt *Packet) ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.LittleEndian, pkt.RPCID); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, pkt.TotalPackets); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, pkt.SeqNumber); err != nil {
		return nil, err
	}
	if _, err := buf.Write(pkt.Payload); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// DeserializePacket decodes bytes into a Packet struct
func DeserializePacket(data []byte) (*Packet, error) {
	if len(data) < 12 { // Minimum header size (8 + 2 + 2)
		return nil, errors.New("packet too small")
	}

	buf := bytes.NewReader(data)
	pkt := &Packet{}

	if err := binary.Read(buf, binary.LittleEndian, &pkt.RPCID); err != nil {
		return nil, err
	}
	if err := binary.Read(buf, binary.LittleEndian, &pkt.TotalPackets); err != nil {
		return nil, err
	}
	if err := binary.Read(buf, binary.LittleEndian, &pkt.SeqNumber); err != nil {
		return nil, err
	}

	pkt.Payload = make([]byte, buf.Len())
	if _, err := buf.Read(pkt.Payload); err != nil {
		return nil, err
	}

	return pkt, nil
}

// FragmentData splits data into multiple UDP packets
func FragmentData(rpcID uint64, data []byte) ([]*Packet, error) {
	chunkSize := MaxUDPPayloadSize - 12                             // Subtract header size
	totalPackets := uint16((len(data) + chunkSize - 1) / chunkSize) // Compute number of packets
	var packets []*Packet

	for i := uint16(0); i < totalPackets; i++ {
		start := int(i) * chunkSize
		end := min(start+chunkSize, len(data))

		// Create a packet for the current chunk
		pkt := &Packet{
			RPCID:        rpcID,
			TotalPackets: totalPackets,
			SeqNumber:    i,
			Payload:      data[start:end],
		}
		packets = append(packets, pkt)
	}

	return packets, nil
}
