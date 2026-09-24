package stockpicker

import (
	"math"
	"testing"
)

// ─── IndustryWinRate (issue #1942) ─────────────────────────────────────────
//
// Fixture-driven tests for the canonical industry-level caliber. The resolver
// is injected, so these tests pin the aggregation contract (grouping,
// coverage, unmapped handling, Wilson reuse) without depending on the L1
// taxonomy contents.

const testCostRate = 0.00585

// fakeIndustryResolver returns a SectorResolver backed by a symbol->industry
// map. Symbols absent from the map are unmapped (ok=false).
func fakeIndustryResolver(mapping map[string]string) SectorResolver {
	return func(symbol string) (string, bool) {
		id, ok := mapping[symbol]
		return id, ok
	}
}

func industryOutcome(symbol, date, source string, forwardReturn float64) SignalOutcome {
	return SignalOutcome{
		Symbol:        symbol,
		TriggerDate:   date,
		Source:        source,
		ForwardReturn: forwardReturn,
		CostRate:      testCostRate,
	}
}

func near(t *testing.T, got, want float64, label string) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}

// TestIndustryWinRate_GroupsByIndustryAndCoverage pins the row key
// (industry_id, source, window), the shared math, and the coverage block.
func TestIndustryWinRate_GroupsByIndustryAndCoverage(t *testing.T) {
	const source = "stockpicker-momentum-20d-positive"
	outcomes := []SignalOutcome{
		industryOutcome("2330", "2026-07-01", source, 0.02),                                      // hit  (net +0.01415)
		industryOutcome("2330", "2026-07-08", source, 0.001),                                     // miss (net -0.00485)
		industryOutcome("2454", "2026-07-02", source, -0.01),                                     // miss
		industryOutcome("1301", "2026-07-03", source, 0.03),                                      // hit
		industryOutcome("9999", "2026-07-04", source, 0.05),                                      // unmapped
		industryOutcome("2330", "2026-07-05", "stockpicker-price-volume-bottom-divergence", 0.9), // other source: skipped
	}
	mapping := map[string]string{"2330": "semiconductor", "2454": "semiconductor", "1301": "plastics"}

	report, err := IndustryWinRate(source, outcomes, fakeIndustryResolver(mapping), testCostRate, 2, 0.95)
	if err != nil {
		t.Fatalf("IndustryWinRate: %v", err)
	}
	if report.Source != source || report.ConditionID != "momentum-20d-positive" || report.Direction != "buy" {
		t.Fatalf("identity fields: %+v", report)
	}
	if len(report.Industries) != 2 {
		t.Fatalf("industries = %d, want 2: %+v", len(report.Industries), report.Industries)
	}
	// Rows are sorted by industry id: plastics, semiconductor.
	plastics, semis := report.Industries[0], report.Industries[1]
	if plastics.IndustryID != "plastics" || semis.IndustryID != "semiconductor" {
		t.Fatalf("industry order = %s/%s, want plastics/semiconductor", plastics.IndustryID, semis.IndustryID)
	}

	// semiconductor: 3 observations across 2 symbols, 1 hit.
	if semis.Observations != 3 || semis.Symbols != 2 || semis.Hits != 1 {
		t.Errorf("semiconductor counts: %+v", semis)
	}
	near(t, semis.WinRate, 1.0/3.0, "semiconductor win_rate")
	wantLower, wantUpper := WilsonScoreInterval(1, 3, 0.95)
	near(t, semis.WilsonLower, wantLower, "semiconductor wilson_lower")
	near(t, semis.WilsonUpper, wantUpper, "semiconductor wilson_upper")
	near(t, semis.AvgForwardReturn, (0.02+0.001-0.01)/3, "semiconductor avg_forward_return")
	near(t, semis.AvgNetForwardReturn, (0.02+0.001-0.01)/3-testCostRate, "semiconductor avg_net_forward_return")
	near(t, semis.CoveragePct, 60, "semiconductor coverage_pct")
	if semis.CalibrationStatus != string(CalibrationEligible) {
		t.Errorf("semiconductor calibration = %q, want eligible (minSamples=2)", semis.CalibrationStatus)
	}
	if semis.DataStart != "2026-07-01" || semis.DataEnd != "2026-07-08" {
		t.Errorf("semiconductor data range = %s..%s", semis.DataStart, semis.DataEnd)
	}

	// plastics: 1 observation, 1 hit, below minSamples -> calibrating.
	if plastics.Observations != 1 || plastics.Hits != 1 || plastics.Symbols != 1 {
		t.Errorf("plastics counts: %+v", plastics)
	}
	near(t, plastics.WinRate, 1, "plastics win_rate")
	near(t, plastics.CoveragePct, 20, "plastics coverage_pct")
	if plastics.CalibrationStatus != string(CalibrationCalibrating) {
		t.Errorf("plastics calibration = %q, want calibrating", plastics.CalibrationStatus)
	}

	// Coverage: 5 observations for the source (4 mapped, 1 unmapped), 4 symbols (3 mapped).
	cov := report.Coverage
	if cov.TotalObservations != 5 || cov.MappedObservations != 4 || cov.UnmappedObservations != 1 {
		t.Errorf("coverage counts: %+v", cov)
	}
	if cov.TotalSymbols != 4 || cov.MappedSymbols != 3 {
		t.Errorf("coverage symbols: %+v", cov)
	}
	near(t, cov.CoveragePct, 80, "coverage_pct")
	near(t, cov.SymbolCoveragePct, 75, "symbol_coverage_pct")
	if len(cov.UnmappedSymbols) != 1 || cov.UnmappedSymbols[0].Symbol != "9999" || cov.UnmappedSymbols[0].Observations != 1 {
		t.Errorf("unmapped symbols: %+v", cov.UnmappedSymbols)
	}
}

