package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	echo "github.com/appnet-org/arpc/examples/echo_proto/proto"
	"github.com/appnet-org/arpc/pkg/metadata"
	"github.com/appnet-org/arpc/pkg/rpc"
	"github.com/appnet-org/arpc/pkg/serializer"
)

var echoClient echo.EchoServiceClient

func handler(w http.ResponseWriter, r *http.Request) {
	message := r.URL.Query().Get("key")
	log.Printf("Received HTTP request with key: %s\n", message)

	// Create and attach metadata with the custom header
	md := metadata.New(map[string]string{
		"username": "Bob", // Here we're setting the custom header "key" to the requestBody
	})
	ctx := metadata.NewOutgoingContext(context.Background(), md)

	req := &echo.EchoRequest{Message: message}
	resp, err := echoClient.Echo(ctx, req)
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
	client, err := rpc.NewClient(serializer, ":9000", nil, nil) // TODO: change to your server's address (currently retrived from k get endpoints)
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
