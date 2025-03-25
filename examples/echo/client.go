package main

import (
	"fmt"
	"log"

	"github.com/appnet-org/aprc/pkg/rpc"
)

func main() {
	client, err := rpc.NewClient()
	if err != nil {
		log.Fatal("Failed to create client:", err)
	}

	message := "Hello, UDP RPC!"
	response, err := client.Call("127.0.0.1:9000", []byte(message))
	if err != nil {
		log.Fatal("RPC call failed:", err)
	}

	fmt.Println("Echoed response:", string(response))
}
