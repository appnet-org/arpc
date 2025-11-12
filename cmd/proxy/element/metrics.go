package element

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/proxy/util"
	"go.uber.org/zap"
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

// RequestMode returns the execution mode for processing requests
func (m *MetricsElement) RequestMode() util.ExecutionMode {
	return util.StreamingMode
}

// ResponseMode returns the execution mode for processing responses
func (m *MetricsElement) ResponseMode() util.ExecutionMode {
	return util.StreamingMode
}

// shouldCount determines if we should count this packet (only count first fragment or full packets)
func shouldCount(packet *util.BufferedPacket) bool {
	if packet == nil {
		return false
	}
	// Count if it's a complete message or the first fragment
	return packet.IsFull || packet.SeqNumber == 0
}

// ProcessRequest increments the request counter (once per message) and returns the request unchanged
func (m *MetricsElement) ProcessRequest(ctx context.Context, packet *util.BufferedPacket) (*util.BufferedPacket, util.PacketVerdict, context.Context, error) {
	if !shouldCount(packet) {
		return packet, util.PacketVerdictPass, ctx, nil
	}

	if _, alreadySeen := m.seenRequests.LoadOrStore(packet.RPCID, struct{}{}); !alreadySeen {
		atomic.AddUint64(&m.requestCount, 1)
		logging.Debug("Request count", zap.Uint64("count", atomic.LoadUint64(&m.requestCount)))
	}
	return packet, util.PacketVerdictPass, ctx, nil
}

// ProcessResponse increments the response counter (once per message) and returns the response unchanged
func (m *MetricsElement) ProcessResponse(ctx context.Context, packet *util.BufferedPacket) (*util.BufferedPacket, util.PacketVerdict, context.Context, error) {
	if !shouldCount(packet) {
		return packet, util.PacketVerdictPass, ctx, nil
	}

	if _, alreadySeen := m.seenResponses.LoadOrStore(packet.RPCID, struct{}{}); !alreadySeen {
		atomic.AddUint64(&m.responseCount, 1)
		logging.Debug("Response count", zap.Uint64("count", atomic.LoadUint64(&m.responseCount)))
	}
	return packet, util.PacketVerdictPass, ctx, nil
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
