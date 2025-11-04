package reliable

import (
	"net"
	"time"

	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/packet"
	"github.com/appnet-org/arpc/pkg/transport"
	"go.uber.org/zap"
)

// ReliableClientHandler implements the client-side reliable transport logic
type ReliableClientHandler struct {
	*ReliableHandler // Embed base handler
}

// NewReliableClientHandler creates a new reliable client handler with default 30s timeout
func NewReliableClientHandler(transportSender TransportSender, timerMgr TimerScheduler) *ReliableClientHandler {
	return NewReliableClientHandlerWithTimeout(transportSender, timerMgr, 30*time.Second)
}

// NewReliableClientHandlerWithTimeout creates a new reliable client handler with custom timeout
func NewReliableClientHandlerWithTimeout(transportSender TransportSender, timerMgr TimerScheduler, timeout time.Duration) *ReliableClientHandler {
	handler := &ReliableClientHandler{
		ReliableHandler: newReliableHandler(transportSender, timerMgr, timeout),
	}
	handler.startCleanupTimer(transport.TimerKey("reliable_client_cleanup"))
	handler.startRetransmitTimer(transport.TimerKey("reliable_client_retransmit"), 1*time.Second)

	logging.Debug("Reliable client handler created",
		zap.Duration("timeout", timeout))

	return handler
}

// OnSend handles outgoing REQUEST packets and ACK(kind=1) packets
func (h *ReliableClientHandler) OnSend(pkt any, addr *net.UDPAddr) error {
	switch p := pkt.(type) {
	case *packet.DataPacket:
		if p.PacketTypeID == packet.PacketTypeRequest.TypeID {
			return h.handleSendRequest(p)
		}
	case *ACKPacket:
		if p.Kind == 1 { // ACK for RESPONSE
			return h.handleSendACK(p, addr)
		}
	}
	return nil
}

// OnReceive handles incoming RESPONSE and ACK(kind=0) packets
func (h *ReliableClientHandler) OnReceive(pkt any, addr *net.UDPAddr) error {
	switch p := pkt.(type) {
	case *packet.DataPacket:
		if p.PacketTypeID == packet.PacketTypeResponse.TypeID {
			return h.handleReceiveResponse(p, addr)
		}
	case *ACKPacket:
		if p.Kind == 0 { // ACK for REQUEST
			return h.handleReceiveACK(p, addr)
		}
	}
	return nil
}

// handleSendRequest tracks outgoing REQUEST packets
func (h *ReliableClientHandler) handleSendRequest(pkt *packet.DataPacket) error {
	// Extract server connection ID from destination
	connID := ConnectionID{IP: pkt.DstIP, Port: pkt.DstPort}
	key := connID.String()
	conn := h.getOrCreateConnection(key, connID)

	h.mu.Lock()
	defer h.mu.Unlock()

	// Serialize packet for buffering
	serializedData, err := h.serializeDataPacket(pkt)
	if err != nil {
		logging.Error("Failed to serialize REQUEST packet for buffering",
			zap.Uint64("rpcID", pkt.RPCID),
			zap.Error(err))
		return err
	}

	// Get packet type for retransmission
	packetType, exists := h.transport.GetPacketRegistry().GetPacketType(pkt.PacketTypeID)
	if !exists {
		logging.Error("Packet type not found in registry",
			zap.Uint8("packetTypeID", uint8(pkt.PacketTypeID)))
		return nil // Continue even if we can't buffer for retransmission
	}

	// Track this packet in TxMsg
	if _, exists := conn.TxMsg[pkt.RPCID]; !exists {
		conn.TxMsg[pkt.RPCID] = &MsgTx{
			Count:      uint32(pkt.TotalPackets),
			SendTs:     time.Now(),
			DstAddr:    key,
			PacketType: packetType,
			Segments:   make(map[uint16][]byte),
		}
	}

	// Buffer this segment
	conn.TxMsg[pkt.RPCID].Segments[pkt.SeqNumber] = serializedData

	logging.Debug("Client tracking sent REQUEST",
		zap.Uint64("rpcID", pkt.RPCID),
		zap.Uint16("seqNumber", pkt.SeqNumber),
		zap.Uint16("totalPackets", pkt.TotalPackets),
		zap.String("server", key))

	return nil
}

