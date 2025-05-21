package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"log"

	"capnproto.org/go/capnp/v3"
	echo_capnp "github.com/appnet-org/arpc/examples/echo_capnp/capnp"
)

func main() {
	// Create a new Cap’n Proto message with a single segment
	msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		log.Fatalf("failed to create message: %v", err)
	}

	// Construct EchoRequest
	req, err := echo_capnp.NewRootEchoRequest(seg)
	if err != nil {
		log.Fatalf("failed to create EchoRequest: %v", err)
	}

	// Set fields
	req.SetId(1)
	req.SetScore(10.0)
	if err := req.SetContent("hello"); err != nil {
		log.Fatalf("failed to set content: %v", err)
	}

	// Serialize the message to a buffer
	var buf bytes.Buffer
	if err := capnp.NewEncoder(&buf).Encode(msg); err != nil {
		log.Fatalf("failed to encode message: %v", err)
	}

	// Print hex dump of serialized message
	fmt.Printf("Serialized EchoRequest (hex): %s\n", hex.EncodeToString(buf.Bytes()))
}
