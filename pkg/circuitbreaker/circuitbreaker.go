// Package circuitbreaker implements a simple circuit breaker for reactor calls.
package circuitbreaker

import (
	"errors"
	"sync"
	"time"
)

// ErrOpen is returned when the circuit is open.
var ErrOpen = errors.New("circuit breaker is open")

// State represents the circuit state.
type State int

const (
	Closed   State = iota // Normal operation
	Open                  // Failing, reject requests
	HalfOpen              // Testing if recovered
)

// Breaker is a circuit breaker.
type Breaker struct {
	mu           sync.Mutex
	state        State
	failures     int
	threshold    int
	resetTimeout time.Duration
	lastFailure  time.Time
}

// New creates a circuit breaker that opens after threshold consecutive failures
// and resets after resetTimeout.
func New(threshold int, resetTimeout time.Duration) *Breaker {
	return &Breaker{
		threshold:    threshold,
		resetTimeout: resetTimeout,
	}
}

// State returns the current circuit state.
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.currentState()
}

func (b *Breaker) currentState() State {
	if b.state == Open && time.Since(b.lastFailure) > b.resetTimeout {
		return HalfOpen
	}
	return b.state
}

// Allow returns true if a request can proceed.
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.currentState() {
	case Open:
		return false
	case HalfOpen:
		return true // allow one test request
	default:
		return true
	}
}

// RecordSuccess records a successful call.
func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	b.state = Closed
}

// RecordFailure records a failed call.
func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures++
	b.lastFailure = time.Now()
	if b.failures >= b.threshold {
		b.state = Open
	}
}
