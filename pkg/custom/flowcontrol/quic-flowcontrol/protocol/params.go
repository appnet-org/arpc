package protocol

import "time"

// WindowUpdateThreshold is the fraction of the receive window that has to be consumed before an higher offset is advertised to the client
const WindowUpdateThreshold = 0.25

// Estimated timer granularity.
// The loss detection timer will not be set to a value smaller than granularity.
const TimerGranularity = time.Millisecond

// ConnectionFlowControlMultiplier determines how much larger the connection flow control windows needs to be relative to any stream's flow control window
// This is the value that Chromium is using
const ConnectionFlowControlMultiplier = 1.5
