package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"testing"
	"time"

	"capnproto.org/go/capnp/v3"

	cp "github.com/appnet-org/arpc/benchmark/serialization/capnp"
	fb "github.com/appnet-org/arpc/benchmark/serialization/flatbuffers/benchmark"
	pb "github.com/appnet-org/arpc/benchmark/serialization/protobuf"
	syn "github.com/appnet-org/arpc/benchmark/serialization/symphony"
	flatbuffers "github.com/google/flatbuffers/go"
	"google.golang.org/protobuf/proto"
)

func main() {
	// Create benchmark results directory
	resultsDir := "results"
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		log.Fatalf("failed to create results directory: %v", err)
	}

	fmt.Println("Serialization Format Comparison")
	fmt.Println("================================")

	// Test data
	id := int32(42)
	score := int32(300)
	username := "alice"
	content := "hello world"

	fmt.Printf("Test data: id=%d, score=%d, username=%s, content=%s\n\n", id, score, username, content)

	// Test each serialization format
	testProtobuf(id, score, username, content)
	testSymphony(id, score, username, content)
	testCapnProto(id, score, username, content)
	testFlatBuffers(id, score, username, content)

	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("PERFORMANCE MEASUREMENTS")
	fmt.Println(strings.Repeat("=", 50))

	// Time measurements
	fmt.Println("\n--- MARSHAL TIME MEASUREMENTS ---")
	measureMarshalTime(id, score, username, content, resultsDir)

	fmt.Println("\n--- UNMARSHAL TIME MEASUREMENTS ---")
	measureUnmarshalTime(id, score, username, content, resultsDir)

	// CPU measurements
	fmt.Println("\n--- MARSHAL CPU USAGE MEASUREMENTS ---")
	measureMarshalCPU(id, score, username, content, resultsDir)

	fmt.Println("\n--- UNMARSHAL CPU USAGE MEASUREMENTS ---")
	measureUnmarshalCPU(id, score, username, content, resultsDir)

	fmt.Printf("\n✓ All benchmark results saved to: %s/\n", resultsDir)
}

func testProtobuf(id, score int32, username, content string) {
	fmt.Println("=== Protobuf ===")

	// Marshal
	pbReq := &pb.BenchmarkMessage{
		Id:       id,
		Score:    score,
		Username: username,
		Content:  content,
	}

	pbBytes, err := proto.Marshal(pbReq)
	if err != nil {
		log.Fatalf("protobuf marshal error: %v", err)
	}

	fmt.Printf("Marshaled size: %d bytes\n", len(pbBytes))
	fmt.Printf("Marshaled data: %x\n", pbBytes)

	// Unmarshal
	pbDecoded := &pb.BenchmarkMessage{}
	err = proto.Unmarshal(pbBytes, pbDecoded)
	if err != nil {
		log.Fatalf("protobuf unmarshal error: %v", err)
	}

	fmt.Printf("Unmarshaled: id=%d, score=%d, username=%s, content=%s\n",
		pbDecoded.Id, pbDecoded.Score, pbDecoded.Username, pbDecoded.Content)

	// Verify correctness
	if pbDecoded.Id == id && pbDecoded.Score == score &&
		pbDecoded.Username == username && pbDecoded.Content == content {
		fmt.Println("✓ Round-trip successful")
	} else {
		fmt.Println("✗ Round-trip failed")
	}
	fmt.Println()
}

func testSymphony(id, score int32, username, content string) {
	fmt.Println("=== Symphony ===")

	// Marshal
	synReq := &syn.BenchmarkMessage{
		Id:       id,
		Score:    score,
		Username: username,
		Content:  content,
	}

	synBytes, err := synReq.MarshalSymphony()
	if err != nil {
		log.Fatalf("symphony marshal error: %v", err)
	}

	fmt.Printf("Marshaled size: %d bytes\n", len(synBytes))
	fmt.Printf("Marshaled data: %x\n", synBytes)

	// Unmarshal
	synDecoded := &syn.BenchmarkMessage{}
	err = synDecoded.UnmarshalSymphony(synBytes)
	if err != nil {
		log.Fatalf("symphony unmarshal error: %v", err)
	}

	fmt.Printf("Unmarshaled: id=%d, score=%d, username=%s, content=%s\n",
		synDecoded.Id, synDecoded.Score, synDecoded.Username, synDecoded.Content)

	// Verify correctness
	if synDecoded.Id == id && synDecoded.Score == score &&
		synDecoded.Username == username && synDecoded.Content == content {
		fmt.Println("✓ Round-trip successful")
	} else {
		fmt.Println("✗ Round-trip failed")
	}
	fmt.Println()
}

