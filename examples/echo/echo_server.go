package main

import (
	"context"
	"log"

	pb "github.com/appnet-org/aprc/examples/echo/proto"
	"github.com/appnet-org/aprc/internal/serializer"
	"github.com/appnet-org/aprc/pkg/rpc"
)

// EchoService implementation
type echoServer struct{}

func (s *echoServer) Echo(ctx context.Context, req *pb.EchoRequest) (*pb.EchoResponse, error) {
	log.Printf("Received message from client: %s", req.Message)
	return &pb.EchoResponse{Message: "Echo: " + req.Message}, nil
}

func main() {
	serializer := &serializer.ProtoSerializer{}
	server, err := rpc.NewServer(":9000", serializer)
	if err != nil {
		log.Fatal("Failed to start server:", err)
	}

	pb.RegisterEchoServiceServer(server, &echoServer{})
	server.Start()
}
