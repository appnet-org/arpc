// pkg/rpc/client.go
package rpc

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"

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
	serviceRegistry *ServiceRegistry
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
		serviceRegistry: NewServiceRegistry(),
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
		serviceRegistry: NewServiceRegistry(),
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

// SetServiceRegistry sets or updates the service registry for the client
func (c *Client) SetServiceRegistry(registry *ServiceRegistry) {
	c.serviceRegistry = registry
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
	// Data is already the raw payload, no framing to parse
	// Deserialize the response into resp
	if err := c.serializer.Unmarshal(data, resp); err != nil {
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
	rpcResp, _, err := c.rpcElementChain.ProcessResponse(ctx, rpcResp)
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

	// Serialize the request payload
	reqPayloadBytes, err := c.serializer.Marshal(rpcReq.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Lookup service and method IDs from registry
	serviceID, ok := c.serviceRegistry.GetServiceID(rpcReq.ServiceName)
	if !ok {
		return fmt.Errorf("service not found in registry: %s", rpcReq.ServiceName)
	}
	methodID, ok := c.serviceRegistry.GetMethodID(rpcReq.ServiceName, rpcReq.Method)
	if !ok {
		return fmt.Errorf("method not found in registry: %s.%s", rpcReq.ServiceName, rpcReq.Method)
	}

	// Write service and method IDs to Symphony reserved header (bytes 5-9 and 9-13)
	if len(reqPayloadBytes) >= 13 {
		binary.LittleEndian.PutUint32(reqPayloadBytes[5:9], serviceID)
		binary.LittleEndian.PutUint32(reqPayloadBytes[9:13], methodID)
	}

	// Create a response channel and register it before sending
	respChan := make(chan *responseData, 1)
	c.registerPendingCall(rpcReq.ID, respChan)
	defer c.unregisterPendingCall(rpcReq.ID)

	// Send the payload directly (no framing)
	err = c.transport.Send(c.defaultAddr, rpcReq.ID, reqPayloadBytes, packet.PacketTypeRequest)
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
