package element

import (
	"context"
	"errors"
	"math/rand"
	"sync"

	kv "github.com/appnet-org/arpc/benchmark/kv-store-symphony-element/symphony"

	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/proxy/util"
	"go.uber.org/zap"
)

// ErrPacketBlocked is returned when a packet is intentionally dropped by firewall
var ErrPacketBlocked = errors.New("message blocked by firewall")

// FirewallElement implements RPCElement to provide firewall functionality
type FirewallElement struct {
	blockThreshold int32 // Score threshold for blocking messages
	mu             sync.Mutex
	rng            *rand.Rand
}

// NewFaultInjectionElement creates a new fault injection element with the specified drop probability
func NewFirewallElement(blockThreshold int32) *FirewallElement {
	return &FirewallElement{
		blockThreshold: blockThreshold,
	}
}

// shouldDrop determines if a packet should be dropped based on the drop probability
func (f *FirewallElement) shouldBlock(score int32) bool {
	return score >= f.blockThreshold
}

// ProcessRequest processes the request and may drop it based on the configured probability
func (f *FirewallElement) ProcessRequest(ctx context.Context, packet *util.BufferedPacket) (*util.BufferedPacket, util.PacketVerdict, context.Context, error) {
	if packet == nil {
		return packet, util.PacketVerdictPass, ctx, nil
	}

	getRequest := kv.GetRequestRaw(packet.Payload)
	logging.Debug("Request score", zap.Int32("score", getRequest.GetScore()))

	if f.shouldBlock(getRequest.GetScore()) {
		logging.Debug("Request blocked by firewall", zap.String("packetType", packet.PacketType.String()))
		return nil, util.PacketVerdictDrop, ctx, ErrPacketBlocked
	}
	return packet, util.PacketVerdictPass, ctx, nil
}

// ProcessResponse processes the response and returns it unchanged (no fault injection on responses)
func (f *FirewallElement) ProcessResponse(ctx context.Context, packet *util.BufferedPacket) (*util.BufferedPacket, util.PacketVerdict, context.Context, error) {
	// Fault injection only applies to requests, not responses
	return packet, util.PacketVerdictPass, ctx, nil
}

// Name returns the name of this element
func (f *FirewallElement) Name() string {
	return "FirewallElement"
}
