package transport

import (
	"net"
	"sync"

	"github.com/appnet-org/arpc/pkg/common"
	"github.com/appnet-org/arpc/pkg/logging"
	protocol "github.com/appnet-org/arpc/pkg/packet"
	"go.uber.org/zap"
)

// fragmentInfo stores both the payload slice and the original buffer to keep it alive
type fragmentInfo struct {
	payload []byte
	buffer  []byte // Keep reference to original buffer to prevent GC
}

// DataReassembler handles the reassembly of fragmented data (request/response) packets
type DataReassembler struct {
	incoming   map[uint64]map[uint16]*fragmentInfo
	bufferPool *common.BufferPool
	mu         sync.Mutex
}

// NewDataReassembler creates a new data reassembler
func NewDataReassembler() *DataReassembler {
	return &DataReassembler{
		incoming: make(map[uint64]map[uint16]*fragmentInfo),
	}
}

// SetBufferPool sets the buffer pool for returning buffers after reassembly
func (r *DataReassembler) SetBufferPool(pool *common.BufferPool) {
	r.bufferPool = pool
}

// ProcessFragment processes a single data packet fragment and returns the reassembled message if complete
// buffer must be the original buffer that contains the payload (to keep it alive)
func (r *DataReassembler) ProcessFragment(pkt any, addr *net.UDPAddr, buffer []byte) ([]byte, *net.UDPAddr, uint64, bool) {
	dataPkt := pkt.(*protocol.DataPacket)
	// log the peer and source port
	logging.Debug("Processing fragment", zap.String("peer", addr.String()), zap.Uint16("srcPort", dataPkt.SrcPort), zap.Uint64("rpcID", dataPkt.RPCID),
		zap.Uint16("seqNumber", dataPkt.SeqNumber), zap.Uint16("totalPackets", dataPkt.TotalPackets), zap.Int("size", len(dataPkt.Payload)))

	r.mu.Lock()
	defer r.mu.Unlock()

	// Initialize fragment map for this RPC if it doesn't exist
	if _, exists := r.incoming[dataPkt.RPCID]; !exists {
		r.incoming[dataPkt.RPCID] = make(map[uint16]*fragmentInfo)
	}

	// Store both payload and buffer reference to keep buffer alive
	r.incoming[dataPkt.RPCID][dataPkt.SeqNumber] = &fragmentInfo{
		payload: dataPkt.Payload,
		buffer:  buffer,
	}

	// Check if we have all fragments
	if len(r.incoming[dataPkt.RPCID]) == int(dataPkt.TotalPackets) {
		// Calculate total size for pre-allocation
		var totalSize int
		for i := uint16(0); i < dataPkt.TotalPackets; i++ {
			totalSize += len(r.incoming[dataPkt.RPCID][i].payload)
		}

		// Reassemble the complete message by copying fragments in order
		// Use buffer pool if available, otherwise allocate normally
		var fullMessage []byte
		if r.bufferPool != nil {
			fullMessage = r.bufferPool.GetSize(totalSize)
			fullMessage = fullMessage[:0] // Reset length but keep capacity
		} else {
			fullMessage = make([]byte, 0, totalSize)
		}
		for i := uint16(0); i < dataPkt.TotalPackets; i++ {
			frag := r.incoming[dataPkt.RPCID][i]
			fullMessage = append(fullMessage, frag.payload...)
		}

		// Return buffers to pool now that we've copied the data
		if r.bufferPool != nil {
			for i := uint16(0); i < dataPkt.TotalPackets; i++ {
				if frag := r.incoming[dataPkt.RPCID][i]; frag != nil {
					r.bufferPool.Put(frag.buffer)
				}
			}
		}

		// Clean up fragment storage and return complete message
		delete(r.incoming, dataPkt.RPCID)
		return fullMessage, addr, dataPkt.RPCID, true
	}

	// Still waiting for more fragments, return nil
	return nil, nil, 0, false
}

// FragmentData splits data into multiple packets for Data (Request/Response) packets
func (r *DataReassembler) FragmentData(data []byte, rpcID uint64, packetType protocol.PacketType, dstIP [4]byte, dstPort uint16, srcIP [4]byte, srcPort uint16) ([]any, error) {
	if packetType == protocol.PacketTypeError || packetType == protocol.PacketTypeUnknown {
		packets := []any{}
		packets = append(packets, &protocol.ErrorPacket{
			PacketTypeID: packetType.TypeID,
			RPCID:        rpcID,
			ErrorMsg:     string(data),
		})
		return packets, nil
	}
	// Calculate chunk size by subtracting header overhead from max UDP payload
	// New header size: 1+8+2+2+4+2+4+2+4 = 29 bytes
	chunkSize := protocol.MaxUDPPayloadSize - 29
	totalPackets := uint16((len(data) + chunkSize - 1) / chunkSize)
	var packets []any

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
			DstIP:        dstIP,
			DstPort:      dstPort,
			SrcIP:        srcIP,
			SrcPort:      srcPort,
			Payload:      data[start:end],
		}
		packets = append(packets, pkt)
	}

	return packets, nil
}
