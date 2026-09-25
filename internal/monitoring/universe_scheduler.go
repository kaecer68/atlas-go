// Package monitoring hosts the SmartUniverseBuilder pipeline and its BTM task
// factories. This file wires the four-layer pipeline (IndustryFilter →
// ScoringScreener → RiskExclusionFilter → NarrativeEventBridge) into
// func(ctx context.Context) error closures (compatible with
// apigateway.BackgroundTaskFunc) registered by cmd/atlas/main.go.
//
// Two tasks are exposed (both gated on the same trigger instant):
//   - Daily refresh (incremental): trading days (Tue–Fri), 14:00 Asia/Taipei
//   - Weekly rebuild (full): Mondays, 14:00 Asia/Taipei
//
// 14:00 Asia/Taipei is 06:00 UTC, the instant that has been in effect in
// production since the tasks were introduced: the atlas container sets no TZ, so
// time.Local there is UTC. The gate is written in Taipei time and compares
// instants, so setting TZ on the host/container can no longer move the trigger.
//
// Both delegate to BuildUniverse, which gathers all symbols, runs the full
// pipeline, and persists the ranked result to data/state/universe_snapshot.json.
package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/monitoring/metrics"
	"github.com/kaecer68/atlas-go/internal/screener"
)

// clockFunc is the time source for scheduler closures. Tests may override
// it to deterministically trigger time-gated branches (isTradingDay,
// alignToTarget, Monday skip). Defaults to time.Now.
var clockFunc = time.Now

// ── Result types ─────────────────────────────────────────────────────────

// Quote-input status values recorded in UniverseBuildResult.QuotesStatus.
//
// The values are machine-readable on purpose: issue #1944 Batch 3 found the
// production artifact reporting symbols_ranked=0 for months while the real
// cause was a nil UniverseBuilderDeps.Quotes (inert-registry.md I25). Nothing
// in the snapshot distinguished "no quote provider was wired" from "the market
// really has no qualifying stock", so the failure was silent.
const (
	// QuotesStatusOK: the quote provider returned at least one quote.
	QuotesStatusOK = "ok"
	// QuotesStatusNotAttempted: the pipeline returned before Step 3 (no symbols
	// gathered, or the industry filter removed every candidate).
	QuotesStatusNotAttempted = "not_attempted"
	// QuotesStatusProviderUnavailable: UniverseBuilderDeps.Quotes was nil, so
	// Step 3 was skipped and the volume/price filters dropped every symbol.
	QuotesStatusProviderUnavailable = "provider_unavailable"
	// QuotesStatusFetchError: the provider was wired but returned an error.
	QuotesStatusFetchError = "fetch_error"
	// QuotesStatusEmpty: the provider returned zero quotes without an error.
	QuotesStatusEmpty = "empty"
	// QuotesStatusPartial: some quote chunks failed while others succeeded. The
	// ranked list is still computed and persisted from the quotes that arrived,
	// but it covers only part of the universe.
	QuotesStatusPartial = "partial"
	// QuotesStatusMock: the provider was wired but identifies itself as a mock,
	// so its (non-empty) answers are fabricated.
	QuotesStatusMock = "mock"
)

// RankedFallbackReason values recorded when SymbolsRanked is not a market
// verdict. Keep these strings stable: they are consumed by operational
// tooling and by the tests that pin this contract.
const (
	// RankedFallbackQuoteProviderUnavailable is recorded when no quote provider
	// was wired (the I25 root cause).
	RankedFallbackQuoteProviderUnavailable = "quote_provider_unavailable"
	// RankedFallbackQuoteFetchError is recorded when the quote fetch failed.
	RankedFallbackQuoteFetchError = "quote_fetch_error"
	// RankedFallbackQuoteFetchEmpty is recorded when the provider answered with
	// an empty quote set.
	RankedFallbackQuoteFetchEmpty = "quote_fetch_empty"
	// RankedFallbackEmptyUniverse is recorded when no symbol was gathered.
	RankedFallbackEmptyUniverse = "empty_universe"
	// RankedFallbackEmptyFiltered is recorded when the industry filter removed
	// every gathered symbol.
	RankedFallbackEmptyFiltered = "empty_filtered"
	// RankedFallbackQuoteFetchPartial is recorded when at least one quote chunk
	// failed: symbols in the failed chunks have no quote, so the ranked list is
	// incomplete and must not be used to expire symbols (D6).
	RankedFallbackQuoteFetchPartial = "quote_fetch_partial"
	// RankedFallbackQuoteProviderMock is recorded when the wired provider is a
	// mock: the ranked list would then describe simulated quotes, not the
	// market, and must never be published as trustworthy.
	RankedFallbackQuoteProviderMock = "quote_provider_mock"
)

// mockQuoteProvider is the optional capability a quote provider exposes when it
// serves fabricated data. marketdata.MockProvider implements it (IsMock).
//
// The pipeline keeps QuoteProvider minimal, so this is a structural check
// rather than an interface requirement. It exists because a mock returns a
// full, plausible quote set for every requested symbol: without this check the
// snapshot would record quotes_status=ok and ranked_trustworthy=true on
// fabricated prices, which is strictly worse than the silent zero this change
// set out to remove.
type mockQuoteProvider interface {
	IsMock() bool
}

// isMockQuoteProvider reports whether p serves fabricated quotes. A nil or
// typed-nil provider is not a mock.
func isMockQuoteProvider(p QuoteProvider) bool {
	m, ok := p.(mockQuoteProvider)
	if !ok || m == nil {
		return false
	}
	return m.IsMock()
}

// UniverseBuildResult captures the outcome of one SmartUniverseBuilder pipeline
// execution. It is serialized into the snapshot file and surfaced to CLI status
// queries so operators can verify the daily/weekly run at a glance.
//
// SymbolsRanked alone is ambiguous: a run with no quote provider produces the
// same symbols_ranked=0 as a run where every candidate failed the volume/price
// filters (and produces an empty `ranked` list that downstream consumers, e.g.
// the D6 watchlist and the coverage alert, silently read as "no symbols").
// QuotesStatus / RankedFallbackReason / RankedTrustworthy disambiguate it, so
// symbols_ranked=0 can never again be misread as a market verdict.
type UniverseBuildResult struct {
	SymbolsBuilt    int       `json:"symbols_built"`
	SymbolsFiltered int       `json:"symbols_filtered"`
	SymbolsRanked   int       `json:"symbols_ranked"`
	SymbolsExcluded int       `json:"symbols_excluded"`
	FullRebuild     bool      `json:"full_rebuild"`
	Timestamp       time.Time `json:"timestamp"`

	// QuotesStatus is what happened at the Step 3 quote fetch. One of the
	// QuotesStatus* constants.
	QuotesStatus string `json:"quotes_status"`
	// QuotesReturned is the number of distinct symbols for which the provider
	// supplied a quote. 0 with QuotesStatus "ok" is impossible (that case is
	// reported as "empty"); comparing it against SymbolsFiltered exposes
	// partial provider coverage that a boolean status cannot express.
	QuotesReturned int `json:"quotes_returned"`
	// QuotesRequested is how many symbols the quote fetch asked for.
	QuotesRequested int `json:"quotes_requested"`
	// QuotesChunks / QuotesChunksFailed expose the fetch granularity: the
	// pipeline calls the provider in bounded chunks (see QuoteFetchPolicy) so a
	// single slow or incomplete chunk cannot take the whole universe down.
	QuotesChunks       int `json:"quotes_chunks"`
	QuotesChunksFailed int `json:"quotes_chunks_failed"`
	// RankedFallbackReason is non-empty when the ranked list is NOT a market
	// verdict, and names the reason. Empty means the ranked list reflects real
	// quote input and may be trusted as-is.
	RankedFallbackReason string `json:"ranked_fallback_reason,omitempty"`
	// RankedTrustworthy is the boolean form of RankedFallbackReason == "".
	// Consumers (alerts, dashboards, watchlist maintenance) must treat
	// symbols_ranked=0 as "universe could not be evaluated" when it is false.
	RankedTrustworthy bool `json:"ranked_trustworthy"`
}

