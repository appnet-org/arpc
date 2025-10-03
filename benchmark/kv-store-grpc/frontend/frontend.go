package main

import (
	"context"
	"net/http"
	"os"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	kv "github.com/appnet-org/arpc/benchmark/kv-store-grpc/proto"
)

var (
	kvClient kv.KVServiceClient
	logger   *zap.Logger
)

// getLoggingConfig reads logging configuration from environment variables with defaults
func getLoggingConfig() zap.Config {
	level := os.Getenv("LOG_LEVEL")
	if level == "" {
		level = "info"
	}

	format := os.Getenv("LOG_FORMAT")
	if format == "" {
		format = "console"
	}

	// Create base config
	config := zap.NewProductionConfig()

	// Set log level
	switch level {
	case "debug":
		config.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		config.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		config.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	// Set output format
	if format == "console" {
		config.Development = true
		config.Encoding = "console"
		config.EncoderConfig.TimeKey = ""
		config.EncoderConfig.CallerKey = ""
	} else {
		config.Encoding = "json"
	}

	return config
}

func handler(w http.ResponseWriter, r *http.Request) {
	operation := r.URL.Query().Get("operation")
	key := r.URL.Query().Get("key")
	value := r.URL.Query().Get("value")

	logger.Debug("Received HTTP request",
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
			logger.Error("Get gRPC call failed", zap.Error(err))
			http.Error(w, "Get gRPC call failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		logger.Debug("Got Get response", zap.String("value", resp.Value))
		http.ResponseWriter(w).Write([]byte("Value for key '" + key + "': " + resp.Value + "\n"))

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
			logger.Error("Set gRPC call failed", zap.Error(err))
			http.Error(w, "Set gRPC call failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		logger.Debug("Got Set response", zap.String("value", resp.Value))
		http.ResponseWriter(w).Write([]byte("Set key '" + key + "' to value '" + value + "', response: " + resp.Value + "\n"))

	default:
		http.Error(w, "Invalid operation. Use 'get' or 'set'", http.StatusBadRequest)
		return
	}
}

func main() {
	// Initialize zap logger with configuration from environment variables
	config := getLoggingConfig()
	var err error
	logger, err = config.Build()
	if err != nil {
		panic("Failed to initialize logger: " + err.Error())
	}
	defer logger.Sync()

	// Create gRPC client connection
	conn, err := grpc.NewClient(
		"kvstore.default.svc.cluster.local:11000",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		logger.Fatal("Failed to create gRPC client", zap.Error(err))
	}
	defer conn.Close()

	// Create KVService client
	kvClient = kv.NewKVServiceClient(conn)

	// Set up HTTP server
	http.HandleFunc("/", handler)
	logger.Info("HTTP server listening", zap.String("port", "8080"))
	if err := http.ListenAndServe(":8080", nil); err != nil {
		logger.Fatal("HTTP server failed", zap.Error(err))
	}
}
