package monitoring

// universe_scale_blockers_test.go — "what else blocks a non-zero ranking?"
// (issue #1944 Batch 3 / I25, follow-up question 5).
//
// Production evidence (Mac Mini 2026-09-25 06:00Z):
//
//	symbols_gathered     count=1599
//	industry_filter_ok   input=1599 output=1599
//	scoring_ok           input=1599 ranked=0
//
// The nil quote provider (I25) explains ranked=0. These tests answer the next
// question at the SAME population size (1,599 symbols): which of the remaining
// Layer-2 stages can still produce an empty ranking, and which cannot.
//
// Every test below states the mechanism, the measured numbers, and whether a
// production run can reach it.

import (
	"context"
	"strconv"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/screener"
)

// scaleBlockersN is the production universe size (symbols_gathered count=1599).
const scaleBlockersN = 1599

// productionScaleSymbols returns n four-digit Taiwan stock codes.
func productionScaleSymbols(n int) []string {
	out := make([]string, 0, n)
	for i := range n {
		out = append(out, strconv.Itoa(1101+i))
	}
	return out
}

// productionScaleQuotes builds one quote per symbol with the given volume.
func productionScaleQuotes(symbols []string, last float64, volume int64) map[string]domain.Quote {
	out := make(map[string]domain.Quote, len(symbols))
	for _, sym := range symbols {
		out[sym] = domain.Quote{Symbol: sym, Last: last, Volume: volume, Market: "TW"}
	}
	return out
}

// productionScaleScreener returns a ScoringScreener configured exactly like
// BuildUniverse configures it: production defaults (TopN 150, NT$10M turnover
// floor, NT$10 price floor, 40 % concentration cap) and the real
// screener.Engine + portfolio factor-engine adapter.
func productionScaleScreener() *ScoringScreener {
	factorEngine := portfolio.NewFactorEngine()
	ss := NewScoringScreener(
		screener.NewEngine(factorEngine, portfolio.NewFundamentalProvider()),
		AdaptFactorEngine(factorEngine),
	)
	ss.TopN = 150
	ss.VolumeFloorTWD = 10_000_000
	ss.PriceMin = 10
	ss.MaxIndustryConcentration = 0.40
	ss.IndustryMapper = &mockMapper{}
	return ss
}

// ── 2a. applyVolumeAndPriceFilters: quotes without Volume wipe the universe ──

// TestScaleBlocker_QuotesWithoutVolumeWipeTheUniverse pins the mechanism that
// makes the I25 family dangerous: a quote provider that answers for every
// symbol but leaves Volume at 0 produces the SAME ranked=0 as a nil provider,
// and QuotesStatus is "ok" for both. 1599/1599 symbols are dropped.
func TestScaleBlocker_QuotesWithoutVolumeWipeTheUniverse(t *testing.T) {
	symbols := productionScaleSymbols(scaleBlockersN)
	ss := productionScaleScreener()

	noVolume := productionScaleQuotes(symbols, 100, 0)
	rankedZeroVolume := ss.Rank(symbols, noVolume)
	t.Logf("Volume=0 (quote present): input=%d ranked=%d", len(symbols), len(rankedZeroVolume))
	if len(rankedZeroVolume) != 0 {
		t.Fatalf("ranked = %d, want 0: the volume floor must drop volume-less quotes", len(rankedZeroVolume))
	}

	withVolume := productionScaleQuotes(symbols, 100, 1_000_000) // 1M shares × NT$100 = NT$100M
	rankedWithVolume := ss.Rank(symbols, withVolume)
	t.Logf("Volume=1,000,000 shares: input=%d ranked=%d", len(symbols), len(rankedWithVolume))
	if len(rankedWithVolume) == 0 {
		t.Fatalf("ranked = 0 with a valid volume: the rest of Layer 2 is not the blocker")
	}

	// The attribution added with this change must name the cause instead of
	// leaving the operator to guess.
	_, stats := ss.applyVolumeAndPriceFilters(symbols, noVolume)
	if stats.ZeroVolume != scaleBlockersN {
		t.Errorf("zero_volume = %d, want %d", stats.ZeroVolume, scaleBlockersN)
	}
	_, okStats := ss.applyVolumeAndPriceFilters(symbols, withVolume)
	if okStats.ZeroVolume != 0 || okStats.Passed != scaleBlockersN {
		t.Errorf("valid-quote stats = %+v, want zero_volume=0 passed=%d", okStats, scaleBlockersN)
	}
}

