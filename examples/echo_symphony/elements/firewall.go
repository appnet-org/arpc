package elements

import (
	"context"

	echo "github.com/appnet-org/arpc/examples/echo_symphony/symphony"
	"github.com/appnet-org/arpc/pkg/rpc"
	"github.com/appnet-org/arpc/pkg/rpc/element"
)

type FirewallElement struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func NewFirewallElement() element.RPCElement {
	ctx, cancel := context.WithCancel(context.Background())
	e := &FirewallElement{
		ctx:    ctx,
		cancel: cancel,
	}
	return e
}

func (e *FirewallElement) ProcessRequest(ctx context.Context, req *element.RPCRequest) (*element.RPCRequest, context.Context, error) {
	payload := req.Payload.(*echo.EchoRequest)
	if payload.GetContent() == "bomb" {
		return nil, ctx, &rpc.RPCError{Type: rpc.RPCFailError, Reason: "acl"}
	}
	return req, ctx, nil
}

func (e *FirewallElement) ProcessResponse(ctx context.Context, resp *element.RPCResponse) (*element.RPCResponse, context.Context, error) {
	return resp, ctx, nil
}

func (e *FirewallElement) Name() string {
	return "firewall"
}
