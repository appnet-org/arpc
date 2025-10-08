package main

import (
	"log"

	"capnproto.org/go/capnp/v3"
	cp "github.com/appnet-org/arpc/benchmark/serialization/capnp"
)

func main() {
	// Test data
	id := int32(42)
	score := int32(300)
	username := "alice"
	content := "hello world"

	// Marshal benchmark
	msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		log.Fatal(err)
	}

	cpReq, err := cp.NewRootBenchmarkMessage(seg)
	if err != nil {
		log.Fatal(err)
	}

	cpReq.SetId(id)
	cpReq.SetScore(score)
	cpReq.SetUsername(username)
	cpReq.SetContent(content)

	data, err := msg.Marshal()
	if err != nil {
		log.Fatal(err)
	}

	// Unmarshal benchmark
	cpMsg, err := capnp.Unmarshal(data)
	if err != nil {
		log.Fatal(err)
	}

	cpDecoded, err := cp.ReadRootBenchmarkMessage(cpMsg)
	if err != nil {
		log.Fatal(err)
	}

	_, err = cpDecoded.Username()
	if err != nil {
		log.Fatal(err)
	}

	_, err = cpDecoded.Content()
	if err != nil {
		log.Fatal(err)
	}

	_ = cpDecoded.Id()
	_ = cpDecoded.Score()
}
