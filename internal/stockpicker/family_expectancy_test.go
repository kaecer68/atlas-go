package stockpicker

import (
	"fmt"
	"math"
	"testing"
	"time"
)

// ─── family expectancy tests ───────────────────────────────────────────────
//
// The pinned literals in this file are a deliberate FINGERPRINT of the shared
// caliber: if the family instrument (or a future edit) changes the numbers the
// per-symbol / condition / industry views already produced, these tests fail.
// They also pin that the instrument reuses NetHit / WinRate /
// WilsonScoreInterval / CalibrationStatusFor instead of a private copy.

const familyTestCostRate = 0.00585

// familyFixtureSource is a stockpicker source so the condition prefix and the
// avoid-semantics rule are exercised as in production.
const familyFixtureSource = "stockpicker-momentum-20d-positive"

func approxEqual(t *testing.T, label string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %v, want %v (tolerance %v)", label, got, want, tol)
	}
}

// TestNetExpectancyT pins the hand-computed value and the documented
// "not computable" sentinel.
func TestNetExpectancyT(t *testing.T) {
	// [0.01 0.02 0.03]: mean 0.02, sample SD 0.01, SE 0.01/sqrt(3).
	approxEqual(t, "t(0.01,0.02,0.03)", NetExpectancyT([]float64{0.01, 0.02, 0.03}), 0.02/(0.01/math.Sqrt(3)), 1e-12)

	for name, in := range map[string][]float64{
		"nil":             nil,
		"empty":           {},
		"single sample":   {0.05},
		"zero dispersion": {0.01, 0.01, 0.01},
	} {
		if got := NetExpectancyT(in); got != 0 {
			t.Errorf("%s: NetExpectancyT = %v, want the 0 sentinel", name, got)
		}
	}
}

// familyFixtureOutcomes is the shared fixture. The fourth row sits exactly at
// the break-even point (-costRate), which pins NetHit's strict "> 0": it is a
// MISS.
func familyFixtureOutcomes() []SignalOutcome {
	dates := []string{"2026-05-11", "2026-05-12", "2026-05-13", "2026-05-14"}
	returns := []float64{0.02, -0.01, 0.03, -familyTestCostRate}
	out := make([]SignalOutcome, 0, len(dates))
	for i, d := range dates {
		out = append(out, SignalOutcome{
			Symbol:        "2330",
			TriggerDate:   d,
			Source:        familyFixtureSource,
			ForwardReturn: returns[i],
			CostRate:      familyTestCostRate,
		})
	}
	return out
}

