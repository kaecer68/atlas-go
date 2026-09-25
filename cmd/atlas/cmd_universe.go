package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/bootstrap"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/screener"
)

// runBuildUniverse is the main dispatch function for universe-builder CLI commands.
// Sub-commands: run, map, status.
func runBuildUniverse(rt *bootstrap.Runtime, cfg config.Config, verbose bool, dateOverride string, subCmd string) error {
	switch subCmd {
	case "run":
		return buildUniverseRun(rt, cfg, verbose, dateOverride)
	case "map":
		return buildUniverseMap(cfg)
	case "status":
		return buildUniverseStatus(cfg)
	default:
		return fmt.Errorf("unknown universe sub-command: %s (valid: run, map, status)", subCmd)
	}
}

// buildUniverseRun runs the full SmartUniverse pipeline once and prints the
// top ranked symbols.
//
// It delegates to monitoring.BuildUniverse so that this sub-command and the
// scheduled auto_universe_refresh / auto_universe_full_rebuild tasks share one
// implementation, one quote provider and one snapshot schema. Two issues
// (issue #1944 Batch 3) came from this function re-implementing the pipeline:
//
//   - N-U1: it called marketdata.NewMockProvider() with a nil symbol list, so
//     GetQuotes returned an empty slice and the sub-command failed 100% of the
//     time with "universe: no quotes available".
//   - N-U3: it persisted a second, incompatible snapshot schema
//     (build_time / ranked_count / top_symbols) to the same path the scheduler
//     writes (result / ranked). Both directions unmarshalled without error and
//     silently read zero, so the CLI clobbered the scheduler's ranked list
//     (breaking D6 watchlist tracking) and the coverage alert input.
//
// The universe population now comes from monitoring.GatherUniverseSymbols
// (classification tree + industry substrate), the same source the scheduler
// uses, and the quotes come from the same gateway-backed provider the
// simulation path uses.
func buildUniverseRun(rt *bootstrap.Runtime, cfg config.Config, _ bool, _ string) error {
	suCfg := config.GetParametersConfig().SmartUniverse

	log.Printf("[universe] TopN=%d VolumeFloorTWD=%.0f MaxIndustryConc=%.2f",
		suCfg.TopN.Value, suCfg.VolumeFloorTWD.Value, suCfg.MaxIndustryConcentration.Value)

	// ── Wire industry components ───────────────────────────────────────────
	classTree := industry.DefaultClassification()
	classTreeAdapter := monitoring.AdaptClassificationTree(classTree)
	supplyGraph := industry.NewSupplyChainGraph()
	supplyAdapter := monitoring.AdaptSupplyChainGraph(supplyGraph)
	// issue #1943: the per-stock industry substrate (nil unless
	// industry.substrate_from_symbol_industry_enabled is true) lets the CLI
	// resolve the whole listed market instead of the tree's ~27 representative
	// stocks — the same wiring the production pipeline uses.
	var pool *pgxpool.Pool
	if rt != nil {
		pool = rt.Pool
	}
	substrate := newSymbolIndustrySubstrate(context.Background(), cfg, pool)
	if substrate != nil {
		industry.RegisterSymbolIndustryConsumer("cmd/atlas.build-universe")
	}
	mapper := monitoring.NewSubstrateIndustryMapper(monitoring.NewTreeBasedMapper(classTreeAdapter), substrate, classTreeAdapter)

	// ── Wire factor engine and screener ────────────────────────────────────
	factorEngine := portfolio.NewFactorEngine()
	scr := screener.NewEngine(factorEngine, portfolio.NewFundamentalProvider())

	// ── Wire the real quote provider (N-U1) ────────────────────────────────
	quoteProvider := newUniverseQuoteProvider(cfg)
	// Layer 2.5 shares the very same provider instance (N-U7), so the liquidity
	// re-check and the Step 3 price/volume filter never disagree about a symbol.
	riskFilter := monitoring.NewRiskExclusionFilter(nil, quoteProvider, portfolio.NewHistoricalPrices())
	riskFilter.Configure(suCfg)

	deps := monitoring.UniverseBuilderDeps{
		Mapper:      mapper,
		Tree:        classTreeAdapter,
		SupplyChain: supplyAdapter,
		Screener:    scr,
		FactorEng:   monitoring.AdaptFactorEngine(factorEngine),
		Quotes:      quoteProvider,
		RiskFilter:  riskFilter,
		// NarrativeBridge stays nil on purpose: a manual CLI run must not
		// scrape external RSS/news feeds. Layer 3 is therefore skipped here;
		// the scheduled tasks own that step.
		NarrativeBridge: nil,
		Config:          suCfg,
		WorkDir:         cfg.WorkDir,
		WatchlistMu:     &universeWatchlistMu,
		Substrate:       substrate,
	}

	// fullRebuild=true: a manual invocation is a fresh run, matching the
	// weekly rebuild stage (and the widest universe).
	result, ranked, err := monitoring.BuildUniverse(context.Background(), deps, true)
	if err != nil {
		return fmt.Errorf("build universe: %w", err)
	}
	if result == nil {
		return errors.New("build universe: nil result")
	}

	log.Printf("[universe] symbols built=%d filtered=%d ranked=%d excluded=%d",
		result.SymbolsBuilt, result.SymbolsFiltered, result.SymbolsRanked, result.SymbolsExcluded)

	// Print the top 20 results.
	log.Printf("")
	log.Printf("── Smart Universe: Top %d ──────────────────────────────────────────────", len(ranked))
	log.Printf("")
	log.Printf("  %-8s  %-8s  %-20s  %s", "RANK", "SCORE", "INDUSTRY", "SYMBOL")
	log.Printf("  %-8s  %-8s  %-20s  %s", "────", "─────", "────────", "──────")
	printCount := min(20, len(ranked))
	for i := range printCount {
		r := ranked[i]
		freshTag := ""
		if !r.ScoreFresh {
			freshTag = " [stale]"
		}
		log.Printf("  %-8d  %-8.1f  %-20s  %s%s", i+1, r.Score, r.Industry, r.Symbol, freshTag)
	}
	log.Printf("")
	log.Printf("  Quotes: status=%s returned=%d | trustworthy=%t | fallback_reason=%q",
		result.QuotesStatus, result.QuotesReturned, result.RankedTrustworthy, result.RankedFallbackReason)
	log.Printf("")

	if !result.RankedTrustworthy {
		// Do not let a run that could not evaluate the universe look like a
		// successful run with zero qualifying stocks (I25).
		return fmt.Errorf(
			"universe: ranked list is not trustworthy (reason=%s, quotes_status=%s, quotes_returned=%d, built=%d, filtered=%d) — no snapshot reflects a market verdict",
			result.RankedFallbackReason, result.QuotesStatus, result.QuotesReturned,
			result.SymbolsBuilt, result.SymbolsFiltered)
	}
	return nil
}

