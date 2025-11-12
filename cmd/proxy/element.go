package main

import (
	"context"

	"github.com/appnet-org/proxy/util"
)

// RPCElement defines the interface for RPC elements.
type RPCElement interface {
	// ProcessRequest processes the request before it's sent to the server.
	ProcessRequest(ctx context.Context, packet *util.BufferedPacket) (*util.BufferedPacket, util.PacketVerdict, context.Context, error)

	// ProcessResponse processes the response after it's received from the server.
	ProcessResponse(ctx context.Context, packet *util.BufferedPacket) (*util.BufferedPacket, util.PacketVerdict, context.Context, error)

	// Name returns the name of the RPC element.
	Name() string

	// RequestMode returns the required execution mode for processing requests.
	RequestMode() util.ExecutionMode

	// ResponseMode returns the required execution mode for processing responses.
	ResponseMode() util.ExecutionMode
}

// RPCElementChain represents a chain of RPC elements.
type RPCElementChain struct {
	elements []RPCElement
}

// NewRPCElementChain creates a new chain of RPC elements.
func NewRPCElementChain(elements ...RPCElement) *RPCElementChain {
	return &RPCElementChain{
		elements: elements,
	}
}

// ProcessRequest processes the request through all RPC elements in the chain.
func (c *RPCElementChain) ProcessRequest(ctx context.Context, packet *util.BufferedPacket) (*util.BufferedPacket, util.PacketVerdict, context.Context, error) {
	var err error
	var verdict util.PacketVerdict
	for _, element := range c.elements {
		packet, verdict, ctx, err = element.ProcessRequest(ctx, packet)
		if verdict == util.PacketVerdictDrop {
			return nil, util.PacketVerdictDrop, ctx, err
		}
		if err != nil {
			return nil, util.PacketVerdictPass, ctx, err
		}

	}
	return packet, util.PacketVerdictPass, ctx, nil
}

// ProcessResponse processes the response through all RPC elements in reverse order.
func (c *RPCElementChain) ProcessResponse(ctx context.Context, packet *util.BufferedPacket) (*util.BufferedPacket, util.PacketVerdict, context.Context, error) {
	var err error
	var verdict util.PacketVerdict
	for i := len(c.elements) - 1; i >= 0; i-- {
		packet, verdict, ctx, err = c.elements[i].ProcessResponse(ctx, packet)
		if verdict == util.PacketVerdictDrop {
			return nil, util.PacketVerdictDrop, ctx, err
		}

		if err != nil {
			return nil, util.PacketVerdictPass, ctx, err
		}
	}
	return packet, util.PacketVerdictPass, ctx, nil
}

// RequiredBufferingMode determines the required execution mode for the chain.
// Priority: FullBuffering > StreamingWithBuffering > Streaming.
// Returns separate modes for request and response processing.
func (c *RPCElementChain) RequiredBufferingMode() (requestMode, responseMode util.ExecutionMode) {
	requestMode = util.StreamingMode
	responseMode = util.StreamingMode

	for _, e := range c.elements {
		// Check request mode
		switch e.RequestMode() {
		case util.FullBufferingMode:
			// Highest priority, set and continue checking other elements
			requestMode = util.FullBufferingMode
		case util.StreamingWithBufferingMode:
			// Keep track if we see this, unless a FullBuffering appears later
			if requestMode != util.FullBufferingMode {
				requestMode = util.StreamingWithBufferingMode
			}
		}
		// Check response mode
		switch e.ResponseMode() {
		case util.FullBufferingMode:
			// Highest priority, set and continue checking other elements
			responseMode = util.FullBufferingMode
		case util.StreamingWithBufferingMode:
			// Keep track if we see this, unless a FullBuffering appears later
			if responseMode != util.FullBufferingMode {
				responseMode = util.StreamingWithBufferingMode
			}
		}
	}
	return requestMode, responseMode
}
