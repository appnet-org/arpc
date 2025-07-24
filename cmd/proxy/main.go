package main

import (
	"bytes"
	"encoding/binary"
	"log"
	"net"
	"sync"
)

type ProxyState struct {
	mu          sync.Mutex
	connections map[string]*net.UDPAddr // key: sender IP:port, value: peer
}

func main() {
	log.Println("Starting bidirectional UDP proxy on :15002...")

	listenAddr := &net.UDPAddr{Port: 15002}
	conn, err := net.ListenUDP("udp", listenAddr)
	if err != nil {
		log.Fatalf("ListenUDP error: %v", err)
	}
	defer conn.Close()

	state := &ProxyState{
		connections: make(map[string]*net.UDPAddr),
	}

	buf := make([]byte, 2048)
	for {
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("ReadFromUDP error: %v", err)
			continue
		}
		data := make([]byte, n)
		copy(data, buf[:n]) // This incurs a full packet copy (TODO: optimize)

		go handlePacket(conn, state, src, data)
	}
}

func handlePacket(conn *net.UDPConn, state *ProxyState, src *net.UDPAddr, data []byte) {
	key := src.String()

	state.mu.Lock()
	peer, known := state.connections[key]
	if !known {
		// Determine direction based on source port
		if src.Port == 9000 {
			log.Printf("New server connection: %v", src)
			// Lookup reverse mapping (from earlier connection)
			// Not yet known, drop unless reverse entry exists
			state.mu.Unlock()
			log.Printf("Unknown client for server response %v — dropping", src)
			return
		} else {
			// New client → server

			// destAddr := os.Getenv("DEST_ADDR")
			// if destAddr == "" {
			// 	log.Printf("DEST_ADDR is not set, using default: %v", destAddr)
			// 	panic("DEST_ADDR is not set")
			// }

			serverAddr, err := net.ResolveUDPAddr("udp", "130.127.133.184:9000")
			if err != nil {
				log.Printf("ResolveUDPAddr error: %v", err)
				state.mu.Unlock()
				return
			}
			log.Printf("New client connection: %v → %v", src, serverAddr)
			state.connections[key] = serverAddr
			state.connections[serverAddr.String()] = src // reverse mapping
			peer = serverAddr
		}
	}
	state.mu.Unlock()

	// Process the packet
	data = processPacket(data)

	// Forward the packet
	_, err := conn.WriteToUDP(data, peer)
	if err != nil {
		log.Printf("Forwarding error (%v → %v): %v", src, peer, err)
		return
	}
	log.Printf("Forwarded %d bytes: %v → %v", len(data), src, peer)
}

func processPacket(data []byte) []byte {
	// Print the packet (in hex)
	log.Printf("Received packet: %x", data)

	// Packet metadata format:
	// [packet_type][rpc_id(8 bytes)][total_packets(2 bytes)][seq_number(2 bytes)][service_len(2 bytes)][service][method_len(2 bytes)][method]

	// Extract metadata
	packetType := data[0]
	rpcId := data[1:9]
	totalPackets := binary.LittleEndian.Uint16(data[9:11])
	seqNumber := binary.LittleEndian.Uint16(data[11:13])
	serviceLen := binary.LittleEndian.Uint16(data[13:15])
	service := data[15 : 15+serviceLen]
	methodLen := binary.LittleEndian.Uint16(data[15+serviceLen : 15+serviceLen+2])
	method := data[15+serviceLen+2 : 15+serviceLen+2+methodLen]

	log.Printf("Packet type: %d", packetType)
	log.Printf("RPC ID: %x", rpcId)
	log.Printf("Total packets: %d", totalPackets)
	log.Printf("Sequence number: %d", seqNumber)
	log.Printf("Service length: %d", serviceLen)
	log.Printf("Service: %s", service)
	log.Printf("Method length: %d", methodLen)
	log.Printf("Method: %s", method)

	// Extract payload
	log.Printf("15+serviceLen+2+methodLen: %d", 15+serviceLen+2+methodLen)
	payload := data[15+serviceLen+2+methodLen:]
	log.Printf("Payload length: %d", len(payload))
	log.Printf("Payload: %x", payload)

	// find the index of string "bob"
	idx := bytes.Index(payload, []byte("Bob"))
	log.Printf("Index of 'Bob': %d", idx)

	return data
}
