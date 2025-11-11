package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"

	thrift "github.com/apache/thrift/lib/go/thrift"
	"go.uber.org/zap"

	kv "github.com/appnet-org/arpc/benchmark/kv-store-thrift-http/gen-go/kv"
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

// httpTransport wraps HTTP request/response to implement TTransport
type httpTransport struct {
	reqBody  io.Reader
	respBody *bytes.Buffer
	closed   bool
}

func newHTTPTransport(r *http.Request) *httpTransport {
	return &httpTransport{
		reqBody:  r.Body,
		respBody: &bytes.Buffer{},
		closed:   false,
	}
}

func (t *httpTransport) Read(buf []byte) (int, error) {
	if t.closed {
		return 0, io.EOF
	}
	return t.reqBody.Read(buf)
}

func (t *httpTransport) ReadByte() (byte, error) {
	if t.closed {
		return 0, io.EOF
	}
	buf := make([]byte, 1)
	n, err := t.reqBody.Read(buf)
	if n == 0 {
		return 0, err
	}
	return buf[0], err
}

func (t *httpTransport) Write(buf []byte) (int, error) {
	if t.closed {
		return 0, io.ErrClosedPipe
	}
	return t.respBody.Write(buf)
}

func (t *httpTransport) WriteByte(c byte) error {
	if t.closed {
		return io.ErrClosedPipe
	}
	return t.respBody.WriteByte(c)
}

func (t *httpTransport) WriteString(s string) (int, error) {
	if t.closed {
		return 0, io.ErrClosedPipe
	}
	return t.respBody.WriteString(s)
}

func (t *httpTransport) Close() error {
	if t.closed {
		return nil
	}
	t.closed = true
	if closer, ok := t.reqBody.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func (t *httpTransport) Flush(ctx context.Context) error {
	return nil
}

func (t *httpTransport) RemainingBytes() uint64 {
	const maxSize = ^uint64(0)
	return maxSize
}

func (t *httpTransport) Open() error {
	return nil
}

func (t *httpTransport) IsOpen() bool {
	return !t.closed
}

// thriftHTTPHandler creates an HTTP handler for Thrift requests
func thriftHTTPHandler(processor thrift.TProcessor, protocolFactory thrift.TProtocolFactory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only handle POST requests (Thrift HTTP uses POST)
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Set content type for Thrift binary protocol
		w.Header().Set("Content-Type", "application/x-thrift")

		// Create HTTP transport from request
		transport := newHTTPTransport(r)
		defer transport.Close()

		// Create input and output protocols
		iprot := protocolFactory.GetProtocol(transport)
		oprot := protocolFactory.GetProtocol(transport)

		// Process the request
		ctx := r.Context()
		_, err := processor.Process(ctx, iprot, oprot)
		if err != nil {
			logger.Error("Failed to process Thrift request", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Write response body to HTTP response
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(transport.respBody.Bytes()); err != nil {
			logger.Error("Failed to write response", zap.Error(err))
		}
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

	// Create KV server with max size constraint (configurable via environment variable)
	maxSize := 1000 // Default max size
	if maxSizeEnv := os.Getenv("KV_MAX_SIZE"); maxSizeEnv != "" {
		if parsed, err := strconv.Atoi(maxSizeEnv); err == nil && parsed > 0 {
			maxSize = parsed
		}
	}

	kvServer := NewKVServer(maxSize)
	processor := kv.NewKVServiceProcessor(kvServer)

	// Create protocol factory
	protocolFactory := thrift.NewTBinaryProtocolFactoryConf(nil)

	// Create Thrift HTTP handler
	handler := thriftHTTPHandler(processor, protocolFactory)

	// Set up HTTP server
	http.Handle("/", handler)

	logger.Info("KV HTTP server starting", zap.String("port", "11000"), zap.Int("maxSize", maxSize))

	// Start HTTP server
	if err := http.ListenAndServe(":11000", nil); err != nil {
		logger.Fatal("Failed to serve", zap.Error(err))
	}
}
