package ack

import (
	"fmt"
	"net"
	"time"

	"github.com/appnet-org/arpc/pkg/logging"
	"go.uber.org/zap"
)

// ACKLoggerHandler logs ACK packets for debugging
type ACKLoggerHandler struct{}

func (h *ACKLoggerHandler) OnReceive(pkt any, addr *net.UDPAddr) error {
	ack, ok := pkt.(*ACKPacket)
	if !ok {
		return fmt.Errorf("expected ACK packet, got %T", pkt)
	}

	logging.Info("Received ACK",
		zap.Uint64("rpcID", ack.RPCID),
		zap.String("addr", addr.String()),
		zap.Uint8("status", ack.Status),
		zap.String("message", ack.Message))
	return nil
}

func (h *ACKLoggerHandler) OnSend(pkt any, addr *net.UDPAddr) error {
	ack, ok := pkt.(*ACKPacket)
	if !ok {
		return fmt.Errorf("expected ACK packet, got %T", pkt)
	}

	logging.Info("Sending ACK",
		zap.Uint64("rpcID", ack.RPCID),
		zap.String("addr", addr.String()),
		zap.Uint8("status", ack.Status),
		zap.String("message", ack.Message))
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
		logging.Info("RPC completed successfully", zap.Uint64("rpcID", ack.RPCID))
	case 1:
		logging.Warn("RPC failed", zap.Uint64("rpcID", ack.RPCID), zap.String("message", ack.Message))
	default:
		logging.Warn("RPC has unknown status", zap.Uint64("rpcID", ack.RPCID), zap.Uint8("status", ack.Status))
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
