package main

import (
	"fmt"
	"log"
	"net"

	"golang.org/x/net/context"

	"google.golang.org/grpc"

	echo "github.com/appnet-org/arpc/benchmark/grpc-echo/proto"
)

type server struct {
	echo.UnimplementedEchoServiceServer
}

func (s *server) Echo(ctx context.Context, req *echo.EchoRequest) (*echo.EchoResponse, error) {

	log.Printf("Server got: [%s]", req.GetMessage())

	msg := &echo.EchoResponse{
		Message: "Echo" + req.GetMessage(),
	}

	return msg, nil
}

func main() {
	lis, err := net.Listen("tcp", ":9000")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	srv := &server{}
	grpcServer := grpc.NewServer()

	fmt.Printf("Starting server pod at port 9000\n")

	echo.RegisterEchoServiceServer(grpcServer, srv)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
