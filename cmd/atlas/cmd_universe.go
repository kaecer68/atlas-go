package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/bootstrap"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/marketdata"
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

// buildUniverseRun wires the full SmartUniverse pipeline and prints the top N ranked symbols.
func buildUniverseRun(_ *bootstrap.Runtime, cfg config.Config, _ bool, _ string) error {
	suCfg := config.GetParametersConfig().SmartUniverse

	log.Printf("[universe] TopN=%d VolumeFloorTWD=%.0f MaxIndustryConc=%.2f",
		suCfg.TopN.Value, suCfg.VolumeFloorTWD.Value, suCfg.MaxIndustryConcentration.Value)

	// ── Wire industry components ───────────────────────────────────────────
	classTree := industry.DefaultClassification()
	classTreeAdapter := monitoring.AdaptClassificationTree(classTree)
	supplyGraph := industry.NewSupplyChainGraph()
	supplyAdapter := monitoring.AdaptSupplyChainGraph(supplyGraph)
	mapper := monitoring.NewTreeBasedMapper(classTreeAdapter)

	indFilter := monitoring.NewIndustryFilter(mapper, classTreeAdapter, supplyAdapter)
	indFilter.ExpandSupplyChainDepth = suCfg.SupplyChainExpandDepth.Value

	// ── Wire factor engine and screener ─────────────────────────────────────
	factorEng := portfolio.NewFactorEngine()
	fundProv := portfolio.NewFundamentalProvider()
	scr := screener.NewEngine(factorEng, fundProv)
	adaptedFE := monitoring.AdaptFactorEngine(factorEng)

	scoring := monitoring.NewScoringScreener(scr, adaptedFE)
	scoring.TopN = suCfg.TopN.Value
	scoring.VolumeFloorTWD = suCfg.VolumeFloorTWD.Value
	scoring.MaxIndustryConcentration = suCfg.MaxIndustryConcentration.Value
	scoring.PriceMin = suCfg.PriceMinimum.Value
	if suCfg.FactorScoreMaxAgeDays.Value > 0 {
		scoring.FactorScoreMaxAge = time.Duration(suCfg.FactorScoreMaxAgeDays.Value) * 24 * time.Hour
	}
	scoring.Weights = monitoring.ScreenerWeights{
		PE:          suCfg.PEWeight.Value,
		PB:          suCfg.PBWeight.Value,
		Volume:      suCfg.VolumeWeight.Value,
		Momentum:    suCfg.MomentumWeight.Value,
		Quality:     suCfg.QualityWeight.Value,
		ForeignFlow: suCfg.ForeignFlowWeight.Value,
	}

	// ── Wire risk exclusion filter ──────────────────────────────────────────
	// RiskManager and QuoteProvider are nil (optional — dependent checks skip).
	// Apply SmartUniverseConfig overrides (5 risk thresholds) via Configure().
	hp := portfolio.NewHistoricalPrices()
	riskFilter := monitoring.NewRiskExclusionFilter(nil, nil, hp)
	riskFilter.Configure(suCfg)

	// ── Build universe of TWSE symbols ──────────────────────────────────────
	// Gather all symbols from the market data provider.
	mdProvider := marketdata.NewMockProvider()
	quotes, err := mdProvider.GetQuotes(context.Background(), time.Now(), nil)
	if err != nil {
		return fmt.Errorf("fetch quotes: %w", err)
	}
	if len(quotes) == 0 {
		log.Printf("[universe] WARNING: no quotes returned from market data provider")
		return fmt.Errorf("universe: no quotes available")
	}

	allSymbols := make([]string, 0, len(quotes))
	quoteMap := make(map[string]domain.Quote, len(quotes))
	for _, q := range quotes {
		allSymbols = append(allSymbols, q.Symbol)
		quoteMap[q.Symbol] = q
	}

	// ── Layer 1: Industry filter ────────────────────────────────────────────
	candidates := indFilter.Filter(allSymbols)
	log.Printf("[universe] Layer 1 (IndustryFilter): %d → %d candidates", len(allSymbols), len(candidates))

	// ── Layer 2: Scoring + ranking ──────────────────────────────────────────
	ranked := scoring.Rank(candidates, quoteMap)
	log.Printf("[universe] Layer 2 (ScoringScreener): %d → %d ranked", len(candidates), len(ranked))

	// ── Layer 2.5: Risk exclusion ───────────────────────────────────────────
	rankedSymbols := make([]string, len(ranked))
	for i, r := range ranked {
		rankedSymbols[i] = r.Symbol
	}
	exResults, _ := riskFilter.Filter(rankedSymbols)

	excludedCount := 0
	for _, er := range exResults {
		if !er.Passed {
			excludedCount++
		}
	}
	log.Printf("[universe] Layer 2.5 (RiskExclusion): %d excluded", excludedCount)

	// ── Persist snapshot ────────────────────────────────────────────────────
	snapshotPath := filepath.Join(cfg.WorkDir, "data", "state", "universe_snapshot.json")
	if err := persistUniverseSnapshot(snapshotPath, ranked, allSymbols, excludedCount); err != nil {
		log.Printf("[universe] WARNING: failed to persist snapshot: %v", err)
	}

	// ── Print top 20 results ────────────────────────────────────────────────
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
	log.Printf("  Total in universe: %d | Filtered: %d | Ranked: %d | Excluded: %d",
		len(allSymbols), len(allSymbols)-len(candidates), len(ranked), excludedCount)
	log.Printf("")
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

// buildUniverseStatus reads the universe snapshot and prints build stats.
func buildUniverseStatus(cfg config.Config) error {
	snapshotPath := filepath.Join(cfg.WorkDir, "data", "state", "universe_snapshot.json")
	snap, err := loadUniverseSnapshot(snapshotPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("[universe status] no snapshot found at %s — run 'universe run' first", snapshotPath)
			return nil
		}
		return fmt.Errorf("load snapshot: %w", err)
	}

	log.Printf("── Universe Snapshot ──────────────────────────────────────────────────")
	log.Printf("  Build time:    %s", snap.BuildTime)
	log.Printf("  Total symbols: %d", snap.TotalSymbols)
	log.Printf("  In universe:   %d", snap.SymbolsInUniverse)
	log.Printf("  Ranked:        %d", snap.RankedCount)
	log.Printf("  Excluded:      %d", snap.ExcludedCount)
	if snap.RankedCount > 0 {
		log.Printf("")
		log.Printf("  Top 10 scored symbols:")
		top := snap.TopSymbols
		sort.Slice(top, func(i, j int) bool { return top[i].Score > top[j].Score })
		limit := min(10, len(top))
		for i := range limit {
			log.Printf("    %-8s  %-8.1f  %s", top[i].Symbol, top[i].Score, top[i].Industry)
		}
	}
	log.Printf("────────────────────────────────────────────────────────────────────────")
	return nil
}

