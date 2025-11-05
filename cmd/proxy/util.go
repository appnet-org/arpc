package main

import (
	"encoding/binary"
	"fmt"
	"net"

	"github.com/appnet-org/proxy/types"
)

// PacketRoutingInfo contains routing information extracted from packet headers
type PacketRoutingInfo struct {
	DstIP   net.IP
	DstPort uint16
	SrcIP   net.IP
	SrcPort uint16
}

// extractRoutingInfo extracts routing information from the packet data
func extractRoutingInfo(data []byte) (*PacketRoutingInfo, error) {
	if len(data) < 29 {
		return nil, fmt.Errorf("packet too short for routing info: %d bytes", len(data))
	}

	// packet format: [PacketTypeID(1B)][RPCID(8B)][TotalPackets(2B)][SeqNumber(2B)][DstIP(4B)][DstPort(2B)][SrcIP(4B)][SrcPort(2B)][PayloadLen(4B)][Payload]

	// Extract destination IP and port
	dstIP := net.IP(data[13:17])
	dstPort := binary.LittleEndian.Uint16(data[17:19])

	// Extract source IP and port
	srcIP := net.IP(data[19:23])
	srcPort := binary.LittleEndian.Uint16(data[23:25])

	return &PacketRoutingInfo{
		DstIP:   dstIP,
		DstPort: dstPort,
		SrcIP:   srcIP,
		SrcPort: srcPort,
	}, nil
}

// extractPacketType extracts the packet type from the packet data
func extractPacketType(data []byte) (types.PacketType, error) {
	if len(data) < 1 {
		return 0, fmt.Errorf("packet too short for packet type: %d bytes", len(data))
	}
	return types.PacketType(data[0]), nil
}