// TestIndustryWinRate_UnmappedNeverPooled: symbols without a canonical L1
// mapping must not create an "unknown" bucket — they are reported and excluded.
func TestIndustryWinRate_UnmappedNeverPooled(t *testing.T) {
	const source = "stockpicker-momentum-20d-positive"
	outcomes := []SignalOutcome{
		industryOutcome("9999", "2026-07-01", source, 0.5),
		industryOutcome("9999", "2026-07-02", source, 0.5),
		industryOutcome("0050", "2026-07-03", source, 0.5),
	}
	report, err := IndustryWinRate(source, outcomes, fakeIndustryResolver(nil), testCostRate, 1, 0.95)
	if err != nil {
		t.Fatalf("IndustryWinRate: %v", err)
	}
	if len(report.Industries) != 0 {
		t.Fatalf("industries = %+v, want none (no mapped symbol)", report.Industries)
	}
	if report.Coverage.CoveragePct != 0 || report.Coverage.SymbolCoveragePct != 0 {
		t.Errorf("coverage should be 0: %+v", report.Coverage)
	}
	if report.Coverage.TotalObservations != 3 || report.Coverage.UnmappedObservations != 3 {
		t.Errorf("coverage counts: %+v", report.Coverage)
	}
	// 9999 has more evidence than 0050 -> first.
	if len(report.Coverage.UnmappedSymbols) != 2 ||
		report.Coverage.UnmappedSymbols[0].Symbol != "9999" || report.Coverage.UnmappedSymbols[0].Observations != 2 ||
		report.Coverage.UnmappedSymbols[1].Symbol != "0050" {
		t.Errorf("unmapped order: %+v", report.Coverage.UnmappedSymbols)
	}
}

