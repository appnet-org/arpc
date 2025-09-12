package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	kv "github.com/appnet-org/arpc/benchmark/kv-store/symphony"
	"github.com/appnet-org/arpc/pkg/rpc"
	"github.com/appnet-org/arpc/pkg/serializer"
)

var kvClient kv.KVServiceClient

func handler(w http.ResponseWriter, r *http.Request) {
	operation := r.URL.Query().Get("operation")
	key := r.URL.Query().Get("key")
	value := r.URL.Query().Get("value")
	log.Printf("Received HTTP request with operation: %s, key: %s, value: %s\n", operation, key, value)

	switch operation {
	case "get":
		if key == "" {
			http.Error(w, "Key parameter is required for get operation", http.StatusBadRequest)
			return
		}

		req := &kv.GetRequest{
			Key: key,
		}
		resp, err := kvClient.Get(context.Background(), req)
		if err != nil {
			http.Error(w, fmt.Sprintf("Get RPC call failed: %v", err), http.StatusInternalServerError)
			return
		}

		log.Printf("Got Get response: %s\n", resp.Value)
		fmt.Fprintf(w, "Value for key '%s': %s\n", key, resp.Value)

	case "set":
		if key == "" || value == "" {
			http.Error(w, "Key and value parameters are required for set operation", http.StatusBadRequest)
			return
		}

		req := &kv.SetRequest{
			Key:   key,
			Value: value,
		}
		resp, err := kvClient.Set(context.Background(), req)
		if err != nil {
			http.Error(w, fmt.Sprintf("Set RPC call failed: %v", err), http.StatusInternalServerError)
			return
		}

		log.Printf("Got Set response: %s\n", resp.Value)
		fmt.Fprintf(w, "Set key '%s' to value '%s', response: %s\n", key, value, resp.Value)

	default:
		http.Error(w, "Invalid operation. Use 'get' or 'set'", http.StatusBadRequest)
		return
	}
}

func main() {
	// Create RPC client
	serializer := &serializer.SymphonySerializer{}

	client, err := rpc.NewClient(serializer, "server.default.svc.cluster.local:11000", nil) // TODO: change to your server's address fully qualified domain name
	if err != nil {
		log.Fatal("Failed to create RPC client:", err)
	}

	// Create KVService client
	kvClient = kv.NewKVServiceClient(client)

	// Set up HTTP server
	http.HandleFunc("/", handler)
	log.Println("HTTP server listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("HTTP server failed:", err)
	}
}
