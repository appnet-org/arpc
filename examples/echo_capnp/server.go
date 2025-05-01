package main

import (
	"context"
	"log"

	echo "github.com/appnet-org/arpc/examples/echo_capnp/capnp"
	"github.com/appnet-org/arpc/internal/serializer"
	"github.com/appnet-org/arpc/pkg/rpc"
)

// EchoService implementation
type echoServer struct{}

func (s *echoServer) Echo(ctx context.Context, req *echo.EchoRequest_) (*echo.EchoResponse_, error) {
	reqContent, err := req.GetContent()
	if err != nil {
		return nil, err
	}

	log.Printf("Server got: [%s]", reqContent)

	resp, err := echo.CreateEchoResponse(
		"Echo " + reqContent,
	)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func main() {
	serializer := &serializer.CapnpSerializer{}
	server, err := rpc.NewServer(":9000", serializer)
	if err != nil {
		log.Fatal("Failed to start server:", err)
	}

	echo.RegisterEchoServiceServer(server, &echoServer{})
	server.Start()
}
