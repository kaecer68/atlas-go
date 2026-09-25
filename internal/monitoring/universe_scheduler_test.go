package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
)

// ─────────────────── test helpers ──────────────────────────────────────────

// testSmartUniverseConfig returns a minimal SmartUniverseConfig with sensible
// defaults suitable for pipeline tests. Only the Value fields matter; Rationale
// and Source are left empty.
func testSmartUniverseConfig() config.SmartUniverseConfig {
	return config.SmartUniverseConfig{
		TopN:                      config.ParameterMetadata[int]{Value: 150},
		PEWeight:                  config.ParameterMetadata[float64]{Value: 0.15},
		PBWeight:                  config.ParameterMetadata[float64]{Value: 0.10},
		VolumeWeight:              config.ParameterMetadata[float64]{Value: 0.15},
		MomentumWeight:            config.ParameterMetadata[float64]{Value: 0.15},
		QualityWeight:             config.ParameterMetadata[float64]{Value: 0.20},
		ForeignFlowWeight:         config.ParameterMetadata[float64]{Value: 0.20},
		VolumeFloorTWD:            config.ParameterMetadata[float64]{Value: 10_000_000},
		MinDailyAmountTWD:         config.ParameterMetadata[float64]{Value: 5_000_000},
		MaxIndustryConcentration:  config.ParameterMetadata[float64]{Value: 0.40},
		PriceMinimum:              config.ParameterMetadata[float64]{Value: 10.0},
		FactorScoreMaxAgeDays:     config.ParameterMetadata[int]{Value: 30},
		D6ExpiryTradingDays:       config.ParameterMetadata[int]{Value: 60},
		VaRContributionMultiplier: config.ParameterMetadata[float64]{Value: 2.0},
		VolatilityMultiplier:      config.ParameterMetadata[float64]{Value: 2.0},
		DrawdownWindow:            config.ParameterMetadata[int]{Value: 60},
		DrawdownThreshold:         config.ParameterMetadata[float64]{Value: 0.30},
		ConfidenceThreshold:       config.ParameterMetadata[int]{Value: 3},
		SupplyChainExpandDepth:    config.ParameterMetadata[int]{Value: 2},
	}
}

