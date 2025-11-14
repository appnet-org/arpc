package main

import (
	"bufio"
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
	"github.com/appnet-org/proxy-http/element"
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
	elementChain *element.RPCElementChain
	// Target server address for proxying (optional, can be configured via env)
	// Used only if SO_ORIGINAL_DST is unavailable
	targetAddr string
}

// Config holds the proxy configuration
type Config struct {
	Ports      []int
	TargetAddr string
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
		elementChain: elementChain,
		targetAddr:   config.TargetAddr,
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
	isHTTP2 := false
	if n > 0 {
		prefaceStr := string(peekBytes[:n])
		logging.Debug("Server preface", zap.String("preface", prefaceStr))
		if prefaceStr == HTTP2Preface || (n >= 3 && prefaceStr[:3] == "PRI") {
			isHTTP2 = true
		}
	}

	if isHTTP2 {
		// Handle HTTP/2 gRPC connection
		handleHTTP2Connection(clientConn, peekBytes[:n], state)
	} else {
		// Handle plain TCP connection (forward as-is)
		handlePlainTCPConnection(clientConn, peekBytes[:n], state)
	}
}

// handleHTTP2Connection handles HTTP/2 connections for gRPC interception
func handleHTTP2Connection(clientConn net.Conn, preface []byte, state *ProxyState) {
	ctx := context.Background()

	// Get the original destination from iptables interception
	targetAddr := state.targetAddr
	if origDst, ok := getOriginalDestination(clientConn); ok {
		targetAddr = origDst
		logging.Info("Using iptables original destination", zap.String("original_dst", origDst))
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

	// Create HTTP/2 framers:
	// - clientFramer: reads from client, writes to target
	// - targetFramer: reads from target, writes to client
	clientFramer := http2.NewFramer(targetConn, clientReader)
	targetFramer := http2.NewFramer(clientConn, targetReader)

	var wg sync.WaitGroup
	wg.Add(2)

	// Handle client -> target (requests)
	go func() {
		defer wg.Done()
		handleHTTP2Stream(clientFramer, state, ctx, true)
	}()

	// Handle target -> client (responses)
	go func() {
		defer wg.Done()
		handleHTTP2Stream(targetFramer, state, ctx, false)
	}()

	wg.Wait()
}

// handleHTTP2Stream processes HTTP/2 frames in a stream direction
func handleHTTP2Stream(framer *http2.Framer, state *ProxyState, ctx context.Context, isRequest bool) {
	for {
		frame, err := framer.ReadFrame()
		if err != nil {
			if err != io.EOF {
				logging.Debug("Frame read error", zap.Error(err), zap.Bool("isRequest", isRequest))
			}
			return
		}

		// Log all received frames
		logging.Info("Received frame",
			zap.String("type", fmt.Sprintf("%T", frame)),
			zap.Bool("isRequest", isRequest))

		// Intercept DATA frames containing gRPC messages
		switch f := frame.(type) {
		case *http2.DataFrame:
			// Get the data from the frame
			data := make([]byte, len(f.Data()))
			copy(data, f.Data())

			processedData, err := processGRPCMessage(ctx, state, data, isRequest)
			if err != nil {
				logging.Error("Error processing gRPC message", zap.Error(err))
				// Forward original data on error
				processedData = data
			}

			// Write new DATA frame with processed data
			// The framer will write to the target (for client->target) or client (for target->client)
			// StreamID is a field, not a method
			if err := framer.WriteData(f.StreamID, f.StreamEnded(), processedData); err != nil {
				logging.Error("Error writing DATA frame", zap.Error(err))
				return
			}

		case *http2.HeadersFrame:
			// Forward HEADERS frames
			if err := framer.WriteHeaders(http2.HeadersFrameParam{
				StreamID:      f.StreamID,
				BlockFragment: f.HeaderBlockFragment(),
				EndHeaders:    f.HeadersEnded(),
				EndStream:     f.StreamEnded(),
				Priority:      f.Priority,
			}); err != nil {
				logging.Error("Error writing HEADERS frame", zap.Error(err))
				return
			}

		case *http2.SettingsFrame:
			// Forward SETTINGS frames
			direction := "server"
			if isRequest {
				direction = "client"
			}
			logging.Debug("Encountered SETTINGS frame from " + direction)
			if f.IsAck() {
				// Forward SETTINGS ACK
				logging.Info("Forwarding SETTINGS ACK", zap.Bool("isRequest", isRequest))
				if err := framer.WriteSettingsAck(); err != nil {
					logging.Error("Error writing SETTINGS ACK frame", zap.Error(err))
					return
				}
				logging.Info("Successfully forwarded SETTINGS ACK", zap.Bool("isRequest", isRequest))
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
				logging.Info("Forwarding SETTINGS frame",
					zap.Bool("isRequest", isRequest),
					zap.Int("numSettings", len(settings)))
				// Forward SETTINGS frame with all settings
				if err := framer.WriteSettings(settings...); err != nil {
					logging.Error("Error writing SETTINGS frame", zap.Error(err))
					return
				}
				logging.Info("Successfully forwarded SETTINGS frame",
					zap.Bool("isRequest", isRequest),
					zap.Int("numSettings", len(settings)))
			}

		case *http2.PingFrame:
			// Forward PING frames
			if err := framer.WritePing(false, f.Data); err != nil {
				logging.Error("Error writing PING frame", zap.Error(err))
				return
			}

		case *http2.GoAwayFrame:
			// Forward GOAWAY frames
			if err := framer.WriteGoAway(f.StreamID, f.ErrCode, f.DebugData()); err != nil {
				logging.Error("Error writing GOAWAY frame", zap.Error(err))
				return
			}

		case *http2.RSTStreamFrame:
			// Forward RST_STREAM frames
			if err := framer.WriteRSTStream(f.StreamID, f.ErrCode); err != nil {
				logging.Error("Error writing RST_STREAM frame", zap.Error(err))
				return
			}

		case *http2.WindowUpdateFrame:
			// Forward WINDOW_UPDATE frames
			if err := framer.WriteWindowUpdate(f.StreamID, f.Increment); err != nil {
				logging.Error("Error writing WINDOW_UPDATE frame", zap.Error(err))
				return
			}

		default:
			// For other frame types, log and skip
			logging.Debug("Unhandled frame type", zap.String("type", fmt.Sprintf("%T", frame)))
		}
	}
}

// processGRPCMessage processes a gRPC message through the element chain
// gRPC wire format: [compression flag (1 byte)][message length (4 bytes big-endian)][message data]
func processGRPCMessage(ctx context.Context, state *ProxyState, data []byte, isRequest bool) ([]byte, error) {
	if len(data) < 5 {
		// Not a valid gRPC message, return as-is
		return data, nil
	}

	// Check compression flag (first byte)
	compressed := data[0] != 0
	if compressed {
		// For now, we don't handle compressed messages
		logging.Debug("Compressed gRPC message detected, skipping processing")
		return data, nil
	}

	// Extract message length (bytes 1-4, big-endian)
	messageLen := binary.BigEndian.Uint32(data[1:5])

	// Validate message length
	if messageLen == 0 || int(messageLen) > len(data)-5 {
		// Invalid message length or incomplete message
		return data, nil
	}

	// Extract the actual gRPC message payload (starting at byte 5)
	grpcMessage := data[5 : 5+messageLen]

	// Process through element chain
	var processedMessage []byte
	var err error

	if isRequest {
		processedMessage, ctx, err = state.elementChain.ProcessRequest(ctx, grpcMessage)
	} else {
		processedMessage, ctx, err = state.elementChain.ProcessResponse(ctx, grpcMessage)
	}

	if err != nil {
		return data, fmt.Errorf("element chain processing failed: %w", err)
	}

	// Reconstruct gRPC message with processed payload
	if len(processedMessage) != int(messageLen) {
		// Message size changed, update the length header
		newLen := uint32(len(processedMessage))
		newData := make([]byte, 5+newLen)
		newData[0] = data[0] // Compression flag
		binary.BigEndian.PutUint32(newData[1:5], newLen)
		copy(newData[5:], processedMessage)
		return newData, nil
	}

	// Message size unchanged, just update the payload
	result := make([]byte, len(data))
	copy(result, data)
	copy(result[5:5+messageLen], processedMessage)
	return result, nil
}

// handlePlainTCPConnection handles plain TCP connections (non-HTTP/2)
func handlePlainTCPConnection(clientConn net.Conn, peekBytes []byte, state *ProxyState) {
	// Get the original destination from iptables interception
	targetAddr := state.targetAddr
	if origDst, ok := getOriginalDestination(clientConn); ok {
		targetAddr = origDst
		logging.Info("Using iptables original destination", zap.String("original_dst", origDst))
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

	// Forward the peeked bytes
	if len(peekBytes) > 0 {
		if _, err := targetConn.Write(peekBytes); err != nil {
			logging.Error("Failed to write peeked bytes", zap.Error(err))
			return
		}
	}

	// Simple bidirectional forwarding
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(targetConn, clientConn)
		targetConn.Close()
	}()

	go func() {
		defer wg.Done()
		io.Copy(clientConn, targetConn)
		clientConn.Close()
	}()

	wg.Wait()
}

// waitForShutdown waits for a shutdown signal
func waitForShutdown() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	logging.Info("Shutting down proxy...")
}
