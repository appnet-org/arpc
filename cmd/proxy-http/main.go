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
	"github.com/appnet-org/proxy-http/element"
	"go.uber.org/zap"
)

const (
	// DefaultBufferSize is the size of the buffer used for reading packets
	DefaultBufferSize = 2048
)

// ProxyState manages the state of the UDP proxy
type ProxyState struct {
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

// PacketRoutingInfo contains routing information extracted from packet headers
type PacketRoutingInfo struct {
	DstIP   net.IP
	DstPort uint16
	SrcIP   net.IP
	SrcPort uint16
}

// extractRoutingInfo extracts routing information from the packet data
func extractRoutingInfo(data []byte) (*PacketRoutingInfo, error) {
	if len(data) < 29 {
		return nil, fmt.Errorf("packet too short for routing info: %d bytes", len(data))
	}

	// packet format: [PacketTypeID(1B)][RPCID(8B)][TotalPackets(2B)][SeqNumber(2B)][DstIP(4B)][DstPort(2B)][SrcIP(4B)][SrcPort(2B)][PayloadLen(4B)][Payload]

	// Extract destination IP and port
	dstIP := net.IP(data[13:17])
	dstPort := binary.LittleEndian.Uint16(data[17:19])

	// Extract source IP and port
	srcIP := net.IP(data[19:23])
	srcPort := binary.LittleEndian.Uint16(data[23:25])

	return &PacketRoutingInfo{
		DstIP:   dstIP,
		DstPort: dstPort,
		SrcIP:   srcIP,
		SrcPort: srcPort,
	}, nil
}

// isRequestPacket checks if this is a request packet (type 1)
func isRequestPacket(data []byte) bool {
	if len(data) < 1 {
		return false
	}
	return data[0] == 1
}

// handlePacket processes incoming packets and forwards them to the appropriate peer
func handlePacket(conn *net.UDPConn, state *ProxyState, src *net.UDPAddr, data []byte) {
	ctx := context.Background()

	// Extract routing information from packet headers
	routingInfo, err := extractRoutingInfo(data)
	if err != nil {
		logging.Debug("Failed to extract routing info, dropping packet", zap.Error(err))
		return
	}

	// Always forward to the destination specified in the packet header (DstIP:DstPort)
	// This works for both requests and responses since the server now correctly
	// sets the destination to the original client address
	forwardTo := &net.UDPAddr{
		IP:   routingInfo.DstIP,
		Port: int(routingInfo.DstPort),
	}
	isRequest := isRequestPacket(data)

	logging.Debug("Intercepted packet",
		zap.String("from", src.String()),
		zap.String("packetSrc", net.JoinHostPort(routingInfo.SrcIP.String(), fmt.Sprintf("%d", routingInfo.SrcPort))),
		zap.String("packetDst", net.JoinHostPort(routingInfo.DstIP.String(), fmt.Sprintf("%d", routingInfo.DstPort))),
		zap.String("forwardTo", forwardTo.String()),
		zap.Bool("isRequest", isRequest))

	// Process packet through buffer (may return nil if still buffering)
	bufferedPacket, err := state.packetBuffer.ProcessPacket(data, src, forwardTo, isRequest)
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

	// Update the buffered packet with processed data
	bufferedPacket.Data = processedData

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
		zap.Int("bytes", len(processedData)),
		zap.String("from", bufferedPacket.Source.String()),
		zap.String("to", bufferedPacket.Peer.String()),
		zap.Bool("isRequest", bufferedPacket.IsRequest))
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
