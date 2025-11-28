package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/appnet-org/arpc/pkg/logging"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	kv "github.com/appnet-org/arpc/benchmark/kv-store-grpc/proto"
)

// const serverAddress = "130.127.134.16:11000"
const serverAddress = "kvstore.default.svc.cluster.local:11000"

// generateDeterministicString produces a fixed pseudo-random string based on keyID and desired length.
func generateDeterministicString(keyID string, length int) string {
	hash := sha256.Sum256([]byte(keyID))
	repeatCount := (length + len(hash)*2 - 1) / (len(hash) * 2)
	hexStr := strings.Repeat(hex.EncodeToString(hash[:]), repeatCount)
	return hexStr[:length]
}

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
	op := strings.ToLower(r.URL.Query().Get("op"))
	keyID := r.URL.Query().Get("key")
	keySizeStr := r.URL.Query().Get("key_size")
	valueSizeStr := r.URL.Query().Get("value_size")

	keySize, _ := strconv.Atoi(keySizeStr)
	valueSize, _ := strconv.Atoi(valueSizeStr)

	if keyID == "" {
		http.Error(w, "key parameter is required", http.StatusBadRequest)
		return
	}

	// Deterministic key/value generation
	keyStr := generateDeterministicString(keyID+"-key", keySize)
	valueStr := generateDeterministicString(keyID+"-value", valueSize)

	logging.Debug("Received HTTP request",
		zap.String("op", op),
		zap.String("key_id", keyID),
		zap.Int("key_size", keySize),
		zap.Int("value_size", valueSize),
	)

	conn, err := grpc.NewClient(
		serverAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		logging.Error("Failed to dial gRPC server", zap.Error(err))
		http.Error(w, "Failed to dial gRPC server: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	kvClient := kv.NewKVServiceClient(conn)

	switch op {
	case "get":
		req := &kv.GetRequest{Key: keyStr}
		resp, err := kvClient.Get(context.Background(), req)
		if err != nil {
			logging.Error("Get gRPC call failed", zap.Error(err))
			http.Error(w, "Get gRPC call failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "Value for key_id '%s' (key='%s'): %s\n", keyID, keyStr, resp.Value)

	case "set":
		req := &kv.SetRequest{Key: keyStr, Value: valueStr}
		resp, err := kvClient.Set(context.Background(), req)
		if err != nil {
			logging.Error("Set gRPC call failed", zap.Error(err))
			http.Error(w, "Set gRPC call failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "Set key_id '%s' (key='%s') to value='%s'. Response: %s\n",
			keyID, keyStr, valueStr, resp.Value)

	default:
		http.Error(w, "Invalid operation. Use op=GET or op=SET", http.StatusBadRequest)
	}
}

func main() {
	// Initialize logging
	config := getLoggingConfig()
	err := logging.Init(config)
	if err != nil {
		panic("Failed to initialize logger: " + err.Error())
	}
	defer logging.Sync()

	http.HandleFunc("/", handler)
	logging.Info("HTTP server listening", zap.String("port", "8080"))
	if err := http.ListenAndServe(":8080", nil); err != nil {
		logging.Fatal("HTTP server failed", zap.Error(err))
	}
}