// markRankedUntrustworthy records why SymbolsRanked must not be read as a
// market verdict, and logs it. Every early return and every degraded quote
// path in BuildUniverse goes through here so the pipeline can never again
// publish an unexplained zero.
func markRankedUntrustworthy(result *UniverseBuildResult, reason string, extra ...any) {
	result.RankedFallbackReason = reason
	result.RankedTrustworthy = false
	// Deliberately NOT logging symbols_ranked: every caller runs before Step 4
	// ranking, so the field is still zero here and the line would read like a
	// verdict. The persisted snapshot carries the authoritative counts.
	args := append([]any{
		"reason", reason,
		"symbols_built", result.SymbolsBuilt,
		"symbols_filtered", result.SymbolsFiltered,
		"quotes_status", result.QuotesStatus,
	}, extra...)
	logging.Warn("universe_scheduler", "ranked_not_trustworthy", args...)
}

// ── D6 Watchlist ──────────────────────────────────────────────────────────

// D6WatchlistEntry tracks one symbol that was dropped from the ranked universe
// and moved to the watchlist after 60+ consecutive trading-day failures.
type D6WatchlistEntry struct {
	Symbol              string `json:"symbol"`
	Industry            string `json:"industry"`
	ConsecutiveFailures int    `json:"consecutive_failures"`
	FirstFailureDate    string `json:"first_failure_date"`
	LastCheckDate       string `json:"last_check_date"`
}

// Watchlist is the on-disk representation of the D6 expiry watchlist.
type Watchlist struct {
	Version   string             `json:"version"`
	Symbols   []D6WatchlistEntry `json:"symbols"`
	UpdatedAt string             `json:"updated_at"`
}

// ── Dependencies ──────────────────────────────────────────────────────────

// UniverseBuilderDeps holds every dependency the SmartUniverseBuilder pipeline
// needs. Callers (typically main.go) wire concrete implementations before
// passing the struct to the task factories.
//
// All fields except WorkDir are mandatory; nil providers cause the associated
// pipeline step to be skipped gracefully rather than producing a hard error.
type UniverseBuilderDeps struct {
	// Mapper resolves symbols to industry classifications and enumerates per-industry
	// symbol lists. Used by IndustryFilter and ScoringScreener.
	Mapper SymbolIndustryMapper
	// Tree supplies Level-1/Level-2 taxonomy data so the pipeline can discover
	// the full universe by iterating top-level industries.
	Tree ClassificationTreeAccessor
	// SupplyChain is consulted by IndustryFilter to expand semiconductor-related
	// industries downstream.
	SupplyChain SupplyChainAccessor
	// Screener provides binary pass/fail screening inside ScoringScreener.
	Screener screener.Screener
	// FactorEng computes per-symbol factor scores consumed by ScoringScreener.Rank.
	FactorEng FactorScoreProvider
	// Quotes fetches the latest market quotes for the candidate universe. Used
	// by both ScoringScreener and RiskExclusionFilter.
	Quotes QuoteProvider
	// RiskFilter runs Layer 2.5 risk checks (VaR contribution, volatility,
	// drawdown, liquidity) against the ranked symbols.
	RiskFilter *RiskExclusionFilter
	// NarrativeBridge scrapes RSS/news keywords and caches narrative events for
	// downstream industry cycle/sector allocation consumers.
	NarrativeBridge *NarrativeEventBridge
	// UniverseMetrics is an optional metrics collector for pipeline instrumentation.
	// When nil, instrumentation calls are no-ops.
	UniverseMetrics *metrics.UniverseMetrics
	// Config holds the SP4 §9 tunable parameters that override the scoring
	// screener defaults.
	Config config.SmartUniverseConfig
	// WorkDir is the runtime working directory. The snapshot path is derived as
	// <WorkDir>/data/state/universe_snapshot.json.
	WorkDir string
	// WatchlistMu serializes CheckD6Expiry calls to prevent concurrent
	// read-modify-write on universe_watchlist.json. The caller (main.go)
	// supplies &sync.Mutex{} so all task closures share the same lock.
	// Exported so callers outside this package can wire it.
	WatchlistMu *sync.Mutex

	// QuotePolicy bounds the Step 3 quote fetch granularity. The zero value
	// means "use the package defaults" (50 symbols per call, 100ms pause,
	// 60s per-chunk timeout); see QuoteFetchPolicy for why production must not
	// ask for the whole universe in one call.
	QuotePolicy QuoteFetchPolicy
	// Substrate is the optional per-stock industry field (issue #1943). When
	// installed it replaces the tree's representative stocks as the universe
	// population, growing the built universe from ~27 symbols to the whole
	// listed market. nil (the default, and the only value production uses while
	// industry.substrate_from_symbol_industry_enabled is false) keeps the
	// pre-#1943 pipeline exactly as it was.
	Substrate industry.SymbolIndustrySubstrate
}

// ── Task factories ───────────────────────────────────────────────────────