// ── Snapshot persistence ──────────────────────────────────────────────────────

type universeSnapshot struct {
	BuildTime         string           `json:"build_time"`
	TotalSymbols      int              `json:"total_symbols"`
	SymbolsInUniverse int              `json:"symbols_in_universe"`
	FilteredCount     int              `json:"filtered_count"`
	RankedCount       int              `json:"ranked_count"`
	ExcludedCount     int              `json:"excluded_count"`
	TopSymbols        []snapshotSymbol `json:"top_symbols"`
}

type snapshotSymbol struct {
	Symbol   string  `json:"symbol"`
	Score    float64 `json:"score"`
	Industry string  `json:"industry"`
}

func persistUniverseSnapshot(path string, ranked []monitoring.RankedSymbol, allSymbols []string, excludedCount int) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create snapshot directory: %w", err)
	}

	top := make([]snapshotSymbol, 0, len(ranked))
	for _, r := range ranked {
		top = append(top, snapshotSymbol{Symbol: r.Symbol, Score: r.Score, Industry: r.Industry})
	}

	snap := universeSnapshot{
		BuildTime:         time.Now().Format(time.RFC3339),
		TotalSymbols:      len(allSymbols),
		SymbolsInUniverse: len(ranked),
		FilteredCount:     len(allSymbols) - len(ranked) - excludedCount,
		RankedCount:       len(ranked),
		ExcludedCount:     excludedCount,
		TopSymbols:        top,
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	if err := os.WriteFile(path, data, 0o640); err != nil {
		return fmt.Errorf("write snapshot: %w", err)
	}
	log.Printf("[universe] snapshot persisted to %s", path)
	return nil
}

func loadUniverseSnapshot(path string) (*universeSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var snap universeSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	return &snap, nil
}
