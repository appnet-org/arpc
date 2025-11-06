package types

// ExecutionMode defines how the proxy should handle packets for an element.
type ExecutionMode int

const (
	// StreamingMode forwards packets immediately without buffering.
	StreamingMode ExecutionMode = iota

	// StreamingWithBufferingMode forwards packets immediately but retains a copy for logging or analysis.
	StreamingWithBufferingMode

	// FullBufferingMode buffers the entire message before forwarding.
	FullBufferingMode
)
