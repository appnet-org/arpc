package element

import (
	"context"
	"errors"
	"math/rand"
	"sync"

	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/proxy/util"
	"go.uber.org/zap"
)

// ErrPacketDropped is returned when a packet is intentionally dropped by fault injection
var ErrPacketDropped = errors.New("packet dropped by fault injection")

// FaultInjectionElement implements RPCElement to provide fault injection functionality
type FaultInjectionElement struct {
	dropProbability float64 // Probability of dropping a packet (0.0 to 1.0)
	mu              sync.Mutex
	rng             *rand.Rand
}

// NewFaultInjectionElement creates a new fault injection element with the specified drop probability
func NewFaultInjectionElement(dropProbability float64) *FaultInjectionElement {
	if dropProbability < 0.0 || dropProbability > 1.0 {
		dropProbability = 0.0
		logging.Warn("Invalid drop probability, defaulting to 0.0", zap.Float64("provided", dropProbability))
	}

	return &FaultInjectionElement{
		dropProbability: dropProbability,
		rng:             rand.New(rand.NewSource(rand.Int63())),
	}
}

// RequestMode returns the execution mode for processing requests
func (f *FaultInjectionElement) RequestMode() util.ExecutionMode {
	return util.StreamingMode
}

// ResponseMode returns the execution mode for processing responses
func (f *FaultInjectionElement) ResponseMode() util.ExecutionMode {
	return util.StreamingMode
}

// shouldDrop determines if a packet should be dropped based on the drop probability
func (f *FaultInjectionElement) shouldDrop() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rng.Float64() < f.dropProbability
}

// ProcessRequest processes the request and may drop it based on the configured probability
func (f *FaultInjectionElement) ProcessRequest(ctx context.Context, packet *util.BufferedPacket) (*util.BufferedPacket, util.PacketVerdict, context.Context, error) {
	if packet == nil {
		return packet, util.PacketVerdictPass, ctx, nil
	}

	if f.shouldDrop() {
		logging.Debug("Request dropped by fault injection", zap.String("packetType", packet.PacketType.String()))
		return nil, util.PacketVerdictDrop, ctx, ErrPacketDropped
	}
	return packet, util.PacketVerdictPass, ctx, nil
}

// ProcessResponse processes the response and returns it unchanged (no fault injection on responses)
func (f *FaultInjectionElement) ProcessResponse(ctx context.Context, packet *util.BufferedPacket) (*util.BufferedPacket, util.PacketVerdict, context.Context, error) {
	// Fault injection only applies to requests, not responses
	return packet, util.PacketVerdictPass, ctx, nil
}

// Name returns the name of this element
func (f *FaultInjectionElement) Name() string {
	return "FaultInjectionElement"
}

// GetDropProbability returns the current drop probability
func (f *FaultInjectionElement) GetDropProbability() float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.dropProbability
}

// SetDropProbability sets the drop probability (thread-safe)
func (f *FaultInjectionElement) SetDropProbability(probability float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if probability < 0.0 || probability > 1.0 {
		logging.Warn("Invalid drop probability, ignoring", zap.Float64("provided", probability))
		return
	}
	f.dropProbability = probability
}
