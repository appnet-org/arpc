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
	"sync"
	"syscall"
	"unsafe"

	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/proxy-h2/element"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
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
	elementChain  *element.RPCElementChain
	bufferManager *StreamBufferManager
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
		elementChain:  elementChain,
		bufferManager: NewStreamBufferManager(),
		targetAddr:    config.TargetAddr,
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
			logging.Error("Client is not following HTTP2.")
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
	targetConn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		logging.Error("Failed to connect to target", zap.String("target", targetAddr), zap.Error(err))
		return
	}
	defer targetConn.Close()

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

			logging.Debug("DATA frame content",
				zap.Uint32("streamID", f.StreamID),
				zap.Int("dataLen", len(data)),
				zap.ByteString("data", data),
				zap.Bool("isRequest", isRequest))

			// processedData, err := processGRPCMessage(ctx, state, data, isRequest)
			// if err != nil {
			// 	logging.Error("Error processing gRPC message", zap.Error(err))
			// 	// Forward original data on error
			// 	processedData = data
			// }

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

			// If stream ended, flush buffer to destination
			if f.StreamEnded() {
				if err := state.bufferManager.FlushAndRemove(connKey, f.StreamID, destConn); err != nil {
					logging.Error("Error flushing stream buffer", zap.Error(err))
					return
				}
				logging.Debug("Flushed stream buffer",
					zap.Uint32("streamID", f.StreamID),
					zap.Bool("isRequest", isRequest),
					zap.String("connKey", connKey.RemoteAddr().String()))
			}

		case *http2.HeadersFrame:
			// Get or create buffer for this stream (keyed by connection + stream ID)
			buf := state.bufferManager.GetOrCreateBuffer(connKey, f.StreamID)

			// Create a framer that writes to the buffer
			bufFramer := http2.NewFramer(buf, nil)

			// Write HEADERS frame to buffer
			if err := bufFramer.WriteHeaders(http2.HeadersFrameParam{
				StreamID:      f.StreamID,
				BlockFragment: f.HeaderBlockFragment(),
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

			// If stream ended, flush buffer to destination
			if f.StreamEnded() {
				if err := state.bufferManager.FlushAndRemove(connKey, f.StreamID, destConn); err != nil {
					logging.Error("Error flushing stream buffer", zap.Error(err))
					return
				}
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

			// RST_STREAM ends the stream, flush buffer
			if err := state.bufferManager.FlushAndRemove(connKey, f.StreamID, destConn); err != nil {
				logging.Error("Error flushing stream buffer after RST_STREAM", zap.Error(err))
				return
			}
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

// // processGRPCMessage processes a gRPC message through the element chain
// // gRPC wire format: [compression flag (1 byte)][message length (4 bytes big-endian)][message data]
// func processGRPCMessage(ctx context.Context, state *ProxyState, data []byte, isRequest bool) ([]byte, error) {
// 	if len(data) < 5 {
// 		// Not a valid gRPC message, return as-is
// 		return data, nil
// 	}

// 	// Check compression flag (first byte)
// 	compressed := data[0] != 0
// 	if compressed {
// 		// For now, we don't handle compressed messages
// 		logging.Debug("Compressed gRPC message detected, skipping processing")
// 		return data, nil
// 	}

// 	// Extract message length (bytes 1-4, big-endian)
// 	messageLen := binary.BigEndian.Uint32(data[1:5])

// 	// Validate message length
// 	if messageLen == 0 || int(messageLen) > len(data)-5 {
// 		// Invalid message length or incomplete message
// 		return data, nil
// 	}

// 	// Extract the actual gRPC message payload (starting at byte 5)
// 	grpcMessage := data[5 : 5+messageLen]

// 	// Process through element chain
// 	var processedMessage []byte
// 	var err error

// 	if isRequest {
// 		processedMessage, ctx, err = state.elementChain.ProcessRequest(ctx, grpcMessage)
// 	} else {
// 		processedMessage, ctx, err = state.elementChain.ProcessResponse(ctx, grpcMessage)
// 	}

// 	if err != nil {
// 		return data, fmt.Errorf("element chain processing failed: %w", err)
// 	}

// 	// Reconstruct gRPC message with processed payload
// 	if len(processedMessage) != int(messageLen) {
// 		// Message size changed, update the length header
// 		newLen := uint32(len(processedMessage))
// 		newData := make([]byte, 5+newLen)
// 		newData[0] = data[0] // Compression flag
// 		binary.BigEndian.PutUint32(newData[1:5], newLen)
// 		copy(newData[5:], processedMessage)
// 		return newData, nil
// 	}

// 	// Message size unchanged, just update the payload
// 	result := make([]byte, len(data))
// 	copy(result, data)
// 	copy(result[5:5+messageLen], processedMessage)
// 	return result, nil
// }

// waitForShutdown waits for a shutdown signal
func waitForShutdown() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	logging.Info("Shutting down proxy...")
}