// buildUniverseMap prints mapping coverage statistics.
func buildUniverseMap(cfg config.Config) error {
	_ = cfg

	// ── Wire industry components ───────────────────────────────────────────
	classTree := industry.DefaultClassification()
	classTreeAdapter := monitoring.AdaptClassificationTree(classTree)
	mapper := monitoring.NewTreeBasedMapper(classTreeAdapter)

	level1 := classTree.GetLevel1()
	log.Printf("[universe map] Classification coverage across %d Level-1 industries:", len(level1))

	grandTotal := 0
	grandMapped := 0
	grandUnknown := 0

	for _, seg := range level1 {
		symbols := mapper.GetSymbolsByIndustry(seg.ID)
		total := len(symbols)
		mapped := 0
		unknown := 0
		for _, sym := range symbols {
			if _, ok := mapper.GetClassification(sym); ok {
				mapped++
			} else {
				unknown++
			}
		}
		grandTotal += total
		grandMapped += mapped
		grandUnknown += unknown

		var pct float64
		if total > 0 {
			pct = float64(mapped) / float64(total) * 100
		}
		log.Printf("  %-30s %s: %d/%d mapped (%.1f%%), %d unknown", seg.Name, seg.ID, mapped, total, pct, unknown)
	}

	grandPct := 0.0
	if grandTotal > 0 {
		grandPct = float64(grandMapped) / float64(grandTotal) * 100
	}
	log.Printf("  ─────────────────────────────────────────────")
	log.Printf("  TOTAL: %d/%d mapped (%.1f%%), %d unknown", grandMapped, grandTotal, grandPct, grandUnknown)

	return nil
}

// buildUniverseStatus reads the canonical universe snapshot and prints build
// stats, including the quote-input evidence fields.
func buildUniverseStatus(cfg config.Config) error {
	snap, err := monitoring.LoadUniverseSnapshot(cfg.WorkDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			log.Printf("[universe status] no snapshot found at %s — run 'universe run' first",
				monitoring.UniverseSnapshotPath(cfg.WorkDir))
			return nil
		}
		return fmt.Errorf("load snapshot: %w", err)
	}
	if snap.Result == nil {
		return errors.New("load snapshot: snapshot has no result block (incompatible schema?)")
	}
	res := snap.Result

	log.Printf("── Universe Snapshot ──────────────────────────────────────────────────")
	log.Printf("  Build time:    %s", res.Timestamp.Format(time.RFC3339))
	log.Printf("  Total symbols: %d", res.SymbolsBuilt)
	log.Printf("  In universe:   %d", res.SymbolsRanked)
	log.Printf("  Ranked:        %d", res.SymbolsRanked)
	log.Printf("  Excluded:      %d", res.SymbolsExcluded)
	log.Printf("  Quotes:        status=%s returned=%d trustworthy=%t",
		res.QuotesStatus, res.QuotesReturned, res.RankedTrustworthy)
	if !res.RankedTrustworthy {
		// Make the ambiguous zero loud: the ranked list is not a market verdict.
		log.Printf("  !! ranked list is NOT trustworthy: reason=%s", res.RankedFallbackReason)
	}
	if len(snap.Ranked) > 0 {
		log.Printf("")
		log.Printf("  Top 10 scored symbols:")
		// snap.Ranked is stored in rank order (descending score), so the first
		// entries are already the top ones.
		limit := min(10, len(snap.Ranked))
		for i := range limit {
			r := snap.Ranked[i]
			log.Printf("    %-8s  %-8.1f  %s", r.Symbol, r.Score, r.Industry)
		}
	}
	log.Printf("────────────────────────────────────────────────────────────────────────")
	return nil
}
