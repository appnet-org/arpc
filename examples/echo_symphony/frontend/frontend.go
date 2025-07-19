package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	echo "github.com/appnet-org/arpc/examples/echo_symphony/symphony"
	"github.com/appnet-org/arpc/pkg/rpc"
	"github.com/appnet-org/arpc/pkg/serializer"
)

var echoClient echo.EchoServiceClient

func handler(w http.ResponseWriter, r *http.Request) {
	message := r.URL.Query().Get("key")
	log.Printf("Received HTTP request with key: %s\n", message)

	req := &echo.EchoRequest{
		Id:       42,
		Score:    100,
		Username: "alice",
		Content:  message,
	}
	resp, err := echoClient.Echo(context.Background(), req)

	log.Printf("RPC response: %s\n", resp.Username)
	if err != nil {
		http.Error(w, fmt.Sprintf("RPC call failed: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("RPC response: %s\n", resp.Content)
	fmt.Fprintf(w, "Response from RPC: %s\n", resp.Content)
}

func main() {
	// Create RPC client
	serializer := &serializer.SymphonySerializer{}
	client, err := rpc.NewClient(serializer, "130.127.133.184:9000", nil, nil) // TODO: change to your server's address (currently retrived from k get endpoints)
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
