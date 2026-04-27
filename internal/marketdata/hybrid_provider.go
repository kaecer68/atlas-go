package marketdata

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// CircuitState represents the state of the circuit breaker.
type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"   // Normal operation, requests pass through
	CircuitOpen     CircuitState = "open"     // Failing, requests blocked
	CircuitHalfOpen CircuitState = "half-open" // Testing if service recovered
)

// circuitBreakerConfig holds the configuration for the circuit breaker.
type circuitBreakerConfig struct {
	failureThreshold int           // Number of failures before opening circuit
	recoveryTimeout  time.Duration // Time before attempting recovery
	halfOpenMaxCalls int           // Max calls allowed in half-open state
}

// defaultCircuitBreakerConfig returns sensible defaults.
func defaultCircuitBreakerConfig() circuitBreakerConfig {
	return circuitBreakerConfig{
		failureThreshold: 3,
		recoveryTimeout:  5 * time.Minute,
		halfOpenMaxCalls: 2,
	}
}

// HybridProvider 混合数据源 Provider
// 优先使用 Fugle（实时），失败时回退到 TWSE OpenAPI（免费但 rate limited）
// 使用 Circuit Breaker 模式防止永久切换，支持自动恢复
type HybridProvider struct {
	fugleProvider *FugleProvider
	twseClient    *TWSEClient

	// Circuit breaker state
	cbState          CircuitState
	cbFailureCount   int
	cbLastFailure    time.Time
	cbHalfOpenCalls  int
	cbConfig         circuitBreakerConfig
	cbMutex          sync.RWMutex

	// Fallback tracking for observability
	fallbackCount    int
	lastFallbackAt   time.Time
	recoveryAttempts int
}

// NewHybridProvider 创建混合 Provider
// apiKey: Fugle API key，如果为空或失败则回退到 TWSE
func NewHybridProvider(apiKey string) *HybridProvider {
	var fugleProvider *FugleProvider
	if apiKey != "" {
		fugleProvider = NewFugleProviderWithAPIKey(apiKey)
	}

	return &HybridProvider{
		fugleProvider: fugleProvider,
		twseClient:    NewTWSEClient(),
		cbState:       CircuitClosed,
		cbConfig:      defaultCircuitBreakerConfig(),
	}
}

// Name 返回 Provider 名称
func (p *HybridProvider) Name() string {
	if p.isCircuitOpen() || p.fugleProvider == nil {
		return "hybrid-twse"
	}
	return "hybrid-fugle"
}

// GetQuotes 获取行情，优先 Fugle，失败时回退 TWSE
// 使用 Circuit Breaker 防止永久切换，支持自动恢复
func (p *HybridProvider) GetQuotes(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error) {
	// If no Fugle provider, always use TWSE
	if p.fugleProvider == nil {
		return p.getQuotesFromTWSE(ctx, symbols)
	}

	// Check if circuit breaker allows Fugle attempt
	if p.shouldTryFugle() {
		quotes, err := p.tryFugle(ctx, asOf, symbols)
		if err == nil && len(quotes) > 0 && !p.hasInvalidQuotes(quotes) {
			return quotes, nil
		}
		// Fugle failed or returned invalid data, record failure and fallback
		p.recordFugleFailure()
		fmt.Printf("[HybridProvider] Fugle failed (%v), falling back to TWSE (circuit: %s, failures: %d)\n",
			err, p.getCircuitState(), p.getFailureCount())
	}

	// Fallback to TWSE
	return p.getQuotesFromTWSE(ctx, symbols)
}

// shouldTryFugle returns true if the circuit breaker allows a Fugle attempt.
func (p *HybridProvider) shouldTryFugle() bool {
	p.cbMutex.Lock()
	defer p.cbMutex.Unlock()

	switch p.cbState {
	case CircuitClosed:
		return true
	case CircuitOpen:
		// Check if recovery timeout has elapsed
		if time.Since(p.cbLastFailure) > p.cbConfig.recoveryTimeout {
			p.cbState = CircuitHalfOpen
			p.cbHalfOpenCalls = 0
			fmt.Printf("[HybridProvider] Circuit breaker entering half-open state, testing Fugle recovery\n")
			return true
		}
		return false
	case CircuitHalfOpen:
		// Allow limited calls in half-open state
		if p.cbHalfOpenCalls < p.cbConfig.halfOpenMaxCalls {
			p.cbHalfOpenCalls++
			return true
		}
		return false
	}
	return false
}

// tryFugle attempts to get quotes from Fugle.
func (p *HybridProvider) tryFugle(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error) {
	quotes, err := p.fugleProvider.GetQuotes(ctx, asOf, symbols)
	if err != nil {
		return nil, err
	}
	if len(quotes) == 0 || p.hasInvalidQuotes(quotes) {
		return quotes, fmt.Errorf("fugle returned invalid/empty data")
	}
	return quotes, nil
}