// TestIndustryWinRate_WilsonBoundaries checks the Wilson reuse at the
// boundaries the metric must survive: a single observation and a perfect run.
func TestIndustryWinRate_WilsonBoundaries(t *testing.T) {
	const source = "stockpicker-momentum-20d-positive"
	mapping := map[string]string{"2330": "semiconductor"}

	single, err := IndustryWinRate(source, []SignalOutcome{industryOutcome("2330", "2026-07-01", source, 0.02)},
		fakeIndustryResolver(mapping), testCostRate, 1, 0.95)
	if err != nil {
		t.Fatalf("IndustryWinRate single: %v", err)
	}
	row := single.Industries[0]
	if row.WinRate != 1 {
		t.Errorf("single obs win_rate = %v, want 1", row.WinRate)
	}
	if row.WilsonLower <= 0 || row.WilsonUpper != 1 {
		t.Errorf("single obs wilson = %v..%v, want (0,1) exclusive lower and upper=1", row.WilsonLower, row.WilsonUpper)
	}

	perfect := make([]SignalOutcome, 0, 3)
	for i, date := range []string{"2026-07-01", "2026-07-02", "2026-07-03"} {
		perfect = append(perfect, industryOutcome("2330", date, source, 0.02+float64(i)*0.001))
	}
	report, err := IndustryWinRate(source, perfect, fakeIndustryResolver(mapping), testCostRate, 1, 0.95)
	if err != nil {
		t.Fatalf("IndustryWinRate perfect: %v", err)
	}
	wantLower, wantUpper := WilsonScoreInterval(3, 3, 0.95)
	near(t, report.Industries[0].WilsonLower, wantLower, "perfect wilson_lower")
	near(t, report.Industries[0].WilsonUpper, wantUpper, "perfect wilson_upper")
	if report.Industries[0].WilsonUpper != 1 {
		t.Errorf("perfect wilson_upper = %v, want 1 (clamped)", report.Industries[0].WilsonUpper)
	}
}

// TestIndustryWinRate_SkipsOtherSources: a miswired caller must not silently
// pool heterogeneous conditions into one industry number.
func TestIndustryWinRate_SkipsOtherSources(t *testing.T) {
	const source = "stockpicker-momentum-20d-positive"
	mapping := map[string]string{"2330": "semiconductor"}
	outcomes := []SignalOutcome{
		industryOutcome("2330", "2026-07-01", source, 0.02),
		industryOutcome("2330", "2026-07-01", "stockpicker-foreign-3d-net-buy", -0.5),
	}
	report, err := IndustryWinRate(source, outcomes, fakeIndustryResolver(mapping), testCostRate, 1, 0.95)
	if err != nil {
		t.Fatalf("IndustryWinRate: %v", err)
	}
	if report.Coverage.TotalObservations != 1 {
		t.Errorf("total observations = %d, want 1 (foreign source skipped, not counted)", report.Coverage.TotalObservations)
	}
	if report.Industries[0].Observations != 1 || report.Industries[0].Hits != 1 {
		t.Errorf("row: %+v", report.Industries[0])
	}
}

// TestIndustryWinRate_AvoidDirection: avoid-semantics conditions keep their
// flag at industry level too (a low win rate CONFIRMS the signal).
func TestIndustryWinRate_AvoidDirection(t *testing.T) {
	const source = "stockpicker-price-volume-top-divergence"
	report, err := IndustryWinRate(source,
		[]SignalOutcome{industryOutcome("2330", "2026-07-01", source, -0.05)},
		fakeIndustryResolver(map[string]string{"2330": "semiconductor"}), testCostRate, 1, 0.95)
	if err != nil {
		t.Fatalf("IndustryWinRate: %v", err)
	}
	if report.Direction != "avoid" || report.Industries[0].Direction != "avoid" {
		t.Fatalf("direction: report=%q row=%q", report.Direction, report.Industries[0].Direction)
	}
	if report.ConditionID != "price-volume-top-divergence" {
		t.Errorf("condition_id = %q", report.ConditionID)
	}
}