// TestFamilyExpectancy_ReusesSharedHelpersWithPinnedValues fingerprints the
// per-symbol / condition / industry calibers and shows the family row is the
// same caliber over the same rows.
func TestFamilyExpectancy_ReusesSharedHelpersWithPinnedValues(t *testing.T) {
	outcomes := familyFixtureOutcomes()

	symbolSummary, err := SignalWinRate(outcomes, familyTestCostRate, 30, 0.95)
	if err != nil {
		t.Fatalf("SignalWinRate: %v", err)
	}
	if symbolSummary.Observations != 4 || symbolSummary.Hits != 2 {
		t.Errorf("per-symbol: observations=%d hits=%d, want 4/2 (break-even row must NOT hit)",
			symbolSummary.Observations, symbolSummary.Hits)
	}
	approxEqual(t, "per-symbol win_rate", symbolSummary.WinRate, 0.5, 1e-12)
	approxEqual(t, "per-symbol wilson_lower", symbolSummary.WilsonLower, 0.15003898915214947, 1e-12)
	approxEqual(t, "per-symbol wilson_upper", symbolSummary.WilsonUpper, 0.8499610108478506, 1e-12)

	condition := ConditionWinRate(familyFixtureSource, outcomes, familyTestCostRate, 30, 0.95)
	approxEqual(t, "condition win_rate", condition.WinRate, 0.5, 1e-12)
	approxEqual(t, "condition wilson_lower", condition.WilsonLower, 0.15003898915214947, 1e-12)

	industryReport, err := IndustryWinRate(familyFixtureSource, outcomes,
		func(symbol string) (string, bool) {
			if symbol == "2330" {
				return "semiconductor", true
			}
			return "", false
		}, familyTestCostRate, 30, 0.95)
	if err != nil {
		t.Fatalf("IndustryWinRate: %v", err)
	}
	if len(industryReport.Industries) != 1 {
		t.Fatalf("industry rows = %d, want 1", len(industryReport.Industries))
	}
	approxEqual(t, "industry win_rate", industryReport.Industries[0].WinRate, 0.5, 1e-12)
	approxEqual(t, "industry avg_net_forward_return", industryReport.Industries[0].AvgNetForwardReturn, 0.0026874999999999994, 1e-12)

	report := FamilyExpectancy(outcomes, familyTestCostRate, 30, 0.95, DefaultTrailingTradingDays, "2026-10-06T00:00:00Z")
	if len(report.Families) != 1 {
		t.Fatalf("families = %d, want 1: %+v", len(report.Families), report.Families)
	}
	row := report.Families[0]
	if row.Source != familyFixtureSource || row.ConditionID != "momentum-20d-positive" || row.Direction != "buy" {
		t.Errorf("identity: source=%q condition=%q direction=%q", row.Source, row.ConditionID, row.Direction)
	}
	if row.N != 4 || row.Hits != 2 {
		t.Errorf("family n=%d hits=%d, want 4/2", row.N, row.Hits)
	}
	approxEqual(t, "family net_hit_rate", row.NetHitRate, symbolSummary.WinRate, 1e-15)
	approxEqual(t, "family wilson_lower", row.WilsonLower, symbolSummary.WilsonLower, 1e-15)
	approxEqual(t, "family wilson_upper", row.WilsonUpper, symbolSummary.WilsonUpper, 1e-15)
	approxEqual(t, "family avg_net_forward_return", row.AvgNetForwardReturn, 0.0026874999999999994, 1e-12)
	approxEqual(t, "family net_expectancy_t", row.NetExpectancyT, 0.27540972849769557, 1e-12)
	if row.CalibrationStatus != string(CalibrationCalibrating) {
		t.Errorf("calibration_status = %q, want calibrating (4 < 30)", row.CalibrationStatus)
	}
	if row.NetCostRate != familyTestCostRate {
		t.Errorf("net_cost_rate = %v, want %v", row.NetCostRate, familyTestCostRate)
	}
	if row.Symbols != 1 || row.DataStart != "2026-05-11" || row.DataEnd != "2026-05-14" {
		t.Errorf("symbols=%d data_start=%q data_end=%q", row.Symbols, row.DataStart, row.DataEnd)
	}
	// Fewer distinct dates than the window length: the window is the whole read.
	if row.TrailingN != 4 || row.TrailingWindowStart != "2026-05-11" || row.TrailingWindowEnd != "2026-05-14" {
		t.Errorf("trailing: n=%d window=%s..%s, want 4 / 2026-05-11..2026-05-14",
			row.TrailingN, row.TrailingWindowStart, row.TrailingWindowEnd)
	}
	approxEqual(t, "trailing avg_net_forward_return", row.TrailingAvgNetForwardReturn, row.AvgNetForwardReturn, 1e-15)
	if report.GeneratedAt != "2026-10-06T00:00:00Z" || row.GeneratedAt != report.GeneratedAt {
		t.Errorf("generated_at: report=%q row=%q", report.GeneratedAt, row.GeneratedAt)
	}
	if report.TotalObservations != 4 || report.InvalidTriggerDates != 0 || report.NonTradingTriggerDates != 0 {
		t.Errorf("counters: %+v", report)
	}
}

// weekdayDates returns n consecutive weekday trigger dates starting at start.
func weekdayDates(start time.Time, n int) []string {
	out := make([]string, 0, n)
	for d := start; len(out) < n; d = d.AddDate(0, 0, 1) {
		if wd := d.Weekday(); wd == time.Saturday || wd == time.Sunday {
			continue
		}
		out = append(out, d.Format("2006-01-02"))
	}
	return out
}

