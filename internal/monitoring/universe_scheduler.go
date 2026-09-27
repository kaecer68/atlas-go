// Package monitoring hosts the SmartUniverseBuilder pipeline and its BTM task
// factories. This file wires the four-layer pipeline (IndustryFilter →
// ScoringScreener → RiskExclusionFilter → NarrativeEventBridge) into
// func(ctx context.Context) error closures (compatible with
// apigateway.BackgroundTaskFunc) registered by cmd/atlas/main.go.
//
// Two tasks are exposed (both gated on the same trigger instant):
//   - Daily refresh (incremental): trading days (Tue–Fri), 14:00 Asia/Taipei
//   - Weekly rebuild (full): trading-day Mondays, 14:00 Asia/Taipei
//
// "Trading day" means the single-source Taiwan calendar
// (marketdata.IsTaiwanTradingDay → internal/taiwanholidays) — weekends AND public
// holidays. This file used to carry its own weekday-only predicate; it ran the
// pipeline on 附市日 (2026-09-28, the first weekday 教師節 after the 2025
// restoration, was the case found) and is deleted instead of kept in sync.
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
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/metrics"
	"github.com/kaecer68/atlas-go/internal/screener"
)

// clockFunc is the time source for scheduler closures. Tests may override
// it to deterministically trigger time-gated branches (trading-day gate,
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
	// QuotesMissingNoData / QuotesMissingNotCovered / QuotesMissingFetchError /
	// QuotesMissingNotAttempted classify every requested symbol that came back
	// without a quote (issue #1986 requirement 3). Only the last two are
	// acquisition failures; the first two are facts about the market or about
	// the scope of the sources this deployment reads, and must be auditable
	// separately so an operator can see whether a gap is ours or upstream's.
	//
	//   - quotes_missing_no_data: a source answered authoritatively that the
	//     symbol has no tradable data (suspended, no trades that day).
	//   - quotes_missing_not_covered: the symbol is absent from every
	//     successfully fetched whole-market table (TWSE 上市 + TPEx 上櫃).
	//   - quotes_missing_fetch_error: transport error, timeout, rate limit,
	//     quota exhaustion, circuit breaker.
	//   - quotes_missing_not_attempted: no arm ever asked for the symbol.
	QuotesMissingNoData       int `json:"quotes_missing_no_data"`
	QuotesMissingNotCovered   int `json:"quotes_missing_not_covered"`
	QuotesMissingFetchError   int `json:"quotes_missing_fetch_error"`
	QuotesMissingNotAttempted int `json:"quotes_missing_not_attempted"`
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
//   - The current day is a Taiwan trading day in Asia/Taipei: a weekday that is
//     not a public holiday (marketdata.IsTaiwanTradingDay → internal/taiwanholidays).
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
		// Gate ORDER is a volume decision, not a behavior decision: the pipeline
		// runs only when every gate passes, so the outcome is identical either way.
		// What changes is how often a gate can SPEAK. This closure is registered
		// with Interval: 1m, so a gate that logs before the alignment check logs on
		// every tick — 1440 lines per non-trading day, and 1440 every Monday for
		// the weekday gate. Keeping the observable decisions inside the ±1 minute
		// alignment window bounds them to the ticks in that window (2-3 per
		// calendar day). NewWeeklyUniverseRebuildTask already ordered its gates
		// this way for exactly this reason; the daily closure had the opposite
		// order, which was invisible only because both of its skip logs were at
		// Debug (see below). The volume claim is pinned by
		// TestDailySkip_InfoLevelAndDailyVolume, not by this comment.
		if !alignToTarget(now) {
			return nil // silent skip — not the trigger minute
		}
		// Single-source judgement: marketdata.IsTaiwanTradingDay delegates to
		// internal/taiwanholidays, so a weekday that is a public holiday (2026-09-28
		// 教師節, 2026-10-09 國慶補假, ...) is not a trading day here.
		//
		// Both skips below used to be logged at Debug while production runs at
		// ATLAS_LOG_LEVEL=info, so "the market was closed" / "it is Monday" — the
		// evidence for "the pipeline did not run today, and that is correct" —
		// left no readable trace at all. That is the same class of defect as the
		// counters: the decisive fact existed and no consumer could read it. They
		// are Info now, at the volume argued above.
		if !marketdata.IsTaiwanTradingDay(local) {
			logging.Info("universe_scheduler", "daily_skip_non_trading",
				"date", local.Format("2006-01-02"),
				"weekday", local.Weekday().String(),
				"criterion", "marketdata.IsTaiwanTradingDay")
			return nil
		}
		if local.Weekday() == time.Monday {
			logging.Info("universe_scheduler", "daily_skip_monday",
				"note", "weekly rebuild handles Monday")
			return nil
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
		// FU-20260925-01: age the audit in the log line itself, so staleness is
		// visible without a new metric or alert rule. as_of unknown (the zero
		// instant) means no successful substrate load has ever been observed;
		// it is logged as "unknown" / -1 and never as "now".
		coverageAsOf, coverageAgeHours := "unknown", float64(-1)
		if !coverage.AsOf.IsZero() {
			coverageAsOf = coverage.AsOf.UTC().Format(time.RFC3339)
			coverageAgeHours = time.Since(coverage.AsOf).Hours()
		}
		logging.Info("universe_scheduler", "coverage_check",
			"source", coverage.Source,
			"mapped", coverage.Mapped,
			"upstream", coverage.Upstream,
			"unmapped", coverage.Unmapped,
			"unknown", coverage.Unknown,
			"ratio", fmt.Sprintf("%.2f", coverage.Ratio),
			"available", coverage.Available,
			"reasons", strings.Join(coverage.Reasons, "; "),
			"as_of", coverageAsOf,
			"data_age_hours", coverageAgeHours,
			"load_error", coverage.LoadError)
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
//   - Today is Monday in Asia/Taipei.
//   - Today is a Taiwan trading day: a Monday public holiday (教師節 2026-09-28,
//     行憲紀念日 2026-12-25, ...) is skipped with an explicit
//     `weekly_skip_holiday` log — the market is closed, so a full rebuild would
//     only re-snapshot the previous session's quotes.
//   - The wall-clock time is within ±1 minute of 14:00 Asia/Taipei (06:00 UTC).
//
// The trading-day check runs after alignToTarget on purpose: the closure fires
// every minute, and the holiday decision is only worth one log line at the
// trigger instant instead of 60.
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
		if !marketdata.IsTaiwanTradingDay(local) {
			logging.Info("universe_scheduler", "weekly_skip_holiday",
				"date", local.Format("2006-01-02"),
				"weekday", local.Weekday().String(),
				"criterion", "marketdata.IsTaiwanTradingDay",
				"note", "market closed — no new session to rebuild from")
			return nil
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
		// FU-20260925-01: age the audit in the log line itself, so staleness is
		// visible without a new metric or alert rule. as_of unknown (the zero
		// instant) means no successful substrate load has ever been observed;
		// it is logged as "unknown" / -1 and never as "now".
		coverageAsOf, coverageAgeHours := "unknown", float64(-1)
		if !coverage.AsOf.IsZero() {
			coverageAsOf = coverage.AsOf.UTC().Format(time.RFC3339)
			coverageAgeHours = time.Since(coverage.AsOf).Hours()
		}
		logging.Info("universe_scheduler", "coverage_check",
			"source", coverage.Source,
			"mapped", coverage.Mapped,
			"upstream", coverage.Upstream,
			"unmapped", coverage.Unmapped,
			"unknown", coverage.Unknown,
			"ratio", fmt.Sprintf("%.2f", coverage.Ratio),
			"available", coverage.Available,
			"reasons", strings.Join(coverage.Reasons, "; "),
			"as_of", coverageAsOf,
			"data_age_hours", coverageAgeHours,
			"load_error", coverage.LoadError)
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

// QuoteFetchStats describes how a chunked quote fetch went, including why any
// requested symbol came back without a quote. Returned by fetchQuotesChunked and
// copied into UniverseBuildResult.
type QuoteFetchStats struct {
	// Requested is the number of symbols asked for.
	Requested int
	// Chunks is the number of provider calls attempted.
	Chunks int
	// ChunksFailed is how many of them returned an error.
	ChunksFailed int

	// Resolved is the number of symbols for which a quote arrived.
	Resolved int
	// NoData is the number of symbols a source answered authoritatively as
	// having no tradable data (suspended, no trades that day). A market fact.
	NoData int
	// NotCovered is the number of symbols absent from every successfully
	// fetched whole-market table: outside the published scope of the venues
	// this deployment reads. A source-scope fact (issue #1986 requirement 3).
	NotCovered int
	// FetchError is the number of symbols that could not be acquired at all
	// (transport error, timeout, rate limit, quota, circuit breaker).
	FetchError int
	// NotAttempted is the number of symbols no arm ever asked for. It is an
	// acquisition gap, kept separate so a budget skip is not silently read as a
	// source-scope fact.
	NotAttempted int
}

// UnresolvedFailures is the number of requested symbols whose absence is an
// acquisition failure rather than a market/source fact. It is the number the
// ranked_trustworthy gate reads.
func (s QuoteFetchStats) UnresolvedFailures() int {
	return s.FetchError + s.NotAttempted
}

// fetchQuotesChunked asks the provider for quotes in bounded chunks.
//
// It returns every quote that arrived plus the fetch statistics. The error is
// non-nil only when EVERY chunk failed (or the context ended), because a partial
// result is still useful input for ranking — the caller labels it via
// QuotesStatusPartial instead of discarding it.
//
// Providers that implement marketdata.PartialBatchProvider are asked through
// GetQuotesBatch, so the statistics can classify each missing symbol. Providers
// that only implement QuoteProvider are asked through GetQuotes and every
// symbol they drop is counted as a fetch error: absence from a source that
// cannot say why it is absent must not be reported as a market fact.
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
		batch, err := quoteBatchForChunk(chunkCtx, provider, chunk)
		if cancel != nil {
			cancel()
		}
		if err != nil {
			stats.ChunksFailed++
			stats.FetchError += len(chunk)
			lastErr = err
			logging.Warn("universe_scheduler", "quotes_chunk_error",
				"chunk_index", stats.Chunks-1,
				"chunk_size", len(chunk),
				logging.Err(err))
			continue
		}
		quotes = append(quotes, batch.Quotes...)
		for _, sym := range chunk {
			switch batch.Outcomes[sym] {
			case marketdata.QuoteOutcomeOK:
				stats.Resolved++
			case marketdata.QuoteOutcomeNoData:
				stats.NoData++
			case marketdata.QuoteOutcomeNotCovered:
				stats.NotCovered++
			case marketdata.QuoteOutcomeNotAttempted:
				stats.NotAttempted++
			default:
				stats.FetchError++
			}
		}
	}

	if stats.ChunksFailed == stats.Chunks && stats.Chunks > 0 {
		return quotes, stats, lastErr
	}
	return quotes, stats, nil
}