// ── 2b. The volume UNIT decides the outcome (股 vs 張) ────────────────────────

// TestScaleBlocker_VolumeUnitIsDecisive shows that the NT$10M turnover floor is
// only meaningful if Volume is a SHARE count. The same real trade — 1,000 張
// (lots) of a NT$100 stock = 1,000,000 shares = NT$100M turnover — passes as
// shares and fails as lots, i.e. a 1000x unit error drops every symbol whose
// turnover is under NT$10B.
func TestScaleBlocker_VolumeUnitIsDecisive(t *testing.T) {
	symbols := productionScaleSymbols(scaleBlockersN)
	ss := productionScaleScreener()

	asShares := productionScaleQuotes(symbols, 100, 1_000_000) // 1,000 張 expressed in 股
	asLots := productionScaleQuotes(symbols, 100, 1_000)       // 1,000 張 expressed in 張

	rankedShares := ss.Rank(symbols, asShares)
	rankedLots := ss.Rank(symbols, asLots)
	t.Logf("NT$100, 1,000 lots: as shares (1,000,000) ranked=%d; as lots (1,000) ranked=%d",
		len(rankedShares), len(rankedLots))

	if len(rankedShares) == 0 {
		t.Fatalf("shares interpretation ranked 0; the fixture is wrong")
	}
	if len(rankedLots) != 0 {
		t.Errorf("lots interpretation ranked %d, want 0 (NT$100k turnover < NT$10M floor)", len(rankedLots))
	}
	if got := float64(1_000*100) / float64(1_000_000*100); got != 0.001 {
		t.Errorf("unit factor = %v, want 0.001", got)
	}
}

// ── 2c. screener.Engine with zero-value criteria passes everything ───────────

// TestScaleBlocker_ZeroValueCriteriaPassAll pins that BuildUniverse's screener
// call is a pass-through: ScoringCriteria is never populated on the universe
// path, and an empty criteria set has no filters to fail.
func TestScaleBlocker_ZeroValueCriteriaPassAll(t *testing.T) {
	symbols := productionScaleSymbols(scaleBlockersN)
	quotes := productionScaleQuotes(symbols, 100, 1_000_000)

	criteria := domain.ScreeningCriteria{}
	if criteria.HasFilters() {
		t.Fatal("zero-value ScreeningCriteria reports filters; the pass-all assumption is wrong")
	}

	engine := screener.NewEngine(portfolio.NewFactorEngine(), portfolio.NewFundamentalProvider())
	passed, err := engine.ScreenUniverse(context.Background(), symbols, criteria, quotes)
	if err != nil {
		t.Fatalf("ScreenUniverse: %v", err)
	}
	t.Logf("screener zero-value criteria: input=%d passed=%d", len(symbols), len(passed))
	if len(passed) != len(symbols) {
		t.Fatalf("passed = %d, want %d (no criteria means no rejections)", len(passed), len(symbols))
	}
}

// ── 2d. scoreAndRank needs non-empty factor scores, not fundamentals ─────────