// buildDepsFixture returns a UniverseBuilderDeps wired with mocks that produce a
// valid 2-symbol pipeline result. It uses the given temp dir for all file I/O.
func buildDepsFixture(t *testing.T, workDir string) UniverseBuilderDeps {
	t.Helper()

	mapper := &mockMapper{
		classifications: map[string]*IndustryClassification{
			"2330": {Symbol: "2330", Level1: IndustrySegment{ID: "semiconductor", Name: "半導體"}},
			"2317": {Symbol: "2317", Level1: IndustrySegment{ID: "tech", Name: "科技"}},
		},
		byIndustry: map[string][]string{
			"semiconductor": {"2330"},
			"tech":          {"2317"},
		},
	}

	segments := []IndustrySegment{
		{ID: "semiconductor", Name: "半導體", Level: 1, Weight: 0.35},
		{ID: "tech", Name: "科技", Level: 1, Weight: 0.25},
	}
	tree := newFakeTree(segments)

	scores := map[string]map[string]float64{
		"2330": {"pe": 100, "pb": 80, "volume": 60, "momentum": 40, "quality": 20, "foreign_flow": 10},
		"2317": {"pe": 90, "pb": 70, "volume": 50, "momentum": 30, "quality": 15, "foreign_flow": 5},
	}
	factorEng := &mockFactorEng{scores: scores}

	quotes := map[string]domain.Quote{
		"2330": {Symbol: "2330", Last: 500, Volume: 50_000_000, AsOf: time.Now()},
		"2317": {Symbol: "2317", Last: 100, Volume: 30_000_000, AsOf: time.Now()},
	}
	quoteProv := &mockQuoteProv{quotes: quotes}

	// Minimal RSS XML that triggers no keywords ⇒ empty events (graceful).
	rssXML := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <item>
      <title>Daily Market Summary</title>
      <description>Markets were mixed today.</description>
    </item>
  </channel>
</rss>`
	feedFetcher := func(_ context.Context, _ string) (*FeedData, error) {
		return &FeedData{Data: []byte(rssXML)}, nil
	}
	neb := NewNarrativeEventBridgeWithFetcher(
		filepath.Join(workDir, "narrative_cache.json"),
		feedFetcher,
	)
	neb.Configure(testSmartUniverseConfig())

	return UniverseBuilderDeps{
		Mapper:          mapper,
		Tree:            tree,
		SupplyChain:     &mockSupplyChain{},
		Screener:        &mockScreener{passAll: true},
		FactorEng:       factorEng,
		Quotes:          quoteProv,
		RiskFilter:      nil, // pass-through
		NarrativeBridge: neb,
		Config:          testSmartUniverseConfig(),
		WorkDir:         workDir,
	}
}

// ─────────────────── 1. isTradingDay ─────────────────────────────────────

// TestIsTradingDay verifies that isTradingDay returns true for weekdays
// and false for weekends.
func TestIsTradingDay(t *testing.T) {
	tests := []struct {
		name string
		t    time.Time
		want bool
	}{
		{
			name: "tuesday_wartrue",
			t:    time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC), // Tuesday
			want: true,
		},
		{
			name: "saturday_returns_false",
			t:    time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC), // Saturday
			want: false,
		},
		{
			name: "sunday_returns_false",
			t:    time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC), // Sunday
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTradingDay(tt.t)
			if got != tt.want {
				t.Fatalf("isTradingDay(%v) = %v, want %v", tt.t.Weekday(), got, tt.want)
			}
		})
	}
}

// ─────────────────── 2. alignToTarget ────────────────────────────────────

// TestAlignToTarget verifies that alignToTarget returns true only when the
// current time is within ±1 minute of the target hour and minute.
func TestAlignToTarget(t *testing.T) {
	// Use a fixed location for deterministic tests.
	loc := time.FixedZone("TW", 8*3600) // UTC+8
	tests := []struct {
		name string
		t    time.Time
		want bool
	}{
		{
			name: "exact_match_at_0600",
			t:    time.Date(2026, 6, 16, 6, 0, 0, 0, loc),
			want: true,
		},
		{
			name: "one_minute_off_0601",
			t:    time.Date(2026, 6, 16, 6, 1, 1, 0, loc),
			want: false,
		},
		{
			name: "week_boundary_monday_0600",
			t:    time.Date(2026, 6, 15, 6, 0, 0, 0, loc), // Monday
			want: true,
		},
		{
			name: "already_aligned_59s_diff",
			t:    time.Date(2026, 6, 16, 6, 0, 59, 0, loc),
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := alignToTarget(tt.t)
			if got != tt.want {
				t.Fatalf("alignToTarget(%v) = %v, want %v", tt.t, got, tt.want)
			}
		})
	}
}

// ─────────────────── 3. CheckD6Expiry ────────────────────────────────────

// TestCheckD6Expiry verifies the watchlist expiry logic across file
// existence, freshness, expiry, and error conditions.
func TestCheckD6Expiry(t *testing.T) {
	mapper := &mockMapper{
		classifications: map[string]*IndustryClassification{
			"2330": {Symbol: "2330", Level1: IndustrySegment{ID: "semiconductor", Name: "半導體"}},
			"2317": {Symbol: "2317", Level1: IndustrySegment{ID: "tech", Name: "科技"}},
		},
	}

	t.Run("no_file_creates_new", func(t *testing.T) {
		workDir := tempDir(t)
		ranked := sampleRankedSymbols(2)
		prev := []string{"2330", "2317"} // same as ranked → no failures

		err := CheckD6Expiry(workDir, ranked, prev, mapper, 60)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Verify the watchlist file was created.
		path := filepath.Join(workDir, "data", "state", "universe_watchlist.json")
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("watchlist file not created: %v", statErr)
		}
		// Verify content: all symbols in both ranked and prev → no entries.
		raw, _ := os.ReadFile(path)
		var wl Watchlist
		if err := json.Unmarshal(raw, &wl); err != nil {
			t.Fatalf("watchlist JSON invalid: %v", err)
		}
		if len(wl.Symbols) != 0 {
			t.Fatalf("expected 0 entries, got %d", len(wl.Symbols))
		}
	})

	t.Run("fresh_entries_no_removals", func(t *testing.T) {
		workDir := tempDir(t)
		// Write a watchlist with a fresh entry (age 0 days, failures 0).
		wlContent := sampleWatchlist([]string{"2454"}, 0)
		dir := filepath.Join(workDir, "data", "state")
		os.MkdirAll(dir, 0o750)
		os.WriteFile(filepath.Join(dir, "universe_watchlist.json"), wlContent, 0o640)

		ranked := sampleRankedSymbols(2) // 2330, 2317
		prev := []string{"2330", "2317"} // same as ranked → no new failures

		err := CheckD6Expiry(workDir, ranked, prev, mapper, 60)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// The entry for 2454 should still be present with failures=0 (reset).
		raw, _ := os.ReadFile(filepath.Join(dir, "universe_watchlist.json"))
		var wl Watchlist
		json.Unmarshal(raw, &wl)
		found := false
		for _, e := range wl.Symbols {
			if e.Symbol == "2454" && e.ConsecutiveFailures == 0 {
				found = true
			}
		}
		if !found {
			t.Fatal("fresh entry 2454 should remain with ConsecutiveFailures=0")
		}
	})

	t.Run("expired_entry_removed", func(t *testing.T) {
		workDir := tempDir(t)
		// Write a watchlist where symbol "MISSING" has been failing for 60+ days.
		wlContent := sampleWatchlist([]string{"MISSING"}, 65)
		dir := filepath.Join(workDir, "data", "state")
		os.MkdirAll(dir, 0o750)
		os.WriteFile(filepath.Join(dir, "universe_watchlist.json"), wlContent, 0o640)

		ranked := sampleRankedSymbols(2)    // 2330, 2317 — MISSING not in ranked
		prev := []string{"MISSING", "2330"} // MISSING was ranked previously but not now

		err := CheckD6Expiry(workDir, ranked, prev, mapper, 60)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// MISSING should now have 66 failures (>=60 expiry), triggering logging.
		raw, _ := os.ReadFile(filepath.Join(dir, "universe_watchlist.json"))
		var wl Watchlist
		json.Unmarshal(raw, &wl)
		for _, e := range wl.Symbols {
			if e.Symbol == "MISSING" && e.ConsecutiveFailures < 60 {
				t.Fatalf("MISSING should have >=60 failures, got %d", e.ConsecutiveFailures)
			}
		}
	})

	t.Run("nil_mapper_panics", func(t *testing.T) {
		workDir := tempDir(t)
		// Write a watchlist with an existing entry. Since "MISSING" is not
		// in the watchlist but IS in previousUniverseSymbols and NOT in
		// ranked, CheckD6Expiry will try to create a new entry, which calls
		// inferredIndustry → mapper.GetClassification → nil deref.
		dir := filepath.Join(workDir, "data", "state")
		os.MkdirAll(dir, 0o750)
		os.WriteFile(filepath.Join(dir, "universe_watchlist.json"), sampleWatchlist([]string{"2330"}, 0), 0o640)

		ranked := sampleRankedSymbols(1) // 2330 only
		prev := []string{"MISSING"}      // not in ranked, not in watchlist → new entry

		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic with nil mapper when creating new entry")
			}
		}()
		_ = CheckD6Expiry(workDir, ranked, prev, nil, 60)
	})

	t.Run("concurrent_watchlist_mu", func(t *testing.T) {
		// Spawn 2 goroutines calling CheckD6Expiry concurrently with a
		// shared sync.Mutex. Requires -race to detect races.
		workDir := tempDir(t)
		mu := &sync.Mutex{}
		ranked := sampleRankedSymbols(2)
		prev := []string{"2330", "2317"}
		expiryDays := 60

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			mu.Lock()
			_ = CheckD6Expiry(workDir, ranked, prev, mapper, expiryDays)
			mu.Unlock()
		}()
		go func() {
			defer wg.Done()
			mu.Lock()
			_ = CheckD6Expiry(workDir, ranked, prev, mapper, expiryDays)
			mu.Unlock()
		}()

		wg.Wait()
	})
}

// ─────────────────── 4. CheckUniverseCoverage ────────────────────────────

// coverageSubstrate is a substrate that also reports the first-party population
// (industry.SymbolIndustryCoverageReporter). It embeds fakeSubstrate so the
// population and resolution behaviour stay the ones the pipeline uses; only the
// accounting is faked here.
type coverageSubstrate struct {
	*fakeSubstrate
	coverage industry.SymbolIndustryCoverage
}

func (c *coverageSubstrate) Coverage() industry.SymbolIndustryCoverage {
	out := c.coverage
	out.Reasons = slices.Clone(c.coverage.Reasons)
	return out
}

func newCoverageSubstrate(cov industry.SymbolIndustryCoverage) *coverageSubstrate {
	symbols := map[string]industry.SectorID{}
	for i := 0; i < cov.Resolved; i++ {
		symbols[fmt.Sprintf("%04d", 1000+i)] = "semiconductor"
	}
	return &coverageSubstrate{fakeSubstrate: newFakeSubstrate(symbols), coverage: cov}
}

// legacyTree is the classification tree the pre-#1943 pipeline used: it
// declares representative stocks, which is exactly the wrong denominator.
func legacyTree() *fakeTree {
	return newFakeTree([]IndustrySegment{
		{ID: "semiconductor", Name: "半導體", Level: 1, RepresentativeStocks: []string{"2330", "2317", "2454"}},
		{ID: "tech", Name: "科技", Level: 1, RepresentativeStocks: []string{"2357"}},
	})
}

// TestCheckUniverseCoverage_HonestRatio is the regression test for the false
// green. The pre-2026-09 audit set total = mapped, so production logged
// `coverage_check mapped=27 total=27 ratio=1.00` while the pipeline had already
// moved to a 1599-symbol population: the ratio could not be anything but 1.00
// and the alert could never fire.
//
// The NEGATIVE CONTROL is the first case: a first-party population of 10 rows
// with 7 resolved MUST produce ratio 0.7 and an alert.
func TestCheckUniverseCoverage_HonestRatio(t *testing.T) {
	t.Run("negative_control_partial_coverage_is_not_green", func(t *testing.T) {
		sub := newCoverageSubstrate(industry.SymbolIndustryCoverage{
			Upstream: 10,
			Resolved: 7,
			Unmapped: 2,
			Unknown:  1,
			Reasons:  []string{"residual bucket (19/20)", "code not declared (upstream drift)"},
		})

		rep := CheckUniverseCoverage(sub, &mockMapper{}, legacyTree(), 0.90)

		if !rep.Available {
			t.Fatalf("a reporting substrate must be measurable, got %+v", rep)
		}
		if rep.Source != industry.L1SourceSymbolIndustry {
			t.Fatalf("Source = %q, want %q", rep.Source, industry.L1SourceSymbolIndustry)
		}
		if rep.Upstream != 10 || rep.Mapped != 7 {
			t.Fatalf("Upstream/Mapped = %d/%d, want 10/7", rep.Upstream, rep.Mapped)
		}
		if rep.Ratio != 0.7 {
			t.Fatalf("Ratio = %.4f, want 0.7 (the audit must not be 1.00 by construction)", rep.Ratio)
		}
		if rep.Ratio == 1.0 {
			t.Fatal("ratio 1.00 here IS the false green the audit exists to prevent")
		}
		// The unresolved counts are the whole point of the alert: they say HOW
		// the population fell short, not just that it did.
		if rep.Unmapped != 2 || rep.Unknown != 1 {
			t.Fatalf("Unmapped/Unknown = %d/%d, want 2/1", rep.Unmapped, rep.Unknown)
		}
		if len(rep.Reasons) != 2 {
			t.Fatalf("Reasons = %q, want the two unresolved reasons", rep.Reasons)
		}
		if !strings.Contains(rep.Alert, "below threshold") {
			t.Fatalf("Alert = %q, want the threshold message shape", rep.Alert)
		}
		for _, want := range []string{"unmapped=2", "unknown=1", "upstream=10", "mapped=7"} {
			if !strings.Contains(rep.Alert, want) {
				t.Fatalf("Alert = %q, want it to carry %q", rep.Alert, want)
			}
		}
	})

	t.Run("gate_off_is_not_measurable_not_green", func(t *testing.T) {
		tree := legacyTree()

		rep := CheckUniverseCoverage(nil, &mockMapper{}, tree, 0.50)

		if rep.Available {
			t.Fatalf("no substrate means no measurable population, got %+v", rep)
		}
		if rep.Source != "unavailable" {
			t.Fatalf("Source = %q, want \"unavailable\"", rep.Source)
		}
		if rep.Ratio != 0 {
			t.Fatalf("Ratio = %.4f, want 0 when unavailable", rep.Ratio)
		}
		if rep.Ratio == 1.0 {
			t.Fatal("RB-1944: the gate-off audit must never report 1.00")
		}
		if rep.Mapped != 0 {
			t.Fatalf("Mapped = %d, want 0 when unavailable", rep.Mapped)
		}
		if !strings.Contains(rep.Alert, "not measurable") {
			t.Fatalf("Alert = %q, want an explicit not-measurable message", rep.Alert)
		}
		// The alert must name the legacy table so nobody uses it as a
		// market-wide denominator.
		if want := fmt.Sprintf("%d", TotalClassifiedSymbols(tree)); !strings.Contains(rep.Alert, want) {
			t.Fatalf("Alert = %q, want the legacy representative-stock count %s", rep.Alert, want)
		}
	})

	t.Run("substrate_without_reporter_is_not_measurable", func(t *testing.T) {
		// A plain substrate knows its population but not the upstream rows it
		// could not resolve, so it cannot answer the audit. It must not be
		// silently treated as full coverage.
		sub := newFakeSubstrate(map[string]industry.SectorID{"2330": "semiconductor"})

		rep := CheckUniverseCoverage(sub, &mockMapper{}, legacyTree(), 0.50)

		if rep.Available || rep.Ratio != 0 {
			t.Fatalf("a non-reporting substrate must be unavailable with ratio 0, got %+v", rep)
		}
		if !strings.Contains(rep.Alert, "not measurable") {
			t.Fatalf("Alert = %q, want an explicit not-measurable message", rep.Alert)
		}
	})

	t.Run("empty_first_party_population_is_not_green", func(t *testing.T) {
		// Loaded nothing (channel has never run / table empty): upstream 0 is
		// NOT 100 % coverage.
		rep := CheckUniverseCoverage(newCoverageSubstrate(industry.SymbolIndustryCoverage{}), &mockMapper{}, nil, 0.50)

		if rep.Available || rep.Ratio != 0 {
			t.Fatalf("empty population must be unavailable with ratio 0, got %+v", rep)
		}
		if !strings.Contains(rep.Alert, "not measurable") {
			t.Fatalf("Alert = %q, want an explicit not-measurable message", rep.Alert)
		}
	})

	t.Run("full_coverage_ratio_one_without_alert", func(t *testing.T) {
		sub := newCoverageSubstrate(industry.SymbolIndustryCoverage{Upstream: 1599, Resolved: 1599})

		rep := CheckUniverseCoverage(sub, &mockMapper{}, legacyTree(), 0.50)

		if !rep.Available {
			t.Fatalf("a reporting substrate must be measurable, got %+v", rep)
		}
		if rep.Ratio != 1.0 {
			t.Fatalf("Ratio = %.4f, want 1.0", rep.Ratio)
		}
		if rep.Upstream != rep.Mapped {
			t.Fatalf("Upstream/Mapped = %d/%d, want equal", rep.Upstream, rep.Mapped)
		}
		if rep.Alert != "" {
			t.Fatalf("full coverage must not alert, got %q", rep.Alert)
		}
	})

	t.Run("threshold_is_inclusive", func(t *testing.T) {
		sub := newCoverageSubstrate(industry.SymbolIndustryCoverage{Upstream: 10, Resolved: 5, Unmapped: 5})

		rep := CheckUniverseCoverage(sub, &mockMapper{}, nil, 0.50)

		if rep.Ratio != 0.5 {
			t.Fatalf("Ratio = %.4f, want 0.5", rep.Ratio)
		}
		if rep.Alert != "" {
			t.Fatalf("ratio == threshold must not alert, got %q", rep.Alert)
		}
	})

	t.Run("above_threshold_ratio_alerts", func(t *testing.T) {
		sub := newCoverageSubstrate(industry.SymbolIndustryCoverage{Upstream: 10, Resolved: 10})

		rep := CheckUniverseCoverage(sub, &mockMapper{}, nil, 1.5)

		if rep.Ratio != 1.0 {
			t.Fatalf("Ratio = %.4f, want 1.0", rep.Ratio)
		}
		if !strings.Contains(rep.Alert, "below threshold") {
			t.Fatalf("Alert = %q, want the threshold message shape", rep.Alert)
		}
	})

	t.Run("nil_mapper_and_tree_are_tolerated", func(t *testing.T) {
		// The audit no longer depends on the legacy tree/mapper for its
		// numbers; they are only used to describe the fallback population.
		sub := newCoverageSubstrate(industry.SymbolIndustryCoverage{Upstream: 4, Resolved: 3, Unmapped: 1})

		measurable := CheckUniverseCoverage(sub, nil, nil, 0.90)
		if !measurable.Available || measurable.Ratio != 0.75 {
			t.Fatalf("nil legacy deps must not break a measurable audit, got %+v", measurable)
		}
		if !strings.Contains(measurable.Alert, "below threshold") {
			t.Fatalf("Alert = %q, want a threshold alert", measurable.Alert)
		}

		unavailable := CheckUniverseCoverage(nil, nil, nil, 0.50)
		if unavailable.Available || unavailable.Ratio != 0 {
			t.Fatalf("nil everything must be unavailable with ratio 0, got %+v", unavailable)
		}
		if !strings.Contains(unavailable.Alert, "not measurable") {
			t.Fatalf("Alert = %q, want an explicit not-measurable message", unavailable.Alert)
		}
	})
}

// ─────────────────── 5. Snapshot + Load roundtrip ────────────────────────

// TestSnapshotRoundtrip verifies that saveUniverseSnapshot writes a file
// that loadPreviousRankedSymbols can read back, and that both functions
// handle missing and corrupt files gracefully.
func TestSnapshotRoundtrip(t *testing.T) {
	t.Run("write_then_read_roundtrip", func(t *testing.T) {
		workDir := tempDir(t)
		result := sampleUniverseBuildResult()
		ranked := sampleRankedSymbols(3)

		if err := saveUniverseSnapshot(workDir, result, ranked); err != nil {
			t.Fatalf("save failed: %v", err)
		}

		got := loadPreviousRankedSymbols(workDir)
		want := []string{"2330", "2317", "2454"}
		if len(got) != len(want) {
			t.Fatalf("loadPreviousRankedSymbols returned %d symbols, want %d (%v)", len(got), len(want), got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("symbol[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("missing_file_returns_nil", func(t *testing.T) {
		workDir := tempDir(t)
		got := loadPreviousRankedSymbols(workDir)
		if got != nil {
			t.Fatalf("expected nil for missing file, got %v", got)
		}
	})

	t.Run("corrupt_json_returns_nil", func(t *testing.T) {
		workDir := tempDir(t)
		dir := filepath.Join(workDir, "data", "state")
		os.MkdirAll(dir, 0o750)
		os.WriteFile(filepath.Join(dir, "universe_snapshot.json"), []byte("{not json}"), 0o640)

		got := loadPreviousRankedSymbols(workDir)
		if got != nil {
			t.Fatalf("expected nil for corrupt JSON, got %v", got)
		}
	})
}

// ─────────────────── 6. BuildUniverse integration ────────────────────────

// TestBuildUniverseIntegration exercises BuildUniverse end-to-end with mocked
// dependencies covering happy path, graceful degradation with nil optionals,
// and error-recovery for quote and narrative failures.
func TestBuildUniverseIntegration(t *testing.T) {
	t.Run("happy_path_all_deps_wired", func(t *testing.T) {
		deps := buildDepsFixture(t, tempDir(t))
		ctx := context.Background()
		result, ranked, err := BuildUniverse(ctx, deps, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if result.SymbolsBuilt == 0 {
			t.Fatal("expected symbols_built > 0")
		}
		if len(ranked) == 0 {
			t.Fatal("expected at least one ranked symbol")
		}
		// Snapshot file should exist.
		path := filepath.Join(deps.WorkDir, "data", "state", "universe_snapshot.json")
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("snapshot not persisted: %v", statErr)
		}
	})

	t.Run("full_rebuild_flag", func(t *testing.T) {
		deps := buildDepsFixture(t, tempDir(t))
		ctx := context.Background()
		result, _, err := BuildUniverse(ctx, deps, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.FullRebuild {
			t.Fatal("expected FullRebuild=true")
		}
	})

	t.Run("all_nil_optionals_no_panic", func(t *testing.T) {
		deps := buildDepsFixture(t, tempDir(t))
		deps.Quotes = nil
		deps.RiskFilter = nil
		deps.NarrativeBridge = nil

		ctx := context.Background()
		result, ranked, err := BuildUniverse(ctx, deps, false)
		if err != nil {
			t.Fatalf("unexpected error with nil optionals: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		// Without quotes, ScoringScreener drops all symbols due to missing quotes.
		// That's expected graceful degradation — but it must be *labelled*
		// degradation (issue #1944 Batch 3 / I25), never a bare zero that a
		// reader could mistake for "the market has no qualifying stock".
		if result.QuotesStatus != QuotesStatusProviderUnavailable {
			t.Errorf("QuotesStatus = %q, want %q", result.QuotesStatus, QuotesStatusProviderUnavailable)
		}
		if result.RankedFallbackReason != RankedFallbackQuoteProviderUnavailable {
			t.Errorf("RankedFallbackReason = %q, want %q",
				result.RankedFallbackReason, RankedFallbackQuoteProviderUnavailable)
		}
		if result.RankedTrustworthy {
			t.Error("RankedTrustworthy = true although no quote provider was wired")
		}
		if len(ranked) != 0 {
			t.Errorf("expected ranked=0 without quotes, got %d", len(ranked))
		}
	})

	t.Run("risk_filter_wired_pass_through", func(t *testing.T) {
		deps := buildDepsFixture(t, tempDir(t))
		// Wire a pass-through RiskExclusionFilter with nil internal providers.
		deps.RiskFilter = NewRiskExclusionFilter(nil, nil, nil)

		ctx := context.Background()
		result, ranked, err := BuildUniverse(ctx, deps, false)
		if err != nil {
			t.Fatalf("unexpected error with risk filter: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if len(ranked) == 0 {
			t.Fatal("expected ranked symbols with pass-through risk filter")
		}
	})

	t.Run("quote_fetch_failure_warn_and_continue", func(t *testing.T) {
		deps := buildDepsFixture(t, tempDir(t))
		deps.Quotes = &mockQuoteProv{err: errors.New("provider offline")}

		ctx := context.Background()
		result, _, err := BuildUniverse(ctx, deps, false)
		if err != nil {
			t.Fatalf("unexpected error on quote failure: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil result")
		}
	})

	t.Run("narrative_scrape_failure_warn_and_continue", func(t *testing.T) {
		workDir := tempDir(t)
		deps := buildDepsFixture(t, workDir)

		// Replace the narrative bridge with one whose fetcher always errors.
		errorFetcher := func(_ context.Context, _ string) (*FeedData, error) {
			return nil, errors.New("RSS feed unreachable")
		}
		neb := NewNarrativeEventBridgeWithFetcher(
			filepath.Join(workDir, "narrative_cache.json"),
			errorFetcher,
		)
		neb.Configure(testSmartUniverseConfig())
		deps.NarrativeBridge = neb

		ctx := context.Background()
		result, _, err := BuildUniverse(ctx, deps, false)
		if err != nil {
			t.Fatalf("unexpected error on narrative scrape failure: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil result")
		}
	})
}

// ─────────────────── 7. WatchlistMu concurrent serialization ─────────────

// TestWatchlistMuConcurrent verifies that daily and weekly task closures
// sharing the same WatchlistMu pointer do not race when invoked concurrently
// under -race. Each goroutine sets clockFunc to a different time to trigger
// its respective branch, then both call CheckD6Expiry under the shared lock.
func TestWatchlistMuConcurrent(t *testing.T) {
	workDir := tempDir(t)

	// pre-create the watchlist file so CheckD6Expiry doesn't create new entries.
	dir := filepath.Join(workDir, "data", "state")
	os.MkdirAll(dir, 0o750)
	os.WriteFile(filepath.Join(dir, "universe_watchlist.json"), sampleWatchlist([]string{"2330"}, 0), 0o640)

	mu := &sync.Mutex{}

	deps := UniverseBuilderDeps{
		Mapper: &mockMapper{
			classifications: map[string]*IndustryClassification{
				"2330": {Symbol: "2330", Level1: IndustrySegment{ID: "semiconductor", Name: "半導體"}},
				"2317": {Symbol: "2317", Level1: IndustrySegment{ID: "tech", Name: "科技"}},
			},
		},
		Tree:        newFakeTree(sampleClassificationTree()),
		SupplyChain: &mockSupplyChain{},
		Screener:    &mockScreener{passAll: true},
		FactorEng: &mockFactorEng{scores: map[string]map[string]float64{
			"2330": {"pe": 100, "volume": 80},
			"2317": {"pe": 90, "volume": 70},
		}},
		Quotes: &mockQuoteProv{quotes: map[string]domain.Quote{
			"2330": {Symbol: "2330", Last: 500, Volume: 20_000_000, AsOf: time.Now()},
			"2317": {Symbol: "2317", Last: 100, Volume: 20_000_000, AsOf: time.Now()},
		}},
		Config:      testSmartUniverseConfig(),
		WorkDir:     workDir,
		WatchlistMu: mu,
	}

	loc := time.FixedZone("TW", 8*3600)
	monday0600 := time.Date(2026, 6, 15, 6, 0, 0, 0, loc)  // Monday 06:00 TW
	tuesday0600 := time.Date(2026, 6, 16, 6, 0, 0, 0, loc) // Tuesday 06:00 TW

	// Use atomic.Value so clockFunc can be set safely from concurrent goroutines
	// without racing on the package-level var itself.
	var clockVal atomic.Value
	clockVal.Store(monday0600)
	defer func() { clockFunc = time.Now }()
	clockFunc = func() time.Time { return clockVal.Load().(time.Time) }

	var wg sync.WaitGroup
	wg.Add(2)

	// Weekly: Monday 06:00 → triggers weekly rebuild.
	go func() {
		defer wg.Done()
		clockVal.Store(monday0600)
		fn := NewWeeklyUniverseRebuildTask(deps)
		_ = fn(context.Background())
	}()

	// Daily: Tuesday 06:00 → triggers daily refresh.
	go func() {
		defer wg.Done()
		clockVal.Store(tuesday0600)
		fn := NewDailyUniverseRefreshTask(deps)
		_ = fn(context.Background())
	}()

	wg.Wait()
	// If we reach here without -race complaints, the test passes.
}

// ─────────────────── 8. Closure time-gating via clockFunc ─────────────────

// TestDailyRefreshTimeGating verifies that the daily refresh closure skips
// execution on Monday, triggers on Tuesday at 06:00, and skips at 06:01.
func TestDailyRefreshTimeGating(t *testing.T) {
	loc := time.FixedZone("TW", 8*3600)
	deps := buildDepsFixture(t, tempDir(t))

	t.Run("monday_0600_skip", func(t *testing.T) {
		defer func() { clockFunc = time.Now }()
		monday0600 := time.Date(2026, 6, 15, 6, 0, 0, 0, loc)
		clockFunc = func() time.Time { return monday0600 }

		fn := NewDailyUniverseRefreshTask(deps)
		err := fn(context.Background())
		if err != nil {
			t.Fatalf("expected nil on monday skip, got: %v", err)
		}
		// No snapshot should be created on Monday skip.
		path := filepath.Join(deps.WorkDir, "data", "state", "universe_snapshot.json")
		if _, statErr := os.Stat(path); statErr == nil {
			t.Fatal("expected no snapshot on Monday skip")
		}
	})

	t.Run("tuesday_0600_triggers_build", func(t *testing.T) {
		workDir := tempDir(t)
		deps2 := buildDepsFixture(t, workDir)

		defer func() { clockFunc = time.Now }()
		tuesday0600 := time.Date(2026, 6, 16, 6, 0, 0, 0, loc)
		clockFunc = func() time.Time { return tuesday0600 }

		fn := NewDailyUniverseRefreshTask(deps2)
		err := fn(context.Background())
		if err != nil {
			t.Fatalf("expected nil on tuesday trigger, got: %v", err)
		}
		// Snapshot should be created.
		path := filepath.Join(workDir, "data", "state", "universe_snapshot.json")
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("expected snapshot after tuesday build: %v", statErr)
		}
	})

	t.Run("tuesday_0601_skip_outside_window", func(t *testing.T) {
		workDir := tempDir(t)
		deps3 := buildDepsFixture(t, workDir)

		defer func() { clockFunc = time.Now }()
		tuesday0601 := time.Date(2026, 6, 16, 6, 1, 1, 0, loc)
		clockFunc = func() time.Time { return tuesday0601 }

		fn := NewDailyUniverseRefreshTask(deps3)
		err := fn(context.Background())
		if err != nil {
			t.Fatalf("expected nil when outside trigger window, got: %v", err)
		}
		// No snapshot should be created outside trigger window.
		path := filepath.Join(workDir, "data", "state", "universe_snapshot.json")
		if _, statErr := os.Stat(path); statErr == nil {
			t.Fatal("expected no snapshot outside trigger window")
		}
	})
}

// TestWeeklyRebuildTimeGating verifies that the weekly rebuild closure
// only triggers on Monday at 06:00.
func TestWeeklyRebuildTimeGating(t *testing.T) {
	loc := time.FixedZone("TW", 8*3600)

	t.Run("monday_0600_triggers_rebuild", func(t *testing.T) {
		workDir := tempDir(t)
		deps := buildDepsFixture(t, workDir)

		defer func() { clockFunc = time.Now }()
		monday0600 := time.Date(2026, 6, 15, 6, 0, 0, 0, loc)
		clockFunc = func() time.Time { return monday0600 }

		fn := NewWeeklyUniverseRebuildTask(deps)
		err := fn(context.Background())
		if err != nil {
			t.Fatalf("expected nil on monday rebuild, got: %v", err)
		}
		path := filepath.Join(workDir, "data", "state", "universe_snapshot.json")
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("expected snapshot after weekly rebuild: %v", statErr)
		}
	})

	t.Run("tuesday_skip_not_monday", func(t *testing.T) {
		workDir := tempDir(t)
		deps := buildDepsFixture(t, workDir)

		defer func() { clockFunc = time.Now }()
		tuesday0600 := time.Date(2026, 6, 16, 6, 0, 0, 0, loc)
		clockFunc = func() time.Time { return tuesday0600 }

		fn := NewWeeklyUniverseRebuildTask(deps)
		err := fn(context.Background())
		if err != nil {
			t.Fatalf("expected nil on tuesday skip, got: %v", err)
		}
		path := filepath.Join(workDir, "data", "state", "universe_snapshot.json")
		if _, statErr := os.Stat(path); statErr == nil {
			t.Fatal("expected no snapshot on tuesday weekly skip")
		}
	})
}

// ─────────────────── 9. TotalClassifiedSymbols ────────────────────────────

// TestTotalClassifiedSymbols verifies counting of representative stocks
// across all Level-1 industry segments and nil-safety.
func TestTotalClassifiedSymbols(t *testing.T) {
	t.Run("counts_representative_stocks", func(t *testing.T) {
		tree := newFakeTree([]IndustrySegment{
			{ID: "semi", Name: "半導體", RepresentativeStocks: []string{"2330", "2317"}},
			{ID: "tech", Name: "科技", RepresentativeStocks: []string{"2454", "2382", "3008"}},
			{ID: "fin", Name: "金融", RepresentativeStocks: []string{"2881"}},
		})
		got := TotalClassifiedSymbols(tree)
		if got != 6 {
			t.Fatalf("expected 6 representative stocks, got %d", got)
		}
	})

	t.Run("nil_tree_returns_zero", func(t *testing.T) {
		got := TotalClassifiedSymbols(nil)
		if got != 0 {
			t.Fatalf("expected 0 for nil tree, got %d", got)
		}
	})

	t.Run("empty_level1_returns_zero", func(t *testing.T) {
		tree := newFakeTree(nil)
		got := TotalClassifiedSymbols(tree)
		if got != 0 {
			t.Fatalf("expected 0 for empty tree, got %d", got)
		}
	})
}

// ─────────────────── 10. InferredIndustry via CheckD6Expiry ───────────────

// TestCheckD6ExpiryInferredIndustry verifies that when a symbol not in the
// mapper creates a new watchlist entry, its industry falls back to "unknown".
func TestCheckD6ExpiryInferredIndustry(t *testing.T) {
	t.Run("missing_classification_falls_back_to_unknown", func(t *testing.T) {
		workDir := tempDir(t)
		ranked := sampleRankedSymbols(1) // "2330" only
		prev := []string{"UKNOWN_SYM"}   // was ranked before but not today → new entry

		mapper := &mockMapper{
			// No classification for UKNOWN_SYM — GetClassification returns false
			classifications: map[string]*IndustryClassification{},
		}

		err := CheckD6Expiry(workDir, ranked, prev, mapper, 60)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		path := filepath.Join(workDir, "data", "state", "universe_watchlist.json")
		raw, _ := os.ReadFile(path)
		var wl Watchlist
		json.Unmarshal(raw, &wl)

		for _, e := range wl.Symbols {
			if e.Symbol == "UKNOWN_SYM" {
				if e.Industry != "unknown" {
					t.Fatalf("expected industry 'unknown' for unmapped symbol, got %q", e.Industry)
				}
				return
			}
		}
		t.Fatal("UKNOWN_SYM entry not found in watchlist")
	})

	t.Run("empty_industry_name_falls_back_to_unknown", func(t *testing.T) {
		workDir := tempDir(t)
		ranked := sampleRankedSymbols(1) // "2330" only
		prev := []string{"EMPTY_NAME"}   // creates new entry

		mapper := &mockMapper{
			// Classification exists but Level1.Name is empty → falls back to "unknown"
			classifications: map[string]*IndustryClassification{
				"EMPTY_NAME": {Symbol: "EMPTY_NAME", Level1: IndustrySegment{ID: "any", Name: ""}},
			},
		}

		err := CheckD6Expiry(workDir, ranked, prev, mapper, 60)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		path := filepath.Join(workDir, "data", "state", "universe_watchlist.json")
		raw, _ := os.ReadFile(path)
		var wl Watchlist
		json.Unmarshal(raw, &wl)

		for _, e := range wl.Symbols {
			if e.Symbol == "EMPTY_NAME" {
				if e.Industry != "unknown" {
					t.Fatalf("expected industry 'unknown' for empty-name symbol, got %q", e.Industry)
				}
				return
			}
		}
		t.Fatal("EMPTY_NAME entry not found in watchlist")
	})
}

// ─────────────────── 11. CheckD6Expiry corrupt watchlist ──────────────────

// TestCheckD6ExpiryCorruptWatchlist verifies that a corrupt JSON watchlist
// file is recovered by resetting to a fresh Watchlist.
func TestCheckD6ExpiryCorruptWatchlist(t *testing.T) {
	workDir := tempDir(t)
	dir := filepath.Join(workDir, "data", "state")
	os.MkdirAll(dir, 0o750)
	os.WriteFile(filepath.Join(dir, "universe_watchlist.json"), []byte("{garbage}"), 0o640)

	mapper := &mockMapper{
		classifications: map[string]*IndustryClassification{
			"2330": {Symbol: "2330", Level1: IndustrySegment{ID: "semiconductor", Name: "半導體"}},
		},
	}
	ranked := sampleRankedSymbols(1) // 2330
	prev := []string{"2330"}         // same as ranked → no failures

	err := CheckD6Expiry(workDir, ranked, prev, mapper, 60)
	if err != nil {
		t.Fatalf("unexpected error after corrupt JSON recovery: %v", err)
	}
	// The file should have been rewritten as valid JSON.
	raw, _ := os.ReadFile(filepath.Join(dir, "universe_watchlist.json"))
	var wl Watchlist
	if unmarshalErr := json.Unmarshal(raw, &wl); unmarshalErr != nil {
		t.Fatalf("watchlist not rewritten as valid JSON: %v", unmarshalErr)
	}
}

// ─────────────────── 12. alignToTarget negative diff ──────────────────────

// TestAlignToTargetNegativeDiff verifies that a time before the target is
// still considered aligned when within 1 minute (the negative diff is
// absolute-valued in the implementation).
func TestAlignToTargetNegativeDiff(t *testing.T) {
	loc := time.FixedZone("TW", 8*3600)

	t.Run("30s_before_target", func(t *testing.T) {
		tm := time.Date(2026, 6, 16, 5, 59, 30, 0, loc)
		if !alignToTarget(tm) {
			t.Fatal("05:59:30 should be within ±1min of 06:00")
		}
	})

	t.Run("61s_before_target", func(t *testing.T) {
		tm := time.Date(2026, 6, 16, 5, 58, 59, 0, loc)
		if alignToTarget(tm) {
			t.Fatal("05:58:59 should NOT be within ±1min of 06:00")
		}
	})
}

// ─────────────────── compile-time guard ──────────────────────────────────

// Ensure runtime is linked (prevent unused import when race detector is not compiled in).
var _ = runtime.GOOS

// Ensure test helpers compile and are referenced.
var _ = fmt.Sprintf
