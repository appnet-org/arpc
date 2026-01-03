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
	"github.com/appnet-org/proxy/element"
	"github.com/appnet-org/proxy/util"
	"go.uber.org/zap"
)

const (
	// DefaultBufferSize is the size of the buffer used for reading packets
	DefaultBufferSize = 2048
)

// ProxyState manages the state of the UDP proxy
type ProxyState struct {
	elementChain *RPCElementChain
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
		Ports:         []int{15002, 15006},
		BufferTimeout: 30 * time.Second,
	}
}

// getLoggingConfig reads logging configuration from environment variables with defaults
func getLoggingConfig() *logging.Config {
	level := os.Getenv("LOG_LEVEL")
	if level == "" {
		level = "info"
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
	elementChain := NewRPCElementChain(
		element.NewFirewallElement(100),
	)

	config := DefaultConfig()

	// Override config from environment variables
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
	packetBuffer := NewPacketBuffer(config.BufferTimeout)
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
		// TODO: We could pre-allocate the buffer and reuse it for each packet
		data := make([]byte, n)
		copy(data, buf[:n])

		go handlePacket(conn, state, src, data)
	}
}

// handlePacket processes incoming packets and forwards them to the appropriate peer
func handlePacket(conn *net.UDPConn, state *ProxyState, src *net.UDPAddr, data []byte) {
	ctx := context.Background()

	// Process packet (may return nil if still buffering fragments).
	// Returns a buffered packet when:
	//   - We have enough data to cover the public segment, OR
	//   - A verdict already exists for this RPC ID (for fast forwarding)
	// Returns nil if still waiting for more fragments.
	bufferedPacket, existingVerdict, err := state.packetBuffer.ProcessPacket(data, src)
	if err != nil {
		logging.Error("Error processing packet through buffer", zap.Error(err))
		return
	}

	// If bufferedPacket is nil, we're still waiting for more fragments
	if bufferedPacket == nil {
		logging.Debug("Still buffering packet fragments", zap.String("src", src.String()))
		return
	}

	// If verdict exists and it's a drop, don't forward the packet
	if existingVerdict == util.PacketVerdictDrop {
		logging.Debug("Packet dropped due to existing drop verdict", zap.Uint64("rpcID", bufferedPacket.RPCID))
		return
	}

	// If verdict exists (not Unknown), skip element chain and forward the original packet directly
	// This enables fast forwarding of subsequent fragments without re-processing
	if existingVerdict != util.PacketVerdictUnknown {
		logging.Debug("Forwarding packet with preexisting verdict, skipping element chain", zap.Uint64("rpcID", bufferedPacket.RPCID))
	} else {
		// Process packet through the element chain
		err = runElementsChain(ctx, state, bufferedPacket)
		if err != nil {
			logging.Error("Error processing packet through element chain or packet was dropped by an element", zap.Error(err))
			// Send error packet back to the source
			if sendErr := util.SendErrorPacket(conn, bufferedPacket.Source, bufferedPacket.RPCID, err.Error()); sendErr != nil {
				logging.Error("Failed to send error packet", zap.Error(sendErr))
			}
			return
		}
	}

	// Fragment the packet if needed and forward all fragments
	fragmentedPackets, err := state.packetBuffer.FragmentPacketForForward(bufferedPacket)
	if err != nil {
		logging.Error("Failed to fragment packet for forwarding", zap.Error(err))
		return
	}

	// Send all fragments
	// TODO: If WriteToUDP fails for one fragment, we currently return early and remaining fragments
	// are not sent. This causes incomplete message delivery. We should either:
	// 1. Continue sending remaining fragments even if one fails (best effort delivery), or
	// 2. Implement retry logic for failed fragments, or
	// 3. Track which fragments succeeded and retry only failed ones
	for _, fragment := range fragmentedPackets {
		if _, err := conn.WriteToUDP(fragment.Data, fragment.Peer); err != nil {
			logging.Error("WriteToUDP error", zap.Error(err))
			return
		}
	}

	logging.Debug("Forwarded packet",
		zap.Int("fragments", len(fragmentedPackets)),
		zap.Int("bytes", len(bufferedPacket.Payload)),
		zap.String("from", bufferedPacket.Source.String()),
		zap.String("to", bufferedPacket.Peer.String()),
		zap.String("packetType", bufferedPacket.PacketType.String()))

	// Clean up fragments that were used to build the public segment
	// Only cleanup if this was a buffered packet (SeqNumber == -1) and we have LastUsedSeqNum set
	// TODO: After cleanup, process remaining buffered fragments if verdict exists (out-of-order fragment bug)
	if bufferedPacket.SeqNumber == -1 && bufferedPacket.LastUsedSeqNum > 0 {
		connKey := bufferedPacket.Source.String()
		state.packetBuffer.CleanupUsedFragments(connKey, bufferedPacket.RPCID, bufferedPacket.LastUsedSeqNum)
		// TODO: Check for remaining fragments in buffer and process them via fast-forward if verdict exists
		// This fixes the bug where out-of-order fragments (e.g., 0,2,1) leave fragment 2 stuck in buffer
	}
}

// runElementsChain processes the packet through the element chain.
// Modifications to the packet payload are made in place via the processedPacket return value.
// Stores the verdict for future fast forwarding of fragments with the same RPC ID.
// Returns an error if processing fails or if the verdict is PacketVerdictDrop.
func runElementsChain(ctx context.Context, state *ProxyState, packet *util.BufferedPacket) error {
	var err error
	var processedPacket *util.BufferedPacket
	var verdict util.PacketVerdict
	switch packet.PacketType {
	case util.PacketTypeRequest:
		// Process request through element chain
		processedPacket, verdict, _, err = state.elementChain.ProcessRequest(ctx, packet)
	case util.PacketTypeResponse:
		// Process response through element chain (in reverse order)
		processedPacket, verdict, _, err = state.elementChain.ProcessResponse(ctx, packet)
	default:
		// For other packet util (Error, Unknown, etc.), skip processing
		// TODO: Add handler for error packets
		logging.Debug("Skipping element chain processing for packet type", zap.String("packetType", packet.PacketType.String()))
		return nil
	}

	// Store the verdict for this RPC ID and packet type (to distinguish requests from responses)
	key := verdictKey{
		RPCID:      packet.RPCID,
		PacketType: packet.PacketType,
	}
	state.packetBuffer.verdictsMu.Lock()
	state.packetBuffer.verdicts[key] = verdict
	state.packetBuffer.verdictTimes[key] = time.Now()
	state.packetBuffer.verdictsMu.Unlock()

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
