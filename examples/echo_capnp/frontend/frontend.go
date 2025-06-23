package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	echo "github.com/appnet-org/arpc/examples/echo_capnp/capnp"
	"github.com/appnet-org/arpc/internal/transport"
	"github.com/appnet-org/arpc/internal/transport/elements"
	"github.com/appnet-org/arpc/pkg/metadata"
	"github.com/appnet-org/arpc/pkg/rpc"
	"github.com/appnet-org/arpc/pkg/rpc/element"
	"github.com/appnet-org/arpc/pkg/serializer"
)

var echoClient echo.EchoServiceClient

func handler(w http.ResponseWriter, r *http.Request) {
	content := r.URL.Query().Get("key")
	log.Printf("Received HTTP request with key: %s\n", content)

	req, err := echo.CreateEchoRequest(
		1,        // id
		10.0,     // score
		content,  // content
		"latest", // tag
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create request: %v", err), http.StatusInternalServerError)
		return
	}

	md := metadata.New(map[string]string{
		"username": "Bob",
	})
	ctx := metadata.NewOutgoingContext(context.Background(), md)

	resp, err := echoClient.Echo(ctx, req)
	if err != nil {
		http.Error(w, fmt.Sprintf("RPC call failed: %v", err), http.StatusInternalServerError)
		return
	}

	respContent, err := resp.GetContent()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get response content: %v", err), http.StatusInternalServerError)
		return
	}
	log.Printf("RPC response: %s\n", respContent)
	fmt.Fprintf(w, "Response from RPC: %s\n", respContent)
}

func main() {
	// Create RPC client with capnp serializer
	serializer := &serializer.CapnpSerializer{}

	// Create transport elements
	transportElements := []transport.TransportElement{
		elements.NewLoggingElement(log.New(os.Stdout, "aRPC: ", log.LstdFlags)),
	}

	// Create metrics element
	metrics := NewMetricsElement()

	// Create RPC elements
	rpcElements := []element.RPCElement{
		metrics,
	}

	// Create client with both transport and RPC elements
	// TODO: change to your server's address (currently retrived from kubectl get endpoints)
	client, err := rpc.NewClient(serializer, ":9000", transportElements, rpcElements)
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
