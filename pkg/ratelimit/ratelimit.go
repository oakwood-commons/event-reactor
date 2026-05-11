// Package ratelimit provides a token-bucket rate limiter for event processing.
package ratelimit

import (
	"sync"
	"time"
)

// Limiter implements a token-bucket rate limiter.
type Limiter struct {
	mu       sync.Mutex
	rate     float64 // tokens per second
	burst    int     // max bucket size
	tokens   float64
	lastTime time.Time
}

// New creates a rate limiter with the given rate (events/sec) and burst size.
func New(rate float64, burst int) *Limiter {
	return &Limiter{
		rate:     rate,
		burst:    burst,
		tokens:   float64(burst),
		lastTime: time.Now(),
	}
}

// Allow returns true if an event can proceed, consuming one token.
func (l *Limiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(l.lastTime).Seconds()
	l.lastTime = now

	l.tokens += elapsed * l.rate
	if l.tokens > float64(l.burst) {
		l.tokens = float64(l.burst)
	}

	if l.tokens < 1 {
		return false
	}
	l.tokens--
	return true
}
