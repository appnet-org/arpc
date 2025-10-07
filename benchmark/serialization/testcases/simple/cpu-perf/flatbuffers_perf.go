package main

import (
	fb "github.com/appnet-org/arpc/benchmark/serialization/flatbuffers/benchmark"
	flatbuffers "github.com/google/flatbuffers/go"
)

func main() {
	// Test data
	id := int32(42)
	score := int32(300)
	username := "alice"
	content := "hello world"

	// Marshal benchmark
	builder := flatbuffers.NewBuilder(0)
	usernameOffset := builder.CreateString(username)
	contentOffset := builder.CreateString(content)

	fb.BenchmarkMessageStart(builder)
	fb.BenchmarkMessageAddId(builder, id)
	fb.BenchmarkMessageAddScore(builder, score)
	fb.BenchmarkMessageAddUsername(builder, usernameOffset)
	fb.BenchmarkMessageAddContent(builder, contentOffset)
	fbReq := fb.BenchmarkMessageEnd(builder)
	builder.Finish(fbReq)

	data := builder.FinishedBytes()

	// Unmarshal benchmark
	fbDecoded := fb.GetRootAsBenchmarkMessage(data, 0)

	_ = fbDecoded.Id()
	_ = fbDecoded.Score()
	_ = string(fbDecoded.Username())
	_ = string(fbDecoded.Content())
}
