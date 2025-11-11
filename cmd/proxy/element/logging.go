package element

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"

	kv "github.com/appnet-org/arpc/benchmark/kv-store-symphony/symphony"
	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/proxy/types"
	"go.uber.org/zap"
)

// LoggingElement implements RPCElement to provide logging functionality
type LoggingElement struct{}

// NewLoggingElement creates a new logging element
func NewLoggingElement() *LoggingElement {
	return &LoggingElement{}
}

// RequestMode returns the execution mode for processing requests
func (l *LoggingElement) RequestMode() types.ExecutionMode {
	return types.FullBufferingMode
}

// ResponseMode returns the execution mode for processing responses
func (l *LoggingElement) ResponseMode() types.ExecutionMode {
	return types.FullBufferingMode
}

// parseKVPayload parses the payload as a KV service message in Symphony format
// Returns fields for logging, or nil if parsing fails
func parseKVPayload(service, method string, payload []byte, isRequest bool) []zap.Field {
	if len(payload) == 0 {
		return nil
	}

	// Only handle KVService
	if service != "KVService" {
		return nil
	}

	var fields []zap.Field
	parseErr := parseKVSymphonyPayload(service, method, payload, isRequest, &fields)

	if parseErr != nil {
		// Parsing failed, log the error
		return []zap.Field{
			zap.String("parse_error", parseErr.Error()),
		}
	}

	return fields
}

// parseKVSymphonyPayload parses KV messages in Symphony format using generated UnmarshalSymphony methods
func parseKVSymphonyPayload(service, method string, payload []byte, isRequest bool, fields *[]zap.Field) error {
	if service != "KVService" {
		return fmt.Errorf("not a KV service")
	}

	// Use the generated UnmarshalSymphony methods to parse the payload
	if isRequest {
		switch method {
		case "Get":
			req := &kv.GetRequest{}
			if err := req.UnmarshalSymphony(payload); err != nil {
				return fmt.Errorf("failed to unmarshal GetRequest: %w", err)
			}
			*fields = append(*fields, zap.String("key", req.Key))
		case "Set":
			req := &kv.SetRequest{}
			if err := req.UnmarshalSymphony(payload); err != nil {
				return fmt.Errorf("failed to unmarshal SetRequest: %w", err)
			}
			*fields = append(*fields, zap.String("key", req.Key))
			*fields = append(*fields, zap.Int("value_len", len(req.Value)))
			if len(req.Value) <= 64 {
				*fields = append(*fields, zap.String("value", req.Value))
			}
		default:
			return fmt.Errorf("unknown method: %s", method)
		}
	} else {
		// Response: GetResponse or SetResponse both have field 1 (Value)
		switch method {
		case "Get", "Set":
			resp := &kv.GetResponse{} // Both GetResponse and SetResponse have the same structure (field 1: Value)
			if err := resp.UnmarshalSymphony(payload); err != nil {
				// Try SetResponse if GetResponse fails (though they should be the same)
				setResp := &kv.SetResponse{}
				if err2 := setResp.UnmarshalSymphony(payload); err2 != nil {
					return fmt.Errorf("failed to unmarshal response: GetResponse error: %w, SetResponse error: %v", err, err2)
				}
				*fields = append(*fields, zap.Int("value_len", len(setResp.Value)))
				if len(setResp.Value) <= 64 {
					*fields = append(*fields, zap.String("value", setResp.Value))
				}
			} else {
				*fields = append(*fields, zap.Int("value_len", len(resp.Value)))
				if len(resp.Value) <= 64 {
					*fields = append(*fields, zap.String("value", resp.Value))
				}
			}
		default:
			return fmt.Errorf("unknown method: %s", method)
		}
	}

	return nil
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
func (l *LoggingElement) ProcessRequest(ctx context.Context, packet *types.BufferedPacket) (*types.BufferedPacket, context.Context, error) {
	if packet == nil || len(packet.Payload) == 0 {
		logging.Info("Received empty request")
		return packet, ctx, nil
	}

	// Parse the request
	service, method, metadataLen, payload, err := parseFramedRequest(packet.Payload)
	if err != nil {
		// If parsing fails, fall back to hex dump
		logging.Debug("Request payload (parse failed)",
			zap.Error(err),
			zap.String("hex", hex.EncodeToString(packet.Payload)))
		return packet, ctx, nil
	}

	// Log parsed request information
	logFields := []zap.Field{
		zap.String("service", service),
		zap.String("method", method),
		zap.Int("payload_size", len(payload)),
		zap.Int("metadata_size", metadataLen),
		zap.Int("total_size", len(packet.Payload)),
		zap.Uint64("rpcID", packet.RPCID),
		zap.String("packetType", packet.PacketType.String()),
	}

	// Try to parse KV payload for structured logging
	if kvFields := parseKVPayload(service, method, payload, true); kvFields != nil {
		logFields = append(logFields, kvFields...)
	} else {
		// Fallback: Include payload hex for debugging (first 128 bytes to avoid huge logs)
		if len(payload) > 0 {
			hexLen := min(len(payload), 128)
			logFields = append(logFields, zap.String("payload_hex_preview", hex.EncodeToString(payload[:hexLen])))
		}
	}

	logging.Debug("Request received", logFields...)

	return packet, ctx, nil
}

// ProcessResponse logs the outgoing response and returns it unchanged
func (l *LoggingElement) ProcessResponse(ctx context.Context, packet *types.BufferedPacket) (*types.BufferedPacket, context.Context, error) {
	if packet == nil || len(packet.Payload) == 0 {
		logging.Info("Received empty response")
		return packet, ctx, nil
	}

	// Parse the response
	service, method, payload, err := parseFramedResponse(packet.Payload)
	if err != nil {
		// If parsing fails, fall back to hex dump
		logging.Debug("Response payload (parse failed)",
			zap.Error(err),
			zap.String("hex", hex.EncodeToString(packet.Payload)))
		return packet, ctx, nil
	}

	// Log parsed response information
	logFields := []zap.Field{
		zap.String("service", service),
		zap.String("method", method),
		zap.Int("payload_size", len(payload)),
		zap.Int("total_size", len(packet.Payload)),
		zap.Uint64("rpcID", packet.RPCID),
		zap.String("packetType", packet.PacketType.String()),
	}

	// Try to parse KV payload for structured logging
	if kvFields := parseKVPayload(service, method, payload, false); kvFields != nil {
		logFields = append(logFields, kvFields...)
	} else {
		// Fallback: Include payload hex for debugging (first 128 bytes to avoid huge logs)
		if len(payload) > 0 {
			hexLen := min(len(payload), 128)
			logFields = append(logFields, zap.String("payload_hex_preview", hex.EncodeToString(payload[:hexLen])))
		}
	}

	logging.Debug("Response sent", logFields...)

	return packet, ctx, nil
}

// Name returns the name of this element
func (l *LoggingElement) Name() string {
	return "LoggingElement"
}
