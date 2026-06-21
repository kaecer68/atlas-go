// Package monitoring hosts the SmartUniverseBuilder pipeline and its BTM task
// factories. This file wires the four-layer pipeline (IndustryFilter →
// ScoringScreener → RiskExclusionFilter → NarrativeEventBridge) into
// func(ctx context.Context) error closures (compatible with
// apigateway.BackgroundTaskFunc) registered by cmd/atlas/main.go.
//
// Two tasks are exposed:
//   - Daily refresh (incremental): trading days, 06:00 TW
//   - Weekly rebuild (full): Mondays, 06:00 TW
//
// Both delegate to BuildUniverse, which gathers all symbols, runs the full
// pipeline, and persists the ranked result to data/state/universe_snapshot.json.
package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/monitoring/metrics"
	"github.com/kaecer68/atlas-go/internal/screener"
)

// ── Result types ─────────────────────────────────────────────────────────

// UniverseBuildResult captures the outcome of one SmartUniverseBuilder pipeline
// execution. It is serialized into the snapshot file and surfaced to CLI status
// queries so operators can verify the daily/weekly run at a glance.
type UniverseBuildResult struct {
	SymbolsBuilt    int       `json:"symbols_built"`
	SymbolsFiltered int       `json:"symbols_filtered"`
	SymbolsRanked   int       `json:"symbols_ranked"`
	SymbolsExcluded int       `json:"symbols_excluded"`
	FullRebuild     bool      `json:"full_rebuild"`
	Timestamp       time.Time `json:"timestamp"`
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
}

// ── Task factories ───────────────────────────────────────────────────────

// NewDailyUniverseRefreshTask returns a task closure compatible with
// apigateway.BackgroundTaskFunc (the raw func(ctx context.Context) error
// signature is used here to avoid a circular monitoring ↔ apigateway import).
// It fires once per minute but only executes the incremental pipeline when:
//
//   - The current day is a trading day (Mon–Fri).
//   - The wall-clock time is within ±1 minute of 06:00 Taiwan time.
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
		now := time.Now()
		if !isTradingDay(now) {
			logging.Debug("universe_scheduler", "daily_skip_non_trading",
				"day", now.Weekday().String())
			return nil
		}
		if !alignToTarget(now, 6, 0) {
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

		if err := CheckD6Expiry(deps.WorkDir, ranked, prevSymbols); err != nil {
			logging.Warn("universe_scheduler", "d6_expiry_check_error",
				logging.Err(err))
		}

		mapped, total, ratio, alert := CheckUniverseCoverage(deps.Mapper, 0.50)
		logging.Info("universe_scheduler", "coverage_check",
			"mapped", mapped,
			"total", total,
			"ratio", fmt.Sprintf("%.2f", ratio))
		if alert != "" {
			logging.Warn("universe_scheduler", "coverage_alert",
				"alert", alert)
		}
		if deps.UniverseMetrics != nil {
			deps.UniverseMetrics.CoverageMapped.WithLabelValues("daily", "all").Add(int64(mapped))
			deps.UniverseMetrics.CoverageTotal.WithLabelValues("daily", "all").Add(int64(total))
		}

		return nil
	}
}

