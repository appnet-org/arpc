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

	"github.com/appnet-org/arpc/cmd/proxy/util"
	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/packet"
	"github.com/appnet-org/arpc/pkg/transport"
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
		// Initialize GCM objects with default keys
		if err := transport.InitGCMObjects(transport.DefaultPublicKey, transport.DefaultPrivateKey); err != nil {
			panic(fmt.Sprintf("Failed to initialize encryption: %v", err))
		}
	} else {
		c.EnableEncryption = true
		c.EncryptionKey = key
		// Initialize GCM objects with provided public key and default private key
		if err := transport.InitGCMObjects(key, transport.DefaultPrivateKey); err != nil {
			panic(fmt.Sprintf("Failed to initialize encryption: %v", err))
		}
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

	// Initialize dynamic element loader
	InitElementLoader(ElementPluginDir + "/" + GetElementPluginPrefix())

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
		// TODO: We could pre-allocate the buffer and reuse it for each packet
		data := make([]byte, n)
		copy(data, buf[:n])

		go handlePacket(conn, state, src, data, config)
	}
}

// handlePacket processes incoming packets and forwards them to the appropriate peer
func handlePacket(conn *net.UDPConn, state *ProxyState, src *net.UDPAddr, data []byte, config *Config) {
	ctx := context.Background()

	// Check if this is an error packet (PacketTypeID == 3)
	if len(data) > 0 && data[0] == byte(packet.PacketTypeError.TypeID) {
		// Process error packet - forward directly without element chain
		bufferedPacket, err := state.packetBuffer.ProcessErrorPacket(data, src)
		if err != nil {
			logging.Error("Error processing error packet", zap.Error(err))
			return
		}

		// Serialize the error packet for forwarding
		errorPacket := &packet.ErrorPacket{
			PacketTypeID: packet.PacketTypeError.TypeID,
			RPCID:        bufferedPacket.RPCID,
			DstIP:        bufferedPacket.DstIP,
			DstPort:      bufferedPacket.DstPort,
			SrcIP:        bufferedPacket.SrcIP,
			SrcPort:      bufferedPacket.SrcPort,
			ErrorMsg:     string(bufferedPacket.Payload),
		}

		codec := &packet.ErrorPacketCodec{}
		serialized, err := codec.Serialize(errorPacket, nil)
		if err != nil {
			logging.Error("Failed to serialize error packet for forwarding", zap.Error(err))
			return
		}

		// Forward the error packet to the destination
		if _, err := conn.WriteToUDP(serialized, bufferedPacket.Peer); err != nil {
			logging.Error("Failed to forward error packet", zap.Error(err))
			return
		}

		logging.Debug("Forwarded error packet",
			zap.Uint64("rpcID", bufferedPacket.RPCID),
			zap.String("from", bufferedPacket.Source.String()),
			zap.String("to", bufferedPacket.Peer.String()),
			zap.String("errorMsg", string(bufferedPacket.Payload)))

		return
	}

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

	// If bufferedPacket is nil, we're still waiting for more fragments.
	// Check if a verdict exists and forward any buffered fragments that are ready.
	if bufferedPacket == nil {
		logging.Debug("Still buffering packet fragments", zap.String("src", src.String()))
		tryForwardBufferedFragmentsFromRawPacket(conn, state, src, data, config)
		return
	}

	// If verdict exists and it's a drop, don't forward the packet
	if existingVerdict == util.PacketVerdictDrop {
		logging.Debug("Packet dropped due to existing drop verdict", zap.Uint64("rpcID", bufferedPacket.RPCID))
		return
	}

	payload := bufferedPacket.Payload
	publicPayload := payload
	privatePayload := []byte{}

	// Only decrypt/split if this is the reassembled public segment (SeqNumber == -1)
	// Fragments (SeqNumber >= 0) should be forwarded as-is without decryption
	if bufferedPacket.SeqNumber == -1 {
		// Split the payload into public and private segments
		if len(payload) > offsetToPrivate(payload) {
			logging.Debug("Splitting payload into public and private segments", zap.Int("size", len(payload)), zap.Int("offsetToPrivate", offsetToPrivate(payload)))
			publicPayload = payload[:offsetToPrivate(payload)]
			privatePayload = payload[offsetToPrivate(payload):]
		}

		// Decrypt the public segment if encryption is enabled
		if config.EnableEncryption {
			publicPayload = transport.DecryptSymphonyData(publicPayload, config.EncryptionKey, nil)
			logging.Debug("Public segment decrypted", zap.Int("size", len(publicPayload)), zap.String("publicPayload", string(publicPayload)))
			logging.Debug("offsetToPrivate", zap.Int("offsetToPrivate", offsetToPrivate(publicPayload)))
		}

		// Update the packet with the decrypted public segment
		bufferedPacket.Payload = publicPayload
	} else {
		// This is a fragment being fast-forwarded - keep payload as-is (already encrypted)
		logging.Debug("Fast-forwarding fragment without decryption", zap.Int16("seqNumber", bufferedPacket.SeqNumber), zap.Uint64("rpcID", bufferedPacket.RPCID))
	}

	// Track if verdict was just stored (to know if we should process remaining fragments)
	verdictJustStored := false

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
			if sendErr := util.SendErrorPacket(conn, bufferedPacket.Source, bufferedPacket.RPCID, err.Error(), bufferedPacket.SrcIP, bufferedPacket.SrcPort, bufferedPacket.DstIP, bufferedPacket.DstPort); sendErr != nil {
				logging.Error("Failed to send error packet", zap.Error(sendErr))
			}
			return
		}
		verdictJustStored = true
	}

	// Encrypt the packet if encryption is enabled
	// Only encrypt if we decrypted it (i.e., SeqNumber == -1)
	// Fragments (SeqNumber >= 0) are already encrypted and should be forwarded as-is
	if bufferedPacket.SeqNumber == -1 {
		// Encrypt the packet if encryption is enabled
		if config.EnableEncryption {
			bufferedPacket.Payload = transport.EncryptSymphonyData(bufferedPacket.Payload, config.EncryptionKey, nil)
		}

		// Append the private segment to the packet
		bufferedPacket.Payload = append(bufferedPacket.Payload, privatePayload...)
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
		zap.Uint64("rpcID", bufferedPacket.RPCID),
		zap.String("from", bufferedPacket.Source.String()),
		zap.String("to", bufferedPacket.Peer.String()),
		zap.String("packetType", bufferedPacket.PacketType.String()))

	// Clean up fragments that were used to build the public segment
	// Only cleanup if this was a buffered packet (SeqNumber == -1) and we have LastUsedSeqNum set
	connKey := bufferedPacket.Source.String()
	if bufferedPacket.SeqNumber == -1 {
		state.packetBuffer.CleanupUsedFragments(connKey, bufferedPacket.RPCID, bufferedPacket.LastUsedSeqNum)
	}

	// After processing any packet (public segment or fast-forwarded fragment), check for remaining buffered fragments
	// This fixes the bug where fragments arrive after ProcessRemainingFragments was called but remain stuck in buffer
	// Only process if verdict exists (was just stored or already existed) and is not dropped
	finalVerdict := existingVerdict
	if verdictJustStored {
		// Verdict was just stored, so it's Pass (drop verdicts return early)
		finalVerdict = util.PacketVerdictPass
	}
	// After processing, forward any remaining buffered fragments that arrived while we were processing
	if finalVerdict == util.PacketVerdictPass {
		forwardBufferedFragments(conn, state, connKey, bufferedPacket.RPCID, bufferedPacket.PacketType, bufferedPacket, config)
	}
}

