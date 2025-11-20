package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/proxy-h2/element"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
	"golang.org/x/sys/unix"
)

const (
	// DefaultBufferSize is the size of the buffer used for reading data
	DefaultBufferSize = 4096
	// HTTP2Preface is the HTTP/2 connection preface
	HTTP2Preface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"
)

// ProxyState manages the state of the TCP proxy
type ProxyState struct {
	elementChain   *element.RPCElementChain
	bufferManager  *StreamBufferManager
	headerManager  *HeaderManager
	payloadManager *StreamPayloadManager
	// Target server address for proxying (optional, can be configured via env)
	// Used only if SO_ORIGINAL_DST is unavailable
	targetAddr string
}

// Config holds the proxy configuration
type Config struct {
	Ports      []int
	TargetAddr string
}

// StreamBufferKey uniquely identifies a stream buffer
// Uses the connection pointer (memory address) and stream ID
type StreamBufferKey struct {
	Conn     net.Conn // Connection pointer acts as unique identifier
	StreamID uint32
}

// StreamBufferManager manages per-stream byte buffers for deferred frame writing
type StreamBufferManager struct {
	buffers map[StreamBufferKey]*bytes.Buffer
	mu      sync.RWMutex
}

// StreamPayloadManager manages per-stream payload accumulation for element chain processing
type StreamPayloadManager struct {
	payloads map[StreamBufferKey][]byte // Accumulated payload per stream
	mu       sync.RWMutex
}

// NewStreamPayloadManager creates a new StreamPayloadManager
func NewStreamPayloadManager() *StreamPayloadManager {
	return &StreamPayloadManager{
		payloads: make(map[StreamBufferKey][]byte),
	}
}

// AppendPayload appends data to the payload for a stream
func (m *StreamPayloadManager) AppendPayload(conn net.Conn, streamID uint32, data []byte) {
	key := StreamBufferKey{Conn: conn, StreamID: streamID}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.payloads[key] = append(m.payloads[key], data...)
}

// GetPayload returns the accumulated payload for a stream
func (m *StreamPayloadManager) GetPayload(conn net.Conn, streamID uint32) ([]byte, bool) {
	key := StreamBufferKey{Conn: conn, StreamID: streamID}
	m.mu.RLock()
	defer m.mu.RUnlock()
	payload, exists := m.payloads[key]
	if !exists {
		return nil, false
	}
	// Return a copy
	payloadCopy := make([]byte, len(payload))
	copy(payloadCopy, payload)
	return payloadCopy, true
}

// SetPayload sets the payload for a stream (used after element chain processing)
func (m *StreamPayloadManager) SetPayload(conn net.Conn, streamID uint32, payload []byte) {
	key := StreamBufferKey{Conn: conn, StreamID: streamID}
	m.mu.Lock()
	defer m.mu.Unlock()
	payloadCopy := make([]byte, len(payload))
	copy(payloadCopy, payload)
	m.payloads[key] = payloadCopy
}

// RemovePayload removes the payload for a stream
func (m *StreamPayloadManager) RemovePayload(conn net.Conn, streamID uint32) {
	key := StreamBufferKey{Conn: conn, StreamID: streamID}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.payloads, key)
}

// Headers represents HTTP/2 headers as a map from header name to values
// Header names are stored in lowercase (HTTP/2 requirement)
type Headers map[string][]string

// HeaderInfo stores decoded headers for a stream along with metadata
type HeaderInfo struct {
	Headers    Headers
	IsRequest  bool
	IsComplete bool // true when EndHeaders is set
}

// HeaderManager manages per-stream decoded headers
type HeaderManager struct {
	headers  map[StreamBufferKey]*HeaderInfo
	decoders map[net.Conn]*hpack.Decoder // HPACK decoder per connection
	mu       sync.RWMutex
}

// NewHeaderManager creates a new HeaderManager
func NewHeaderManager() *HeaderManager {
	return &HeaderManager{
		headers:  make(map[StreamBufferKey]*HeaderInfo),
		decoders: make(map[net.Conn]*hpack.Decoder),
	}
}

