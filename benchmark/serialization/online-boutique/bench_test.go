package main

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

// --- INITIALIZATION ---
func init() {
	// Load all payloads from JSONL files
	if err := loadAllPayloads(); err != nil {
		panic(fmt.Sprintf("Failed to load payloads: %v", err))
	}

	if len(payloadEntries) == 0 {
		panic("No payloads loaded")
	}

	fmt.Printf("Loaded %d payload entries\n", len(payloadEntries))

	// Initialize pre-serialized buffers
	protoBufs = make([][]byte, len(payloadEntries))
	flatBufs = make([][]byte, len(payloadEntries))
	capnpBufs = make([][]byte, len(payloadEntries))
	symphonyBufs = make([][]byte, len(payloadEntries))
	hybridBufs = make([][]byte, len(payloadEntries))

	// Pre-serialize all messages
	for i, entry := range payloadEntries {
		// Proto
		buf, err := serializeProto(entry.Message)
		if err != nil {
			panic(fmt.Sprintf("Warning: failed to serialize proto for entry %d (%s): %v\n", i, entry.TypeName, err))
		} else {
			protoBufs[i] = buf
		}

		// FlatBuffers
		buf, err = serializeFlatbuffers(entry.Message)
		if err != nil {
			panic(fmt.Sprintf("Warning: failed to serialize flatbuffers for entry %d (%s): %v\n", i, entry.TypeName, err))
		} else {
			flatBufs[i] = buf
		}

		// Cap'n Proto
		buf, err = serializeCapnp(entry.Message)
		if err != nil {
			panic(fmt.Sprintf("Warning: failed to serialize capnp for entry %d (%s): %v\n", i, entry.TypeName, err))
		} else {
			capnpBufs[i] = buf
		}

		// Symphony
		buf, err = serializeSymphony(entry.Message)
		if err != nil {
			panic(fmt.Sprintf("Warning: failed to serialize symphony for entry %d (%s): %v\n", i, entry.TypeName, err))
		} else {
			symphonyBufs[i] = buf
		}

		// Symphony Hybrid
		buf, err = serializeSymphonyHybrid(entry.Message)
		if err != nil {
			panic(fmt.Sprintf("Warning: failed to serialize symphony hybrid for entry %d (%s): %v\n", i, entry.TypeName, err))
		} else {
			hybridBufs[i] = buf
		}
	}

	fmt.Printf("Pre-serialized %d messages\n", len(payloadEntries))
}

func BenchmarkProtobuf_Write(b *testing.B) {
	timings := make([]int64, 0, b.N)
	sizes := make([]int, 0, b.N)
	payloadSize := len(payloadEntries)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		idx := i % payloadSize
		entry := payloadEntries[idx]

		start := time.Now()
		buf, err := serializeProto(entry.Message)
		if err != nil {
			b.Fatalf("Failed to serialize: %v", err)
		}
		elapsed := time.Since(start)
		timings = append(timings, elapsed.Nanoseconds())
		sizes = append(sizes, len(buf))
	}
	b.StopTimer()
	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}
	if err := writeTimings("protobuf_write_times.txt", timings); err != nil {
		b.Logf("Failed to write timing data: %v", err)
	}
	if err := writeSizes("protobuf_write_sizes.txt", sizes); err != nil {
		b.Logf("Failed to write size data: %v", err)
	}
	b.StartTimer()
}

func BenchmarkProtobuf_Read(b *testing.B) {
	timings := make([]int64, 0, b.N)
	payloadSize := len(payloadEntries)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		idx := i % payloadSize
		entry := payloadEntries[idx]
		in := protoBufs[idx]
		if in == nil {
			panic(fmt.Sprintf("failed to serialize Protobuf: %s", entry.TypeName))
		}

		start := time.Now()
		// Create a new instance of the message type
		msgType := getMessageType(entry.TypeName)
		if msgType == nil {
			panic(fmt.Sprintf("failed to get message type: %s", entry.TypeName))
		}
		msg := reflect.New(msgType.Elem()).Interface().(proto.Message)
		if err := proto.Unmarshal(in, msg); err != nil {
			panic(fmt.Sprintf("failed to unmarshal Protobuf: %v", err))
		}
		// Access all fields to ensure deserialization
		accessAllFields(msg)
		elapsed := time.Since(start)
		timings = append(timings, elapsed.Nanoseconds())
	}
	b.StopTimer()
	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}
	if err := writeTimings("protobuf_read_times.txt", timings); err != nil {
		b.Logf("Failed to write timing data: %v", err)
	}
	b.StartTimer()
}