func testCapnProto(id, score int32, username, content string) {
	fmt.Println("=== Cap'n Proto ===")

	// Marshal
	msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		log.Fatalf("capnp message creation error: %v", err)
	}

	cpReq, err := cp.NewRootBenchmarkMessage(seg)
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

	fmt.Printf("Marshaled size: %d bytes\n", len(cpBytes))
	fmt.Printf("Marshaled data: %x\n", cpBytes)

	// Unmarshal
	cpMsg, err := capnp.Unmarshal(cpBytes)
	if err != nil {
		log.Fatalf("capnp unmarshal error: %v", err)
	}

	cpDecoded, err := cp.ReadRootBenchmarkMessage(cpMsg)
	if err != nil {
		log.Fatalf("capnp read root error: %v", err)
	}

	decodedUsername, err := cpDecoded.Username()
	if err != nil {
		log.Fatalf("capnp read username error: %v", err)
	}

	decodedContent, err := cpDecoded.Content()
	if err != nil {
		log.Fatalf("capnp read content error: %v", err)
	}

	fmt.Printf("Unmarshaled: id=%d, score=%d, username=%s, content=%s\n",
		cpDecoded.Id(), cpDecoded.Score(), decodedUsername, decodedContent)

	// Verify correctness
	if cpDecoded.Id() == id && cpDecoded.Score() == score &&
		decodedUsername == username && decodedContent == content {
		fmt.Println("✓ Round-trip successful")
	} else {
		fmt.Println("✗ Round-trip failed")
	}
	fmt.Println()
}

func testFlatBuffers(id, score int32, username, content string) {
	fmt.Println("=== FlatBuffers ===")

	// Marshal
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

	fbBytes := builder.FinishedBytes()

	fmt.Printf("Marshaled size: %d bytes\n", len(fbBytes))
	fmt.Printf("Marshaled data: %x\n", fbBytes)

	// Unmarshal
	fbDecoded := fb.GetRootAsBenchmarkMessage(fbBytes, 0)

	decodedUsername := string(fbDecoded.Username())
	decodedContent := string(fbDecoded.Content())

	fmt.Printf("Unmarshaled: id=%d, score=%d, username=%s, content=%s\n",
		fbDecoded.Id(), fbDecoded.Score(), decodedUsername, decodedContent)

	// Verify correctness
	if fbDecoded.Id() == id && fbDecoded.Score() == score &&
		decodedUsername == username && decodedContent == content {
		fmt.Println("✓ Round-trip successful")
	} else {
		fmt.Println("✗ Round-trip failed")
	}
	fmt.Println()
}

// measureMarshalTime uses Go's testing.Benchmark for advanced timing measurements
func measureMarshalTime(id, score int32, username, content string, resultsDir string) {
	fmt.Println("Measuring marshal time using Go's testing.Benchmark:")

	// Create timing results log file
	logFile, err := os.Create(filepath.Join(resultsDir, "marshal_timing_results.log"))
	if err != nil {
		log.Fatalf("could not create timing log file: %v", err)
	}
	defer logFile.Close()

	logFile.WriteString("=== MARSHAL TIME MEASUREMENTS ===\n")
	logFile.WriteString(fmt.Sprintf("Test data: id=%d, score=%d, username=%s, content=%s\n", id, score, username, content))
	logFile.WriteString(fmt.Sprintf("Timestamp: %s\n\n", time.Now().Format(time.RFC3339)))

	// Protobuf benchmark
	pbResult := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			pbReq := &pb.BenchmarkMessage{Id: id, Score: score, Username: username, Content: content}
			_, _ = proto.Marshal(pbReq)
		}
	})
	pbResultStr := formatBenchmarkResult(pbResult)
	fmt.Printf("Protobuf:    %s\n", pbResultStr)
	logFile.WriteString(fmt.Sprintf("Protobuf:    %s\n", pbResultStr))

	// Symphony benchmark
	synResult := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			synReq := &syn.BenchmarkMessage{Id: id, Score: score, Username: username, Content: content}
			_, _ = synReq.MarshalSymphony()
		}
	})
	synResultStr := formatBenchmarkResult(synResult)
	fmt.Printf("Symphony:    %s\n", synResultStr)
	logFile.WriteString(fmt.Sprintf("Symphony:    %s\n", synResultStr))

	// Cap'n Proto benchmark
	cpResult := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			msg, seg, _ := capnp.NewMessage(capnp.SingleSegment(nil))
			cpReq, _ := cp.NewRootBenchmarkMessage(seg)
			cpReq.SetId(id)
			cpReq.SetScore(score)
			cpReq.SetUsername(username)
			cpReq.SetContent(content)
			_, _ = msg.Marshal()
		}
	})
	cpResultStr := formatBenchmarkResult(cpResult)
	fmt.Printf("Cap'n Proto: %s\n", cpResultStr)
	logFile.WriteString(fmt.Sprintf("Cap'n Proto: %s\n", cpResultStr))

	// FlatBuffers benchmark
	fbResult := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
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
			_ = builder.FinishedBytes()
		}
	})
	fbResultStr := formatBenchmarkResult(fbResult)
	fmt.Printf("FlatBuffers: %s\n", fbResultStr)
	logFile.WriteString(fmt.Sprintf("FlatBuffers: %s\n", fbResultStr))

	fmt.Printf("✓ Marshal timing results saved to: %s\n", filepath.Join(resultsDir, "marshal_timing_results.log"))
}