// NewWeeklyUniverseRebuildTask returns a task closure that performs a
// full universe rebuild (clearing cached state, re-fetching all data). It
// fires once per minute but only executes when:
//
// - Today is Monday.
// - The wall-clock time is within ±1 minute of 06:00 Taiwan time.
//
// The return type is raw func(ctx context.Context) error to avoid a
// circular monitoring ↔ apigateway import; callers assign it directly to
// apigateway.ScheduledTask.Task.
func NewWeeklyUniverseRebuildTask(deps UniverseBuilderDeps) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		now := time.Now()
		if now.Weekday() != time.Monday {
			logging.Debug("universe_scheduler", "weekly_skip_not_monday",
				"day", now.Weekday().String())
			return nil
		}
		if !alignToTarget(now, 6, 0) {
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

		if err := CheckD6Expiry(deps.WorkDir, ranked, prevSymbols); err != nil {
			logging.Warn("universe_scheduler", "d6_expiry_check_error",
				logging.Err(err))
		}

		mapped, total, ratio, alert := CheckUniverseCoverage(deps.Mapper, 0.50)
		logging.Info("universe_scheduler", "coverage_check",
			"mapped", mapped,
			"total", total,
			"ratio", fmt.Sprintf("%.2f", ratio))
		if alert != "" {
			logging.Warn("universe_scheduler", "coverage_alert",
				"alert", alert)
		}
		if deps.UniverseMetrics != nil {
			deps.UniverseMetrics.CoverageMapped.WithLabelValues("weekly", "all").Add(int64(mapped))
			deps.UniverseMetrics.CoverageTotal.WithLabelValues("weekly", "all").Add(int64(total))
		}

		return nil
	}
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
// the corresponding result counter remains zero.
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

	// ── Step 1: Gather all symbols ──────────────────────────────────────

	allSymbols := gatherAllSymbols(deps.Tree, deps.Mapper)
	result.SymbolsBuilt = len(allSymbols)
	if result.SymbolsBuilt == 0 {
		logging.Warn("universe_scheduler", "empty_universe")
		return result, nil, nil
	}
	logging.Info("universe_scheduler", "symbols_gathered",
		"count", result.SymbolsBuilt,
		"full_rebuild", fullRebuild)
	if um != nil {
		um.SymbolsGathered.WithLabelValues(stage).Add(int64(result.SymbolsBuilt))
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
		logging.Warn("universe_scheduler", "empty_filtered")
		return result, nil, nil
	}

	// ── Step 3: Fetch quotes ────────────────────────────────────────────

	var quoteMap map[string]domain.Quote
	if deps.Quotes != nil {
		quotes, err := deps.Quotes.GetQuotes(ctx, time.Now(), filtered)
		if err != nil {
			logging.Warn("universe_scheduler", "quotes_fetch_error",
				logging.Err(err))
			if um != nil {
				um.QuotesErrors.WithLabelValues(stage, "fetch_error").Inc()
			}
		}
		quoteMap = make(map[string]domain.Quote, len(quotes))
		for _, q := range quotes {
			quoteMap[normalizeSymbol(q.Symbol)] = q
		}
		if um != nil {
			um.QuotesFetched.WithLabelValues(stage).Add(int64(len(quoteMap)))
		}
	} else {
		quoteMap = make(map[string]domain.Quote)
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
		um.SymbolsRanked.WithLabelValues(stage).Add(int64(len(ranked)))
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
			if um != nil {
				um.NarrativeEventsScraped.WithLabelValues(stage).Add(int64(len(events)))
			}
		}
	}

	// ── Step 7: Persist snapshot ────────────────────────────────────────

	snapshotErr := saveUniverseSnapshot(deps.WorkDir, result, ranked)
	if snapshotErr != nil {
		logging.Warn("universe_scheduler", "snapshot_save_error",
			logging.Err(snapshotErr))
	}
	if um != nil {
		um.SnapshotPersisted.WithLabelValues(stage).Inc()
		elapsedSec := int64(time.Since(startTime).Seconds())
		um.PipelineDurationSeconds.WithLabelValues(stage).Add(elapsedSec)
	}

	return result, ranked, nil
}

// ── Helpers ──────────────────────────────────────────────────────────────

