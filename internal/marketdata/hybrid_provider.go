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

	// narrowResidualLimit / narrowTimeout override the per-symbol fallback
	// guardrails (see quoteChain); 0 means "use the package constants".
	narrowResidualLimit int
	narrowTimeout       time.Duration

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

// ── Hybrid quote chain (issue #1986) ────────────────────────────────────────
//
// The chain resolves a request *incrementally*: each arm is asked only for the
// symbols no earlier arm resolved, and an arm that answers unusably for one
// symbol no longer discards the whole batch.
//
// Production evidence this replaces (Mac Mini, 2026-09-25 07:10Z, universe
// snapshot): quotes_requested=1599 quotes_returned=1301 quotes_chunks=32
// quotes_chunks_failed=2 → quotes_status=partial → ranked_trustworthy=false.
// The old rule was "if ANY quote in the batch is incomplete, throw the whole
// batch away and re-ask the next arm". The next two arms (FinMind, Fugle) issue
// one HTTP request per symbol and are rate limited to 0.17-0.5 req/s, so a
// single suspended stock turned a 50-symbol chunk into up to 50 sequential
// rate-limited requests — far more than the 60 s chunk budget — and everything
// after it failed with it.
//
// Narrow-arm guardrails (both measured, see the chunk-parameter section in
// docs/specs/universe-quote-reliability-spec.md):
//
//   - hybridNarrowResidualMax: above this many unresolved symbols a per-symbol
//     arm is not asked at all. 8 symbols at the slowest shipped tier (Fugle
//     free, 30 req/min ≈ 2 s/request) is ~16 s, which fits the 10 s arm
//     deadline below only in the common case, and is why the wide TWSE arm —
//     one request for the entire listed market — always runs last and is what
//     actually closes the gap.
//   - hybridNarrowArmTimeout: hard per-arm deadline for the per-symbol arms,
//     so one slow symbol cannot consume the caller's whole chunk budget.
//
// The caller's chunk timeout (DefaultQuoteChunkTimeout = 60 s) is the outer
// bound; see QuoteFetchPolicy.
const (
	hybridNarrowResidualMax = 8
	hybridNarrowArmTimeout  = 10 * time.Second
)

// hybridArm is one provider in the fallback chain.
type hybridArm struct {
	name string
	// wide marks an arm whose successful answer covers a complete venue, so a
	// requested symbol missing from it is "not covered by this source" rather
	// than a failed acquisition. Only whole-market tables (TWSE STOCK_DAY_ALL)
	// may set it.
	wide bool
	// narrow marks an arm that issues one upstream request per symbol.
	narrow  bool
	breaker *providerBreaker
	// fetch returns the arm's raw answer. It is the default path, and the only
	// one that lets the chain tell "the source published this symbol without a
	// usable price" (no_data) from "the source does not publish it at all".
	fetch func(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error)
	// fetchBatch is used instead of fetch when the arm can classify per symbol
	// itself (marketdata.PartialBatchProvider) — e.g. FinMind knows whether a
	// missing row means "no trades that day" or "the request failed".
	fetchBatch func(ctx context.Context, asOf time.Time, symbols []string) (QuoteBatch, error)
}

// narrowResidualMax / narrowArmTimeout are the effective guardrails; they are
// fields so tests can shrink them and so a future caller with a different
// budget can raise them deliberately.
func (p *HybridProvider) narrowResidualMax() int {
	if p.narrowResidualLimit > 0 {
		return p.narrowResidualLimit
	}
	return hybridNarrowResidualMax
}

func (p *HybridProvider) narrowArmTimeout() time.Duration {
	if p.narrowTimeout > 0 {
		return p.narrowTimeout
	}
	return hybridNarrowArmTimeout
}

