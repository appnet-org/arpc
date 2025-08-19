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

// Role indicates whether this is a client (caller) or server (callee)
type Role string

const (
	RoleClient Role = "client" // caller
	RoleServer Role = "server" // callee
)

// TransportElement defines the interface that all transport elements must implement
type TransportElement interface {
	// ProcessSend processes outgoing data before it's sent
	ProcessSend(addr string, data []byte, rpcID uint64) ([]byte, error)

	// ProcessReceive processes incoming data after it's received
	ProcessReceive(data []byte, rpcID uint64, packetType protocol.PacketType, addr *net.UDPAddr, conn *net.UDPConn) ([]byte, error)

	// Name returns the name of the transport element
	Name() string

	// GetRole returns the role of this element (client/caller or server/callee)
	GetRole() Role
}

// TransportElementChain represents a chain of transport elements
type TransportElementChain struct {
	elements []TransportElement
}

// NewTransportElementChain creates a new chain of transport elements
func NewTransportElementChain(elements ...TransportElement) *TransportElementChain {
	return &TransportElementChain{
		elements: elements,
	}
}

// ProcessSend processes data through all elements in the chain
func (c *TransportElementChain) ProcessSend(addr string, data []byte, rpcID uint64) ([]byte, error) {
	log.Println("Processing sent data through elements")
	var err error
	for _, element := range c.elements {
		data, err = element.ProcessSend(addr, data, rpcID)
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

// ProcessReceive processes data through all elements in reverse order
func (c *TransportElementChain) ProcessReceive(data []byte, rpcID uint64, packetType protocol.PacketType, addr *net.UDPAddr, conn *net.UDPConn) ([]byte, error) {
	log.Println("Processing received data through elements")
	var err error
	for i := len(c.elements) - 1; i >= 0; i-- {
		data, err = c.elements[i].ProcessReceive(data, rpcID, packetType, addr, conn)
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

// GenerateRPCID creates a unique RPC ID
func GenerateRPCID() uint64 {
	return uint64(time.Now().UnixNano()) + uint64(rand.Intn(1000))
}

type UDPTransport struct {
	conn     *net.UDPConn
	incoming map[uint64]map[uint16][]byte // Buffer for reassembling messages
	mu       sync.Mutex                   // Ensures thread safety
	elements *TransportElementChain
	resolver *balancer.Resolver // Add resolver field
}

func NewUDPTransport(address string, elements ...TransportElement) (*UDPTransport, error) {
	return NewUDPTransportWithBalancer(address, balancer.DefaultResolver(), elements...)
}

// NewUDPTransportWithBalancer creates a new UDP transport with a custom balancer
func NewUDPTransportWithBalancer(address string, resolver *balancer.Resolver, elements ...TransportElement) (*UDPTransport, error) {
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
		elements: NewTransportElementChain(elements...),
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
	// Process data through user-defined transport elements
	processedData, err := t.elements.ProcessSend(addr, data, rpcID)
	if err != nil {
		return err
	}

	// Use the transport's resolver instead of the global function
	udpAddr, err := t.resolver.ResolveUDPTarget(addr)
	if err != nil {
		return err
	}

	// TODO(XZ): this is a temporary solution fix issue #5
	if packetType == protocol.PacketTypeRequest {
		if ip4 := udpAddr.IP.To4(); ip4 != nil {
			if len(processedData) < 6 {
				return fmt.Errorf("processedData too short to embed IP and port")
			}
			copy(processedData[0:4], ip4)
			binary.LittleEndian.PutUint16(processedData[4:6], uint16(udpAddr.Port))
			log.Printf("Embedded IP and port: %s:%d", ip4, udpAddr.Port)
		} else {
			return fmt.Errorf("destination IP is not IPv4")
		}
	}

	// Fragment the data into multiple packets if it exceeds the UDP payload limit
	packets, err := protocol.FragmentData(processedData, rpcID, packetType)
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
		t.elements.ProcessReceive(buffer[:n], ackPkt.RPCID, packetType, addr, t.conn)
		return nil, addr, ackPkt.RPCID, nil
	}

	// Type assert to DataPacket for Request/Response packets
	dataPkt := pkt.(*protocol.DataPacket)

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

		// Process received data through user-defined transport elements
		processedData, err := t.elements.ProcessReceive(fullMessage, dataPkt.RPCID, packetType, addr, t.conn)
		if err != nil {
			return nil, nil, 0, err
		}

		return processedData, addr, dataPkt.RPCID, nil
	}

	// If the message is incomplete, return nil to indicate more packets are needed
	return nil, nil, 0, nil
}

func (t *UDPTransport) Close() error {
	return t.conn.Close()
}