// measureUnmarshalTime uses Go's testing.Benchmark for unmarshal timing measurements
func measureUnmarshalTime(id, score int32, username, content string, resultsDir string) {
	fmt.Println("Measuring unmarshal time using Go's testing.Benchmark:")

	// Create timing results log file
	logFile, err := os.Create(filepath.Join(resultsDir, "unmarshal_timing_results.log"))
	if err != nil {
		log.Fatalf("could not create unmarshal timing log file: %v", err)
	}
	defer logFile.Close()

	logFile.WriteString("=== UNMARSHAL TIME MEASUREMENTS ===\n")
	logFile.WriteString(fmt.Sprintf("Test data: id=%d, score=%d, username=%s, content=%s\n", id, score, username, content))
	logFile.WriteString(fmt.Sprintf("Timestamp: %s\n\n", time.Now().Format(time.RFC3339)))

	// Prepare serialized data for each format
	pbReq := &pb.BenchmarkMessage{Id: id, Score: score, Username: username, Content: content}
	pbBytes, _ := proto.Marshal(pbReq)

	synReq := &syn.BenchmarkMessage{Id: id, Score: score, Username: username, Content: content}
	synBytes, _ := synReq.MarshalSymphony()

	msg, seg, _ := capnp.NewMessage(capnp.SingleSegment(nil))
	cpReq, _ := cp.NewRootBenchmarkMessage(seg)
	cpReq.SetId(id)
	cpReq.SetScore(score)
	cpReq.SetUsername(username)
	cpReq.SetContent(content)
	cpBytes, _ := msg.Marshal()

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
	fbBytes := builder.FinishedBytes()

	// Protobuf unmarshal benchmark
	pbResult := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			pbDecoded := &pb.BenchmarkMessage{}
			_ = proto.Unmarshal(pbBytes, pbDecoded)
		}
	})
	pbResultStr := formatBenchmarkResult(pbResult)
	fmt.Printf("Protobuf:    %s\n", pbResultStr)
	logFile.WriteString(fmt.Sprintf("Protobuf:    %s\n", pbResultStr))

	// Symphony unmarshal benchmark
	synResult := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			synDecoded := &syn.BenchmarkMessage{}
			_ = synDecoded.UnmarshalSymphony(synBytes)
		}
	})
	synResultStr := formatBenchmarkResult(synResult)
	fmt.Printf("Symphony:    %s\n", synResultStr)
	logFile.WriteString(fmt.Sprintf("Symphony:    %s\n", synResultStr))

	// Cap'n Proto unmarshal benchmark
	cpResult := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			cpMsg, _ := capnp.Unmarshal(cpBytes)
			cpDecoded, _ := cp.ReadRootBenchmarkMessage(cpMsg)
			_, _ = cpDecoded.Username()
			_, _ = cpDecoded.Content()
			_ = cpDecoded.Id()
			_ = cpDecoded.Score()
		}
	})
	cpResultStr := formatBenchmarkResult(cpResult)
	fmt.Printf("Cap'n Proto: %s\n", cpResultStr)
	logFile.WriteString(fmt.Sprintf("Cap'n Proto: %s\n", cpResultStr))

	// FlatBuffers unmarshal benchmark
	fbResult := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			fbDecoded := fb.GetRootAsBenchmarkMessage(fbBytes, 0)
			// Actually access the fields to trigger deserialization work
			_ = fbDecoded.Id()
			_ = fbDecoded.Score()
			_ = string(fbDecoded.Username())
			_ = string(fbDecoded.Content())
		}
	})
	fbResultStr := formatBenchmarkResult(fbResult)
	fmt.Printf("FlatBuffers: %s\n", fbResultStr)
	logFile.WriteString(fmt.Sprintf("FlatBuffers: %s\n", fbResultStr))

	fmt.Printf("✓ Unmarshal timing results saved to: %s\n", filepath.Join(resultsDir, "unmarshal_timing_results.log"))
}