// NewDailyUniverseRefreshTask returns a task closure compatible with
// apigateway.BackgroundTaskFunc (the raw func(ctx context.Context) error
// signature is used here to avoid a circular monitoring ↔ apigateway import).
// It fires once per minute but only executes the incremental pipeline when:
//
//   - The current day is a trading day (Mon–Fri) in Asia/Taipei.
//   - The wall-clock time is within ±1 minute of 14:00 Asia/Taipei (06:00 UTC).
//
// Registration example (caller casts in main.go):
//
//	_ = taskMgr.Register(&apigateway.ScheduledTask{
//	    Name:     "auto_universe_refresh",
//	    Interval: 1 * time.Minute,
//	    Enabled:  true,
//	    Task:     NewDailyUniverseRefreshTask(deps),
//	})
func NewDailyUniverseRefreshTask(deps UniverseBuilderDeps) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		now := clockFunc()
		// Every gate decision is made in the pipeline's own timezone: the trigger
		// instant is fixed (06:00 UTC), so the weekday of that instant must not
		// depend on the host/container TZ either. Production runs with TZ unset,
		// where UTC and Asia/Taipei agree on the weekday for this instant.
		local := now.In(universeLocation())
		if !isTradingDay(local) {
			logging.Debug("universe_scheduler", "daily_skip_non_trading",
				"day", local.Weekday().String())
			return nil
		}
		if local.Weekday() == time.Monday {
			logging.Debug("universe_scheduler", "daily_skip_monday",
				"note", "weekly rebuild handles Monday")
			return nil
		}
		if !alignToTarget(now) {
			return nil // silent skip — not the trigger minute
		}

		logging.Info("universe_scheduler", "daily_refresh_start")

		prevSymbols := loadPreviousRankedSymbols(deps.WorkDir)

		result, ranked, err := BuildUniverse(ctx, deps, false)
		if err != nil {
			logging.Error("universe_scheduler", "daily_refresh_failed",
				logging.Err(err))
			return fmt.Errorf("daily universe refresh: %w", err)
		}
		logging.Info("universe_scheduler", "daily_refresh_ok",
			"built", result.SymbolsBuilt,
			"filtered", result.SymbolsFiltered,
			"ranked", result.SymbolsRanked,
			"excluded", result.SymbolsExcluded)

		if deps.WatchlistMu != nil {
			deps.WatchlistMu.Lock()
		}
		if err := CheckD6Expiry(deps.WorkDir, ranked, prevSymbols, deps.Mapper, deps.Config.D6ExpiryTradingDays.Value); err != nil {
			if deps.WatchlistMu != nil {
				deps.WatchlistMu.Unlock()
			}
			logging.Warn("universe_scheduler", "d6_expiry_check_error",
				logging.Err(err))
		} else {
			if deps.WatchlistMu != nil {
				deps.WatchlistMu.Unlock()
			}
		}

		// Coverage audit against the FIRST-PARTY population (issue #1943):
		// the denominator is the upstream `symbol_industry` rows, the numerator
		// is the part with a canonical L1 answer. When no first-party population
		// is measurable the report says so instead of printing a green 1.00
		// (the pre-2026-09 audit set total = mapped).
		coverage := CheckUniverseCoverage(deps.Substrate, deps.Mapper, deps.Tree, 0.50)
		logging.Info("universe_scheduler", "coverage_check",
			"source", coverage.Source,
			"mapped", coverage.Mapped,
			"upstream", coverage.Upstream,
			"unmapped", coverage.Unmapped,
			"unknown", coverage.Unknown,
			"ratio", fmt.Sprintf("%.2f", coverage.Ratio),
			"available", coverage.Available,
			"reasons", strings.Join(coverage.Reasons, "; "))
		if coverage.Alert != "" {
			logging.Warn("universe_scheduler", "coverage_alert",
				"alert", coverage.Alert)
		}
		if deps.UniverseMetrics != nil {
			// Label values are unchanged (dashboards depend on them); only the
			// meaning of the pair changes: the denominator is the upstream
			// population instead of the mapped count itself.
			deps.UniverseMetrics.CoverageMapped.WithLabelValues("daily", "all").Add(int64(coverage.Mapped))
			deps.UniverseMetrics.CoverageTotal.WithLabelValues("daily", "all").Add(int64(coverage.Upstream))
		}

		return nil
	}
}

// NewWeeklyUniverseRebuildTask returns a task closure that performs a
// full universe rebuild (clearing cached state, re-fetching all data). It
// fires once per minute but only executes when:
//
// - Today is Monday in Asia/Taipei.
// - The wall-clock time is within ±1 minute of 14:00 Asia/Taipei (06:00 UTC).
//
// The return type is raw func(ctx context.Context) error to avoid a
// circular monitoring ↔ apigateway import; callers assign it directly to
// apigateway.ScheduledTask.Task.
func NewWeeklyUniverseRebuildTask(deps UniverseBuilderDeps) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		now := clockFunc()
		// See the daily task: gate on the pipeline's timezone, not the host TZ.
		local := now.In(universeLocation())
		if local.Weekday() != time.Monday {
			logging.Debug("universe_scheduler", "weekly_skip_not_monday",
				"day", local.Weekday().String())
			return nil
		}
		if !alignToTarget(now) {
			return nil // silent skip
		}

		logging.Info("universe_scheduler", "weekly_rebuild_start")

		prevSymbols := loadPreviousRankedSymbols(deps.WorkDir)

		result, ranked, err := BuildUniverse(ctx, deps, true)
		if err != nil {
			logging.Error("universe_scheduler", "weekly_rebuild_failed",
				logging.Err(err))
			return fmt.Errorf("weekly universe rebuild: %w", err)
		}
		logging.Info("universe_scheduler", "weekly_rebuild_ok",
			"built", result.SymbolsBuilt,
			"filtered", result.SymbolsFiltered,
			"ranked", result.SymbolsRanked,
			"excluded", result.SymbolsExcluded)

		if deps.WatchlistMu != nil {
			deps.WatchlistMu.Lock()
		}
		if err := CheckD6Expiry(deps.WorkDir, ranked, prevSymbols, deps.Mapper, deps.Config.D6ExpiryTradingDays.Value); err != nil {
			if deps.WatchlistMu != nil {
				deps.WatchlistMu.Unlock()
			}
			logging.Warn("universe_scheduler", "d6_expiry_check_error",
				logging.Err(err))
		} else {
			if deps.WatchlistMu != nil {
				deps.WatchlistMu.Unlock()
			}
		}

		// Coverage audit against the FIRST-PARTY population (issue #1943):
		// the denominator is the upstream `symbol_industry` rows, the numerator
		// is the part with a canonical L1 answer. When no first-party population
		// is measurable the report says so instead of printing a green 1.00
		// (the pre-2026-09 audit set total = mapped).
		coverage := CheckUniverseCoverage(deps.Substrate, deps.Mapper, deps.Tree, 0.50)
		logging.Info("universe_scheduler", "coverage_check",
			"source", coverage.Source,
			"mapped", coverage.Mapped,
			"upstream", coverage.Upstream,
			"unmapped", coverage.Unmapped,
			"unknown", coverage.Unknown,
			"ratio", fmt.Sprintf("%.2f", coverage.Ratio),
			"available", coverage.Available,
			"reasons", strings.Join(coverage.Reasons, "; "))
		if coverage.Alert != "" {
			logging.Warn("universe_scheduler", "coverage_alert",
				"alert", coverage.Alert)
		}
		if deps.UniverseMetrics != nil {
			// Label values are unchanged (dashboards depend on them); only the
			// meaning of the pair changes: the denominator is the upstream
			// population instead of the mapped count itself.
			deps.UniverseMetrics.CoverageMapped.WithLabelValues("weekly", "all").Add(int64(coverage.Mapped))
			deps.UniverseMetrics.CoverageTotal.WithLabelValues("weekly", "all").Add(int64(coverage.Upstream))
		}

		return nil
	}
}

// ── Quote fetch policy (issue #1944 Batch 3, I25 production-scale follow-up) ──

// QuoteFetchPolicy bounds how the pipeline asks the provider for quotes.
//
// Reason (production evidence, 2026-09-25): with the per-stock industry
// substrate enabled the universe is the whole listed market (1,599 symbols on
// that day), so Step 3 used to ask for thousands of symbols in one call. Two
// facts make a single all-symbols call unsafe:
//
//   - The fubon-proxy /quotes endpoint loops over the symbol list and issues one
//     Fubon SDK intraday.quote() call per symbol (services/fubon-proxy/main.py),
//     so the "batch" HTTP request is really N upstream calls.
//   - provider.HybridProvider discards the whole batch when any single quote is
//     incomplete (hasInvalidQuotes) and falls through to the next provider, whose
//     FinMind implementation issues one HTTP request per symbol
//     (FinMindProvider.GetQuotes). A single suspended stock therefore used to
//     cost ~1,599 FinMind requests (~11% of the 14,400/day quota).
//
// Chunking bounds both: a slow, failing or incomplete chunk degrades alone, and
// the per-symbol fallback cost shrinks from the whole universe to one chunk.
//
// The zero value means "use the package defaults" so production wiring (and the
// CLI) does not have to know about any of this.
type QuoteFetchPolicy struct {
	// ChunkSize is the maximum number of symbols per provider call.
	ChunkSize int
	// Pause spaces consecutive chunks so a per-symbol provider does not see an
	// instantaneous burst.
	Pause time.Duration
	// ChunkTimeout bounds a single chunk call. 0 inherits the caller's context.
	ChunkTimeout time.Duration
}

