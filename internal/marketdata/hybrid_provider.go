package marketdata

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/fubonproxy"
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

	breakers map[string]*providerBreaker

	fallbackCount    int
	lastFallbackAt   time.Time
	recoveryAttempts int

	traceWriter TraceWriter
}

func NewHybridProvider(finmindAPIKey, fugleAPIKey string) *HybridProvider {
	var fubonProvider *FubonProvider
	// Probe the proxy before creating the client to avoid constant
	// "connection refused" warnings when the proxy is not running.
	//
	// 使用 fubonproxy.ProxyHostPort() 而非硬編碼舊值,
	// 確保與 cmd/atlas -fubon-port flag 同步(歷史 bug:此處原本硬編碼 18081,
	// 當 fubon-proxy 跑在 alt-port 時 probe 仍打 18081 → 永遠 "not reachable")。
	if conn, err := net.DialTimeout("tcp", fubonproxy.ProxyHostPort(), 2*time.Second); err != nil {
		logging.Info("hybrid_provider", "fubon_proxy_not_reachable", "msg", "skipping fubon fallback — proxy not running")
	} else {
		_ = conn.Close()
		fubonClient := GetSharedFubonClient()
		fubonProvider = NewFubonProviderWithClient(fubonClient)
	}

	var finmindProvider *FinMindProvider
	if finmindAPIKey != "" {
		finmindProvider = NewFinMindProvider(finmindAPIKey)
	}

	var fugleProvider *FugleProvider
	if fugleAPIKey != "" {
		// Shared singleton client: one rate limiter enforces the Fugle tier
		// limit across hybrid provider, stocktools, gateway channel, and
		// warmup. A per-instance client would give each its own 60/min
		// budget and blow past the free-tier limit (SK-22 Fugle audit).
		fugleProvider = NewFugleProviderWithClient(GetSharedFugleClient(fugleAPIKey))
	}

	breakers := map[string]*providerBreaker{
		"fugle": newProviderBreaker("fugle", defaultCircuitBreakerConfig()),
	}
	if fubonProvider != nil {
		breakers["fubon"] = newProviderBreaker("fubon", defaultCircuitBreakerConfig())
	}

	return &HybridProvider{
		fubonProvider:   fubonProvider,
		finmindProvider: finmindProvider,
		fugleProvider:   fugleProvider,
		twseClient:      GetSharedTWSEClient(),
		breakers:        breakers,
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
	if p.fubonProvider != nil && p.breakers["fubon"].shouldTry() {
		quotes, err := p.fubonProvider.GetQuotes(ctx, asOf, symbols)
		if err == nil && len(quotes) > 0 && !p.hasInvalidQuotes(quotes) {
			p.breakers["fubon"].recordSuccess()
			return quotes, nil
		}
		p.breakers["fubon"].recordFailure()
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
			p.breakers["fugle"].recordSuccess()
			return quotes, nil
		}
		p.breakers["fugle"].recordFailure()
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
	return p.breakers["fugle"].shouldTry() && p.fugleProvider != nil
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
		// 共用完整性判定（manifest Phase B1）：無資料（全 0）與
		// closePrice-only 殘缺（Last>0 但 OHLC 全 0）都視為 invalid。
		if !QuoteComplete(q) {
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
	if p.fugleProvider == nil {
		p.breakers["fugle"].forceState(ProviderCircuitOpen)
	} else {
		p.breakers["fugle"].reset()
	}
	if fb, ok := p.breakers["fubon"]; ok {
		fb.reset()
	}
	p.fallbackCount = 0
	p.recoveryAttempts = 0
}

func (p *HybridProvider) UseTWSE() {
	p.breakers["fugle"].forceState(ProviderCircuitOpen)
}

func (p *HybridProvider) UseFugle() {
	p.breakers["fugle"].forceState(ProviderCircuitClosed)
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
	return p.breakers["fugle"].stateSnapshot().State == ProviderCircuitOpen
}

func (p *HybridProvider) CircuitBreakerStats() map[string]any {
	providers := make(map[string]ProviderBreakerInfo)
	for name, b := range p.breakers {
		providers[name] = b.stateSnapshot()
	}

	stats := map[string]any{
		"fallback_count":    p.fallbackCount,
		"last_fallback":     p.lastFallbackAt.Format(time.RFC3339),
		"recovery_attempts": p.recoveryAttempts,
		"providers":         providers,
	}

	// backward-compatible top-level fields (Fugle aggregate)
	if fb, ok := providers["fugle"]; ok {
		stats["state"] = string(fb.State)
		stats["failure_count"] = fb.FailureCount
		stats["failure_threshold"] = fb.Threshold
		if !fb.LastFailure.IsZero() {
			stats["last_failure"] = fb.LastFailure.Format(time.RFC3339)
		}
	}

	return stats
}
