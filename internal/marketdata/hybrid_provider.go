package marketdata

import (
	"context"
	"fmt"
	"net"
	"sync"
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

	// fubonMu guards fubonProvider and nextFubonArmAt. The Fubon primary can be
	// armed AFTER construction (see maybeArmFubon): the startup probe is only a
	// starting condition, because a long-lived provider (runLiveTrading creates
	// exactly one) must recover when fubon-proxy comes up later. The pointed-to
	// FubonProvider itself is immutable once created, so returning the pointer
	// out of the lock is safe.
	fubonMu sync.RWMutex
	// nextFubonArmAt is the earliest time the next possibly-reachable re-probe
	// may run; it bounds the re-probe rate when the proxy stays down.
	nextFubonArmAt time.Time
	// fubonProbe is the reachability probe used to arm the Fubon primary.
	// nil → default TCP dial of fubonproxy.ProxyHostPort() with a 2s timeout.
	// Overridable in tests.
	fubonProbe func() bool

	fallbackCount    int
	lastFallbackAt   time.Time
	recoveryAttempts int

	traceWriter TraceWriter
}

func NewHybridProvider(finmindAPIKey, fugleAPIKey string) *HybridProvider {
	// The Fubon probe below is only a STARTING condition, not a permanent
	// verdict: maybeArmFubon() (called from the quote path) re-probes while the
	// provider is unarmed, so a proxy that starts after this constructor ran is
	// picked up by the same process instead of being lost forever.
	//
	// 2026-09-21 evidence: a 0.18s startup race (atlas-go probed before the
	// fubon-proxy container listened) permanently disabled the fubon channel
	// and, on this path, the fubon primary for the whole process lifetime.
	// Long-lived providers — runLiveTrading builds exactly one and reuses it —
	// must not be able to lose a data source to one lost race.
	//
	// 使用 fubonproxy.ProxyHostPort() 而非硬編碼舊值,
	// 確保與 cmd/atlas -fubon-port flag 同步(歷史 bug:此處原本硬編碼 18081,
	// 當 fubon-proxy 跑在 alt-port 時 probe 仍打 18081 → 永遠 "not reachable")。
	// (probe 實作在 probeFubonProxy;預設 TCP dial 2s timeout。)

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
		// The fubon breaker exists whether or not the provider is armed yet:
		// armFubonPrimary() can arm it later, and adding a key to this map after
		// construction would race with CircuitBreakerStats()/Reset() readers.
		"fubon": newProviderBreaker("fubon", defaultCircuitBreakerConfig()),
	}

	p := &HybridProvider{
		finmindProvider: finmindProvider,
		fugleProvider:   fugleProvider,
		twseClient:      GetSharedTWSEClient(),
		breakers:        breakers,
	}
	if !p.armFubonPrimary() {
		logging.Info("hybrid_provider", "fubon_proxy_not_reachable",
			"msg", "skipping fubon fallback — proxy not running (will re-probe on the next GetQuotes, at most once per "+hybridFubonArmInterval.String()+")")
	}
	return p
}

// hybridFubonArmInterval bounds how often an unarmed Fubon primary is
// re-probed. It keeps the "do not hammer a dead proxy" intent of the original
// one-shot probe while removing the "never try again" part.
const hybridFubonArmInterval = 2 * time.Minute

// fubon returns the currently armed Fubon provider (nil when unarmed), safe
// against the lazy arming in maybeArmFubon.
func (p *HybridProvider) fubon() *FubonProvider {
	p.fubonMu.RLock()
	defer p.fubonMu.RUnlock()
	return p.fubonProvider
}

// probeFubonProxy reports whether fubon-proxy answers on the shared
// ProxyHostPort(). Tests inject p.fubonProbe.
func (p *HybridProvider) probeFubonProxy() bool {
	if p.fubonProbe != nil {
		return p.fubonProbe()
	}
	conn, err := net.DialTimeout("tcp", fubonproxy.ProxyHostPort(), 2*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// armFubonPrimary creates the Fubon provider when the proxy is reachable.
// Idempotent: an already-armed provider short-circuits without probing.
func (p *HybridProvider) armFubonPrimary() bool {
	p.fubonMu.RLock()
	armed := p.fubonProvider != nil
	p.fubonMu.RUnlock()
	if armed {
		return true
	}

	// Probe OUTSIDE the lock: a 2s dial must not block Name()/GetQuotes()
	// readers (and the arm is re-checked under the write lock below).
	if !p.probeFubonProxy() {
		return false
	}

	p.fubonMu.Lock()
	defer p.fubonMu.Unlock()
	if p.fubonProvider == nil {
		p.fubonProvider = NewFubonProviderWithClient(GetSharedFubonClient())
		logging.Info("hybrid_provider", "fubon_primary_armed",
			"msg", "fubon-proxy reachable — fubon primary armed")
	}
	return true
}

// maybeArmFubon retries the reachability probe when the Fubon primary is not
// armed yet, at most once per hybridFubonArmInterval. Called from the quote
// path so a long-lived provider recovers after a proxy restart or a lost
// startup race without needing a process restart.
func (p *HybridProvider) maybeArmFubon() {
	if p.fubon() != nil {
		return
	}

	p.fubonMu.Lock()
	if p.fubonProvider != nil {
		p.fubonMu.Unlock()
		return
	}
	now := time.Now()
	if now.Before(p.nextFubonArmAt) {
		p.fubonMu.Unlock()
		return
	}
	// Reserve the next attempt before releasing the lock so concurrent callers
	// cannot all dial at once.
	p.nextFubonArmAt = now.Add(hybridFubonArmInterval)
	p.fubonMu.Unlock()

	// armFubonPrimary logs on success; repeated failures stay silent on purpose
	// (the caller's fallback chain already reports the degraded path).
	p.armFubonPrimary()
}

func (p *HybridProvider) Name() string {
	if p.fubon() != nil {
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
	// Self-healing primary selection: if the Fubon primary was not armed at
	// construction (proxy not up yet), re-probe here instead of never trying
	// again. No-op once armed; rate-limited to one probe per
	// hybridFubonArmInterval while the proxy stays down.
	p.maybeArmFubon()

	if fp := p.fubon(); fp != nil {
		if fb, ok := p.breakers["fubon"]; ok && fb.shouldTry() {
			quotes, err := fp.GetQuotes(ctx, asOf, symbols)
			if err == nil && len(quotes) > 0 && !p.hasInvalidQuotes(quotes) {
				fb.recordSuccess()
				return quotes, nil
			}
			fb.recordFailure()
			logging.Warn("hybrid_provider", "fubon_failed_fallback", logging.Err(err))
			if p.traceWriter != nil {
				p.traceWriter.Record(0, "marketdata", "WARN", map[string]any{
					"primary":         "fubon",
					"fallback_reason": fmt.Sprintf("fubon failed: %v", err),
					"symbols":         len(symbols),
				})
			}
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
	fp := p.fubon()
	if fp == nil {
		return nil
	}
	return fp.GetClient()
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
