package main

import (
	"context"
	"fmt"
	"log"

	pb "github.com/appnet-org/aprc/examples/echo/proto"
	"github.com/appnet-org/aprc/internal/serializer"
	"github.com/appnet-org/aprc/pkg/rpc"
)

func main() {
	serializer := &serializer.ProtoSerializer{}
	client, err := rpc.NewClient(serializer, "127.0.0.1:9000")
	if err != nil {
		log.Fatal("Failed to create client:", err)
	}

	echoClient := pb.NewEchoServiceClient(client)

	req := &pb.EchoRequest{Message: "hello"}
	resp, err := echoClient.Echo(context.Background(), req)
	if err != nil {
		log.Fatal("RPC call failed:", err)
	}

	fmt.Println("Response:", resp.Message)
}
