package transport

import (
	"fmt"
	"net"
	"time"

	"github.com/appnet-org/arpc/pkg/common"
	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/packet"
	"github.com/appnet-org/arpc/pkg/transport/balancer"
	"go.uber.org/zap"
)

// GenerateRPCID creates a unique RPC ID
func GenerateRPCID() uint64 {
	return uint64(time.Now().UnixNano())
}

type UDPTransport struct {
	conn         *net.UDPConn
	reassembler  *DataReassembler
	resolver     *balancer.Resolver
	handlers     *HandlerRegistry
	packets      *packet.PacketRegistry
	timerManager *TimerManager
	bufferPool   *common.BufferPool
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

	// Check if binding to 0.0.0.0 - if so, discover the actual source IP
	if udpAddr.IP.IsUnspecified() {
		// Dial a dummy destination to discover the actual source IP
		dummyConn, err := net.Dial("udp", "8.8.8.8:80")
		if err != nil {
			return nil, fmt.Errorf("failed to discover source IP: %w", err)
		}

		// Get the local address that would be used
		localAddr := dummyConn.LocalAddr().(*net.UDPAddr)
		actualIP := localAddr.IP
		dummyConn.Close()

		// Update the bind address to use the discovered IP with the original port
		udpAddr = &net.UDPAddr{
			IP:   actualIP,
			Port: udpAddr.Port,
		}
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}

	// Set UDP socket buffer sizes to handle large bursts of packets
	// For large messages that fragment into many packets, we need larger buffers
	// to prevent packet loss when sending/receiving many packets quickly
	const socketBufferSize = 8 * 1024 * 1024 // 8MB for both send and receive
	if err := conn.SetReadBuffer(socketBufferSize); err != nil {
		logging.Warn("Failed to set UDP read buffer size", zap.Error(err))
	}
	if err := conn.SetWriteBuffer(socketBufferSize); err != nil {
		logging.Warn("Failed to set UDP write buffer size", zap.Error(err))
	}

	transport := &UDPTransport{
		conn:         conn,
		reassembler:  NewDataReassembler(),
		resolver:     resolver,
		handlers:     nil, // Will be set after transport is created
		timerManager: NewTimerManager(),
		bufferPool:   common.NewBufferPool(65536), // Default to 64KB buffer size
	}

	// Set buffer pool in reassembler so it can return buffers after reassembly
	transport.reassembler.SetBufferPool(transport.bufferPool)

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

	// Get source IP and port from the connection's local address
	localAddr := t.LocalAddr()
	var srcIP [4]byte
	if ip4 := localAddr.IP.To4(); ip4 != nil {
		copy(srcIP[:], ip4)
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

		// Serialize the packet into a byte slice for transmission using buffer pool
		packetData, err := packet.SerializePacket(pkt, packetType, t.bufferPool)
		logging.Debug("Serialized packet", zap.Uint64("rpcID", rpcID))
		if err != nil {
			return err
		}

		_, err = t.conn.WriteToUDP(packetData, udpAddr)
		logging.Debug("Sent packet", zap.Uint64("rpcID", rpcID))

		// Return buffer to pool after sending (WriteToUDP copies the data, so it's safe)
		t.bufferPool.Put(packetData)

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
	// Get a buffer from the pool with at least the requested size
	buffer := t.bufferPool.GetSize(bufferSize)

	n, addr, err := t.conn.ReadFromUDP(buffer)
	if err != nil {
		// Return buffer to pool on error
		t.bufferPool.Put(buffer)
		return nil, nil, 0, packet.PacketTypeUnknown, err
	}

	// Deserialize the received data using transport's packet registry
	// (not DefaultRegistry, which doesn't have custom packets like ACK)
	if n < 1 {
		t.bufferPool.Put(buffer)
		return nil, nil, 0, packet.PacketTypeUnknown, fmt.Errorf("data too short to read packet type")
	}

	packetTypeID := packet.PacketTypeID(buffer[0])
	codec, exists := t.packets.GetCodec(packetTypeID)
	if !exists {
		t.bufferPool.Put(buffer)
		return nil, nil, 0, packet.PacketTypeUnknown, fmt.Errorf("codec not found for packet type ID %d", packetTypeID)
	}

	// Deserialize from the buffer (uses zero-copy slices, so buffer must stay alive)
	pkt, err := codec.Deserialize(buffer[:n])
	if err != nil {
		t.bufferPool.Put(buffer)
		return nil, nil, 0, packet.PacketTypeUnknown, err
	}

	packetType, _ := t.packets.GetPacketType(packetTypeID)

	// Use the handler registry to process the packet
	handler, exists := t.handlers.GetHandlerChain(packetType.TypeID, role)
	if !exists {
		// For non-DataPackets, return buffer immediately since we don't need to keep it
		if _, ok := pkt.(*packet.DataPacket); !ok {
			t.bufferPool.Put(buffer)
		}
		return nil, nil, 0, packetType, fmt.Errorf("no handler chain found for packet type: %s", packetType.Name)
	}

	// Process the packet through its handlers first
	if err := handler.OnReceive(pkt, addr); err != nil {
		// For non-DataPackets, return buffer immediately since we don't need to keep it
		if _, ok := pkt.(*packet.DataPacket); !ok {
			t.bufferPool.Put(buffer)
		}
		return nil, nil, 0, packetType, fmt.Errorf("handler processing failed: %w", err)
	}

	// Handle different packet types based on their nature
	switch p := pkt.(type) {
	case *packet.DataPacket:
		// Pass buffer to reassembler - it will return it to pool after reassembly
		return t.ReassembleDataPacket(p, addr, packetType, buffer)
	case *packet.ErrorPacket:
		// ErrorPacket doesn't need buffer kept alive, return it now
		t.bufferPool.Put(buffer)
		return []byte(p.ErrorMsg), addr, p.RPCID, packetType, nil
	default:
		// Unknown packet type - return buffer and return early with no data
		t.bufferPool.Put(buffer)
		logging.Debug("Unknown packet type", zap.String("packetType", packetType.Name))
		return nil, nil, 0, packetType, nil
	}
}

// ReassembleDataPacket processes data packets through the reassembly layer
// buffer is the original buffer containing the packet data - it will be returned to pool after reassembly
func (t *UDPTransport) ReassembleDataPacket(pkt *packet.DataPacket, addr *net.UDPAddr, packetType packet.PacketType, buffer []byte) ([]byte, *net.UDPAddr, uint64, packet.PacketType, error) {
	// Process fragment through reassembly layer
	// Pass buffer so reassembler can keep it alive until reassembly completes
	fullMessage, _, reassembledRPCID, isComplete := t.reassembler.ProcessFragment(pkt, addr, buffer)

	if isComplete {
		// For responses, return the original source address from packet headers (SrcIP:SrcPort)
		// This allows the server to send responses back to the original client
		originalSrcAddr := &net.UDPAddr{
			IP:   net.IP(pkt.SrcIP[:]),
			Port: int(pkt.SrcPort),
		}
		return fullMessage, originalSrcAddr, reassembledRPCID, packetType, nil
	}

	// Still waiting for more fragments - buffer is kept alive in reassembler
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

// GetBufferPool returns the buffer pool for reuse in serialization and other operations
func (t *UDPTransport) GetBufferPool() *common.BufferPool {
	return t.bufferPool
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
