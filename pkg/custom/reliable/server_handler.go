package reliable

import (
	"net"
	"time"

	"github.com/appnet-org/arpc/pkg/logging"
	"github.com/appnet-org/arpc/pkg/packet"
	"github.com/appnet-org/arpc/pkg/transport"
	"go.uber.org/zap"
)

// ReliableServerHandler implements the server-side reliable transport logic
type ReliableServerHandler struct {
	*ReliableHandler // Embed base handler
}

// NewReliableServerHandler creates a new reliable server handler with default 30s timeout
func NewReliableServerHandler(transportSender TransportSender, timerMgr TimerScheduler) *ReliableServerHandler {
	return NewReliableServerHandlerWithTimeout(transportSender, timerMgr, 30*time.Second)
}

// NewReliableServerHandlerWithTimeout creates a new reliable server handler with custom timeout
func NewReliableServerHandlerWithTimeout(transportSender TransportSender, timerMgr TimerScheduler, timeout time.Duration) *ReliableServerHandler {
	handler := &ReliableServerHandler{
		ReliableHandler: newReliableHandler(transportSender, timerMgr, timeout),
	}
	handler.startCleanupTimer(transport.TimerKey("reliable_server_cleanup"))
	handler.startRetransmitTimer(transport.TimerKey("reliable_server_retransmit"), 1*time.Second)

	logging.Debug("Reliable server handler created",
		zap.Duration("timeout", timeout))

	return handler
}

// OnReceive handles incoming REQUEST and ACK(kind=1) packets
func (h *ReliableServerHandler) OnReceive(pkt any, addr *net.UDPAddr) error {
	switch p := pkt.(type) {
	case *packet.DataPacket:
		if p.PacketTypeID == packet.PacketTypeRequest.TypeID {
			return h.handleReceiveRequest(p, addr)
		}
	case *ACKPacket:
		if p.Kind == 1 { // ACK for RESPONSE
			return h.handleReceiveACK(p, addr)
		}
	}
	return nil
}

// OnSend handles outgoing RESPONSE packets and ACK(kind=0) packets
func (h *ReliableServerHandler) OnSend(pkt any, addr *net.UDPAddr) error {
	switch p := pkt.(type) {
	case *packet.DataPacket:
		if p.PacketTypeID == packet.PacketTypeResponse.TypeID {
			return h.handleSendResponse(p)
		}
	case *ACKPacket:
		if p.Kind == 0 { // ACK for REQUEST
			return h.handleSendACK(p, addr)
		}
	}
	return nil
}

// handleReceiveRequest processes incoming REQUEST packets
func (h *ReliableServerHandler) handleReceiveRequest(pkt *packet.DataPacket, addr *net.UDPAddr) error {
	// Extract client connection ID from source
	connID := ConnectionID{IP: pkt.SrcIP, Port: pkt.SrcPort}
	key := connID.String()
	conn := h.getOrCreateConnection(key, connID)

	h.mu.Lock()
	defer h.mu.Unlock()

	// Check for duplicate (message already complete)
	if conn.RxMsgComplete[pkt.RPCID] {
		logging.Debug("Server received duplicate REQUEST, resending ACK",
			zap.Uint64("rpcID", pkt.RPCID),
			zap.String("client", key))

		// Resend ACK without holding lock
		h.mu.Unlock()
		err := h.sendACK(pkt.RPCID, 0, addr)
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

	logging.Debug("Server received REQUEST segment",
		zap.Uint64("rpcID", pkt.RPCID),
		zap.Uint16("seqNumber", pkt.SeqNumber),
		zap.Uint16("totalPackets", pkt.TotalPackets),
		zap.Uint32("receivedCount", conn.RxMsgSeen[pkt.RPCID].PopCount()),
		zap.String("client", key))

	// Check if complete
	if conn.RxMsgSeen[pkt.RPCID].PopCount() == conn.RxMsgCount[pkt.RPCID] {
		conn.RxMsgComplete[pkt.RPCID] = true

		logging.Debug("Server received complete REQUEST, sending ACK",
			zap.Uint64("rpcID", pkt.RPCID),
			zap.String("client", key))

		// Send ACK without holding lock
		h.mu.Unlock()
		err := h.sendACK(pkt.RPCID, 0, addr)
		h.mu.Lock()

		if err != nil {
			logging.Error("Failed to send ACK", zap.Error(err))
			return err
		}
	}

	return nil
}

// handleReceiveACK processes ACK packets for RESPONSEs
func (h *ReliableServerHandler) handleReceiveACK(ack *ACKPacket, addr *net.UDPAddr) error {
	// Extract client connection ID from source
	var connID ConnectionID
	if ip4 := addr.IP.To4(); ip4 != nil {
		copy(connID.IP[:], ip4)
	}
	connID.Port = uint16(addr.Port)
	key := connID.String()

	h.mu.Lock()
	defer h.mu.Unlock()

	// Check if we're tracking this client
	if conn, exists := h.connections[key]; exists {
		// Mark message as ACKed by zeroing SendTs and clear buffered segments
		if msgTx, msgExists := conn.TxMsg[ack.RPCID]; msgExists {
			msgTx.SendTs = time.Time{} // Zero time indicates ACKed
			msgTx.Segments = nil       // Clear buffered data to save memory

			logging.Debug("Server received ACK for RESPONSE",
				zap.Uint64("rpcID", ack.RPCID),
				zap.String("client", key))
		}
	}

	return nil
}

// handleSendResponse tracks outgoing RESPONSE packets
func (h *ReliableServerHandler) handleSendResponse(pkt *packet.DataPacket) error {
	// Extract client connection ID from destination
	connID := ConnectionID{IP: pkt.DstIP, Port: pkt.DstPort}
	key := connID.String()
	conn := h.getOrCreateConnection(key, connID)

	h.mu.Lock()
	defer h.mu.Unlock()

	// Serialize packet for buffering
	serializedData, err := h.serializeDataPacket(pkt)
	if err != nil {
		logging.Error("Failed to serialize RESPONSE packet for buffering",
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

	logging.Debug("Server tracking sent RESPONSE",
		zap.Uint64("rpcID", pkt.RPCID),
		zap.Uint16("seqNumber", pkt.SeqNumber),
		zap.Uint16("totalPackets", pkt.TotalPackets),
		zap.String("client", key))

	return nil
}

// handleSendACK handles sending ACK packets (already formed, just update activity)
func (h *ReliableServerHandler) handleSendACK(ack *ACKPacket, addr *net.UDPAddr) error {
	// Extract client connection ID from destination
	// Note: ACK packet doesn't have DstIP/Port, so we use addr
	var connID ConnectionID
	if ip4 := addr.IP.To4(); ip4 != nil {
		copy(connID.IP[:], ip4)
	}
	connID.Port = uint16(addr.Port)
	key := connID.String()

	// Just update activity timestamp
	h.getOrCreateConnection(key, connID)

	logging.Debug("Server sending ACK for REQUEST",
		zap.Uint64("rpcID", ack.RPCID),
		zap.String("client", key))

	return nil
}

// Cleanup cleans up resources
func (h *ReliableServerHandler) Cleanup() {
	h.ReliableHandler.Cleanup(
		transport.TimerKey("reliable_server_cleanup"),
		transport.TimerKey("reliable_server_retransmit"),
	)
}
