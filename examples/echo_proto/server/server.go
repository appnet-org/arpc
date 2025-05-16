package main

import (
	"context"
	"fmt"
	"log"

	echo "github.com/appnet-org/arpc/examples/echo_proto/proto"
	"github.com/appnet-org/arpc/pkg/metadata"
	"github.com/appnet-org/arpc/pkg/rpc"
	"github.com/appnet-org/arpc/pkg/serializer"
)

// EchoService implementation
type echoServer struct{}

func (s *echoServer) Echo(ctx context.Context, req *echo.EchoRequest) (*echo.EchoResponse, context.Context, error) {

	log.Printf("Server got: [%s]", req.GetMessage())

	// Inject some outgoing metadata for the response
	md := metadata.New(map[string]string{
		"handled-by": "echoServer",
		"req-len":    fmt.Sprintf("%d", len(req.GetMessage())),
	})
	respCtx := metadata.NewOutgoingContext(ctx, md)

	resp := &echo.EchoResponse{
		Message: "Echo " + req.GetMessage(),
	}

	return resp, respCtx, nil
}

func main() {
	serializer := &serializer.ProtoSerializer{}
	server, err := rpc.NewServer(":9000", serializer)
	if err != nil {
		log.Fatal("Failed to start server:", err)
	}

	echo.RegisterEchoServiceServer(server, &echoServer{})
	server.Start()
}
