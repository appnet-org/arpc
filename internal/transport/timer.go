// Package transport provides timer management functionality for aRPC transport layer operations.
// This file implements a TimerManager that handles both scheduled one-time timers and periodic
// recurring timers, designed specifically for transport layer needs such as:
//
//   - Retransmission timeouts for reliable packet delivery
//   - Congestion control intervals for bandwidth management
//   - General timeout handling for transport operations
//
// The TimerManager is thread-safe and provides automatic cleanup of completed timers.
package transport

import (
	"sync"
	"time"
)

// TimerCallback is a function type for timer callbacks
type TimerCallback func()

type TimerKey string

// Timer represents a single timer instance
type Timer struct {
	ID       TimerKey
	Duration time.Duration
	Callback TimerCallback
	Stop     chan struct{}
}

// TimerManager handles both scheduled and periodic timers
type TimerManager struct {
	mu       sync.RWMutex
	timers   map[TimerKey]*Timer
	periodic map[TimerKey]*Timer
	stopAll  chan struct{}
	wg       sync.WaitGroup
}

// NewTimerManager creates a new timer manager
func NewTimerManager() *TimerManager {
	tm := &TimerManager{
		timers:   make(map[TimerKey]*Timer),
		periodic: make(map[TimerKey]*Timer),
		stopAll:  make(chan struct{}),
	}
	tm.start()
	return tm
}

// Schedule creates a one-time timer that executes after the given duration
func (tm *TimerManager) Schedule(id TimerKey, duration time.Duration, callback TimerCallback) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Stop existing timer if it exists
	if existing, exists := tm.timers[id]; exists {
		close(existing.Stop)
	}

	timer := &Timer{
		ID:       id,
		Duration: duration,
		Callback: callback,
		Stop:     make(chan struct{}),
	}

	tm.timers[id] = timer

	tm.wg.Add(1)
	go func() {
		defer tm.wg.Done()
		select {
		case <-time.After(duration):
			tm.executeCallback(id, false)
		case <-timer.Stop:
			// Timer was stopped
		}
	}()
}

// SchedulePeriodic creates a timer that executes repeatedly at the given interval
func (tm *TimerManager) SchedulePeriodic(id TimerKey, interval time.Duration, callback TimerCallback) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Stop existing periodic timer if it exists
	if existing, exists := tm.periodic[id]; exists {
		close(existing.Stop)
	}

	timer := &Timer{
		ID:       id,
		Duration: interval,
		Callback: callback,
		Stop:     make(chan struct{}),
	}

	tm.periodic[id] = timer

	tm.wg.Add(1)
	go func() {
		defer tm.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				tm.executeCallback(id, true)
			case <-timer.Stop:
				return
			case <-tm.stopAll:
				return
			}
		}
	}()
}

// StopTimer stops a specific timer
func (tm *TimerManager) StopTimer(id TimerKey) bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Check scheduled timers
	if timer, exists := tm.timers[id]; exists {
		close(timer.Stop)
		delete(tm.timers, id)
		return true
	}

	// Check periodic timers
	if timer, exists := tm.periodic[id]; exists {
		close(timer.Stop)
		delete(tm.periodic, id)
		return true
	}

	return false
}

// HasTimer checks if a timer with the given ID exists
func (tm *TimerManager) HasTimer(id TimerKey) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	_, hasScheduled := tm.timers[id]
	_, hasPeriodic := tm.periodic[id]
	return hasScheduled || hasPeriodic
}

// executeCallback safely executes a timer callback
func (tm *TimerManager) executeCallback(id TimerKey, isPeriodic bool) {
	tm.mu.RLock()
	var timer *Timer
	if isPeriodic {
		timer = tm.periodic[id]
	} else {
		timer = tm.timers[id]
	}
	tm.mu.RUnlock()

	if timer != nil && timer.Callback != nil {
		timer.Callback()

		// Remove one-time timers after execution
		if !isPeriodic {
			tm.mu.Lock()
			delete(tm.timers, id)
			tm.mu.Unlock()
		}
	}
}

// start initializes the timer manager
func (tm *TimerManager) start() {
	// Manager is ready to use
}

// Stop stops all timers and shuts down the manager
func (tm *TimerManager) Stop() {
	close(tm.stopAll)

	tm.mu.Lock()
	for _, timer := range tm.timers {
		close(timer.Stop)
	}
	for _, timer := range tm.periodic {
		close(timer.Stop)
	}
	tm.mu.Unlock()

	tm.wg.Wait()
}
