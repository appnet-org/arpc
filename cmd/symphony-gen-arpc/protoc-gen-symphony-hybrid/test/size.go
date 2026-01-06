package main

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"capnproto.org/go/capnp/v3"
	flatbuffers "github.com/google/flatbuffers/go"
	"google.golang.org/protobuf/proto"

	// NOTE: Protobuf, Symphony, and Hybrid Symphony all generate code in the same money package
	// FlatBuffers and Cap'n Proto need separate schemas and generate in different packages
	money "github.com/appnet-org/arpc/cmd/symphony-gen-arpc/protoc-gen-symphony-hybrid/test/money"
	cp "github.com/appnet-org/arpc/cmd/symphony-gen-arpc/protoc-gen-symphony-hybrid/test/money/capnp"
	fb "github.com/appnet-org/arpc/cmd/symphony-gen-arpc/protoc-gen-symphony-hybrid/test/money/flatbuffers"
)

var (
	currencyCodes = []string{"USD", "EUR", "GBP", "JPY", "CNY", "AUD", "CAD", "CHF", "HKD", "NZD"}
)

type SizeResult struct {
	Format string
	Size   int
	Error  error
}

func main() {
	// Seed random number generator
	rand.Seed(time.Now().UnixNano())

	// Generate random test data
	testData := generateTestData()

	fmt.Println("Serialization Size Comparison")
	fmt.Println("==============================")
	fmt.Println()
	fmt.Printf("Test Data:\n")
	fmt.Printf("  Currency Code: %s\n", testData.CurrencyCode)
	fmt.Printf("  Units: %d\n", testData.Units)
	fmt.Printf("  Nanos: %d\n", testData.Nanos)
	fmt.Printf("  To Code: %s\n", testData.ToCode)
	fmt.Printf("  User ID: %s\n", testData.UserID)
	fmt.Println()

	// Test each format
	results := []SizeResult{
		testProtobuf(testData),
		testSymphony(testData),
		testHybridSymphony(testData),
		testFlatBuffers(testData),
		testCapnProto(testData),
	}

	// Print results
	printResults(results)
}

type TestData struct {
	CurrencyCode string
	Units        int64
	Nanos        int32
	ToCode       string
	UserID       string
}

func generateTestData() TestData {
	// Generate random currency code
	fromCode := currencyCodes[rand.Intn(len(currencyCodes))]
	toCode := currencyCodes[rand.Intn(len(currencyCodes))]
	for toCode == fromCode {
		toCode = currencyCodes[rand.Intn(len(currencyCodes))]
	}

	// Generate random units (-1000000 to 1000000)
	units := int64(rand.Intn(2000001) - 1000000)

	// Generate random nanos (-999999999 to 999999999)
	nanos := int32(rand.Intn(1999999999) - 999999999)

	// Generate random user ID
	userID := fmt.Sprintf("user_%d_%d", rand.Intn(10000), time.Now().Unix())

	return TestData{
		CurrencyCode: fromCode,
		Units:        units,
		Nanos:        nanos,
		ToCode:       toCode,
		UserID:       userID,
	}
}

func testProtobuf(data TestData) SizeResult {
	// Create Money message
	m := &money.Money{
		CurrencyCode: data.CurrencyCode,
		Units:        data.Units,
		Nanos:        data.Nanos,
	}

	// Create CurrencyConversionRequest
	req := &money.CurrencyConversionRequest{
		From:   m,
		ToCode: data.ToCode,
		UserId: data.UserID,
	}

	// Marshal
	bytes, err := proto.Marshal(req)
	if err != nil {
		return SizeResult{Format: "Protobuf", Size: 0, Error: err}
	}

	return SizeResult{Format: "Protobuf", Size: len(bytes), Error: nil}
}

func testSymphony(data TestData) SizeResult {
	// Create Money message
	m := &money.Money{
		CurrencyCode: data.CurrencyCode,
		Units:        data.Units,
		Nanos:        data.Nanos,
	}

	// Create CurrencyConversionRequest
	req := &money.CurrencyConversionRequest{
		From:   m,
		ToCode: data.ToCode,
		UserId: data.UserID,
	}

	// Marshal
	bytes, err := req.MarshalSymphony()
	if err != nil {
		return SizeResult{Format: "Symphony", Size: 0, Error: err}
	}

	return SizeResult{Format: "Symphony", Size: len(bytes), Error: nil}
}

