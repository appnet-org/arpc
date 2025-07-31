package protocol

import (
	"bytes"
	"encoding/binary"
	"log"
)

const MaxUDPPayloadSize = 1400 // Adjust based on MTU considerations

// PacketType is the type of packet. 0 is reserved for errors.
type PacketType uint8

const (
	PacketTypeRequest  PacketType = 1
	PacketTypeResponse PacketType = 2
	PacketTypeAck      PacketType = 3
	PacketTypeError    PacketType = 4
)

type Packet struct {
	PacketType   PacketType
	RPCID        uint64 // Unique RPC ID
	TotalPackets uint16 // Total number of packets in this RPC
	SeqNumber    uint16 // Sequence number of this packet
	Payload      []byte // Partial application data
}

func writeToBuffer(buf *bytes.Buffer, data ...any) error {
	for _, d := range data {
		if err := binary.Write(buf, binary.LittleEndian, d); err != nil {
			return err
		}
	}
	return nil
}

// SerializePacket encodes a RPC Packet into bytes
func SerializePacket(pkt *Packet, packetType PacketType) ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := writeToBuffer(buf, packetType, pkt.RPCID, pkt.TotalPackets, pkt.SeqNumber, pkt.Payload); err != nil {
		return nil, err
	}

	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, pkt.RPCID)
	log.Printf("RPC ID bytes: %x", b)

	return buf.Bytes(), nil
}

// DeserializePacket decodes bytes into a Packet struct
func DeserializePacket(data []byte) (*Packet, PacketType, error) {
	buf := bytes.NewReader(data)
	pkt := &Packet{}

	if err := binary.Read(buf, binary.LittleEndian, &pkt.PacketType); err != nil {
		return nil, 0, err
	}

	// If the packet is an ACK, return it immediately
	if pkt.PacketType == PacketTypeAck {
		return pkt, pkt.PacketType, nil
	}

	if err := binary.Read(buf, binary.LittleEndian, &pkt.RPCID); err != nil {
		return nil, 0, err
	}
	if err := binary.Read(buf, binary.LittleEndian, &pkt.TotalPackets); err != nil {
		return nil, 0, err
	}
	if err := binary.Read(buf, binary.LittleEndian, &pkt.SeqNumber); err != nil {
		return nil, 0, err
	}

	pkt.Payload = make([]byte, buf.Len())
	if _, err := buf.Read(pkt.Payload); err != nil {
		return nil, 0, err
	}

	return pkt, pkt.PacketType, nil
}

// FragmentData splits data into multiple UDP packets
func FragmentData(rpcID uint64, data []byte) ([]*Packet, error) {
	chunkSize := MaxUDPPayloadSize - 12                             // Subtract header size
	totalPackets := uint16((len(data) + chunkSize - 1) / chunkSize) // Compute number of packets
	var packets []*Packet

	for i := range int(totalPackets) {
		start := i * chunkSize
		end := min(start+chunkSize, len(data))

		// Create a packet for the current chunk
		pkt := &Packet{
			RPCID:        rpcID,
			TotalPackets: totalPackets,
			SeqNumber:    uint16(i),
			Payload:      data[start:end],
		}
		packets = append(packets, pkt)
	}

	return packets, nil
}
