package elements

import (
	"bytes"
	"encoding/binary"
	"log"
	"net"
	"sync"
	"time"

	"github.com/appnet-org/arpc/internal/protocol"
	"github.com/appnet-org/arpc/internal/transport"
)

// ReliabilityElement implements reliability features like retransmission
type ReliabilityElement struct {
	transport  *transport.UDPTransport
	role       transport.Role
	maxRetries uint16
	timeout    time.Duration

	mu      sync.Mutex
	pending map[uint64]*reliableMessage

	// For retry mechanism
	stopChan chan struct{}
	wg       sync.WaitGroup
}

type reliableMessage struct {
	data     []byte
	sentTime time.Time
	retries  uint16
	addr     string // Store the destination address for retries
}

func NewReliabilityElement(role transport.Role, maxRetries uint16, timeout time.Duration) *ReliabilityElement {
	re := &ReliabilityElement{
		role:       role,
		maxRetries: maxRetries,
		timeout:    timeout,
		pending:    make(map[uint64]*reliableMessage),
		stopChan:   make(chan struct{}),
	}

	// Start retry goroutine only for clients
	if role == transport.RoleClient {
		re.wg.Add(1)
		go re.retryLoop()
	}

	return re
}

// Stop stops the retry loop
func (r *ReliabilityElement) Stop() {
	close(r.stopChan)
	r.wg.Wait()
}

func (r *ReliabilityElement) ProcessSend(addr string, data []byte, rpcID uint64) ([]byte, error) {
	log.Printf("ReliabilityElement[%s]: Processing send for RPC ID %d", r.role, rpcID)

	r.mu.Lock()
	defer r.mu.Unlock()

	// Store the message for potential retries (only for clients)
	if r.role == transport.RoleClient {
		r.pending[rpcID] = &reliableMessage{
			data:     data,
			sentTime: time.Now(),
			retries:  0,
			addr:     addr,
		}
	}

	return data, nil
}

func (r *ReliabilityElement) ProcessReceive(data []byte, rpcID uint64, packetType protocol.PacketType, addr *net.UDPAddr, conn *net.UDPConn) ([]byte, error) {
	log.Printf("ReliabilityElement[%s]: Processing receive for RPC ID %d", r.role, rpcID)

	// Only send ACK if we're the server (callee) and the packet type is data
	if r.role == transport.RoleServer && (packetType == protocol.PacketTypeRequest || packetType == protocol.PacketTypeResponse) {
		buf := new(bytes.Buffer)

		if err := binary.Write(buf, binary.LittleEndian, protocol.PacketTypeAck); err != nil {
			return nil, err
		}

		if err := binary.Write(buf, binary.LittleEndian, rpcID); err != nil {
			return nil, err
		}

		if _, err := conn.WriteToUDP(buf.Bytes(), addr); err != nil {
			log.Printf("ReliabilityElement[%s]: Failed to send ACK for RPC ID %d: %v", r.role, rpcID, err)
			return nil, err
		}
	}

	// If we're the client and the packet type is ACK, delete the pending message
	if r.role == transport.RoleClient && packetType == protocol.PacketTypeAck {
		r.mu.Lock()
		delete(r.pending, rpcID)
		r.mu.Unlock()
		log.Printf("ReliabilityElement[%s]: Received ACK for RPC ID %d, removed from pending", r.role, rpcID)
	}

	return data, nil
}

// retryLoop periodically checks for timed-out messages and retries them
func (r *ReliabilityElement) retryLoop() {
	defer r.wg.Done()

	ticker := time.NewTicker(r.timeout / 10) // Check ten times as often as timeout
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.checkTimeouts()
		case <-r.stopChan:
			return
		}
	}
}

// checkTimeouts checks for timed-out messages and retries them
func (r *ReliabilityElement) checkTimeouts() {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	var toDelete []uint64

	for rpcID, msg := range r.pending {
		if now.Sub(msg.sentTime) > r.timeout {
			if msg.retries >= r.maxRetries {
				log.Printf("ReliabilityElement[%s]: RPC ID %d exceeded max retries (%d), giving up", r.role, rpcID, r.maxRetries)
				toDelete = append(toDelete, rpcID)
				continue
			}

			// Retry the message
			if err := r.transport.Send(msg.addr, rpcID, msg.data, protocol.PacketTypeRequest); err != nil {
				log.Printf("ReliabilityElement[%s]: Failed to retry RPC ID %d: %v", r.role, rpcID, err)
				toDelete = append(toDelete, rpcID)
			} else {
				msg.retries++
				msg.sentTime = now
				log.Printf("ReliabilityElement[%s]: Retried RPC ID %d (attempt %d/%d)", r.role, rpcID, msg.retries, r.maxRetries)
			}
		}
	}

	// Clean up messages that exceeded max retries
	for _, rpcID := range toDelete {
		delete(r.pending, rpcID)
	}
}

func (r *ReliabilityElement) Name() string {
	return "reliability"
}

func (r *ReliabilityElement) GetRole() transport.Role {
	return r.role
}
