package main

import (
	"log"

	pb "github.com/appnet-org/arpc/benchmark/serialization/protobuf"
	"google.golang.org/protobuf/proto"
)

func main() {
	// Test data
	id := int32(42)
	score := int32(300)
	username := "alice"
	content := "hello world"

	// Marshal benchmark
	pbReq := &pb.BenchmarkMessage{
		Id:       id,
		Score:    score,
		Username: username,
		Content:  content,
	}

	data, err := proto.Marshal(pbReq)
	if err != nil {
		log.Fatal(err)
	}

	// Unmarshal benchmark
	pbDecoded := &pb.BenchmarkMessage{}
	err = proto.Unmarshal(data, pbDecoded)
	if err != nil {
		log.Fatal(err)
	}
}
