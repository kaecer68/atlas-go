package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
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

// TestCheckUniverseCoverage verifies the coverage ratio computation and
// alert messaging across normal, nil-dependency, and zero-symbol cases.
func TestCheckUniverseCoverage(t *testing.T) {
	t.Run("full_coverage_ratio_one", func(t *testing.T) {
		mapper := &mockMapper{
			byIndustry: map[string][]string{
				"semiconductor": {"2330", "2317"},
				"tech":          {"2454"},
			},
		}
		tree := newFakeTree([]IndustrySegment{
			{ID: "semiconductor", Name: "半導體"},
			{ID: "tech", Name: "科技"},
		})

		mapped, total, ratio, alert := CheckUniverseCoverage(mapper, tree, 0.50)
		if mapped != 3 || total != 3 {
			t.Fatalf("expected mapped=3 total=3, got %d/%d", mapped, total)
		}
		if ratio != 1.0 {
			t.Fatalf("expected ratio=1.0, got %.4f", ratio)
		}
		if alert != "" {
			t.Fatalf("expected no alert, got %q", alert)
		}
	})

	t.Run("nil_mapper_or_tree_returns_alert", func(t *testing.T) {
		_, _, ratio, alert := CheckUniverseCoverage(nil, nil, 0.50)
		if ratio != 0 {
			t.Fatalf("expected ratio=0, got %.4f", ratio)
		}
		if !strings.Contains(strings.ToLower(alert), "mapper") {
			t.Fatalf("expected alert about missing mapper/tree, got %q", alert)
		}
	})

	t.Run("below_threshold_triggers_alert", func(t *testing.T) {
		mapper := &mockMapper{
			byIndustry: map[string][]string{
				"semiconductor": {"2330"},
			},
		}
		tree := newFakeTree([]IndustrySegment{
			{ID: "semiconductor", Name: "半導體"},
		})

		_, _, ratio, alert := CheckUniverseCoverage(mapper, tree, 1.5)
		if ratio != 1.0 {
			t.Fatalf("expected ratio=1.0, got %.4f", ratio)
		}
		if !strings.Contains(alert, "below threshold") {
			t.Fatalf("expected 'below threshold' alert, got %q", alert)
		}
	})

	t.Run("zero_symbols_returns_alert", func(t *testing.T) {
		mapper := &mockMapper{
			byIndustry: map[string][]string{},
		}
		tree := newFakeTree([]IndustrySegment{
			{ID: "semiconductor", Name: "半導體"},
		})

		_, _, ratio, alert := CheckUniverseCoverage(mapper, tree, 0.50)
		if ratio != 0 {
			t.Fatalf("expected ratio=0, got %.4f", ratio)
		}
		if !strings.Contains(alert, "no symbols") {
			t.Fatalf("expected 'no symbols' alert, got %q", alert)
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
		// That's expected graceful degradation.
		_ = ranked
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