// SetNarrowFallbackPolicy overrides the per-symbol fallback guardrails for
// callers that carry a different chunk budget than the shipped 60 s policy.
//
// Production deliberately uses the package constants
// (hybridNarrowResidualMax / hybridNarrowArmTimeout): the shipped
// QuoteFetchPolicy and these guardrails are a matched pair, documented together
// in docs/specs/universe-quote-reliability-spec.md §3.2. The setter stays
// exported so a caller with a different chunk timeout can raise the residual
// budget deliberately instead of editing the chain; today only the boundary
// tests call it.
//
// inert-ok[writer-no-consumer]: the clamped-budget behavior is exercised by
// TestHybridQuoteChain_AsksPerSymbolArmUpToTheResidualBudget and
// TestHybridQuoteChain_PerSymbolArmDeadlineBoundsTheArm.
func (p *HybridProvider) SetNarrowFallbackPolicy(residualMax int, armTimeout time.Duration) {
	p.narrowResidualLimit = residualMax
	p.narrowTimeout = armTimeout
}

// quoteChain returns the arms in priority order. The fubon primary is resolved
// lazily because it can be armed after construction (maybeArmFubon).
func (p *HybridProvider) quoteChain() []hybridArm {
	arms := make([]hybridArm, 0, 4)
	if fp := p.fubon(); fp != nil {
		arms = append(arms, hybridArm{
			name:    "fubon",
			breaker: p.breakers["fubon"],
			fetch:   fp.GetQuotes,
		})
	}
	if p.finmindProvider != nil {
		arms = append(arms, hybridArm{
			name:   "finmind",
			narrow: true,
			fetch:  p.finmindProvider.GetQuotes,
			fetchBatch: func(ctx context.Context, asOf time.Time, symbols []string) (QuoteBatch, error) {
				return p.finmindProvider.GetQuotesBatch(ctx, asOf, symbols)
			},
		})
	}
	if p.fugleProvider != nil {
		arms = append(arms, hybridArm{
			name:    "fugle",
			narrow:  true,
			breaker: p.breakers["fugle"],
			fetch:   p.fugleProvider.GetQuotes,
		})
	}
	if p.twseClient != nil {
		arms = append(arms, hybridArm{
			name: "twse",
			wide: true,
			fetch: func(ctx context.Context, _ time.Time, symbols []string) ([]domain.Quote, error) {
				return p.getQuotesFromTWSE(ctx, symbols)
			},
		})
	}
	return arms
}

// GetQuotes implements marketdata.Provider. It keeps the pre-#1986 signature
// and returns every quote the chain could resolve.
func (p *HybridProvider) GetQuotes(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error) {
	batch, err := p.GetQuotesBatch(ctx, asOf, symbols)
	return batch.Quotes, err
}

// GetQuotesBatch implements PartialBatchProvider: it returns the resolved
// quotes plus a per-symbol verdict for every requested symbol, so a consumer can
// tell "this source does not have that stock" (benign) from "we failed to
// acquire it" (not benign) — the distinction issue #1986 requires.
func (p *HybridProvider) GetQuotesBatch(ctx context.Context, asOf time.Time, symbols []string) (QuoteBatch, error) {
	batch := NewQuoteBatch(symbols)
	if len(symbols) == 0 {
		return batch, nil
	}

	// Self-healing primary selection: if the Fubon primary was not armed at
	// construction (proxy not up yet), re-probe here instead of never trying
	// again. No-op once armed; rate-limited to one probe per
	// hybridFubonArmInterval while the proxy stays down.
	p.maybeArmFubon()

	var lastErr error
	for _, arm := range p.quoteChain() {
		residual := batch.Missing(symbols)
		if len(residual) == 0 {
			break
		}
		if arm.narrow && len(residual) > p.narrowResidualMax() {
			// Bound the per-symbol fan-out (see the block comment above): ask a
			// per-symbol arm for the whole residual and the chunk cannot finish
			// inside its timeout, so skip it and leave the symbols to the wide
			// arm that follows.
			logging.Warn("hybrid_provider", "narrow_arm_skipped",
				"arm", arm.name,
				"residual", len(residual),
				"residual_max", p.narrowResidualMax())
			continue
		}
		if arm.breaker != nil && !arm.breaker.shouldTry() {
			logging.Warn("hybrid_provider", "arm_circuit_open",
				"arm", arm.name,
				"residual", len(residual))
			continue
		}
		if err := ctx.Err(); err != nil {
			// The caller's budget is gone: record what is left as an
			// acquisition gap and stop instead of asking a dead context.
			batch.Resolve(symbols, QuoteOutcomeError)
			return batch, err
		}
		if err := p.runArm(ctx, &batch, arm, residual, asOf); err != nil {
			lastErr = err
		}
	}

	// Everything still unresolved was either asked for and failed (error) or
	// never asked at all (not_attempted, e.g. a narrow arm skipped by the budget
	// with no wide arm behind it).
	if unresolved := batch.Missing(symbols); len(unresolved) > 0 {
		logging.Warn("hybrid_provider", "batch_unresolved",
			"symbols", len(symbols),
			"unresolved", len(unresolved),
			"sample", sampleSymbols(unresolved, 10))
	}

	// Report an error only when the chain produced nothing at all: a caller
	// (the universe pipeline) treats a non-empty partial result as usable input
	// and reads the per-symbol verdicts for the rest.
	if len(batch.Quotes) == 0 && lastErr != nil {
		return batch, lastErr
	}
	return batch, nil
}

