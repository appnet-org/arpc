package main

import (
	"context"
	"log"
	"net"
	"os"
	"strconv"
	"sync"

	"github.com/appnet-org/arpc/pkg/logging"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	kv "github.com/appnet-org/arpc/benchmark/kv-store-grpc/proto"
)

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

// KVService implementation
type kvServer struct {
	kv.UnimplementedKVServiceServer
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
	logging.Debug("Server got Get request", zap.String("key", key))

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

	logging.Debug("Server returning value for key", zap.String("key", key), zap.String("value", value))
	return resp, nil
}

func (s *kvServer) Set(ctx context.Context, req *kv.SetRequest) (*kv.SetResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := req.GetKey()
	value := req.GetValue()
	logging.Debug("Server got Set request", zap.String("key", key), zap.String("value", value))

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

	logging.Debug("Server set key to value", zap.String("key", key), zap.String("value", value))
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

	logging.Debug("Evicted LRU key", zap.String("key", keyToRemove))
}

func main() {
	// Initialize logging
	config := getLoggingConfig()
	err := logging.Init(config)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logging.Sync()

	// Create listener
	lis, err := net.Listen("tcp", ":11000")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// Create gRPC server
	s := grpc.NewServer()

	// Create KV server with max size constraint (configurable via environment variable)
	maxSize := 1000 // Default max size
	if maxSizeEnv := os.Getenv("KV_MAX_SIZE"); maxSizeEnv != "" {
		if parsed, err := strconv.Atoi(maxSizeEnv); err == nil && parsed > 0 {
			maxSize = parsed
		}
	}

	kvServer := NewKVServer(maxSize)
	kv.RegisterKVServiceServer(s, kvServer)

	logging.Info("KV server starting", zap.String("port", "11000"), zap.Int("maxSize", maxSize))

	// Start server
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