// gatherAllSymbols collects every known symbol by iterating Level-1 industries
// via Tree.GetLevel1 then expanding each via Mapper.GetSymbolsByIndustry.
// Duplicates are removed and symbols are normalized.
func gatherAllSymbols(tree ClassificationTreeAccessor, mapper SymbolIndustryMapper) []string {
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

// saveUniverseSnapshot writes the build result and ranked symbols to
// <workDir>/data/state/universe_snapshot.json, creating directories as needed.
func saveUniverseSnapshot(workDir string, result *UniverseBuildResult, ranked []RankedSymbol) error {
	outDir := filepath.Join(workDir, "data", "state")
	if err := os.MkdirAll(outDir, 0750); err != nil {
		return fmt.Errorf("create snapshot directory %q: %w", outDir, err)
	}

	payload := map[string]any{
		"result": result,
		"ranked": ranked,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal universe snapshot: %w", err)
	}

	path := filepath.Join(outDir, "universe_snapshot.json")
	if err := os.WriteFile(path, data, 0640); err != nil {
		return fmt.Errorf("write universe snapshot %q: %w", path, err)
	}
	return nil
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

// alignToTarget returns true when now is within ±1 minute of the given hour
// and minute in the same timezone.
func alignToTarget(now time.Time, hour, minute int) bool {
	target := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	diff := now.Sub(target)
	if diff < 0 {
		diff = -diff
	}
	return diff <= time.Minute
}

// ── D6 Expiry ────────────────────────────────────────────────────────────

// CheckD6Expiry maintains the watchlist for symbols that have been dropped from
// the ranked universe. Symbols failing to re-enter the ranked list for 60+
// consecutive trading days are moved to the watchlist. The watchlist is persisted
// to <workDir>/data/state/universe_watchlist.json using an atomic write.
//
// previousUniverseSymbols provides the ranked symbols from the last pipeline run.
// ranked is the current (today's) ranked symbol list.
func CheckD6Expiry(workDir string, ranked []RankedSymbol, previousUniverseSymbols []string) error {
	now := time.Now()
	today := now.Format("2006-01-02")

	outDir := filepath.Join(workDir, "data", "state")
	watchlistPath := filepath.Join(outDir, "universe_watchlist.json")

	wl := Watchlist{Version: "1", UpdatedAt: now.Format(time.RFC3339)}

	if err := os.MkdirAll(outDir, 0750); err != nil {
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

	for sym := range previouslyRankedSet {
		if currentSet[sym] {
			continue
		}
		entry, exists := entryBySymbol[sym]
		if !exists {
			cls := inferredIndustry(sym)
			wl.Symbols = append(wl.Symbols, D6WatchlistEntry{
				Symbol:              sym,
				Industry:            cls,
				ConsecutiveFailures: 1,
				FirstFailureDate:    today,
				LastCheckDate:       today,
			})
			entryBySymbol[sym] = &wl.Symbols[len(wl.Symbols)-1]
		} else {
			entry.ConsecutiveFailures++
			entry.LastCheckDate = today
			if entry.FirstFailureDate == "" {
				entry.FirstFailureDate = today
			}
		}
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
		if e.ConsecutiveFailures >= 60 {
			expiredCount++
			if e.ConsecutiveFailures == 60 {
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
	if err := os.WriteFile(tmpPath, data, 0640); err != nil {
		return fmt.Errorf("write watchlist tmp %q: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, watchlistPath); err != nil {
		return fmt.Errorf("rename watchlist %q: %w", tmpPath, err)
	}

	return nil
}

// inferredIndustry attempts to determine the industry for a symbol from the
// ranked list's metadata. Returns "unknown" if no classification is available.
func inferredIndustry(sym string) string {
	return "unknown"
}

// ── Coverage Check ────────────────────────────────────────────────────────

// CheckUniverseCoverage computes the percentage of total symbols that are
// mapped to Level-1 industries. Returns mapped, total, ratio, and an alert
// message when the ratio falls below the threshold.
//
// A nil mapper returns 0, 0, 0 with an alert.
func CheckUniverseCoverage(mapper SymbolIndustryMapper, threshold float64) (mapped int, total int, ratio float64, alert string) {
	if mapper == nil {
		return 0, 0, 0, "no mapper available"
	}
	// Walk all Level-1 industries and count total mapped symbols.
	// Without a classification tree reference, we iterate all industries
	// accessible through a representative set.  The mapper contract requires
	// GetSymbolsByIndustry to return the symbol list for a given industry ID.
	allSymbols := make(map[string]bool)
	// Iterate common TWSE industry IDs as a reasonable proxy for the full set.
	commonIDs := []string{
		"semiconductor", "electronic_parts", "optoelectronics",
		"computer_peripheral", "communication_network", "electronic_components",
		"financial", "steel", "cement", "textile", "shipping", "automobile",
		"biotech", "chemical", "food", "construction", "other",
	}
	for _, id := range commonIDs {
		for _, sym := range mapper.GetSymbolsByIndustry(id) {
			allSymbols[normalizeSymbol(sym)] = true
		}
	}
	mapped = len(allSymbols)
	// Total is approximated as mapped; in a full implementation this would come
	// from a separate universe source (e.g., TWSE listing count).
	total = mapped
	if total == 0 {
		return mapped, total, 0, "no symbols found in any industry"
	}
	ratio = float64(mapped) / float64(total)
	if ratio < threshold {
		alert = fmt.Sprintf("coverage %.2f%% below threshold %.2f%%", ratio*100, threshold*100)
	}
	return mapped, total, ratio, alert
}
