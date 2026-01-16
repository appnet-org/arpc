package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/transport"
	"github.com/appnet-org/proxy-buffer/util"
	"go.uber.org/zap"
)

const (
	// DefaultBufferSize is the size of the buffer used for reading packets
	DefaultBufferSize = 2048
	// DefaultSocketBufferSize is the size of the UDP socket receive buffer
	// A larger buffer prevents packet loss during high-throughput bursts
	DefaultSocketBufferSize = 16 * 1024 * 1024 // 16MB
)

// ProxyState manages the state of the UDP proxy
type ProxyState struct {
	elementChain *RPCElementChain
	packetBuffer *PacketBuffer
}

// Config holds the proxy configuration
type Config struct {
	Ports            []int
	EnableEncryption bool
	EncryptionKey    []byte
	BufferTimeout    time.Duration
}

// DefaultConfig returns the default proxy configuration
func DefaultConfig() *Config {
	return &Config{
		Ports:            []int{15002, 15006},
		BufferTimeout:    30 * time.Second,
		EnableEncryption: false,
		EncryptionKey:    nil,
	}
}

// SetEncryption sets the encryption key for the proxy
func (c *Config) SetEncryption(key []byte) {
	if key == nil {
		c.EnableEncryption = true
		c.EncryptionKey = transport.DefaultPublicKey
	} else {
		c.EnableEncryption = true
		c.EncryptionKey = key
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

	logging.Info("Starting buffered UDP proxy on :15002 and :15006...")

	// Initialize dynamic element loader
	InitElementLoader(ElementPluginDir + "/" + ElementPluginPrefix)

	config := DefaultConfig()

	// Override config from environment variables
	if bufferTimeout := os.Getenv("BUFFER_TIMEOUT"); bufferTimeout != "" {
		if timeout, err := time.ParseDuration(bufferTimeout); err == nil {
			config.BufferTimeout = timeout
		}
	}

	// Configure encryption from environment variable
	if enableEncryption := os.Getenv("ENABLE_ENCRYPTION"); enableEncryption == "true" {
		config.SetEncryption(nil)
		// Initialize GCM cipher objects for encryption/decryption
		if err := transport.InitGCMObjects(transport.DefaultPublicKey, transport.DefaultPrivateKey); err != nil {
			logging.Fatal("Failed to initialize GCM objects", zap.Error(err))
		}
		logging.Info("Encryption GCM objects initialized")
	}

	logging.Info("Proxy configuration",
		zap.Duration("bufferTimeout", config.BufferTimeout),
		zap.Bool("enableEncryption", config.EnableEncryption),
		zap.Ints("ports", config.Ports))

	// Initialize packet buffer
	packetBuffer := NewPacketBuffer(config.BufferTimeout)
	defer packetBuffer.Close()

	// Get the dynamically loaded element chain
	elementChain := GetElementChain()
	if elementChain == nil {
		// Fallback to empty chain if no plugin loaded
		elementChain = NewRPCElementChain()
		logging.Warn("No element chain available, using empty chain")
	}

	state := &ProxyState{
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
			if err := runProxyServer(p, state, config); err != nil {
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
func runProxyServer(port int, state *ProxyState, config *Config) error {
	listenAddr := &net.UDPAddr{Port: port}
	conn, err := net.ListenUDP("udp", listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP port %d: %w", port, err)
	}
	defer conn.Close()

	// Set a larger receive buffer to prevent packet loss during high-throughput bursts
	if err := conn.SetReadBuffer(DefaultSocketBufferSize); err != nil {
		logging.Warn("Failed to set UDP receive buffer size", zap.Int("port", port), zap.Error(err))
	}

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

		go handlePacket(conn, state, src, data, config)
	}
}

// handlePacket processes incoming packets and forwards them to the appropriate peer.
// This simplified version buffers ALL fragments before processing through the element chain.
func handlePacket(conn *net.UDPConn, state *ProxyState, src *net.UDPAddr, data []byte, config *Config) {
	ctx := context.Background()

	// Process packet - returns nil if still buffering fragments
	// Returns a complete BufferedPacket only when ALL fragments have been received
	bufferedPacket, err := state.packetBuffer.ProcessPacket(data, src)
	if err != nil {
		logging.Error("Error processing packet through buffer", zap.Error(err))
		return
	}

	// If bufferedPacket is nil, we're still waiting for more fragments
	if bufferedPacket == nil {
		logging.Debug("Still buffering packet fragments", zap.String("src", src.String()))
		return
	}

	payload := bufferedPacket.Payload
	publicPayload := payload
	privatePayload := []byte{}

	// Split the payload into public and private segments
	if len(payload) > offsetToPrivate(payload) {
		logging.Debug("Splitting payload into public and private segments",
			zap.Int("size", len(payload)),
			zap.Int("offsetToPrivate", offsetToPrivate(payload)))
		publicPayload = payload[:offsetToPrivate(payload)]
		privatePayload = payload[offsetToPrivate(payload):]
	} else {
		logging.Error("Payload is too short to split into public and private segments", zap.Int("size", len(payload)))
		return
	}

	// Decrypt the public segment if encryption is enabled
	if config.EnableEncryption {
		publicPayload = transport.DecryptSymphonyData(publicPayload, config.EncryptionKey, nil)
		logging.Debug("Public segment decrypted",
			zap.Int("size", len(publicPayload)),
			zap.String("publicPayload", string(publicPayload)))
	}

	// Update the packet with the decrypted public segment
	bufferedPacket.Payload = publicPayload

	// Process packet through the element chain
	err = runElementsChain(ctx, state, bufferedPacket)
	if err != nil {
		logging.Error("Error processing packet through element chain or packet was dropped",
			zap.Error(err))
		// Send error packet back to the source
		if sendErr := util.SendErrorPacket(conn, bufferedPacket.Source, bufferedPacket.RPCID, err.Error()); sendErr != nil {
			logging.Error("Failed to send error packet", zap.Error(sendErr))
		}
		return
	}

	// Encrypt the packet if encryption is enabled
	if config.EnableEncryption {
		bufferedPacket.Payload = transport.EncryptSymphonyData(bufferedPacket.Payload, config.EncryptionKey, nil)
	}

	// Append the private segment to the packet
	bufferedPacket.Payload = append(bufferedPacket.Payload, privatePayload...)

	// Fragment the packet if needed and forward all fragments
	fragmentedPackets, err := state.packetBuffer.FragmentPacketForForward(bufferedPacket)
	if err != nil {
		logging.Error("Failed to fragment packet for forwarding", zap.Error(err))
		return
	}

	// Send all fragments
	for _, fragment := range fragmentedPackets {
		if _, err := conn.WriteToUDP(fragment.Data, fragment.Peer); err != nil {
			logging.Error("WriteToUDP error", zap.Error(err))
			return
		}
	}

	logging.Debug("Forwarded packet",
		zap.Int("fragments", len(fragmentedPackets)),
		zap.Int("bytes", len(bufferedPacket.Payload)),
		zap.Uint64("rpcID", bufferedPacket.RPCID),
		zap.String("from", bufferedPacket.Source.String()),
		zap.String("to", bufferedPacket.Peer.String()),
		zap.String("packetType", bufferedPacket.PacketType.String()))
}

// runElementsChain processes the packet through the element chain.
// Modifications to the packet payload are made in place via the processedPacket return value.
// Returns an error if processing fails or if the verdict is PacketVerdictDrop.
func runElementsChain(ctx context.Context, state *ProxyState, packet *util.BufferedPacket) error {
	// Get current element chain (may have been updated by plugin loader)
	elementChain := GetElementChain()
	var err error
	var processedPacket *util.BufferedPacket
	var verdict util.PacketVerdict

	if elementChain == nil {
		// No element chain available, pass through
		logging.Debug("No element chain available, passing packet through")
		return nil
	}

	switch packet.PacketType {
	case util.PacketTypeRequest:
		// Process request through element chain
		processedPacket, verdict, _, err = elementChain.ProcessRequest(ctx, packet)
	case util.PacketTypeResponse:
		// Process response through element chain (in reverse order)
		processedPacket, verdict, _, err = elementChain.ProcessResponse(ctx, packet)
	default:
		// For other packet types (Error, Unknown, etc.), skip processing
		logging.Debug("Skipping element chain processing for packet type",
			zap.String("packetType", packet.PacketType.String()))
		return nil
	}

	// Check verdict - if dropped, don't forward the packet
	if verdict == util.PacketVerdictDrop || err != nil {
		return err
	}

	// Update the packet with any changes made by the element chain
	if processedPacket != nil {
		*packet = *processedPacket
	}

	return nil
}

// waitForShutdown waits for a shutdown signal
func waitForShutdown() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	logging.Info("Shutting down proxy...")
}
