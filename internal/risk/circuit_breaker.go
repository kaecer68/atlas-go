package risk

import (
	"fmt"
	"sync"
	"time"
)

// CircuitBreaker monitors daily portfolio returns and trips when risk thresholds are
// exceeded, suspending all trading until manually reset or auto-recovery.
//
// Two trip conditions:
//   - A single-day loss exceeding maxDailyLossPct (e.g., -5%)
//   - Consecutive losing days reaching maxConsecutiveLossDays (e.g., 3)
//
// Once tripped, the breaker stays tripped until Reset() is called or the auto-reset
// duration elapses (T+1). While tripped, the risk gate should be set to ModeSuspended.
type CircuitBreaker struct {
	mu sync.RWMutex

	maxDailyLossPct       float64
	maxConsecutiveLossDays int

	consecutiveLosses int
	tripped           bool
	trippedAt         time.Time
	autoResetAfter    time.Duration
}

// NewCircuitBreaker creates a CircuitBreaker with the given thresholds.
// maxDailyLossPct is expressed as a positive fraction (e.g., 0.05 = 5%).
// maxConsecutiveLossDays is the number of consecutive losing days to trigger a trip.
// Default auto-reset is 24 hours.
func NewCircuitBreaker(maxDailyLossPct float64, maxConsecutiveLossDays int) *CircuitBreaker {
	return &CircuitBreaker{
		maxDailyLossPct:       maxDailyLossPct,
		maxConsecutiveLossDays: maxConsecutiveLossDays,
		autoResetAfter:        24 * time.Hour,
	}
}

// WithAutoResetAfter sets a custom auto-reset duration.
func (cb *CircuitBreaker) WithAutoResetAfter(d time.Duration) *CircuitBreaker {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.autoResetAfter = d
	return cb
}

// Check evaluates the day's return against trip thresholds.
// dayReturn is the fractional daily return (e.g., -0.06 = -6%).
// Returns (true, reason) if the breaker tripped, (false, "") otherwise.
// If the breaker is already tripped and auto-reset has elapsed, it auto-recovers
// before evaluating.
func (cb *CircuitBreaker) Check(dayReturn float64) (tripped bool, reason string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Auto-reset if enough time has passed since trip.
	if cb.tripped && time.Since(cb.trippedAt) >= cb.autoResetAfter {
		cb.tripped = false
		cb.consecutiveLosses = 0
		cb.trippedAt = time.Time{}
	}

	// Already tripped — no further evaluation needed.
	if cb.tripped {
		return true, fmt.Sprintf("circuit breaker remains tripped since %s", cb.trippedAt.Format(time.RFC3339))
	}

	// Track consecutive losses.
	if dayReturn < 0 {
		cb.consecutiveLosses++
	} else {
		cb.consecutiveLosses = 0
	}

	// Check single-day loss threshold.
	if dayReturn < 0 && -dayReturn >= cb.maxDailyLossPct {
		cb.tripped = true
		cb.trippedAt = time.Now()
		return true, fmt.Sprintf(
			"daily loss %.2f%% exceeds %.2f%% threshold",
			-dayReturn*100, cb.maxDailyLossPct*100,
		)
	}

	// Check consecutive loss days threshold.
	if cb.consecutiveLosses >= cb.maxConsecutiveLossDays {
		cb.tripped = true
		cb.trippedAt = time.Now()
		cb.consecutiveLosses = 0
		return true, fmt.Sprintf(
			"%d consecutive losing days (limit %d)",
			cb.maxConsecutiveLossDays, cb.maxConsecutiveLossDays,
		)
	}

	return false, ""
}

// Reset manually clears the tripped state and resets the consecutive loss counter.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.tripped = false
	cb.consecutiveLosses = 0
	cb.trippedAt = time.Time{}
}

// IsTripped returns true if the circuit breaker is in tripped state.
func (cb *CircuitBreaker) IsTripped() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.tripped
}

// TrippedAt returns the time the breaker tripped, or zero time if not tripped.
func (cb *CircuitBreaker) TrippedAt() time.Time {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.trippedAt
}

// ConsecutiveLosses returns the current consecutive loss count.
func (cb *CircuitBreaker) ConsecutiveLosses() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.consecutiveLosses
}
