package main

import (
	"bytes"
	"encoding/binary"
	"log"
	"net"
	"os"
	"sync"
)

type ProxyState struct {
	mu          sync.Mutex
	connections map[string]*net.UDPAddr // key: sender IP:port, value: peer
}

func main() {
	log.Println("Starting bidirectional UDP proxy on :15002 and :15006...")

	ports := []int{15002, 15006}
	state := &ProxyState{
		connections: make(map[string]*net.UDPAddr),
	}

	for _, port := range ports {
		go func(p int) {
			listenAddr := &net.UDPAddr{Port: p}
			conn, err := net.ListenUDP("udp", listenAddr)
			if err != nil {
				log.Fatalf("ListenUDP error on port %d: %v", p, err)
			}
			defer conn.Close()

			log.Printf("Listening on UDP port %d", p)
			buf := make([]byte, 2048)
			for {
				n, src, err := conn.ReadFromUDP(buf)
				if err != nil {
					log.Printf("ReadFromUDP error on port %d: %v", p, err)
					continue
				}
				data := make([]byte, n)
				copy(data, buf[:n]) // This incurs a full packet copy (TODO: optimize)

				go handlePacket(conn, state, src, data)
			}
		}(port)
	}

	select {} // block forever
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
			destAddr := os.Getenv("SYMPHONY_DEST_ADDR")
			if destAddr == "" {
				destAddr = "130.127.133.184:11000"
				log.Printf("SYMPHONY_DEST_ADDR is not set, using default: %v", destAddr)
			} else {
				log.Printf("SYMPHONY_DEST_ADDR is set to %v", destAddr)
			}

			serverAddr, err := net.ResolveUDPAddr("udp", destAddr)
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
	if len(data) < 15 {
		log.Printf("Invalid packet: length %d is too short", len(data))
		return data
	}
	packetType := data[0]
	rpcId := data[1:9]
	totalPackets := binary.LittleEndian.Uint16(data[9:11])
	seqNumber := binary.LittleEndian.Uint16(data[11:13])
	ip := data[13:17]
	log.Printf("Original IP: %s", ip)
	serviceLen := binary.LittleEndian.Uint16(data[17:19])
	if 15+serviceLen+2 > uint16(len(data)) {
		log.Printf("Invalid packet: service length %d is too large", serviceLen)
		return data
	}
	service := data[19 : 19+serviceLen]
	methodLen := binary.LittleEndian.Uint16(data[19+serviceLen : 19+serviceLen+2])
	if 15+serviceLen+2+methodLen > uint16(len(data)) {
		log.Printf("Invalid packet: method length %d is too large", methodLen)
		return data
	}
	method := data[19+serviceLen+2 : 19+serviceLen+2+methodLen]

	log.Printf("Packet type: %d", packetType)
	log.Printf("RPC ID: %x", rpcId)
	log.Printf("Total packets: %d", totalPackets)
	log.Printf("Sequence number: %d", seqNumber)
	log.Printf("Service length: %d", serviceLen)
	log.Printf("Service: %s", service)
	log.Printf("Method length: %d", methodLen)
	log.Printf("Method: %s", method)

	// Extract payload
	payload := data[19+serviceLen+2+methodLen:]
	log.Printf("Payload length: %d", len(payload))
	log.Printf("Payload: %x", payload)

	if packetType == 1 {
		// find the index of string "bob"
		idx := bytes.Index(data, []byte("Bob"))
		log.Printf("Index of 'Bob': %d", idx)

		// replace string "Bob" with "Max" if found
		if idx != -1 && idx+3 <= len(data) {
			copy(data[idx:idx+3], []byte("Max"))
		}
	}

	return data
}
