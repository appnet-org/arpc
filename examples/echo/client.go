package main

import (
	"fmt"
	"log"
	"github.com/appnet-org/aprc/pkg/rpc"
	"github.com/appnet-org/aprc/internal/protocol"
)

func main() {
	client := rpc.NewClient()
	message := &protocol.RPCMessage{
		ID:      1,
		Method:  "Echo",
		Payload: []byte("Hello, UDP RPC!"),
	}
	response, err := client.Call("localhost:9000", message)
	if err != nil {
		log.Fatalf("Failed to call server: %v", err)
	}
	fmt.Printf("Response: %s\n", string(response.Payload))
}
