// universe_exclusion_reasons_test.go — issue #2019: every symbol that leaves
// the pipeline must be attributable to the rule that removed it.
//
// The three properties these tests pin, in the order they matter:
//  1. the arithmetic closes (no symbol disappears between input and output
//     without appearing in a counter);
//  2. a stage that did not run is distinguishable from a stage that ran and
//     excluded nobody (absent key vs present zero);
//  3. the counts survive the snapshot round trip, because the snapshot file is
//     the only thing a reader has when a number looks wrong.
package monitoring

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
)

// ── 1. risk-filter summarizer ────────────────────────────────────────────

func TestSummarizeRiskExclusionReasons(t *testing.T) {
	t.Run("nil means the stage did not run", func(t *testing.T) {
		if got := SummarizeRiskExclusionReasons(nil); got != nil {
			t.Fatalf("SummarizeRiskExclusionReasons(nil) = %v, want nil (absent keys, not zeros)", got)
		}
	})

	t.Run("empty result still reports the vocabulary with zeros", func(t *testing.T) {
		got := SummarizeRiskExclusionReasons([]RiskExclusionResult{})
		if got == nil {
			t.Fatal("a stage that ran must return the vocabulary, so a zero can be read as checked-and-clean")
		}
		for _, key := range []string{
			ExclusionReasonRiskVaRContribution,
			ExclusionReasonRiskVolatility,
			ExclusionReasonRiskLiquidity,
			ExclusionReasonRiskUnspecified,
			ExclusionReasonRiskTotal,
		} {
			if _, ok := got[key]; !ok {
				t.Errorf("key %q missing from the vocabulary", key)
			}
			if got[key] != 0 {
				t.Errorf("%s = %d, want 0", key, got[key])
			}
		}
	})

	t.Run("per-rule counts and the symbol total are different questions", func(t *testing.T) {
		got := SummarizeRiskExclusionReasons([]RiskExclusionResult{
			{Symbol: "2330", Passed: true},
			// One symbol failing TWO rules: counted once per rule, once in total.
			{Symbol: "1101", Passed: false, FailReasons: []string{"liquidity", "volatility"}},
			{Symbol: "1216", Passed: false, FailReasons: []string{"liquidity"}},
			{Symbol: "1301", Passed: false, FailReasons: []string{"var_contribution"}},
		})
		want := map[string]int{
			ExclusionReasonRiskVaRContribution: 1,
			ExclusionReasonRiskVolatility:      1,
			ExclusionReasonRiskLiquidity:       2,
			ExclusionReasonRiskUnspecified:     0,
			ExclusionReasonRiskTotal:           3,
		}
		for key, count := range want {
			if got[key] != count {
				t.Errorf("%s = %d, want %d", key, got[key], count)
			}
		}
		// Documented asymmetry: the named rules can sum to more than the total.
		named := got[ExclusionReasonRiskVaRContribution] + got[ExclusionReasonRiskVolatility] + got[ExclusionReasonRiskLiquidity]
		if named <= got[ExclusionReasonRiskTotal] {
			t.Fatalf("named rules summed to %d with total %d: the multi-rule case is not exercising the asymmetry",
				named, got[ExclusionReasonRiskTotal])
		}
	})

	t.Run("a failed symbol without a reason is still counted", func(t *testing.T) {
		got := SummarizeRiskExclusionReasons([]RiskExclusionResult{
			{Symbol: "2330", Passed: false},
		})
		if got[ExclusionReasonRiskUnspecified] != 1 {
			t.Errorf("%s = %d, want 1 (an exclusion must never be unattributable)",
				ExclusionReasonRiskUnspecified, got[ExclusionReasonRiskUnspecified])
		}
		if got[ExclusionReasonRiskTotal] != 1 {
			t.Errorf("%s = %d, want 1", ExclusionReasonRiskTotal, got[ExclusionReasonRiskTotal])
		}
	})

	t.Run("an unknown rule token still surfaces", func(t *testing.T) {
		got := SummarizeRiskExclusionReasons([]RiskExclusionResult{
			{Symbol: "2330", Passed: false, FailReasons: []string{"brand_new_check"}},
		})
		if got["risk_brand_new_check"] != 1 {
			t.Errorf("a new rule must surface as risk_<token>, got %v", got)
		}
	})
}

// ── 2. screener accounting ───────────────────────────────────────────────

