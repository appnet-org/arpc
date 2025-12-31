package transport

import "fmt"

// FragmentPackets splits a marshalled Symphony buffer into MTU-sized packets.
// Public segment data is head-aligned (filled from byte 0), private segment data
// is tail-aligned (filled from end), and a meeting packet combines both to
// minimize wasted space.
//
// Returns a list of packets where:
//   - Public segment packets are head-aligned (filled from byte 0)
//   - Private segment packets are tail-aligned (filled from end)
//   - A "meeting packet" combines remaining public + private data to save space
//
// Algorithm:
//  1. Parse offset_to_private from data[1:5] to identify public/private boundary
//  2. Extract public and private segments
//  3. Create head-aligned public packets until all public data is packed
//  4. Optimize by creating a meeting packet if last public packet has space
//  5. Create tail-aligned private packets for remaining private data
func FragmentPackets(data []byte, mtu int) ([][]byte, error) {
	// Validation
	if mtu <= 0 {
		return nil, fmt.Errorf("MTU must be positive, got %d", mtu)
	}
	if mtu < 13 {
		return nil, fmt.Errorf("MTU must be at least 13 bytes (minimum header size), got %d", mtu)
	}
	if len(data) < 13 {
		return nil, fmt.Errorf("data too short: must be at least 13 bytes, got %d", len(data))
	}

	// Edge case: entire data fits in one MTU
	if len(data) <= mtu {
		return [][]byte{data}, nil
	}

	// Parse offset_to_private from bytes 1-5 (little-endian uint32)
	offsetToPrivate := int(data[1]) | int(data[2])<<8 | int(data[3])<<16 | int(data[4])<<24

	if offsetToPrivate > len(data) {
		return nil, fmt.Errorf("invalid offset_to_private: %d exceeds data length %d", offsetToPrivate, len(data))
	}

	// Extract segments
	publicData := data[0:offsetToPrivate]
	privateData := data[offsetToPrivate:]

	var packets [][]byte

	// Phase 1: Head-aligned public packets
	publicOffset := 0
	for publicOffset < len(publicData) {
		chunk := mtu
		if remaining := len(publicData) - publicOffset; remaining < chunk {
			chunk = remaining
		}
		packet := make([]byte, chunk)
		copy(packet, publicData[publicOffset:publicOffset+chunk])
		packets = append(packets, packet)
		publicOffset += chunk
	}

	// Edge case: no private data, return public packets only
	if len(privateData) == 0 {
		return packets, nil
	}

	// Phase 2: Meeting packet optimization
	// If last public packet has space remaining, extend it with private data
	lastPacketIdx := len(packets) - 1
	lastPacket := packets[lastPacketIdx]
	if len(lastPacket) < mtu {
		available := mtu - len(lastPacket)
		privateChunk := available
		if privateChunk > len(privateData) {
			privateChunk = len(privateData)
		}
		// Create meeting packet by combining remaining public + start of private
		meetingPacket := make([]byte, len(lastPacket)+privateChunk)
		copy(meetingPacket, lastPacket)
		copy(meetingPacket[len(lastPacket):], privateData[0:privateChunk])
		packets[lastPacketIdx] = meetingPacket
		// Remove consumed private data
		privateData = privateData[privateChunk:]
	}

	// Phase 3: Tail-aligned private packets
	// Process from end to beginning to achieve tail-alignment
	for len(privateData) > 0 {
		chunk := mtu
		if len(privateData) < chunk {
			chunk = len(privateData)
		}
		packet := make([]byte, mtu)
		// Tail-align: put data at the end, leaving empty space at beginning
		offset := mtu - chunk
		// Take from the end of privateData
		copy(packet[offset:], privateData[len(privateData)-chunk:])
		packets = append(packets, packet)
		// Remove consumed private data
		privateData = privateData[:len(privateData)-chunk]
	}

	return packets, nil
}
