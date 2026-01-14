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

// seqCompletionInfo tracks completion status for a sequence number
type seqCompletionInfo struct {
	lastFragmentIndex uint8 // The highest FragmentIndex received with MoreFragments=false
	isComplete        bool  // True if we've received the last fragment (MoreFragments=false)
	hasLastFragment   bool  // True if we've received at least one fragment with MoreFragments=false
}

// DataReassembler handles the reassembly of fragmented data (request/response) packets
type DataReassembler struct {
	incoming   map[uint64]map[uint16]map[uint8]*fragmentInfo // RPCID -> SeqNumber -> FragmentIndex -> fragmentInfo
	completion map[uint64]map[uint16]*seqCompletionInfo      // RPCID -> SeqNumber -> completion info
	bufferPool *common.BufferPool
	mu         sync.Mutex
}

// NewDataReassembler creates a new data reassembler
func NewDataReassembler() *DataReassembler {
	return &DataReassembler{
		incoming:   make(map[uint64]map[uint16]map[uint8]*fragmentInfo),
		completion: make(map[uint64]map[uint16]*seqCompletionInfo),
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
		zap.Uint16("seqNumber", dataPkt.SeqNumber), zap.Uint16("totalPackets", dataPkt.TotalPackets), zap.Bool("moreFragments", dataPkt.MoreFragments), zap.Uint8("fragmentIndex", dataPkt.FragmentIndex), zap.Int("size", len(dataPkt.Payload)))

	r.mu.Lock()
	defer r.mu.Unlock()

	// Initialize fragment map for this RPC if it doesn't exist
	if _, exists := r.incoming[dataPkt.RPCID]; !exists {
		r.incoming[dataPkt.RPCID] = make(map[uint16]map[uint8]*fragmentInfo)
		r.completion[dataPkt.RPCID] = make(map[uint16]*seqCompletionInfo)
	}

	// Initialize fragment map for this sequence number if it doesn't exist
	if _, exists := r.incoming[dataPkt.RPCID][dataPkt.SeqNumber]; !exists {
		r.incoming[dataPkt.RPCID][dataPkt.SeqNumber] = make(map[uint8]*fragmentInfo)
		r.completion[dataPkt.RPCID][dataPkt.SeqNumber] = &seqCompletionInfo{
			isComplete: false,
		}
	}

	// Store both payload and buffer reference to keep buffer alive
	r.incoming[dataPkt.RPCID][dataPkt.SeqNumber][dataPkt.FragmentIndex] = &fragmentInfo{
		payload: dataPkt.Payload,
		buffer:  buffer,
	}

	// If this is the last fragment (MoreFragments=false), update lastFragmentIndex
	// This tells us the highest FragmentIndex we need to receive
	if !dataPkt.MoreFragments {
		compInfo := r.completion[dataPkt.RPCID][dataPkt.SeqNumber]
		compInfo.hasLastFragment = true
		if dataPkt.FragmentIndex >= compInfo.lastFragmentIndex {
			compInfo.lastFragmentIndex = dataPkt.FragmentIndex
		}
	}

	// Check if we have all fragments for this RPC
	rpcFragments := r.incoming[dataPkt.RPCID]
	rpcCompletion := r.completion[dataPkt.RPCID]

	// Check if all sequence numbers are present
	if len(rpcFragments) != int(dataPkt.TotalPackets) {
		// Still waiting for more sequence numbers
		return nil, nil, 0, false
	}

	// Check if all sequence numbers are complete
	for seqNum := uint16(0); seqNum < dataPkt.TotalPackets; seqNum++ {
		seqFragments, exists := rpcFragments[seqNum]
		if !exists || len(seqFragments) == 0 {
			// Missing this sequence number entirely
			return nil, nil, 0, false
		}

		compInfo, compExists := rpcCompletion[seqNum]
		if !compExists {
			// Missing completion info for this sequence number
			return nil, nil, 0, false
		}

		// Check if this sequence number is complete
		// A sequence is complete when:
		// 1. We know the last fragment index (we've received at least one fragment with MoreFragments=false)
		// 2. We have all fragments from 0 to lastFragmentIndex
		// This handles out-of-order packet arrival correctly
		if !compInfo.hasLastFragment {
			// We haven't received the last fragment for this sequence number yet
			return nil, nil, 0, false
		}

		// Verify we have all fragments from 0 to lastFragmentIndex
		for fragIdx := uint8(0); fragIdx <= compInfo.lastFragmentIndex; fragIdx++ {
			if _, exists := seqFragments[fragIdx]; !exists {
				// Missing a fragment in the sequence
				return nil, nil, 0, false
			}
		}
	}

	// All fragments are complete, reassemble the message
	// Calculate total size for pre-allocation
	var totalSize int
	for seqNum := uint16(0); seqNum < dataPkt.TotalPackets; seqNum++ {
		seqFragments := rpcFragments[seqNum]
		compInfo := rpcCompletion[seqNum]
		// Sum up all fragment sizes for this sequence (from 0 to lastFragmentIndex)
		for fragIdx := uint8(0); fragIdx <= compInfo.lastFragmentIndex; fragIdx++ {
			if frag, exists := seqFragments[fragIdx]; exists {
				totalSize += len(frag.payload)
			}
		}
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

	// For each sequence number in order
	for seqNum := uint16(0); seqNum < dataPkt.TotalPackets; seqNum++ {
		seqFragments := rpcFragments[seqNum]
		compInfo := rpcCompletion[seqNum]
		// Concatenate all FragmentIndex fragments in order (0, 1, 2, ..., lastFragmentIndex)
		for fragIdx := uint8(0); fragIdx <= compInfo.lastFragmentIndex; fragIdx++ {
			if frag, exists := seqFragments[fragIdx]; exists {
				fullMessage = append(fullMessage, frag.payload...)
			}
		}
	}

	// Return buffers to pool now that we've copied the data
	if r.bufferPool != nil {
		for seqNum := uint16(0); seqNum < dataPkt.TotalPackets; seqNum++ {
			seqFragments := rpcFragments[seqNum]
			for _, frag := range seqFragments {
				if frag != nil {
					r.bufferPool.Put(frag.buffer)
				}
			}
		}
	}

	// Clean up fragment storage and return complete message
	delete(r.incoming, dataPkt.RPCID)
	delete(r.completion, dataPkt.RPCID)
	return fullMessage, addr, dataPkt.RPCID, true
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
	// New header size: 1+8+2+2+1+1+4+2+4+2+4 = 31 bytes
	chunkSize := protocol.MaxUDPPayloadSize - 31
	totalPackets := uint16((len(data) + chunkSize - 1) / chunkSize)
	var packets []any

	for i := range int(totalPackets) {
		// Calculate start and end indices for current chunk
		start := i * chunkSize
		end := min(start+chunkSize, len(data))

		// Create a packet for the current chunk
		pkt := &protocol.DataPacket{
			PacketTypeID:  packetType.TypeID,
			RPCID:         rpcID,
			TotalPackets:  totalPackets,
			SeqNumber:     uint16(i),
			MoreFragments: false,
			FragmentIndex: 0,
			DstIP:         dstIP,
			DstPort:       dstPort,
			SrcIP:         srcIP,
			SrcPort:       srcPort,
			Payload:       data[start:end],
		}
		packets = append(packets, pkt)
	}

	return packets, nil
}