// exclusionReasonsScreener builds a deterministic screener whose every drop
// cause can be engineered from the quotes and the mocks.
//
// Fixture (10 candidates, one per cause):
//
//	2330  survives everything                     → ranked
//	2331  no quote                                → screener_no_quote
//	2332  quote with Volume 0                     → screener_zero_volume
//	2333  Volume*Last under the floor             → screener_below_turnover_floor
//	2334  priced under PriceMin (turnover ok)     → screener_below_price_floor
//	2335  binary screener rejects it              → screener_binary_rejected
//	2336  factor engine has no scores for it      → screener_no_factor_score
//	2337  tech, loses the concentration cap       → concentration_cap
//	2338  finance, loses the concentration cap    → concentration_cap
//	2339  ranked but cut by TopN                  → screener_topn_truncated
func exclusionReasonsScreener() (*ScoringScreener, []string, map[string]domain.Quote) {
	now := time.Now()
	ok := func(sym string) domain.Quote {
		// 100 × 200,000 = 20M, above the 10M floor; price above the 10 floor.
		return domain.Quote{Symbol: sym, Last: 100, Volume: 200_000, AsOf: now}
	}
	symbols := []string{"2330", "2331", "2332", "2333", "2334", "2335", "2336", "2337", "2338", "2339"}
	quotes := map[string]domain.Quote{
		"2330": ok("2330"),
		// 2331 deliberately absent.
		"2332": {Symbol: "2332", Last: 100, Volume: 0, AsOf: now},
		"2333": {Symbol: "2333", Last: 50, Volume: 100_000, AsOf: now},  // 5M < 10M
		"2334": {Symbol: "2334", Last: 5, Volume: 5_000_000, AsOf: now}, // 25M ok, price 5 < 10
		"2335": ok("2335"),
		"2336": ok("2336"),
		"2337": ok("2337"),
		"2338": ok("2338"),
		"2339": ok("2339"),
	}
	// Distinct total scores for the four scored symbols, so the ranking (and
	// therefore which symbol survives the cap and the TopN cut) is deterministic:
	// sort.Slice is not stable, so tied scores would make the assertions flaky.
	// 2330 (100) > 2338 (80) > 2339 (60) > 2337 (10).
	flat := func(v float64) map[string]float64 {
		return map[string]float64{"pe": v, "pb": v, "volume": v, "momentum": v, "quality": v, "foreign_flow": v}
	}
	scores := map[string]map[string]float64{
		"2330": flat(100),
		"2337": flat(10),
		"2338": flat(80),
		"2339": flat(60),
	}
	mapper := &mockMapper{classifications: map[string]*IndustryClassification{
		"2330": {Symbol: "2330", Level1: IndustrySegment{ID: "tech"}},
		"2337": {Symbol: "2337", Level1: IndustrySegment{ID: "tech"}},
		"2338": {Symbol: "2338", Level1: IndustrySegment{ID: "finance"}},
		"2339": {Symbol: "2339", Level1: IndustrySegment{ID: "finance"}},
	}}
	ss := &ScoringScreener{
		Screener:                 &mockScreener{reject: map[string]bool{"2335": true}},
		FactorEng:                &mockFactorEng{scores: scores},
		IndustryMapper:           mapper,
		Weights:                  DefaultScreenerWeights(),
		TopN:                     1,
		MaxIndustryConcentration: 0.25, // 0.25 × 4 scored = 1 per industry
		VolumeFloorTWD:           10_000_000,
		PriceMin:                 10,
		FactorScoreMaxAge:        30 * 24 * time.Hour,
	}
	return ss, symbols, quotes
}

