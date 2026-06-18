package llm_annotator

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// Circuit breaker defaults for the LLM annotator. The thresholds mirror
// internal/apigateway/circuitbreaker.go but are local copies because the
// apigateway package transitively imports llm_annotator via monitoring,
// creating an import cycle.
const (
	cbFailureThreshold = 3
	cbRecoveryTimeout  = 5 * time.Minute
	cbHalfOpenMaxCalls = 2
)

// ErrCircuitOpen is returned by Annotate when the breaker rejects a call.
// Callers should treat it like ErrUnavailable — fallback to rule-based
// attribution.
var ErrCircuitOpen = errors.New("llm annotator circuit breaker open")

// CircuitState is the breaker's high-level state.
type CircuitState int

const (
	CircuitClosed   CircuitState = iota // normal operation
	CircuitOpen                         // reject calls until recovery timeout
	CircuitHalfOpen                     // allow a limited number of test calls
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker tracks consecutive failures of the LLM annotator. After
// cbFailureThreshold consecutive failures the breaker opens for
// cbRecoveryTimeout, after which it transitions to half-open and lets
// cbHalfOpenMaxCalls test calls through. A single success closes the
// breaker; a single failure reopens it.
//
// ForceOpen sets manualOverride, which prevents subsequent successes
// from auto-closing the breaker — the budget callback's "stop calling
// the LLM" decision persists until an operator calls Reset.
//
// NOTE: This is a local implementation rather than a reuse of
// internal/apigateway/circuitbreaker.go because of an unavoidable
// import cycle: monitoring.dashboard_api imports llm_annotator, and
// monitoring.api.scheduler imports apigateway, so llm_annotator cannot
// import apigateway. The local type also has a smaller surface
// (Reset, manualOverride, ErrCircuitOpen sentinel) that the apigateway
// type does not need but this client does.
type CircuitBreaker struct {
	mu             sync.RWMutex
	state          CircuitState
	failures       int
	lastFailure    time.Time
	halfOpenCalls  int
	now            func() time.Time
	manualOverride bool
}

func newCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{state: CircuitClosed, now: time.Now}
}

// State returns the current breaker state. Safe for any goroutine.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// IsOpen reports whether the breaker is in the open state.
func (cb *CircuitBreaker) IsOpen() bool {
	return cb.State() == CircuitOpen
}

// ForceOpen manually opens the breaker. The recovery timeout starts now
// and manualOverride is set so that subsequent successes do NOT
// auto-close the breaker — the budget callback's "stop calling the LLM"
// decision persists until an operator calls Reset.
func (cb *CircuitBreaker) ForceOpen() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = CircuitOpen
	cb.lastFailure = cb.now()
	cb.failures = cbFailureThreshold
	cb.manualOverride = true
}

// Reset moves the breaker back to the closed state and clears the manual
// override flag. Used by tests and when an operator manually clears the
// failure state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = CircuitClosed
	cb.failures = 0
	cb.halfOpenCalls = 0
	cb.manualOverride = false
}

// Call runs fn under the breaker. If the breaker is open and the recovery
// timeout has not elapsed, Call returns ErrCircuitOpen without invoking
// fn. If fn returns an error, the failure is recorded; if fn returns nil,
// the breaker is reset to closed.
func (cb *CircuitBreaker) Call(fn func() error) error {
	cb.mu.Lock()
	now := cb.now()

	if cb.state == CircuitOpen {
		if now.Sub(cb.lastFailure) < cbRecoveryTimeout {
			cb.mu.Unlock()
			return fmt.Errorf("%w: recovery in %s",
				ErrCircuitOpen, cbRecoveryTimeout-now.Sub(cb.lastFailure))
		}
		cb.state = CircuitHalfOpen
		cb.halfOpenCalls = 0
	}

	if cb.state == CircuitHalfOpen && cb.halfOpenCalls >= cbHalfOpenMaxCalls {
		cb.mu.Unlock()
		return fmt.Errorf("%w: half-open limit reached", ErrCircuitOpen)
	}

	if cb.state == CircuitHalfOpen {
		cb.halfOpenCalls++
	}
	cb.mu.Unlock()

	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()
	if err != nil {
		cb.failures++
		cb.lastFailure = cb.now()
		if cb.failures >= cbFailureThreshold {
			cb.state = CircuitOpen
		}
		return err
	}
	cb.failures = 0
	if !cb.manualOverride {
		cb.state = CircuitClosed
		cb.halfOpenCalls = 0
	}
	return nil
}
