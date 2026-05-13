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
}

// NewCircuitBreaker creates a breaker for a channel.
func NewCircuitBreaker(channelID string) *CircuitBreaker {
	return &CircuitBreaker{
		channelID: channelID,
		state:     StateClosed,
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
		if cb.failures >= CircuitBreakerFailureThreshold {
			cb.state = StateOpen
		}
		return err
	}

	cb.failures = 0
	cb.state = StateClosed
	cb.halfOpenCalls = 0
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

// CircuitBreakerManager manages breakers for all channels.
type CircuitBreakerManager struct {
	breakers map[string]*CircuitBreaker
	mu       sync.RWMutex
}

// NewCircuitBreakerManager creates a manager with breakers for all channels.
func NewCircuitBreakerManager(channelIDs []string) *CircuitBreakerManager {
	m := &CircuitBreakerManager{
		breakers: make(map[string]*CircuitBreaker),
	}
	for _, id := range channelIDs {
		m.breakers[id] = NewCircuitBreaker(id)
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