func BenchmarkFlatBuffers_Write(b *testing.B) {
	timings := make([]int64, 0, b.N)
	sizes := make([]int, 0, b.N)
	payloadSize := len(payloadEntries)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		idx := i % payloadSize
		entry := payloadEntries[idx]

		start := time.Now()
		buf, err := serializeFlatbuffers(entry.Message)
		if err != nil {
			panic(fmt.Sprintf("failed to serialize FlatBuffers: %v", err))
		}
		elapsed := time.Since(start)
		timings = append(timings, elapsed.Nanoseconds())
		sizes = append(sizes, len(buf))
	}
	b.StopTimer()
	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}
	if err := writeTimings("flatbuffers_write_times.txt", timings); err != nil {
		b.Logf("Failed to write timing data: %v", err)
	}
	if err := writeSizes("flatbuffers_write_sizes.txt", sizes); err != nil {
		b.Logf("Failed to write size data: %v", err)
	}
	b.StartTimer()
}

func BenchmarkFlatBuffers_Read(b *testing.B) {
	timings := make([]int64, 0, b.N)
	payloadSize := len(payloadEntries)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		idx := i % payloadSize
		in := flatBufs[idx]
		if in == nil {
			panic(fmt.Sprintf("failed to unmarshal FlatBuffers: %s", payloadEntries[idx].TypeName))
		}

		start := time.Now()
		entry := payloadEntries[idx]
		if err := unmarshalFlatbuffersAndAccessFields(entry.TypeName, in); err != nil {
			panic(fmt.Sprintf("failed to unmarshal FlatBuffers: %v", err))
		}
		elapsed := time.Since(start)
		timings = append(timings, elapsed.Nanoseconds())
	}
	b.StopTimer()
	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}
	if err := writeTimings("flatbuffers_read_times.txt", timings); err != nil {
		b.Logf("Failed to write timing data: %v", err)
	}
	b.StartTimer()
}

func BenchmarkCapnp_Write(b *testing.B) {
	timings := make([]int64, 0, b.N)
	sizes := make([]int, 0, b.N)
	payloadSize := len(payloadEntries)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		idx := i % payloadSize
		entry := payloadEntries[idx]

		start := time.Now()
		buf, err := serializeCapnp(entry.Message)
		if err != nil {
			panic(fmt.Sprintf("failed to serialize Cap'n Proto: %v", err))
		}
		elapsed := time.Since(start)
		timings = append(timings, elapsed.Nanoseconds())
		sizes = append(sizes, len(buf))
	}
	b.StopTimer()
	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}
	if err := writeTimings("capnp_write_times.txt", timings); err != nil {
		b.Logf("Failed to write timing data: %v", err)
	}
	if err := writeSizes("capnp_write_sizes.txt", sizes); err != nil {
		b.Logf("Failed to write size data: %v", err)
	}
	b.StartTimer()
}

func BenchmarkCapnp_Read(b *testing.B) {
	timings := make([]int64, 0, b.N)
	payloadSize := len(payloadEntries)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		idx := i % payloadSize
		in := capnpBufs[idx]
		if in == nil {
			panic(fmt.Sprintf("failed to unmarshal Cap'n Proto: %s", payloadEntries[idx].TypeName))
		}

		start := time.Now()
		entry := payloadEntries[idx]
		if err := unmarshalCapnpAndAccessFields(entry.TypeName, in); err != nil {
			panic(fmt.Sprintf("failed to unmarshal Cap'n Proto: %v", err))
		}
		elapsed := time.Since(start)
		timings = append(timings, elapsed.Nanoseconds())
	}
	b.StopTimer()
	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}
	if err := writeTimings("capnp_read_times.txt", timings); err != nil {
		b.Logf("Failed to write timing data: %v", err)
	}
	b.StartTimer()
}

