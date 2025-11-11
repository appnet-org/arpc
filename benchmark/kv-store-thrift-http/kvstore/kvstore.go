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
	reqBody  *bytes.Buffer
	respBody *bytes.Buffer
	closed   bool
}

func newHTTPTransport(reqBodyData []byte) *httpTransport {
	return &httpTransport{
		reqBody:  bytes.NewBuffer(reqBodyData),
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
	return nil
}

func (t *httpTransport) Flush(ctx context.Context) error {
	return nil
}

func (t *httpTransport) RemainingBytes() uint64 {
	if t.closed || t.reqBody == nil {
		return 0
	}
	// Return the number of bytes remaining in the request buffer
	return uint64(t.reqBody.Len())
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

		// Read the entire request body into a buffer first
		// This is necessary because HTTP request bodies can only be read once
		reqBodyData, err := io.ReadAll(r.Body)
		if err != nil {
			logger.Error("Failed to read request body", zap.Error(err))
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		logger.Debug("Received Thrift HTTP request",
			zap.Int("body_size", len(reqBodyData)),
			zap.String("content_type", r.Header.Get("Content-Type")),
			zap.String("url", r.URL.String()))

		// Create HTTP transport from buffered request data
		transport := newHTTPTransport(reqBodyData)
		defer transport.Close()

		// Create input and output protocols
		iprot := protocolFactory.GetProtocol(transport)
		oprot := protocolFactory.GetProtocol(transport)

		// Process the request
		ctx := r.Context()
		success, err := processor.Process(ctx, iprot, oprot)

		// Flush the output protocol to ensure all data is written to the buffer
		if flushable, ok := oprot.(interface{ Flush(context.Context) error }); ok {
			if flushErr := flushable.Flush(ctx); flushErr != nil {
				logger.Warn("Failed to flush output protocol", zap.Error(flushErr))
			}
		}

		// If processing failed at the protocol level (success == false), return HTTP 500
		if !success {
			logger.Error("Failed to process Thrift request", zap.Error(err))
			// If there's any response data written, try to send it (Thrift may have written an exception)
			// Otherwise, send a generic error
			if transport.respBody.Len() > 0 {
				w.WriteHeader(http.StatusOK)
				if _, writeErr := w.Write(transport.respBody.Bytes()); writeErr != nil {
					logger.Error("Failed to write error response", zap.Error(writeErr))
				}
			} else {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}

		// If success == true, Thrift has written a response (success or exception) to the output protocol
		// Write it to the HTTP response. In Thrift over HTTP, both success and exceptions use HTTP 200.
		// The Thrift protocol itself encodes whether it's a success or exception response.
		if err != nil {
			logger.Debug("Thrift request processed with application error (exception written to response)", zap.Error(err))
		}

		// Write the response body (Thrift protocol response)
		w.WriteHeader(http.StatusOK)
		responseData := transport.respBody.Bytes()
		if len(responseData) == 0 {
			logger.Warn("Thrift processor returned success but response body is empty")
		}
		if _, err := w.Write(responseData); err != nil {
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
