package transport

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/appnet-org/arpc/internal/packet"
	"github.com/appnet-org/arpc/internal/transport/balancer"
	"github.com/appnet-org/arpc/pkg/logging"
	"go.uber.org/zap"
)

// GenerateRPCID creates a unique RPC ID
func GenerateRPCID() uint64 {
	return uint64(time.Now().UnixNano()) + uint64(rand.Intn(1000))
}

type UDPTransport struct {
	conn         *net.UDPConn
	reassembler  *DataReassembler
	resolver     *balancer.Resolver
	handlers     *HandlerRegistry
	timerManager *TimerManager
}

func NewUDPTransport(address string) (*UDPTransport, error) {
	return NewUDPTransportWithBalancer(address, balancer.DefaultResolver())
}

// NewUDPTransportWithBalancer creates a new UDP transport with a custom balancer
func NewUDPTransportWithBalancer(address string, resolver *balancer.Resolver) (*UDPTransport, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}

	transport := &UDPTransport{
		conn:         conn,
		reassembler:  NewDataReassembler(),
		resolver:     resolver,
		handlers:     nil, // Will be set after transport is created
		timerManager: NewTimerManager(),
	}

	// Set handlers after transport is fully constructed
	transport.handlers = NewHandlerRegistry(transport)

	return transport, nil
}

// ResolveUDPTarget resolves a UDP address string that may be an IP, FQDN, or empty.
// If it's empty or ":port", it binds to 0.0.0.0:<port>. For FQDNs, it uses the configured balancer
// to select an IP from the resolved addresses.
func ResolveUDPTarget(addr string) (*net.UDPAddr, error) {
	// Use default resolver for backward compatibility
	return balancer.DefaultResolver().ResolveUDPTarget(addr)
}

func (t *UDPTransport) Send(addr string, rpcID uint64, data []byte, packetType packet.PacketType) error {
	// Use the transport's resolver instead of the global function
	udpAddr, err := t.resolver.ResolveUDPTarget(addr)
	if err != nil {
		return err
	}

	// TODO(XZ): this is a temporary solution fix issue #5
	if packetType == packet.PacketTypeRequest {
		if ip4 := udpAddr.IP.To4(); ip4 != nil {
			if len(data) < 6 {
				return fmt.Errorf("data too short to embed IP and port")
			}
			copy(data[0:4], ip4)
			binary.LittleEndian.PutUint16(data[4:6], uint16(udpAddr.Port))
			logging.Debug("Embedded IP and port",
				zap.String("ip", ip4.String()),
				zap.Uint16("port", uint16(udpAddr.Port)))
		} else {
			return fmt.Errorf("destination IP is not IPv4")
		}
	}

	// Fragment the data into multiple packets if it exceeds the UDP payload limit
	packets, err := t.reassembler.FragmentData(data, rpcID, packetType)
	if err != nil {
		return err
	}

	// Iterate through each fragment and send it via the UDP connection
	for _, pkt := range packets {
		// Serialize the packet into a byte slice for transmission
		packetData, err := packet.SerializePacket(pkt, packetType)
		logging.Debug("Serialized packet", zap.String("packetData", fmt.Sprintf("%x", packetData)))
		if err != nil {
			return err
		}

		_, err = t.conn.WriteToUDP(packetData, udpAddr)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *UDPTransport) Receive(bufferSize int) ([]byte, *net.UDPAddr, uint64, packet.PacketType, error) {
	// Read data from the UDP connection into the buffer
	buffer := make([]byte, bufferSize)
	n, addr, err := t.conn.ReadFromUDP(buffer)
	if err != nil {
		return nil, nil, 0, packet.PacketTypeUnknown, err
	}

	// Deserialize the received data into a structured packet format
	pkt, packetType, err := packet.DeserializePacketAny(buffer[:n])
	if err != nil {
		return nil, nil, 0, packetType, err
	}

	// Use the handler registry to process the packet
	handler, exists := t.handlers.GetHandlerChain(packetType.ID)
	if !exists {
		return nil, nil, 0, packetType, fmt.Errorf("no handler chain found for packet type: %s", packetType.Name)
	}

	// Process the packet through its handlers first
	if err := handler.OnReceive(pkt, addr); err != nil {
		return nil, nil, 0, packetType, fmt.Errorf("handler processing failed: %w", err)
	}

	// Handle different packet types based on their nature
	switch p := pkt.(type) {
	case *packet.DataPacket:
		return t.ReassembleDataPacket(p, addr, packetType)
	case *packet.ErrorPacket:
		return []byte(p.ErrorMsg), addr, p.RPCID, packetType, nil
	default:
		// Unknown packet type - return early with no data
		logging.Debug("Unknown packet type", zap.String("packetType", packetType.Name))
		return nil, nil, 0, packetType, nil
	}
}

// ReassembleDataPacket processes data packets through the reassembly layer
func (t *UDPTransport) ReassembleDataPacket(pkt *packet.DataPacket, addr *net.UDPAddr, packetType packet.PacketType) ([]byte, *net.UDPAddr, uint64, packet.PacketType, error) {
	// Process fragment through reassembly layer
	fullMessage, reassembledAddr, reassembledRPCID, isComplete := t.reassembler.ProcessFragment(pkt, addr)

	if isComplete {
		// Message is complete, return the reassembled data
		return fullMessage, reassembledAddr, reassembledRPCID, packetType, nil
	}

	// Still waiting for more fragments
	return nil, nil, 0, packetType, nil
}

func (t *UDPTransport) Close() error {
	// Stop the timer manager before closing the connection
	t.timerManager.Stop()
	return t.conn.Close()
}
