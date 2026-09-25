package marketdata

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// MarketWideQuoteSource is a first-party quote table that covers a whole venue
// in ONE request (TWSE STOCK_DAY_ALL, TPEx 上櫃每日收盤行情).
//
// It exists because the per-symbol arms of the hybrid chain cannot close a
// whole-market coverage gap: FinMind and Fugle each issue one HTTP request per
// symbol and are rate limited to 0.17-0.5 req/s, so covering the 695 上櫃
// symbols the TWSE table does not publish would take 23 minutes at the free
// Fugle tier (issue #1986, measured 2026-09-25).
type MarketWideQuoteSource interface {
	Name() string
	// Snapshot returns every quote the source currently publishes for the
	// market it covers. Callers filter by symbol.
	Snapshot(ctx context.Context) ([]domain.Quote, error)
}

// CoverageCompletingProvider wraps the hybrid chain and fills the residual of
// each quote request from whole-market first-party tables (issue #1986).
//
// Why a wrapper and not a new arm inside HybridProvider: HybridProvider is on
// the live-trading path as well as the universe path. Appending an end-of-day
// 上櫃 table to that chain would silently feed previous-close prices to live
// trading for symbols the intraday arms could not quote. The universe pipeline
// — an end-of-day ranking job — is exactly the consumer that wants them, and it
// is the only consumer wired to this type (see cmd/atlas/bootstrap_helpers.go
// newUniverseQuoteProvider).
//
// It implements PartialBatchProvider, so the universe pipeline receives a
// per-symbol verdict instead of having to guess why a symbol came back empty.
type CoverageCompletingProvider struct {
	primary Provider
	sources []MarketWideQuoteSource

	// mu guards snaps. Each whole-market table is an end-of-day artifact
	// published once per trading day, so re-downloading it for every chunk is
	// pure waste: the universe pipeline asks in 32 chunks of 50 symbols and then
	// asks again for the Layer 2.5 risk re-check. The first chunk pays one
	// download (~2 s, 2 MB) and the rest are served from here.
	mu    sync.Mutex
	snaps []marketWideSnapshot
}

// marketWideCacheTTL bounds how long a fetched whole-market table is reused.
// Five minutes costs nothing in freshness (the table changes once a day) and
// keeps a long-lived process from serving yesterday's close after the nightly
// publish.
const marketWideCacheTTL = 5 * time.Minute

type marketWideSnapshot struct {
	quotes []domain.Quote
	at     time.Time
}

// marketWideSnapshotTimeout bounds one whole-market download (retry schedule
// included) so a slow table cannot consume a chunk's whole 60s budget.
const marketWideSnapshotTimeout = 20 * time.Second

// NewCoverageCompletingProvider wraps primary with the given whole-market
// sources, tried in order. A nil primary is allowed (the sources then serve
// every request) and keeps the type usable when no market-data provider is
// configured.
func NewCoverageCompletingProvider(primary Provider, sources ...MarketWideQuoteSource) *CoverageCompletingProvider {
	kept := make([]MarketWideQuoteSource, 0, len(sources))
	for _, s := range sources {
		if s != nil {
			kept = append(kept, s)
		}
	}
	return &CoverageCompletingProvider{primary: primary, sources: kept}
}

// Name implements marketdata.Provider.
func (p *CoverageCompletingProvider) Name() string {
	inner := "none"
	if p.primary != nil {
		inner = p.primary.Name()
	}
	return "coverage-completing-" + inner
}

// GetQuotes implements marketdata.Provider.
func (p *CoverageCompletingProvider) GetQuotes(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error) {
	batch, err := p.GetQuotesBatch(ctx, asOf, symbols)
	return batch.Quotes, err
}

// Primary exposes the wrapped provider (nil when only whole-market sources are
// configured). Wiring tests use it to assert the coverage layer wraps — rather
// than replaces — the provider the simulation path uses.
func (p *CoverageCompletingProvider) Primary() Provider { return p.primary }

// MarketWideSources returns a copy of the configured whole-market sources.
func (p *CoverageCompletingProvider) MarketWideSources() []MarketWideQuoteSource {
	return append([]MarketWideQuoteSource(nil), p.sources...)
}

