package main

import (
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

	req := &pb.EchoRequest{Message: "hello"}
	resp := &pb.EchoResponse{}

	err = client.Call("echo", req, resp)
	fmt.Println("Response:", resp.Message)
}