// TestFamilyExpectancy_TrailingWindowUsesLatestDistinctDates: the trailing
// window is the most recent 60 distinct trigger dates of the READ, so a family
// that stopped firing reports trailing_n = 0 while the window itself is still
// reported.
func TestFamilyExpectancy_TrailingWindowUsesLatestDistinctDates(t *testing.T) {
	dates := weekdayDates(time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC), 70)
	if len(dates) != 70 {
		t.Fatalf("fixture dates = %d, want 70", len(dates))
	}

	outcomes := make([]SignalOutcome, 0, len(dates)+5)
	for _, d := range dates {
		outcomes = append(outcomes, SignalOutcome{
			Symbol: "2330", TriggerDate: d, Source: "stockpicker-alive", ForwardReturn: 0.01, CostRate: familyTestCostRate,
		})
	}
	for _, d := range dates[:5] {
		outcomes = append(outcomes, SignalOutcome{
			Symbol: "2330", TriggerDate: d, Source: "stockpicker-dead", ForwardReturn: 0.01, CostRate: familyTestCostRate,
		})
	}

	report := FamilyExpectancy(outcomes, familyTestCostRate, 30, 0.95, DefaultTrailingTradingDays, "2026-10-06T00:00:00Z")
	if report.TrailingWindowStart != dates[10] || report.TrailingWindowEnd != dates[69] {
		t.Fatalf("window = %s..%s, want %s..%s", report.TrailingWindowStart, report.TrailingWindowEnd, dates[10], dates[69])
	}
	if len(report.Families) != 2 || report.Families[0].Source != "stockpicker-alive" {
		t.Fatalf("families = %+v, want [alive dead] in source order", report.Families)
	}
	alive, dead := report.Families[0], report.Families[1]
	if alive.N != 70 || dead.N != 5 {
		t.Errorf("full-period n: alive=%d dead=%d, want 70/5", alive.N, dead.N)
	}
	if alive.TrailingN != 60 {
		t.Errorf("alive trailing_n = %d, want 60 (70 dates - 10 oldest)", alive.TrailingN)
	}
	if dead.TrailingN != 0 || dead.TrailingAvgNetForwardReturn != 0 || dead.TrailingNetExpectancyT != 0 {
		t.Errorf("dead family must report an empty trailing window, got %+v", dead)
	}
	if dead.TrailingWindowStart != dates[10] || dead.TrailingWindowEnd != dates[69] {
		t.Errorf("dead family window = %s..%s, want the read window (so 'no recent evidence' is readable)",
			dead.TrailingWindowStart, dead.TrailingWindowEnd)
	}
	if report.NonTradingTriggerDates != 0 || report.InvalidTriggerDates != 0 {
		t.Errorf("counters: %+v", report)
	}
}

// TestFamilyExpectancy_TrailingWindowDedupesDates: the window counts distinct
// DATES, so many rows sharing one date are one trading day.
func TestFamilyExpectancy_TrailingWindowDedupesDates(t *testing.T) {
	outcomes := []SignalOutcome{
		{Symbol: "2330", TriggerDate: "2026-05-11", Source: "s1", ForwardReturn: 0.01, CostRate: familyTestCostRate},
		{Symbol: "2454", TriggerDate: "2026-05-11", Source: "s1", ForwardReturn: 0.02, CostRate: familyTestCostRate},
		{Symbol: "1301", TriggerDate: "2026-05-11", Source: "s1", ForwardReturn: 0.03, CostRate: familyTestCostRate},
		{Symbol: "2330", TriggerDate: "2026-05-12", Source: "s1", ForwardReturn: 0.04, CostRate: familyTestCostRate},
	}
	report := FamilyExpectancy(outcomes, familyTestCostRate, 30, 0.95, 1, "2026-10-06T00:00:00Z")
	row := report.Families[0]
	if row.TrailingWindowStart != "2026-05-12" || row.TrailingWindowEnd != "2026-05-12" {
		t.Fatalf("window = %s..%s, want only the latest date", row.TrailingWindowStart, row.TrailingWindowEnd)
	}
	if row.TrailingN != 1 {
		t.Errorf("trailing_n = %d, want 1 (one row on 2026-05-12)", row.TrailingN)
	}
	if row.N != 4 {
		t.Errorf("full-period n = %d, want 4", row.N)
	}
}

