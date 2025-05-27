package main

import (
	"fmt"
	"log"
	"net"
	"time"

	"golang.org/x/net/context"

	echo "github.com/appnet-org/arpc/benchmark/echo-grpc/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type server struct {
	echo.UnimplementedEchoServiceServer
}

func (s *server) Echo(ctx context.Context, x *echo.EchoRequest) (*echo.EchoResponse, error) {

	// Log the HTTP headers received
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		log.Println("Received HTTP Headers:")
		for key, values := range md {
			log.Printf("  %s: %v", key, values)
		}
	} else {
		log.Println("No metadata (HTTP headers) received.")
	}

	log.Printf("Server got: [%s]", x.GetMessage())

	// Check if the message contains "sleep"
	if x.GetMessage() == "sleep" {
		log.Printf("Sleeping for 30 seconds...")
		time.Sleep(30 * time.Second)
	}

	msg := &echo.EchoResponse{
		Message: x.GetMessage(),
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
