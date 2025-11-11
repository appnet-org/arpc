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
	"time"

	thrift "github.com/apache/thrift/lib/go/thrift"
	"go.uber.org/zap"

	kv "github.com/appnet-org/arpc/benchmark/kv-store-thrift-http/gen-go/kv"
)

// global logger only
var logger *zap.Logger

// generateDeterministicString produces a fixed pseudo-random string based on keyID and desired length.
func generateDeterministicString(keyID string, length int) string {
	hash := sha256.Sum256([]byte(keyID))
	repeatCount := (length + len(hash)*2 - 1) / (len(hash) * 2)
	hexStr := strings.Repeat(hex.EncodeToString(hash[:]), repeatCount)
	return hexStr[:length]
}

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

	config := zap.NewProductionConfig()

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

	logger.Debug("Received HTTP request",
		zap.String("op", op),
		zap.String("key_id", keyID),
		zap.Int("key_size", keySize),
		zap.Int("value_size", valueSize),
	)

	// Create Thrift HTTP transport
	transport, err := thrift.NewTHttpClient("http://kvstore.default.svc.cluster.local:11000")
	if err != nil {
		logger.Error("Failed to create HTTP transport", zap.Error(err))
		http.Error(w, "Failed to create HTTP transport: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Open the transport (for HTTP this is a no-op, but required for interface compatibility)
	if err := transport.Open(); err != nil {
		logger.Error("Failed to open transport", zap.Error(err))
		http.Error(w, "Failed to open transport: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer transport.Close()

	// Create protocol factory and client
	protocolFactory := thrift.NewTBinaryProtocolFactoryConf(nil)
	client := kv.NewKVServiceClientFactory(transport, protocolFactory)

	// Set timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	switch op {
	case "get":
		req := &kv.GetRequest{Key: keyStr}
		resp, err := client.Get(ctx, req)
		if err != nil {
			logger.Error("Get Thrift call failed", zap.Error(err))
			http.Error(w, "Get Thrift call failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "Value for key_id '%s' (key='%s'): %s\n", keyID, keyStr, resp.Value)

	case "set":
		req := &kv.SetRequest{Key: keyStr, Value: valueStr}
		resp, err := client.SetValue(ctx, req)
		if err != nil {
			logger.Error("SetValue Thrift call failed", zap.Error(err))
			http.Error(w, "SetValue Thrift call failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "Set key_id '%s' (key='%s') to value='%s'. Response: %s\n",
			keyID, keyStr, valueStr, resp.Value)

	default:
		http.Error(w, "Invalid operation. Use op=GET or op=SET", http.StatusBadRequest)
	}
}

func main() {
	// Initialize zap logger
	config := getLoggingConfig()
	var err error
	logger, err = config.Build()
	if err != nil {
		panic("Failed to initialize logger: " + err.Error())
	}
	defer logger.Sync()

	http.HandleFunc("/", handler)
	logger.Info("HTTP server listening", zap.String("port", "8080"))
	if err := http.ListenAndServe(":8080", nil); err != nil {
		logger.Fatal("HTTP server failed", zap.Error(err))
	}
}