// formatBenchmarkResult formats testing.BenchmarkResult for display
func formatBenchmarkResult(result testing.BenchmarkResult) string {
	nsPerOp := result.NsPerOp()
	allocsPerOp := result.AllocsPerOp()
	bytesPerOp := result.AllocedBytesPerOp()

	return fmt.Sprintf("%d iterations, %s/op, %d allocs/op, %d B/op",
		result.N,
		formatDuration(time.Duration(nsPerOp)),
		allocsPerOp,
		bytesPerOp)
}

// formatDuration formats duration with appropriate units
func formatDuration(d time.Duration) string {
	return fmt.Sprintf("%dns", d.Nanoseconds())
}

// measureMarshalCPU uses Go's CPU profiler to generate CPU profiles for marshal operations
func measureMarshalCPU(id, score int32, username, content string, resultsDir string) {
	const iterations = 200000 // More iterations for better CPU profiling

	fmt.Println("Measuring CPU usage using Go's CPU profiler:")
	fmt.Println("Generating CPU profile files for analysis...")

	// Profile Protobuf
	profileMarshalFunction(filepath.Join(resultsDir, "protobuf_marshal.prof"), func() {
		pbReq := &pb.BenchmarkMessage{Id: id, Score: score, Username: username, Content: content}
		for i := 0; i < iterations; i++ {
			_, _ = proto.Marshal(pbReq)
		}
	})
	fmt.Printf("✓ Protobuf CPU profile saved to: %s\n", filepath.Join(resultsDir, "protobuf_marshal.prof"))

	// Profile Symphony
	profileMarshalFunction(filepath.Join(resultsDir, "symphony_marshal.prof"), func() {
		synReq := &syn.BenchmarkMessage{Id: id, Score: score, Username: username, Content: content}
		for i := 0; i < iterations; i++ {
			_, _ = synReq.MarshalSymphony()
		}
	})
	fmt.Printf("✓ Symphony CPU profile saved to: %s\n", filepath.Join(resultsDir, "symphony_marshal.prof"))

	// Profile Cap'n Proto
	profileMarshalFunction(filepath.Join(resultsDir, "capnproto_marshal.prof"), func() {
		for i := 0; i < iterations; i++ {
			msg, seg, _ := capnp.NewMessage(capnp.SingleSegment(nil))
			cpReq, _ := cp.NewRootBenchmarkMessage(seg)
			cpReq.SetId(id)
			cpReq.SetScore(score)
			cpReq.SetUsername(username)
			cpReq.SetContent(content)
			_, _ = msg.Marshal()
		}
	})
	fmt.Printf("✓ Cap'n Proto CPU profile saved to: %s\n", filepath.Join(resultsDir, "capnproto_marshal.prof"))

	// Profile FlatBuffers
	profileMarshalFunction(filepath.Join(resultsDir, "flatbuffers_marshal.prof"), func() {
		for i := 0; i < iterations; i++ {
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
			_ = builder.FinishedBytes()
		}
	})
	fmt.Printf("✓ FlatBuffers CPU profile saved to: %s\n", filepath.Join(resultsDir, "flatbuffers_marshal.prof"))

	fmt.Println("\nTo analyze marshal CPU profiles, use:")
	fmt.Printf("  go tool pprof %s\n", filepath.Join(resultsDir, "protobuf_marshal.prof"))
	fmt.Printf("  go tool pprof %s\n", filepath.Join(resultsDir, "symphony_marshal.prof"))
	fmt.Printf("  go tool pprof %s\n", filepath.Join(resultsDir, "capnproto_marshal.prof"))
	fmt.Printf("  go tool pprof %s\n", filepath.Join(resultsDir, "flatbuffers_marshal.prof"))
	fmt.Println("\nIn pprof, use commands like: top, list, web, png")
}

// profileMarshalFunction profiles a marshal function and saves the CPU profile
func profileMarshalFunction(filename string, marshalFunc func()) {
	// Create profile file
	f, err := os.Create(filename)
	if err != nil {
		log.Fatalf("could not create CPU profile file %s: %v", filename, err)
	}
	defer f.Close()

	// Start CPU profiling
	if err := pprof.StartCPUProfile(f); err != nil {
		log.Fatalf("could not start CPU profile: %v", err)
	}
	defer pprof.StopCPUProfile()

	// Run the marshal function (this is where CPU profiling happens)
	marshalFunc()
}

