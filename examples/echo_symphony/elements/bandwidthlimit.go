package elements

import (
	"context"
	"sync"
	"time"
	"unsafe"

	"github.com/appnet-org/arpc/pkg/rpc"
	"github.com/appnet-org/arpc/pkg/rpc/element"
)

type BandwidthlimitElement struct {
	ctx      context.Context
	cancel   context.CancelFunc
	perSecBw float64
	limitBw  float64
	tokenBw  float64
	lastTs   float64
	lock     *sync.Mutex
}

func getNowTs() float64 {
	return float64(time.Now().UnixMicro())
}

func NewBandwidthlimitElement() element.RPCElement {
	ctx, cancel := context.WithCancel(context.Background())
	e := &BandwidthlimitElement{
		ctx:      ctx,
		cancel:   cancel,
		perSecBw: 100000.0,
		limitBw:  100000.0,
		tokenBw:  100000.0,
		lastTs:   getNowTs(),
		lock:     &sync.Mutex{},
	}
	return e
}

func (e *BandwidthlimitElement) ProcessRequest(ctx context.Context, req *element.RPCRequest) (*element.RPCRequest, error) {
	nowTs := getNowTs()
	e.lock.Lock()
	defer e.lock.Unlock()
	// FIX(YW): this size does not include variable length fields
	sizeBw := float64(unsafe.Sizeof(req.Payload))
	e.tokenBw = min(float64(e.limitBw), float64(e.tokenBw+e.perSecBw*(nowTs-e.lastTs)/1000000.0))
	e.lastTs = nowTs
	if e.tokenBw >= sizeBw {
		e.tokenBw -= sizeBw
		return req, nil
	} else {
		return nil, &rpc.RPCError{Type: rpc.RPCFailError, Reason: "bandwidth limit"}
	}
}

func (e *BandwidthlimitElement) ProcessResponse(ctx context.Context, resp *element.RPCResponse) (*element.RPCResponse, error) {
	return resp, nil
}

func (e *BandwidthlimitElement) Name() string {
	return "bandwidthlimit"
}
