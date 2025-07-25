package main

import (
	"fmt"
	"log"

	"capnproto.org/go/capnp/v3"

	cp "github.com/appnet-org/arpc/examples/serialization/capnp"
	fb "github.com/appnet-org/arpc/examples/serialization/flatbuffers/echo"
	pb "github.com/appnet-org/arpc/examples/serialization/protobuf"
	syn "github.com/appnet-org/arpc/examples/serialization/symphony"
	flatbuffers "github.com/google/flatbuffers/go"
	"google.golang.org/protobuf/proto"
)

func main() {
	id := int32(42)
	score := int32(100)
	content := "hello world"
	username := "alice"

	// --- Protobuf ---
	pbReq := &pb.EchoRequest{
		Id:       id,
		Score:    score,
		Username: username,
		Content:  content,
	}
	pbBytes, err := proto.Marshal(pbReq)
	if err != nil {
		log.Fatalf("protobuf marshal error: %v", err)
	}

	// --- Symphony ---
	symphonyReq := &syn.EchoRequest{
		Id:       id,
		Score:    score,
		Username: username,
		Content:  content,
	}
	symphonyBytes, err := symphonyReq.MarshalSymphony()
	if err != nil {
		log.Fatalf("symphony marshal error: %v", err)
	}

	// --- Cap’n Proto ---
	msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		log.Fatalf("capnp message creation error: %v", err)
	}
	cpReq, err := cp.NewRootEchoRequest(seg)
	if err != nil {
		log.Fatalf("capnp root error: %v", err)
	}
	cpReq.SetId(id)
	cpReq.SetScore(score)
	cpReq.SetUsername(username)
	cpReq.SetContent(content)
	cpBytes, err := msg.Marshal()
	if err != nil {
		log.Fatalf("capnp marshal error: %v", err)
	}

	// --- FlatBuffers ---
	builder := flatbuffers.NewBuilder(0)
	usernameOffset := builder.CreateString(username)
	contentOffset := builder.CreateString(content)
	fb.EchoRequestStart(builder)
	fb.EchoRequestAddId(builder, id)
	fb.EchoRequestAddScore(builder, score)
	fb.EchoRequestAddUsername(builder, usernameOffset)
	fb.EchoRequestAddContent(builder, contentOffset)
	fbReq := fb.EchoRequestEnd(builder)
	builder.Finish(fbReq)
	fbBytes := builder.FinishedBytes()

	// // Print sizes and encodings
	fmt.Printf("Protobuf (%d bytes): %x\n", len(pbBytes), pbBytes)
	fmt.Printf("Cap'n Proto (%d bytes): %x\n", len(cpBytes), cpBytes)
	fmt.Printf("FlatBuffers (%d bytes): %x\n", len(fbBytes), fbBytes)
	fmt.Printf("Symphony (%d bytes): %x\n", len(symphonyBytes), symphonyBytes)
}
