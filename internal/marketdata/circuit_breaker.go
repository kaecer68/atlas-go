package marketdata

import (
	"sync"
	"time"
)

type ProviderCircuitState string

const (
	ProviderCircuitClosed   ProviderCircuitState = "closed"
	ProviderCircuitOpen     ProviderCircuitState = "open"
	ProviderCircuitHalfOpen ProviderCircuitState = "half-open"
)

type circuitBreakerConfig struct {
	failureThreshold int
	recoveryTimeout  time.Duration
	halfOpenMaxCalls int
}

func defaultCircuitBreakerConfig() circuitBreakerConfig {
	return circuitBreakerConfig{
		failureThreshold: 3,
		recoveryTimeout:  5 * time.Minute,
		halfOpenMaxCalls: 2,
	}
}

// CircuitBreaker 實作獨立的斷路器邏輯，每個資料提供者應擁有自己的實例。
//
// 狀態轉換流程：
//
//	Closed --(連續失敗達 threshold)--> Open --(超過 recoveryTimeout)--> HalfOpen --(成功達 halfOpenMaxCalls)--> Closed
//	HalfOpen --(失敗)--> Open
type CircuitBreaker struct {
	state         ProviderCircuitState
	failureCount  int
	lastFailure   time.Time
	halfOpenCalls int
	config        circuitBreakerConfig
	mu            sync.RWMutex
}

// NewCircuitBreaker 建立一個處於 Closed 狀態的新斷路器。
func NewCircuitBreaker(config circuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		state:  ProviderCircuitClosed,
		config: config,
	}
}

// Allow 檢查當前是否允許發起請求。
//
// - Closed：永遠允許
// - HalfOpen：僅允許最多 halfOpenMaxCalls 次探測請求
// - Open：若已超過 recoveryTimeout，自動轉為 HalfOpen 並允許首次探測
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case ProviderCircuitClosed:
		return true
	case ProviderCircuitHalfOpen:
		if cb.halfOpenCalls < cb.config.halfOpenMaxCalls {
			cb.halfOpenCalls++
			return true
		}
		return false
	case ProviderCircuitOpen:
		if time.Since(cb.lastFailure) > cb.config.recoveryTimeout {
			cb.state = ProviderCircuitHalfOpen
			cb.halfOpenCalls = 1
			return true
		}
		return false
	}
	return false
}

// RecordSuccess 記錄一次成功響應，並在條件滿足時將 HalfOpen 轉為 Closed。
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == ProviderCircuitHalfOpen {
		if cb.halfOpenCalls >= cb.config.halfOpenMaxCalls {
			cb.state = ProviderCircuitClosed
			cb.failureCount = 0
			cb.halfOpenCalls = 0
		}
		return
	}

	if cb.state == ProviderCircuitClosed {
		cb.failureCount = 0
	}
}

// RecordFailure 記錄一次失敗響應，並在條件滿足時打開斷路器。
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++
	cb.lastFailure = time.Now()

	if cb.state == ProviderCircuitHalfOpen {
		cb.state = ProviderCircuitOpen
		return
	}

	if cb.failureCount >= cb.config.failureThreshold {
		cb.state = ProviderCircuitOpen
	}
}

// State 返回當前斷路器狀態。
func (cb *CircuitBreaker) State() ProviderCircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Reset 將斷路器重設為 Closed 狀態。
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = ProviderCircuitClosed
	cb.failureCount = 0
	cb.halfOpenCalls = 0
	cb.lastFailure = time.Time{}
}

// ForceOpen 強制將斷路器設為 Open 狀態。
func (cb *CircuitBreaker) ForceOpen() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = ProviderCircuitOpen
	cb.lastFailure = time.Now()
}

// Stats 返回斷路器內部統計資料。
func (cb *CircuitBreaker) Stats() map[string]interface{} {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return map[string]interface{}{
		"state":             string(cb.state),
		"failure_count":     cb.failureCount,
		"failure_threshold": cb.config.failureThreshold,
		"last_failure":      cb.lastFailure.Format(time.RFC3339),
	}
}
