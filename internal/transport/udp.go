package transport

import (
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/appnet-org/aprc/internal/protocol"
)

// GenerateRPCID creates a unique RPC ID
func GenerateRPCID() uint64 {
	return uint64(time.Now().UnixNano()) + uint64(rand.Intn(1000))
}

type UDPTransport struct {
	conn     *net.UDPConn
	incoming map[uint64]map[uint16][]byte // Buffer for reassembling messages
	mu       sync.Mutex                   // Ensures thread safety
}

func NewUDPTransport(address string) (*UDPTransport, error) {
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
	}, nil
}

func (t *UDPTransport) Send(addr string, rpcID uint64, data []byte) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}

	// Fragment the data into multiple packets if it exceeds the UDP payload limit
	packets, err := protocol.FragmentData(rpcID, data)
	if err != nil {
		return err
	}

	// Iterate through each fragment and send it via the UDP connection
	for _, pkt := range packets {
		// Serialize the packet into a byte slice for transmission
		packetData, err := protocol.SerializePacket(pkt)
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
	pkt, err := protocol.DeserializePacket(buffer[:n])
	if err != nil {
		return nil, nil, 0, err
	}

	// Lock to ensure thread-safe access to the incoming packet storage
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, exists := t.incoming[pkt.RPCID]; !exists {
		t.incoming[pkt.RPCID] = make(map[uint16][]byte)
	}

	t.incoming[pkt.RPCID][pkt.SeqNumber] = pkt.Payload

	// If all fragments for this RPCID have been received, reassemble the full message
	if len(t.incoming[pkt.RPCID]) == int(pkt.TotalPackets) {
		var fullMessage []byte
		for i := uint16(0); i < pkt.TotalPackets; i++ {
			fullMessage = append(fullMessage, t.incoming[pkt.RPCID][i]...)
		}

		delete(t.incoming, pkt.RPCID)
		return fullMessage, addr, pkt.RPCID, nil
	}

	// If the message is incomplete, return nil to indicate more packets are needed
	return nil, nil, 0, nil
}

func (t *UDPTransport) Close() error {
	return t.conn.Close()
}
