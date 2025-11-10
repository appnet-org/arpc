package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

func main() {
	port := 15002
	if len(os.Args) > 1 {
		fmt.Sscanf(os.Args[1], "%d", &port)
	}

	log.Printf("Starting simple transparent proxy on port %d...", port)
	log.Printf("This proxy will intercept traffic and forward it to the original destination")

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatalf("Failed to listen on port %d: %v", port, err)
	}
	defer listener.Close()

	log.Printf("Listening on :%d", port)

	// Handle shutdown gracefully
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down proxy...")
		listener.Close()
		os.Exit(0)
	}()

	for {
		clientConn, err := listener.Accept()
		if err != nil {
			log.Printf("Accept error: %v", err)
			continue
		}

		go handleConnection(clientConn)
	}
}

func handleConnection(clientConn net.Conn) {
	defer clientConn.Close()

	clientAddr := clientConn.RemoteAddr().String()

	// Get the original destination from iptables
	origDst, err := getOriginalDestination(clientConn)
	if err != nil {
		log.Printf("[%s] Failed to get original destination: %v", clientAddr, err)
		return
	}

	log.Printf("[%s] -> [%s] New connection", clientAddr, origDst)

	// Connect to the original destination
	targetConn, err := net.Dial("tcp", origDst)
	if err != nil {
		log.Printf("[%s] -> [%s] Failed to connect: %v", clientAddr, origDst, err)
		return
	}
	defer targetConn.Close()

	log.Printf("[%s] -> [%s] Connected successfully", clientAddr, origDst)

	// Bidirectional forwarding
	done := make(chan error, 2)

	// Client -> Target
	go func() {
		n, err := io.Copy(targetConn, clientConn)
		log.Printf("[%s] -> [%s] Client to target closed (%d bytes): %v", clientAddr, origDst, n, err)
		targetConn.(*net.TCPConn).CloseWrite()
		done <- err
	}()

	// Target -> Client
	go func() {
		n, err := io.Copy(clientConn, targetConn)
		log.Printf("[%s] <- [%s] Target to client closed (%d bytes): %v", clientAddr, origDst, n, err)
		clientConn.(*net.TCPConn).CloseWrite()
		done <- err
	}()

	// Wait for both directions to complete
	<-done
	<-done

	log.Printf("[%s] <-> [%s] Connection closed", clientAddr, origDst)
}

func getOriginalDestination(conn net.Conn) (string, error) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return "", fmt.Errorf("not a TCP connection")
	}

	file, err := tcpConn.File()
	if err != nil {
		return "", fmt.Errorf("failed to get file descriptor: %w", err)
	}
	defer file.Close()

	fd := file.Fd()

	// Get SO_ORIGINAL_DST (set by iptables REDIRECT)
	var sockaddr [128]byte
	size := uint32(len(sockaddr))

	err = getSockopt(int(fd), syscall.IPPROTO_IP, unix.SO_ORIGINAL_DST,
		unsafe.Pointer(&sockaddr[0]), &size)
	if err != nil {
		return "", fmt.Errorf("SO_ORIGINAL_DST failed: %w", err)
	}

	if size < 8 {
		return "", fmt.Errorf("invalid sockaddr size: %d", size)
	}

	// Parse sockaddr_in structure
	// [2 bytes: family][2 bytes: port][4 bytes: IP][8 bytes: padding]
	family := uint16(sockaddr[0]) | uint16(sockaddr[1])<<8
	if family != syscall.AF_INET {
		return "", fmt.Errorf("unsupported address family: %d", family)
	}

	// Port is in network byte order (big-endian)
	port := uint16(sockaddr[2])<<8 | uint16(sockaddr[3])

	// IP address
	ip := net.IPv4(sockaddr[4], sockaddr[5], sockaddr[6], sockaddr[7])

	return fmt.Sprintf("%s:%d", ip.String(), port), nil
}

func getSockopt(s, level, name int, val unsafe.Pointer, vallen *uint32) error {
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
		return e1
	}
	return nil
}
