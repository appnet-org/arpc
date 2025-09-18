package rpc

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"

	"github.com/appnet-org/arpc/internal/packet"
	"github.com/appnet-org/arpc/internal/transport"
	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/metadata"
	"github.com/appnet-org/arpc/pkg/rpc/element"
	"github.com/appnet-org/arpc/pkg/serializer"
	"go.uber.org/zap"
)

// Client represents an RPC client with a transport and serializer.
type Client struct {
	transport       *transport.UDPTransport
	serializer      serializer.Serializer
	metadataCodec   metadata.MetadataCodec
	defaultAddr     string
	rpcElementChain *element.RPCElementChain
}

// NewClient creates a new Client using the given serializer and target address.
// The client will bind to any available local UDP port to avoid port conflicts
// when creating multiple clients in the same process.
func NewClient(serializer serializer.Serializer, addr string, rpcElements []element.RPCElement) (*Client, error) {
	// Use port 0 to let the OS assign an available port
	t, err := transport.NewUDPTransport(":0")
	if err != nil {
		return nil, err
	}
	return &Client{
		transport:       t,
		serializer:      serializer,
		metadataCodec:   metadata.MetadataCodec{},
		defaultAddr:     addr,
		rpcElementChain: element.NewRPCElementChain(rpcElements...),
	}, nil
}

// NewClientWithLocalAddr creates a new Client using the given serializer, target address, and local address.
// This allows specifying a custom local UDP address to bind to.
func NewClientWithLocalAddr(serializer serializer.Serializer, addr, localAddr string, rpcElements []element.RPCElement) (*Client, error) {
	t, err := transport.NewUDPTransport(localAddr)
	if err != nil {
		return nil, err
	}
	return &Client{
		transport:       t,
		serializer:      serializer,
		metadataCodec:   metadata.MetadataCodec{},
		defaultAddr:     addr,
		rpcElementChain: element.NewRPCElementChain(rpcElements...),
	}, nil
}

// Transport returns the underlying UDP transport for cleanup purposes
func (c *Client) Transport() *transport.UDPTransport {
	return c.transport
}

