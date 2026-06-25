package apigateway

import (
	"fmt"
	"sync"
	"time"
)

const (
	// CircuitBreakerFailureThreshold is the number of consecutive failures before opening.
	CircuitBreakerFailureThreshold = 3
	// CircuitBreakerRecoveryTimeout is the duration before attempting recovery.
	CircuitBreakerRecoveryTimeout = 5 * time.Minute
	// CircuitBreakerHalfOpenMaxCalls is the maximum test calls in half-open state.
	CircuitBreakerHalfOpenMaxCalls = 2
)

// State represents the circuit breaker state.
type State int

const (
	StateClosed   State = iota // Normal operation
	StateOpen                  // Failing, reject calls
	StateHalfOpen              // Testing if recovered
)

// CircuitBreaker tracks failures and controls access to a channel.
type CircuitBreaker struct {
	mu            sync.RWMutex
	state         State
	failures      int
	lastFailure   time.Time
	halfOpenCalls int
	channelID     string
	maxFailures   int
	// manualOverride, when true, prevents successes from auto-closing
	// the breaker. This is the "force open until an operator resets"
	// behavior used by llm_annotator's budget callback. The apigateway
	// channel breakers do not set this directly; SetManualOverride is
	// the explicit entry point for callers that need this guarantee.
	manualOverride bool
}

// NewCircuitBreaker creates a breaker for a channel.
// maxFailures is optional; defaults to CircuitBreakerFailureThreshold.
func NewCircuitBreaker(channelID string, maxFailures ...int) *CircuitBreaker {
	threshold := CircuitBreakerFailureThreshold
	if len(maxFailures) > 0 && maxFailures[0] > 0 {
		threshold = maxFailures[0]
	}
	return &CircuitBreaker{
		channelID:   channelID,
		state:       StateClosed,
		maxFailures: threshold,
	}
}

// Call executes fn if the circuit allows it.
func (cb *CircuitBreaker) Call(fn func() error) error {
	cb.mu.Lock()

	if cb.state == StateOpen {
		if time.Since(cb.lastFailure) < CircuitBreakerRecoveryTimeout {
			cb.mu.Unlock()
			return fmt.Errorf("circuit breaker open for channel %s", cb.channelID)
		}
		cb.state = StateHalfOpen
		cb.halfOpenCalls = 0
	}

	if cb.state == StateHalfOpen && cb.halfOpenCalls >= CircuitBreakerHalfOpenMaxCalls {
		cb.mu.Unlock()
		return fmt.Errorf("circuit breaker half-open limit reached for %s", cb.channelID)
	}

	if cb.state == StateHalfOpen {
		cb.halfOpenCalls++
	}

	cb.mu.Unlock()

	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.failures++
		cb.lastFailure = time.Now()
		if cb.failures >= cb.maxFailures {
			cb.state = StateOpen
		}
		return err
	}

	cb.failures = 0
	if !cb.manualOverride {
		cb.state = StateClosed
		cb.halfOpenCalls = 0
	}
	return nil
}

// State returns the current breaker state.
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// IsOpen reports whether the circuit is open.
func (cb *CircuitBreaker) IsOpen() bool {
	return cb.State() == StateOpen
}

// ForceOpen manually opens the circuit breaker regardless of failure count.
// Used for crisis-driven override (e.g., VIX ≥ 35 detected by MacroRiskAssessment).
func (cb *CircuitBreaker) ForceOpen() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = StateOpen
	cb.lastFailure = time.Now()
	cb.failures = cb.maxFailures
}

// SetManualOverride enables or disables manual override mode. When enabled,
// successful calls no longer auto-close the breaker — only Reset() does.
// Designed for budget callbacks (llm_annotator) where "force open until
// the operator decides the budget is restored" is the correct semantic.
func (cb *CircuitBreaker) SetManualOverride(enabled bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.manualOverride = enabled
}

// Reset returns the breaker to StateClosed and clears manual override.
// Use this to release a forced-open breaker once the operator decides
// the upstream dependency is healthy again.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = StateClosed
	cb.failures = 0
	cb.halfOpenCalls = 0
	cb.manualOverride = false
}

// CircuitBreakerManager manages breakers for all channels.
type CircuitBreakerManager struct {
	breakers map[string]*CircuitBreaker
	mu       sync.RWMutex
}

// NewCircuitBreakerManager creates a manager with breakers for all channels
// using the default failure threshold.
func NewCircuitBreakerManager(channelIDs []string) *CircuitBreakerManager {
	return NewCircuitBreakerManagerWithThresholds(nil, CircuitBreakerFailureThreshold, channelIDs)
}

// NewCircuitBreakerManagerWithThresholds creates a manager with per-channel
// failure thresholds. channelThresholds maps channel ID to its maxFailures.
// Channels not in the map use defaultThreshold.
func NewCircuitBreakerManagerWithThresholds(channelThresholds map[string]int, defaultThreshold int, channelIDs []string) *CircuitBreakerManager {
	m := &CircuitBreakerManager{
		breakers: make(map[string]*CircuitBreaker),
	}
	for _, id := range channelIDs {
		threshold := defaultThreshold
		if t, ok := channelThresholds[id]; ok {
			threshold = t
		}
		m.breakers[id] = NewCircuitBreaker(id, threshold)
	}
	return m
}

// Get returns the breaker for a channel.
func (m *CircuitBreakerManager) Get(channelID string) (*CircuitBreaker, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cb, ok := m.breakers[channelID]
	if !ok {
		return nil, fmt.Errorf("unknown channel: %s", channelID)
	}
	return cb, nil
}

// ForceOpen forces a breaker open by channel ID. Returns error if channel unknown.
func (m *CircuitBreakerManager) ForceOpen(channelID string) error {
	m.mu.RLock()
	cb, ok := m.breakers[channelID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("unknown channel: %s", channelID)
	}
	cb.ForceOpen()
	return nil
}

// Status returns breaker states for all channels.
func (m *CircuitBreakerManager) Status() map[string]CircuitBreakerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]CircuitBreakerStatus)
	for id, cb := range m.breakers {
		cb.mu.RLock()
		result[id] = CircuitBreakerStatus{
			ChannelID: id,
			State:     stateString(cb.state),
			Failures:  cb.failures,
			LastError: cb.lastFailure,
		}
		cb.mu.RUnlock()
	}
	return result
}

// CircuitBreakerStatus represents the current breaker state.
type CircuitBreakerStatus struct {
	ChannelID string    `json:"channel_id"`
	State     string    `json:"state"`
	Failures  int       `json:"failures"`
	LastError time.Time `json:"last_error"`
}

func stateString(s State) string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}