// Default chunking parameters. They are deliberately constants rather than
// config parameters: they are operational guardrails, not tunable policy, and
// the values only need to be small enough to bound the blast radius of one
// provider call.
const (
	DefaultQuoteChunkSize    = 50
	DefaultQuoteChunkPause   = 100 * time.Millisecond
	DefaultQuoteChunkTimeout = 60 * time.Second
)

// normalized resolves the zero value to the package defaults. A caller that
// wants a shorter pause (tests) passes a small positive duration; there is no
// "disabled" sentinel because an unset policy must never run unpaced in
// production.
func (p QuoteFetchPolicy) normalized() QuoteFetchPolicy {
	if p.ChunkSize <= 0 {
		p.ChunkSize = DefaultQuoteChunkSize
	}
	if p.Pause == 0 {
		p.Pause = DefaultQuoteChunkPause
	}
	if p.ChunkTimeout == 0 {
		p.ChunkTimeout = DefaultQuoteChunkTimeout
	}
	return p
}

// QuoteFetchStats describes how a chunked quote fetch went. Returned by
// fetchQuotesChunked and copied into UniverseBuildResult.
type QuoteFetchStats struct {
	// Requested is the number of symbols asked for.
	Requested int
	// Chunks is the number of provider calls attempted.
	Chunks int
	// ChunksFailed is how many of them returned an error.
	ChunksFailed int
}

// fetchQuotesChunked asks the provider for quotes in bounded chunks.
//
// It returns every quote that arrived plus the fetch statistics. The error is
// non-nil only when EVERY chunk failed (or the context ended), because a partial
// result is still useful input for ranking — the caller labels it via
// QuotesStatusPartial instead of discarding it.
func fetchQuotesChunked(ctx context.Context, provider QuoteProvider, symbols []string, policy QuoteFetchPolicy) ([]domain.Quote, QuoteFetchStats, error) {
	policy = policy.normalized()
	stats := QuoteFetchStats{Requested: len(symbols)}
	if provider == nil || len(symbols) == 0 {
		return nil, stats, nil
	}

	quotes := make([]domain.Quote, 0, len(symbols))
	var lastErr error
	for start := 0; start < len(symbols); start += policy.ChunkSize {
		if err := ctx.Err(); err != nil {
			return quotes, stats, err
		}
		if start > 0 && policy.Pause > 0 {
			select {
			case <-time.After(policy.Pause):
			case <-ctx.Done():
				return quotes, stats, ctx.Err()
			}
		}
		end := min(start+policy.ChunkSize, len(symbols))
		chunk := symbols[start:end]
		stats.Chunks++

		chunkCtx := ctx
		var cancel context.CancelFunc
		if policy.ChunkTimeout > 0 {
			chunkCtx, cancel = context.WithTimeout(ctx, policy.ChunkTimeout)
		}
		chunkQuotes, err := provider.GetQuotes(chunkCtx, time.Now(), chunk)
		if cancel != nil {
			cancel()
		}
		if err != nil {
			stats.ChunksFailed++
			lastErr = err
			logging.Warn("universe_scheduler", "quotes_chunk_error",
				"chunk_index", stats.Chunks-1,
				"chunk_size", len(chunk),
				logging.Err(err))
			continue
		}
		quotes = append(quotes, chunkQuotes...)
	}

	if stats.ChunksFailed == stats.Chunks && stats.Chunks > 0 {
		return quotes, stats, lastErr
	}
	return quotes, stats, nil
}

// ── Pipeline orchestrator ────────────────────────────────────────────────

