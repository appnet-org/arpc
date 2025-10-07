package elements

import (
	"context"
	"math/rand/v2"
	"sync"

	"github.com/appnet-org/arpc/pkg/rpc"
	"github.com/appnet-org/arpc/pkg/rpc/element"
)

type AdmissioncontrolElement struct {
	ctx         context.Context
	cancel      context.CancelFunc
	multiplier  float64
	total       float64
	totalLock   *sync.Mutex
	success     float64
	successLock *sync.RWMutex
}

func NewAdmissioncontrolElement() element.RPCElement {
	ctx, cancel := context.WithCancel(context.Background())
	e := &AdmissioncontrolElement{
		ctx:         ctx,
		cancel:      cancel,
		multiplier:  1.1,
		total:       0.0,
		totalLock:   &sync.Mutex{},
		success:     0.0,
		successLock: &sync.RWMutex{},
	}
	return e
}

func (e *AdmissioncontrolElement) ProcessRequest(ctx context.Context, req *element.RPCRequest) (*element.RPCRequest, context.Context, error) {
	e.successLock.RLock()
	e.totalLock.Lock()
	// See https://sre.google/sre-book/handling-overload/#eq2101 for details
	// prob is accepting probability
	prob := 1.0 - max(0.0, (e.total-e.multiplier*e.success)/(e.total+1.0))
	e.total += 1.0
	e.totalLock.Unlock()
	e.successLock.RUnlock()

	if rand.Float64() < prob {
		return req, ctx, nil
	} else {
		return nil, ctx, &rpc.RPCError{Type: rpc.RPCFailError, Reason: "admission control"}
	}
}

func (e *AdmissioncontrolElement) ProcessResponse(ctx context.Context, resp *element.RPCResponse) (*element.RPCResponse, context.Context, error) {
	if resp.Error == nil {
		e.successLock.Lock()
		e.success += 1.0
		e.successLock.Unlock()
	}
	return resp, ctx, nil
}

func (e *AdmissioncontrolElement) Name() string {
	return "admissioncontrol"
}