// TestIndustryWinRate_EmptyInput: no outcomes is not an error; every surface
// is empty and non-nil so JSON consumers see stable types.
func TestIndustryWinRate_EmptyInput(t *testing.T) {
	report, err := IndustryWinRate("stockpicker-momentum-20d-positive", nil, fakeIndustryResolver(nil), testCostRate, 30, 0.95)
	if err != nil {
		t.Fatalf("IndustryWinRate: %v", err)
	}
	if len(report.Industries) != 0 || report.Coverage.TotalObservations != 0 {
		t.Errorf("empty report: %+v", report)
	}
	if report.Industries == nil || report.Coverage.UnmappedSymbols == nil {
		t.Error("empty slices must be non-nil for stable JSON")
	}
	if report.Coverage.CoveragePct != 0 {
		t.Errorf("coverage_pct = %v, want 0", report.Coverage.CoveragePct)
	}
}

// TestIndustryWinRate_NilResolver: missing attribution is a wiring bug.
func TestIndustryWinRate_NilResolver(t *testing.T) {
	if _, err := IndustryWinRate("stockpicker-momentum-20d-positive", nil, nil, testCostRate, 30, 0.95); err == nil {
		t.Fatal("expected error for nil resolver")
	}
}

// TestIndustryWinRateFor covers the single-industry lookup shared by the HTTP
// handler and the MCP tool.
func TestIndustryWinRateFor(t *testing.T) {
	const source = "stockpicker-momentum-20d-positive"
	report, err := IndustryWinRate(source,
		[]SignalOutcome{industryOutcome("2330", "2026-07-01", source, 0.02)},
		fakeIndustryResolver(map[string]string{"2330": "semiconductor"}), testCostRate, 1, 0.95)
	if err != nil {
		t.Fatalf("IndustryWinRate: %v", err)
	}
	row, ok := IndustryWinRateFor(report, "semiconductor")
	if !ok || row.Observations != 1 {
		t.Fatalf("IndustryWinRateFor(semiconductor) = %+v, %v", row, ok)
	}
	if _, ok := IndustryWinRateFor(report, "shipbuilding"); ok {
		t.Error("unknown industry must not be found")
	}
}

// TestPercentage_Rounding documents the 2-decimal coverage convention.
func TestPercentage_Rounding(t *testing.T) {
	cases := []struct {
		part, whole int
		want        float64
	}{
		{0, 0, 0},
		{1, 3, 33.33},
		{2, 3, 66.67},
		{1, 2, 50},
		{851, 851, 100},
	}
	for _, tc := range cases {
		if got := percentage(tc.part, tc.whole); got != tc.want {
			t.Errorf("percentage(%d,%d) = %v, want %v", tc.part, tc.whole, got, tc.want)
		}
	}
}

// TestIndustryWinRate_TieIsNotAHit: hit = net > 0 is STRICT — a return exactly
// equal to the round-trip cost is a miss (same rule as NetHit at symbol level).
func TestIndustryWinRate_TieIsNotAHit(t *testing.T) {
	const source = "stockpicker-momentum-20d-positive"
	report, err := IndustryWinRate(source,
		[]SignalOutcome{industryOutcome("2330", "2026-07-01", source, testCostRate)},
		fakeIndustryResolver(map[string]string{"2330": "semiconductor"}), testCostRate, 1, 0.95)
	if err != nil {
		t.Fatalf("IndustryWinRate: %v", err)
	}
	row := report.Industries[0]
	if row.Hits != 0 || row.WinRate != 0 {
		t.Errorf("exact-cost return must not be a hit: %+v", row)
	}
	if row.AvgNetForwardReturn != 0 {
		t.Errorf("avg_net_forward_return = %v, want 0", row.AvgNetForwardReturn)
	}
}