// frameRequest constructs a binary message with [serviceLen][service][methodLen][method][headerLen][headers][payload]
func frameRequest(service, method string, payload []byte) ([]byte, error) {
	buf := new(bytes.Buffer)

	// TODO(XZ): this is a temporary solution fix issue #5.
	// The first 6 bytes are the original IP address and port. The actual values are added in the transport layer.
	ip := net.ParseIP("0.0.0.0").To4()
	if _, err := buf.Write(ip); err != nil {
		return nil, err
	}

	port := uint16(0)
	if err := binary.Write(buf, binary.LittleEndian, port); err != nil {
		return nil, err
	}

	// Write service name
	if err := binary.Write(buf, binary.LittleEndian, uint16(len(service))); err != nil {
		return nil, err
	}
	if _, err := buf.Write([]byte(service)); err != nil {
		return nil, err
	}

	// Write method name
	if err := binary.Write(buf, binary.LittleEndian, uint16(len(method))); err != nil {
		return nil, err
	}
	if _, err := buf.Write([]byte(method)); err != nil {
		return nil, err
	}

	// Write payload
	if _, err := buf.Write(payload); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func parseFramedResponse(data []byte) (service string, method string, payload []byte, err error) {
	offset := 0

	// Parse service name
	if len(data) < 2 {
		return "", "", nil, fmt.Errorf("invalid response (too short for serviceLen)")
	}
	serviceLen := int(binary.LittleEndian.Uint16(data[offset:]))
	offset += 2
	if offset+serviceLen > len(data) {
		return "", "", nil, fmt.Errorf("invalid response (truncated service)")
	}
	service = string(data[offset : offset+serviceLen])
	offset += serviceLen

	// Parse method name
	if offset+2 > len(data) {
		return "", "", nil, fmt.Errorf("invalid response (too short for methodLen)")
	}
	methodLen := int(binary.LittleEndian.Uint16(data[offset:]))
	offset += 2
	if offset+methodLen > len(data) {
		return "", "", nil, fmt.Errorf("invalid response (truncated method)")
	}
	method = string(data[offset : offset+methodLen])
	offset += methodLen

	payload = data[offset:]
	return service, method, payload, nil
}

func (c *Client) handleErrorPacket(ctx context.Context, errorMsg string) error {
	// Create error response for RPC element processing
	rpcResp := &element.RPCResponse{
		Result: nil,
		Error:  fmt.Errorf("server error: %s", errorMsg),
	}

	// Process error response through RPC elements
	rpcResp, err := c.rpcElementChain.ProcessResponse(ctx, rpcResp)
	if err != nil {
		return err
	}

	// Return the processed error
	return rpcResp.Error
}

func (c *Client) handleResponsePacket(ctx context.Context, data []byte, rpcID uint64, resp any) error {
	// Parse framed response: extract service, method, payload
	_, _, respPayloadBytes, err := parseFramedResponse(data)
	if err != nil {
		return fmt.Errorf("failed to parse framed response: %w", err)
	}

	// Deserialize the response into resp
	if err := c.serializer.Unmarshal(respPayloadBytes, resp); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	logging.Debug("Successfully received response", zap.Uint64("rpcID", rpcID))

	// Create response for RPC element processing
	rpcResp := &element.RPCResponse{
		Result: resp,
		Error:  nil,
	}

	// Process response through RPC elements
	rpcResp, err = c.rpcElementChain.ProcessResponse(ctx, rpcResp)
	if err != nil {
		return err
	}

	return rpcResp.Error
}

// Call makes an RPC call with RPC element processing
func (c *Client) Call(ctx context.Context, service, method string, req any, resp any) error {

	rpcReqID := transport.GenerateRPCID()

	// Create request with service and method information
	rpcReq := &element.RPCRequest{
		ServiceName: service,
		Method:      method,
		ID:          rpcReqID,
		Payload:     req,
	}

	// Process request through RPC elements
	rpcReq, err := c.rpcElementChain.ProcessRequest(ctx, rpcReq)
	if err != nil {
		return err
	}

	// Serialize the request payload
	reqPayloadBytes, err := c.serializer.Marshal(rpcReq.Payload)
	logging.Debug("Serialized request payload", zap.String("payload", fmt.Sprintf("%x", reqPayloadBytes)))
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Frame the request into binary format
	framedReq, err := frameRequest(rpcReq.ServiceName, rpcReq.Method, reqPayloadBytes)
	logging.Debug("Framed request Message", zap.String("message", fmt.Sprintf("%x", framedReq)))
	if err != nil {
		return fmt.Errorf("failed to frame request: %w", err)
	}

	// Send the framed request
	if err := c.transport.Send(c.defaultAddr, rpcReq.ID, framedReq, packet.PacketTypeRequest); err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	// Wait and process the response
	for {
		data, _, respID, packetType, err := c.transport.Receive(packet.MaxUDPPayloadSize)
		if err != nil {
			return fmt.Errorf("failed to receive response: %w", err)
		}

		if data == nil {
			continue // Either still waiting for fragments or we received an non-data/error packet
		}

		if respID != rpcReq.ID {
			logging.Debug("Ignoring response with mismatched RPC ID",
				zap.Uint64("receivedID", respID),
				zap.Uint64("expectedID", rpcReq.ID))
			continue
		}

		// Process the packet based on its type
		switch packetType {
		case packet.PacketTypeResponse:
			return c.handleResponsePacket(ctx, data, respID, resp)
		case packet.PacketTypeError:
			return c.handleErrorPacket(ctx, string(data))
		default:
			logging.Debug("Ignoring packet with unknown type", zap.String("packetType", packetType.Name))
			continue
		}
	}
}
