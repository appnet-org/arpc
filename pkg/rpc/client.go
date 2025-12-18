// pkg/rpc/client.go
package rpc

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/appnet-org/arpc/pkg/common"
	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/metadata"
	"github.com/appnet-org/arpc/pkg/packet"
	"github.com/appnet-org/arpc/pkg/rpc/element"
	"github.com/appnet-org/arpc/pkg/serializer"
	"github.com/appnet-org/arpc/pkg/transport"
	"go.uber.org/zap"
)

// responseData holds response information for a pending RPC call
type responseData struct {
	data       []byte
	packetType packet.PacketType
	err        error
}

// Client represents an RPC client with a transport and serializer.
type Client struct {
	transport       *transport.UDPTransport
	serializer      serializer.Serializer
	metadataCodec   metadata.MetadataCodec
	defaultAddr     string
	rpcElementChain *element.RPCElementChain

	// Response dispatcher for handling concurrent calls
	pendingCalls map[uint64]chan *responseData
	pendingMu    sync.RWMutex
	receiverDone chan struct{}
	receiverOnce sync.Once
}

// NewClient creates a new Client using the given serializer and target address.
// The client will bind to any available local UDP port to avoid port conflicts
// when creating multiple clients in the same process.
func NewClient(serializer serializer.Serializer, addr string, rpcElements []element.RPCElement) (*Client, error) {
	// Use port 0 to let the OS assign an available port
	t, err := transport.NewUDPTransport("0.0.0.0:0")
	if err != nil {
		return nil, err
	}
	c := &Client{
		transport:       t,
		serializer:      serializer,
		metadataCodec:   metadata.MetadataCodec{},
		defaultAddr:     addr,
		rpcElementChain: element.NewRPCElementChain(rpcElements...),
		pendingCalls:    make(map[uint64]chan *responseData),
		receiverDone:    make(chan struct{}),
	}
	// Start the background receiver goroutine
	go c.receiveLoop()
	return c, nil
}

// NewClientWithLocalAddr creates a new Client using the given serializer, target address, and local address.
// This allows specifying a custom local UDP address to bind to.
func NewClientWithLocalAddr(serializer serializer.Serializer, addr, localAddr string, rpcElements []element.RPCElement) (*Client, error) {
	t, err := transport.NewUDPTransport(localAddr)
	if err != nil {
		return nil, err
	}
	c := &Client{
		transport:       t,
		serializer:      serializer,
		metadataCodec:   metadata.MetadataCodec{},
		defaultAddr:     addr,
		rpcElementChain: element.NewRPCElementChain(rpcElements...),
		pendingCalls:    make(map[uint64]chan *responseData),
		receiverDone:    make(chan struct{}),
	}
	// Start the background receiver goroutine
	go c.receiveLoop()
	return c, nil
}

// Transport returns the underlying UDP transport for cleanup purposes
func (c *Client) Transport() *transport.UDPTransport {
	return c.transport
}

// receiveLoop runs in a background goroutine and dispatches responses to pending calls
func (c *Client) receiveLoop() {
	for {
		// Check for shutdown signal (non-blocking)
		select {
		case <-c.receiverDone:
			return
		default:
		}

		// Block on receive (this will block until data arrives or error occurs)
		data, _, respID, packetType, err := c.transport.Receive(packet.MaxUDPPayloadSize, transport.RoleClient)

		// Check if we should dispatch this response
		if data != nil || err != nil {
			c.pendingMu.RLock()
			respChan, exists := c.pendingCalls[respID]
			c.pendingMu.RUnlock()

			if exists {
				// Send response to the waiting goroutine
				respChan <- &responseData{
					data:       data,
					packetType: packetType,
					err:        err,
				}
			} else {
				// No one waiting for this response - log and return buffer to pool
				if data != nil {
					logging.Debug("Ignoring response with no pending call",
						zap.Uint64("rpcID", respID))
					c.transport.GetBufferPool().Put(data)
				}
			}
		}
	}
}

// registerPendingCall registers a channel for a pending RPC call
func (c *Client) registerPendingCall(rpcID uint64, ch chan *responseData) {
	c.pendingMu.Lock()
	c.pendingCalls[rpcID] = ch
	c.pendingMu.Unlock()
}

