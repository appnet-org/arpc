package main

import (
	"time"

	"github.com/appnet-org/arpc/pkg/transport"
)

// DummyTimerManager is a no-op timer implementation for testing timer overhead.
// This implementation does absolutely nothing - no goroutines, no timers, no callbacks.
// Use this to measure the latency impact of the timer system itself.
//
// To use:
//  1. Replace udpTransport.GetTimerManager() with NewDummyTimerManager()
//  2. Run benchmarks to measure latency without timer overhead
//  3. Compare with real timer implementation to see timer impact
type DummyTimerManager struct{}

// NewDummyTimerManager creates a new dummy timer manager that does nothing
func NewDummyTimerManager() *DummyTimerManager {
	return &DummyTimerManager{}
}

// Schedule does nothing (no-op)
// This method is called when scheduling per-packet/message timeouts
func (d *DummyTimerManager) Schedule(id transport.TimerKey, duration time.Duration, callback transport.TimerCallback) {
	// Do nothing - this is a dummy implementation to test timer overhead
	// In real implementation, this would spawn a goroutine and set up a timer
}

// SchedulePeriodic does nothing (no-op)
// This method is called when scheduling periodic cleanup timers
func (d *DummyTimerManager) SchedulePeriodic(id transport.TimerKey, interval time.Duration, callback transport.TimerCallback) {
	// Do nothing - this is a dummy implementation to test timer overhead
	// In real implementation, this would spawn a goroutine with a ticker
}

// StopTimer does nothing (no-op)
// This method is called when stopping timers (e.g., when packets are ACKed)
func (d *DummyTimerManager) StopTimer(id transport.TimerKey) bool {
	// Do nothing - this is a dummy implementation to test timer overhead
	// In real implementation, this would stop the timer and clean up resources
	return false
}
