package main

import (
	"fmt"
	"log"
	"github.com/appnet-org/aprc/pkg/rpc"
	"github.com/appnet-org/aprc/internal/protocol"
)

func echoHandler(msg *protocol.RPCMessage) *protocol.RPCMessage {
	return &protocol.RPCMessage{
		ID:      msg.ID,
		Method:  msg.Method,
		Payload: msg.Payload,
	}
}

func main() {
	server, err := rpc.NewServer(":9000", echoHandler)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	fmt.Println("UDP RPC Echo Server is running on port 9000...")
	server.Start()
}