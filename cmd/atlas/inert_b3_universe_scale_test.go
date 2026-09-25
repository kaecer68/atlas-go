package main

// inert_b3_universe_scale_test.go — production-scale acceptance evidence for
// issue #1944 Batch 3 / I25 (the wired quote provider) and for the follow-up
// question "what else blocks the ranking?".
//
// Production evidence that motivated this file (Mac Mini, 2026-09-25 06:00Z):
//
//	symbols_gathered     count=1599
//	industry_filter_ok   input=1599 output=1599
//	scoring_ok           input=1599 ranked=0
//
// The I25 fix (newUniverseBuilderDepsWithQuotes wiring a real quote provider)
// explains the ranked=0, but only a run at the production population size can
// show that ranked>0 comes back. The population is the per-stock industry
// substrate's Symbols() (issue #1943/#1977: 1988 upstream rows -> 1599 mapped),
// so the fixture here is exactly that size.
//
// The pipeline is driven through the production wiring function
// (newUniverseBuilderDepsWithQuotes) with a quote provider injected by the
// test; no network call is made.

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

// scalePopulation is the production universe size the owner asked us to prove
// (symbols_gathered count=1599 on 2026-09-25).
const scalePopulation = 1599

// scaleQuotedCoverage is how many of the 1,599 symbols receive a quote in the
// fixture. It is the real TWSE STOCK_DAY_ALL row count measured live on
// 2026-09-24 (1,380 rows: 1,093 four-digit listed stocks plus ETFs/warrants),
// which is what the default ATLAS_MARKET_DATA_PROVIDER=twse covers. The
// remaining 219 symbols of the population are TPEx (上櫃) names that the
// TWSE-only endpoint never returns — see the coverage note in the test report.
const scaleQuotedCoverage = 1380

// scaleRealSymbolsEnv lets an operator run the same acceptance assertions
// against a REAL production symbol population from an arbitrary path (one
// symbol per line, "#" starts a comment). It takes precedence over the
// checked-in fixture below, so the test can also be pointed at a freshly dumped
// population from the production host.
const scaleRealSymbolsEnv = "ATLAS_UNIVERSE_SCALE_SYMBOLS"

// scaleRealSymbolsFixture is the checked-in real population: the 1,599 symbols
// the production substrate installed on 2026-09-25 (see the file header for its
// provenance and regeneration recipe). Committing it keeps the real-population
// assertions runnable without a database, an env var or a network call.
const scaleRealSymbolsFixture = "testdata/universe_scale_symbols.txt"

// scaleLiveEnv gates the tests that hit the real market-data endpoint.
const scaleLiveEnv = "ATLAS_TEST_UNIVERSE_LIVE"

// ── Fixture ─────────────────────────────────────────────────────────────────

// generatedProductionScaleSymbols returns n legal Taiwan stock codes
// (four-digit, the same shape as 1101..2699). The generated fixture exists so
// the scale assertions always run in CI; the real list is opt-in through
// scaleRealSymbolsEnv.
func generatedProductionScaleSymbols(n int) []string {
	out := make([]string, 0, n)
	for i := range n {
		out = append(out, strconv.Itoa(1101+i))
	}
	return out
}

// parseScaleSymbolList reads a newline-separated symbol list, dropping blanks
// and "#" comments and normalizing the ".TW" suffix the snapshot may carry.
func parseScaleSymbolList(raw string) []string {
	var out []string
	seen := make(map[string]bool)
	for line := range strings.SplitSeq(raw, "\n") {
		sym := strings.TrimSpace(line)
		if sym == "" || strings.HasPrefix(sym, "#") {
			continue
		}
		sym = strings.TrimSuffix(sym, ".TW")
		if seen[sym] {
			continue
		}
		seen[sym] = true
		out = append(out, sym)
	}
	return out
}

// scaleSubstrate is an industry.SymbolIndustrySubstrate over a fixed symbol
// list. It stands in for cmd/atlas's storeSymbolIndustrySubstrate (the
// production population source) so the fixture works without a database.
type scaleSubstrate struct {
	symbols  []string
	sectorBy map[string]industry.SectorID
}

func (s *scaleSubstrate) Symbols() []string { return append([]string(nil), s.symbols...) }

func (s *scaleSubstrate) ResolveL1(symbol string) (industry.SectorID, bool) {
	sector, ok := s.sectorBy[strings.TrimSuffix(symbol, ".TW")]
	return sector, ok
}

// newScaleSubstrate assigns every symbol a canonical L1 sector.
//
// The mix is deliberately skewed the way the real listed market is (electronics
// and semiconductor dominate) so the 40 % industry concentration cap is
// exercised rather than vacuous under a uniform 20-sector split.
func newScaleSubstrate(symbols []string) *scaleSubstrate {
	sectors := monitoringCanonicalL1()
	sub := &scaleSubstrate{
		symbols:  symbols,
		sectorBy: make(map[string]industry.SectorID, len(symbols)),
	}
	for i, sym := range symbols {
		sub.sectorBy[sym] = industry.SectorID(scaleSectorFor(i, sectors))
	}
	return sub
}