// measureUnmarshalCPU uses Go's CPU profiler to generate CPU profiles for unmarshal operations
func measureUnmarshalCPU(id, score int32, username, content string, resultsDir string) {
	const iterations = 200000 // More iterations for better CPU profiling

	fmt.Println("Measuring unmarshal CPU usage using Go's CPU profiler:")
	fmt.Println("Generating unmarshal CPU profile files for analysis...")

	// Prepare serialized data for each format
	pbReq := &pb.BenchmarkMessage{Id: id, Score: score, Username: username, Content: content}
	pbBytes, _ := proto.Marshal(pbReq)

	synReq := &syn.BenchmarkMessage{Id: id, Score: score, Username: username, Content: content}
	synBytes, _ := synReq.MarshalSymphony()

	msg, seg, _ := capnp.NewMessage(capnp.SingleSegment(nil))
	cpReq, _ := cp.NewRootBenchmarkMessage(seg)
	cpReq.SetId(id)
	cpReq.SetScore(score)
	cpReq.SetUsername(username)
	cpReq.SetContent(content)
	cpBytes, _ := msg.Marshal()

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
	fbBytes := builder.FinishedBytes()

	// Profile Protobuf unmarshal
	profileMarshalFunction(filepath.Join(resultsDir, "protobuf_unmarshal.prof"), func() {
		for i := 0; i < iterations; i++ {
			pbDecoded := &pb.BenchmarkMessage{}
			_ = proto.Unmarshal(pbBytes, pbDecoded)
		}
	})
	fmt.Printf("✓ Protobuf unmarshal CPU profile saved to: %s\n", filepath.Join(resultsDir, "protobuf_unmarshal.prof"))

	// Profile Symphony unmarshal
	profileMarshalFunction(filepath.Join(resultsDir, "symphony_unmarshal.prof"), func() {
		for i := 0; i < iterations; i++ {
			synDecoded := &syn.BenchmarkMessage{}
			_ = synDecoded.UnmarshalSymphony(synBytes)
		}
	})
	fmt.Printf("✓ Symphony unmarshal CPU profile saved to: %s\n", filepath.Join(resultsDir, "symphony_unmarshal.prof"))

	// Profile Cap'n Proto unmarshal
	profileMarshalFunction(filepath.Join(resultsDir, "capnproto_unmarshal.prof"), func() {
		for i := 0; i < iterations; i++ {
			cpMsg, _ := capnp.Unmarshal(cpBytes)
			cpDecoded, _ := cp.ReadRootBenchmarkMessage(cpMsg)
			_, _ = cpDecoded.Username()
			_, _ = cpDecoded.Content()
			_ = cpDecoded.Id()
			_ = cpDecoded.Score()
		}
	})
	fmt.Printf("✓ Cap'n Proto unmarshal CPU profile saved to: %s\n", filepath.Join(resultsDir, "capnproto_unmarshal.prof"))

	// Profile FlatBuffers unmarshal
	profileMarshalFunction(filepath.Join(resultsDir, "flatbuffers_unmarshal.prof"), func() {
		for i := 0; i < iterations; i++ {
			fbDecoded := fb.GetRootAsBenchmarkMessage(fbBytes, 0)
			// Actually access the fields to trigger deserialization work
			_ = fbDecoded.Id()
			_ = fbDecoded.Score()
			_ = string(fbDecoded.Username())
			_ = string(fbDecoded.Content())
		}
	})
	fmt.Printf("✓ FlatBuffers unmarshal CPU profile saved to: %s\n", filepath.Join(resultsDir, "flatbuffers_unmarshal.prof"))

	fmt.Println("\nTo analyze unmarshal CPU profiles, use:")
	fmt.Printf("  go tool pprof %s\n", filepath.Join(resultsDir, "protobuf_unmarshal.prof"))
	fmt.Printf("  go tool pprof %s\n", filepath.Join(resultsDir, "symphony_unmarshal.prof"))
	fmt.Printf("  go tool pprof %s\n", filepath.Join(resultsDir, "capnproto_unmarshal.prof"))
	fmt.Printf("  go tool pprof %s\n", filepath.Join(resultsDir, "flatbuffers_unmarshal.prof"))
	fmt.Println("\nIn pprof, use commands like: top, list, web, png")
}
