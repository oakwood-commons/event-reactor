package ratelimit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLimiter_AllowBurst(t *testing.T) {
	l := New(10, 5)

	// Should allow up to burst
	for i := 0; i < 5; i++ {
		assert.True(t, l.Allow(), "token %d should be allowed", i)
	}
	// Next should be denied
	assert.False(t, l.Allow())
}

func TestLimiter_RefillsOverTime(t *testing.T) {
	l := New(100, 1) // 100/sec, burst 1

	assert.True(t, l.Allow())
	assert.False(t, l.Allow())

	// Wait for refill
	time.Sleep(15 * time.Millisecond)
	assert.True(t, l.Allow())
}