// monitoringCanonicalL1 returns the canonical L1 sector IDs used by the
// production substrate (the sector vocabulary the mapper resolves to).
func monitoringCanonicalL1() []string {
	return []string{
		"auto", "biotech", "cement", "chemicals", "construction", "electronics",
		"energy", "financials", "food", "machinery", "optoelectronics",
		"other_electronics", "plastics", "retail", "semiconductor", "shipping",
		"steel", "telecom", "textiles", "tourism",
	}
}

func scaleSectorFor(i int, sectors []string) string {
	switch {
	case i%100 < 45:
		return "electronics"
	case i%100 < 70:
		return "semiconductor"
	default:
		return sectors[i%len(sectors)]
	}
}

// scaleQuoteProvider is the injected quote provider: it answers for a fixed
// quote map and records what the pipeline asked for. It exposes no IsMock
// method, so the pipeline treats its answers as real market input (which is the
// behaviour under test: only a real provider may publish rank worthy=true).
type scaleQuoteProvider struct {
	quotes map[string]domain.Quote

	// Calls / RequestedPerCall record every fetch, because the pipeline fetches
	// twice per run: Step 3 asks for the whole filtered population, and the
	// Layer 2.5 risk filter (N-U7) re-uses the same provider instance for the
	// ranked subset. RequestedMax is the Step 3 width.
	Calls            int
	RequestedPerCall []int
	RequestedMax     int
}

func (p *scaleQuoteProvider) GetQuotes(_ context.Context, _ time.Time, symbols []string) ([]domain.Quote, error) {
	p.Calls++
	p.RequestedPerCall = append(p.RequestedPerCall, len(symbols))
	if len(symbols) > p.RequestedMax {
		p.RequestedMax = len(symbols)
	}
	out := make([]domain.Quote, 0, len(symbols))
	for _, sym := range symbols {
		if q, ok := p.quotes[sym]; ok {
			out = append(out, q)
		}
	}
	return out, nil
}

// newScaleQuoteProvider builds quotes for the first scaleQuotedCoverage symbols
// (the TWSE-covered part of the population).
//
// Quote values — and why:
//
//	Last   : NT$15-394 for the bulk, so the NT$10 PriceMinimum floor is not the
//	         binding constraint for most names. Every 50th symbol is priced at
//	         NT$8.5 to exercise the price filter.
//	Volume : SHARES (股), the unit TWSE STOCK_DAY_ALL reports (成交股數) and the
//	         unit ScoringScreener.applyVolumeAndPriceFilters assumes when it
//	         multiplies Volume*Last to compare against VolumeFloorTWD=NT$10M.
//	         400k-2.2M shares × the price above gives NT$6M-NT$800M daily
//	         turnover, i.e. a mix of pass and fail. Every 25th symbol is thin
//	         (5.4M TWD) on purpose so the volume floor drops a visible slice.
//	AsOf   : the run time. Production providers stamp time.Now() (see
//	         twse_openapi.go convertToQuote), which keeps factor scores "fresh".
func newScaleQuoteProvider(symbols []string, asOf time.Time) *scaleQuoteProvider {
	p := &scaleQuoteProvider{quotes: make(map[string]domain.Quote, len(symbols))}
	for i, sym := range symbols {
		if i >= scaleQuotedCoverage {
			// 上櫃 (TPEx) names: STOCK_DAY_ALL, the TWSE-only endpoint the
			// default provider selects, never returns them.
			continue
		}
		var last float64
		var shares int64
		switch {
		case i%50 == 0:
			last, shares = 8.5, 400_000 // below the NT$10 price floor
		case i%25 == 0:
			last, shares = 45, 120_000 // NT$5.4M turnover, below the NT$10M floor
		default:
			last = float64(15 + (i*17)%380)
			shares = int64(400_000 + (i*4999)%1_800_000)
		}
		p.quotes[sym] = domain.Quote{
			Symbol:     sym,
			Last:       last,
			Open:       last,
			High:       last,
			Low:        last,
			Volume:     shares,
			Market:     "TW",
			AsOf:       asOf,
			IsTradable: true,
			Source:     "scale-fixture",
		}
	}
	return p
}

// ── Acceptance ──────────────────────────────────────────────────────────────

// TestBuildUniverseProductionScale_GeneratedFixture runs the production wiring
// over a 1,599-symbol population and asserts the pipeline ranks something.
func TestBuildUniverseProductionScale_GeneratedFixture(t *testing.T) {
	runProductionScalePipeline(t, generatedProductionScaleSymbols(scalePopulation), "generated")
}

