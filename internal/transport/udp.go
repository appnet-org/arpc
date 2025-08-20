package transport

import (
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/appnet-org/arpc/internal/protocol"
	"github.com/appnet-org/arpc/internal/transport/balancer"
)

// GenerateRPCID creates a unique RPC ID
func GenerateRPCID() uint64 {
	return uint64(time.Now().UnixNano()) + uint64(rand.Intn(1000))
}

type UDPTransport struct {
	conn     *net.UDPConn
	incoming map[uint64]map[uint16][]byte // Buffer for reassembling messages
	mu       sync.Mutex                   // Ensures thread safety
	resolver *balancer.Resolver           // Add resolver field
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

	return &UDPTransport{
		conn:     conn,
		incoming: make(map[uint64]map[uint16][]byte),
		resolver: resolver,
	}, nil
}

// ResolveUDPTarget resolves a UDP address string that may be an IP, FQDN, or empty.
// If it's empty or ":port", it binds to 0.0.0.0:<port>. For FQDNs, it uses the configured balancer
// to select an IP from the resolved addresses.
func ResolveUDPTarget(addr string) (*net.UDPAddr, error) {
	// Use default resolver for backward compatibility
	return balancer.DefaultResolver().ResolveUDPTarget(addr)
}

func (t *UDPTransport) Send(addr string, rpcID uint64, data []byte, packetType protocol.PacketType) error {
	// Use the transport's resolver instead of the global function
	udpAddr, err := t.resolver.ResolveUDPTarget(addr)
	if err != nil {
		return err
	}

	// TODO(XZ): this is a temporary solution fix issue #5
	if packetType == protocol.PacketTypeRequest {
		if ip4 := udpAddr.IP.To4(); ip4 != nil {
			if len(data) < 6 {
				return fmt.Errorf("data too short to embed IP and port")
			}
			copy(data[0:4], ip4)
			binary.LittleEndian.PutUint16(data[4:6], uint16(udpAddr.Port))
			log.Printf("Embedded IP and port: %s:%d", ip4, udpAddr.Port)
		} else {
			return fmt.Errorf("destination IP is not IPv4")
		}
	}

	// Fragment the data into multiple packets if it exceeds the UDP payload limit
	packets, err := protocol.FragmentData(data, rpcID, packetType)
	if err != nil {
		return err
	}

	// Iterate through each fragment and send it via the UDP connection
	for _, pkt := range packets {
		// Serialize the packet into a byte slice for transmission
		packetData, err := protocol.SerializePacket(pkt, packetType)
		log.Printf("Serialized packet: %x", packetData)
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

func (t *UDPTransport) Receive(bufferSize int) ([]byte, *net.UDPAddr, uint64, error) {

	// Read data from the UDP connection into the buffer
	buffer := make([]byte, bufferSize)
	n, addr, err := t.conn.ReadFromUDP(buffer)
	if err != nil {
		return nil, nil, 0, err
	}

	// Deserialize the received data into a structured packet format
	pkt, packetType, err := protocol.DeserializePacketAny(buffer[:n])
	if err != nil {
		return nil, nil, 0, err
	}

	if packetType == protocol.PacketTypeAck {
		ackPkt := pkt.(*protocol.AckPacket)
		return nil, addr, ackPkt.RPCID, nil
	}

	// Type assert to DataPacket for Request/Response packets
	dataPkt := pkt.(protocol.DataPacket)

	// Lock to ensure thread-safe access to the incoming packet storage
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, exists := t.incoming[dataPkt.RPCID]; !exists {
		t.incoming[dataPkt.RPCID] = make(map[uint16][]byte)
	}

	t.incoming[dataPkt.RPCID][dataPkt.SeqNumber] = dataPkt.Payload

	// If all fragments for this RPCID have been received, reassemble the full message
	if len(t.incoming[dataPkt.RPCID]) == int(dataPkt.TotalPackets) {
		var fullMessage []byte
		for i := uint16(0); i < dataPkt.TotalPackets; i++ {
			fullMessage = append(fullMessage, t.incoming[dataPkt.RPCID][i]...)
		}

		delete(t.incoming, dataPkt.RPCID)

		return fullMessage, addr, dataPkt.RPCID, nil
	}

	// If the message is incomplete, return nil to indicate more packets are needed
	return nil, nil, 0, nil
}

func (t *UDPTransport) Close() error {
	return t.conn.Close()
}
