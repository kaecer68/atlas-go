package forecast_bridge

import (
	"sync"
	"time"
)

// CircuitBreaker protects the forecast bridge from repeated failures.
// After 10 consecutive failures, the breaker opens for 1 hour, after
// which it auto-resets to half-open (allowing one probe attempt).
type CircuitBreaker struct {
	mu               sync.Mutex
	consecutiveFail  int
	openedAt         time.Time
	openDuration     time.Duration
	failureThreshold int
	nowFn            func() time.Time
}

// NewCircuitBreaker creates a breaker that opens after threshold consecutive
// failures and stays open for openFor duration.
func NewCircuitBreaker(threshold int, openFor time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		failureThreshold: threshold,
		openDuration:     openFor,
		nowFn:            time.Now,
	}
}

// DefaultCircuitBreaker creates a breaker with the M4 spec defaults:
// 10 consecutive failures, 1-hour open window.
func DefaultCircuitBreaker() *CircuitBreaker {
	return NewCircuitBreaker(10, 1*time.Hour)
}

// IsOpen returns true when the breaker has tripped and the cooldown has not elapsed.
func (cb *CircuitBreaker) IsOpen() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.consecutiveFail < cb.failureThreshold {
		return false
	}
	if cb.nowFn().Sub(cb.openedAt) > cb.openDuration {
		cb.consecutiveFail = 0
		return false
	}
	return true
}

// RecordSuccess resets the failure counter.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.consecutiveFail = 0
}

// RecordFailure increments the failure counter and opens the breaker
// when the threshold is reached.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.consecutiveFail++
	if cb.consecutiveFail >= cb.failureThreshold {
		cb.openedAt = cb.nowFn()
	}
}
