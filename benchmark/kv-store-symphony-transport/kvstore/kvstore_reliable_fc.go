package main

import (
	"context"
	"os"
	"strconv"
	"sync"

	kv "github.com/appnet-org/arpc/benchmark/kv-store-symphony-transport/symphony"
	"github.com/appnet-org/arpc/pkg/custom/flowcontrol"
	"github.com/appnet-org/arpc/pkg/custom/reliable"
	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/packet"
	"github.com/appnet-org/arpc/pkg/rpc"
	"github.com/appnet-org/arpc/pkg/serializer"
	"github.com/appnet-org/arpc/pkg/transport"
	"go.uber.org/zap"
)

// KVService implementation
type kvServerReliableFC struct {
	mu          sync.RWMutex
	data        map[string]string
	maxSize     int
	accessOrder []string // For LRU eviction
}

func NewKVServerReliableFC(maxSize int) *kvServerReliableFC {
	if maxSize <= 0 {
		maxSize = 1000 // Default max size
	}
	return &kvServerReliableFC{
		data:        make(map[string]string),
		maxSize:     maxSize,
		accessOrder: make([]string, 0, maxSize),
	}
}

func (s *kvServerReliableFC) Get(ctx context.Context, req *kv.GetRequest) (*kv.GetResponse, context.Context, error) {
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

func (s *kvServerReliableFC) Set(ctx context.Context, req *kv.SetRequest) (*kv.SetResponse, context.Context, error) {
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
func (s *kvServerReliableFC) moveToEnd(key string) {
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
func (s *kvServerReliableFC) evictLRU() {
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
func getLoggingConfigReliableFC() *logging.Config {
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
	if err := logging.Init(getLoggingConfigReliableFC()); err != nil {
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

	// Register ACK packet type
	ackPacketType, err := udpTransport.RegisterPacketType(reliable.AckPacketName, &reliable.ACKPacketCodec{})
	if err != nil {
		logging.Fatal("Failed to register ACK packet type", zap.Error(err))
	}

	// Register FCFeedback packet type
	fcFeedbackPacketType, err := udpTransport.RegisterPacketType(flowcontrol.FCFeedbackPacketName, &flowcontrol.FCFeedbackCodec{})
	if err != nil {
		logging.Fatal("Failed to register FCFeedback packet type", zap.Error(err))
	}

	// Create reliable server handler
	reliableHandler := reliable.NewReliableServerHandler(
		udpTransport,
		udpTransport.GetTimerManager(),
	)
	defer reliableHandler.Cleanup()

	// Create flow control server handler
	fcHandler := flowcontrol.NewFCServerHandler(
		udpTransport,
		udpTransport.GetTimerManager(),
	)
	defer fcHandler.Cleanup()

	// Register for REQUEST packets (OnReceive)
	// Both handlers need to process REQUEST packets
	requestChain, exists := udpTransport.GetHandlerRegistry().GetHandlerChain(
		packet.PacketTypeRequest.TypeID,
		transport.RoleServer,
	)
	if !exists {
		logging.Fatal("Failed to get REQUEST handler chain")
	}
	// Add reliable handler first (for ACK), then FC handler (for feedback)
	requestChain.AddHandler(reliableHandler)
	requestChain.AddHandler(fcHandler)

	// Register for RESPONSE packets (OnSend)
	// Both handlers need to process RESPONSE packets
	responseChain, exists := udpTransport.GetHandlerRegistry().GetHandlerChain(
		packet.PacketTypeResponse.TypeID,
		transport.RoleServer,
	)
	if !exists {
		logging.Fatal("Failed to get RESPONSE handler chain")
	}
	// Add FC handler first (for flow control checks), then reliable handler
	responseChain.AddHandler(fcHandler)
	responseChain.AddHandler(reliableHandler)

	// Register handler chain for ACK packets
	// ACK packets use the same handler chain for both send and receive on server
	ackChain := transport.NewHandlerChain("ServerACKHandlerChain", reliableHandler)
	udpTransport.RegisterHandlerChain(ackPacketType.TypeID, ackChain, transport.RoleServer)

	// Register handler chain for FCFeedback packets
	// FCFeedback packets use the same handler chain for both send and receive on server
	fcFeedbackChain := transport.NewHandlerChain("ServerFCFeedbackHandlerChain", fcHandler)
	udpTransport.RegisterHandlerChain(fcFeedbackPacketType.TypeID, fcFeedbackChain, transport.RoleServer)

	// Create KV server with max size constraint (configurable via environment variable)
	maxSize := 1000 // Default max size
	if maxSizeEnv := os.Getenv("KV_MAX_SIZE"); maxSizeEnv != "" {
		if parsed, err := strconv.Atoi(maxSizeEnv); err == nil && parsed > 0 {
			maxSize = parsed
		}
	}

	kvServer := NewKVServerReliableFC(maxSize)
	kv.RegisterKVServiceServer(server, kvServer)

	logging.Info("Reliable + flow control KV server starting", zap.String("addr", ":11000"))
	server.Start()
}
