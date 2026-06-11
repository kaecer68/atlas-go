package marketdata

import (
	"sync"
	"time"
)

// ProviderBreakerInfo is a read-only snapshot of a providerBreaker's state.
type ProviderBreakerInfo struct {
	Name                   string
	State                  ProviderCircuitState
	FailureCount           int
	Threshold              int
	LastFailure            time.Time
	HalfOpenCallsRemaining int
}

type providerBreaker struct {
	name          string
	config        circuitBreakerConfig
	mu            sync.RWMutex
	state         ProviderCircuitState
	failureCount  int
	lastFailure   time.Time
	halfOpenCalls int
}

func newProviderBreaker(name string, cfg circuitBreakerConfig) *providerBreaker {
	return &providerBreaker{
		name:   name,
		config: cfg,
		state:  ProviderCircuitClosed,
	}
}

func (b *providerBreaker) shouldTry() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case ProviderCircuitClosed, ProviderCircuitHalfOpen:
		return true
	case ProviderCircuitOpen:
		if time.Since(b.lastFailure) > b.config.recoveryTimeout {
			b.state = ProviderCircuitHalfOpen
			b.halfOpenCalls = 0
			return true
		}
		return false
	}
	return false
}

func (b *providerBreaker) recordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failureCount = 0
	b.halfOpenCalls = 0
	b.state = ProviderCircuitClosed
}

func (b *providerBreaker) recordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failureCount++
	b.lastFailure = time.Now()
	if b.state == ProviderCircuitHalfOpen {
		b.state = ProviderCircuitOpen
		return
	}
	if b.failureCount >= b.config.failureThreshold {
		b.state = ProviderCircuitOpen
	}
}

func (b *providerBreaker) stateSnapshot() ProviderBreakerInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return ProviderBreakerInfo{
		Name:                   b.name,
		State:                  b.state,
		FailureCount:           b.failureCount,
		Threshold:              b.config.failureThreshold,
		LastFailure:            b.lastFailure,
		HalfOpenCallsRemaining: b.config.halfOpenMaxCalls - b.halfOpenCalls,
	}
}

func (b *providerBreaker) forceState(s ProviderCircuitState) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.state = s
	if s == ProviderCircuitClosed {
		b.failureCount = 0
		b.halfOpenCalls = 0
	}
	if s == ProviderCircuitOpen {
		b.lastFailure = time.Now()
	}
}

func (b *providerBreaker) reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.state = ProviderCircuitClosed
	b.failureCount = 0
	b.lastFailure = time.Time{}
	b.halfOpenCalls = 0
}
