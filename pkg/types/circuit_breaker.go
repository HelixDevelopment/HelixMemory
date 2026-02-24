// Package types provides core types for the HelixMemory unified cognitive
// memory engine.
package types

import (
	"sync"
	"time"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	// CircuitClosed means the circuit is operating normally.
	CircuitClosed CircuitState = iota
	// CircuitOpen means the circuit has tripped and calls are rejected.
	CircuitOpen
	// CircuitHalfOpen means the circuit is testing if the backend recovered.
	CircuitHalfOpen
)

// CircuitBreaker implements the circuit breaker pattern for backend resilience.
type CircuitBreaker struct {
	mu              sync.RWMutex
	state           CircuitState
	failures        int
	threshold       int
	timeout         time.Duration
	lastFailure     time.Time
	lastStateChange time.Time
	halfOpenSuccess int
	halfOpenMax     int
}

// NewCircuitBreaker creates a circuit breaker with the given threshold and timeout.
func NewCircuitBreaker(threshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:           CircuitClosed,
		threshold:       threshold,
		timeout:         timeout,
		lastStateChange: time.Now(),
		halfOpenMax:     2,
	}
}

// Allow checks if a request should be allowed through the circuit.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		if time.Since(cb.lastFailure) > cb.timeout {
			cb.state = CircuitHalfOpen
			cb.halfOpenSuccess = 0
			cb.lastStateChange = time.Now()
			return true
		}
		return false
	case CircuitHalfOpen:
		return true
	default:
		return false
	}
}

// RecordSuccess records a successful operation.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitHalfOpen:
		cb.halfOpenSuccess++
		if cb.halfOpenSuccess >= cb.halfOpenMax {
			cb.state = CircuitClosed
			cb.failures = 0
			cb.lastStateChange = time.Now()
		}
	case CircuitClosed:
		cb.failures = 0
	}
}

// RecordFailure records a failed operation.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()

	switch cb.state {
	case CircuitClosed:
		if cb.failures >= cb.threshold {
			cb.state = CircuitOpen
			cb.lastStateChange = time.Now()
		}
	case CircuitHalfOpen:
		cb.state = CircuitOpen
		cb.lastStateChange = time.Now()
	}
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Reset resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = CircuitClosed
	cb.failures = 0
	cb.halfOpenSuccess = 0
	cb.lastStateChange = time.Now()
}

// Failures returns the current failure count.
func (cb *CircuitBreaker) Failures() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.failures
}