// runArm asks one arm for the residual and folds the answer into batch.
//
// The returned error is the arm's hard failure. It is advisory: the chain keeps
// going with the next arm because a failed arm is exactly what fallback is for.
func (p *HybridProvider) runArm(ctx context.Context, batch *QuoteBatch, arm hybridArm, symbols []string, asOf time.Time) error {
	callCtx := ctx
	if arm.narrow {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, p.narrowArmTimeout())
		defer cancel()
	}

	start := time.Now()
	if arm.fetchBatch != nil {
		return p.runClassifyingArm(callCtx, batch, arm, symbols, asOf, start)
	}
	quotes, err := arm.fetch(callCtx, asOf, symbols)
	elapsed := time.Since(start)

	if err != nil {
		p.recordArmFailure(arm)
		logging.Warn("hybrid_provider", "arm_error",
			"arm", arm.name,
			"symbols", len(symbols),
			"elapsed_ms", elapsed.Milliseconds(),
			logging.Err(err))
		p.recordTrace(arm.name, fmt.Sprintf("arm %s failed: %v", arm.name, err), len(symbols))
		batch.Resolve(symbols, QuoteOutcomeError)
		return err
	}

	valid, resolved, unusable := usableQuotes(quotes, symbols)
	if len(valid) == 0 {
		// An arm that answers with nothing usable for a non-empty request is
		// treated as failed (this is the pre-#1986 "invalid quotes" condition,
		// now scoped to the arm instead of the whole batch).
		p.recordArmFailure(arm)
		err := fmt.Errorf("%s returned no usable quote for %d symbols", arm.name, len(symbols))
		p.recordTrace(arm.name, err.Error(), len(symbols))
		batch.Resolve(symbols, QuoteOutcomeError)
		return err
	}

	p.recordArmSuccess(arm)
	for _, q := range valid {
		batch.RecordFor(normalizeQuoteSymbol(q.Symbol), q)
	}

	unresolved := make([]string, 0, len(symbols))
	answeredNoData := make([]string, 0, len(symbols))
	for _, sym := range symbols {
		key := normalizeQuoteSymbol(sym)
		if resolved[key] {
			continue
		}
		if unusable[key] && arm.wide {
			// A WHOLE-MARKET arm DID publish the symbol, it just has no usable
			// quote for it (all-zero or closePrice-only: suspended, no trades
			// that day). Its table is the venue's complete daily publication,
			// so that is an authoritative "no data", not a missing row.
			//
			// A per-symbol arm gets no such credit: one all-zero intraday answer
			// is indistinguishable from "this arm is broken" (a closed market, a
			// dead proxy), and letting it declare no_data would stop the chain
			// and silently lose the symbol.
			answeredNoData = append(answeredNoData, sym)
			continue
		}
		unresolved = append(unresolved, sym)
	}
	if len(answeredNoData) > 0 {
		batch.SetOutcomeFor(answeredNoData, QuoteOutcomeNoData)
	}
	if len(unresolved) == 0 {
		return nil
	}
	if arm.wide {
		// The arm answered for a complete venue, so absence is a source-scope
		// fact, not an acquisition failure.
		batch.Resolve(unresolved, QuoteOutcomeNotCovered)
	} else {
		batch.Resolve(unresolved, QuoteOutcomeError)
	}
	logging.Info("hybrid_provider", "arm_partial",
		"arm", arm.name,
		"requested", len(symbols),
		"resolved", len(valid),
		"unresolved", len(unresolved),
		"unresolved_outcome", string(batch.Outcomes[unresolved[0]]),
		"elapsed_ms", elapsed.Milliseconds())
	return nil
}