// TestFamilyExpectancy_EmptyRegimeBecomesUnknown: the empty regime column maps
// to "unknown" — never imputed, never merged into a tagged stratum.
func TestFamilyExpectancy_EmptyRegimeBecomesUnknown(t *testing.T) {
	outcomes := []SignalOutcome{
		{Symbol: "2330", TriggerDate: "2026-05-11", Source: "s1", ForwardReturn: 0.01, CostRate: familyTestCostRate},
		{Symbol: "2454", TriggerDate: "2026-05-12", Source: "s1", ForwardReturn: 0.02, CostRate: familyTestCostRate},
		{Symbol: "1301", TriggerDate: "2026-05-13", Source: "s1", ForwardReturn: -0.02, CostRate: familyTestCostRate, Regime: "RISK_ON"},
	}
	report := FamilyExpectancy(outcomes, familyTestCostRate, 30, 0.95, 60, "2026-10-06T00:00:00Z")
	row := report.Families[0]
	if len(row.ByRegime) != 2 {
		t.Fatalf("strata = %+v, want unknown + RISK_ON", row.ByRegime)
	}
	if row.ByRegime[0].Regime != UnknownRegime || row.ByRegime[0].N != 2 {
		t.Errorf("first stratum = %+v, want unknown with n=2 (most evidence first)", row.ByRegime[0])
	}
	if row.ByRegime[1].Regime != "RISK_ON" || row.ByRegime[1].N != 1 {
		t.Errorf("second stratum = %+v, want RISK_ON with n=1", row.ByRegime[1])
	}
	// The strata partition the row: no imputation, nothing double counted.
	total := 0
	for _, s := range row.ByRegime {
		total += s.N
	}
	if total != row.N {
		t.Errorf("strata n sum = %d, want the family n = %d", total, row.N)
	}
	// A stratum carries the same caliber as the row.
	approxEqual(t, "unknown stratum net_hit_rate", row.ByRegime[0].NetHitRate, WinRate(2, 2), 1e-15)
	lower, upper := WilsonScoreInterval(2, 2, 0.95)
	approxEqual(t, "unknown stratum wilson_lower", row.ByRegime[0].WilsonLower, lower, 1e-15)
	approxEqual(t, "unknown stratum wilson_upper", row.ByRegime[0].WilsonUpper, upper, 1e-15)

	// A family whose rows are all tagged has no unknown stratum.
	tagged := FamilyExpectancy(outcomes[2:], familyTestCostRate, 30, 0.95, 60, "2026-10-06T00:00:00Z")
	if len(tagged.Families[0].ByRegime) != 1 || tagged.Families[0].ByRegime[0].Regime != "RISK_ON" {
		t.Errorf("fully tagged family strata = %+v, want only RISK_ON", tagged.Families[0].ByRegime)
	}
}

// TestFamilyExpectancy_NoOutcomesDoesNotPanic: zero samples must answer with
// zeros, not a divide-by-zero.
func TestFamilyExpectancy_NoOutcomesDoesNotPanic(t *testing.T) {
	for name, in := range map[string][]SignalOutcome{"nil": nil, "empty": {}} {
		report := FamilyExpectancy(in, familyTestCostRate, 30, 0.95, DefaultTrailingTradingDays, "2026-10-06T00:00:00Z")
		if len(report.Families) != 0 {
			t.Errorf("%s: families = %+v, want none", name, report.Families)
		}
		if report.TotalObservations != 0 || report.InvalidTriggerDates != 0 || report.NonTradingTriggerDates != 0 {
			t.Errorf("%s: counters = %+v, want zeros", name, report)
		}
	}
	// An empty window must not produce a dangling window boundary.
	report := FamilyExpectancy(nil, familyTestCostRate, 30, 0.95, 0, "2026-10-06T00:00:00Z")
	if report.TrailingWindowStart != "" || report.TrailingWindowEnd != "" {
		t.Errorf("empty window boundaries = %q..%q", report.TrailingWindowStart, report.TrailingWindowEnd)
	}
}

