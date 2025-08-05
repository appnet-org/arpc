package element

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
)

// LoggingElement implements RPCElement to provide logging functionality
type LoggingElement struct {
	verbose bool
}

// NewLoggingElement creates a new logging element
func NewLoggingElement(verbose bool) *LoggingElement {
	return &LoggingElement{
		verbose: verbose,
	}
}

// ProcessRequest logs the incoming request and returns it unchanged
func (l *LoggingElement) ProcessRequest(ctx context.Context, req []byte) ([]byte, error) {
	if len(req) == 0 {
		log.Printf("[LOGGING] Received empty request")
		return req, nil
	}

	// Parse basic packet information
	packetType := req[0]
	rpcID, service, method, err := l.parseMetadata(req)
	if err != nil {
		log.Printf("[LOGGING] Error parsing request metadata: %v", err)
		return req, nil // Continue processing even if parsing fails
	}

	log.Printf("[LOGGING] REQUEST | Type: %d | RPC ID: %d | Service: %s | Method: %s | Size: %d bytes",
		packetType, rpcID, service, method, len(req))

	if l.verbose {
		log.Printf("[LOGGING] Request payload (hex): %s", hex.EncodeToString(req))
	}

	return req, nil
}

// ProcessResponse logs the outgoing response and returns it unchanged
func (l *LoggingElement) ProcessResponse(ctx context.Context, resp []byte) ([]byte, error) {
	if len(resp) == 0 {
		log.Printf("[LOGGING] Received empty response")
		return resp, nil
	}

	// Parse basic packet information
	packetType := resp[0]
	rpcID, service, method, err := l.parseMetadata(resp)
	if err != nil {
		log.Printf("[LOGGING] Error parsing response metadata: %v", err)
		return resp, nil // Continue processing even if parsing fails
	}

	log.Printf("[LOGGING] RESPONSE | Type: %d | RPC ID: %d | Service: %s | Method: %s | Size: %d bytes",
		packetType, rpcID, service, method, len(resp))

	if l.verbose {
		log.Printf("[LOGGING] Response payload (hex): %s", hex.EncodeToString(resp))
	}

	return resp, nil
}

// Name returns the name of this element
func (l *LoggingElement) Name() string {
	return "LoggingElement"
}

// parseMetadata extracts RPC metadata from the packet
func (l *LoggingElement) parseMetadata(data []byte) (uint64, string, string, error) {
	if len(data) < 13 {
		return 0, "", "", fmt.Errorf("packet too short: %d bytes", len(data))
	}

	offset := uint16(1)
	rpcID := binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 12

	if offset+2 > uint16(len(data)) {
		return rpcID, "", "", fmt.Errorf("packet too short for service length")
	}

	serviceLen := binary.LittleEndian.Uint16(data[offset : offset+2])
	offset += 2

	if offset+serviceLen > uint16(len(data)) {
		return rpcID, "", "", fmt.Errorf("service length %d exceeds packet size", serviceLen)
	}

	service := string(data[offset : offset+serviceLen])
	offset += serviceLen

	if offset+2 > uint16(len(data)) {
		return rpcID, service, "", fmt.Errorf("packet too short for method length")
	}

	methodLen := binary.LittleEndian.Uint16(data[offset : offset+2])
	offset += 2

	if offset+methodLen > uint16(len(data)) {
		return rpcID, service, "", fmt.Errorf("method length %d exceeds packet size", methodLen)
	}

	method := string(data[offset : offset+methodLen])

	return rpcID, service, method, nil
}