// runClassifyingArm folds an arm that reports its own per-symbol verdicts.
//
// The verdicts are authoritative for the residual the arm was asked for, so they
// are merged as-is: that is what lets a suspended symbol be reported as no_data
// instead of being retried by every remaining arm.
func (p *HybridProvider) runClassifyingArm(ctx context.Context, batch *QuoteBatch, arm hybridArm, symbols []string, asOf time.Time, start time.Time) error {
	sub, err := arm.fetchBatch(ctx, asOf, symbols)
	elapsed := time.Since(start)
	if err != nil {
		p.recordArmFailure(arm)
		logging.Warn("hybrid_provider", "arm_error",
			"arm", arm.name,
			"symbols", len(symbols),
			"elapsed_ms", elapsed.Milliseconds(),
			logging.Err(err))
		p.recordTrace(arm.name, fmt.Sprintf("arm %s failed: %v", arm.name, err), len(symbols))
		batch.Resolve(symbols, QuoteOutcomeError)
		return err
	}
	if len(sub.Quotes) == 0 {
		p.recordArmFailure(arm)
		err := fmt.Errorf("%s returned no usable quote for %d symbols", arm.name, len(symbols))
		p.recordTrace(arm.name, err.Error(), len(symbols))
		batch.Resolve(symbols, QuoteOutcomeError)
		return err
	}

	p.recordArmSuccess(arm)
	MergeBatch(batch, sub)
	logging.Info("hybrid_provider", "arm_partial",
		"arm", arm.name,
		"requested", len(symbols),
		"resolved", len(sub.Quotes),
		"elapsed_ms", elapsed.Milliseconds())
	return nil
}

func (p *HybridProvider) recordArmFailure(arm hybridArm) {
	if arm.breaker != nil {
		arm.breaker.recordFailure()
	}
}

func (p *HybridProvider) recordArmSuccess(arm hybridArm) {
	if arm.breaker != nil {
		arm.breaker.recordSuccess()
	}
}

func (p *HybridProvider) recordTrace(provider, reason string, symbols int) {
	if p.traceWriter == nil {
		return
	}
	p.traceWriter.Record(0, "marketdata", "WARN", map[string]any{
		"primary":         provider,
		"fallback_reason": reason,
		"symbols":         symbols,
	})
}

// usableQuotes splits an arm's answer into the quotes a consumer can use and
// the set of requested symbols they cover.
//
// A quote is usable when QuoteComplete holds and every price/volume field is
// non-negative (the pre-#1986 hasInvalidQuotes predicate, applied per quote).
// Symbols answered with unusable quotes count as NOT resolved, so the next arm
// is asked for exactly them.
func usableQuotes(quotes []domain.Quote, requested []string) (valid []domain.Quote, resolved, unusable map[string]bool) {
	want := make(map[string]bool, len(requested))
	for _, s := range requested {
		want[normalizeQuoteSymbol(s)] = true
	}
	resolved = make(map[string]bool, len(requested))
	unusable = make(map[string]bool)
	valid = make([]domain.Quote, 0, len(quotes))
	for _, q := range quotes {
		sym := normalizeQuoteSymbol(q.Symbol)
		if !want[sym] || resolved[sym] {
			continue
		}
		if !QuoteComplete(q) || q.Last < 0 || q.Open < 0 || q.High < 0 || q.Low < 0 || q.Volume < 0 {
			unusable[sym] = true
			continue
		}
		resolved[sym] = true
		valid = append(valid, q)
	}
	return valid, resolved, unusable
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

// hasInvalidQuotes reports whether any quote in quotes is unusable under the
// shared completeness rule (manifest Phase B1) plus the non-negative sanity
// checks. It is retained for callers that need the batch-level predicate; the
// fallback chain itself now uses usableQuotes, which drops only the offending
// quotes instead of the whole batch.
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
