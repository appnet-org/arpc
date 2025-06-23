package main

import (
	"context"
	"fmt"
	"log"

	echo "github.com/appnet-org/arpc/examples/echo_capnp/capnp"
	"github.com/appnet-org/arpc/pkg/metadata"
	"github.com/appnet-org/arpc/pkg/rpc"
	"github.com/appnet-org/arpc/pkg/serializer"
)

// EchoService implementation
type echoServer struct{}

func (s *echoServer) Echo(ctx context.Context, req *echo.EchoRequest_) (*echo.EchoResponse_, context.Context, error) {
	reqContent, _ := req.GetContent()
	log.Printf("Server got: [%s]", reqContent)

	md := metadata.New(map[string]string{
		"handled-by": "echoServer",
		"req-len":    fmt.Sprintf("%d", len(reqContent)),
	})
	respCtx := metadata.NewOutgoingContext(ctx, md)

	resp, err := echo.CreateEchoResponse(
		3,                  // id
		30.0,               // score
		"Echo "+reqContent, // content
		"latest",           // tag
	)
	if err != nil {
		return nil, ctx, err
	}

	return resp, respCtx, nil
}

func main() {
	serializer := &serializer.CapnpSerializer{}
	server, err := rpc.NewServer(":9000", serializer, nil)
	if err != nil {
		log.Fatal("Failed to start server:", err)
	}

	echo.RegisterEchoServiceServer(server, &echoServer{})
	server.Start()
}