// BuildUniverse runs the complete SmartUniverseBuilder pipeline:
//
//  1. Gather all symbols by walking Level-1 industries via Tree.GetLevel1 +
//     Mapper.GetSymbolsByIndustry.
//  2. Layer 1 — IndustryFilter.Filter() narrows the broad universe down to
//     candidates matching the supply-chain depth and cyclicality preferences.
//  3. Fetch latest quotes via Quotes.GetQuotes() for the filtered set.
//  4. Layer 2 — ScoringScreener.Rank() applies volume/price filters, binary
//     screening, weighted 6-factor scoring, concentration cap, and Top-N cut.
//  5. Layer 2.5 — RiskExclusionFilter.Filter() checks VaR contribution,
//     volatility, drawdown, and liquidity against every ranked symbol.
//  6. Layer 3 — NarrativeEventBridge.Scrape() collects RSS/news signals and
//     caches them for downstream industry cycle consumers.
//  7. Persist the ranked symbols and build result to
//     <WorkDir>/data/state/universe_snapshot.json.
//
// When fullRebuild is true, the pipeline treats this as a fresh run; callers
// should have cleared any cached mapper state beforehand. An incremental
// run (fullRebuild=false) reuses cached mapper data and re-scores only.
//
// Nil providers are handled gracefully: the associated step is skipped and
// the corresponding result counter remains zero — but never silently. A
// skipped quote fetch marks the result untrustworthy (QuotesStatus +
// RankedFallbackReason + RankedTrustworthy) so downstream readers cannot
// mistake an unevaluated universe for an empty market.
func BuildUniverse(ctx context.Context, deps UniverseBuilderDeps, fullRebuild bool) (*UniverseBuildResult, []RankedSymbol, error) {
	startTime := time.Now()
	result := &UniverseBuildResult{
		FullRebuild: fullRebuild,
		Timestamp:   startTime,
	}

	stage := "daily"
	if fullRebuild {
		stage = "weekly"
	}

	um := deps.UniverseMetrics

	// Cache single-label counters at task entry to avoid repeated
	// WithLabelValues() resolution across pipeline stages.
	var gatheredCounter, quotesFetchedCounter, rankedCounter, scrapedCounter, snapshotCounter, durationCounter *metrics.Counter
	if um != nil {
		gatheredCounter = um.SymbolsGathered.WithLabelValues(stage)
		quotesFetchedCounter = um.QuotesFetched.WithLabelValues(stage)
		rankedCounter = um.SymbolsRanked.WithLabelValues(stage)
		scrapedCounter = um.NarrativeEventsScraped.WithLabelValues(stage)
		snapshotCounter = um.SnapshotPersisted.WithLabelValues(stage)
		durationCounter = um.PipelineDurationSeconds.WithLabelValues(stage)
	}

	// ── Step 1: Gather all symbols ──────────────────────────────────────

	allSymbols := gatherAllSymbols(deps.Tree, deps.Mapper, deps.Substrate)
	result.SymbolsBuilt = len(allSymbols)
	result.QuotesStatus = QuotesStatusNotAttempted
	if result.SymbolsBuilt == 0 {
		markRankedUntrustworthy(result, RankedFallbackEmptyUniverse)
		return result, nil, nil
	}
	logging.Info("universe_scheduler", "symbols_gathered",
		"count", result.SymbolsBuilt,
		"full_rebuild", fullRebuild)
	if gatheredCounter != nil {
		gatheredCounter.Add(int64(result.SymbolsBuilt))
	}

	// ── Step 2: IndustryFilter ──────────────────────────────────────────

	filter := NewIndustryFilter(deps.Mapper, deps.Tree, deps.SupplyChain)
	filter.ExpandSupplyChainDepth = deps.Config.SupplyChainExpandDepth.Value
	filtered := filter.Filter(allSymbols)
	result.SymbolsFiltered = len(filtered)
	logging.Info("universe_scheduler", "industry_filter_ok",
		"input", len(allSymbols),
		"output", len(filtered))
	if um != nil {
		um.SymbolsFiltered.WithLabelValues(stage, "industry_filter").Add(int64(result.SymbolsFiltered))
		dropped := len(allSymbols) - len(filtered)
		if dropped > 0 {
			um.SymbolsFiltered.WithLabelValues(stage, "dropped").Add(int64(dropped))
		}
	}

	if len(filtered) == 0 {
		markRankedUntrustworthy(result, RankedFallbackEmptyFiltered)
		return result, nil, nil
	}

	// ── Step 3: Fetch quotes ────────────────────────────────────────────

	var quoteMap map[string]domain.Quote
	if deps.Quotes == nil {
		// No quote provider: ScoringScreener.applyVolumeAndPriceFilters drops
		// every symbol (it cannot evaluate volume or price), so SymbolsRanked
		// is guaranteed to be 0 for a reason that has nothing to do with the
		// market. Record it explicitly — this is the I25 root cause and it
		// stayed silent in production until issue #1944 Batch 3.
		quoteMap = make(map[string]domain.Quote)
		result.QuotesStatus = QuotesStatusProviderUnavailable
		markRankedUntrustworthy(result, RankedFallbackQuoteProviderUnavailable)
		if um != nil {
			um.QuotesErrors.WithLabelValues(stage, "provider_unavailable").Inc()
		}
	} else {
		// Chunked fetch (see QuoteFetchPolicy): never ask for the whole universe
		// in one call.
		quotes, fetchStats, err := fetchQuotesChunked(ctx, deps.Quotes, filtered, deps.QuotePolicy)
		result.QuotesRequested = fetchStats.Requested
		result.QuotesChunks = fetchStats.Chunks
		result.QuotesChunksFailed = fetchStats.ChunksFailed
		if err != nil {
			logging.Warn("universe_scheduler", "quotes_fetch_error",
				"chunks", fetchStats.Chunks,
				"chunks_failed", fetchStats.ChunksFailed,
				logging.Err(err))
			if um != nil {
				um.QuotesErrors.WithLabelValues(stage, "fetch_error").Inc()
			}
			result.QuotesStatus = QuotesStatusFetchError
			markRankedUntrustworthy(result, RankedFallbackQuoteFetchError, logging.Err(err))
		}
		quoteMap = make(map[string]domain.Quote, len(quotes))
		for _, q := range quotes {
			quoteMap[normalizeSymbol(q.Symbol)] = q
		}
		if quotesFetchedCounter != nil {
			quotesFetchedCounter.Add(int64(len(quoteMap)))
		}
		result.QuotesReturned = len(quoteMap)
		switch {
		case err != nil:
			// already recorded above (QuotesStatusFetchError).
		case len(quoteMap) == 0:
			// A wired provider that answers with nothing is equally
			// untrustworthy input: an empty quote set cannot rank anything.
			result.QuotesStatus = QuotesStatusEmpty
			markRankedUntrustworthy(result, RankedFallbackQuoteFetchEmpty)
		case isMockQuoteProvider(deps.Quotes):
			// Fabricated quotes must never masquerade as a market verdict.
			result.QuotesStatus = QuotesStatusMock
			markRankedUntrustworthy(result, RankedFallbackQuoteProviderMock)
		case fetchStats.ChunksFailed > 0:
			// Some chunks failed: the ranked list covers only the symbols whose
			// quotes arrived. Symbols in the failed chunks are absent, and every
			// downstream consumer that reads absence as "no longer qualifies"
			// (the D6 expiry counter) would fabricate failures for them, so the
			// partial result is explicitly not trustworthy. The ranking is still
			// computed and persisted for inspection.
			result.QuotesStatus = QuotesStatusPartial
			if um != nil {
				um.QuotesErrors.WithLabelValues(stage, "chunk_error").Add(int64(fetchStats.ChunksFailed))
			}
			markRankedUntrustworthy(result, RankedFallbackQuoteFetchPartial,
				"chunks", fetchStats.Chunks,
				"chunks_failed", fetchStats.ChunksFailed,
				"quotes_returned", len(quoteMap))
		default:
			// Real quote input: the ranked list is a genuine market verdict,
			// even when it is empty after the volume/price filters.
			result.QuotesStatus = QuotesStatusOK
			result.RankedTrustworthy = true
		}
	}

	// ── Step 4: ScoringScreener ─────────────────────────────────────────

	weights := DefaultScreenerWeights()
	cfg := deps.Config
	weights.PE = cfg.PEWeight.Value
	weights.PB = cfg.PBWeight.Value
	weights.Volume = cfg.VolumeWeight.Value
	weights.Momentum = cfg.MomentumWeight.Value
	weights.Quality = cfg.QualityWeight.Value
	weights.ForeignFlow = cfg.ForeignFlowWeight.Value

	ss := NewScoringScreener(deps.Screener, deps.FactorEng)
	ss.Weights = weights
	ss.TopN = cfg.TopN.Value
	ss.VolumeFloorTWD = cfg.VolumeFloorTWD.Value
	ss.PriceMin = cfg.PriceMinimum.Value
	ss.MaxIndustryConcentration = cfg.MaxIndustryConcentration.Value
	ss.IndustryMapper = deps.Mapper
	if cfg.FactorScoreMaxAgeDays.Value > 0 {
		ss.FactorScoreMaxAge = time.Duration(cfg.FactorScoreMaxAgeDays.Value) * 24 * time.Hour
	}

	ranked := ss.Rank(filtered, quoteMap)
	result.SymbolsRanked = len(ranked)
	logging.Info("universe_scheduler", "scoring_ok",
		"input", len(filtered),
		"ranked", len(ranked))
	if um != nil {
		um.SymbolsScreened.WithLabelValues(stage, "passed").Add(int64(len(ranked)))
		if len(filtered) > len(ranked) {
			um.SymbolsScreened.WithLabelValues(stage, "failed").Add(int64(len(filtered) - len(ranked)))
		}
		if rankedCounter != nil {
			rankedCounter.Add(int64(len(ranked)))
		}
	}

	// ── Step 5: RiskExclusionFilter ─────────────────────────────────────

	riskPassed := 0
	riskExcluded := 0
	if deps.RiskFilter != nil && len(ranked) > 0 {
		rankedSymbols := make([]string, len(ranked))
		for i, r := range ranked {
			rankedSymbols[i] = r.Symbol
		}
		riskResults, err := deps.RiskFilter.Filter(rankedSymbols)
		if err != nil {
			logging.Warn("universe_scheduler", "risk_filter_error",
				logging.Err(err))
			if um != nil {
				um.RiskErrors.WithLabelValues(stage, "filter_error").Inc()
			}
		} else {
			for _, rr := range riskResults {
				if rr.Passed {
					riskPassed++
				} else {
					riskExcluded++
					result.SymbolsExcluded++
				}
			}
			logging.Info("universe_scheduler", "risk_filter_ok",
				"checked", len(riskResults),
				"excluded", result.SymbolsExcluded)
			if um != nil {
				um.RiskChecked.WithLabelValues(stage, "passed").Add(int64(riskPassed))
				um.RiskChecked.WithLabelValues(stage, "excluded").Add(int64(riskExcluded))
			}
		}
	}

	// ── Step 6: NarrativeEventBridge ────────────────────────────────────

	if deps.NarrativeBridge != nil {
		events, err := deps.NarrativeBridge.Scrape(ctx)
		if err != nil {
			logging.Warn("universe_scheduler", "narrative_scrape_error",
				logging.Err(err))
			if um != nil {
				um.NarrativeErrors.WithLabelValues(stage, "scrape_error").Inc()
			}
		} else {
			if err := deps.NarrativeBridge.SaveCache(events); err != nil {
				logging.Warn("universe_scheduler", "narrative_cache_save_error",
					logging.Err(err))
				if um != nil {
					um.NarrativeErrors.WithLabelValues(stage, "cache_save_error").Inc()
				}
			}
			logging.Info("universe_scheduler", "narrative_scrape_ok",
				"events", len(events))
			if scrapedCounter != nil {
				scrapedCounter.Add(int64(len(events)))
			}
		}
	}

	// ── Step 7: Persist snapshot ────────────────────────────────────────

	snapshotErr := saveUniverseSnapshot(deps.WorkDir, result, ranked)
	if snapshotErr != nil {
		logging.Warn("universe_scheduler", "snapshot_save_error",
			logging.Err(snapshotErr))
	}

	// Also persist as agents.json-compatible universe registry.
	registryPath := filepath.Join(deps.WorkDir, "data", "state", "universe.json")
	version := 1
	if prev, err := LoadUniverseRegistry(registryPath); err == nil {
		version = prev.Version + 1
	}
	if err := WriteUniverseRegistry(registryPath, result, ranked, version); err != nil {
		logging.Warn("universe_scheduler", "universe_registry_write_error", logging.Err(err))
	}

	if snapshotCounter != nil {
		snapshotCounter.Inc()
		elapsedSec := int64(time.Since(startTime).Seconds())
		if durationCounter != nil {
			durationCounter.Add(elapsedSec)
		}
	}

	return result, ranked, nil
}

