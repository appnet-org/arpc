package util

import (
	"fmt"
	"net"

	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/packet"
	"go.uber.org/zap"
)

// SendErrorPacket sends an error packet back to the source with routing information
func SendErrorPacket(conn *net.UDPConn, dest *net.UDPAddr, rpcID uint64, errorMsg string, dstIP [4]byte, dstPort uint16, srcIP [4]byte, srcPort uint16) error {
	// Create error packet
	errorPacket := &packet.ErrorPacket{
		PacketTypeID: packet.PacketTypeError.TypeID,
		RPCID:        rpcID,
		DstIP:        dstIP,
		DstPort:      dstPort,
		SrcIP:        srcIP,
		SrcPort:      srcPort,
		ErrorMsg:     errorMsg,
	}

	// Serialize the error packet
	codec := &packet.ErrorPacketCodec{}
	serialized, err := codec.Serialize(errorPacket, nil)
	if err != nil {
		return fmt.Errorf("failed to serialize error packet: %w", err)
	}

	// Send the error packet back to the source
	if _, err := conn.WriteToUDP(serialized, dest); err != nil {
		return fmt.Errorf("failed to send error packet: %w", err)
	}

	logging.Debug("Sent error packet",
		zap.Uint64("rpcID", rpcID),
		zap.String("dest", dest.String()),
		zap.String("errorMsg", errorMsg))

	return nil
}
