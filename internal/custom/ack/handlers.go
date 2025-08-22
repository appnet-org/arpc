package ack

import (
	"fmt"
	"log"
	"net"
	"time"
)

// ACKLoggerHandler logs ACK packets for debugging
type ACKLoggerHandler struct{}

func (h *ACKLoggerHandler) OnReceive(pkt any, addr *net.UDPAddr) error {
	ack, ok := pkt.(*ACKPacket)
	if !ok {
		return fmt.Errorf("expected ACK packet, got %T", pkt)
	}

	log.Printf("Received ACK for RPC %d from %s: status=%d, message='%s'",
		ack.RPCID, addr.String(), ack.Status, ack.Message)
	return nil
}

func (h *ACKLoggerHandler) OnSend(pkt any, addr *net.UDPAddr) error {
	ack, ok := pkt.(*ACKPacket)
	if !ok {
		return fmt.Errorf("expected ACK packet, got %T", pkt)
	}

	log.Printf("Sending ACK for RPC %d to %s: status=%d, message='%s'",
		ack.RPCID, addr.String(), ack.Status, ack.Message)
	return nil
}

// ACKProcessorHandler processes ACK packets and updates application state
type ACKProcessorHandler struct {
	// You can add application-specific state here
	processedACKs map[uint64]bool
}

func NewACKProcessorHandler() *ACKProcessorHandler {
	return &ACKProcessorHandler{
		processedACKs: make(map[uint64]bool),
	}
}

func (h *ACKProcessorHandler) OnReceive(pkt any, addr *net.UDPAddr) error {
	ack, ok := pkt.(*ACKPacket)
	if !ok {
		return fmt.Errorf("expected ACK packet, got %T", pkt)
	}

	// Mark this RPC as acknowledged
	h.processedACKs[ack.RPCID] = true

	// Process based on status
	switch ack.Status {
	case 0:
		log.Printf("RPC %d completed successfully", ack.RPCID)
	case 1:
		log.Printf("RPC %d failed: %s", ack.RPCID, ack.Message)
	default:
		log.Printf("RPC %d has unknown status %d", ack.RPCID, ack.Status)
	}

	return nil
}

func (h *ACKProcessorHandler) OnSend(pkt any, addr *net.UDPAddr) error {
	ack, ok := pkt.(*ACKPacket)
	if !ok {
		return fmt.Errorf("expected ACK packet, got %T", pkt)
	}

	// Set timestamp when sending
	ack.Timestamp = time.Now().UnixNano()
	return nil
}
