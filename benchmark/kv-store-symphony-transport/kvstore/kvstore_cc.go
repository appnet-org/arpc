package main

import (
	"context"
	"os"
	"strconv"
	"sync"

	kv "github.com/appnet-org/arpc/benchmark/kv-store-symphony/symphony"
	"github.com/appnet-org/arpc/pkg/custom/congestion"
	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/packet"
	"github.com/appnet-org/arpc/pkg/rpc"
	"github.com/appnet-org/arpc/pkg/serializer"
	"github.com/appnet-org/arpc/pkg/transport"
	"go.uber.org/zap"
)

// KVService implementation
type kvServerCC struct {
	mu          sync.RWMutex
	data        map[string]string
	maxSize     int
	accessOrder []string // For LRU eviction
}

func NewKVServerCC(maxSize int) *kvServerCC {
	if maxSize <= 0 {
		maxSize = 1000 // Default max size
	}
	return &kvServerCC{
		data:        make(map[string]string),
		maxSize:     maxSize,
		accessOrder: make([]string, 0, maxSize),
	}
}

func (s *kvServerCC) Get(ctx context.Context, req *kv.GetRequest) (*kv.GetResponse, context.Context, error) {
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
	return resp, context.Background(), nil
}

func (s *kvServerCC) Set(ctx context.Context, req *kv.SetRequest) (*kv.SetResponse, context.Context, error) {
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
	return resp, context.Background(), nil
}

// moveToEnd moves the key to the end of the access order (most recently used)
func (s *kvServerCC) moveToEnd(key string) {
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
func (s *kvServerCC) evictLRU() {
	if len(s.accessOrder) == 0 {
		return
	}

	// Remove the first (oldest) item
	keyToRemove := s.accessOrder[0]
	s.accessOrder = s.accessOrder[1:]
	delete(s.data, keyToRemove)

	logging.Debug("Evicted LRU key", zap.String("key", keyToRemove))
}

// getLoggingConfig reads logging configuration from environment variables with defaults
func getLoggingConfigCC() *logging.Config {
	level := os.Getenv("LOG_LEVEL")
	if level == "" {
		level = "info"
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

func main() {
	// Initialize logging with configuration from environment variables
	if err := logging.Init(getLoggingConfigCC()); err != nil {
		panic(err)
	}

	// Create RPC server (creates UDP transport internally)
	serializer := &serializer.SymphonySerializer{}
	server, err := rpc.NewServer(":11000", serializer, nil)
	if err != nil {
		logging.Fatal("Failed to start server", zap.Error(err))
	}

	// Get the UDP transport from the server
	udpTransport := server.GetTransport()
	defer udpTransport.Close()

	// Register CCFeedback packet type
	ccFeedbackPacketType, err := udpTransport.RegisterPacketType(congestion.CCFeedbackPacketName, &congestion.CCFeedbackCodec{})
	if err != nil {
		logging.Fatal("Failed to register CCFeedback packet type", zap.Error(err))
	}

	// Create congestion control server handler
	serverHandler := congestion.NewCCServerHandler(
		udpTransport,
		udpTransport.GetTimerManager(),
	)
	defer serverHandler.Cleanup()

	// Register for REQUEST packets (OnReceive)
	requestChain, exists := udpTransport.GetHandlerRegistry().GetHandlerChain(
		packet.PacketTypeRequest.TypeID,
		transport.RoleServer,
	)
	if !exists {
		logging.Fatal("Failed to get REQUEST handler chain")
	}
	requestChain.AddHandler(serverHandler)

	// Register for RESPONSE packets (OnSend)
	responseChain, exists := udpTransport.GetHandlerRegistry().GetHandlerChain(
		packet.PacketTypeResponse.TypeID,
		transport.RoleServer,
	)
	if !exists {
		logging.Fatal("Failed to get RESPONSE handler chain")
	}
	responseChain.AddHandler(serverHandler)

	// Register handler chain for CCFeedback packets
	// CCFeedback packets use the same handler chain for both send and receive on server
	ccFeedbackChain := transport.NewHandlerChain("ServerCCFeedbackHandlerChain", serverHandler)
	udpTransport.RegisterHandlerChain(ccFeedbackPacketType.TypeID, ccFeedbackChain, transport.RoleServer)

	// Create KV server with max size constraint (configurable via environment variable)
	maxSize := 1000 // Default max size
	if maxSizeEnv := os.Getenv("KV_MAX_SIZE"); maxSizeEnv != "" {
		if parsed, err := strconv.Atoi(maxSizeEnv); err == nil && parsed > 0 {
			maxSize = parsed
		}
	}

	kvServer := NewKVServerCC(maxSize)
	kv.RegisterKVServiceServer(server, kvServer)

	logging.Info("Congestion control KV server starting", zap.String("addr", ":11000"))
	server.Start()
}