// DecodeAndStoreHeaders decodes HPACK-encoded headers and stores them for a stream
func (m *HeaderManager) DecodeAndStoreHeaders(conn net.Conn, streamID uint32, blockFragment []byte, endHeaders bool, isRequest bool) error {
	key := StreamBufferKey{Conn: conn, StreamID: streamID}

	m.mu.Lock()
	// Get or create header info for this stream
	if _, exists := m.headers[key]; !exists {
		m.headers[key] = &HeaderInfo{
			Headers:    make(Headers),
			IsRequest:  isRequest,
			IsComplete: false,
		}
	}
	// Get decoder for this connection (must hold lock to get decoder safely)
	decoder := m.getDecoderUnsafe(conn)
	m.mu.Unlock()

	// Set the emit function to capture decoded header fields for this specific stream
	// Capture key in closure so callback knows which stream these headers belong to
	decoder.SetEmitFunc(func(f hpack.HeaderField) {
		// Store header (HTTP/2 header names must be lowercase)
		headerName := strings.ToLower(f.Name)
		m.mu.Lock()
		defer m.mu.Unlock()
		if info, exists := m.headers[key]; exists {
			info.Headers[headerName] = append(info.Headers[headerName], f.Value)
		}
	})

	// Decode the header block fragment
	// HPACK decoder expects to receive fragments and accumulates them until EndHeaders
	_, err := decoder.Write(blockFragment)
	if err != nil {
		return fmt.Errorf("failed to decode HPACK block fragment: %w", err)
	}

	// Mark as complete when EndHeaders flag is set
	if endHeaders {
		m.mu.Lock()
		if info, exists := m.headers[key]; exists {
			info.IsComplete = true
		}
		m.mu.Unlock()
	}

	return nil
}

// getDecoderUnsafe returns or creates an HPACK decoder for a connection
// Must be called while holding m.mu lock
func (m *HeaderManager) getDecoderUnsafe(conn net.Conn) *hpack.Decoder {
	if decoder, exists := m.decoders[conn]; exists {
		return decoder
	}

	// Create decoder with a no-op callback (we'll set it per-stream in DecodeAndStoreHeaders)
	decoder := hpack.NewDecoder(4096, func(hpack.HeaderField) {}) // 4096 is the default dynamic table size
	m.decoders[conn] = decoder
	return decoder
}

// GetHeaders returns the decoded headers for a stream
func (m *HeaderManager) GetHeaders(conn net.Conn, streamID uint32) (Headers, bool) {
	key := StreamBufferKey{Conn: conn, StreamID: streamID}

	m.mu.RLock()
	defer m.mu.RUnlock()

	info, exists := m.headers[key]
	if !exists {
		return nil, false
	}

	// Return a copy to prevent external modification
	headersCopy := make(Headers)
	for k, v := range info.Headers {
		headersCopy[k] = make([]string, len(v))
		copy(headersCopy[k], v)
	}

	return headersCopy, true
}

// GetHeaderInfo returns the full header info for a stream
func (m *HeaderManager) GetHeaderInfo(conn net.Conn, streamID uint32) (*HeaderInfo, bool) {
	key := StreamBufferKey{Conn: conn, StreamID: streamID}

	m.mu.RLock()
	defer m.mu.RUnlock()

	info, exists := m.headers[key]
	if !exists {
		return nil, false
	}

	// Return a copy
	infoCopy := &HeaderInfo{
		Headers:    make(Headers),
		IsRequest:  info.IsRequest,
		IsComplete: info.IsComplete,
	}
	for k, v := range info.Headers {
		infoCopy.Headers[k] = make([]string, len(v))
		copy(infoCopy.Headers[k], v)
	}

	return infoCopy, true
}

// RemoveHeaders removes stored headers for a stream
func (m *HeaderManager) RemoveHeaders(conn net.Conn, streamID uint32) {
	key := StreamBufferKey{Conn: conn, StreamID: streamID}

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.headers, key)
}

// RemoveAllForConnection removes all headers and decoder for a connection
func (m *HeaderManager) RemoveAllForConnection(conn net.Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Remove all headers for this connection
	var keysToDelete []StreamBufferKey
	for key := range m.headers {
		if key.Conn == conn {
			keysToDelete = append(keysToDelete, key)
		}
	}
	for _, key := range keysToDelete {
		delete(m.headers, key)
	}

	// Remove decoder for this connection
	delete(m.decoders, conn)
}