// unregisterPendingCall removes a pending call registration
func (c *Client) unregisterPendingCall(rpcID uint64) {
	c.pendingMu.Lock()
	delete(c.pendingCalls, rpcID)
	c.pendingMu.Unlock()
}

// frameRequest constructs a binary message with
// [serviceLen(2B)][service][methodLen(2B)][method][metadataLen(2B)][metadata][payload]
func (c *Client) frameRequest(service, method string, metadataBytes, payload []byte, pool *common.BufferPool) ([]byte, error) {
	// Pre-calculate buffer size (headers: 2 + 2 + 2 = 6 bytes)
	totalSize := 6 + len(service) + len(method) + len(metadataBytes) + len(payload)

	var buf []byte
	if pool != nil {
		buf = pool.GetSize(totalSize)
	} else {
		buf = make([]byte, totalSize)
	}

	// service
	binary.LittleEndian.PutUint16(buf[0:2], uint16(len(service)))
	copy(buf[2:], service)

	// method
	methodStart := 2 + len(service)
	binary.LittleEndian.PutUint16(buf[methodStart:methodStart+2], uint16(len(method)))
	copy(buf[methodStart+2:], method)

	// metadata
	metadataStart := methodStart + 2 + len(method)
	binary.LittleEndian.PutUint16(buf[metadataStart:metadataStart+2], uint16(len(metadataBytes)))
	copy(buf[metadataStart+2:], metadataBytes)

	// payload
	payloadStart := metadataStart + 2 + len(metadataBytes)
	copy(buf[payloadStart:], payload)

	return buf, nil
}

