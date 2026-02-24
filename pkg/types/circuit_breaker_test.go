package types

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCircuitBreaker_InitialState(t *testing.T) {
	cb := NewCircuitBreaker(3, 5*time.Second)

	assert.Equal(t, CircuitClosed, cb.State())
	assert.Equal(t, 0, cb.Failures())
	assert.True(t, cb.Allow())
}

func TestCircuitBreaker_TransitionToOpen(t *testing.T) {
	cb := NewCircuitBreaker(3, 5*time.Second)

	// Record failures up to threshold
	cb.RecordFailure()
	assert.Equal(t, CircuitClosed, cb.State())
	assert.Equal(t, 1, cb.Failures())

	cb.RecordFailure()
	assert.Equal(t, CircuitClosed, cb.State())
	assert.Equal(t, 2, cb.Failures())

	cb.RecordFailure()
	assert.Equal(t, CircuitOpen, cb.State())
	assert.False(t, cb.Allow())
}

func TestCircuitBreaker_SuccessResetsFailures(t *testing.T) {
	cb := NewCircuitBreaker(3, 5*time.Second)

	cb.RecordFailure()
	cb.RecordFailure()
	assert.Equal(t, 2, cb.Failures())

	cb.RecordSuccess()
	assert.Equal(t, 0, cb.Failures())
	assert.Equal(t, CircuitClosed, cb.State())
}

func TestCircuitBreaker_HalfOpenTransition(t *testing.T) {
	cb := NewCircuitBreaker(3, 1*time.Millisecond)

	// Trip the breaker
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	assert.Equal(t, CircuitOpen, cb.State())

	// Wait for timeout
	time.Sleep(5 * time.Millisecond)

	// Should transition to half-open
	assert.True(t, cb.Allow())
	assert.Equal(t, CircuitHalfOpen, cb.State())
}

func TestCircuitBreaker_HalfOpenToClosedOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker(3, 1*time.Millisecond)

	// Trip and wait
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(5 * time.Millisecond)

	// Transition to half-open
	cb.Allow()
	assert.Equal(t, CircuitHalfOpen, cb.State())

	// Two successes needed (halfOpenMax = 2)
	cb.RecordSuccess()
	assert.Equal(t, CircuitHalfOpen, cb.State())

	cb.RecordSuccess()
	assert.Equal(t, CircuitClosed, cb.State())
}

func TestCircuitBreaker_HalfOpenToOpenOnFailure(t *testing.T) {
	cb := NewCircuitBreaker(3, 1*time.Millisecond)

	// Trip and wait
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(5 * time.Millisecond)

	// Transition to half-open
	cb.Allow()
	assert.Equal(t, CircuitHalfOpen, cb.State())

	// Failure goes back to open
	cb.RecordFailure()
	assert.Equal(t, CircuitOpen, cb.State())
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(3, 5*time.Second)

	// Trip the breaker
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	assert.Equal(t, CircuitOpen, cb.State())

	// Reset
	cb.Reset()
	assert.Equal(t, CircuitClosed, cb.State())
	assert.Equal(t, 0, cb.Failures())
	assert.True(t, cb.Allow())
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	cb := NewCircuitBreaker(100, 5*time.Second)

	done := make(chan struct{})
	for i := 0; i < 50; i++ {
		go func() {
			cb.Allow()
			cb.RecordFailure()
			cb.RecordSuccess()
			cb.State()
			cb.Failures()
			done <- struct{}{}
		}()
	}

	for i := 0; i < 50; i++ {
		<-done
	}

	// Should not panic or deadlock
	assert.NotNil(t, cb)
}