// forwardBufferedFragments retrieves and forwards all remaining buffered fragments for an RPC.
// ProcessRemainingFragments atomically returns all fragments and removes them from the buffer,
// so a single call is sufficient.
func forwardBufferedFragments(conn *net.UDPConn, state *ProxyState, connKey string, rpcID uint64, packetType util.PacketType, metadata *util.BufferedPacket, config *Config) {
	fragments := state.packetBuffer.ProcessRemainingFragments(connKey, rpcID, packetType, metadata)
	for _, fragment := range fragments {
		if err := processFragmentViaFastForward(conn, state, fragment, config); err != nil {
			logging.Error("Error forwarding buffered fragment",
				zap.Error(err),
				zap.Uint64("rpcID", fragment.RPCID),
				zap.Int16("seqNum", fragment.SeqNumber))
		}
	}
}

// tryForwardBufferedFragmentsFromRawPacket attempts to forward buffered fragments using raw packet data.
// This is called when a packet fragment arrives but we're still waiting for more data.
// If a verdict already exists for this RPC, we can forward any buffered fragments immediately.
func tryForwardBufferedFragmentsFromRawPacket(conn *net.UDPConn, state *ProxyState, src *net.UDPAddr, data []byte, config *Config) {
	dataPacket, err := state.packetBuffer.deserializePacket(data)
	if err != nil {
		return
	}

	connKey := src.String()
	packetType := util.PacketType(dataPacket.PacketTypeID)
	peer := &net.UDPAddr{IP: net.IP(dataPacket.DstIP[:]), Port: int(dataPacket.DstPort)}

	metadata := &util.BufferedPacket{
		Source:     src,
		Peer:       peer,
		PacketType: packetType,
		RPCID:      dataPacket.RPCID,
		DstIP:      dataPacket.DstIP,
		DstPort:    dataPacket.DstPort,
		SrcIP:      dataPacket.SrcIP,
		SrcPort:    dataPacket.SrcPort,
	}

	forwardBufferedFragments(conn, state, connKey, dataPacket.RPCID, packetType, metadata, config)
}