// NewStreamBufferManager creates a new StreamBufferManager
func NewStreamBufferManager() *StreamBufferManager {
	return &StreamBufferManager{
		buffers: make(map[StreamBufferKey]*bytes.Buffer),
	}
}

// GetOrCreateBuffer returns the buffer for a stream, creating it if it doesn't exist
func (m *StreamBufferManager) GetOrCreateBuffer(conn net.Conn, streamID uint32) *bytes.Buffer {
	key := StreamBufferKey{Conn: conn, StreamID: streamID}

	m.mu.Lock()
	defer m.mu.Unlock()

	if buf, exists := m.buffers[key]; exists {
		return buf
	}

	buf := new(bytes.Buffer)
	m.buffers[key] = buf
	return buf
}

// FlushAndRemove flushes the buffer to the writer and removes it from the map
func (m *StreamBufferManager) FlushAndRemove(conn net.Conn, streamID uint32, w io.Writer) error {
	key := StreamBufferKey{Conn: conn, StreamID: streamID}

	m.mu.Lock()
	buf, exists := m.buffers[key]
	if !exists {
		m.mu.Unlock()
		return nil // No buffer to flush
	}
	delete(m.buffers, key)
	m.mu.Unlock()

	// Write the buffered data to the connection
	if buf.Len() > 0 {
		_, err := w.Write(buf.Bytes())
		return err
	}
	return nil
}

// FlushAllForConnection flushes all buffered streams for a connection
// Used when connection is shutting down (e.g., GOAWAY received)
func (m *StreamBufferManager) FlushAllForConnection(conn net.Conn, w io.Writer) error {
	m.mu.Lock()
	// Collect all buffers for this connection
	var toFlush []*bytes.Buffer
	var keysToDelete []StreamBufferKey

	for key, buf := range m.buffers {
		if key.Conn == conn {
			toFlush = append(toFlush, buf)
			keysToDelete = append(keysToDelete, key)
		}
	}

	// Remove from map
	for _, key := range keysToDelete {
		delete(m.buffers, key)
	}
	m.mu.Unlock()

	// Write all buffered data
	for _, buf := range toFlush {
		if buf.Len() > 0 {
			if _, err := w.Write(buf.Bytes()); err != nil {
				return err
			}
		}
	}
	return nil
}

// DefaultConfig returns the default proxy configuration
func DefaultConfig() *Config {
	targetAddr := os.Getenv("TARGET_ADDR")
	if targetAddr == "" {
		targetAddr = "" // Empty by default - use iptables interception
	}

	return &Config{
		Ports:      []int{15002, 15006},
		TargetAddr: targetAddr,
	}
}

// getOriginalDestination retrieves the original destination address for a TCP connection
// that was redirected by iptables. Returns the address and true if available.
func getOriginalDestination(conn net.Conn) (string, bool) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return "", false
	}

	file, err := tcpConn.File()
	if err != nil {
		logging.Debug("Failed to get file from connection", zap.Error(err))
		return "", false
	}
	defer file.Close()

	fd := file.Fd()

	// Try to get the original destination using SO_ORIGINAL_DST
	// This socket option is set by iptables REDIRECT target
	return getOriginalDestinationIPv4(fd)
}

// getOriginalDestinationIPv4 retrieves the original destination for IPv4 connections
func getOriginalDestinationIPv4(fd uintptr) (string, bool) {
	// For IPv4, SO_ORIGINAL_DST returns a sockaddr_in structure
	// Size of sockaddr_in: family (2) + port (2) + addr (4) + zero padding (8) = 16 bytes
	var sockaddr [128]byte
	size := uint32(len(sockaddr))

	// SO_ORIGINAL_DST is at IPPROTO_IP level, not SOL_SOCKET
	// This socket option is set by iptables REDIRECT target
	err := getSockopt(int(fd), syscall.IPPROTO_IP, unix.SO_ORIGINAL_DST, unsafe.Pointer(&sockaddr[0]), &size)
	if err != nil {
		logging.Debug("Failed to get SO_ORIGINAL_DST", zap.Error(err))
		return "", false
	}

	// Parse sockaddr_in: [family(2)][port(2)][addr(4)][...]
	if size < 8 {
		return "", false
	}

	family := binary.LittleEndian.Uint16(sockaddr[0:2])
	if family != syscall.AF_INET {
		return "", false
	}

	port := binary.BigEndian.Uint16(sockaddr[2:4])
	ip := net.IPv4(sockaddr[4], sockaddr[5], sockaddr[6], sockaddr[7])

	return fmt.Sprintf("%s:%d", ip.String(), port), true
}

