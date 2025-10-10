package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/proxy/element"
	"go.uber.org/zap"
)

const (
	// DefaultBufferSize is the size of the buffer used for reading packets
	DefaultBufferSize = 2048
)

// ProxyState manages the state of the UDP proxy
type ProxyState struct {
	mu           sync.RWMutex
	connections  map[string]*net.UDPAddr // key: sender IP:port, value: peer
	elementChain *element.RPCElementChain
	packetBuffer *PacketBuffer
}

// Config holds the proxy configuration
type Config struct {
	Ports           []int
	EnableBuffering bool
	BufferTimeout   time.Duration
}

// DefaultConfig returns the default proxy configuration
func DefaultConfig() *Config {
	return &Config{
		Ports:           []int{15002, 15006},
		EnableBuffering: false, // Disabled by default
		BufferTimeout:   30 * time.Second,
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

	logging.Info("Starting bidirectional UDP proxy on :15002 and :15006...")

	// Create element chain with logging
	elementChain := element.NewRPCElementChain(
	// element.NewLoggingElement(true), // Enable verbose logging
	)

	config := DefaultConfig()

	// Override config from environment variables
	if enableBuffering := os.Getenv("ENABLE_PACKET_BUFFERING"); enableBuffering != "" {
		config.EnableBuffering = enableBuffering == "true"
	}
	if bufferTimeout := os.Getenv("BUFFER_TIMEOUT"); bufferTimeout != "" {
		if timeout, err := time.ParseDuration(bufferTimeout); err == nil {
			config.BufferTimeout = timeout
		}
	}

	logging.Info("Proxy configuration",
		zap.Bool("enableBuffering", config.EnableBuffering),
		zap.Duration("bufferTimeout", config.BufferTimeout),
		zap.Ints("ports", config.Ports))

	// Initialize packet buffer
	packetBuffer := NewPacketBuffer(config.EnableBuffering, config.BufferTimeout)
	defer packetBuffer.Close()

	state := &ProxyState{
		connections:  make(map[string]*net.UDPAddr),
		elementChain: elementChain,
		packetBuffer: packetBuffer,
	}

	// Start proxy servers
	if err := startProxyServers(config, state); err != nil {
		logging.Fatal("Failed to start proxy servers", zap.Error(err))
	}

	// Wait for shutdown signal
	waitForShutdown()
}

// startProxyServers starts UDP listeners on the configured ports
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

// runProxyServer runs a single UDP proxy server on the specified port
func runProxyServer(port int, state *ProxyState) error {
	listenAddr := &net.UDPAddr{Port: port}
	conn, err := net.ListenUDP("udp", listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP port %d: %w", port, err)
	}
	defer conn.Close()

	logging.Info("Listening on UDP port", zap.Int("port", port))

	buf := make([]byte, DefaultBufferSize)

	for {
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			logging.Error("ReadFromUDP error", zap.Int("port", port), zap.Error(err))
			continue
		}

		// Create a copy of the data to avoid race conditions
		data := make([]byte, n)
		copy(data, buf[:n])

		go handlePacket(conn, state, src, data)
	}
}

// extractPeer extracts peer information from the packet data
func extractPeer(data []byte) (*net.UDPAddr, uint16) {
	if len(data) < 19 {
		return nil, 0
	}

	// Filter out non-request packets
	packetType := data[0]
	if packetType != 1 {
		return nil, 0
	}

	// Payload starts at index 17 for data packets
	peerIp := data[13:17]
	peerPort := binary.LittleEndian.Uint16(data[17:19])
	localPort := binary.LittleEndian.Uint16(data[19:21])
	return &net.UDPAddr{IP: net.IP(peerIp), Port: int(peerPort)}, localPort
}

// handlePacket processes incoming packets and forwards them to the appropriate peer
func handlePacket(conn *net.UDPConn, state *ProxyState, src *net.UDPAddr, data []byte) {
	ctx := context.Background()
	peer, localPort := extractPeer(data)
	logging.Debug("Extracted peer", zap.String("peer", peer.String()), zap.Uint16("localPort", localPort))

	if peer != nil {
		// It's a request: map src <-> peer
		state.mu.Lock()

		// TODO(XZ): temp solution for issue #6. We only rewrite the port for client-side proxy.
		if src.Port != 15002 {
			src.Port = int(localPort) // hack
		}

		state.connections[src.String()] = peer
		state.connections[peer.String()] = src // reverse mapping
		state.mu.Unlock()
	} else {
		// It's a response: look up the reverse mapping
		state.mu.RLock()
		var ok bool
		peer, ok = state.connections[src.String()]
		state.mu.RUnlock()

		if !ok {
			logging.Warn("Unknown client for server response, dropping", zap.String("src", src.String()))
			return
		}
	}

	// Process packet through buffer (may return nil if still buffering)
	bufferedPacket, err := state.packetBuffer.ProcessPacket(data, src, peer, peer != nil)
	if err != nil {
		logging.Error("Error processing packet through buffer", zap.Error(err))
		return
	}

	// If bufferedPacket is nil, we're still waiting for more fragments
	if bufferedPacket == nil {
		logging.Debug("Still buffering packet fragments", zap.String("src", src.String()))
		return
	}

	processedData := processPacket(ctx, state, bufferedPacket.Data, bufferedPacket.IsRequest)

	// Send the processed packet to the peer
	if _, err := conn.WriteToUDP(processedData, bufferedPacket.Peer); err != nil {
		logging.Error("WriteToUDP error", zap.Error(err))
		return
	}

	logging.Debug("Forwarded packet",
		zap.Int("bytes", len(processedData)),
		zap.String("src", bufferedPacket.Source.String()),
		zap.String("peer", bufferedPacket.Peer.String()),
		zap.Uint64("rpcID", bufferedPacket.RPCID))
}

// processPacket processes the packet through the element chain
func processPacket(ctx context.Context, state *ProxyState, data []byte, isRequest bool) []byte {
	// Log the packet (in hex)
	// logging.Debug("Received packet", zap.String("hex", fmt.Sprintf("%x", data)))

	var err error
	if isRequest {
		// Process request through element chain
		data, _, err = state.elementChain.ProcessRequest(ctx, data)
	} else {
		// Process response through element chain (in reverse order)
		data, _, err = state.elementChain.ProcessResponse(ctx, data)
	}

	if err != nil {
		logging.Error("Error processing packet through element chain", zap.Error(err))
		return data // Return original data on error
	}

	return data
}

// waitForShutdown waits for a shutdown signal
func waitForShutdown() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	logging.Info("Shutting down proxy...")
}
