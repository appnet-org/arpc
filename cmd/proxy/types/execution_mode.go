package types

// ExecutionMode defines how the proxy should handle packets for an element.
type ExecutionMode int

const (
	// StreamingMode forwards packets immediately without buffering.
	StreamingMode ExecutionMode = iota

	// StreamingWithBufferingMode forwards packets immediately but retains a copy for logging or analysis.
	StreamingWithBufferingMode

	// MaybeBufferingMode may buffer the packet if the message needs.
	MaybeBufferingMode

	// FullBufferingMode buffers the entire message before forwarding.
	FullBufferingMode
)

// String returns the string representation of the ExecutionMode.
func (e ExecutionMode) String() string {
	switch e {
	case StreamingMode:
		return "streaming"
	case StreamingWithBufferingMode:
		return "streaming-with-buffering"
	case FullBufferingMode:
		return "full-buffering"
	case MaybeBufferingMode:
		return "maybe-buffering"
	default:
		return "unknown"
	}
}
