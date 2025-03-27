package main

import (
	"bytes"
	"log"

	"github.com/appnet-org/aprc/pkg/rpc"
)

func main() {
	client, err := rpc.NewClient()
	if err != nil {
		log.Fatal("Failed to create client:", err)
	}

	// message := "Hello, UDP RPC!"

	// Build a ~1500 byte message to force 2 packets
	size := 1600
	message := bytes.Repeat([]byte("A"), size)

	log.Printf("Client sending %d bytes\n", len(message))

	response, err := client.Call("127.0.0.1:9000", []byte(message))
	if err != nil {
		log.Fatal("RPC call failed:", err)
	}

	log.Printf("Client got %d bytes back\n", len(response))
}
