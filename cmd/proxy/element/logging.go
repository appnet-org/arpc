package element

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"

	"github.com/appnet-org/arpc/pkg/logging"
	"go.uber.org/zap"
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
		logging.Info("Received empty request")
		return req, nil
	}

	// Parse basic packet information
	packetType := req[0]
	rpcID, service, method, err := l.parseMetadata(req)
	if err != nil {
		logging.Warn("Error parsing request metadata", zap.Error(err))
		return req, nil // Continue processing even if parsing fails
	}

	logging.Info("REQUEST",
		zap.Uint8("type", packetType),
		zap.Uint64("rpc_id", rpcID),
		zap.String("service", service),
		zap.String("method", method),
		zap.Int("size_bytes", len(req)),
	)

	if l.verbose {
		logging.Debug("Request payload", zap.String("hex", hex.EncodeToString(req)))
	}

	return req, nil
}

// ProcessResponse logs the outgoing response and returns it unchanged
func (l *LoggingElement) ProcessResponse(ctx context.Context, resp []byte) ([]byte, error) {
	if len(resp) == 0 {
		logging.Info("Received empty response")
		return resp, nil
	}

	// Parse basic packet information
	packetType := resp[0]
	rpcID, service, method, err := l.parseMetadata(resp)
	if err != nil {
		logging.Warn("Error parsing response metadata", zap.Error(err))
		return resp, nil // Continue processing even if parsing fails
	}

	logging.Info("RESPONSE",
		zap.Uint8("type", packetType),
		zap.Uint64("rpc_id", rpcID),
		zap.String("service", service),
		zap.String("method", method),
		zap.Int("size_bytes", len(resp)),
	)

	if l.verbose {
		logging.Debug("Response payload", zap.String("hex", hex.EncodeToString(resp)))
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

	// Packet header
	offset := uint16(1)
	rpcID := binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8
	totalPackets := binary.LittleEndian.Uint16(data[offset : offset+2])
	offset += 2
	seqNumber := binary.LittleEndian.Uint16(data[offset : offset+2])
	offset += 2
	logging.Debug("Total packets", zap.Uint16("total_packets", totalPackets), zap.Uint16("seq_number", seqNumber))

	// Message header
	payloadLen := binary.LittleEndian.Uint32(data[offset : offset+4])
	logging.Debug("Payload length", zap.Uint32("payload_len", payloadLen))
	offset += 4

	offset += 8 // account for peer address and source port

	//TODO(xz): the code below is not correct, fix it.

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
