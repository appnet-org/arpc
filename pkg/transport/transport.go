package transport

import (
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/packet"
	"github.com/appnet-org/arpc/pkg/transport/balancer"
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
	packets      *packet.PacketRegistry
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

	// Set default packet registry
	transport.packets = packet.DefaultRegistry.Copy()

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

	// Extract destination IP and port
	var dstIP [4]byte
	var dstPort uint16
	if ip4 := udpAddr.IP.To4(); ip4 != nil {
		copy(dstIP[:], ip4)
		dstPort = uint16(udpAddr.Port)
	}

	// Get source IP and port from local address
	localAddr := t.LocalAddr()
	var srcIP [4]byte

	// Check if bound to unspecified address (0.0.0.0 or ::)
	if localAddr.IP.IsUnspecified() {
		// Resolve actual source IP for unspecified bindings
		if actualSrcIP := getSourceIPForDestination(udpAddr); actualSrcIP != nil {
			if ip4 := actualSrcIP.To4(); ip4 != nil {
				copy(srcIP[:], ip4)
			}
		}
	} else if ip4 := localAddr.IP.To4(); ip4 != nil {
		// Use the bound IPv4 address
		copy(srcIP[:], ip4)
	} else {
		// IPv6 address bound to specific IP - resolve IPv4 equivalent for packet format
		if actualSrcIP := getSourceIPForDestination(udpAddr); actualSrcIP != nil {
			if ip4 := actualSrcIP.To4(); ip4 != nil {
				copy(srcIP[:], ip4)
			}
		}
	}
	srcPort := uint16(localAddr.Port)

	// Fragment the data into multiple packets if it exceeds the UDP payload limit
	packets, err := t.reassembler.FragmentData(data, rpcID, packetType, dstIP, dstPort, srcIP, srcPort)
	if err != nil {
		return err
	}

	// Iterate through each fragment and send it via the UDP connection
	for _, pkt := range packets {
		// Get the handler chain for this packet type
		handler, exists := t.handlers.GetHandlerChain(packetType.TypeID, RoleClient)
		if !exists {
			return fmt.Errorf("no handler chain found for packet type: %s", packetType.Name)
		}

		// Process the packet through OnSend handlers before sending
		if err := handler.OnSend(pkt, udpAddr); err != nil {
			return fmt.Errorf("handler processing failed: %w", err)
		}

		// Serialize the packet into a byte slice for transmission
		packetData, err := packet.SerializePacket(pkt, packetType)
		logging.Debug("Serialized packet", zap.Uint64("rpcID", rpcID))
		if err != nil {
			return err
		}

		_, err = t.conn.WriteToUDP(packetData, udpAddr)
		logging.Debug("Sent packet", zap.Uint64("rpcID", rpcID))
		if err != nil {
			return err
		}
	}

	return nil
}

// Receive takes a buffer size as input, read data from the UDP socket, and return
// the following information when receiving the complete data for an RPC message:
// * complete data for a message (if no message is complete, it will return nil)
// * original source address from packet headers (for responses)
// * RPC id
// * packet type
// * error
func (t *UDPTransport) Receive(bufferSize int, role Role) ([]byte, *net.UDPAddr, uint64, packet.PacketType, error) {
	// Read data from the UDP connection into the buffer
	buffer := make([]byte, bufferSize)
	n, addr, err := t.conn.ReadFromUDP(buffer)
	if err != nil {
		return nil, nil, 0, packet.PacketTypeUnknown, err
	}

	// Deserialize the received data using transport's packet registry
	// (not DefaultRegistry, which doesn't have custom packets like ACK)
	if n < 1 {
		return nil, nil, 0, packet.PacketTypeUnknown, fmt.Errorf("data too short to read packet type")
	}

	packetTypeID := packet.PacketTypeID(buffer[0])
	codec, exists := t.packets.GetCodec(packetTypeID)
	if !exists {
		return nil, nil, 0, packet.PacketTypeUnknown, fmt.Errorf("codec not found for packet type ID %d", packetTypeID)
	}

	pkt, err := codec.Deserialize(buffer[:n])
	if err != nil {
		return nil, nil, 0, packet.PacketTypeUnknown, err
	}

	packetType, _ := t.packets.GetPacketType(packetTypeID)

	// Use the handler registry to process the packet
	handler, exists := t.handlers.GetHandlerChain(packetType.TypeID, role)
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
	fullMessage, _, reassembledRPCID, isComplete := t.reassembler.ProcessFragment(pkt, addr)

	if isComplete {
		// For responses, return the original source address from packet headers (SrcIP:SrcPort)
		// This allows the server to send responses back to the original client
		originalSrcAddr := &net.UDPAddr{
			IP:   net.IP(pkt.SrcIP[:]),
			Port: int(pkt.SrcPort),
		}
		return fullMessage, originalSrcAddr, reassembledRPCID, packetType, nil
	}

	// Still waiting for more fragments
	return nil, nil, 0, packetType, nil
}

func (t *UDPTransport) Close() error {
	// Stop the timer manager before closing the connection
	t.timerManager.Stop()
	return t.conn.Close()
}

// RegisterHandlerChain registers a handler chain for a packet type
func (t *UDPTransport) RegisterHandlerChain(packetTypeID packet.PacketTypeID, chain *HandlerChain, role Role) {
	t.handlers.RegisterHandlerChain(packetTypeID, chain, role)
}

// RegisterPacketType registers a packet type with the transport
func (t *UDPTransport) RegisterPacketType(packetType string, codec packet.PacketCodec) (packet.PacketType, error) {
	return t.packets.RegisterPacketType(packetType, codec)
}

// RegisterPacketTypeWithID registers a packet type with a specific ID
func (t *UDPTransport) RegisterPacketTypeWithID(packetType string, id packet.PacketTypeID, codec packet.PacketCodec) (packet.PacketType, error) {
	return t.packets.RegisterPacketTypeWithID(packetType, id, codec)
}

// GetPacketRegistry returns the packet registry for advanced operations
func (t *UDPTransport) GetPacketRegistry() *packet.PacketRegistry {
	return t.packets
}

// GetHandlerRegistry returns the handler registry for advanced operations
func (t *UDPTransport) GetHandlerRegistry() *HandlerRegistry {
	return t.handlers
}

// GetTimerManager returns the timer manager for advanced operations
func (t *UDPTransport) GetTimerManager() *TimerManager {
	return t.timerManager
}

// GetConn returns the underlying UDP connection for direct packet sending
func (t *UDPTransport) GetConn() *net.UDPConn {
	return t.conn
}

// ListRegisteredPackets returns all registered packet types
func (t *UDPTransport) ListRegisteredPackets() []packet.PacketType {
	return t.packets.ListPacketTypes()
}

// LocalAddr returns the local UDP address of the transport
func (t *UDPTransport) LocalAddr() *net.UDPAddr {
	return t.conn.LocalAddr().(*net.UDPAddr)
}

// getSourceIPForDestination determines which local IP would be used to reach the destination
// This solves the 0.0.0.0 binding issue by asking the OS routing table
func getSourceIPForDestination(dst *net.UDPAddr) net.IP {
	// Create a temporary connection to determine the source IP
	conn, err := net.Dial("udp", dst.String())
	if err != nil {
		return nil
	}
	defer conn.Close()

	if udpAddr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return udpAddr.IP
	}

	return nil
}