// ── Helpers ──────────────────────────────────────────────────────────────

// GatherUniverseSymbols is the exported entry point to the pipeline's symbol
// population. Non-scheduler callers (the -build-universe CLI) must use it so
// the CLI and the scheduled pipeline agree on what the universe contains
// instead of each deriving its own list.
func GatherUniverseSymbols(tree ClassificationTreeAccessor, mapper SymbolIndustryMapper, substrate industry.SymbolIndustrySubstrate) []string {
	return gatherAllSymbols(tree, mapper, substrate)
}

// gatherAllSymbols collects every known symbol.
//
// With a per-stock industry substrate installed (issue #1943) the population is
// the substrate's symbol list — the whole listed market (TWSE 上市 + TPEx
// 上櫃) mapped to canonical L1 — instead of the ~27 representative stocks the
// classification tree declares. Without one it iterates Level-1 industries via
// Tree.GetLevel1 and expands each through Mapper.GetSymbolsByIndustry, exactly
// as before.
//
// Duplicates are removed and symbols are normalized in both paths.
func gatherAllSymbols(tree ClassificationTreeAccessor, mapper SymbolIndustryMapper, substrate industry.SymbolIndustrySubstrate) []string {
	if population := substratePopulation(substrate); len(population) > 0 {
		return population
	}
	if tree == nil || mapper == nil {
		return nil
	}
	l1 := tree.GetLevel1()
	if len(l1) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var symbols []string
	add := func(sym string) {
		norm := normalizeSymbol(sym)
		if !seen[norm] {
			seen[norm] = true
			symbols = append(symbols, norm)
		}
	}
	for _, seg := range l1 {
		for _, sym := range mapper.GetSymbolsByIndustry(seg.ID) {
			add(sym)
		}
	}
	return symbols
}

// UniverseSnapshotPath returns the canonical location of the universe
// snapshot for a working directory. Every writer and reader of the snapshot
// must resolve the path through this function so the two can never drift
// apart (see UniverseSnapshot for the history of that failure).
func UniverseSnapshotPath(workDir string) string {
	return filepath.Join(workDir, "data", "state", "universe_snapshot.json")
}

// UniverseSnapshot is the canonical on-disk shape of the universe snapshot.
//
// There is exactly ONE schema. Both the scheduled pipeline (BuildUniverse) and
// the `atlas -build-universe run` CLI write it through SaveUniverseSnapshot,
// and every reader — the D6 watchlist (loadPreviousRankedSymbols), the
// universe-coverage alert in cmd/atlas/main.go, and `-build-universe status` —
// reads it back through LoadUniverseSnapshot.
//
// Before issue #1944 Batch 3 the CLI wrote a second, incompatible schema
// (build_time / ranked_count / top_symbols) to the same path. Both schemas
// unmarshal into the other's reader without error, so each reader silently saw
// zero: the scheduled pipeline lost its previous ranked list (breaking D6
// tracking) and the CLI status command reported an empty universe (N-U3).
type UniverseSnapshot struct {
	// Result carries the run counters and the quote-input evidence fields.
	Result *UniverseBuildResult `json:"result"`
	// Ranked is the ordered ranked universe. It is empty when Result is not
	// trustworthy (see UniverseBuildResult.RankedTrustworthy).
	Ranked []RankedSymbol `json:"ranked"`
}

// SaveUniverseSnapshot writes the canonical snapshot (result + ranked) to
// <workDir>/data/state/universe_snapshot.json, creating directories as needed.
// It is exported so non-scheduler callers (the -build-universe CLI) emit the
// same schema instead of inventing their own.
func SaveUniverseSnapshot(workDir string, result *UniverseBuildResult, ranked []RankedSymbol) error {
	outDir := filepath.Dir(UniverseSnapshotPath(workDir))
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return fmt.Errorf("create snapshot directory %q: %w", outDir, err)
	}

	data, err := json.MarshalIndent(UniverseSnapshot{Result: result, Ranked: ranked}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal universe snapshot: %w", err)
	}

	path := UniverseSnapshotPath(workDir)
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o640); err != nil {
		return fmt.Errorf("write universe snapshot tmp %q: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename universe snapshot %q: %w", tmpPath, err)
	}
	return nil
}

// LoadUniverseSnapshot reads the canonical snapshot written by
// SaveUniverseSnapshot. A missing file surfaces as an error wrapping
// fs.ErrNotExist so callers can distinguish "never ran" from "corrupt".
func LoadUniverseSnapshot(workDir string) (*UniverseSnapshot, error) {
	path := UniverseSnapshotPath(workDir)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read universe snapshot %q: %w", path, err)
	}
	var snap UniverseSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return nil, fmt.Errorf("unmarshal universe snapshot %q: %w", path, err)
	}
	return &snap, nil
}

// saveUniverseSnapshot is the scheduler-internal alias kept for readability of
// the pipeline call site. See SaveUniverseSnapshot for the contract.
func saveUniverseSnapshot(workDir string, result *UniverseBuildResult, ranked []RankedSymbol) error {
	return SaveUniverseSnapshot(workDir, result, ranked)
}