// getSockopt performs getsockopt syscall
func getSockopt(s, level, name int, val unsafe.Pointer, vallen *uint32) (err error) {
	_, _, e1 := syscall.Syscall6(
		syscall.SYS_GETSOCKOPT,
		uintptr(s),
		uintptr(level),
		uintptr(name),
		uintptr(val),
		uintptr(unsafe.Pointer(vallen)),
		0,
	)
	if e1 != 0 {
		err = e1
	}
	return
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

func main() {
	// Initialize logging
	err := logging.Init(getLoggingConfig())
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize logging: %v", err))
	}

	logging.Info("Starting bidirectional TCP proxy for gRPC on :15002 and :15006...")

	// Create element chain with logging
	elementChain := element.NewRPCElementChain(
	// element.NewLoggingElement(true), // Enable verbose logging
	)

	config := DefaultConfig()

	state := &ProxyState{
		elementChain:   elementChain,
		bufferManager:  NewStreamBufferManager(),
		headerManager:  NewHeaderManager(),
		payloadManager: NewStreamPayloadManager(),
		targetAddr:     config.TargetAddr,
	}

	logging.Info("Proxy target configured", zap.String("target", state.targetAddr))

	// Start proxy servers
	if err := startProxyServers(config, state); err != nil {
		logging.Fatal("Failed to start proxy servers", zap.Error(err))
	}

	// Wait for shutdown signal
	waitForShutdown()
}

// startProxyServers starts TCP listeners on the configured ports
func startProxyServers(config *Config, state *ProxyState) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(config.Ports))

	for _, port := range config.Ports {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			if err := runProxyServer(p, state); err != nil {
				errCh <- fmt.Errorf("proxy server on port %d failed: %w", p, err)
			}
		}(port)
	}

	// Wait for all servers to start or fail
	wg.Wait()
	close(errCh)

	// Check for any startup errors
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

// runProxyServer runs a single TCP proxy server on the specified port
func runProxyServer(port int, state *ProxyState) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen on TCP port %d: %w", port, err)
	}
	defer listener.Close()

	logging.Info("Listening on TCP port", zap.Int("port", port))

	for {
		clientConn, err := listener.Accept()
		if err != nil {
			logging.Error("Accept error", zap.Int("port", port), zap.Error(err))
			continue
		}

		go handleConnection(clientConn, state)
	}
}

// handleConnection processes a TCP connection and intercepts gRPC traffic
func handleConnection(clientConn net.Conn, state *ProxyState) {
	defer clientConn.Close()

	// Peek at the first bytes to detect HTTP/2
	peekBytes := make([]byte, len(HTTP2Preface))
	n, err := clientConn.Read(peekBytes)
	if err != nil && err != io.EOF {
		logging.Error("Error reading connection preface", zap.Error(err))
		return
	}

	// Check if this is an HTTP/2 connection
	// HTTP/2 preface starts with "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"
	if n > 0 {
		prefaceStr := string(peekBytes[:n])
		if prefaceStr != HTTP2Preface && !(n >= 3 && prefaceStr[:3] == "PRI") {
			return
		}
	}

	handleHTTP2Connection(clientConn, peekBytes[:n], state)
}

