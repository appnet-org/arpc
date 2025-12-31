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
	protoBufs [][]byte
	flatBufs  [][]byte
	capnpBufs [][]byte
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

func BenchmarkProtobuf_Write(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	traceSize := len(traceEntries)
	for i := 0; i < b.N; i++ {
		idx := i % traceSize
		entry := traceEntries[idx]
		item := traceData[idx]

		if entry.Op == "SET" {
			obj := &kv_proto.SetRequest{Key: item.Key, Value: item.Value}
			_, _ = proto.Marshal(obj)
		} else {
			obj := &kv_proto.GetRequest{Key: item.Key}
			_, _ = proto.Marshal(obj)
		}
	}
	b.StopTimer()
	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}
	b.StartTimer()
}

func BenchmarkProtobuf_Read(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	traceSize := len(traceEntries)
	for i := 0; i < b.N; i++ {
		idx := i % traceSize
		entry := traceEntries[idx]
		in := protoBufs[idx]

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
	}
	b.StopTimer()
	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}
	b.StartTimer()
}

func BenchmarkFlatBuffers_Write(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	builder := flatbuffers.NewBuilder(2048)
	traceSize := len(traceEntries)
	for i := 0; i < b.N; i++ {
		builder.Reset()
		idx := i % traceSize
		entry := traceEntries[idx]
		item := traceData[idx]

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
	}
	b.StopTimer()
	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}
	b.StartTimer()
}

func BenchmarkFlatBuffers_Read(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	traceSize := len(traceEntries)
	for i := 0; i < b.N; i++ {
		idx := i % traceSize
		entry := traceEntries[idx]
		in := flatBufs[idx]

		if entry.Op == "SET" {
			obj := kv_flat.GetRootAsSetRequest(in, 0)
			_ = obj.Key()
			_ = obj.Value()
		} else {
			obj := kv_flat.GetRootAsGetRequest(in, 0)
			_ = obj.Key()
		}
	}
	b.StopTimer()
	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}
	b.StartTimer()
}

func BenchmarkCapnp_Write(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	traceSize := len(traceEntries)

	// Create a reusable arena
	arenaBuf := make([]byte, 4096)

	for i := 0; i < b.N; i++ {
		idx := i % traceSize
		entry := traceEntries[idx]
		item := traceData[idx]

		// Reuse the arena
		msg, seg, _ := capnp.NewMessage(capnp.SingleSegment(arenaBuf[:0]))

		if entry.Op == "SET" {
			req, _ := kv_capnp.NewRootSetRequest(seg)
			req.SetKey([]byte(item.Key))
			req.SetValue([]byte(item.Value))
		} else {
			req, _ := kv_capnp.NewRootGetRequest(seg)
			req.SetKey([]byte(item.Key))
		}
		_, _ = msg.Marshal()
	}
	b.StopTimer()
	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}
	b.StartTimer()
}

func BenchmarkCapnp_Read(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	traceSize := len(traceEntries)
	for i := 0; i < b.N; i++ {
		idx := i % traceSize
		entry := traceEntries[idx]
		in := capnpBufs[idx]

		if entry.Op == "SET" {
			msg, _ := capnp.Unmarshal(in)
			obj, _ := kv_capnp.ReadRootSetRequest(msg)
			k, _ := obj.Key()
			v, _ := obj.Value()
			_, _ = k, v
		} else {
			msg, _ := capnp.Unmarshal(in)
			obj, _ := kv_capnp.ReadRootGetRequest(msg)
			k, _ := obj.Key()
			_ = k
		}
	}
	b.StopTimer()
	if b.N > 0 {
		nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
		msgPerSec := 1e9 / nsPerOp
		b.ReportMetric(msgPerSec, "msg/s")
	}
	b.StartTimer()
}
