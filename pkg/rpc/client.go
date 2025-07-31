package rpc

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/appnet-org/arpc/internal/protocol"
	"github.com/appnet-org/arpc/internal/transport"
	"github.com/appnet-org/arpc/pkg/metadata"
	"github.com/appnet-org/arpc/pkg/rpc/element"
	"github.com/appnet-org/arpc/pkg/serializer"
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
func NewClient(serializer serializer.Serializer, addr string, transportElements []transport.TransportElement, rpcElements []element.RPCElement) (*Client, error) {
	t, err := transport.NewUDPTransport("", transportElements...)
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
	log.Printf("Serialized request payload: %x", reqPayloadBytes)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Frame the request into binary format
	framedReq, err := frameRequest(rpcReq.ServiceName, rpcReq.Method, reqPayloadBytes)
	log.Printf("Framed request Message: %x", framedReq)
	if err != nil {
		return fmt.Errorf("failed to frame request: %w", err)
	}

	// Send the framed request
	if err := c.transport.Send(c.defaultAddr, rpcReq.ID, framedReq, protocol.PacketTypeRequest); err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	// Wait and process the response
	for {
		data, _, respID, err := c.transport.Receive(protocol.MaxUDPPayloadSize)
		if err != nil {
			return fmt.Errorf("failed to receive response: %w", err)
		}
		if data == nil {
			time.Sleep(10 * time.Millisecond)
			continue // waiting for complete response
		}
		if respID != rpcReq.ID {
			log.Printf("Ignoring response with mismatched RPC ID: %d (expected %d)", respID, rpcReq.ID)
			continue
		}

		// Parse framed response: extract service, method, payload
		_, _, respPayloadBytes, err := parseFramedResponse(data)
		if err != nil {
			return fmt.Errorf("failed to parse framed response: %w", err)
		}

		// Deserialize the response into resp
		if err := c.serializer.Unmarshal(respPayloadBytes, resp); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}

		log.Printf("Successfully received response for RPC ID %d\n", rpcReq.ID)

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
}
