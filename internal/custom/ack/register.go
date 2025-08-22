//go:build !exclude_ack
// +build !exclude_ack

package ack

import (
	"fmt"

	"github.com/appnet-org/arpc/internal/packet"
)

func init() {
	// Register the ACK packet type with the default registry
	_, err := packet.DefaultRegistry.RegisterPacketType("Acknowledgement", &ACKPacketCodec{})
	if err != nil {
		panic(fmt.Sprintf("Failed to register ACK packet type: %v", err))
	}
}