// processFragmentViaFastForward processes a fragment via the fast-forward path.
// It encrypts if needed, serializes it, and forwards it without element chain processing.
func processFragmentViaFastForward(conn *net.UDPConn, state *ProxyState, fragment *util.BufferedPacket, config *Config) error {
	// Serialize and forward the fragment
	fragmentedPackets, err := state.packetBuffer.FragmentPacketForForward(fragment)
	if err != nil {
		return fmt.Errorf("failed to fragment packet for forwarding: %w", err)
	}

	// Send all fragments
	for _, fp := range fragmentedPackets {
		if _, err := conn.WriteToUDP(fp.Data, fp.Peer); err != nil {
			return fmt.Errorf("WriteToUDP error: %w", err)
		}
	}

	logging.Debug("Forwarded remaining fragment via fast-forward",
		zap.Uint64("rpcID", fragment.RPCID),
		zap.Int16("seqNum", fragment.SeqNumber),
		zap.Int("fragments", len(fragmentedPackets)))

	return nil
}

// runElementsChain processes the packet through the element chain.
// Modifications to the packet payload are made in place via the processedPacket return value.
// Stores the verdict for future fast forwarding of fragments with the same RPC ID.
// Returns an error if processing fails or if the verdict is PacketVerdictDrop.
func runElementsChain(ctx context.Context, state *ProxyState, packet *util.BufferedPacket) error {
	// Get current element chain (may have been updated by plugin loader)
	elementChain := GetElementChain()
	var err error
	var processedPacket *util.BufferedPacket
	var verdict util.PacketVerdict

	if elementChain == nil {
		// No element chain available, pass through with Pass verdict
		logging.Debug("No element chain available, passing packet through")
		verdict = util.PacketVerdictPass
	} else {
		switch packet.PacketType {
		case util.PacketTypeRequest:
			// Process request through element chain
			processedPacket, verdict, _, err = elementChain.ProcessRequest(ctx, packet)
		case util.PacketTypeResponse:
			// Process response through element chain (in reverse order)
			processedPacket, verdict, _, err = elementChain.ProcessResponse(ctx, packet)
		default:
			// For other packet util (Error, Unknown, etc.), skip processing
			// TODO: Add handler for error packets
			logging.Debug("Skipping element chain processing for packet type", zap.String("packetType", packet.PacketType.String()))
			verdict = util.PacketVerdictPass
		}
	}

	// Store the verdict for this RPC ID and packet type (to distinguish requests from responses)
	// This is critical for fast-forwarding remaining fragments after public segment processing
	state.packetBuffer.StoreVerdict(packet.RPCID, packet.PacketType, verdict)

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