// GetQuotesBatch asks the primary chain first and then the whole-market tables
// for whatever is left.
//
// Per-symbol verdicts:
//
//   - resolved by the primary → whatever the primary reported (ok/no_data/...).
//   - present in a whole-market table → ok (quote from that table).
//   - absent from EVERY table, all of which answered successfully → not_covered:
//     the symbol is outside the published scope of every venue this deployment
//     reads. This is a source-scope fact, not a failed acquisition, and the
//     requirement in issue #1986 is that it must not mark the ranking
//     untrustworthy.
//   - absent while at least one table FAILED → error: we cannot claim the
//     symbol is uncovered when a source that would have covered it was down.
//
// Precondition for the not_covered verdict: the configured sources span every
// venue the requested population is drawn from. The universe population comes
// from the first-party symbol_industry substrate (TWSE 上市 + TPEx 上櫃), so the
// default wiring (TWSE STOCK_DAY_ALL + TPEx daily close) satisfies it by
// construction. A deployment that narrows the source set must expect the extra
// symbols to be reported as not_covered rather than error.
func (p *CoverageCompletingProvider) GetQuotesBatch(ctx context.Context, asOf time.Time, symbols []string) (QuoteBatch, error) {
	batch, primaryErr := GetQuotesBatch(ctx, p.primary, asOf, symbols, QuoteOutcomeError)

	residual := batch.MissingQuote(symbols)
	if len(residual) == 0 || len(p.sources) == 0 {
		return batch, primaryErr
	}

	allSourcesAnswered := true
	for i, src := range p.sources {
		if err := ctx.Err(); err != nil {
			allSourcesAnswered = false
			break
		}
		if len(batch.MissingQuote(symbols)) == 0 {
			break
		}
		snapCtx, cancel := context.WithTimeout(ctx, marketWideSnapshotTimeout)
		quotes, err := p.sourceSnapshot(snapCtx, i, src)
		cancel()
		if err != nil {
			allSourcesAnswered = false
			logging.Warn("coverage_completing", "source_unavailable",
				"source", src.Name(),
				"residual", len(batch.MissingQuote(symbols)),
				logging.Err(err))
			continue
		}
		p.mergeFromSnapshot(&batch, symbols, src.Name(), quotes)
	}

	if remaining := batch.MissingQuote(symbols); len(remaining) > 0 {
		if allSourcesAnswered {
			batch.Resolve(remaining, QuoteOutcomeNotCovered)
		} else {
			// A coverage source failed, so "outside every covered venue" is no
			// longer a fact for anything that relied on it: downgrade the
			// not_covered verdicts the primary chain produced and report the
			// residual as an acquisition failure.
			batch.Downgrade(remaining, QuoteOutcomeError)
			batch.Resolve(remaining, QuoteOutcomeError)
		}
		logging.Info("coverage_completing", "residual_classified",
			"symbols", len(remaining),
			"sources_ok", allSourcesAnswered,
			"sample", sampleSymbols(remaining, 10))
	}

	return batch, primaryErr
}

// sourceSnapshot returns source i's table, served from a short-lived cache.
func (p *CoverageCompletingProvider) sourceSnapshot(ctx context.Context, i int, src MarketWideQuoteSource) ([]domain.Quote, error) {
	p.mu.Lock()
	if i < len(p.snaps) && len(p.snaps[i].quotes) > 0 && time.Since(p.snaps[i].at) < marketWideCacheTTL {
		quotes := p.snaps[i].quotes
		p.mu.Unlock()
		return quotes, nil
	}
	p.mu.Unlock()

	quotes, err := src.Snapshot(ctx)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	for len(p.snaps) <= i {
		p.snaps = append(p.snaps, marketWideSnapshot{})
	}
	p.snaps[i] = marketWideSnapshot{quotes: quotes, at: time.Now()}
	p.mu.Unlock()
	return quotes, nil
}

// mergeFromSnapshot records every requested-but-unresolved symbol the snapshot
// carries, using the same completeness rule as the fallback chain: a row whose
// price is unusable is not a quote, but it does prove the source publishes the
// symbol, so it is reported as no_data rather than left as an acquisition gap.
func (p *CoverageCompletingProvider) mergeFromSnapshot(batch *QuoteBatch, requested []string, source string, snapshot []domain.Quote) {
	if len(snapshot) == 0 {
		return
	}
	missing := batch.MissingQuote(requested)
	if len(missing) == 0 {
		return
	}

	valid, resolved, unusable := usableQuotes(snapshot, missing)
	filled := 0
	for _, q := range valid {
		sym := normalizeQuoteSymbol(q.Symbol)
		if q.Source == "" {
			q.Source = source
		}
		batch.RecordFor(sym, q)
		filled++
	}
	noData := make([]string, 0, len(missing))
	for _, sym := range missing {
		key := normalizeQuoteSymbol(sym)
		if resolved[key] || !unusable[key] {
			continue
		}
		noData = append(noData, sym)
	}
	if len(noData) > 0 {
		batch.SetOutcomeFor(noData, QuoteOutcomeNoData)
	}
	if filled > 0 || len(noData) > 0 {
		logging.Info("coverage_completing", "residual_filled",
			"source", source,
			"filled", filled,
			"no_data", len(noData))
	}
}

// normalizeQuoteSymbol mirrors the pipeline's symbol normalization (strip the
// ".TW" suffix and surrounding whitespace) so a table row and a requested
// symbol always compare equal.
func normalizeQuoteSymbol(symbol string) string {
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(symbol), ".TW"))
}

// sampleSymbols returns up to n symbols, for bounded logging.
func sampleSymbols(symbols []string, n int) string {
	if len(symbols) <= n {
		return fmt.Sprintf("%v", symbols)
	}
	return fmt.Sprintf("%v(+%d more)", symbols[:n], len(symbols)-n)
}
