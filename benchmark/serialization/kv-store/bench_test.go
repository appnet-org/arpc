package main

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	// Generated imports
	kv_capnp "github.com/appnet-org/arpc/benchmark/serialization/kv-store/capnp"
	"github.com/appnet-org/arpc/benchmark/serialization/kv-store/flatbuffers/kv_flat"
	kv_proto "github.com/appnet-org/arpc/benchmark/serialization/kv-store/proto"

	// Libraries
	"capnproto.org/go/capnp/v3"
	flatbuffers "github.com/google/flatbuffers/go"
	"google.golang.org/protobuf/proto"
)

// --- DATA CONTAINERS ---
type TraceEntry struct {
	Op        string // "GET" or "SET"
	Key       string
	KeySize   int
	ValueSize int
}

type NativeData struct {
	Key   string
	Value string
}

var (
	traceEntries []TraceEntry
	traceData    []NativeData

	// Pre-serialized buffers for Read/Deserialize benchmarks
	protoBufs    [][]byte
	flatBufs     [][]byte
	capnpBufs    [][]byte
	symphonyBufs [][]byte
)

// --- INITIALIZATION ---
func init() {
	// Load trace file - try paths relative to repo root or test file location
	var traceFile string
	possiblePaths := []string{
		filepath.Join("benchmark", "meta-kv-trace", "trace_large.req"), // from repo root
		filepath.Join("..", "..", "meta-kv-trace", "trace_large.req"),  // from kv-store dir
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			traceFile = path
			break
		}
	}

	if traceFile == "" {
		panic("Failed to find trace_large.req file. Tried: " + fmt.Sprintf("%v", possiblePaths))
	}

	entries, err := loadTrace(traceFile)
	if err != nil {
		panic(fmt.Sprintf("Failed to load trace file %s: %v", traceFile, err))
	}
	traceEntries = entries

	// Initialize data structures
	traceData = make([]NativeData, len(traceEntries))
	protoBufs = make([][]byte, len(traceEntries))
	flatBufs = make([][]byte, len(traceEntries))
	capnpBufs = make([][]byte, len(traceEntries))
	symphonyBufs = make([][]byte, len(traceEntries))

	// Pre-generate data and serialize based on trace entries
	for i, entry := range traceEntries {
		// Generate key and value strings based on sizes from trace
		key := generateString(entry.Key, entry.KeySize)
		value := generateString("", entry.ValueSize)

		traceData[i] = NativeData{
			Key:   key,
			Value: value,
		}

		// Pre-serialize based on operation type
		if entry.Op == "SET" {
			// SetRequest
			pSet := &kv_proto.SetRequest{Key: key, Value: value}
			protoBufs[i], _ = proto.Marshal(pSet)

			fb := flatbuffers.NewBuilder(0)
			k := fb.CreateString(key)
			v := fb.CreateString(value)
			kv_flat.SetRequestStart(fb)
			kv_flat.SetRequestAddKey(fb, k)
			kv_flat.SetRequestAddValue(fb, v)
			fb.Finish(kv_flat.SetRequestEnd(fb))
			flatBufs[i] = fb.FinishedBytes()

			msg, seg, _ := capnp.NewMessage(capnp.SingleSegment(nil))
			req, _ := kv_capnp.NewRootSetRequest(seg)
			req.SetKey([]byte(key))
			req.SetValue([]byte(value))
			capnpBufs[i], _ = msg.Marshal()

			// Symphony SetRequest
			synSet := &kv_proto.SetRequest{Key: key, Value: value}
			symphonyBufs[i], _ = synSet.MarshalSymphony()
		} else {
			// GetRequest
			pGet := &kv_proto.GetRequest{Key: key}
			protoBufs[i], _ = proto.Marshal(pGet)

			fb := flatbuffers.NewBuilder(0)
			k := fb.CreateString(key)
			kv_flat.GetRequestStart(fb)
			kv_flat.GetRequestAddKey(fb, k)
			fb.Finish(kv_flat.GetRequestEnd(fb))
			flatBufs[i] = fb.FinishedBytes()

			msg, seg, _ := capnp.NewMessage(capnp.SingleSegment(nil))
			req, _ := kv_capnp.NewRootGetRequest(seg)
			req.SetKey([]byte(key))
			capnpBufs[i], _ = msg.Marshal()

			// Symphony GetRequest
			synGet := &kv_proto.GetRequest{Key: key}
			symphonyBufs[i], _ = synGet.MarshalSymphony()
		}
	}
}