func TestRankWithStats_AccountsForEveryDrop(t *testing.T) {
	ss, symbols, quotes := exclusionReasonsScreener()

	ranked, stats := ss.RankWithStats(symbols, quotes)

	// Each engineered cause is attributed exactly.
	wantFields := map[string]struct{ got, want int }{
		"screener_no_quote":             {stats.NoQuote, 1},
		"screener_zero_volume":          {stats.ZeroVolume, 1},
		"screener_below_turnover_floor": {stats.BelowTurnoverFloor, 1},
		"screener_below_price_floor":    {stats.BelowPriceFloor, 1},
		"screener_binary_rejected":      {stats.BinaryRejected, 1},
		"screener_no_factor_score":      {stats.NoFactorScore, 1},
		"concentration_cap":             {stats.ConcentrationCap, 2},
		"screener_topn_truncated":       {stats.TopNTruncated, 1},
		"input":                         {stats.Input, 10},
		"survivors":                     {stats.Survivors, 6},
		"ranked":                        {stats.Ranked, 1},
		"ranked slice length":           {len(ranked), 1},
	}
	for name, w := range wantFields {
		if w.got != w.want {
			t.Errorf("%s = %d, want %d (stats=%+v, ranked=%v)", name, w.got, w.want, stats, ranked)
		}
	}

	// Identity 1: nobody disappears between input and output.
	if sum := stats.Ranked + stats.ScreenerTotal() + stats.ConcentrationCap; sum != stats.Input {
		t.Errorf("input %d != ranked %d + screener_total %d + concentration_cap %d = %d",
			stats.Input, stats.Ranked, stats.ScreenerTotal(), stats.ConcentrationCap, sum)
	}

	// The map is a faithful rendering of the same numbers, and screener_total
	// equals the sum of the individual screener keys (each symbol is attributed
	// to exactly one cause).
	reasons := stats.ExclusionReasons()
	wantReasons := map[string]int{
		ExclusionReasonScreenerNoQuote:            1,
		ExclusionReasonScreenerZeroVolume:         1,
		ExclusionReasonScreenerBelowTurnoverFloor: 1,
		ExclusionReasonScreenerBelowPriceFloor:    1,
		ExclusionReasonScreenerBinaryRejected:     1,
		ExclusionReasonScreenerNoFactorScore:      1,
		ExclusionReasonScreenerTopNTruncated:      1,
		ExclusionReasonScreenerTotal:              7,
		ExclusionReasonConcentrationCap:           2,
	}
	for key, want := range wantReasons {
		if reasons[key] != want {
			t.Errorf("reasons[%s] = %d, want %d", key, reasons[key], want)
		}
	}
	individual := 0
	for _, key := range []string{
		ExclusionReasonScreenerNoQuote,
		ExclusionReasonScreenerZeroVolume,
		ExclusionReasonScreenerBelowTurnoverFloor,
		ExclusionReasonScreenerBelowPriceFloor,
		ExclusionReasonScreenerBinaryRejected,
		ExclusionReasonScreenerNoFactorScore,
		ExclusionReasonScreenerTopNTruncated,
	} {
		individual += reasons[key]
	}
	if individual != reasons[ExclusionReasonScreenerTotal] {
		t.Errorf("individual screener reasons sum to %d, but screener_total = %d",
			individual, reasons[ExclusionReasonScreenerTotal])
	}

	// Rank and RankWithStats must not diverge: the accounting is an addition to
	// the existing behavior, not a second implementation of it.
	plain := ss.Rank(symbols, quotes)
	if len(plain) != len(ranked) || plain[0].Symbol != ranked[0].Symbol {
		t.Errorf("Rank = %v, RankWithStats = %v: the two forms disagree", plain, ranked)
	}
}

func TestRankStats_ScreenerTotalIsTheScreenerStageDrop(t *testing.T) {
	ss, symbols, quotes := exclusionReasonsScreener()
	_, stats := ss.RankWithStats(symbols, quotes)
	// Everything from the input down to the scored list (the concentration cap
	// runs after that point, on the scored list) ...
	if want := stats.Input - stats.BeforeCap; stats.ScreenerTotal() != want+stats.TopNTruncated {
		t.Fatalf("ScreenerTotal() = %d, want %d (input %d - before cap %d + topn_truncated %d)",
			stats.ScreenerTotal(), want+stats.TopNTruncated, stats.Input, stats.BeforeCap, stats.TopNTruncated)
	}
	// ... and the whole account closes: the cap and the TopN cut are the only
	// drops outside the screener stage's own total.
	if sum := stats.Ranked + stats.ScreenerTotal() + stats.ConcentrationCap; sum != stats.Input {
		t.Fatalf("input %d != ranked %d + screener_total %d + concentration_cap %d = %d",
			stats.Input, stats.Ranked, stats.ScreenerTotal(), stats.ConcentrationCap, sum)
	}
}

// ── 3. persistence ───────────────────────────────────────────────────────

