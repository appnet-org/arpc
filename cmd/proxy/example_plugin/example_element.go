// This is an example element plugin that can be compiled as a dynamically loadable .so file
// To build: go build -buildmode=plugin -o element-example.so example_element.go
//
// The compiled .so file should be placed in /appnet/elements/ with a name like:
// element-example.so, element-example-v2.so, etc. (the highest alphabetically sorted
// file matching the "element-" prefix will be loaded)

package main

import (
	"context"
	"fmt"

	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/proxy/util"
	"go.uber.org/zap"
)

// ExampleElement is a simple example element that logs requests and responses
type ExampleElement struct {
	name string
}

// ProcessRequest processes incoming requests
func (e *ExampleElement) ProcessRequest(ctx context.Context, packet *util.BufferedPacket) (*util.BufferedPacket, util.PacketVerdict, context.Context, error) {
	if packet == nil {
		return packet, util.PacketVerdictPass, ctx, nil
	}

	logging.Info("ExampleElement: Processing request",
		zap.Uint64("rpcID", packet.RPCID),
		zap.String("packetType", packet.PacketType.String()),
		zap.Int("payloadSize", len(packet.Payload)),
		zap.String("source", packet.Source.String()),
		zap.String("peer", packet.Peer.String()))

	// Example: You can modify the packet here if needed
	// For this example, we just pass it through unchanged
	return packet, util.PacketVerdictPass, ctx, nil
}

// ProcessResponse processes outgoing responses
func (e *ExampleElement) ProcessResponse(ctx context.Context, packet *util.BufferedPacket) (*util.BufferedPacket, util.PacketVerdict, context.Context, error) {
	if packet == nil {
		return packet, util.PacketVerdictPass, ctx, nil
	}

	logging.Info("ExampleElement: Processing response",
		zap.Uint64("rpcID", packet.RPCID),
		zap.String("packetType", packet.PacketType.String()),
		zap.Int("payloadSize", len(packet.Payload)))

	// Example: You can modify the packet here if needed
	// For this example, we just pass it through unchanged
	return packet, util.PacketVerdictPass, ctx, nil
}

// Name returns the name of this element
func (e *ExampleElement) Name() string {
	return e.name
}

// RPCElement is the interface that elements must implement
// NOTE: This must match the interface defined in cmd/proxy/element.go
// For proper type sharing, consider moving RPCElement to a shared package
type RPCElement interface {
	ProcessRequest(ctx context.Context, packet *util.BufferedPacket) (*util.BufferedPacket, util.PacketVerdict, context.Context, error)
	ProcessResponse(ctx context.Context, packet *util.BufferedPacket) (*util.BufferedPacket, util.PacketVerdict, context.Context, error)
	Name() string
}

// ExampleElementInit implements the elementInit interface required by the plugin loader
type ExampleElementInit struct {
	element *ExampleElement
}

// Element returns the RPCElement instance
func (e *ExampleElementInit) Element() RPCElement {
	return e.element
}

// Kill is called when the plugin is being unloaded (optional cleanup)
func (e *ExampleElementInit) Kill() {
	// Cleanup any background goroutines or resources here
	// For this example, there's nothing to clean up
	logging.Info("ExampleElement: Plugin being unloaded, performing cleanup")
}

// ElementInit is the exported symbol that the plugin loader looks for
// This must be named exactly "ElementInit" and must implement the elementInit interface
var ElementInit = &ExampleElementInit{
	element: &ExampleElement{
		name: "ExampleElement",
	},
}

// init function is called when the plugin is loaded
func init() {
	fmt.Println("ExampleElement plugin loaded successfully")
}

