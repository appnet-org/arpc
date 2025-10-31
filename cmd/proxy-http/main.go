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

	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/proxy-http/element"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
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
		targetAddr = "localhost:8080" // Default target
	}

	return &Config{
		Ports:      []int{15002, 15006},
		TargetAddr: targetAddr,
	}
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

	// Connect to target server
	targetConn, err := net.Dial("tcp", state.targetAddr)
	if err != nil {
		logging.Error("Failed to connect to target", zap.String("target", state.targetAddr), zap.Error(err))
		return
	}
	defer targetConn.Close()

	// Write the HTTP/2 preface to target
	// The preface we read from client will be included in the MultiReader
	if _, err := targetConn.Write([]byte(HTTP2Preface)); err != nil {
		logging.Error("Failed to write preface", zap.Error(err))
		return
	}

	// Create buffered readers for framing
	// We need to prepend any bytes we already read from the client connection
	// Framer writes to first arg, reads from second arg
	clientReader := bufio.NewReader(io.MultiReader(bytes.NewReader(preface), clientConn))
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
			// SettingsFrame doesn't expose settings directly, but we can check IsAck
			// For non-ACK settings frames, we need to write them differently
			// Since we can't reconstruct the original settings, we'll log a warning
			// In practice, settings are usually handled at connection establishment
			logging.Debug("SETTINGS frame encountered", zap.Bool("isAck", f.IsAck()))
			// Note: Settings frames are typically handled during connection setup
			// For now, we'll let the connection handle them naturally

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
	// Connect to target server
	targetConn, err := net.Dial("tcp", state.targetAddr)
	if err != nil {
		logging.Error("Failed to connect to target", zap.String("target", state.targetAddr), zap.Error(err))
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
