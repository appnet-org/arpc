package main

import (
	"context"
	"encoding/binary"
	"log"
	"net"
	"sync"

	"github.com/appnet-org/proxy/element"
)

type ProxyState struct {
	mu           sync.Mutex
	connections  map[string]*net.UDPAddr // key: sender IP:port, value: peer
	elementChain *element.RPCElementChain
}

func main() {
	log.Println("Starting bidirectional UDP proxy on :15002 and :15006...")

	// Create element chain - you can add your custom elements here
	elementChain := element.NewRPCElementChain(
		element.NewLoggingElement(true), // Enable verbose logging
	)

	ports := []int{15002, 15006}
	state := &ProxyState{
		connections:  make(map[string]*net.UDPAddr),
		elementChain: elementChain,
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

func extractPeer(data []byte) *net.UDPAddr {
	packetType := data[0]

	var peer *net.UDPAddr = nil
	if packetType == 1 {
		ip := data[13:17]
		port := binary.LittleEndian.Uint16(data[17:19])
		peer = &net.UDPAddr{IP: net.IP(ip), Port: int(port)}
	}

	return peer
}

func handlePacket(conn *net.UDPConn, state *ProxyState, src *net.UDPAddr, data []byte) {
	ctx := context.Background()
	peer := extractPeer(data)

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

	data = processPacket(ctx, state, data, peer != nil)

	// Send the processed packet to the peer
	_, err := conn.WriteToUDP(data, peer)
	if err != nil {
		log.Printf("WriteToUDP error: %v", err)
	}

	log.Printf("Forwarded %d bytes: %v → %v", len(data), src, peer)
}

func processPacket(ctx context.Context, state *ProxyState, data []byte, isRequest bool) []byte {
	// Print the packet (in hex)
	log.Printf("Received packet: %x", data)

	var err error
	if isRequest {
		// Process request through element chain
		data, err = state.elementChain.ProcessRequest(ctx, data)
	} else {
		// Process response through element chain (in reverse order)
		data, err = state.elementChain.ProcessResponse(ctx, data) // TODO: fix this
	}

	if err != nil {
		log.Printf("Error processing response through element chain: %v", err)
		return data // Return original data on error
	}

	return data
}
