package elements

import (
	"context"
	"sync"

	"github.com/appnet-org/arpc/pkg/rpc"
	"github.com/appnet-org/arpc/pkg/rpc/element"
)

type CircuitbreakerElement struct {
	ctx              context.Context
	cancel           context.CancelFunc
	pendingReq       uint32
	pendingReqLock   *sync.Mutex
	maxConcurrentReq uint32
}

func NewCircuitbreakerElement() element.RPCElement {
	ctx, cancel := context.WithCancel(context.Background())
	e := &CircuitbreakerElement{
		ctx:              ctx,
		cancel:           cancel,
		pendingReq:       0,
		pendingReqLock:   &sync.Mutex{},
		maxConcurrentReq: 5,
	}
	return e
}

func (e *CircuitbreakerElement) ProcessRequest(ctx context.Context, req *element.RPCRequest) (*element.RPCRequest, context.Context, error) {
	e.pendingReqLock.Lock()
	defer e.pendingReqLock.Unlock()
	if e.pendingReq < e.maxConcurrentReq {
		e.pendingReq += 1
		return req, ctx, nil
	} else {
		return nil, ctx, &rpc.RPCError{Type: rpc.RPCFailError, Reason: "circuit breaker"}
	}
}

func (e *CircuitbreakerElement) ProcessResponse(ctx context.Context, resp *element.RPCResponse) (*element.RPCResponse, context.Context, error) {
	e.pendingReqLock.Lock()
	defer e.pendingReqLock.Unlock()
	e.pendingReq -= 1
	return resp, ctx, nil
}

func (e *CircuitbreakerElement) Name() string {
	return "circuitbreaker"
}
