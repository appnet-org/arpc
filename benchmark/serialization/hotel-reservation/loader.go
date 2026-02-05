package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	hotel_reservation "github.com/appnet-org/arpc/benchmark/serialization/hotel-reservation/proto"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// loadAllPayloads loads all JSONL files from the payloads directory
func loadAllPayloads() error {
	// Try to find payloads directory
	var payloadsDir string
	possiblePaths := []string{
		filepath.Join("benchmark", "serialization", "hotel-reservation", "payloads"),
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
	jsonBytes, err := json.Marshal(jsonData)
	if err != nil {
		return nil, err
	}

	msgType := getMessageType(typeName)
	if msgType == nil {
		return nil, fmt.Errorf("unknown message type: %s", typeName)
	}

	msg := reflect.New(msgType.Elem()).Interface().(proto.Message)

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
	typeMap := map[string]reflect.Type{
		"NearbyRequest":             reflect.TypeOf((*hotel_reservation.NearbyRequest)(nil)),
		"NearbyResult":              reflect.TypeOf((*hotel_reservation.NearbyResult)(nil)),
		"GetProfilesRequest":        reflect.TypeOf((*hotel_reservation.GetProfilesRequest)(nil)),
		"GetProfilesResult":         reflect.TypeOf((*hotel_reservation.GetProfilesResult)(nil)),
		"Hotel":                     reflect.TypeOf((*hotel_reservation.Hotel)(nil)),
		"Address":                   reflect.TypeOf((*hotel_reservation.Address)(nil)),
		"Image":                     reflect.TypeOf((*hotel_reservation.Image)(nil)),
		"GetRecommendationsRequest": reflect.TypeOf((*hotel_reservation.GetRecommendationsRequest)(nil)),
		"GetRecommendationsResult":  reflect.TypeOf((*hotel_reservation.GetRecommendationsResult)(nil)),
		"GetRatesRequest":           reflect.TypeOf((*hotel_reservation.GetRatesRequest)(nil)),
		"GetRatesResult":            reflect.TypeOf((*hotel_reservation.GetRatesResult)(nil)),
		"RatePlan":                  reflect.TypeOf((*hotel_reservation.RatePlan)(nil)),
		"RoomType":                  reflect.TypeOf((*hotel_reservation.RoomType)(nil)),
		"ReservationRequest":        reflect.TypeOf((*hotel_reservation.ReservationRequest)(nil)),
		"ReservationResult":         reflect.TypeOf((*hotel_reservation.ReservationResult)(nil)),
		"SearchRequest":             reflect.TypeOf((*hotel_reservation.SearchRequest)(nil)),
		"SearchResult":              reflect.TypeOf((*hotel_reservation.SearchResult)(nil)),
		"CheckUserRequest":          reflect.TypeOf((*hotel_reservation.CheckUserRequest)(nil)),
		"CheckUserResult":           reflect.TypeOf((*hotel_reservation.CheckUserResult)(nil)),
	}

	return typeMap[typeName]
}