func BenchmarkSymphony_Write(b *testing.B) {
	timings := make([]int64, 0, b.N)
	sizes := make([]int, 0, b.N)
	payloadSize := len(payloadEntries)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		idx := i % payloadSize
		entry := payloadEntries[idx]

		start := time.Now()
		buf, err := serializeSymphony(entry.Message)
		if err != nil {
			panic(fmt.Sprintf("failed to serialize Symphony: %v", err))
		}
		elapsed := time.Since(start)
		timings = append(timings, elapsed.Nanoseconds())
		sizes = append(sizes, len(buf))
	}
	b.StopTimer()
	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}
	if err := writeTimings("symphony_write_times.txt", timings); err != nil {
		b.Logf("Failed to write timing data: %v", err)
	}
	if err := writeSizes("symphony_write_sizes.txt", sizes); err != nil {
		b.Logf("Failed to write size data: %v", err)
	}
	b.StartTimer()
}

func BenchmarkSymphony_Read(b *testing.B) {
	timings := make([]int64, 0, b.N)
	payloadSize := len(payloadEntries)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		idx := i % payloadSize
		entry := payloadEntries[idx]
		in := symphonyBufs[idx]
		if in == nil {
			panic(fmt.Sprintf("failed to unmarshal Symphony: %s", entry.TypeName))
		}

		start := time.Now()
		// Create a new instance and unmarshal
		msgType := getMessageType(entry.TypeName)
		if msgType == nil {
			panic(fmt.Sprintf("failed to get message type: %s", entry.TypeName))
		}
		msg := reflect.New(msgType.Elem()).Interface().(proto.Message)
		if err := unmarshalSymphony(msg, in); err != nil {
			panic(fmt.Sprintf("failed to unmarshal Symphony: %v", err))
		}
		// Access all fields to ensure deserialization
		accessAllFields(msg)
		elapsed := time.Since(start)
		timings = append(timings, elapsed.Nanoseconds())
	}
	b.StopTimer()
	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}
	if err := writeTimings("symphony_read_times.txt", timings); err != nil {
		b.Logf("Failed to write timing data: %v", err)
	}
	b.StartTimer()
}

func BenchmarkSymphonyHybrid_Write(b *testing.B) {
	timings := make([]int64, 0, b.N)
	sizes := make([]int, 0, b.N)
	payloadSize := len(payloadEntries)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		idx := i % payloadSize
		entry := payloadEntries[idx]

		start := time.Now()
		buf, err := serializeSymphonyHybrid(entry.Message)
		if err != nil {
			panic(fmt.Sprintf("failed to serialize Symphony Hybrid: %v", err))
		}
		elapsed := time.Since(start)
		timings = append(timings, elapsed.Nanoseconds())
		sizes = append(sizes, len(buf))
	}
	b.StopTimer()
	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}
	if err := writeTimings("symphony_hybrid_write_times.txt", timings); err != nil {
		b.Logf("Failed to write timing data: %v", err)
	}
	if err := writeSizes("symphony_hybrid_write_sizes.txt", sizes); err != nil {
		b.Logf("Failed to write size data: %v", err)
	}
	b.StartTimer()
}

func BenchmarkSymphonyHybrid_Read(b *testing.B) {
	timings := make([]int64, 0, b.N)
	payloadSize := len(payloadEntries)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		idx := i % payloadSize
		entry := payloadEntries[idx]
		in := hybridBufs[idx]
		if in == nil {
			panic(fmt.Sprintf("failed to unmarshal Symphony Hybrid: %s", entry.TypeName))
		}

		start := time.Now()
		// Create a new instance and unmarshal
		msgType := getMessageType(entry.TypeName)
		if msgType == nil {
			panic(fmt.Sprintf("failed to get message type: %s", entry.TypeName))
		}
		msg := reflect.New(msgType.Elem()).Interface().(proto.Message)
		if err := unmarshalSymphonyHybrid(msg, in); err != nil {
			panic(fmt.Sprintf("failed to unmarshal Symphony Hybrid: %v", err))
		}
		// Access all fields to ensure deserialization
		accessAllFields(msg)
		elapsed := time.Since(start)
		timings = append(timings, elapsed.Nanoseconds())
	}
	b.StopTimer()
	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}
	if err := writeTimings("symphony_hybrid_read_times.txt", timings); err != nil {
		b.Logf("Failed to write timing data: %v", err)
	}
	b.StartTimer()
}