// TestBuildUniverseProductionScale_RealSymbolList runs the same assertions on
// the real population when an operator supplies it (ATLAS_UNIVERSE_SCALE_SYMBOLS).
// Missing data skips instead of failing: CI has no access to the production DB.
func TestBuildUniverseProductionScale_RealSymbolList(t *testing.T) {
	path, symbols, ok := realSymbolPopulation()
	if !ok {
		t.Skipf("no real symbol population at %s (set %s=<file> to supply one); skipping",
			scaleRealSymbolsFixture, scaleRealSymbolsEnv)
	}
	if len(symbols) != scalePopulation {
		t.Fatalf("%s holds %d symbols, want the production population %d", path, len(symbols), scalePopulation)
	}
	runProductionScalePipeline(t, symbols, path)
}

// realSymbolPopulation resolves the real population: ATLAS_UNIVERSE_SCALE_SYMBOLS
// first, then the checked-in fixture. ok=false means neither is readable, which
// callers turn into a skip (CI must not fail when the data is absent).
func realSymbolPopulation() (path string, symbols []string, ok bool) {
	path = os.Getenv(scaleRealSymbolsEnv)
	if path == "" {
		path = scaleRealSymbolsFixture
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return path, nil, false
	}
	return path, parseScaleSymbolList(string(raw)), true
}

// TestBuildUniverseProductionScale_LiveQuoteProvider is the end-to-end check
// with the REAL market-data provider instead of the injected stub: the same
// production wiring, the real 1,599-symbol population and the real
// gateway-backed provider (with no API keys configured the chain resolves to
// the TWSE STOCK_DAY_ALL endpoint the production default
// ATLAS_MARKET_DATA_PROVIDER=twse uses).
//
// It is env-gated so CI never depends on upstream availability:
//
//	ATLAS_TEST_UNIVERSE_LIVE=1 go test ./cmd/atlas -run LiveQuoteProvider -v
//
// Two assertions differ from the stub runs because they describe the real
// upstream, not the pipeline: coverage may be partial (the endpoint returns
// listed names only) and ranked may therefore be smaller than in the fixture.
func TestBuildUniverseProductionScale_LiveQuoteProvider(t *testing.T) {
	if os.Getenv(scaleLiveEnv) == "" {
		t.Skipf("set %s=1 to hit the real TWSE market-data endpoint", scaleLiveEnv)
	}

	path, symbols, ok := realSymbolPopulation()
	if !ok || len(symbols) != scalePopulation {
		t.Fatalf("live run needs the %d-symbol population at %s (got %d from %q)",
			scalePopulation, scaleRealSymbolsFixture, len(symbols), path)
	}

	substrate := newScaleSubstrate(symbols)
	adapter := monitoring.AdaptClassificationTree(industry.DefaultClassification())
	cfg := config.Config{WorkDir: t.TempDir()}
	suCfg := config.DefaultParametersConfig().SmartUniverse

	// Production wiring: newUniverseBuilderDeps builds its own gateway-backed
	// provider (newUniverseQuoteProvider), so no stub is injected here.
	deps := newUniverseBuilderDeps(cfg, adapter, nil, nil, suCfg, substrate)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	result, ranked, err := monitoring.BuildUniverse(ctx, deps, false)
	if err != nil {
		t.Fatalf("live BuildUniverse: %v", err)
	}
	t.Logf("live provider: provider=%T", deps.Quotes)
	t.Logf("symbols_gathered count=%d", result.SymbolsBuilt)
	t.Logf("industry_filter_ok input=%d output=%d", result.SymbolsBuilt, result.SymbolsFiltered)
	t.Logf("scoring_ok input=%d ranked=%d", result.SymbolsFiltered, result.SymbolsRanked)
	t.Logf("quotes: status=%s returned=%d excluded=%d ranked_trustworthy=%v",
		result.QuotesStatus, result.QuotesReturned, result.SymbolsExcluded, result.RankedTrustworthy)

	if result.QuotesStatus != monitoring.QuotesStatusOK {
		t.Fatalf("live quotes_status = %q (fallback %q): the upstream call did not produce usable quotes",
			result.QuotesStatus, result.RankedFallbackReason)
	}
	if result.SymbolsRanked <= 0 {
		t.Fatalf("live ranked = 0 at production scale with a real provider")
	}
	if len(ranked) != result.SymbolsRanked {
		t.Errorf("ranked slice = %d, counter = %d", len(ranked), result.SymbolsRanked)
	}

	// Diagnostic second pass with the Top-N cut disabled, so the report can say
	// how much of the ranking comes from the candidate pool and how much is the
	// published-size cap. This is a test-local copy of the parameter table; the
	// production default is not touched.
	diagCfg := suCfg
	diagCfg.TopN.Value = 0
	diagDeps := newUniverseBuilderDeps(cfg, adapter, nil, nil, diagCfg, substrate)
	diagResult, diagRanked, err := monitoring.BuildUniverse(ctx, diagDeps, false)
	if err != nil {
		t.Fatalf("diagnostic BuildUniverse (TopN disabled): %v", err)
	}
	t.Logf("no-TopN diagnostic: input=%d survivors_after_cap=%d (top_n=%d cut the published list to %d)",
		diagResult.SymbolsFiltered, diagResult.SymbolsRanked, suCfg.TopN.Value, result.SymbolsRanked)
	if diagResult.SymbolsRanked < result.SymbolsRanked || len(diagRanked) != diagResult.SymbolsRanked {
		t.Errorf("diagnostic ranking %d inconsistent with the published %d", diagResult.SymbolsRanked, result.SymbolsRanked)
	}
}

