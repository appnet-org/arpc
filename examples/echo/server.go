package main

import (
	"context"
	"log"

	echo "github.com/appnet-org/aprc/examples/echo/proto"
	"github.com/appnet-org/aprc/internal/serializer"
	"github.com/appnet-org/aprc/pkg/rpc"
)

// EchoService implementation
type echoServer struct{}

func (s *echoServer) Echo(ctx context.Context, req *echo.EchoRequest) (*echo.EchoResponse, error) {

	log.Printf("Server got: [%s]", req.GetMessage())

	resp := &echo.EchoResponse{
		Message: "Echo " + req.GetMessage(),
	}

	return resp, nil
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
