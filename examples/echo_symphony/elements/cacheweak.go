package elements

import (
	"context"
	"sync"

	echo "github.com/appnet-org/arpc/examples/echo_symphony/symphony"
	"github.com/appnet-org/arpc/pkg/rpc"
	"github.com/appnet-org/arpc/pkg/rpc/element"
)

type CacheweakElement struct {
	ctx            context.Context
	cancel         context.CancelFunc
	cacheTable     map[string]string
	cacheTableLock *sync.RWMutex
	bodyRecord     map[uint64]string
	bodyRecordLock *sync.RWMutex
}

func NewCacheweakElement() element.RPCElement {
	ctx, cancel := context.WithCancel(context.Background())
	e := &CacheweakElement{
		ctx:            ctx,
		cancel:         cancel,
		cacheTable:     make(map[string]string),
		cacheTableLock: &sync.RWMutex{},
		bodyRecord:     make(map[uint64]string),
		bodyRecordLock: &sync.RWMutex{},
	}
	// TODO(YW): add a background thread to periodically sync the state to a persistent storage
	return e
}

func (e *CacheweakElement) ProcessRequest(ctx context.Context, req *element.RPCRequest) (*element.RPCRequest, error) {
	content := req.Payload.(*echo.EchoRequest).GetContent()

	e.cacheTableLock.RLock()
	_, ok := e.cacheTable[content]
	e.cacheTableLock.RUnlock()
	if ok {
		// cache hit, return an error
		return nil, &rpc.RPCError{Type: rpc.RPCFailError, Reason: "cached"}
	}

	e.bodyRecordLock.Lock()
	e.bodyRecord[req.ID] = content
	e.bodyRecordLock.Unlock()

	return req, nil
}

func (e *CacheweakElement) ProcessResponse(ctx context.Context, resp *element.RPCResponse) (*element.RPCResponse, error) {
	if echoResp, ok := resp.Result.(*echo.EchoResponse); ok {
		respContent := echoResp.GetContent()

		e.bodyRecordLock.RLock()
		content := e.bodyRecord[resp.ID]
		e.bodyRecordLock.RUnlock()

		e.cacheTableLock.Lock()
		e.cacheTable[content] = respContent
		e.cacheTableLock.Unlock()
	}

	return resp, nil
}

func (e *CacheweakElement) Name() string {
	return "cacheweak"
}
