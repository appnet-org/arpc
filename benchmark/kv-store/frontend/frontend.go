package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	kv "github.com/appnet-org/arpc/benchmark/kv-store/symphony"
	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/rpc"
	"github.com/appnet-org/arpc/pkg/serializer"
	"go.uber.org/zap"
)

var kvClient kv.KVServiceClient

// getLoggingConfig reads logging configuration from environment variables with defaults
func getLoggingConfig() *logging.Config {
	level := os.Getenv("LOG_LEVEL")
	if level == "" {
		level = "debug"
	}

	format := os.Getenv("LOG_FORMAT")
	if format == "" {
		format = "console"
	}

	return &logging.Config{
		Level:  level,
		Format: format,
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	operation := r.URL.Query().Get("operation")
	key := r.URL.Query().Get("key")
	value := r.URL.Query().Get("value")
	logging.Debug("Received HTTP request",
		zap.String("operation", operation),
		zap.String("key", key),
		zap.String("value", value),
	)

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
			if rpcErr, ok := err.(*rpc.RPCError); !ok || rpcErr.Type == rpc.RPCUnknownError {
				logging.Error("Get RPC call failed", zap.Error(err))
			}
			http.Error(w, fmt.Sprintf("Get RPC call failed: %v", err), http.StatusInternalServerError)
			return
		}

		logging.Debug("Got Get response", zap.String("value", resp.Value))
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
			if rpcErr, ok := err.(*rpc.RPCError); !ok || rpcErr.Type == rpc.RPCUnknownError {
				logging.Error("Set RPC call failed", zap.Error(err))
			}
			http.Error(w, fmt.Sprintf("Set RPC call failed: %v", err), http.StatusInternalServerError)
			return
		}

		logging.Debug("Got Set response", zap.String("value", resp.Value))
		fmt.Fprintf(w, "Set key '%s' to value '%s', response: %s\n", key, value, resp.Value)

	default:
		http.Error(w, "Invalid operation. Use 'get' or 'set'", http.StatusBadRequest)
		return
	}
}

func main() {
	// Initialize logging with configuration from environment variables
	err := logging.Init(getLoggingConfig())
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize logging: %v", err))
	}

	// Create RPC client
	serializer := &serializer.SymphonySerializer{}

	client, err := rpc.NewClient(serializer, ":11000", nil) // TODO: change to your server's address fully qualified domain name
	if err != nil {
		logging.Fatal("Failed to create RPC client", zap.Error(err))
	}

	// Create KVService client
	kvClient = kv.NewKVServiceClient(client)

	// Set up HTTP server
	http.HandleFunc("/", handler)
	logging.Info("HTTP server listening", zap.String("addr", ":8080"))
	if err := http.ListenAndServe(":8080", nil); err != nil {
		logging.Fatal("HTTP server failed", zap.Error(err))
	}
}
