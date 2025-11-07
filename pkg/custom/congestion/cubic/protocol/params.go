package protocol

import "time"

const (
	// Estimated timer granularity.
	// The loss detection timer will not be set to a value smaller than granularity.
	TimerGranularity = time.Millisecond

	// InitialPacketSize is the initial (before Path MTU discovery) maximum packet size used.
	InitialPacketSize = 1280

	// MaxCongestionWindowPackets is the maximum congestion window in packet.
	MaxCongestionWindowPackets = 10000

	// MinPacingDelay is the minimum duration that is used for packet pacing
	// If the packet packing frequency is higher, multiple packets might be sent at once.
	// Example: For a packet pacing delay of 200μs, we would send 5 packets at once, wait for 1ms, and so forth.
	MinPacingDelay = time.Millisecond
)
