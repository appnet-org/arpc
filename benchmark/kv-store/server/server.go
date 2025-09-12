package main

import (
	"context"
	"log"
	"sync"

	kv "github.com/appnet-org/arpc/benchmark/kv-store/symphony"
	"github.com/appnet-org/arpc/pkg/rpc"
	"github.com/appnet-org/arpc/pkg/serializer"
)

// KVService implementation
type kvServer struct {
	mu   sync.RWMutex
	data map[string]string
}

func NewKVServer() *kvServer {
	return &kvServer{
		data: make(map[string]string),
	}
}

func (s *kvServer) Get(ctx context.Context, req *kv.GetRequest) (*kv.GetResponse, context.Context, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := req.GetKey()
	log.Printf("Server got Get request for key: [%s]", key)

	value, exists := s.data[key]
	if !exists {
		value = "" // Return empty string if key doesn't exist
	}

	resp := &kv.GetResponse{
		Value: value,
	}

	log.Printf("Server returning value: [%s] for key: [%s]", value, key)
	return resp, context.Background(), nil
}

func (s *kvServer) Set(ctx context.Context, req *kv.SetRequest) (*kv.SetResponse, context.Context, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := req.GetKey()
	value := req.GetValue()
	log.Printf("Server got Set request for key: [%s], value: [%s]", key, value)

	s.data[key] = value

	resp := &kv.SetResponse{
		Value: value,
	}

	log.Printf("Server set key: [%s] to value: [%s]", key, value)
	return resp, context.Background(), nil
}

func main() {
	serializer := &serializer.SymphonySerializer{}
	server, err := rpc.NewServer(":11000", serializer, nil)
	if err != nil {
		log.Fatal("Failed to start server:", err)
	}

	kvServer := NewKVServer()
	kv.RegisterKVServiceServer(server, kvServer)
	server.Start()
}