// TestIndustryWinRate_ResolverEdgeCases pins both unmapped shapes: a resolver
// that reports "not found" and one that reports a found-but-empty id must both
// land in the unmapped bucket (never in an industry row).
func TestIndustryWinRate_ResolverEdgeCases(t *testing.T) {
	const source = "stockpicker-momentum-20d-positive"
	outcomes := []SignalOutcome{
		industryOutcome("2330", "2026-07-01", source, 0.02),
		industryOutcome("2454", "2026-07-02", source, 0.02),
	}
	resolver := func(symbol string) (string, bool) {
		switch symbol {
		case "2330":
			return "", true // found-but-empty id
		case "2454":
			return "semiconductor", false // id present but ok=false
		default:
			return "", false
		}
	}
	report, err := IndustryWinRate(source, outcomes, resolver, testCostRate, 1, 0.95)
	if err != nil {
		t.Fatalf("IndustryWinRate: %v", err)
	}
	if len(report.Industries) != 0 {
		t.Fatalf("industries = %+v, want none", report.Industries)
	}
	if report.Coverage.UnmappedObservations != 2 || report.Coverage.MappedObservations != 0 {
		t.Errorf("coverage: %+v", report.Coverage)
	}
	if len(report.Coverage.UnmappedSymbols) != 2 {
		t.Errorf("unmapped list: %+v", report.Coverage.UnmappedSymbols)
	}
}

// TestIndustryWinRate_MissingTriggerDate: rows without a trigger date still
// count as evidence but must not widen the reported data range.
func TestIndustryWinRate_MissingTriggerDate(t *testing.T) {
	const source = "stockpicker-momentum-20d-positive"
	report, err := IndustryWinRate(source,
		[]SignalOutcome{
			industryOutcome("2330", "2026-07-05", source, 0.02),
			industryOutcome("2330", "", source, 0.02),
		},
		fakeIndustryResolver(map[string]string{"2330": "semiconductor"}), testCostRate, 1, 0.95)
	if err != nil {
		t.Fatalf("IndustryWinRate: %v", err)
	}
	row := report.Industries[0]
	if row.Observations != 2 {
		t.Errorf("observations = %d, want 2 (a missing date is not a dropped row)", row.Observations)
	}
	if row.DataStart != "2026-07-05" || row.DataEnd != "2026-07-05" {
		t.Errorf("data range = %s..%s, want 2026-07-05..2026-07-05", row.DataStart, row.DataEnd)
	}
}

// TestIndustryWinRate_CoverageSharesSumToReportCoverage documents the row-level
// coverage_pct contract: each row is its share of the source's observations, so
// the rows add up to the report-level coverage up to 2-decimal rounding.
func TestIndustryWinRate_CoverageSharesSumToReportCoverage(t *testing.T) {
	const source = "stockpicker-momentum-20d-positive"
	mapping := map[string]string{"2330": "semiconductor", "1301": "plastics", "2002": "steel"}
	outcomes := []SignalOutcome{
		industryOutcome("2330", "2026-07-01", source, 0.02),
		industryOutcome("1301", "2026-07-02", source, 0.02),
		industryOutcome("2002", "2026-07-03", source, 0.02),
	}
	report, err := IndustryWinRate(source, outcomes, fakeIndustryResolver(mapping), testCostRate, 1, 0.95)
	if err != nil {
		t.Fatalf("IndustryWinRate: %v", err)
	}
	if len(report.Industries) != 3 {
		t.Fatalf("industries = %d, want 3", len(report.Industries))
	}
	var sum float64
	for _, row := range report.Industries {
		sum += row.CoveragePct
	}
	tolerance := 0.005 * float64(len(report.Industries))
	if math.Abs(sum-report.Coverage.CoveragePct) > tolerance {
		t.Errorf("sum(row coverage_pct) = %v, report coverage_pct = %v (tolerance %v)", sum, report.Coverage.CoveragePct, tolerance)
	}
	if report.Coverage.CoveragePct != 100 {
		t.Errorf("report coverage_pct = %v, want 100 (every symbol mapped)", report.Coverage.CoveragePct)
	}
}
