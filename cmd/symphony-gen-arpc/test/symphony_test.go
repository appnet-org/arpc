package Test

import (
	"bytes"
	"reflect"
	"testing"
)

func TestTestRequest_MarshalSymphony_WithRepeatedFields(t *testing.T) {
	tests := []struct {
		name    string
		request *TestRequest
		wantErr bool
	}{
		{
			name: "empty repeated field",
			request: &TestRequest{
				Id:       123,
				Score:    456,
				Username: "testuser",
				Content:  []string{},
				Numbers:  []int32{},
			},
			wantErr: false,
		},
		{
			name: "single item in repeated field",
			request: &TestRequest{
				Id:       789,
				Score:    101,
				Username: "singleuser",
				Content:  []string{"hello"},
				Numbers:  []int32{42},
			},
			wantErr: false,
		},
		{
			name: "multiple items in repeated field",
			request: &TestRequest{
				Id:       999,
				Score:    888,
				Username: "multiuser",
				Content:  []string{"hello", "world", "test"},
				Numbers:  []int32{1, 2, 3, 4, 5},
			},
			wantErr: false,
		},
		{
			name: "empty strings in repeated field",
			request: &TestRequest{
				Id:       111,
				Score:    222,
				Username: "emptyuser",
				Content:  []string{"", "notempty", ""},
				Numbers:  []int32{0, 100, 0},
			},
			wantErr: false,
		},
		{
			name: "long strings in repeated field",
			request: &TestRequest{
				Id:       333,
				Score:    444,
				Username: "longuser",
				Content:  []string{"very long string that should test the serialization", "another long string", "short"},
				Numbers:  []int32{999, 888, 777},
			},
			wantErr: false,
		},
		{
			name: "repeated ints only",
			request: &TestRequest{
				Id:       555,
				Score:    666,
				Username: "intuser",
				Content:  []string{},
				Numbers:  []int32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			},
			wantErr: false,
		},
		{
			name: "negative ints in repeated field",
			request: &TestRequest{
				Id:       777,
				Score:    888,
				Username: "neguser",
				Content:  []string{"test"},
				Numbers:  []int32{-1, -100, -1000, 0, 100, 1000},
			},
			wantErr: false,
		},
		{
			name: "large ints in repeated field",
			request: &TestRequest{
				Id:       999,
				Score:    111,
				Username: "largeuser",
				Content:  []string{"large"},
				Numbers:  []int32{2147483647, -2147483648, 0, 1000000, -1000000},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal
			data, err := tt.request.MarshalSymphony()
			if (err != nil) != tt.wantErr {
				t.Errorf("TestRequest.MarshalSymphony() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			// Unmarshal to a new instance
			unmarshaled := &TestRequest{}
			if err := unmarshaled.UnmarshalSymphony(data); err != nil {
				t.Errorf("TestRequest.UnmarshalSymphony() error = %v", err)
				return
			}

			// Verify all fields match
			if unmarshaled.Id != tt.request.Id {
				t.Errorf("Id mismatch: got %v, want %v", unmarshaled.Id, tt.request.Id)
			}
			if unmarshaled.Score != tt.request.Score {
				t.Errorf("Score mismatch: got %v, want %v", unmarshaled.Score, tt.request.Score)
			}
			if unmarshaled.Username != tt.request.Username {
				t.Errorf("Username mismatch: got %v, want %v", unmarshaled.Username, tt.request.Username)
			}
			if !reflect.DeepEqual(unmarshaled.Content, tt.request.Content) {
				t.Errorf("Content mismatch: got %v, want %v", unmarshaled.Content, tt.request.Content)
			}
			if !reflect.DeepEqual(unmarshaled.Numbers, tt.request.Numbers) {
				t.Errorf("Numbers mismatch: got %v, want %v", unmarshaled.Numbers, tt.request.Numbers)
			}
		})
	}
}

func TestTestRequest_MarshalSymphony_Consistency(t *testing.T) {
	request := &TestRequest{
		Id:       12345,
		Score:    67890,
		Username: "consistency_test",
		Content:  []string{"item1", "item2", "item3", "item4"},
	}

	// Marshal multiple times
	data1, err := request.MarshalSymphony()
	if err != nil {
		t.Fatalf("First MarshalSymphony failed: %v", err)
	}

	data2, err := request.MarshalSymphony()
	if err != nil {
		t.Fatalf("Second MarshalSymphony failed: %v", err)
	}

	// Verify consistency
	if !bytes.Equal(data1, data2) {
		t.Error("MarshalSymphony is not consistent - same input produced different output")
	}
}

func TestTestRequest_UnmarshalSymphony_InvalidData(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name:    "empty data",
			data:    []byte{},
			wantErr: true,
		},
		{
			name:    "incomplete header",
			data:    []byte{0x00},
			wantErr: true,
		},
		{
			name:    "incomplete field order",
			data:    []byte{0x00, 1, 2},
			wantErr: true,
		},
		{
			name:    "truncated offset table",
			data:    []byte{0x00, 1, 2, 3, 4, 1, 0, 0},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &TestRequest{}
			err := request.UnmarshalSymphony(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("TestRequest.UnmarshalSymphony() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTestRequest_UnmarshalSymphony_EmptyRepeatedField(t *testing.T) {
	// Create a request with empty repeated fields
	original := &TestRequest{
		Id:       1,
		Score:    2,
		Username: "empty",
		Content:  []string{},
		Numbers:  []int32{},
	}

	// Marshal it
	data, err := original.MarshalSymphony()
	if err != nil {
		t.Fatalf("MarshalSymphony failed: %v", err)
	}

	// Unmarshal it
	unmarshaled := &TestRequest{}
	if err := unmarshaled.UnmarshalSymphony(data); err != nil {
		t.Fatalf("UnmarshalSymphony failed: %v", err)
	}

	// Verify the empty repeated fields are handled correctly
	if unmarshaled.Content == nil {
		t.Error("Content should not be nil after unmarshaling empty repeated field")
	}
	if len(unmarshaled.Content) != 0 {
		t.Errorf("Content length should be 0, got %d", len(unmarshaled.Content))
	}
	if unmarshaled.Numbers == nil {
		t.Error("Numbers should not be nil after unmarshaling empty repeated field")
	}
	if len(unmarshaled.Numbers) != 0 {
		t.Errorf("Numbers length should be 0, got %d", len(unmarshaled.Numbers))
	}
}

func TestTestRequest_UnmarshalSymphony_NilRepeatedField(t *testing.T) {
	// Create a request with nil repeated fields
	original := &TestRequest{
		Id:       1,
		Score:    2,
		Username: "nil",
		Content:  nil,
		Numbers:  nil,
	}

	// Marshal it
	data, err := original.MarshalSymphony()
	if err != nil {
		t.Fatalf("MarshalSymphony failed: %v", err)
	}

	// Unmarshal it
	unmarshaled := &TestRequest{}
	if err := unmarshaled.UnmarshalSymphony(data); err != nil {
		t.Fatalf("UnmarshalSymphony failed: %v", err)
	}

	// Verify the nil repeated fields are handled correctly
	if unmarshaled.Content == nil {
		t.Error("Content should not be nil after unmarshaling nil repeated field")
	}
	if len(unmarshaled.Content) != 0 {
		t.Errorf("Content length should be 0, got %d", len(unmarshaled.Content))
	}
	if unmarshaled.Numbers == nil {
		t.Error("Numbers should not be nil after unmarshaling nil repeated field")
	}
	if len(unmarshaled.Numbers) != 0 {
		t.Errorf("Numbers length should be 0, got %d", len(unmarshaled.Numbers))
	}
}

func TestTestResponse_MarshalSymphony(t *testing.T) {
	response := &TestResponse{
		Resp: "test response",
	}

	data, err := response.MarshalSymphony()
	if err != nil {
		t.Fatalf("TestResponse.MarshalSymphony() error = %v", err)
	}

	// Unmarshal to verify
	unmarshaled := &TestResponse{}
	if err := unmarshaled.UnmarshalSymphony(data); err != nil {
		t.Fatalf("TestResponse.UnmarshalSymphony() error = %v", err)
	}

	if unmarshaled.Resp != response.Resp {
		t.Errorf("Resp mismatch: got %v, want %v", unmarshaled.Resp, response.Resp)
	}
}

func TestTestRequest_RepeatedIntsField(t *testing.T) {
	tests := []struct {
		name    string
		numbers []int32
	}{
		{
			name:    "empty ints array",
			numbers: []int32{},
		},
		{
			name:    "single int",
			numbers: []int32{42},
		},
		{
			name:    "multiple positive ints",
			numbers: []int32{1, 2, 3, 4, 5},
		},
		{
			name:    "mixed positive and negative ints",
			numbers: []int32{-1, 0, 1, -100, 100},
		},
		{
			name:    "large ints",
			numbers: []int32{2147483647, -2147483648, 1000000, -1000000},
		},
		{
			name:    "repeated same values",
			numbers: []int32{0, 0, 0, 1, 1, 1},
		},
		{
			name:    "sequential ints",
			numbers: []int32{-5, -4, -3, -2, -1, 0, 1, 2, 3, 4, 5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &TestRequest{
				Id:       123,
				Score:    456,
				Username: "int_test_user",
				Content:  []string{"test"},
				Numbers:  tt.numbers,
			}

			// Marshal
			data, err := request.MarshalSymphony()
			if err != nil {
				t.Errorf("MarshalSymphony failed: %v", err)
				return
			}

			// Unmarshal
			unmarshaled := &TestRequest{}
			if err := unmarshaled.UnmarshalSymphony(data); err != nil {
				t.Errorf("UnmarshalSymphony failed: %v", err)
				return
			}

			// Verify numbers field matches exactly
			if !reflect.DeepEqual(unmarshaled.Numbers, tt.numbers) {
				t.Errorf("Numbers mismatch: got %v, want %v", unmarshaled.Numbers, tt.numbers)
			}

			// Verify other fields are preserved
			if unmarshaled.Id != request.Id {
				t.Errorf("Id mismatch: got %v, want %v", unmarshaled.Id, request.Id)
			}
			if unmarshaled.Score != request.Score {
				t.Errorf("Score mismatch: got %v, want %v", unmarshaled.Score, request.Score)
			}
			if unmarshaled.Username != request.Username {
				t.Errorf("Username mismatch: got %v, want %v", unmarshaled.Username, request.Username)
			}
			if !reflect.DeepEqual(unmarshaled.Content, request.Content) {
				t.Errorf("Content mismatch: got %v, want %v", unmarshaled.Content, request.Content)
			}
		})
	}
}

func TestTestRequest_RepeatedFieldEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		content []string
		numbers []int32
	}{
		{
			name:    "very long strings",
			content: []string{string(make([]byte, 1000)), string(make([]byte, 500))},
			numbers: []int32{1000, 500},
		},
		{
			name:    "unicode strings",
			content: []string{"Hello 世界", "Привет мир", "こんにちは世界"},
			numbers: []int32{1, 2, 3},
		},
		{
			name:    "special characters",
			content: []string{"\x00\x01\x02", "tab\tnewline\n", "quote\"backslash\\"},
			numbers: []int32{-1, 0, 1},
		},
		{
			name:    "mixed content",
			content: []string{"", "normal", "123", "!@#$%^&*()", ""},
			numbers: []int32{0, 1, 123, 0, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &TestRequest{
				Id:       999,
				Score:    888,
				Username: "edgecase",
				Content:  tt.content,
				Numbers:  tt.numbers,
			}

			// Marshal
			data, err := request.MarshalSymphony()
			if err != nil {
				t.Errorf("MarshalSymphony failed: %v", err)
				return
			}

			// Unmarshal
			unmarshaled := &TestRequest{}
			if err := unmarshaled.UnmarshalSymphony(data); err != nil {
				t.Errorf("UnmarshalSymphony failed: %v", err)
				return
			}

			// Verify content matches exactly
			if !reflect.DeepEqual(unmarshaled.Content, tt.content) {
				t.Errorf("Content mismatch: got %v, want %v", unmarshaled.Content, tt.content)
			}
			// Verify numbers matches exactly
			if !reflect.DeepEqual(unmarshaled.Numbers, tt.numbers) {
				t.Errorf("Numbers mismatch: got %v, want %v", unmarshaled.Numbers, tt.numbers)
			}
		})
	}
}

// Benchmark tests for performance
func BenchmarkTestRequest_MarshalSymphony(b *testing.B) {
	request := &TestRequest{
		Id:       123,
		Score:    456,
		Username: "benchmark_user",
		Content:  []string{"item1", "item2", "item3", "item4", "item5"},
		Numbers:  []int32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := request.MarshalSymphony()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTestRequest_UnmarshalSymphony(b *testing.B) {
	request := &TestRequest{
		Id:       123,
		Score:    456,
		Username: "benchmark_user",
		Content:  []string{"item1", "item2", "item3", "item4", "item5"},
		Numbers:  []int32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
	}

	data, err := request.MarshalSymphony()
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		unmarshaled := &TestRequest{}
		err := unmarshaled.UnmarshalSymphony(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}