func loadTrace(filename string) ([]TraceEntry, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var entries []TraceEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		entry, err := parseTraceLine(line)
		if err != nil {
			continue // Skip malformed lines
		}
		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

func parseTraceLine(line string) (TraceEntry, error) {
	// Parse URL format: /?op={GET|SET}&key={key}&key_size={size}&value_size={size}
	parsed, err := url.Parse(line)
	if err != nil {
		return TraceEntry{}, err
	}

	params := parsed.Query()
	op := params.Get("op")
	key := params.Get("key")
	keySizeStr := params.Get("key_size")
	valueSizeStr := params.Get("value_size")

	keySize, err := strconv.Atoi(keySizeStr)
	if err != nil {
		return TraceEntry{}, err
	}

	valueSize, err := strconv.Atoi(valueSizeStr)
	if err != nil {
		return TraceEntry{}, err
	}

	return TraceEntry{
		Op:        op,
		Key:       key,
		KeySize:   keySize,
		ValueSize: valueSize,
	}, nil
}

func generateString(seed string, size int) string {
	if size <= 0 {
		return ""
	}

	// Use seed if provided, otherwise use a default
	source := seed
	if source == "" {
		source = "default"
	}

	// Pad or truncate to desired size
	if len(source) >= size {
		return source[:size]
	}

	// Repeat the source to reach desired size
	result := strings.Builder{}
	result.Grow(size)
	for result.Len() < size {
		remaining := size - result.Len()
		if remaining >= len(source) {
			result.WriteString(source)
		} else {
			result.WriteString(source[:remaining])
		}
	}
	return result.String()
}

// writeTimings writes timing data (in nanoseconds) to a file, one value per line
func writeTimings(filename string, timings []int64) error {
	// Create subdirectory for profile data
	dir := "profile_data"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Write to file in subdirectory
	path := filepath.Join(dir, filename)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, t := range timings {
		fmt.Fprintf(f, "%d\n", t)
	}
	return nil
}

func BenchmarkProtobuf_Write(b *testing.B) {
	timings := make([]int64, 0, b.N)
	traceSize := len(traceEntries)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		idx := i % traceSize
		entry := traceEntries[idx]
		item := traceData[idx]

		start := time.Now()
		if entry.Op == "SET" {
			obj := &kv_proto.SetRequest{Key: item.Key, Value: item.Value}
			_, _ = proto.Marshal(obj)
		} else {
			obj := &kv_proto.GetRequest{Key: item.Key}
			_, _ = proto.Marshal(obj)
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
	if err := writeTimings("protobuf_write_times.txt", timings); err != nil {
		b.Logf("Failed to write timing data: %v", err)
	}
	b.StartTimer()
}

func BenchmarkProtobuf_Read(b *testing.B) {
	timings := make([]int64, 0, b.N)
	traceSize := len(traceEntries)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		idx := i % traceSize
		entry := traceEntries[idx]
		in := protoBufs[idx]

		start := time.Now()
		if entry.Op == "SET" {
			var obj kv_proto.SetRequest
			proto.Unmarshal(in, &obj)
			_ = obj.GetKey()
			_ = obj.GetValue()
		} else {
			var obj kv_proto.GetRequest
			proto.Unmarshal(in, &obj)
			_ = obj.GetKey()
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
	if err := writeTimings("protobuf_read_times.txt", timings); err != nil {
		b.Logf("Failed to write timing data: %v", err)
	}
	b.StartTimer()
}

func BenchmarkFlatBuffers_Write(b *testing.B) {
	timings := make([]int64, 0, b.N)
	traceSize := len(traceEntries)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		builder := flatbuffers.NewBuilder(1024)
		idx := i % traceSize
		entry := traceEntries[idx]
		item := traceData[idx]

		start := time.Now()
		if entry.Op == "SET" {
			k := builder.CreateString(item.Key)
			v := builder.CreateString(item.Value)
			kv_flat.SetRequestStart(builder)
			kv_flat.SetRequestAddKey(builder, k)
			kv_flat.SetRequestAddValue(builder, v)
			builder.Finish(kv_flat.SetRequestEnd(builder))
			_ = builder.FinishedBytes()
		} else {
			k := builder.CreateString(item.Key)
			kv_flat.GetRequestStart(builder)
			kv_flat.GetRequestAddKey(builder, k)
			builder.Finish(kv_flat.GetRequestEnd(builder))
			_ = builder.FinishedBytes()
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
	if err := writeTimings("flatbuffers_write_times.txt", timings); err != nil {
		b.Logf("Failed to write timing data: %v", err)
	}
	b.StartTimer()
}

func BenchmarkFlatBuffers_Read(b *testing.B) {
	timings := make([]int64, 0, b.N)
	traceSize := len(traceEntries)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		idx := i % traceSize
		entry := traceEntries[idx]
		in := flatBufs[idx]

		start := time.Now()
		if entry.Op == "SET" {
			obj := kv_flat.GetRootAsSetRequest(in, 0)
			_ = string(obj.Key())
			_ = string(obj.Value())
		} else {
			obj := kv_flat.GetRootAsGetRequest(in, 0)
			_ = string(obj.Key())
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
	traceSize := len(traceEntries)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		idx := i % traceSize
		entry := traceEntries[idx]
		item := traceData[idx]

		start := time.Now()
		msg, seg, _ := capnp.NewMessage(capnp.SingleSegment(nil))

		if entry.Op == "SET" {
			req, _ := kv_capnp.NewRootSetRequest(seg)
			req.SetKey([]byte(item.Key))
			req.SetValue([]byte(item.Value))
		} else {
			req, _ := kv_capnp.NewRootGetRequest(seg)
			req.SetKey([]byte(item.Key))
		}
		_, _ = msg.Marshal()
		elapsed := time.Since(start)
		timings = append(timings, elapsed.Nanoseconds())
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
	b.StartTimer()
}

func BenchmarkCapnp_Read(b *testing.B) {
	timings := make([]int64, 0, b.N)
	traceSize := len(traceEntries)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		idx := i % traceSize
		entry := traceEntries[idx]
		in := capnpBufs[idx]

		start := time.Now()
		if entry.Op == "SET" {
			msg, _ := capnp.Unmarshal(in)
			obj, _ := kv_capnp.ReadRootSetRequest(msg)
			k, _ := obj.Key()
			v, _ := obj.Value()
			_ = string(k)
			_ = string(v)
		} else {
			msg, _ := capnp.Unmarshal(in)
			obj, _ := kv_capnp.ReadRootGetRequest(msg)
			k, _ := obj.Key()
			_ = string(k)
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
	traceSize := len(traceEntries)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		idx := i % traceSize
		entry := traceEntries[idx]
		item := traceData[idx]

		start := time.Now()
		if entry.Op == "SET" {
			obj := &kv_proto.SetRequest{Key: item.Key, Value: item.Value}
			_, _ = obj.MarshalSymphony()
		} else {
			obj := &kv_proto.GetRequest{Key: item.Key}
			_, _ = obj.MarshalSymphony()
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
	if err := writeTimings("symphony_write_times.txt", timings); err != nil {
		b.Logf("Failed to write timing data: %v", err)
	}
	b.StartTimer()
}

// func BenchmarkSymphony_Write_ZeroCopy(b *testing.B) {
// 	timings := make([]int64, 0, b.N)
// 	traceSize := len(traceEntries)
// 	b.ResetTimer()
// 	b.ReportAllocs()
// 	for i := 0; i < b.N; i++ {
// 		idx := i % traceSize
// 		entry := traceEntries[idx]
// 		item := traceData[idx]

// 		start := time.Now()
// 		if entry.Op == "SET" {
// 			// Create empty struct and marshal to get buffer structure
// 			empty := &kv_proto.SetRequest{}
// 			buf, _ := empty.MarshalSymphony()
// 			raw := kv_proto.SetRequestRaw(buf)
// 			_ = raw.SetKey(item.Key)
// 			_ = raw.SetValue(item.Value)
// 		} else {
// 			// Create empty struct and marshal to get buffer structure
// 			empty := &kv_proto.GetRequest{}
// 			buf, _ := empty.MarshalSymphony()
// 			raw := kv_proto.GetRequestRaw(buf)
// 			_ = raw.SetKey(item.Key)
// 		}
// 		elapsed := time.Since(start)
// 		timings = append(timings, elapsed.Nanoseconds())
// 	}
// 	b.StopTimer()
// 	if b.N > 0 {
// 		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
// 		msgPerSec := 1e9 / nsPerOp
// 		b.ReportMetric(msgPerSec, "msg/s")
// 	}
// 	if err := writeTimings("symphony_write_zerocopy_times.txt", timings); err != nil {
// 		b.Logf("Failed to write timing data: %v", err)
// 	}
// 	b.StartTimer()
// }

func BenchmarkSymphony_Read(b *testing.B) {
	timings := make([]int64, 0, b.N)
	traceSize := len(traceEntries)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		idx := i % traceSize
		entry := traceEntries[idx]
		in := symphonyBufs[idx]

		start := time.Now()
		if entry.Op == "SET" {
			var obj kv_proto.SetRequest
			_ = obj.UnmarshalSymphony(in)
			_ = obj.Key
			_ = obj.Value
		} else {
			var obj kv_proto.GetRequest
			_ = obj.UnmarshalSymphony(in)
			_ = obj.Key
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
	if err := writeTimings("symphony_read_times.txt", timings); err != nil {
		b.Logf("Failed to write timing data: %v", err)
	}
	b.StartTimer()
}

// func BenchmarkSymphony_Read_ZeroCopy(b *testing.B) {
// 	timings := make([]int64, 0, b.N)
// 	traceSize := len(traceEntries)
// 	b.ResetTimer()
// 	b.ReportAllocs()
// 	for i := 0; i < b.N; i++ {
// 		idx := i % traceSize
// 		entry := traceEntries[idx]
// 		in := symphonyBufs[idx]

// 		start := time.Now()
// 		if entry.Op == "SET" {
// 			// Convert buffer to Raw type (zero-copy)
// 			raw := kv_proto.SetRequestRaw(in)
// 			_ = raw.GetKey()
// 			_ = raw.GetValue()
// 		} else {
// 			// Convert buffer to Raw type (zero-copy)
// 			raw := kv_proto.GetRequestRaw(in)
// 			_ = raw.GetKey()
// 		}
// 		elapsed := time.Since(start)
// 		timings = append(timings, elapsed.Nanoseconds())
// 	}
// 	b.StopTimer()
// 	if b.N > 0 {
// 		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
// 		msgPerSec := 1e9 / nsPerOp
// 		b.ReportMetric(msgPerSec, "msg/s")
// 	}
// 	if err := writeTimings("symphony_read_zerocopy_times.txt", timings); err != nil {
// 		b.Logf("Failed to write timing data: %v", err)
// 	}
// 	b.StartTimer()
// }