func runProductionScalePipeline(t *testing.T, symbols []string, source string) {
	t.Helper()

	substrate := newScaleSubstrate(symbols)
	adapter := monitoring.AdaptClassificationTree(industry.DefaultClassification())
	cfg := config.Config{WorkDir: t.TempDir()}
	suCfg := config.DefaultParametersConfig().SmartUniverse
	asOf := time.Now()
	provider := newScaleQuoteProvider(symbols, asOf)

	deps := newUniverseBuilderDepsWithQuotes(cfg, adapter, nil, nil, suCfg, substrate, provider)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result, ranked, err := monitoring.BuildUniverse(ctx, deps, false)
	if err != nil {
		t.Fatalf("BuildUniverse at production scale (%s): %v", source, err)
	}

	// The three production log lines, reproduced with the test's numbers. The
	// owner asked for scoring_ok verbatim in the PR body.
	t.Logf("fixture source: %s", source)
	t.Logf("symbols_gathered count=%d", result.SymbolsBuilt)
	t.Logf("industry_filter_ok input=%d output=%d", result.SymbolsBuilt, result.SymbolsFiltered)
	t.Logf("scoring_ok input=%d ranked=%d", result.SymbolsFiltered, result.SymbolsRanked)
	t.Logf("quotes: status=%s returned=%d excluded=%d ranked_trustworthy=%v fetch_widths=%v",
		result.QuotesStatus, result.QuotesReturned, result.SymbolsExcluded, result.RankedTrustworthy,
		provider.RequestedPerCall)

	if result.SymbolsBuilt != scalePopulation {
		t.Fatalf("symbols_gathered = %d, want %d (the substrate population must reach the pipeline intact)",
			result.SymbolsBuilt, scalePopulation)
	}
	if result.SymbolsFiltered != scalePopulation {
		t.Errorf("industry_filter_ok output = %d, want %d (the production filter passes the whole population)",
			result.SymbolsFiltered, scalePopulation)
	}
	if provider.RequestedMax != scalePopulation {
		t.Errorf("quote provider was asked for at most %d symbols, want the whole filtered population %d (calls: %v)",
			provider.RequestedMax, scalePopulation, provider.RequestedPerCall)
	}
	if result.QuotesStatus != monitoring.QuotesStatusOK {
		t.Errorf("quotes_status = %q, want %q", result.QuotesStatus, monitoring.QuotesStatusOK)
	}
	if !result.RankedTrustworthy {
		t.Errorf("ranked_trustworthy = false (fallback reason %q); the ranked list would be unpublishable",
			result.RankedFallbackReason)
	}
	if result.SymbolsRanked <= 0 {
		t.Fatalf("symbols_ranked = 0 at production scale: something still blocks the ranking (I25 regression)")
	}
	if result.SymbolsRanked != len(ranked) {
		t.Errorf("symbols_ranked = %d but %d symbols were returned", result.SymbolsRanked, len(ranked))
	}
	if topN := suCfg.TopN.Value; topN > 0 && result.SymbolsRanked > topN {
		t.Errorf("symbols_ranked = %d exceeds top_n = %d", result.SymbolsRanked, topN)
	}
	if result.SymbolsBuilt < scaleQuotedCoverage {
		t.Fatalf("fixture broken: population %d smaller than the quoted coverage %d", result.SymbolsBuilt, scaleQuotedCoverage)
	}

	// The snapshot every downstream reader consumes must carry the same result.
	snapshot, err := monitoring.LoadUniverseSnapshot(cfg.WorkDir)
	if err != nil {
		t.Fatalf("LoadUniverseSnapshot: %v", err)
	}
	if snapshot.Result == nil || snapshot.Result.SymbolsRanked != result.SymbolsRanked {
		t.Errorf("snapshot ranked count does not match the in-memory result")
	}
	if len(snapshot.Ranked) != len(ranked) {
		t.Errorf("snapshot carries %d ranked symbols, want %d", len(snapshot.Ranked), len(ranked))
	}
}