// quoteBatchForChunk asks one chunk's provider for quotes and returns the
// per-symbol verdict.
//
// A provider that can classify (marketdata.PartialBatchProvider: the hybrid
// chain and the whole-market coverage layer) answers for itself. A provider
// that cannot has every absent symbol recorded as a fetch error: the caller then
// knows the ranking is incomplete for a reason it cannot verify, which is the
// honest reading and keeps the old behavior for every un-migrated provider.
func quoteBatchForChunk(ctx context.Context, provider QuoteProvider, chunk []string) (marketdata.QuoteBatch, error) {
	if aware, ok := provider.(marketdata.PartialBatchProvider); ok {
		return aware.GetQuotesBatch(ctx, time.Now(), chunk)
	}
	quotes, err := provider.GetQuotes(ctx, time.Now(), chunk)
	batch := marketdata.NewQuoteBatch(chunk)
	for _, q := range quotes {
		batch.Record(q)
	}
	batch.Resolve(chunk, marketdata.QuoteOutcomeError)
	return batch, err
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

	// Publish the run verdict from ONE point: a defer, not one call per return.
	// Every exit path (success, Step 1/2 early return, degraded quote fetch,
	// error) then reports what it produced, and no future edit can leave a path
	// that publishes nothing — the 2026-09-25 incident was exactly "the pipeline
	// ran, the snapshot was healthy, and two signals never left it".
	// The verdict also refreshes the schedule heartbeat (see
	// universe_run_verdict.go), so a missed trigger is visible without any
	// holiday-blind window arithmetic in the alert rules.
	// snapshotPersisted records whether THIS run's output was verified on disk
	// (Step 7 reads the file back). It is deliberately a local of the run, not a
	// field of UniverseBuildResult: the result is what gets WRITTEN to the
	// snapshot, so it cannot contain a statement about the write that produces it.
	// The defer below is the only reader, so no exit path can publish the verdict
	// of a different run.
	//
	// registryPersisted is the same measurement for the SECOND artifact Step 7
	// writes (data/state/universe.json). It is a separate local for the same
	// reason and one more: the two artifacts fail independently (different
	// writers, different consumers), and a single collapsed signal could not tell
	// the operator which file to stat.
	var snapshotPersisted, registryPersisted bool
	if um != nil {
		defer func() {
			verdict := UniverseRunVerdictFor(result, stage, time.Now())
			verdict.SnapshotPersisted = snapshotPersisted
			verdict.RegistryPersisted = registryPersisted
			um.ReportRun(verdict)
			ReportUniverseHeartbeat(um, time.Now())
		}()
	}

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
		result.QuotesMissingNoData = fetchStats.NoData
		result.QuotesMissingNotCovered = fetchStats.NotCovered
		result.QuotesMissingFetchError = fetchStats.FetchError
		result.QuotesMissingNotAttempted = fetchStats.NotAttempted
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
		case fetchStats.ChunksFailed > 0 || fetchStats.UnresolvedFailures() > 0:
			// The ranked list covers only the symbols whose quotes arrived, and
			// at least one requested symbol is missing for a reason that is an
			// acquisition failure rather than a market or source-scope fact:
			// either a whole chunk failed, or a symbol came back as an error /
			// was never asked for. Every downstream consumer that reads absence
			// as "no longer qualifies" (the D6 expiry counter) would fabricate
			// failures for those symbols, so the partial result is explicitly
			// not trustworthy. The ranking is still computed and persisted for
			// inspection.
			//
			// Deliberately NOT untrustworthy (issue #1986 requirement 3): the
			// symbols classified as quotes_missing_no_data or
			// quotes_missing_not_covered. Those are facts — "that stock did not
			// trade that day" / "no venue this deployment reads publishes that
			// symbol" — and a ranking that drops a suspended stock is a correct
			// market verdict, not an incomplete measurement. They are still
			// counted and persisted so the coverage gap stays auditable (and
			// separately alarmed, see the universe coverage alerts).
			result.QuotesStatus = QuotesStatusPartial
			if um != nil {
				if fetchStats.ChunksFailed > 0 {
					um.QuotesErrors.WithLabelValues(stage, "chunk_error").Add(int64(fetchStats.ChunksFailed))
				}
				if n := fetchStats.UnresolvedFailures(); n > 0 {
					um.QuotesErrors.WithLabelValues(stage, "unresolved").Add(int64(n))
				}
			}
			markRankedUntrustworthy(result, RankedFallbackQuoteFetchPartial,
				"chunks", fetchStats.Chunks,
				"chunks_failed", fetchStats.ChunksFailed,
				"quotes_returned", len(quoteMap),
				"missing_fetch_error", fetchStats.FetchError,
				"missing_not_attempted", fetchStats.NotAttempted,
				"missing_no_data", fetchStats.NoData,
				"missing_not_covered", fetchStats.NotCovered)
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
	// The verdict published by the defer reports whether the artifact is really
	// there, read back from the filesystem — never whether the write call
	// returned nil. Reasons to measure instead of trust:
	//   - a silently misconfigured WorkDir writes the snapshot somewhere nobody
	//     reads, and the write itself succeeds;
	//   - MkdirAll/Rename failures are only logged (the run must still publish
	//     its verdict), so the error is not a reliable signal downstream;
	//   - atlas_universe_snapshot_persisted_total increments even on failure
	//     (Step 7 below), so the counter cannot answer this question either.
	// See SnapshotPersistedOnDisk and AtlasUniverseSnapshotNotPersisted
	// (monitoring/rules/atlas_universe_scoring_alerts.yml).
	snapshotPersisted = SnapshotPersistedOnDisk(deps.WorkDir, result.Timestamp)
	if !snapshotPersisted {
		logging.Warn("universe_scheduler", "snapshot_not_persisted",
			"path", UniverseSnapshotPath(deps.WorkDir),
			"run_started", result.Timestamp.UTC().Format(time.RFC3339),
			"save_error", snapshotErr != nil)
	}

	// Also persist as agents.json-compatible universe registry.
	//
	// The registry is the second artifact of this step and it has its own
	// consumers (the agents.json-compatible view). Until 2026-09-27 a failed
	// write here produced exactly one warning line and nothing else, so the
	// observable symptom was "registry stale, snapshot fresh": two consumers of
	// the same run reading two different universes, with no rule able to see it.
	// registryPersisted below is that missing signal (it feeds
	// atlas_universe_last_run_registry_persisted and the
	// AtlasUniverseRegistryNotPersisted rule).
	registryPath := UniverseRegistryPath(deps.WorkDir)
	version := 1
	if prev, err := LoadUniverseRegistry(registryPath); err == nil {
		version = prev.Version + 1
	}
	registryErr := WriteUniverseRegistry(registryPath, result, ranked, version)
	if registryErr != nil {
		logging.Warn("universe_scheduler", "universe_registry_write_error", logging.Err(registryErr))
	}
	// Same read-back discipline as the snapshot: the question is "is the file at
	// the canonical path this run's?", not "did the write call return nil". The
	// mtime comparison is what makes a wrong WorkDir, a missing mount or a
	// silently older file visible.
	registryPersisted = RegistryPersistedOnDisk(deps.WorkDir, result.Timestamp)
	if !registryPersisted {
		logging.Warn("universe_scheduler", "universe_registry_not_persisted",
			"path", registryPath,
			"run_started", result.Timestamp.UTC().Format(time.RFC3339),
			"write_error", registryErr != nil)
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

// UniverseRegistryPath returns the canonical location of the
// agents.json-compatible universe registry (data/state/universe.json) for a
// working directory.
//
// It exists for the same reason as UniverseSnapshotPath: the writer
// (BuildUniverse Step 7) and the read-back measurement that judges it
// (RegistryPersistedOnDisk) must resolve the same path, and a caller that
// spelled it by hand could drift from the gauge that reports on it. The CLI and
// the D6 watchlist resolve the snapshot through its helper; the registry had no
// such single source until 2026-09-27.
func UniverseRegistryPath(workDir string) string {
	return filepath.Join(workDir, "data", "state", "universe.json")
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
	// The artifact carries BOTH the ranked list and its own count, and until
	// 2026-09-27 nothing in the repo compared them: the verifier printed the two
	// readings side by side without judging, and no rule or test asserted they
	// agreed. A disagreement is not a cosmetic defect — a reader that counts
	// entries (rankedSymbolsToAgentSpecs, the D6 watchlist reader, a human) and a
	// reader that trusts symbols_ranked (the coverage check, -build-universe
	// status, the verifier's L3) would then be looking at two different markets,
	// and neither reading would be wrong.
	//
	// The guard below is deliberately NON-FATAL and observable:
	//   - non-fatal, because the snapshot file is the only thing downstream
	//     readers have; refusing to write would turn a bookkeeping contradiction
	//     into an outage (every consumer would fall back to an older file, or to
	//     nothing at all);
	//   - observable, because a silent contradiction is the failure mode this
	//     whole audit was about, so it must leave a line an operator can grep:
	//     universe_ranked_count_mismatch.
	// See RankedCountConflict for why the invariant holds today and why the
	// assertion is kept anyway.
	if declared, persisted, conflict := RankedCountConflict(result, ranked); conflict {
		logging.Warn("universe_scheduler", "universe_ranked_count_mismatch",
			"declared_symbols_ranked", declared,
			"persisted_ranked_entries", persisted,
			"path", UniverseSnapshotPath(workDir))
	}

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

// RankedCountConflict reports whether a run's declared ranked count
// (result.SymbolsRanked) disagrees with the length of the ranked list that is
// about to be persisted with it.
//
// Why it is a function instead of two lines inside the writer: the judgement has
// to be testable in isolation. The writer is the only place that sees both
// readings at the moment they become one artifact, so the assertion lives there,
// but a test that could only reach it through a full pipeline run could not
// drive the contradictory case at all (nothing in the pipeline produces one).
//
// Why the contradictory case cannot happen today (measured, not assumed):
// BuildUniverse assigns result.SymbolsRanked = len(ranked) in Step 4, and no
// later step filters the slice — Step 5 (RiskExclusionFilter) only counts
// exclusions into result.SymbolsExcluded. So the invariant holds by
// construction, and this function is a TRIPWIRE: it is the assertion that fails
// loudly if a future caller (the writer is exported, and the snapshot schema is
// the contract its readers rely on) hands over a pair that disagrees.
//
// A nil result declares a count of zero, which conflicts with any non-empty
// list: "a ranked list with no run result attached" is exactly the pair whose
// count cannot be trusted.
func RankedCountConflict(result *UniverseBuildResult, ranked []RankedSymbol) (declared, persisted int, conflict bool) {
	persisted = len(ranked)
	if result != nil {
		declared = result.SymbolsRanked
	}
	return declared, persisted, declared != persisted
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

// snapshotMtimeTolerance absorbs filesystem timestamp granularity when a run's
// artifact is compared against the instant the run started. It is small on
// purpose: a false "1" (reporting an artifact that is not this run's as if it
// were) is the failure mode that would silence the alert this check feeds.
const snapshotMtimeTolerance = 2 * time.Second

// SnapshotPersistedOnDisk reports whether the canonical snapshot artifact at
// UniverseSnapshotPath(workDir) carries a write that is at least as recent as
// the run that started at runStart.
//
// It is the measurement behind atlas_universe_last_run_snapshot_persisted. The
// question it answers is "did this run's output reach the file the consumers
// read?", which is NOT the same as "did the write call return nil":
//
//   - a wrong WorkDir makes the write succeed at a path nobody reads;
//   - a stale file left by a previous (or out-of-band CLI) run makes the write
//     call irrelevant to whether the artifact describes the market now;
//   - the failure is only warned about (see BuildUniverse Step 7), so a caller
//     that reads the error would still have to decide what "the artifact" means.
//
// Reading the file back makes the answer auditable from outside the process:
// `stat data/state/universe_snapshot.json` must agree with the gauge.
//
// A missing file is false, never an error: "there is no artifact" is exactly
// the state the alert must see.
func SnapshotPersistedOnDisk(workDir string, runStart time.Time) bool {
	return artifactPersistedOnDisk(UniverseSnapshotPath(workDir), runStart)
}

// RegistryPersistedOnDisk reports whether the agents.json-compatible registry at
// UniverseRegistryPath(workDir) carries a write that is at least as recent as the
// run that started at runStart.
//
// It is the measurement behind atlas_universe_last_run_registry_persisted, and it
// shares artifactPersistedOnDisk with the snapshot gauge on purpose: the two
// artifacts are judged by the same rule ("the file at the canonical path is at
// least as new as this run"), so a difference between the gauges can only come
// from the filesystem or from the writer — never from two divergent notions of
// freshness. What differs between them is which file they name, and that is the
// whole point of having two series (see the constant's comment in
// internal/monitoring/metrics/universe_run.go).
//
// A missing file is false, never an error: "there is no registry" is a state the
// alert has to see.
func RegistryPersistedOnDisk(workDir string, runStart time.Time) bool {
	return artifactPersistedOnDisk(UniverseRegistryPath(workDir), runStart)
}

// artifactPersistedOnDisk is the shared mtime judgement of both artifact gauges.
func artifactPersistedOnDisk(path string, runStart time.Time) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.ModTime().Before(runStart.Add(-snapshotMtimeTolerance))
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
	// AsOf is the instant the accounting above was loaded, when the installed
	// substrate can date it (industry.SymbolIndustryCoverageAsOfReporter).
	//
	// The ZERO VALUE means "no successful substrate load has ever been
	// observed", which must be read as NOT MEASURABLE, never as fresh: the
	// numbers above come from an in-memory view whose reload TTL is hours long,
	// so a store that broke after the last successful load keeps being divided
	// into a healthy-looking Ratio. Publishing the instant is what lets an
	// operator age the report; on its own it neither fixes nor hides anything.
	AsOf time.Time
	// LoadError is the message of the last FAILED substrate reload, and "" when
	// the last reload succeeded (or no substrate reported it).
	//
	// It exists to separate the two states that both show up as Upstream == 0:
	// "the load failed" (fix the store) and "the load succeeded but the upstream
	// population was empty" (investigate the channel). Neither is measurable,
	// and they need opposite responses.
	LoadError string
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
//   - AsOf and LoadError are filled ONLY when the substrate additionally
//     implements industry.SymbolIndustryCoverageAsOfReporter /
//     industry.SymbolIndustryReloadErrorReporter, which are independent and
//     optional. A substrate that cannot date its accounting leaves AsOf at its
//     zero value (unknown age) and one that cannot report a failure leaves
//     LoadError empty; neither changes Available, the numbers, the Ratio or the
//     Alert.
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

	// Staleness visibility (FU-20260925-01): both assertions are OPTIONAL and
	// independent, so a reporter that cannot date its accounting (or cannot
	// report a failure) leaves the zero values in place and changes nothing
	// else in this report -- no number, no Alert. Filled before the
	// not-measurable return below on purpose: "Upstream == 0 AND a load error"
	// is the diagnostic, and dropping the error there would hide it exactly
	// when it is most useful.
	if asOfReporter, ok := substrate.(industry.SymbolIndustryCoverageAsOfReporter); ok {
		if asOf, known := asOfReporter.CoverageAsOf(); known {
			report.AsOf = asOf
		}
	}
	if errReporter, ok := substrate.(industry.SymbolIndustryReloadErrorReporter); ok {
		report.LoadError = errReporter.LastReloadError()
	}

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
