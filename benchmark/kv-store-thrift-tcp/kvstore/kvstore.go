package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"sync"

	thrift "github.com/apache/thrift/lib/go/thrift"
	"go.uber.org/zap"

	kv "github.com/appnet-org/arpc/benchmark/kv-store-thrift-tcp/gen-go/kv"
)

var logger *zap.Logger

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

// KVService implementation
type kvServer struct {
	mu          sync.RWMutex
	data        map[string]string
	maxSize     int
	accessOrder []string // For LRU eviction
}

func NewKVServer(maxSize int) *kvServer {
	if maxSize <= 0 {
		maxSize = 1000 // Default max size
	}
	return &kvServer{
		data:        make(map[string]string),
		maxSize:     maxSize,
		accessOrder: make([]string, 0, maxSize),
	}
}

func (s *kvServer) Get(ctx context.Context, req *kv.GetRequest) (*kv.GetResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := req.GetKey()
	logger.Debug("Server got Get request", zap.String("key", key))

	value, exists := s.data[key]
	if !exists {
		value = "" // Return empty string if key doesn't exist
	} else {
		// Move to end of access order for LRU
		s.moveToEnd(key)
	}

	resp := &kv.GetResponse{
		Value: value,
	}

	logger.Debug("Server returning value for key", zap.String("key", key), zap.String("value", value))
	return resp, nil
}

func (s *kvServer) SetValue(ctx context.Context, req *kv.SetRequest) (*kv.SetResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := req.GetKey()
	value := req.GetValue()
	logger.Debug("Server got SetValue request", zap.String("key", key), zap.String("value", value))

	// Check if we need to evict an item
	if len(s.data) >= s.maxSize {
		if _, exists := s.data[key]; !exists {
			s.evictLRU()
		}
	}

	s.data[key] = value
	s.moveToEnd(key)

	resp := &kv.SetResponse{
		Value: value,
	}

	logger.Debug("Server set key to value", zap.String("key", key), zap.String("value", value))
	return resp, nil
}

// moveToEnd moves the key to the end of the access order (most recently used)
func (s *kvServer) moveToEnd(key string) {
	// Remove from current position if it exists
	for i, k := range s.accessOrder {
		if k == key {
			s.accessOrder = append(s.accessOrder[:i], s.accessOrder[i+1:]...)
			break
		}
	}
	// Add to end
	s.accessOrder = append(s.accessOrder, key)
}

// evictLRU removes the least recently used item
func (s *kvServer) evictLRU() {
	if len(s.accessOrder) == 0 {
		return
	}

	// Remove the first (oldest) item
	keyToRemove := s.accessOrder[0]
	s.accessOrder = s.accessOrder[1:]
	delete(s.data, keyToRemove)

	logger.Debug("Evicted LRU key", zap.String("key", keyToRemove))
}

func main() {
	// Initialize zap logger with configuration from environment variables
	config := getLoggingConfig()
	var err error
	logger, err = config.Build()
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Sync()

	// Create listener
	transport, err := thrift.NewTServerSocket(":11000")
	if err != nil {
		log.Fatalf("Failed to create server socket: %v", err)
	}

	// Create KV server with max size constraint (configurable via environment variable)
	maxSize := 1000 // Default max size
	if maxSizeEnv := os.Getenv("KV_MAX_SIZE"); maxSizeEnv != "" {
		if parsed, err := strconv.Atoi(maxSizeEnv); err == nil && parsed > 0 {
			maxSize = parsed
		}
	}

	kvServer := NewKVServer(maxSize)
	processor := kv.NewKVServiceProcessor(kvServer)

	// Use binary protocol and framed transport
	transportFactory := thrift.NewTBufferedTransportFactory(8192)
	protocolFactory := thrift.NewTBinaryProtocolFactoryConf(nil)

	server := thrift.NewTSimpleServer4(
		processor,
		transport,
		transportFactory,
		protocolFactory,
	)

	logger.Info("KV server starting", zap.String("port", "11000"), zap.Int("maxSize", maxSize))

	// Start server
	if err := server.Serve(); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