// handleSendACK handles sending ACK packets (already formed, just update activity)
func (h *ReliableClientHandler) handleSendACK(ack *ACKPacket, addr *net.UDPAddr) error {
	// Extract server connection ID from destination
	// Note: ACK packet doesn't have DstIP/Port, so we use addr
	var connID ConnectionID
	if ip4 := addr.IP.To4(); ip4 != nil {
		copy(connID.IP[:], ip4)
	}
	connID.Port = uint16(addr.Port)
	key := connID.String()

	// Just update activity timestamp
	h.getOrCreateConnection(key, connID)

	logging.Debug("Client sending ACK for RESPONSE",
		zap.Uint64("rpcID", ack.RPCID),
		zap.String("server", key))

	return nil
}

// handleReceiveResponse processes incoming RESPONSE packets
func (h *ReliableClientHandler) handleReceiveResponse(pkt *packet.DataPacket, addr *net.UDPAddr) error {
	// Extract server connection ID from source
	connID := ConnectionID{IP: pkt.SrcIP, Port: pkt.SrcPort}
	key := connID.String()
	conn := h.getOrCreateConnection(key, connID)

	h.mu.Lock()
	defer h.mu.Unlock()

	// Check for duplicate (message already complete)
	if conn.RxMsgComplete[pkt.RPCID] {
		logging.Debug("Client received duplicate RESPONSE, resending ACK",
			zap.Uint64("rpcID", pkt.RPCID),
			zap.String("server", key))

		// Resend ACK without holding lock
		h.mu.Unlock()
		err := h.sendACK(pkt.RPCID, 1, addr)
		h.mu.Lock()
		if err != nil {
			logging.Error("Failed to resend ACK for duplicate", zap.Error(err))
		}
		return err
	}

	// Initialize tracking if first segment
	if _, exists := conn.RxMsgSeen[pkt.RPCID]; !exists {
		conn.RxMsgSeen[pkt.RPCID] = NewBitset(uint32(pkt.TotalPackets))
		conn.RxMsgCount[pkt.RPCID] = uint32(pkt.TotalPackets)
	}

	// Mark segment received
	conn.RxMsgSeen[pkt.RPCID].Set(uint32(pkt.SeqNumber), true)

	logging.Debug("Client received RESPONSE segment",
		zap.Uint64("rpcID", pkt.RPCID),
		zap.Uint16("seqNumber", pkt.SeqNumber),
		zap.Uint16("totalPackets", pkt.TotalPackets),
		zap.Uint32("receivedCount", conn.RxMsgSeen[pkt.RPCID].PopCount()),
		zap.String("server", key))

	// Check if complete
	if conn.RxMsgSeen[pkt.RPCID].PopCount() == conn.RxMsgCount[pkt.RPCID] {
		conn.RxMsgComplete[pkt.RPCID] = true

		logging.Debug("Client received complete RESPONSE, sending ACK",
			zap.Uint64("rpcID", pkt.RPCID),
			zap.String("server", key))

		// Send ACK without holding lock
		h.mu.Unlock()
		err := h.sendACK(pkt.RPCID, 1, addr)
		h.mu.Lock()

		if err != nil {
			logging.Error("Failed to send ACK", zap.Error(err))
			return err
		}
	}

	return nil
}

// handleReceiveACK processes ACK packets for REQUESTs
func (h *ReliableClientHandler) handleReceiveACK(ack *ACKPacket, addr *net.UDPAddr) error {
	// Extract server connection ID from source
	var connID ConnectionID
	if ip4 := addr.IP.To4(); ip4 != nil {
		copy(connID.IP[:], ip4)
	}
	connID.Port = uint16(addr.Port)
	key := connID.String()

	h.mu.Lock()
	defer h.mu.Unlock()

	// Check if we're tracking this request
	if conn, exists := h.connections[key]; exists {
		// Mark message as ACKed by zeroing SendTs and clear buffered segments
		if msgTx, msgExists := conn.TxMsg[ack.RPCID]; msgExists {
			msgTx.SendTs = time.Time{} // Zero time indicates ACKed
			msgTx.Segments = nil       // Clear buffered data to save memory

			logging.Debug("Client received ACK for REQUEST",
				zap.Uint64("rpcID", ack.RPCID),
				zap.String("server", key))
		}
	}

	return nil
}

// Cleanup cleans up resources
func (h *ReliableClientHandler) Cleanup() {
	h.ReliableHandler.Cleanup(
		transport.TimerKey("reliable_client_cleanup"),
		transport.TimerKey("reliable_client_retransmit"),
	)
}