// TestScaleBlocker_FactorEngineWithoutFundamentalsStillScores documents that
// the production factor engine returns a non-empty score map for every symbol
// even with no fundamentals and no bridge inputs, so scoreAndRank keeps the
// symbol. What it does NOT get is volume / foreign_flow information: those two
// sub-scores are 0 for the whole universe (bridgeInputs is nil on this path),
// which silently costs 0.35 of the 0.95 weight budget instead of failing.
func TestScaleBlocker_FactorEngineWithoutFundamentalsStillScores(t *testing.T) {
	symbols := productionScaleSymbols(scaleBlockersN)
	quotes := productionScaleQuotes(symbols, 100, 1_000_000)
	ss := productionScaleScreener()

	ranked := ss.Rank(symbols, quotes)
	if len(ranked) == 0 {
		t.Fatal("ranked = 0: the factor engine produced no scores at all")
	}

	var zeroVolume, zeroForeign, missingBreakdown int
	for _, r := range ranked {
		if r.FactorBreakdown["volume"] == 0 {
			zeroVolume++
		}
		if r.FactorBreakdown["foreign_flow"] == 0 {
			zeroForeign++
		}
		if len(r.FactorBreakdown) == 0 {
			missingBreakdown++
		}
	}
	t.Logf("ranked=%d zero_volume_subscore=%d zero_foreign_flow_subscore=%d empty_breakdown=%d",
		len(ranked), zeroVolume, zeroForeign, missingBreakdown)
	if zeroVolume != len(ranked) || zeroForeign != len(ranked) {
		t.Errorf("volume / foreign_flow sub-scores are not uniformly zero: %d / %d of %d",
			zeroVolume, zeroForeign, len(ranked))
	}
	if missingBreakdown != 0 {
		t.Errorf("%d ranked symbols carry an empty factor breakdown", missingBreakdown)
	}

	// The score is still positive: the ranking survives (degraded quality, not
	// a wipe-out). Top score must be > 0 or every symbol would tie at 0.
	if ranked[0].Score <= 0 {
		t.Errorf("top score = %v, want > 0", ranked[0].Score)
	}
}

// ── 2e. Concentration cap and Top-N ─────────────────────────────────────────

// TestScaleBlocker_ConcentrationCapShapesButCannotZeroTheRanking pins the two
// arithmetic facts the owner asked about:
//
//   - the cap keeps max(int(0.40 × len(ranked)), 1) per industry, so it can
//     never return an empty list for non-empty input;
//   - at 1,599 symbols all in one industry it cuts the list to 639, which is
//     still far above Top-N = 150, so the cap cannot be why ranked is small.
func TestScaleBlocker_ConcentrationCapShapesButCannotZeroTheRanking(t *testing.T) {
	symbols := productionScaleSymbols(scaleBlockersN)
	ss := productionScaleScreener()

	// mockMapper has no classifications, so every symbol lands in "unknown".
	oneIndustry := make([]RankedSymbol, 0, len(symbols))
	for _, sym := range symbols {
		oneIndustry = append(oneIndustry, RankedSymbol{Symbol: sym, Industry: "unknown", Score: 50})
	}

	capped := ss.ApplyConcentrationCap(oneIndustry)
	wantCap := int(0.40 * float64(len(oneIndustry)))
	t.Logf("all-one-industry: ranked=%d cap=%d kept=%d", len(oneIndustry), wantCap, len(capped))
	if len(capped) != wantCap {
		t.Errorf("kept = %d, want %d (0.40 x %d)", len(capped), wantCap, len(oneIndustry))
	}
	if len(capped) == 0 {
		t.Fatal("cap returned an empty list; it must keep at least one symbol")
	}

	// Degenerate but worth pinning: a single symbol is always kept.
	if got := ss.ApplyConcentrationCap([]RankedSymbol{{Symbol: "1101", Industry: "unknown"}}); len(got) != 1 {
		t.Errorf("single-symbol cap kept %d, want 1", len(got))
	}

	// Top-N is the real ceiling on the published universe.
	if ss.TopN >= wantCap {
		t.Fatalf("TopN = %d is not below the cap %d; the fixture no longer isolates the cap", ss.TopN, wantCap)
	}
}

