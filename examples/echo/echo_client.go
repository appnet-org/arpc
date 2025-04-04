package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	echo "github.com/appnet-org/aprc/examples/echo/proto"
	"github.com/appnet-org/aprc/internal/serializer"
	"github.com/appnet-org/aprc/pkg/rpc"
)

var echoClient echo.EchoServiceClient

func handler(w http.ResponseWriter, r *http.Request) {
	message := r.URL.Query().Get("key")
	log.Printf("Received HTTP request with key: %s\n", message)

	req := &echo.EchoRequest{Message: message}
	resp, err := echoClient.Echo(context.Background(), req)
	if err != nil {
		http.Error(w, fmt.Sprintf("RPC call failed: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("RPC response: %s\n", resp.Message)
	fmt.Fprintf(w, "Response from RPC: %s\n", resp.Message)
}

func main() {
	// Create RPC client
	serializer := &serializer.ProtoSerializer{}
	client, err := rpc.NewClient(serializer, "127.0.0.1:9000")
	if err != nil {
		log.Fatal("Failed to create RPC client:", err)
	}

	// Create EchoService client
	echoClient = echo.NewEchoServiceClient(client)

	// Set up HTTP server
	http.HandleFunc("/", handler)
	log.Println("HTTP server listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("HTTP server failed:", err)
	}
}
