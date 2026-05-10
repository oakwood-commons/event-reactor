package circuitbreaker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBreaker_ClosedByDefault(t *testing.T) {
	b := New(3, time.Second)
	assert.Equal(t, Closed, b.State())
	assert.True(t, b.Allow())
}

func TestBreaker_OpensAfterThreshold(t *testing.T) {
	b := New(3, time.Second)
	b.RecordFailure()
	b.RecordFailure()
	assert.True(t, b.Allow()) // still closed
	b.RecordFailure()
	assert.Equal(t, Open, b.State())
	assert.False(t, b.Allow())
}

func TestBreaker_HalfOpenAfterTimeout(t *testing.T) {
	b := New(2, 10*time.Millisecond)
	b.RecordFailure()
	b.RecordFailure()
	assert.False(t, b.Allow())

	time.Sleep(15 * time.Millisecond)
	assert.Equal(t, HalfOpen, b.State())
	assert.True(t, b.Allow())
}

func TestBreaker_ResetsOnSuccess(t *testing.T) {
	b := New(2, 10*time.Millisecond)
	b.RecordFailure()
	b.RecordFailure()

	time.Sleep(15 * time.Millisecond)
	b.RecordSuccess()
	assert.Equal(t, Closed, b.State())
	assert.True(t, b.Allow())
}
