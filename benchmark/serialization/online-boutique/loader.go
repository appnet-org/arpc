package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	onlineboutique "github.com/appnet-org/arpc/benchmark/serialization/online-boutique/proto"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// loadAllPayloads loads all JSONL files from the payloads directory
func loadAllPayloads() error {
	// Try to find payloads directory
	var payloadsDir string
	possiblePaths := []string{
		filepath.Join("benchmark", "serialization", "online-boutique", "payloads"),
		filepath.Join("payloads"),
		filepath.Join("..", "payloads"),
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			payloadsDir = path
			break
		}
	}

	if payloadsDir == "" {
		return fmt.Errorf("failed to find payloads directory. Tried: %v", possiblePaths)
	}

	// Get all JSONL files
	files, err := filepath.Glob(filepath.Join(payloadsDir, "*.jsonl"))
	if err != nil {
		return fmt.Errorf("failed to glob payload files: %w", err)
	}

	if len(files) == 0 {
		return fmt.Errorf("no JSONL files found in %s", payloadsDir)
	}

	// Load each file
	for _, file := range files {
		typeName := filepath.Base(file)
		typeName = typeName[:len(typeName)-6] // Remove ".jsonl" extension

		entries, err := loadPayloadFile(file, typeName)
		if err != nil {
			return fmt.Errorf("failed to load %s: %w", file, err)
		}

		payloadEntries = append(payloadEntries, entries...)
	}

	return nil
}

// loadPayloadFile loads a single JSONL file and parses each line as a message
func loadPayloadFile(filename, typeName string) ([]PayloadEntry, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var entries []PayloadEntry
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// Parse JSON
		var jsonData map[string]interface{}
		if err := json.Unmarshal(line, &jsonData); err != nil {
			fmt.Printf("Warning: failed to parse line %d in %s: %v\n", lineNum, filename, err)
			continue
		}

		// Convert JSON to proto message
		msg, err := jsonToProtoMessage(typeName, jsonData)
		if err != nil {
			fmt.Printf("Warning: failed to convert line %d in %s: %v\n", lineNum, filename, err)
			continue
		}

		entries = append(entries, PayloadEntry{
			TypeName: typeName,
			Message:  msg,
			MsgType:  getMessageType(typeName),
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

// jsonToProtoMessage converts a JSON map to the appropriate proto message type
func jsonToProtoMessage(typeName string, jsonData map[string]interface{}) (proto.Message, error) {
	// Convert JSON to proto message using JSON unmarshaling
	// We'll use protojson for this, but since we don't have it, we'll use a workaround:
	// Marshal JSON back to bytes and use proto's JSON unmarshaling
	jsonBytes, err := json.Marshal(jsonData)
	if err != nil {
		return nil, err
	}

	// Get the message type by name
	msgType := getMessageType(typeName)
	if msgType == nil {
		return nil, fmt.Errorf("unknown message type: %s", typeName)
	}

	// Create a new instance of the message type
	msg := reflect.New(msgType.Elem()).Interface().(proto.Message)

	// Unmarshal JSON into proto message
	// Note: We'll use protojson if available, otherwise we need to manually convert
	// For now, let's use a simpler approach with protojson
	err = protojsonUnmarshal(jsonBytes, msg)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON to %s: %w", typeName, err)
	}

	return msg, nil
}

// protojsonUnmarshal unmarshals JSON bytes into a proto message using protojson
func protojsonUnmarshal(data []byte, msg proto.Message) error {
	unmarshaler := &protojson.UnmarshalOptions{
		DiscardUnknown: true,
	}
	return unmarshaler.Unmarshal(data, msg)
}

// getMessageType returns the reflect.Type for a message type by name
func getMessageType(typeName string) reflect.Type {
	// Map type names to proto message types
	typeMap := map[string]reflect.Type{
		"Ad":                             reflect.TypeOf((*onlineboutique.Ad)(nil)),
		"AddItemRequest":                 reflect.TypeOf((*onlineboutique.AddItemRequest)(nil)),
		"Address":                        reflect.TypeOf((*onlineboutique.Address)(nil)),
		"AdRequest":                      reflect.TypeOf((*onlineboutique.AdRequest)(nil)),
		"AdResponse":                     reflect.TypeOf((*onlineboutique.AdResponse)(nil)),
		"Cart":                           reflect.TypeOf((*onlineboutique.Cart)(nil)),
		"CartItem":                       reflect.TypeOf((*onlineboutique.CartItem)(nil)),
		"ChargeRequest":                  reflect.TypeOf((*onlineboutique.ChargeRequest)(nil)),
		"ChargeResponse":                 reflect.TypeOf((*onlineboutique.ChargeResponse)(nil)),
		"CreditCardInfo":                 reflect.TypeOf((*onlineboutique.CreditCardInfo)(nil)),
		"CurrencyConversionRequest":      reflect.TypeOf((*onlineboutique.CurrencyConversionRequest)(nil)),
		"Empty":                          reflect.TypeOf((*onlineboutique.Empty)(nil)),
		"EmptyCartRequest":               reflect.TypeOf((*onlineboutique.EmptyCartRequest)(nil)),
		"EmptyUser":                      reflect.TypeOf((*onlineboutique.EmptyUser)(nil)),
		"GetCartRequest":                 reflect.TypeOf((*onlineboutique.GetCartRequest)(nil)),
		"GetProductRequest":              reflect.TypeOf((*onlineboutique.GetProductRequest)(nil)),
		"GetQuoteRequest":                reflect.TypeOf((*onlineboutique.GetQuoteRequest)(nil)),
		"GetQuoteResponse":               reflect.TypeOf((*onlineboutique.GetQuoteResponse)(nil)),
		"GetSupportedCurrenciesResponse": reflect.TypeOf((*onlineboutique.GetSupportedCurrenciesResponse)(nil)),
		"ListProductsResponse":           reflect.TypeOf((*onlineboutique.ListProductsResponse)(nil)),
		"ListRecommendationsRequest":     reflect.TypeOf((*onlineboutique.ListRecommendationsRequest)(nil)),
		"ListRecommendationsResponse":    reflect.TypeOf((*onlineboutique.ListRecommendationsResponse)(nil)),
		"Money":                          reflect.TypeOf((*onlineboutique.Money)(nil)),
		"OrderItem":                      reflect.TypeOf((*onlineboutique.OrderItem)(nil)),
		"OrderResult":                    reflect.TypeOf((*onlineboutique.OrderResult)(nil)),
		"PlaceOrderRequest":              reflect.TypeOf((*onlineboutique.PlaceOrderRequest)(nil)),
		"PlaceOrderResponse":             reflect.TypeOf((*onlineboutique.PlaceOrderResponse)(nil)),
		"Product":                        reflect.TypeOf((*onlineboutique.Product)(nil)),
		"SearchProductsRequest":          reflect.TypeOf((*onlineboutique.SearchProductsRequest)(nil)),
		"SearchProductsResponse":         reflect.TypeOf((*onlineboutique.SearchProductsResponse)(nil)),
		"SendOrderConfirmationRequest":   reflect.TypeOf((*onlineboutique.SendOrderConfirmationRequest)(nil)),
		"ShipOrderRequest":               reflect.TypeOf((*onlineboutique.ShipOrderRequest)(nil)),
		"ShipOrderResponse":              reflect.TypeOf((*onlineboutique.ShipOrderResponse)(nil)),
	}

	return typeMap[typeName]
}