// TestFamilyExpectancy_InvalidTriggerDatesReported: an unparsable trigger_date
// is counted and excluded from date-based computations — never silently
// dropped, never guessed into a date.
func TestFamilyExpectancy_InvalidTriggerDatesReported(t *testing.T) {
	outcomes := []SignalOutcome{
		{Symbol: "2330", TriggerDate: "2026-05-11", Source: "s1", ForwardReturn: 0.01, CostRate: familyTestCostRate},
		{Symbol: "2454", TriggerDate: "2026-5-12", Source: "s1", ForwardReturn: 0.02, CostRate: familyTestCostRate}, // not ISO-padded
		{Symbol: "1301", TriggerDate: "", Source: "s1", ForwardReturn: 0.03, CostRate: familyTestCostRate},
	}
	report := FamilyExpectancy(outcomes, familyTestCostRate, 30, 0.95, 60, "2026-10-06T00:00:00Z")
	if report.InvalidTriggerDates != 2 {
		t.Errorf("invalid_trigger_dates = %d, want 2", report.InvalidTriggerDates)
	}
	row := report.Families[0]
	if row.N != 3 {
		t.Errorf("n = %d, want 3 (invalid dates are still observations)", row.N)
	}
	if row.DataStart != "2026-05-11" || row.DataEnd != "2026-05-11" {
		t.Errorf("data range = %q..%q, want the only valid date", row.DataStart, row.DataEnd)
	}
	if row.TrailingN != 1 {
		t.Errorf("trailing_n = %d, want 1 (only the valid date is in the window)", row.TrailingN)
	}
	if report.TotalObservations != 3 {
		t.Errorf("total_observations = %d, want 3", report.TotalObservations)
	}
}

// TestFamilyExpectancy_NonTradingDatesReported: weekend trigger dates are
// counted (no exchange-holiday calendar is consulted) and stay in the sample.
func TestFamilyExpectancy_NonTradingDatesReported(t *testing.T) {
	outcomes := []SignalOutcome{
		{Symbol: "2330", TriggerDate: "2026-05-16", Source: "s1", ForwardReturn: 0.01, CostRate: familyTestCostRate}, // Saturday
		{Symbol: "2330", TriggerDate: "2026-05-18", Source: "s1", ForwardReturn: 0.02, CostRate: familyTestCostRate}, // Monday
	}
	report := FamilyExpectancy(outcomes, familyTestCostRate, 30, 0.95, 60, "2026-10-06T00:00:00Z")
	if report.NonTradingTriggerDates != 1 {
		t.Errorf("non_trading_trigger_dates = %d, want 1 (2026-05-16 is a Saturday)", report.NonTradingTriggerDates)
	}
	if report.InvalidTriggerDates != 0 {
		t.Errorf("invalid_trigger_dates = %d, want 0 (the format is valid)", report.InvalidTriggerDates)
	}
	if report.Families[0].N != 2 {
		t.Errorf("n = %d, want 2 (a weekend date is reported, not dropped)", report.Families[0].N)
	}
}

// TestFamilyExpectancy_NonPositiveTrailingDaysDisablesWindow: the handler
// rejects a bad value, but the pure function must still behave predictably.
func TestFamilyExpectancy_NonPositiveTrailingDaysDisablesWindow(t *testing.T) {
	for _, days := range []int{0, -1} {
		report := FamilyExpectancy(familyFixtureOutcomes(), familyTestCostRate, 30, 0.95, days, "2026-10-06T00:00:00Z")
		row := report.Families[0]
		if row.TrailingN != 0 || row.TrailingAvgNetForwardReturn != 0 || row.TrailingNetExpectancyT != 0 {
			t.Errorf("trailing_days=%d: trailing = %+v, want an empty window", days, row)
		}
		if report.TrailingWindowStart != "" || report.TrailingWindowEnd != "" {
			t.Errorf("trailing_days=%d: window = %q..%q, want empty", days, report.TrailingWindowStart, report.TrailingWindowEnd)
		}
		if row.N != 4 {
			t.Errorf("trailing_days=%d: n = %d, want the full period untouched", days, row.N)
		}
	}
}