// TestBuildUniverse_PersistsExclusionReasons drives the real pipeline with a
// risk filter configured so that every ranked symbol fails the liquidity
// re-check — the production shape that produced the 130/150 exclusion of issue
// #1987 whose cause was only a code comment until this field existed.
func TestBuildUniverse_PersistsExclusionReasons(t *testing.T) {
	deps := buildDepsFixture(t, tempDir(t))
	cfg := deps.Config
	// A floor no symbol can clear: liquidity excludes 100% of the ranked input.
	cfg.MinDailyAmountTWD = config.ParameterMetadata[float64]{Value: 1e15}
	deps.Config = cfg
	risk := NewRiskExclusionFilter(nil, deps.Quotes, nil)
	risk.Configure(cfg)
	deps.RiskFilter = risk

	result, ranked, err := BuildUniverse(context.Background(), deps, false)
	if err != nil {
		t.Fatalf("BuildUniverse: %v", err)
	}
	if len(ranked) == 0 {
		t.Fatal("fixture produced no ranked symbols")
	}
	if result.SymbolsExcluded != len(ranked) {
		t.Fatalf("symbols_excluded = %d, want %d (every ranked symbol fails the re-check)",
			result.SymbolsExcluded, len(ranked))
	}

	reasons := result.SymbolsExcludedReasons
	if reasons == nil {
		t.Fatal("symbols_excluded_reasons is nil: the exclusion is not attributable")
	}
	if got := reasons[ExclusionReasonRiskLiquidity]; got != len(ranked) {
		t.Errorf("reasons[%s] = %d, want %d", ExclusionReasonRiskLiquidity, got, len(ranked))
	}
	// Identity 2: the risk total is the exclusion total.
	if got := reasons[ExclusionReasonRiskTotal]; got != result.SymbolsExcluded {
		t.Errorf("reasons[%s] = %d, want symbols_excluded %d", ExclusionReasonRiskTotal, got, result.SymbolsExcluded)
	}
	// Identity 1, on the snapshot's own fields.
	if sum := result.SymbolsRanked + reasons[ExclusionReasonScreenerTotal] + reasons[ExclusionReasonConcentrationCap]; sum != result.SymbolsFiltered {
		t.Errorf("symbols_filtered %d != symbols_ranked %d + screener_total %d + concentration_cap %d = %d",
			result.SymbolsFiltered, result.SymbolsRanked,
			reasons[ExclusionReasonScreenerTotal], reasons[ExclusionReasonConcentrationCap], sum)
	}

	// The counts must survive the round trip: the snapshot file is what an
	// operator reads.
	snap, err := LoadUniverseSnapshot(deps.WorkDir)
	if err != nil {
		t.Fatalf("LoadUniverseSnapshot: %v", err)
	}
	if snap.Result == nil {
		t.Fatal("snapshot carries no result")
	}
	if got := snap.Result.SymbolsExcludedReasons[ExclusionReasonRiskTotal]; got != result.SymbolsExcluded {
		t.Errorf("round-tripped reasons[%s] = %d, want %d", ExclusionReasonRiskTotal, got, result.SymbolsExcluded)
	}
	if len(snap.Result.SymbolsExcludedReasons) != len(reasons) {
		t.Errorf("round trip changed the map size: %d vs %d", len(snap.Result.SymbolsExcludedReasons), len(reasons))
	}
}

// TestBuildUniverse_RiskStageNotRunLeavesNoRiskKeys pins the absent-vs-zero
// contract: with no risk filter wired, the snapshot must not claim the risk
// stage looked at anything.
func TestBuildUniverse_RiskStageNotRunLeavesNoRiskKeys(t *testing.T) {
	deps := buildDepsFixture(t, tempDir(t))
	deps.RiskFilter = nil

	result, _, err := BuildUniverse(context.Background(), deps, false)
	if err != nil {
		t.Fatalf("BuildUniverse: %v", err)
	}
	if result.SymbolsExcluded != 0 {
		t.Fatalf("symbols_excluded = %d without a risk filter, want 0", result.SymbolsExcluded)
	}
	for key := range result.SymbolsExcludedReasons {
		if strings.HasPrefix(key, "risk_") {
			t.Errorf("key %q present although the risk stage never ran; a missing key is how a reader tells 'not run' from 'ran and found nothing'", key)
		}
	}
	// The screener stage always runs, so its keys must be there even when its
	// input produced no exclusions.
	if _, ok := result.SymbolsExcludedReasons[ExclusionReasonScreenerTotal]; !ok {
		t.Errorf("screener_total missing: the screener always runs, so its accounting must always be present (%v)", result.SymbolsExcludedReasons)
	}
}
