package main

import (
	"log"

	"github.com/appnet-org/aprc/pkg/rpc"
)

func main() {
	server, err := rpc.NewServer(":9000", func(data []byte) []byte {
		// log.Println("Server received:", string(data))
		log.Printf("Message length: %d bytes\n", len(data))
		return data // Echo back
	})

	if err != nil {
		log.Fatal("Failed to start server:", err)
	}

	server.Start()
}