// TestFamilyExpectancy_UnattributedObservationsCounted: a row with no source
// cannot be attributed to a family; it is counted, never pooled.
func TestFamilyExpectancy_UnattributedObservationsCounted(t *testing.T) {
	outcomes := []SignalOutcome{
		{Symbol: "2330", TriggerDate: "2026-05-11", Source: "", ForwardReturn: 0.01, CostRate: familyTestCostRate},
		{Symbol: "2330", TriggerDate: "2026-05-11", Source: "s1", ForwardReturn: 0.02, CostRate: familyTestCostRate},
	}
	report := FamilyExpectancy(outcomes, familyTestCostRate, 30, 0.95, 60, "2026-10-06T00:00:00Z")
	if report.UnattributedObservations != 1 {
		t.Errorf("unattributed_observations = %d, want 1", report.UnattributedObservations)
	}
	if len(report.Families) != 1 || report.Families[0].N != 1 {
		t.Errorf("families = %+v, want one row with n=1", report.Families)
	}
}

// TestFamilyExpectancy_DeterministicOrder: family rows sort by source, strata
// by sample count then name.
func TestFamilyExpectancy_DeterministicOrder(t *testing.T) {
	outcomes := []SignalOutcome{
		{Symbol: "2330", TriggerDate: "2026-05-11", Source: "stockpicker-zed", ForwardReturn: 0.01, CostRate: familyTestCostRate, Regime: "RISK_ON"},
		{Symbol: "2330", TriggerDate: "2026-05-12", Source: "stockpicker-zed", ForwardReturn: 0.01, CostRate: familyTestCostRate},
		{Symbol: "2330", TriggerDate: "2026-05-13", Source: "stockpicker-zed", ForwardReturn: 0.01, CostRate: familyTestCostRate},
		{Symbol: "2330", TriggerDate: "2026-05-13", Source: "stockpicker-zed", ForwardReturn: 0.01, CostRate: familyTestCostRate, Regime: "RISK_OFF"},
		{Symbol: "2330", TriggerDate: "2026-05-11", Source: "stockpicker-abc", ForwardReturn: 0.01, CostRate: familyTestCostRate},
	}
	report := FamilyExpectancy(outcomes, familyTestCostRate, 30, 0.95, 60, "2026-10-06T00:00:00Z")
	if len(report.Families) != 2 || report.Families[0].Source != "stockpicker-abc" || report.Families[1].Source != "stockpicker-zed" {
		t.Fatalf("family order = %+v, want abc then zed", report.Families)
	}
	strata := report.Families[1].ByRegime
	if len(strata) != 3 {
		t.Fatalf("strata = %+v, want RISK_OFF, RISK_ON, unknown", strata)
	}
	if strata[0].Regime != "unknown" || strata[0].N != 2 {
		t.Errorf("strata[0] = %+v, want unknown n=2", strata[0])
	}
	if strata[1].Regime != "RISK_OFF" || strata[2].Regime != "RISK_ON" {
		t.Errorf("tie order = %q,%q, want RISK_OFF then RISK_ON", strata[1].Regime, strata[2].Regime)
	}
}

// TestTrailingTriggerDates covers the window helper directly.
func TestTrailingTriggerDates(t *testing.T) {
	all := []string{"2026-05-01", "2026-05-04", "2026-05-05", "2026-05-06"}
	got := TrailingTriggerDates(all, 2)
	want := []string{"2026-05-05", "2026-05-06"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("TrailingTriggerDates(4 dates, 2) = %v, want %v", got, want)
	}
	if got := TrailingTriggerDates(all, 10); len(got) != 4 || got[0] != "2026-05-01" {
		t.Errorf("days > len(dates) = %v, want all 4 dates ascending", got)
	}
	if got := TrailingTriggerDates(nil, 5); len(got) != 0 {
		t.Errorf("nil dates = %v, want empty", got)
	}
	if got := TrailingTriggerDates(all, 0); len(got) != 0 {
		t.Errorf("days=0 = %v, want empty", got)
	}
	// The input slice must not be reordered in place.
	if all[0] != "2026-05-01" {
		t.Errorf("TrailingTriggerDates mutated its input: %v", all)
	}
}
