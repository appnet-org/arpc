package elements

import (
	"log"
	"net"

	"github.com/appnet-org/arpc/internal/protocol"
	"github.com/appnet-org/arpc/internal/transport"
)

// LoggingElement implements logging for transport operations
type LoggingElement struct {
	role   transport.Role
	logger *log.Logger
}

func NewLoggingElement(role transport.Role, logger *log.Logger) *LoggingElement {
	return &LoggingElement{
		role:   role,
		logger: logger,
	}
}

func (l *LoggingElement) ProcessSend(addr string, data []byte, rpcID uint64) ([]byte, error) {
	l.logger.Printf("[%s] Sending data of length %d for RPC ID %d", l.role, len(data), rpcID)
	return data, nil
}

func (l *LoggingElement) ProcessReceive(data []byte, rpcID uint64, packetType protocol.PacketType, addr *net.UDPAddr, conn *net.UDPConn) ([]byte, error) {
	l.logger.Printf("[%s] Received data of length %d for RPC ID %d, packet type %d", l.role, len(data), rpcID, packetType)
	return data, nil
}

func (l *LoggingElement) Name() string {
	return "logging"
}

// GetRole returns the role of this element
func (l *LoggingElement) GetRole() transport.Role {
	return l.role
}