// loadPreviousRankedSymbols reads the most recent universe snapshot from disk
// and extracts just the symbol names from the ranked list. Returns nil when
// no snapshot exists (first run).
//
// It reads the canonical schema through LoadUniverseSnapshot. A snapshot
// written by a non-canonical writer unmarshals into this shape without error
// and yields an empty ranked list, which is why N-U3 was silent for so long;
// UniverseSnapshot documents the single-schema requirement.
func loadPreviousRankedSymbols(workDir string) []string {
	snap, err := LoadUniverseSnapshot(workDir)
	if err != nil {
		// A missing snapshot is the normal first-run case; anything else
		// (corrupt JSON, permissions) is worth reporting.
		if !errors.Is(err, fs.ErrNotExist) {
			logging.Warn("universe_scheduler", "load_previous_ranked_error",
				logging.Err(err))
		}
		return nil
	}
	if snap.Result != nil && !snap.Result.RankedTrustworthy {
		// The previous ranked list was produced without usable quote input.
		// Returning it as "previously ranked" would fabricate D6 failures for
		// every symbol, so treat it as no previous list at all.
		logging.Warn("universe_scheduler", "load_previous_ranked_untrustworthy",
			"reason", snap.Result.RankedFallbackReason,
			"quotes_status", snap.Result.QuotesStatus)
		return nil
	}

	symbols := make([]string, 0, len(snap.Ranked))
	for _, r := range snap.Ranked {
		symbols = append(symbols, normalizeSymbol(r.Symbol))
	}
	return symbols
}

// isTradingDay returns true when t falls on a weekday (Mon–Fri).
func isTradingDay(t time.Time) bool {
	switch t.Weekday() {
	case time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday:
		return true
	default:
		return false
	}
}

// Universe pipeline trigger time, expressed in the pipeline's own timezone.
//
// universeTriggerHourTW = 14 (Asia/Taipei) == 06:00 UTC, which is the instant
// that actually fires in production. 14:00 Taipei is *after* the 13:30 close, so
// the pipeline runs on the previous session's data — this constant documents the
// observed behavior, it does not change it.
const (
	taipeiTZName          = "Asia/Taipei"
	taipeiOffsetSeconds   = 8 * 60 * 60
	universeTriggerHourTW = 14
)

// universeLocation returns the timezone the pipeline's schedule is expressed in
// (Asia/Taipei).
//
// Taiwan has been on a fixed +08:00 offset without DST since 1979, so the fixed
// zone is exact; it exists only as a fallback for minimal images that cannot load
// the IANA database (Alpine without tzdata). Failing loudly is not an option here
// — the previous code silently used the host TZ instead, which is the defect this
// guards against — and a UTC fallback would shift the trigger by 8 hours.
func universeLocation() *time.Location {
	if loc, err := time.LoadLocation(taipeiTZName); err == nil {
		return loc
	}
	return time.FixedZone(taipeiTZName, taipeiOffsetSeconds)
}

// alignToTarget returns true when now is within ±1 minute of the pipeline trigger
// instant (14:00 Asia/Taipei == 06:00 UTC).
//
// The comparison is instant-based: now may carry any location, the trigger instant
// does not move with it. Until 2026-09-25 the target was built from
// now.Location() at 06:00, so the real trigger silently followed the host/container
// TZ (production runs with no TZ, i.e. 06:00 UTC = 14:00 Taipei, while the comment
// claimed 06:00 Taipei "market open" — the market opens at 09:00).
func alignToTarget(now time.Time) bool {
	loc := universeLocation()
	local := now.In(loc)
	target := time.Date(local.Year(), local.Month(), local.Day(), universeTriggerHourTW, 0, 0, 0, loc)
	diff := now.Sub(target)
	if diff < 0 {
		diff = -diff
	}
	return diff <= time.Minute
}

// ── D6 Expiry ────────────────────────────────────────────────────────────

// CheckD6Expiry maintains the watchlist for symbols that have been dropped from
// the ranked universe. Symbols failing to re-enter the ranked list for
// expiryDays consecutive trading days are moved to the watchlist. The watchlist
// is persisted to <workDir>/data/state/universe_watchlist.json using an atomic write.
//
// previousUniverseSymbols provides the ranked symbols from the last pipeline run.
// ranked is the current (today's) ranked symbol list.
func CheckD6Expiry(workDir string, ranked []RankedSymbol, previousUniverseSymbols []string, mapper SymbolIndustryMapper, expiryDays int) error {
	now := time.Now()
	today := now.Format("2006-01-02")

	outDir := filepath.Join(workDir, "data", "state")
	watchlistPath := filepath.Join(outDir, "universe_watchlist.json")

	wl := Watchlist{Version: "1", UpdatedAt: now.Format(time.RFC3339)}

	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return fmt.Errorf("create watchlist directory %q: %w", outDir, err)
	}

	raw, err := os.ReadFile(watchlistPath)
	if err == nil {
		if unmarshalErr := json.Unmarshal(raw, &wl); unmarshalErr != nil {
			logging.Warn("universe_scheduler", "watchlist_unmarshal_error",
				logging.Err(unmarshalErr))
			wl = Watchlist{Version: "1", UpdatedAt: now.Format(time.RFC3339)}
		}
	}

	entryBySymbol := make(map[string]*D6WatchlistEntry)
	for i := range wl.Symbols {
		entryBySymbol[wl.Symbols[i].Symbol] = &wl.Symbols[i]
	}

	currentSet := make(map[string]bool, len(ranked))
	for _, r := range ranked {
		currentSet[normalizeSymbol(r.Symbol)] = true
	}

	// Increment ConsecutiveFailures for symbols previously ranked but missing today
	previouslyRankedSet := make(map[string]bool, len(previousUniverseSymbols))
	for _, sym := range previousUniverseSymbols {
		previouslyRankedSet[normalizeSymbol(sym)] = true
	}

	// Collect new entries separately to avoid slice realloc
	// invalidating entryBySymbol pointers (see C4).
	var newEntries []D6WatchlistEntry

	for sym := range previouslyRankedSet {
		if currentSet[sym] {
			continue
		}
		entry, exists := entryBySymbol[sym]
		if !exists {
			cls := inferredIndustry(sym, mapper)
			newEntries = append(newEntries, D6WatchlistEntry{
				Symbol:              sym,
				Industry:            cls,
				ConsecutiveFailures: 1,
				FirstFailureDate:    today,
				LastCheckDate:       today,
			})
		} else {
			entry.ConsecutiveFailures++
			entry.LastCheckDate = today
			if entry.FirstFailureDate == "" {
				entry.FirstFailureDate = today
			}
		}
	}

	// Append all new entries at once, then rebuild the pointer map
	// so subsequent mutations go to the final backing array.
	wl.Symbols = append(wl.Symbols, newEntries...)
	entryBySymbol = make(map[string]*D6WatchlistEntry, len(wl.Symbols))
	for i := range wl.Symbols {
		entryBySymbol[wl.Symbols[i].Symbol] = &wl.Symbols[i]
	}

	// Reset failures for symbols that returned to ranked
	for sym := range currentSet {
		if entry, exists := entryBySymbol[sym]; exists {
			entry.ConsecutiveFailures = 0
			entry.LastCheckDate = today
		}
	}

	// Log and track D6-expired symbols
	expiredCount := 0
	for i := range wl.Symbols {
		e := &wl.Symbols[i]
		if e.ConsecutiveFailures >= expiryDays {
			expiredCount++
			if e.ConsecutiveFailures == expiryDays {
				logging.Warn("universe_scheduler", "d6_expiry_entered",
					"symbol", e.Symbol,
					"industry", e.Industry,
					"consecutive_failures", e.ConsecutiveFailures,
					"first_failure_date", e.FirstFailureDate)
			}
		}
	}

	if expiredCount > 0 {
		logging.Warn("universe_scheduler", "d6_watchlist_summary",
			"expired_symbols", expiredCount,
			"total_tracked", len(wl.Symbols))
	}

	wl.UpdatedAt = now.Format(time.RFC3339)

	data, err := json.MarshalIndent(wl, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal watchlist: %w", err)
	}

	tmpPath := watchlistPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o640); err != nil {
		return fmt.Errorf("write watchlist tmp %q: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, watchlistPath); err != nil {
		return fmt.Errorf("rename watchlist %q: %w", tmpPath, err)
	}

	return nil
}

