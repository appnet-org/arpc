package main

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	"github.com/appnet-org/arpc/pkg/rpc/element"
)

// MetricsElement tracks various RPC metrics
type MetricsElement struct {
	requestCount uint64
	ctx          context.Context
	cancel       context.CancelFunc
}

// NewMetricsElement creates a new metrics element and starts the metrics printer
func NewMetricsElement() element.RPCElement {
	ctx, cancel := context.WithCancel(context.Background())
	m := &MetricsElement{
		ctx:    ctx,
		cancel: cancel,
	}

	// Start the metrics printer in a goroutine
	go m.printMetrics()

	return m
}

// printMetrics prints the metrics every 10 seconds
func (m *MetricsElement) printMetrics() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			count := atomic.LoadUint64(&m.requestCount)
			log.Printf("Metrics - Total Requests: %d", count)
		case <-m.ctx.Done():
			return
		}
	}
}

// ProcessRequest increments the request counter and passes through the request
func (m *MetricsElement) ProcessRequest(ctx context.Context, req *element.RPCRequest) (*element.RPCRequest, context.Context, error) {
	atomic.AddUint64(&m.requestCount, 1)
	return req, ctx, nil
}

// ProcessResponse passes through the response without modification
func (m *MetricsElement) ProcessResponse(ctx context.Context, resp *element.RPCResponse) (*element.RPCResponse, context.Context, error) {
	if resp.Error != nil {
		m.cancel() // Stop metrics on error
	}
	return resp, ctx, nil
}

// Name returns the name of the metrics element
func (m *MetricsElement) Name() string {
	return "metrics"
}

// GetRequestCount returns the total number of requests processed
func (m *MetricsElement) GetRequestCount() uint64 {
	return atomic.LoadUint64(&m.requestCount)
}
