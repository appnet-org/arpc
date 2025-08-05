package main

import (
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
	// Process the packet and possibly extract the peer (original sender)
	data, peer := processPacket(data)

	if peer != nil {
		// It's a request: map src <-> peer
		state.mu.Lock()

		// TODO(XZ): temp solution for issue #6. We only rewrite the port for client-side proxy.
		if src.Port != 15002 {
			src.Port = 53357 // hack
		}

		state.connections[src.String()] = peer
		state.connections[peer.String()] = src // reverse mapping
		state.mu.Unlock()
	} else {
		// It's a response: look up the reverse mapping
		state.mu.Lock()
		var ok bool
		peer, ok = state.connections[src.String()]
		state.mu.Unlock()

		if !ok {
			log.Printf("Unknown client for server response %v — dropping", src)
			return
		}
	}

	// Send the processed packet to the peer
	_, err := conn.WriteToUDP(data, peer)
	if err != nil {
		log.Printf("WriteToUDP error: %v", err)
	}

	log.Printf("Forwarded %d bytes: %v → %v", len(data), src, peer)
}

func processPacket(data []byte) ([]byte, *net.UDPAddr) {
	// Print the packet (in hex)
	log.Printf("Received packet: %x", data)

	// Packet metadata format:
	// [packet_type][rpc_id(8 bytes)][total_packets(2 bytes)][seq_number(2 bytes)][service_len(2 bytes)][service][method_len(2 bytes)][method]

	// Extract metadata
	if len(data) < 15 {
		log.Printf("Invalid packet: length %d is too short", len(data))
		return data, nil
	}

	var offset uint16 = 0

	packetType := data[offset]
	offset += 1
	// rpcId := data[offset : offset+8]
	offset += 8
	// totalPackets := binary.LittleEndian.Uint16(data[offset : offset+2])
	offset += 2
	// seqNumber := binary.LittleEndian.Uint16(data[offset : offset+2])
	offset += 2

	// log.Printf("Packet type: %d", packetType)
	// log.Printf("RPC ID: %x", rpcId)
	// log.Printf("Total packets: %d", totalPackets)
	// log.Printf("Sequence number: %d", seqNumber)

	var peer *net.UDPAddr = nil
	if packetType == 1 {
		ip := data[offset : offset+4]
		offset += 4
		port := binary.LittleEndian.Uint16(data[offset : offset+2])
		offset += 2
		peer = &net.UDPAddr{IP: net.IP(ip), Port: int(port)}
		log.Printf("Original IP and port: %s:%d", net.IP(ip), port)
	}

	// serviceLen := binary.LittleEndian.Uint16(data[offset : offset+2])
	// offset += 2
	// if offset+serviceLen > uint16(len(data)) {
	// 	log.Printf("Invalid packet: service length %d is too large", serviceLen)
	// 	return data, nil
	// }
	// service := data[offset : offset+serviceLen]
	// offset += serviceLen
	// methodLen := binary.LittleEndian.Uint16(data[offset : offset+2])
	// offset += 2
	// if offset+methodLen > uint16(len(data)) {
	// 	log.Printf("Invalid packet: method length %d is too large", methodLen)
	// 	return data, nil
	// }
	// method := data[offset : offset+methodLen]
	// offset += methodLen
	// log.Printf("Service length: %d", serviceLen)
	// log.Printf("Service: %s", service)
	// log.Printf("Method length: %d", methodLen)
	// log.Printf("Method: %s", method)

	// // Extract payload
	// payload := data[offset:]
	// log.Printf("Payload length: %d", len(payload))
	// log.Printf("Payload: %x", payload)

	// if packetType == 1 {
	// 	// find the index of string "bob"
	// 	idx := bytes.Index(data, []byte("Bob"))
	// 	log.Printf("Index of 'Bob': %d", idx)

	// 	// replace string "Bob" with "Max" if found
	// 	if idx != -1 && idx+3 <= len(data) {
	// 		copy(data[idx:idx+3], []byte("Max"))
	// 	}
	// }

	return data, peer
}