// inferredIndustry resolves the industry classification for sym using the
// mapper, which queries the full classification tree (not just today's
// ranked set). Falls back to "unknown" when no classification is found.
func inferredIndustry(sym string, mapper SymbolIndustryMapper) string {
	cls, ok := mapper.GetClassification(sym)
	if !ok || cls.Level1.Name == "" {
		return "unknown"
	}
	return cls.Level1.Name
}

// ── Coverage Check ────────────────────────────────────────────────────────

// CoverageReport is the outcome of one universe coverage audit.
//
// The audit exists to answer one question honestly: how much of the population
// the first-party `symbol_industry` channel saw was actually classified? The
// pre-2026-09 audit could not answer it -- it set total = mapped, so Ratio was
// 1.00 by construction and the alert could never fire (production logged
// `coverage_check mapped=27 total=27 ratio=1.00` right after the daily refresh
// while the pipeline had already moved to a 1599-symbol population).
type CoverageReport struct {
	// Available is false when no first-party population is measurable. In that
	// case the other numeric fields carry no coverage meaning and Alert is set:
	// not measurable must be reported as such, never as 100 %.
	Available bool
	// Source is "symbol_industry" when the audit ran against the first-party
	// population, and "unavailable" otherwise.
	Source string
	// Upstream is the denominator: the upstream rows the first-party channel
	// saw (TWSE 上市 + TPEx 上櫃 company/industry reports).
	Upstream int
	// Mapped is the numerator: rows with a canonical L1 answer, which is
	// exactly the population the pipeline builds from.
	Mapped int
	// Unmapped / Unknown are the two documented unresolved reasons (declared
	// code without a defensible canonical L1; code not declared at all).
	Unmapped int
	Unknown  int
	// Reasons carries the distinct reasons the unresolved rows report.
	Reasons []string
	// Ratio is Mapped/Upstream; 0 when !Available.
	Ratio float64
	// Alert is non-empty when the coverage must be looked at: either it is not
	// measurable or it fell below the threshold.
	Alert string
}

// coverageSourceUnavailable marks an audit that could not measure the
// first-party population. The other source value is
// industry.L1SourceSymbolIndustry ("symbol_industry"), reused so the audit and
// the resolver report the same source name.
const coverageSourceUnavailable = "unavailable"

// CheckUniverseCoverage audits how much of the first-party `symbol_industry`
// population the substrate could resolve to a canonical L1 sector.
//
// Semantics (deliberate audit-only change, see the PR body):
//
//   - substrate installed AND implements
//     industry.SymbolIndustryCoverageReporter: Source = "symbol_industry",
//     Upstream/Mapped/Unmapped/Unknown/Reasons come from the reporter, and
//     Ratio = Mapped/Upstream. Upstream == 0 (nothing loaded / empty channel)
//     is NOT full coverage: it is reported as unavailable with an alert.
//   - no substrate, or a substrate that does not report coverage: Available =
//     false, Source = "unavailable", Ratio = 0, and an explicit alert saying the
//     first-party population is not measurable. The alert names the legacy
//     representative-stock table (TotalClassifiedSymbols) and the population the
//     pipeline would build instead, so nobody mistakes either for a market-wide
//     denominator.
//
// mapper and tree are used only to describe that fallback population; this
// function never changes the population, the scoring or the ordering. A nil
// mapper or tree is tolerated (a nil tree yields 0 legacy symbols).
func CheckUniverseCoverage(substrate industry.SymbolIndustrySubstrate, mapper SymbolIndustryMapper, tree ClassificationTreeAccessor, threshold float64) CoverageReport {
	report := CoverageReport{Source: coverageSourceUnavailable}

	if substrate == nil {
		report.Alert = coverageUnavailableAlert(
			"no per-stock industry substrate is installed", substrate, mapper, tree)
		return report
	}
	reporter, ok := substrate.(industry.SymbolIndustryCoverageReporter)
	if !ok {
		report.Alert = coverageUnavailableAlert(
			"the installed substrate does not report the first-party population", substrate, mapper, tree)
		return report
	}

	cov := reporter.Coverage()
	report.Source = industry.L1SourceSymbolIndustry
	report.Upstream = cov.Upstream
	report.Mapped = cov.Resolved
	report.Unmapped = cov.Unmapped
	report.Unknown = cov.Unknown
	report.Reasons = append([]string(nil), cov.Reasons...)

	if report.Upstream <= 0 {
		report.Alert = fmt.Sprintf(
			"coverage not measurable: the first-party symbol_industry population is empty (upstream=%d resolved=%d)",
			report.Upstream, report.Mapped)
		return report
	}

	report.Available = true
	report.Ratio = float64(report.Mapped) / float64(report.Upstream)
	if report.Ratio < threshold {
		report.Alert = fmt.Sprintf(
			"coverage %.2f%% below threshold %.2f%% (upstream=%d mapped=%d unmapped=%d unknown=%d)",
			report.Ratio*100, threshold*100,
			report.Upstream, report.Mapped, report.Unmapped, report.Unknown)
	}
	return report
}

// coverageUnavailableAlert describes an audit that could not measure the
// first-party population. The message must leave no room for a green reading:
// it states that coverage is unknown, then the two numbers an operator might be
// tempted to use instead -- the population the pipeline would build right now
// (population) and the legacy representative-stock table the classification tree
// declares (legacy) -- and says plainly that neither is a market-wide
// denominator.
func coverageUnavailableAlert(reason string, substrate industry.SymbolIndustrySubstrate, mapper SymbolIndustryMapper, tree ClassificationTreeAccessor) string {
	population := len(gatherAllSymbols(tree, mapper, substrate))
	legacy := TotalClassifiedSymbols(tree)
	return fmt.Sprintf(
		"coverage not measurable: %s, so the upstream market-wide population is unknown; "+
			"the pipeline population is now %d symbols and the legacy representative-stock table declares %d, "+
			"neither of which is a market-wide denominator",
		reason, population, legacy)
}

// TotalClassifiedSymbols counts the total number of representative stocks across
// all Level-1 industry segments visible through the classification tree.
// It sums len(seg.RepresentativeStocks) for every top-level segment.
func TotalClassifiedSymbols(tree ClassificationTreeAccessor) int {
	if tree == nil {
		return 0
	}
	total := 0
	for _, seg := range tree.GetLevel1() {
		total += len(seg.RepresentativeStocks)
	}
	return total
}