func testHybridSymphony(data TestData) SizeResult {
	// Create Money message
	m := &money.Money{
		CurrencyCode: data.CurrencyCode,
		Units:        data.Units,
		Nanos:        data.Nanos,
	}

	// Create CurrencyConversionRequest
	req := &money.CurrencyConversionRequest{
		From:   m,
		ToCode: data.ToCode,
		UserId: data.UserID,
	}

	// Marshal
	bytes, err := req.MarshalSymphonyHybrid()
	if err != nil {
		return SizeResult{Format: "Hybrid Symphony", Size: 0, Error: err}
	}

	return SizeResult{Format: "Hybrid Symphony", Size: len(bytes), Error: nil}
}

func testFlatBuffers(data TestData) SizeResult {
	builder := flatbuffers.NewBuilder(0)

	// Create Money
	currencyCodeOffset := builder.CreateString(data.CurrencyCode)
	fb.MoneyStart(builder)
	fb.MoneyAddCurrencyCode(builder, currencyCodeOffset)
	fb.MoneyAddUnits(builder, data.Units)
	fb.MoneyAddNanos(builder, data.Nanos)
	moneyOffset := fb.MoneyEnd(builder)

	// Create CurrencyConversionRequest
	toCodeOffset := builder.CreateString(data.ToCode)
	userIDOffset := builder.CreateString(data.UserID)
	fb.CurrencyConversionRequestStart(builder)
	fb.CurrencyConversionRequestAddFrom(builder, moneyOffset)
	fb.CurrencyConversionRequestAddToCode(builder, toCodeOffset)
	fb.CurrencyConversionRequestAddUserId(builder, userIDOffset)
	reqOffset := fb.CurrencyConversionRequestEnd(builder)

	builder.Finish(reqOffset)
	bytes := builder.FinishedBytes()

	return SizeResult{Format: "FlatBuffers", Size: len(bytes), Error: nil}
}

func testCapnProto(data TestData) SizeResult {
	msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return SizeResult{Format: "Cap'n Proto", Size: 0, Error: err}
	}

	// Create Money
	money, err := cp.NewRootMoney(seg)
	if err != nil {
		return SizeResult{Format: "Cap'n Proto", Size: 0, Error: err}
	}
	money.SetCurrencyCode(data.CurrencyCode)
	money.SetUnits(data.Units)
	money.SetNanos(data.Nanos)

	// Create CurrencyConversionRequest
	req, err := cp.NewRootCurrencyConversionRequest(seg)
	if err != nil {
		return SizeResult{Format: "Cap'n Proto", Size: 0, Error: err}
	}
	req.SetFrom(money)
	req.SetToCode(data.ToCode)
	req.SetUserId(data.UserID)

	// Marshal
	bytes, err := msg.Marshal()
	if err != nil {
		return SizeResult{Format: "Cap'n Proto", Size: 0, Error: err}
	}

	return SizeResult{Format: "Cap'n Proto", Size: len(bytes), Error: nil}
}

func printResults(results []SizeResult) {
	// Find minimum size
	minSize := -1
	for _, r := range results {
		if r.Error == nil && (minSize == -1 || r.Size < minSize) {
			minSize = r.Size
		}
	}

	if minSize == -1 {
		fmt.Println("Error: All formats failed to serialize")
		os.Exit(1)
	}

	fmt.Println("Results:")
	fmt.Println("--------")
	fmt.Printf("%-20s %10s %15s\n", "Format", "Size (bytes)", "vs Smallest")
	fmt.Println(strings.Repeat("-", 50))

	for _, r := range results {
		if r.Error != nil {
			fmt.Printf("%-20s %10s %15s\n", r.Format, "ERROR", r.Error.Error())
		} else {
			diff := float64(r.Size-minSize) / float64(minSize) * 100
			fmt.Printf("%-20s %10d %14.1f%%\n", r.Format, r.Size, diff)
		}
	}
	fmt.Println()
}

