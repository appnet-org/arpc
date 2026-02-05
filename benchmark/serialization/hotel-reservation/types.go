package main

import (
	"reflect"

	"google.golang.org/protobuf/proto"
)

// PayloadEntry represents a loaded payload with its type information
type PayloadEntry struct {
	TypeName string
	Message  proto.Message
	MsgType  reflect.Type // Pre-computed reflect.Type for faster instance creation
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
