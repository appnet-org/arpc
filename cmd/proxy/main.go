package main

import (
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
		copy(data, buf[:n])

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

			serverAddr, err := net.ResolveUDPAddr("udp", "10.244.0.33:9000")
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

	// Forward the packet
	_, err := conn.WriteToUDP(data, peer)
	if err != nil {
		log.Printf("Forwarding error (%v → %v): %v", src, peer, err)
		return
	}
	log.Printf("Forwarded %d bytes: %v → %v", len(data), src, peer)
}
