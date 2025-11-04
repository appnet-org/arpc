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
type LoggingElement struct{}

// NewLoggingElement creates a new logging element
func NewLoggingElement() *LoggingElement {
	return &LoggingElement{}
}

// parseFramedRequest parses a framed request into its components
// Format: [serviceLen(2B)][service][methodLen(2B)][method][metadataLen(2B)][metadata][payload]
func parseFramedRequest(data []byte) (service string, method string, metadataLen int, payload []byte, err error) {
	offset := 0

	// Service
	if offset+2 > len(data) {
		return "", "", 0, nil, fmt.Errorf("data too short for service length")
	}
	serviceLen := int(binary.LittleEndian.Uint16(data[offset:]))
	offset += 2
	if offset+serviceLen > len(data) {
		return "", "", 0, nil, fmt.Errorf("service length %d exceeds data length", serviceLen)
	}
	service = string(data[offset : offset+serviceLen])
	offset += serviceLen

	// Method
	if offset+2 > len(data) {
		return "", "", 0, nil, fmt.Errorf("data too short for method length")
	}
	methodLen := int(binary.LittleEndian.Uint16(data[offset:]))
	offset += 2
	if offset+methodLen > len(data) {
		return "", "", 0, nil, fmt.Errorf("method length %d exceeds data length", methodLen)
	}
	method = string(data[offset : offset+methodLen])
	offset += methodLen

	// Metadata
	if offset+2 > len(data) {
		return "", "", 0, nil, fmt.Errorf("data too short for metadata length")
	}
	metadataLen = int(binary.LittleEndian.Uint16(data[offset:]))
	offset += 2

	if metadataLen > 0 {
		if offset+metadataLen > len(data) {
			return "", "", 0, nil, fmt.Errorf("metadata length %d exceeds data length", metadataLen)
		}
		offset += metadataLen
	}

	// Payload
	payload = data[offset:]

	return service, method, metadataLen, payload, nil
}

// parseFramedResponse parses a framed response into its components
// Format: [serviceLen(2B)][service][methodLen(2B)][method][payload]
func parseFramedResponse(data []byte) (service string, method string, payload []byte, err error) {
	offset := 0

	// Service
	if offset+2 > len(data) {
		return "", "", nil, fmt.Errorf("data too short for service length")
	}
	serviceLen := int(binary.LittleEndian.Uint16(data[offset:]))
	offset += 2
	if offset+serviceLen > len(data) {
		return "", "", nil, fmt.Errorf("service length %d exceeds data length", serviceLen)
	}
	service = string(data[offset : offset+serviceLen])
	offset += serviceLen

	// Method
	if offset+2 > len(data) {
		return "", "", nil, fmt.Errorf("data too short for method length")
	}
	methodLen := int(binary.LittleEndian.Uint16(data[offset:]))
	offset += 2
	if offset+methodLen > len(data) {
		return "", "", nil, fmt.Errorf("method length %d exceeds data length", methodLen)
	}
	method = string(data[offset : offset+methodLen])
	offset += methodLen

	// Payload
	payload = data[offset:]

	return service, method, payload, nil
}

// ProcessRequest logs the incoming request and returns it unchanged
func (l *LoggingElement) ProcessRequest(ctx context.Context, req []byte) ([]byte, context.Context, error) {
	if len(req) == 0 {
		logging.Info("Received empty request")
		return req, ctx, nil
	}

	// Parse the request
	service, method, metadataLen, payload, err := parseFramedRequest(req)
	if err != nil {
		// If parsing fails, fall back to hex dump
		logging.Debug("Request payload (parse failed)",
			zap.Error(err),
			zap.String("hex", hex.EncodeToString(req)))
		return req, ctx, nil
	}

	// Log parsed request information
	logFields := []zap.Field{
		zap.String("service", service),
		zap.String("method", method),
		zap.Int("payload_size", len(payload)),
		zap.Int("metadata_size", metadataLen),
		zap.Int("total_size", len(req)),
	}

	// Include payload hex for debugging (first 128 bytes to avoid huge logs)
	if len(payload) > 0 {
		hexLen := min(len(payload), 128)
		logFields = append(logFields, zap.String("payload_hex_preview", hex.EncodeToString(payload[:hexLen])))
	}

	logging.Debug("Request received", logFields...)

	return req, ctx, nil
}

// ProcessResponse logs the outgoing response and returns it unchanged
func (l *LoggingElement) ProcessResponse(ctx context.Context, resp []byte) ([]byte, context.Context, error) {
	if len(resp) == 0 {
		logging.Info("Received empty response")
		return resp, ctx, nil
	}

	// Parse the response
	service, method, payload, err := parseFramedResponse(resp)
	if err != nil {
		// If parsing fails, fall back to hex dump
		logging.Debug("Response payload (parse failed)",
			zap.Error(err),
			zap.String("hex", hex.EncodeToString(resp)))
		return resp, ctx, nil
	}

	// Log parsed response information
	logFields := []zap.Field{
		zap.String("service", service),
		zap.String("method", method),
		zap.Int("payload_size", len(payload)),
		zap.Int("total_size", len(resp)),
	}

	// Include payload hex for debugging (first 128 bytes to avoid huge logs)
	if len(payload) > 0 {
		hexLen := min(len(payload), 128)
		logFields = append(logFields, zap.String("payload_hex_preview", hex.EncodeToString(payload[:hexLen])))
	}

	logging.Debug("Response sent", logFields...)

	return resp, ctx, nil
}

// Name returns the name of this element
func (l *LoggingElement) Name() string {
	return "LoggingElement"
}