// handleHTTP2Connection handles HTTP/2 connections for gRPC interception
func handleHTTP2Connection(clientConn net.Conn, preface []byte, state *ProxyState) {
	ctx := context.Background()
	_ = preface // Preface already consumed

	logging.Debug("New HTTP/2 connection",
		zap.String("clientAddr", clientConn.RemoteAddr().String()))

	// Get the original destination from iptables interception
	targetAddr := state.targetAddr
	if origDst, ok := getOriginalDestination(clientConn); ok {
		targetAddr = origDst
		logging.Debug("Using iptables original destination", zap.String("original_dst", origDst))
	} else if targetAddr == "" {
		logging.Error("No target address available (neither SO_ORIGINAL_DST nor TARGET_ADDR)")
		return
	}

	// Connect to target server
	logging.Info("Creating TCP connection to target server",
		zap.String("target", targetAddr),
		zap.String("clientAddr", clientConn.RemoteAddr().String()),
		zap.String("connectionID", fmt.Sprintf("%p", clientConn)))
	targetConn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		logging.Error("Failed to connect to target", zap.String("target", targetAddr), zap.Error(err))
		return
	}
	logging.Info("TCP connection to target established",
		zap.String("target", targetAddr),
		zap.String("targetConnLocal", targetConn.LocalAddr().String()),
		zap.String("targetConnRemote", targetConn.RemoteAddr().String()),
		zap.String("clientAddr", clientConn.RemoteAddr().String()),
		zap.String("connectionID", fmt.Sprintf("%p", clientConn)))
	defer func() {
		logging.Info("Closing TCP connection to target",
			zap.String("target", targetAddr),
			zap.String("clientAddr", clientConn.RemoteAddr().String()),
			zap.String("connectionID", fmt.Sprintf("%p", clientConn)))
		targetConn.Close()
	}()

	// Write the HTTP/2 preface to target
	if _, err := targetConn.Write([]byte(HTTP2Preface)); err != nil {
		logging.Error("Failed to write preface", zap.Error(err))
		return
	}

	// Create buffered readers for framing
	// Note: We do NOT include the preface in clientReader because:
	// 1. The preface has already been consumed from the client connection
	// 2. The preface has been sent to the target
	// 3. http2.Framer expects to read frames, not the preface
	// The SETTINGS frame (and other frames) are still in the clientConn TCP buffer
	clientReader := bufio.NewReader(clientConn)
	targetReader := bufio.NewReader(targetConn)

	var wg sync.WaitGroup
	wg.Add(2)

	// Handle client -> target (requests)
	// Reads from client, buffers frames, writes to target when stream ends
	// Use clientConn as the key for buffering (identifies the source connection)
	go func() {
		defer wg.Done()
		handleHTTP2Stream(clientReader, targetConn, state, ctx, true, clientConn)
	}()

	// Handle target -> client (responses)
	// Reads from target, buffers frames, writes to client when stream ends
	// Use clientConn as the key for buffering (identifies which client to send to)
	go func() {
		defer wg.Done()
		handleHTTP2Stream(targetReader, clientConn, state, ctx, false, clientConn)
	}()

	wg.Wait()
}

// processStreamThroughElementChain processes a complete stream (headers + payload) through the element chain
// Returns verdict and whether the stream should be flushed (false if dropped)
func processStreamThroughElementChain(ctx context.Context, state *ProxyState, connKey net.Conn, streamID uint32, isRequest bool, buf *bytes.Buffer) (element.Verdict, bool) {
	// Get headers for this stream
	headers, headersExist := state.headerManager.GetHeaders(connKey, streamID)
	if !headersExist {
		logging.Debug("No headers found for stream, skipping element chain processing",
			zap.Uint32("streamID", streamID),
			zap.Bool("isRequest", isRequest))
		return element.VerdictPass, true
	}

	// Get accumulated payload for this stream
	payload, payloadExists := state.payloadManager.GetPayload(connKey, streamID)
	if !payloadExists || len(payload) == 0 {
		logging.Debug("No payload found for stream, skipping element chain processing",
			zap.Uint32("streamID", streamID),
			zap.Bool("isRequest", isRequest))
		return element.VerdictPass, true
	}

	// Create HTTP2RPCContext for element chain processing
	// Note: GetHeaders() and GetPayload() already return copies, so we just need to convert types
	rpcCtx := &element.HTTP2RPCContext{
		Headers:    element.Headers(headers), // Already a copy from GetHeaders(), just converting type
		Payload:    payload,                  // Already a copy from GetPayload()
		StreamID:   streamID,
		IsRequest:  isRequest,
		RemoteAddr: connKey.RemoteAddr().String(),
	}

	// Extract pseudo-headers if available
	if path := rpcCtx.GetHeader(":path"); path != "" {
		rpcCtx.Path = path
	}
	if method := rpcCtx.GetHeader(":method"); method != "" {
		rpcCtx.Method = method
	}
	if authority := rpcCtx.GetHeader(":authority"); authority != "" {
		rpcCtx.Authority = authority
	}

	// Process through element chain
	var verdict element.Verdict
	var err error
	if isRequest {
		verdict, _, err = state.elementChain.ProcessRequest(ctx, rpcCtx)
	} else {
		verdict, _, err = state.elementChain.ProcessResponse(ctx, rpcCtx)
	}

	if err != nil {
		logging.Error("Error processing stream through element chain",
			zap.Uint32("streamID", streamID),
			zap.Bool("isRequest", isRequest),
			zap.Error(err))
		// On error, pass through (don't drop)
		return element.VerdictPass, true
	}

	if verdict == element.VerdictDrop {
		logging.Debug("Stream dropped by element chain",
			zap.Uint32("streamID", streamID),
			zap.Bool("isRequest", isRequest))
		return element.VerdictDrop, false
	}

	return element.VerdictPass, true
}

