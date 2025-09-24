package transport

import (
	"net"
	"sync"

	protocol "github.com/appnet-org/arpc/internal/packet"
)

// DataReassembler handles the reassembly of fragmented data (request/response) packets
type DataReassembler struct {
	incoming map[uint64]map[uint16][]byte
	mu       sync.Mutex
}

// NewDataReassembler creates a new data reassembler
func NewDataReassembler() *DataReassembler {
	return &DataReassembler{
		incoming: make(map[uint64]map[uint16][]byte),
	}
}

// ProcessFragment processes a single data packet fragment and returns the reassembled message if complete
func (r *DataReassembler) ProcessFragment(pkt any, addr *net.UDPAddr) ([]byte, *net.UDPAddr, uint64, bool) {
	dataPkt := pkt.(*protocol.DataPacket)

	r.mu.Lock()
	defer r.mu.Unlock()

	// Initialize fragment map for this RPC if it doesn't exist
	if _, exists := r.incoming[dataPkt.RPCID]; !exists {
		r.incoming[dataPkt.RPCID] = make(map[uint16][]byte)
	}

	r.incoming[dataPkt.RPCID][dataPkt.SeqNumber] = dataPkt.Payload

	// Check if we have all fragments
	if len(r.incoming[dataPkt.RPCID]) == int(dataPkt.TotalPackets) {
		// Reassemble the complete message by concatenating fragments in order
		var fullMessage []byte
		for i := uint16(0); i < dataPkt.TotalPackets; i++ {
			fullMessage = append(fullMessage, r.incoming[dataPkt.RPCID][i]...)
		}

		// Clean up fragment storage and return complete message
		delete(r.incoming, dataPkt.RPCID)
		return fullMessage, addr, dataPkt.RPCID, true
	}

	// Still waiting for more fragments, return nil
	return nil, nil, 0, false
}

// FragmentData splits data into multiple packets for Data (Request/Response) packets
func (r *DataReassembler) FragmentData(data []byte, rpcID uint64, packetType protocol.PacketType) ([]*protocol.DataPacket, error) {
	// Calculate chunk size by subtracting header overhead from max UDP payload
	chunkSize := protocol.MaxUDPPayloadSize - 20 // Subtract header size (4+8+2+2+4)
	totalPackets := uint16((len(data) + chunkSize - 1) / chunkSize)
	var packets []*protocol.DataPacket

	for i := range int(totalPackets) {
		// Calculate start and end indices for current chunk
		start := i * chunkSize
		end := min(start+chunkSize, len(data))

		// Create a packet for the current chunk
		pkt := &protocol.DataPacket{
			PacketTypeID: packetType.TypeID,
			RPCID:        rpcID,
			TotalPackets: totalPackets,
			SeqNumber:    uint16(i),
			Payload:      data[start:end],
		}
		packets = append(packets, pkt)
	}

	return packets, nil
}
