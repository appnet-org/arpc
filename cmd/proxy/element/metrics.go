package element

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/appnet-org/proxy/types"
)

// MetricsElement implements RPCElement to provide metrics functionality
type MetricsElement struct {
	requestCount  uint64
	responseCount uint64
	seenRequests  sync.Map // map[uint64]struct{} - tracks seen RPC IDs for requests
	seenResponses sync.Map // map[uint64]struct{} - tracks seen RPC IDs for responses
}

// NewMetricsElement creates a new metrics element
func NewMetricsElement() *MetricsElement {
	return &MetricsElement{}
}

// Mode returns the execution mode for this element
func (m *MetricsElement) Mode() types.ExecutionMode {
	return types.StreamingMode
}

// shouldCount determines if we should count this packet (only count first fragment or full packets)
func shouldCount(packet *types.BufferedPacket) bool {
	if packet == nil {
		return false
	}
	// Count if it's a complete message or the first fragment
	return packet.IsFull || packet.SeqNumber == 0
}

// ProcessRequest increments the request counter (once per message) and returns the request unchanged
func (m *MetricsElement) ProcessRequest(ctx context.Context, packet *types.BufferedPacket) (*types.BufferedPacket, context.Context, error) {
	if !shouldCount(packet) {
		return packet, ctx, nil
	}

	if _, alreadySeen := m.seenRequests.LoadOrStore(packet.RPCID, struct{}{}); !alreadySeen {
		atomic.AddUint64(&m.requestCount, 1)
	}
	return packet, ctx, nil
}

// ProcessResponse increments the response counter (once per message) and returns the response unchanged
func (m *MetricsElement) ProcessResponse(ctx context.Context, packet *types.BufferedPacket) (*types.BufferedPacket, context.Context, error) {
	if !shouldCount(packet) {
		return packet, ctx, nil
	}

	if _, alreadySeen := m.seenResponses.LoadOrStore(packet.RPCID, struct{}{}); !alreadySeen {
		atomic.AddUint64(&m.responseCount, 1)
	}
	return packet, ctx, nil
}

// Name returns the name of this element
func (m *MetricsElement) Name() string {
	return "MetricsElement"
}

// GetRequestCount returns the total number of requests processed
func (m *MetricsElement) GetRequestCount() uint64 {
	return atomic.LoadUint64(&m.requestCount)
}

// GetResponseCount returns the total number of responses processed
func (m *MetricsElement) GetResponseCount() uint64 {
	return atomic.LoadUint64(&m.responseCount)
}