// handleHTTP2Stream processes HTTP/2 frames in a stream direction
// Buffers stream-specific frames and flushes them when END_STREAM is received
func handleHTTP2Stream(reader *bufio.Reader, destConn io.Writer, state *ProxyState, ctx context.Context, isRequest bool, connKey net.Conn) {
	// Create a framer that reads from the source
	// We'll create buffer-specific framers for writing to buffers
	readFramer := http2.NewFramer(nil, reader)

	// Direct framer for connection-level frames (SETTINGS, PING, GOAWAY, WINDOW_UPDATE)
	directFramer := http2.NewFramer(destConn, nil)

	for {
		frame, err := readFramer.ReadFrame()
		if err != nil {
			if err != io.EOF {
				logging.Debug("Frame read error", zap.Error(err), zap.Bool("isRequest", isRequest))
			}
			return
		}

		// Log all received frames
		logging.Debug("Received frame",
			zap.String("type", fmt.Sprintf("%T", frame)),
			zap.Bool("isRequest", isRequest),
			zap.String("connKey", connKey.RemoteAddr().String()))

		// Handle frames based on type
		switch f := frame.(type) {
		case *http2.DataFrame:
			// Get the data from the frame
			data := make([]byte, len(f.Data()))
			copy(data, f.Data())

			// Accumulate payload for element chain processing
			state.payloadManager.AppendPayload(connKey, f.StreamID, data)

			logging.Debug("DATA frame content",
				zap.Uint32("streamID", f.StreamID),
				zap.Int("dataLen", len(data)),
				zap.ByteString("data", data),
				zap.Bool("isRequest", isRequest))

			// Get or create buffer for this stream (keyed by connection + stream ID)
			buf := state.bufferManager.GetOrCreateBuffer(connKey, f.StreamID)

			// Create a framer that writes to the buffer
			bufFramer := http2.NewFramer(buf, nil)

			// Write DATA frame to buffer
			if err := bufFramer.WriteData(f.StreamID, f.StreamEnded(), data); err != nil {
				logging.Error("Error writing DATA frame to buffer", zap.Error(err))
				return
			}

			logging.Debug("Buffered DATA frame",
				zap.Uint32("streamID", f.StreamID),
				zap.Bool("endStream", f.StreamEnded()),
				zap.Bool("isRequest", isRequest),
				zap.String("connKey", connKey.RemoteAddr().String()))

			// If stream ended, process through element chain and flush buffer
			if f.StreamEnded() {
				// Process through element chain before flushing
				verdict, shouldFlush := processStreamThroughElementChain(ctx, state, connKey, f.StreamID, isRequest, buf)
				if !shouldFlush {
					// Stream was dropped by element chain
					logging.Debug("Stream dropped by element chain",
						zap.Uint32("streamID", f.StreamID),
						zap.Bool("isRequest", isRequest),
						zap.String("verdict", verdict.String()))
					// Remove buffer without flushing
					state.bufferManager.FlushAndRemove(connKey, f.StreamID, io.Discard)
					state.headerManager.RemoveHeaders(connKey, f.StreamID)
					state.payloadManager.RemovePayload(connKey, f.StreamID)
					return
				}

				// Flush buffer to destination
				if err := state.bufferManager.FlushAndRemove(connKey, f.StreamID, destConn); err != nil {
					logging.Error("Error flushing stream buffer", zap.Error(err))
					return
				}
				// Clean up headers and payload for this stream
				state.headerManager.RemoveHeaders(connKey, f.StreamID)
				state.payloadManager.RemovePayload(connKey, f.StreamID)
				logging.Debug("Flushed stream buffer",
					zap.Uint32("streamID", f.StreamID),
					zap.Bool("isRequest", isRequest),
					zap.String("connKey", connKey.RemoteAddr().String()))
			}

		case *http2.HeadersFrame:
			// Log when a new stream starts (first HEADERS frame)
			logging.Info("Processing stream on existing TCP connection",
				zap.Uint32("streamID", f.StreamID),
				zap.Bool("isRequest", isRequest),
				zap.String("connKey", connKey.RemoteAddr().String()),
				zap.String("connectionID", fmt.Sprintf("%p", connKey)))
			// Decode and store headers
			blockFragment := f.HeaderBlockFragment()
			if err := state.headerManager.DecodeAndStoreHeaders(connKey, f.StreamID, blockFragment, f.HeadersEnded(), isRequest); err != nil {
				logging.Error("Error decoding headers", zap.Error(err))
				// Continue processing even if decoding fails
			} else {
				// Log decoded headers when complete
				if f.HeadersEnded() {
					headers, _ := state.headerManager.GetHeaders(connKey, f.StreamID)
					logging.Debug("Decoded HTTP/2 headers",
						zap.Uint32("streamID", f.StreamID),
						zap.Bool("isRequest", isRequest),
						zap.Any("headers", headers),
						zap.String("connKey", connKey.RemoteAddr().String()))
				}
			}

			// Get or create buffer for this stream (keyed by connection + stream ID)
			buf := state.bufferManager.GetOrCreateBuffer(connKey, f.StreamID)

			// Create a framer that writes to the buffer
			bufFramer := http2.NewFramer(buf, nil)

			// Write HEADERS frame to buffer
			if err := bufFramer.WriteHeaders(http2.HeadersFrameParam{
				StreamID:      f.StreamID,
				BlockFragment: blockFragment,
				EndHeaders:    f.HeadersEnded(),
				EndStream:     f.StreamEnded(),
				Priority:      f.Priority,
			}); err != nil {
				logging.Error("Error writing HEADERS frame to buffer", zap.Error(err))
				return
			}

			logging.Debug("Buffered HEADERS frame",
				zap.Uint32("streamID", f.StreamID),
				zap.Bool("endStream", f.StreamEnded()),
				zap.Bool("isRequest", isRequest),
				zap.String("connKey", connKey.RemoteAddr().String()))

			// If stream ended, flush buffer to destination and clean up headers
			if f.StreamEnded() {
				if err := state.bufferManager.FlushAndRemove(connKey, f.StreamID, destConn); err != nil {
					logging.Error("Error flushing stream buffer", zap.Error(err))
					return
				}
				// Clean up headers for this stream
				state.headerManager.RemoveHeaders(connKey, f.StreamID)
				logging.Debug("Flushed stream buffer",
					zap.Uint32("streamID", f.StreamID),
					zap.Bool("isRequest", isRequest),
					zap.String("connKey", connKey.RemoteAddr().String()))
			}

		case *http2.SettingsFrame:
			// Forward SETTINGS frames immediately (connection-level)
			direction := "server"
			if isRequest {
				direction = "client"
			}
			logging.Debug("Encountered SETTINGS frame from " + direction)
			if f.IsAck() {
				// Forward SETTINGS ACK
				logging.Debug("Forwarding SETTINGS ACK", zap.Bool("isRequest", isRequest))
				if err := directFramer.WriteSettingsAck(); err != nil {
					logging.Error("Error writing SETTINGS ACK frame", zap.Error(err))
					return
				}
				logging.Debug("Successfully forwarded SETTINGS ACK", zap.Bool("isRequest", isRequest))
			} else {
				// Collect all settings from the frame
				var settings []http2.Setting
				f.ForeachSetting(func(s http2.Setting) error {
					settings = append(settings, s)
					logging.Debug("SETTINGS parameter",
						zap.String("ID", s.ID.String()),
						zap.Uint32("Val", s.Val))
					return nil
				})
				logging.Debug("Forwarding SETTINGS frame",
					zap.Bool("isRequest", isRequest),
					zap.Int("numSettings", len(settings)))
				// Forward SETTINGS frame with all settings
				if err := directFramer.WriteSettings(settings...); err != nil {
					logging.Error("Error writing SETTINGS frame", zap.Error(err))
					return
				}
				logging.Debug("Successfully forwarded SETTINGS frame",
					zap.Bool("isRequest", isRequest),
					zap.Int("numSettings", len(settings)))
			}

		case *http2.PingFrame:
			// Forward PING frames immediately (connection-level)
			// Preserve the ACK flag from the original frame
			if err := directFramer.WritePing(f.IsAck(), f.Data); err != nil {
				logging.Error("Error writing PING frame", zap.Error(err))
				return
			}

		case *http2.GoAwayFrame:
			// Flush all pending buffers before forwarding GOAWAY (connection is shutting down)
			logging.Debug("Received GOAWAY, flushing all pending buffers",
				zap.Uint32("lastStreamID", f.StreamID),
				zap.String("errCode", f.ErrCode.String()))
			if err := state.bufferManager.FlushAllForConnection(connKey, destConn); err != nil {
				logging.Error("Error flushing buffers on GOAWAY", zap.Error(err))
				return
			}

			// Clean up all headers and decoders for this connection
			state.headerManager.RemoveAllForConnection(connKey)

			// Forward GOAWAY frame (connection-level)
			if err := directFramer.WriteGoAway(f.StreamID, f.ErrCode, f.DebugData()); err != nil {
				logging.Error("Error writing GOAWAY frame", zap.Error(err))
				return
			}

		case *http2.RSTStreamFrame:
			// Buffer RST_STREAM frame (stream-level)
			buf := state.bufferManager.GetOrCreateBuffer(connKey, f.StreamID)
			bufFramer := http2.NewFramer(buf, nil)

			if err := bufFramer.WriteRSTStream(f.StreamID, f.ErrCode); err != nil {
				logging.Error("Error writing RST_STREAM frame to buffer", zap.Error(err))
				return
			}

			// RST_STREAM ends the stream, flush buffer and clean up headers
			if err := state.bufferManager.FlushAndRemove(connKey, f.StreamID, destConn); err != nil {
				logging.Error("Error flushing stream buffer after RST_STREAM", zap.Error(err))
				return
			}
			// Clean up headers for this stream
			state.headerManager.RemoveHeaders(connKey, f.StreamID)
			logging.Debug("Flushed stream buffer after RST_STREAM",
				zap.Uint32("streamID", f.StreamID),
				zap.Bool("isRequest", isRequest),
				zap.String("connKey", connKey.RemoteAddr().String()))

		case *http2.WindowUpdateFrame:
			if f.StreamID == 0 {
				// Connection-level flow control - forward immediately
				if err := directFramer.WriteWindowUpdate(f.StreamID, f.Increment); err != nil {
					logging.Error("Error writing WINDOW_UPDATE frame", zap.Error(err))
					return
				}
			} else {
				// Stream-specific flow control - buffer with stream
				buf := state.bufferManager.GetOrCreateBuffer(connKey, f.StreamID)
				bufFramer := http2.NewFramer(buf, nil)
				if err := bufFramer.WriteWindowUpdate(f.StreamID, f.Increment); err != nil {
					logging.Error("Error writing WINDOW_UPDATE frame to buffer", zap.Error(err))
					return
				}
				logging.Debug("Buffered WINDOW_UPDATE frame",
					zap.Uint32("streamID", f.StreamID),
					zap.Uint32("increment", f.Increment),
					zap.Bool("isRequest", isRequest),
					zap.String("connKey", connKey.RemoteAddr().String()))
			}

		default:
			// For other frame types, log and skip
			logging.Debug("Unhandled frame type", zap.String("type", fmt.Sprintf("%T", frame)))
		}
	}
}

// waitForShutdown waits for a shutdown signal
func waitForShutdown() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	logging.Info("Shutting down proxy...")
}