// TestScaleBlocker_PartialQuoteCoverageStillRanks covers the production shape
// where the provider covers only part of the population: the default
// ATLAS_MARKET_DATA_PROVIDER=twse endpoint is STOCK_DAY_ALL, which returns
// listed (上市) names only — 1,380 rows measured 2026-09-24 — so the 上櫃 part
// of the 1,599-symbol population never receives a quote. Those symbols are
// attributed to no_quote, and the ranking still comes back non-empty.
func TestScaleBlocker_PartialQuoteCoverageStillRanks(t *testing.T) {
	symbols := productionScaleSymbols(scaleBlockersN)
	const covered = 1380 // TWSE STOCK_DAY_ALL rows measured 2026-09-24

	quotes := productionScaleQuotes(symbols[:covered], 100, 1_000_000)

	ss := productionScaleScreener()
	ranked := ss.Rank(symbols, quotes)
	_, stats := ss.applyVolumeAndPriceFilters(symbols, quotes)
	t.Logf("partial coverage: input=%d no_quote=%d zero_volume=%d below_turnover=%d below_price=%d survivors=%d ranked=%d",
		stats.Input, stats.NoQuote, stats.ZeroVolume, stats.BelowTurnoverFloor, stats.BelowPriceFloor,
		stats.Passed, len(ranked))

	if stats.NoQuote != scaleBlockersN-covered {
		t.Errorf("no_quote = %d, want %d (the uncovered part of the population)", stats.NoQuote, scaleBlockersN-covered)
	}
	if stats.Passed != covered {
		t.Errorf("survivors = %d, want all %d quoted symbols", stats.Passed, covered)
	}
	if len(ranked) == 0 {
		t.Fatal("partial coverage produced an empty ranking")
	}
}

// TestScaleBlocker_FilterStatsAttributionIsExhaustive keeps the diagnostic
// counters honest: every input symbol is attributed exactly once, so the log
// line can be read as a partition of the 1,599-symbol population.
func TestScaleBlocker_FilterStatsAttributionIsExhaustive(t *testing.T) {
	symbols := productionScaleSymbols(scaleBlockersN)
	quotes := make(map[string]domain.Quote, len(symbols))
	for i, sym := range symbols {
		switch {
		case i%7 == 0:
			// no quote at all
			continue
		case i%5 == 0:
			quotes[sym] = domain.Quote{Symbol: sym, Last: 100} // Volume 0
		case i%3 == 0:
			quotes[sym] = domain.Quote{Symbol: sym, Last: 100, Volume: 1_000} // NT$100k
		case i%2 == 0:
			// NT$15M turnover (passes the floor) at NT$5 (below PriceMin), so
			// this bucket is reached only through the price rule.
			quotes[sym] = domain.Quote{Symbol: sym, Last: 5, Volume: 3_000_000}
		default:
			quotes[sym] = domain.Quote{Symbol: sym, Last: 100, Volume: 1_000_000}
		}
	}

	ss := productionScaleScreener()
	survivors, stats := ss.applyVolumeAndPriceFilters(symbols, quotes)
	total := stats.NoQuote + stats.ZeroVolume + stats.BelowTurnoverFloor + stats.BelowPriceFloor + stats.Passed
	t.Logf("attribution: input=%d no_quote=%d zero_volume=%d below_turnover=%d below_price=%d passed=%d",
		stats.Input, stats.NoQuote, stats.ZeroVolume, stats.BelowTurnoverFloor, stats.BelowPriceFloor, stats.Passed)

	if total != stats.Input {
		t.Errorf("attribution total = %d, want %d (every symbol must have exactly one cause)", total, stats.Input)
	}
	if stats.NoQuote == 0 || stats.ZeroVolume == 0 || stats.BelowTurnoverFloor == 0 ||
		stats.BelowPriceFloor == 0 || stats.Passed == 0 {
		t.Errorf("fixture did not exercise every bucket: %+v", stats)
	}
	if len(survivors) != stats.Passed {
		t.Errorf("survivors = %d, stats.Passed = %d", len(survivors), stats.Passed)
	}
}
