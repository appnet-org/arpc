package main

import (
	"context"
	"log"

	echo "github.com/appnet-org/arpc/examples/echo_symphony/symphony"
	"github.com/appnet-org/arpc/pkg/rpc"
	"github.com/appnet-org/arpc/pkg/serializer"
)

// EchoService implementation
type echoServer struct{}

func (s *echoServer) Echo(ctx context.Context, req *echo.EchoRequest) (*echo.EchoResponse, context.Context, error) {

	log.Printf("Server got: [%s]", req.GetContent())

	resp := &echo.EchoResponse{
		Id:       req.GetId(),
		Score:    req.GetScore(),
		Username: req.GetUsername(),
		Content:  "Echo " + req.GetContent(),
	}

	return resp, context.Background(), nil
}

func main() {
	serializer := &serializer.SymphonySerializer{}
	server, err := rpc.NewServer(":11000", serializer, nil)
	if err != nil {
		log.Fatal("Failed to start server:", err)
	}

	echo.RegisterEchoServiceServer(server, &echoServer{})
	server.Start()
}
