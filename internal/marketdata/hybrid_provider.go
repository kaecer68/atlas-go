package marketdata

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// TraceWriter is a nil-safe interface for recording execution traces.
// It avoids circular imports by being defined locally in marketdata.
type TraceWriter interface {
	Record(step int, layer, status string, meta map[string]any)
}

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

type HybridProvider struct {
	fubonProvider   *FubonProvider
	finmindProvider *FinMindProvider
	fugleProvider   *FugleProvider
	twseClient      *TWSEClient

	cbState         ProviderCircuitState
	cbFailureCount  int
	cbLastFailure   time.Time
	cbHalfOpenCalls int
	cbConfig        circuitBreakerConfig
	cbMutex         sync.RWMutex

	fallbackCount    int
	lastFallbackAt   time.Time
	recoveryAttempts int

	traceWriter TraceWriter
}

func NewHybridProvider(finmindAPIKey, fugleAPIKey string) *HybridProvider {
	var fubonProvider *FubonProvider
	if os.Getenv("FUBON_PROXY_URL") != "" || fubonProxyBaseURL != "" {
		fubonClient := NewFubonClient("")
		fubonProvider = NewFubonProviderWithClient(fubonClient)
	}

	var finmindProvider *FinMindProvider
	if finmindAPIKey != "" {
		finmindProvider = NewFinMindProvider(finmindAPIKey)
	}

	var fugleProvider *FugleProvider
	if fugleAPIKey != "" {
		fugleProvider = NewFugleProviderWithAPIKey(fugleAPIKey)
	}

	return &HybridProvider{
		fubonProvider:   fubonProvider,
		finmindProvider: finmindProvider,
		fugleProvider:   fugleProvider,
		twseClient:      GetSharedTWSEClient(),
		cbState:         ProviderCircuitClosed,
		cbConfig:        defaultCircuitBreakerConfig(),
	}
}

func (p *HybridProvider) Name() string {
	if p.fubonProvider != nil {
		return "hybrid-fubon"
	}
	if p.finmindProvider != nil {
		return "hybrid-finmind"
	}
	if p.fugleProvider != nil {
		return "hybrid-fugle"
	}
	return "hybrid-twse"
}

func (p *HybridProvider) GetQuotes(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error) {
	if p.fubonProvider != nil {
		quotes, err := p.fubonProvider.GetQuotes(ctx, asOf, symbols)
		if err == nil && len(quotes) > 0 && !p.hasInvalidQuotes(quotes) {
			return quotes, nil
		}
		logging.Warn("hybrid_provider", "fubon_failed_fallback", logging.Err(err))
		if p.traceWriter != nil {
			p.traceWriter.Record(0, "marketdata", "WARN", map[string]any{
				"primary":         "fubon",
				"fallback_reason": fmt.Sprintf("fubon failed: %v", err),
				"symbols":         len(symbols),
			})
		}
	}

	if p.finmindProvider != nil {
		quotes, err := p.finmindProvider.GetQuotes(ctx, asOf, symbols)
		if err == nil && len(quotes) > 0 && !p.hasInvalidQuotes(quotes) {
			return quotes, nil
		}
		logging.Warn("hybrid_provider", "finmind_failed_fallback", logging.Err(err))
		if p.traceWriter != nil {
			p.traceWriter.Record(0, "marketdata", "WARN", map[string]any{
				"primary":         "finmind",
				"fallback_reason": fmt.Sprintf("finmind failed: %v", err),
				"symbols":         len(symbols),
			})
		}
	}

	return p.getQuotesFromFugleOrTWSE(ctx, asOf, symbols)
}

func (p *HybridProvider) getQuotesFromFugleOrTWSE(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error) {
	if p.fugleProvider != nil && p.shouldTryFugle() {
		quotes, err := p.tryFugle(ctx, asOf, symbols)
		if err == nil && len(quotes) > 0 && !p.hasInvalidQuotes(quotes) {
			p.UseFugle()
			return quotes, nil
		}
		if err != nil {
			logging.Warn("hybrid_provider", "fugle_failed_fallback", logging.Err(err))
			if p.traceWriter != nil {
				p.traceWriter.Record(0, "marketdata", "WARN", map[string]any{
					"primary":         "fugle",
					"fallback_reason": fmt.Sprintf("fugle failed: %v", err),
					"symbols":         len(symbols),
				})
			}
		}
	}
	return p.getQuotesFromTWSE(ctx, symbols)
}

func (p *HybridProvider) shouldTryFugle() bool {
	p.cbMutex.Lock()
	defer p.cbMutex.Unlock()

	if p.cbState == ProviderCircuitClosed || p.cbState == ProviderCircuitHalfOpen {
		return p.fugleProvider != nil
	}
	if p.cbState == ProviderCircuitOpen && time.Since(p.cbLastFailure) > p.cbConfig.recoveryTimeout {
		return p.fugleProvider != nil
	}
	return false
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

func (p *HybridProvider) isCircuitOpen() bool {
	p.cbMutex.RLock()
	defer p.cbMutex.RUnlock()
	return p.cbState == ProviderCircuitOpen
}

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

func (p *HybridProvider) Reset() {
	p.cbMutex.Lock()
	defer p.cbMutex.Unlock()
	if p.fugleProvider == nil {
		p.cbState = ProviderCircuitOpen
	} else {
		p.cbState = ProviderCircuitClosed
	}
	p.cbFailureCount = 0
	p.cbHalfOpenCalls = 0
	p.fallbackCount = 0
	p.recoveryAttempts = 0
}

func (p *HybridProvider) UseTWSE() {
	p.cbMutex.Lock()
	defer p.cbMutex.Unlock()
	p.cbState = ProviderCircuitOpen
	p.cbLastFailure = time.Now()
}

func (p *HybridProvider) UseFugle() {
	p.cbMutex.Lock()
	defer p.cbMutex.Unlock()
	p.cbState = ProviderCircuitClosed
	p.cbFailureCount = 0
	p.cbHalfOpenCalls = 0
}

func (p *HybridProvider) GetFinMindClient() *FinMindClient {
	if p.finmindProvider == nil {
		return nil
	}
	return p.finmindProvider.GetClient()
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

func (p *HybridProvider) GetFubonClient() *FubonClient {
	if p.fubonProvider == nil {
		return nil
	}
	return p.fubonProvider.GetClient()
}

func (p *HybridProvider) SetTraceWriter(tw TraceWriter) {
	p.traceWriter = tw
}

func (p *HybridProvider) IsUsingTWSE() bool {
	return p.isCircuitOpen()
}

func (p *HybridProvider) CircuitBreakerStats() map[string]any {
	p.cbMutex.RLock()
	defer p.cbMutex.RUnlock()
	return map[string]any{
		"state":             string(p.cbState),
		"failure_count":     p.cbFailureCount,
		"failure_threshold": p.cbConfig.failureThreshold,
		"last_failure":      p.cbLastFailure.Format(time.RFC3339),
		"fallback_count":    p.fallbackCount,
		"last_fallback":     p.lastFallbackAt.Format(time.RFC3339),
		"recovery_attempts": p.recoveryAttempts,
	}
}
