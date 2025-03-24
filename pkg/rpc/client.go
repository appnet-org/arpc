// pkg/rpc/client.go
package rpc

import (
	"github.com/appnet-org/aprc/internal/transport"
	"github.com/appnet-org/aprc/internal/protocol"
)

type Client struct {
	transport *transport.UDPTransport
}

func NewClient() *Client {
	return &Client{}
}

func (c *Client) Call(addr string, msg *protocol.RPCMessage) (*protocol.RPCMessage, error) {
	transport, err := transport.NewUDPTransport("")
	if err != nil {
		return nil, err
	}
	defer transport.Close()
	
	data, err := protocol.EncodeMessage(msg)
	if err != nil {
		return nil, err
	}
	if err := transport.Send(addr, data); err != nil {
		return nil, err
	}
	respData, _, err := transport.Receive(1024)
	if err != nil {
		return nil, err
	}
	return protocol.DecodeMessage(respData)
}
