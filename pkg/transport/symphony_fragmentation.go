package transport

import "fmt"

// FragmentPackets fragments data into MTU-sized packets using a slack optimization strategy.
// The input data is expected to contain an offset header (bytes 1-5) that separates the data
// into public and private sections. The function performs three phases:
//
//  1. Public Packets: Creates full MTU-sized packets from the public data section.
//  2. Meeting Packet: Combines the remainder of public data with a chunk of private data
//     to minimize wasted space (slack optimization). This ensures subsequent private data
//     packets are aligned to MTU boundaries.
//  3. Private Packets: Creates full MTU-sized packets from the remaining private data.
//
// Parameters:
//   - data: The input data to fragment, containing an offset header in bytes 1-5 that
//     indicates where the private data section begins.
//   - mtu: Maximum Transmission Unit size. Must be positive.
//
// Returns:
//   - [][]byte: A slice of packet fragments, each no larger than MTU bytes.
//   - error: Returns an error if MTU is invalid, data is too short, or the offset is invalid.
func FragmentPackets(data []byte, mtu int) ([][]byte, error) {
	// 1. Validation & Setup
	if mtu <= 0 {
		return nil, fmt.Errorf("MTU must be positive, got %d", mtu)
	}
	if len(data) <= mtu {
		return [][]byte{data}, nil
	}

	// Parse offset (bytes 1-5)
	if len(data) < 5 {
		return nil, fmt.Errorf("data too short for offset header")
	}
	offsetToPrivate := int(data[1]) | int(data[2])<<8 | int(data[3])<<16 | int(data[4])<<24
	if offsetToPrivate > len(data) {
		return nil, fmt.Errorf("invalid offset")
	}

	publicData := data[0:offsetToPrivate]
	privateData := data[offsetToPrivate:]

	var packets [][]byte

	// 2. Phase 1: Public Packets (Standard)
	// Fill all full MTU packets for public data
	publicOffset := 0
	for len(publicData)-publicOffset > mtu {
		packet := make([]byte, mtu)
		copy(packet, publicData[publicOffset:publicOffset+mtu])
		packets = append(packets, packet)
		publicOffset += mtu
	}

	// The "remainder" of public data starts the meeting packet
	meetingPacket := make([]byte, len(publicData)-publicOffset)
	copy(meetingPacket, publicData[publicOffset:])

	// 3. Phase 2: The Meeting Packet (Slack Optimization)
	if len(privateData) > 0 {
		// Calculate how much private data we need to peel off
		// so that the REST of privateData is a multiple of MTU.
		privateHeadLen := len(privateData) % mtu

		// If privateData is already a multiple of MTU (rem == 0),
		// we don't add anything here unless you want to force an empty check.
		// Usually, 0 remainder means the rest is already aligned.

		chunk := privateData[:privateHeadLen]
		privateData = privateData[privateHeadLen:] // Advance slice

		// Append this chunk to the meeting packet
		// This might exceed MTU if (public_rem + private_rem) > MTU
		currentMeetingLen := len(meetingPacket)
		totalLen := currentMeetingLen + len(chunk)

		if totalLen <= mtu {
			// Case A: Fits in one packet. Slack is here.
			newPacket := make([]byte, totalLen)
			copy(newPacket, meetingPacket)
			copy(newPacket[currentMeetingLen:], chunk)
			packets = append(packets, newPacket)
		} else {
			// Case B: Overflow.
			// We fill the meeting packet to MTU, and push the rest to a new small packet.
			// This ensures the SUBSEQUENT private packets stay aligned.

			// 1. Fill current meeting packet to MTU
			fillNeeded := mtu - currentMeetingLen
			packet1 := make([]byte, mtu)
			copy(packet1, meetingPacket)
			copy(packet1[currentMeetingLen:], chunk[:fillNeeded])
			packets = append(packets, packet1)

			// 2. Put the overflow in a new packet (this is where the slack ends up)
			overflow := chunk[fillNeeded:]
			packet2 := make([]byte, len(overflow))
			copy(packet2, overflow)
			packets = append(packets, packet2)
		}
	} else {
		// If no private data, just append the public remainder
		if len(meetingPacket) > 0 {
			packets = append(packets, meetingPacket)
		}
	}

	// 4. Phase 3: Remaining Private Data
	// At this point, privateData is guaranteed to be a multiple of MTU (or empty).
	for len(privateData) > 0 {
		chunkSize := mtu
		if len(privateData) < mtu {
			chunkSize = len(privateData) // Should not happen if math works, but safe
		}

		packet := make([]byte, chunkSize)
		copy(packet, privateData[:chunkSize])
		packets = append(packets, packet)

		privateData = privateData[chunkSize:]
	}

	return packets, nil
}
