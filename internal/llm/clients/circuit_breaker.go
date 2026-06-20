package clients

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// Circuit breaker defaults. Mirrors internal/llm_annotator/circuit_breaker.go
// but is a local copy because the llm_annotator package transitively imports
// apigateway, which would create an import cycle.
const (
	failureThreshold = 3
	recoveryTimeout  = 5 * time.Minute
	halfOpenMaxCalls = 2
)

// ErrCircuitOpen is returned by BaseClient.DoRequest when the circuit
// breaker rejects a call. Callers should treat it as a transient error
// and fall back to another provider.
var ErrCircuitOpen = errors.New("circuit breaker open")

// circuitState is the breaker's high-level state.
type circuitState int

const (
	circuitClosed   circuitState = iota // normal operation
	circuitOpen                         // reject calls until recovery timeout
	circuitHalfOpen                     // allow a limited number of test calls
)

func (s circuitState) String() string {
	switch s {
	case circuitClosed:
		return "closed"
	case circuitOpen:
		return "open"
	case circuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker tracks consecutive failures. After failureThreshold
// consecutive failures the breaker opens for recoveryTimeout, after which
// it transitions to half-open and lets halfOpenMaxCalls test calls through.
// A single success closes the breaker; a single failure reopens it.
//
// ForceOpen sets manualOverride, which prevents subsequent successes from
// auto-closing the breaker — it persists until Reset is called.
type CircuitBreaker struct {
	mu             sync.RWMutex
	state          circuitState
	failures       int
	lastFailure    time.Time
	halfOpenCalls  int
	now            func() time.Time
	manualOverride bool
}

// callUnderLock is the function type passed to Call; it should perform the
// actual work (e.g., an HTTP request) that the breaker is protecting.
type callUnderLock func() error

// NewCircuitBreaker returns a fresh CircuitBreaker in the closed state.
func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{state: circuitClosed, now: time.Now}
}

// State returns the current breaker state as a human-readable string.
// Safe for any goroutine.
func (cb *CircuitBreaker) State() string {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state.String()
}

// ForceOpen manually opens the breaker. The recovery timeout starts now
// and manualOverride is set so that subsequent successes do NOT auto-close
// the breaker.
func (cb *CircuitBreaker) ForceOpen() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = circuitOpen
	cb.lastFailure = cb.now()
	cb.failures = failureThreshold
	cb.manualOverride = true
}

// Reset moves the breaker back to the closed state and clears the manual
// override flag. Used by tests and when an operator manually clears the
// failure state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = circuitClosed
	cb.failures = 0
	cb.halfOpenCalls = 0
	cb.manualOverride = false
}

// Call runs fn under the breaker. If the breaker is open and the recovery
// timeout has not elapsed, Call returns ErrCircuitOpen without invoking fn.
// If fn returns an error, the failure is recorded; if fn returns nil,
// the breaker is reset to closed (unless manualOverride is set).
func (cb *CircuitBreaker) Call(fn callUnderLock) error {
	cb.mu.Lock()
	now := cb.now()

	if cb.state == circuitOpen {
		if now.Sub(cb.lastFailure) < recoveryTimeout {
			cb.mu.Unlock()
			return fmt.Errorf("%w: recovery in %s",
				ErrCircuitOpen, recoveryTimeout-now.Sub(cb.lastFailure))
		}
		cb.state = circuitHalfOpen
		cb.halfOpenCalls = 0
	}

	if cb.state == circuitHalfOpen && cb.halfOpenCalls >= halfOpenMaxCalls {
		cb.mu.Unlock()
		return fmt.Errorf("%w: half-open limit reached", ErrCircuitOpen)
	}

	if cb.state == circuitHalfOpen {
		cb.halfOpenCalls++
	}
	cb.mu.Unlock()

	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()
	if err != nil {
		cb.failures++
		cb.lastFailure = cb.now()
		if cb.failures >= failureThreshold {
			cb.state = circuitOpen
		}
		return err
	}
	cb.failures = 0
	if !cb.manualOverride {
		cb.state = circuitClosed
		cb.halfOpenCalls = 0
	}
	return nil
}
