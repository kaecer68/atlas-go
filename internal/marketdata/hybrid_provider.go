package marketdata

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// ProviderCircuitState represents the state of the provider circuit breaker.
type ProviderCircuitState string

const (
	ProviderCircuitClosed   ProviderCircuitState = "closed"    // Normal operation, requests pass through
	ProviderCircuitOpen     ProviderCircuitState = "open"      // Failing, requests blocked
	ProviderCircuitHalfOpen ProviderCircuitState = "half-open" // Testing if service recovered
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
// 优先级：TWSE（免费） → FinMind（免费） → Fubon（免费需账户） → Fugle（付费）
// 使用 Circuit Breaker 模式防止永久切换，支持自动恢复
type HybridProvider struct {
	twseClient      *TWSEClient
	finmindProvider *FinMindProvider
	fubonProvider   *FubonProvider
	fugleProvider   *FugleProvider

	// Circuit breaker state for paid provider (Fugle)
	cbState         ProviderCircuitState
	cbFailureCount  int
	cbLastFailure   time.Time
	cbHalfOpenCalls int
	cbConfig        circuitBreakerConfig
	cbMutex         sync.RWMutex

	// Fallback tracking for observability
	fallbackCount    int
	lastFallbackAt   time.Time
	recoveryAttempts int
}

// NewHybridProvider creates a new hybrid provider with multi-layer fallback.
// Priority: TWSE (free) → FinMind (free) → Fubon (free, needs account) → Fugle (paid)
func NewHybridProvider(twseClient *TWSEClient, finmindAPIKey, fubonAPIKey, fugleAPIKey string) *HybridProvider {
	var finmindProvider *FinMindProvider
	if finmindAPIKey != "" {
		finmindProvider = NewFinMindProvider(finmindAPIKey)
	}

	var fubonProvider *FubonProvider
	if fubonAPIKey != "" {
		fubonProvider = NewFubonProviderWithClient(NewFubonClient(fubonAPIKey))
	}

	var fugleProvider *FugleProvider
	if fugleAPIKey != "" {
		fugleProvider = NewFugleProviderWithAPIKey(fugleAPIKey)
	}

	return &HybridProvider{
		twseClient:      twseClient,
		finmindProvider: finmindProvider,
		fubonProvider:   fubonProvider,
		fugleProvider:   fugleProvider,
		cbState:         ProviderCircuitClosed,
		cbConfig:        defaultCircuitBreakerConfig(),
	}
}

// Name returns the provider name based on current active provider.
func (p *HybridProvider) Name() string {
	return "hybrid"
}

// GetQuotes fetches quotes with priority-based fallback:
// 1. TWSE (free)
// 2. FinMind (free)
// 3. Fubon (free, requires account)
// 4. Fugle (paid, circuit breaker protected)
func (p *HybridProvider) GetQuotes(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error) {
	// 1. Try TWSE first (free)
	quotes, err := p.tryTWSE(ctx, symbols)
	if err == nil && len(quotes) > 0 && !p.hasInvalidQuotes(quotes) {
		return quotes, nil
	}
	if err != nil {
		fmt.Printf("[HybridProvider] TWSE failed: %v\n", err)
	}

	// 2. Try FinMind (free)
	if p.finmindProvider != nil {
		quotes, err = p.tryFinMind(ctx, asOf, symbols)
		if err == nil && len(quotes) > 0 && !p.hasInvalidQuotes(quotes) {
			return quotes, nil
		}
		if err != nil {
			fmt.Printf("[HybridProvider] FinMind failed: %v\n", err)
		}
	}

	// 3. Try Fubon (free, requires account)
	if p.fubonProvider != nil {
		quotes, err = p.tryFubon(ctx, asOf, symbols)
		if err == nil && len(quotes) > 0 && !p.hasInvalidQuotes(quotes) {
			return quotes, nil
		}
		if err != nil {
			fmt.Printf("[HybridProvider] Fubon failed: %v\n", err)
		}
	}

	// 4. Fallback to Fugle (paid, circuit breaker protected)
	if p.fugleProvider != nil && p.shouldTryFugle() {
		quotes, err = p.tryFugle(ctx, asOf, symbols)
		if err == nil && len(quotes) > 0 && !p.hasInvalidQuotes(quotes) {
			p.recordFugleSuccess()
			return quotes, nil
		}
		if err != nil {
			p.recordFugleFailure()
			fmt.Printf("[HybridProvider] Fugle failed (%v), all providers exhausted (circuit: %s, failures: %d)\n",
				err, p.getCircuitState(), p.getFailureCount())
		}
	}

	return nil, fmt.Errorf("all providers failed for symbols: %v", symbols)
}

func (p *HybridProvider) tryTWSE(ctx context.Context, symbols []string) ([]domain.Quote, error) {
	if len(symbols) == 1 {
		quote, err := p.twseClient.GetQuote(ctx, symbols[0])
		if err != nil {
			return nil, err
		}
		return []domain.Quote{quote}, nil
	}
	return p.twseClient.GetQuotesBySymbols(ctx, symbols)
}

func (p *HybridProvider) tryFinMind(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error) {
	quotes, err := p.finmindProvider.GetQuotes(ctx, asOf, symbols)
	if err != nil {
		return nil, err
	}
	if len(quotes) == 0 || p.hasInvalidQuotes(quotes) {
		return quotes, fmt.Errorf("finmind returned invalid/empty data")
	}
	return quotes, nil
}

func (p *HybridProvider) tryFubon(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error) {
	quotes, err := p.fubonProvider.GetQuotes(ctx, asOf, symbols)
	if err != nil {
		return nil, err
	}
	if len(quotes) == 0 || p.hasInvalidQuotes(quotes) {
		return quotes, fmt.Errorf("fubon returned invalid/empty data")
	}
	return quotes, nil
}

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

func (p *HybridProvider) shouldTryFugle() bool {
	p.cbMutex.Lock()
	defer p.cbMutex.Unlock()

	switch p.cbState {
	case ProviderCircuitClosed:
		return true
	case ProviderCircuitOpen:
		if time.Since(p.cbLastFailure) > p.cbConfig.recoveryTimeout {
			p.cbState = ProviderCircuitHalfOpen
			p.cbHalfOpenCalls = 0
			fmt.Printf("[HybridProvider] Circuit breaker entering half-open state, testing Fugle recovery\n")
			return true
		}
		return false
	case ProviderCircuitHalfOpen:
		if p.cbHalfOpenCalls < p.cbConfig.halfOpenMaxCalls {
			p.cbHalfOpenCalls++
			return true
		}
		return false
	}
	return false
}

func (p *HybridProvider) recordFugleFailure() {
	p.cbMutex.Lock()
	defer p.cbMutex.Unlock()

	p.cbFailureCount++
	p.cbLastFailure = time.Now()
	p.fallbackCount++
	p.lastFallbackAt = time.Now()

	if p.cbState == ProviderCircuitHalfOpen {
		p.cbState = ProviderCircuitOpen
		p.cbHalfOpenCalls = 0
		fmt.Printf("[HybridProvider] Fugle recovery failed in half-open state, circuit re-opened\n")
	} else if p.cbFailureCount >= p.cbConfig.failureThreshold {
		p.cbState = ProviderCircuitOpen
		fmt.Printf("[HybridProvider] Circuit breaker opened after %d consecutive failures\n", p.cbFailureCount)
	}
}

func (p *HybridProvider) recordFugleSuccess() {
	p.cbMutex.Lock()
	defer p.cbMutex.Unlock()

	if p.cbState != ProviderCircuitClosed {
		fmt.Printf("[HybridProvider] Fugle recovered, circuit breaker closed\n")
		p.recoveryAttempts++
	}
	p.cbState = ProviderCircuitClosed
	p.cbFailureCount = 0
	p.cbHalfOpenCalls = 0
}

func (p *HybridProvider) isCircuitOpen() bool {
	p.cbMutex.RLock()
	defer p.cbMutex.RUnlock()
	return p.cbState == ProviderCircuitOpen
}

// getCircuitState returns the current circuit state (for logging).
func (p *HybridProvider) getCircuitState() ProviderCircuitState {
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

// hasInvalidQuotes checks for invalid quote data (e.g., all prices are 0).
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

// Reset resets the provider state (re-try all providers).
func (p *HybridProvider) Reset() {
	p.cbMutex.Lock()
	defer p.cbMutex.Unlock()
	p.cbState = ProviderCircuitClosed
	p.cbFailureCount = 0
	p.cbHalfOpenCalls = 0
	p.fallbackCount = 0
	p.recoveryAttempts = 0
}

// UseTWSE forces using TWSE.
func (p *HybridProvider) UseTWSE() {
	p.cbMutex.Lock()
	defer p.cbMutex.Unlock()
	p.cbState = ProviderCircuitOpen
	p.cbLastFailure = time.Now()
}

// UseFugle forces using Fugle (if configured).
func (p *HybridProvider) UseFugle() {
	p.cbMutex.Lock()
	defer p.cbMutex.Unlock()
	p.cbState = ProviderCircuitClosed
	p.cbFailureCount = 0
	p.cbHalfOpenCalls = 0
}

func (p *HybridProvider) GetFubonClient() *FubonClient {
	if p.fubonProvider == nil {
		return nil
	}
	return p.fubonProvider.GetClient()
}

func (p *HybridProvider) UseFubon() {
	p.cbMutex.Lock()
	defer p.cbMutex.Unlock()
	p.cbState = ProviderCircuitClosed
	p.cbFailureCount = 0
	p.cbHalfOpenCalls = 0
}

func (p *HybridProvider) GetTWSEClient() *TWSEClient {
	return p.twseClient
}

func (p *HybridProvider) GetFugleClient() *FugleClient {
	if p.fugleProvider == nil {
		return nil
	}
	return p.fugleProvider.GetClient()
}

func (p *HybridProvider) GetFinMindClient() *FinMindClient {
	if p.finmindProvider == nil {
		return nil
	}
	return p.finmindProvider.GetClient()
}

// IsUsingTWSE returns true since TWSE is always the primary provider.
func (p *HybridProvider) IsUsingTWSE() bool {
	return true
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
