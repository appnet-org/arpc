// Package transport provides timer management functionality for aRPC transport layer operations.
package transport

import (
	"sync"
	"time"
)

// TimerCallback is a function type for timer callbacks
type TimerCallback func()

// TimerKey is a unique identifier for timers (uint64 for efficiency)
type TimerKey uint64

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

// Schedule creates a one-time timer that executes after the given duration.
func (tm *TimerManager) Schedule(id TimerKey, duration time.Duration, callback TimerCallback) {
	tm.mu.Lock()
	// Replace safely: delete-before-close to avoid races with StopTimer.
	if existing, exists := tm.timers[id]; exists {
		delete(tm.timers, id)
		close(existing.Stop)
	}

	t := &Timer{
		ID:       id,
		Duration: duration,
		Callback: callback,
		Stop:     make(chan struct{}),
	}
	tm.timers[id] = t
	tm.mu.Unlock()

	tm.wg.Add(1)
	go func(t *Timer) {
		defer tm.wg.Done()

		tt := time.NewTimer(t.Duration)
		defer tt.Stop()

		select {
		case <-tt.C:
			// Fired normally
			tm.executeCallback(t.ID, false)

		case <-t.Stop:
			// Canceled: stop the underlying timer and drain if it already fired
			if !tt.Stop() {
				<-tt.C
			}

		case <-tm.stopAll:
			// Global shutdown: stop and drain if necessary
			if !tt.Stop() {
				<-tt.C
			}
		}

		// Single source of truth for one-shot removal: do it here.
		tm.mu.Lock()
		delete(tm.timers, t.ID)
		tm.mu.Unlock()
	}(t)
}

// SchedulePeriodic creates a timer that executes repeatedly at the given interval.
func (tm *TimerManager) SchedulePeriodic(id TimerKey, interval time.Duration, callback TimerCallback) {
	tm.mu.Lock()
	// Replace safely: delete-before-close to avoid races with StopTimer.
	if existing, exists := tm.periodic[id]; exists {
		delete(tm.periodic, id)
		close(existing.Stop)
	}

	t := &Timer{
		ID:       id,
		Duration: interval,
		Callback: callback,
		Stop:     make(chan struct{}),
	}
	tm.periodic[id] = t
	tm.mu.Unlock()

	tm.wg.Add(1)
	go func(t *Timer) {
		defer tm.wg.Done()

		ticker := time.NewTicker(t.Duration)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				tm.executeCallback(t.ID, true)

			case <-t.Stop:
				// Remove entry before exit
				tm.mu.Lock()
				delete(tm.periodic, t.ID)
				tm.mu.Unlock()
				return

			case <-tm.stopAll:
				// Remove entry before exit
				tm.mu.Lock()
				delete(tm.periodic, t.ID)
				tm.mu.Unlock()
				return
			}
		}
	}(t)
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

// executeCallback safely executes a timer callback (no locks held during user code)
func (tm *TimerManager) executeCallback(id TimerKey, isPeriodic bool) {
	// Snapshot the callback under read lock
	tm.mu.RLock()
	var cb TimerCallback
	if isPeriodic {
		if t := tm.periodic[id]; t != nil {
			cb = t.Callback
		}
	} else {
		if t := tm.timers[id]; t != nil {
			cb = t.Callback
		}
	}
	tm.mu.RUnlock()

	if cb == nil {
		return
	}

	// Panic safety so a bad callback doesn't break the scheduler.
	// TODO: add logging
	defer func() {
		_ = recover()
	}()

	cb()
}

// start initializes the timer manager
func (tm *TimerManager) start() {
	// Manager is ready to use
}

// Stop stops all timers and shuts down the manager
func (tm *TimerManager) Stop() {
	// Broadcast shutdown
	close(tm.stopAll)
	tm.wg.Wait()
}