func (c *Client) parseFramedResponse(data []byte) (service string, method string, payload []byte, err error) {
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

func (c *Client) handleErrorPacket(ctx context.Context, data []byte, errType packet.PacketType) error {
	// Convert data to string for error message
	errMsg := string(data)

	// Return buffer to pool after converting to string
	c.transport.GetBufferPool().Put(data)

	// Create error response for RPC element processing
	rpcResp := &element.RPCResponse{
		Result: nil,
		Error:  fmt.Errorf("server error: %s", errMsg),
	}

	// Process error response through RPC elements
	_, _, err := c.rpcElementChain.ProcessResponse(ctx, rpcResp)
	if err != nil {
		return err
	}

	var rpcErrType RPCErrorType
	if errType == packet.PacketTypeError {
		rpcErrType = RPCFailError
	} else {
		rpcErrType = RPCUnknownError
	}
	return &RPCError{Type: rpcErrType, Reason: errMsg}
}

func (c *Client) handleResponsePacket(ctx context.Context, data []byte, rpcID uint64, resp any) error {
	// Parse framed response: extract service, method, payload
	_, _, respPayloadBytes, err := c.parseFramedResponse(data)
	if err != nil {
		// Return buffer to pool on parse error
		c.transport.GetBufferPool().Put(data)
		return fmt.Errorf("failed to parse framed response: %w", err)
	}

	// Deserialize the response into resp
	if err := c.serializer.Unmarshal(respPayloadBytes, resp); err != nil {
		// Return buffer to pool on unmarshal error
		c.transport.GetBufferPool().Put(data)
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Return buffer to pool after unmarshaling (unmarshaler has copied what it needs)
	c.transport.GetBufferPool().Put(data)

	logging.Debug("Successfully received response", zap.Uint64("rpcID", rpcID))

	// Create response for RPC element processing
	rpcResp := &element.RPCResponse{
		ID:     rpcID,
		Result: resp,
		Error:  nil,
	}

	// Process response through RPC elements
	rpcResp, _, err = c.rpcElementChain.ProcessResponse(ctx, rpcResp)
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
	rpcReq, ctx, err := c.rpcElementChain.ProcessRequest(ctx, rpcReq)
	if err != nil {
		return err
	}

	// Extract metadata from context
	md := metadata.FromOutgoingContext(ctx)
	var metadataBytes []byte
	if len(md) > 0 {
		metadataBytes, err = c.metadataCodec.EncodeHeaders(md, c.transport.GetBufferPool())
		if err != nil {
			return fmt.Errorf("failed to encode metadata: %w", err)
		}
		logging.Debug("Encoded metadata", zap.String("metadata", fmt.Sprintf("%x", metadataBytes)))
	}

	// Serialize the request payload
	reqPayloadBytes, err := c.serializer.Marshal(rpcReq.Payload)
	// logging.Debug("Serialized request payload", zap.String("payload", fmt.Sprintf("%x", reqPayloadBytes)))
	if err != nil {
		// Return metadata buffer to pool on error
		if len(md) > 0 && metadataBytes != nil {
			c.transport.GetBufferPool().Put(metadataBytes)
		}
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Add the destination IP address and port to the request payload
	// Frame the request into binary format using buffer pool
	framedReq, err := c.frameRequest(rpcReq.ServiceName, rpcReq.Method, metadataBytes, reqPayloadBytes, c.transport.GetBufferPool())

	// Return metadata buffer to pool after frameRequest copies it
	if len(md) > 0 && metadataBytes != nil {
		c.transport.GetBufferPool().Put(metadataBytes)
	}

	if err != nil {
		return fmt.Errorf("failed to frame request: %w", err)
	}

	// Create a response channel and register it before sending
	respChan := make(chan *responseData, 1)
	c.registerPendingCall(rpcReq.ID, respChan)
	defer c.unregisterPendingCall(rpcReq.ID)

	// Send the framed request
	err = c.transport.Send(c.defaultAddr, rpcReq.ID, framedReq, packet.PacketTypeRequest)

	// Return buffer to pool after sending (transport.Send copies the data)
	c.transport.GetBufferPool().Put(framedReq)

	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	// Wait for the response from the dispatcher
	respData := <-respChan

	// Check for receive error
	if respData.err != nil {
		return fmt.Errorf("failed to receive response: %w", respData.err)
	}

	// Check if we got data
	if respData.data == nil {
		return fmt.Errorf("received nil response data")
	}

	// Process the packet based on its type
	switch respData.packetType {
	case packet.PacketTypeResponse:
		return c.handleResponsePacket(ctx, respData.data, rpcReq.ID, resp)
	case packet.PacketTypeError, packet.PacketTypeUnknown:
		// handleErrorPacket will return the buffer to pool
		return c.handleErrorPacket(ctx, respData.data, respData.packetType)
	default:
		logging.Debug("Ignoring packet with unknown type", zap.String("packetType", respData.packetType.Name))
		// Return buffer to pool for unknown packet type
		c.transport.GetBufferPool().Put(respData.data)
		return fmt.Errorf("unexpected packet type: %s", respData.packetType.Name)
	}
}

// Temporary functions to register packet types and handlers.
// TODO(XZ): remove these once the transport can be dynamically configured.

// RegisterPacketType registers a custom packet type with the client's transport
func (c *Client) RegisterPacketType(packetType string, codec packet.PacketCodec) (packet.PacketType, error) {
	return c.transport.RegisterPacketType(packetType, codec)
}

// RegisterPacketTypeWithID registers a custom packet type with a specific ID
func (c *Client) RegisterPacketTypeWithID(packetType string, id packet.PacketTypeID, codec packet.PacketCodec) (packet.PacketType, error) {
	return c.transport.RegisterPacketTypeWithID(packetType, id, codec)
}

// RegisterHandler registers a handler for a specific packet type and role
func (c *Client) RegisterHandler(packetTypeID packet.PacketTypeID, handler transport.Handler, role transport.Role) {
	handlerChain := transport.NewHandlerChain(fmt.Sprintf("ClientHandler_%d", packetTypeID), handler)
	c.transport.RegisterHandlerChain(packetTypeID, handlerChain, role)
}

// RegisterHandlerChain registers a complete handler chain for a packet type and role
func (c *Client) RegisterHandlerChain(packetTypeID packet.PacketTypeID, chain *transport.HandlerChain, role transport.Role) {
	c.transport.RegisterHandlerChain(packetTypeID, chain, role)
}

// GetRegisteredPackets returns all registered packet types
func (c *Client) GetRegisteredPackets() []packet.PacketType {
	return c.transport.ListRegisteredPackets()
}

// GetTransport returns the underlying transport for advanced operations
func (c *Client) GetTransport() *transport.UDPTransport {
	return c.transport
}

// Close closes the client and stops the background receiver goroutine
func (c *Client) Close() error {
	// Signal the receiver goroutine to stop (only once)
	c.receiverOnce.Do(func() {
		close(c.receiverDone)
	})

	// Close the transport
	return c.transport.Close()
}
