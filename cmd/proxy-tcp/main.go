package main

import (
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
	"github.com/appnet-org/proxy-tcp/element"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"
)

const (
	// DefaultBufferSize is the size of the buffer used for reading data
	DefaultBufferSize = 4096
)

// ProxyState manages the state of the TCP proxy
type ProxyState struct {
	elementChain *element.RPCElementChain
	// Target server address for proxying (optional, can be configured via env)
	// Used only if SO_ORIGINAL_DST is unavailable
	targetAddr string
	// Connection counter for unique IDs
	connCounter uint64
	connMu      sync.Mutex
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

// getNextConnectionID generates a unique connection ID
func (s *ProxyState) getNextConnectionID() string {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	s.connCounter++
	return fmt.Sprintf("conn-%d", s.connCounter)
}

func main() {
	// Initialize logging
	err := logging.Init(getLoggingConfig())
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize logging: %v", err))
	}

	logging.Info("Starting bidirectional TCP proxy on :15002 and :15006...")

	// Create element chain
	elementChain := element.NewRPCElementChain(
	// element.NewLoggingElement(true), // Enable verbose logging if needed
	)

	config := DefaultConfig()

	state := &ProxyState{
		elementChain: elementChain,
		targetAddr:   config.TargetAddr,
		connCounter:  0,
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

// handleConnection processes a TCP connection and forwards traffic
func handleConnection(clientConn net.Conn, state *ProxyState) {
	defer clientConn.Close()

	connID := state.getNextConnectionID()
	logging.Debug("New TCP connection",
		zap.String("connID", connID),
		zap.String("clientAddr", clientConn.RemoteAddr().String()))

	// Get the original destination from iptables interception
	targetAddr := state.targetAddr
	if origDst, ok := getOriginalDestination(clientConn); ok {
		targetAddr = origDst
		logging.Debug("Using iptables original destination",
			zap.String("connID", connID),
			zap.String("original_dst", origDst))
	} else if targetAddr == "" {
		logging.Error("No target address available (neither SO_ORIGINAL_DST nor TARGET_ADDR)",
			zap.String("connID", connID))
		return
	}

	// Connect to target server
	targetConn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		logging.Error("Failed to connect to target",
			zap.String("connID", connID),
			zap.String("target", targetAddr),
			zap.Error(err))
		return
	}
	defer targetConn.Close()

	logging.Debug("Connected to target",
		zap.String("connID", connID),
		zap.String("target", targetAddr))

	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(2)

	// Handle client -> target (requests)
	go func() {
		defer wg.Done()
		handleTCPStream(clientConn, targetConn, state, ctx, true, connID, targetAddr)
	}()

	// Handle target -> client (responses)
	go func() {
		defer wg.Done()
		handleTCPStream(targetConn, clientConn, state, ctx, false, connID, targetAddr)
	}()

	wg.Wait()
	logging.Debug("Connection closed", zap.String("connID", connID))
}

// processDataThroughElementChain processes data through the element chain
// Returns verdict and whether the data should be forwarded (false if dropped)
func processDataThroughElementChain(ctx context.Context, state *ProxyState, connID string, data []byte, isRequest bool, remoteAddr, targetAddr string) (element.Verdict, []byte, bool) {
	// Create a copy of the data for processing
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	// Create TCPRPCContext for element chain processing
	rpcCtx := &element.TCPRPCContext{
		Data:         dataCopy,
		ConnectionID: connID,
		IsRequest:    isRequest,
		RemoteAddr:   remoteAddr,
		TargetAddr:   targetAddr,
	}

	// Process through element chain
	var verdict element.Verdict
	var err error
	if isRequest {
		verdict, ctx, err = state.elementChain.ProcessRequest(ctx, rpcCtx)
	} else {
		verdict, ctx, err = state.elementChain.ProcessResponse(ctx, rpcCtx)
	}

	if err != nil {
		logging.Error("Error processing data through element chain",
			zap.String("connID", connID),
			zap.Bool("isRequest", isRequest),
			zap.Error(err))
		// On error, pass through (don't drop)
		return element.VerdictPass, data, true
	}

	if verdict == element.VerdictDrop {
		logging.Debug("Data dropped by element chain",
			zap.String("connID", connID),
			zap.Bool("isRequest", isRequest),
			zap.String("verdict", verdict.String()))
		return element.VerdictDrop, nil, false
	}

	// Return the potentially modified data
	return element.VerdictPass, rpcCtx.Data, true
}

// handleTCPStream processes TCP data in one direction
func handleTCPStream(srcConn net.Conn, dstConn io.Writer, state *ProxyState, ctx context.Context, isRequest bool, connID, targetAddr string) {
	buffer := make([]byte, DefaultBufferSize)
	direction := "request"
	if !isRequest {
		direction = "response"
	}

	for {
		n, err := srcConn.Read(buffer)
		if err != nil {
			if err != io.EOF {
				logging.Debug("Read error",
					zap.String("connID", connID),
					zap.String("direction", direction),
					zap.Error(err))
			}
			return
		}

		if n == 0 {
			continue
		}

		data := buffer[:n]
		logging.Debug("Received TCP data",
			zap.String("connID", connID),
			zap.String("direction", direction),
			zap.Int("dataLen", n))

		// Process through element chain
		verdict, processedData, shouldForward := processDataThroughElementChain(
			ctx, state, connID, data, isRequest,
			srcConn.RemoteAddr().String(), targetAddr)

		if !shouldForward {
			// Data was dropped by element chain
			logging.Debug("Data dropped, not forwarding",
				zap.String("connID", connID),
				zap.String("direction", direction),
				zap.String("verdict", verdict.String()))
			continue
		}

		// Forward processed data to destination
		if len(processedData) > 0 {
			written, err := dstConn.Write(processedData)
			if err != nil {
				logging.Error("Write error",
					zap.String("connID", connID),
					zap.String("direction", direction),
					zap.Error(err))
				return
			}
			if written != len(processedData) {
				logging.Warn("Partial write",
					zap.String("connID", connID),
					zap.String("direction", direction),
					zap.Int("expected", len(processedData)),
					zap.Int("written", written))
			}
			logging.Debug("Forwarded TCP data",
				zap.String("connID", connID),
				zap.String("direction", direction),
				zap.Int("dataLen", len(processedData)))
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

