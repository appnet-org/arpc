package main

import (
	"log"

	syn "github.com/appnet-org/arpc/benchmark/serialization/symphony"
)

func main() {
	// Test data
	id := int32(42)
	score := int32(300)
	username := "alice"
	content := "hello world"

	// Marshal benchmark
	synReq := &syn.BenchmarkMessage{
		Id:       id,
		Score:    score,
		Username: username,
		Content:  content,
	}

	data, err := synReq.MarshalSymphony()
	if err != nil {
		log.Fatal(err)
	}
	// Unmarshal benchmark using direct getter functions for maximum performance

	// _ = syn.GetIdFromBytes(data)
	// _ = syn.GetScoreFromBytes(data)
	// _ = syn.GetUsernameFromBytes(data)
	// _ = syn.GetContentFromBytes(data)
}
