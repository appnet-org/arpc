package main

import (
	"google.golang.org/protobuf/proto"
)

// PayloadEntry represents a loaded payload with its type information
type PayloadEntry struct {
	TypeName string
	Message  proto.Message
}

var (
	payloadEntries []PayloadEntry

	// Pre-serialized buffers for Read/Deserialize benchmarks
	protoBufs    [][]byte
	flatBufs     [][]byte
	capnpBufs    [][]byte
	symphonyBufs [][]byte
	hybridBufs   [][]byte
)