// recordFugleFailure records a failure and potentially opens the circuit.
func (p *HybridProvider) recordFugleFailure() {
	p.cbMutex.Lock()
	defer p.cbMutex.Unlock()

	p.cbFailureCount++
	p.cbLastFailure = time.Now()
	p.fallbackCount++
	p.lastFallbackAt = time.Now()

	if p.cbState == CircuitHalfOpen {
		// Failure in half-open state -> back to open
		p.cbState = CircuitOpen
		p.cbHalfOpenCalls = 0
		fmt.Printf("[HybridProvider] Fugle recovery failed in half-open state, circuit re-opened\n")
	} else if p.cbFailureCount >= p.cbConfig.failureThreshold {
		// Too many failures -> open circuit
		p.cbState = CircuitOpen
		fmt.Printf("[HybridProvider] Circuit breaker opened after %d consecutive failures\n", p.cbFailureCount)
	}
}

// recordFugleSuccess records a successful Fugle call and resets the circuit.
func (p *HybridProvider) recordFugleSuccess() {
	p.cbMutex.Lock()
	defer p.cbMutex.Unlock()

	if p.cbState != CircuitClosed {
		fmt.Printf("[HybridProvider] Fugle recovered, circuit breaker closed\n")
		p.recoveryAttempts++
	}
	p.cbState = CircuitClosed
	p.cbFailureCount = 0
	p.cbHalfOpenCalls = 0
}

// isCircuitOpen returns true if the circuit is currently open.
func (p *HybridProvider) isCircuitOpen() bool {
	p.cbMutex.RLock()
	defer p.cbMutex.RUnlock()
	return p.cbState == CircuitOpen
}

// getCircuitState returns the current circuit state (for logging).
func (p *HybridProvider) getCircuitState() CircuitState {
	p.cbMutex.RLock()
	defer p.cbMutex.RUnlock()
	return p.cbState
}

// getFailureCount returns the current failure count (for logging).
func (p *HybridProvider) getFailureCount() int {
	p.cbMutex.RLock()
	defer p.cbMutex.RUnlock()
	return p.cbFailureCount
}

// getQuotesFromTWSE 从 TWSE 获取行情
func (p *HybridProvider) getQuotesFromTWSE(ctx context.Context, symbols []string) ([]domain.Quote, error) {
	if len(symbols) == 1 {
		quote, err := p.twseClient.GetQuote(ctx, symbols[0])
		if err != nil {
			return nil, err
		}
		return []domain.Quote{quote}, nil
	}

	return p.twseClient.GetQuotesBySymbols(ctx, symbols)
}

// hasInvalidQuotes 检查是否有无效的行情数据（如价格为 0）
func (p *HybridProvider) hasInvalidQuotes(quotes []domain.Quote) bool {
	for _, q := range quotes {
		if q.Last == 0 && q.Open == 0 && q.High == 0 && q.Low == 0 {
			return true
		}
		if q.Last < 0 || q.Open < 0 || q.High < 0 || q.Low < 0 {
			return true
		}
		if q.Volume < 0 {
			return true
		}
	}
	return false
}

// Reset 重置 Provider 状态（重新尝试 Fugle）
func (p *HybridProvider) Reset() {
	p.cbMutex.Lock()
	defer p.cbMutex.Unlock()
	if p.fugleProvider == nil {
		p.cbState = CircuitOpen
	} else {
		p.cbState = CircuitClosed
	}
	p.cbFailureCount = 0
	p.cbHalfOpenCalls = 0
	p.fallbackCount = 0
	p.recoveryAttempts = 0
}

// UseTWSE 强制使用 TWSE（忽略 Fugle）
func (p *HybridProvider) UseTWSE() {
	p.cbMutex.Lock()
	defer p.cbMutex.Unlock()
	p.cbState = CircuitOpen
	p.cbLastFailure = time.Now()
}

// UseFugle 强制使用 Fugle（如果配置了）
func (p *HybridProvider) UseFugle() {
	p.cbMutex.Lock()
	defer p.cbMutex.Unlock()
	p.cbState = CircuitClosed
	p.cbFailureCount = 0
	p.cbHalfOpenCalls = 0
}

// GetClient 获取底层客户端（用于直接访问）
func (p *HybridProvider) GetTWSEClient() *TWSEClient {
	return p.twseClient
}

func (p *HybridProvider) GetFugleClient() *FugleClient {
	if p.fugleProvider == nil {
		return nil
	}
	return p.fugleProvider.GetClient()
}

// IsUsingTWSE 返回当前是否使用 TWSE
func (p *HybridProvider) IsUsingTWSE() bool {
	return p.isCircuitOpen()
}

// CircuitBreakerStats returns current circuit breaker statistics for observability.
func (p *HybridProvider) CircuitBreakerStats() map[string]interface{} {
	p.cbMutex.RLock()
	defer p.cbMutex.RUnlock()
	return map[string]interface{}{
		"state":             string(p.cbState),
		"failure_count":     p.cbFailureCount,
		"failure_threshold": p.cbConfig.failureThreshold,
		"last_failure":      p.cbLastFailure.Format(time.RFC3339),
		"fallback_count":    p.fallbackCount,
		"last_fallback":     p.lastFallbackAt.Format(time.RFC3339),
		"recovery_attempts": p.recoveryAttempts,
	}
}
