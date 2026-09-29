package industry

// silicon_index_ma_test.go — I22-overheat. Pins that
// SiliconIndicators.TaiwanSemiconductorIndexMA is a deviation above the
// quarterly moving average (not the upstream single-day return), that the 1→2
// overheat trigger is therefore reachable, that a flat series does NOT trigger
// it, that an absent/short series falls back to the legacy value instead of
// pretending to be zero, and that the kill switch restores the old behaviour.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/taiwanholidays"
)

const testTAISEMISymbol = "TAISEMI"

// writeMacroDays writes one dated macro snapshot per day, with the given level.
// Dates that are not trading days are skipped unless forceWeekend is true.
func writeMacroDays(t *testing.T, dir string, days []time.Time, level func(i int, day time.Time) float64, forceWeekend bool) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	for i, day := range days {
		if !forceWeekend && !taiwanholidays.IsTradingDay(day) {
			continue
		}
		payload := map[string]any{
			"taiwan_semi_index": map[string]any{
				"symbol":     testTAISEMISymbol,
				"value":      level(i, day),
				"change_pct": 0.5,
				"timestamp":  day.Unix(),
			},
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal fixture: %v", err)
		}
		name := filepath.Join(dir, day.In(taipeiLocation()).Format("2006-01-02")+".json")
		if err := os.WriteFile(name, raw, 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
	}
}

// trailingTradingDays returns n trading days ending at (and including) end.
func trailingTradingDays(n int, end time.Time) []time.Time {
	days := make([]time.Time, 0, n)
	for day := end; len(days) < n; day = day.AddDate(0, 0, -1) {
		if taiwanholidays.IsTradingDay(day) {
			days = append(days, day)
		}
	}
	// ascending order, which is how the loader sorts anyway
	for i, j := 0, len(days)-1; i < j; i, j = i+1, j-1 {
		days[i], days[j] = days[j], days[i]
	}
	return days
}

// useSeriesDir points the reader at a fixture directory for the duration of a test.
func useSeriesDir(t *testing.T, dir string) {
	t.Helper()
	previous := siliconIndexMASeriesDirOverride
	siliconIndexMASeriesDirOverride = dir
	resetSiliconIndexMACache()
	t.Cleanup(func() {
		siliconIndexMASeriesDirOverride = previous
		resetSiliconIndexMACache()
	})
}

// indexSnapshot builds the macro snapshot the extractor consumes, with the
// observation time the loader uses as its as-of.
func indexSnapshot(asOf time.Time, dailyReturnPct float64) marketdata.MacroDataSnapshot {
	return marketdata.MacroDataSnapshot{
		TaiwanSemiIndex: marketdata.MacroDataPoint{
			Symbol:    testTAISEMISymbol,
			Value:     1000,
			ChangePct: dailyReturnPct,
			Timestamp: asOf.Unix(),
		},
	}
}

func TestComputeSiliconIndexMADeviation(t *testing.T) {
	flat := make([]float64, SiliconIndexMAWindowSessions)
	for i := range flat {
		flat[i] = 100
	}
	if dev := ComputeSiliconIndexMADeviation(flat, SiliconIndexMAWindowSessions); !dev.Available || dev.Value != 0 {
		t.Fatalf("flat series: got available=%v value=%v, want available=true value=0", dev.Available, dev.Value)
	}

	rising := append([]float64{}, flat...)
	rising[len(rising)-1] = 150
	dev := ComputeSiliconIndexMADeviation(rising, SiliconIndexMAWindowSessions)
	// mean of the last 60 = (59*100 + 150)/60; deviation = 150/mean - 1
	wantMean := (59*100.0 + 150) / 60
	if !dev.Available || dev.Sessions != SiliconIndexMAWindowSessions {
		t.Fatalf("rising series: got %+v, want an available 60-session reading", dev)
	}
	if got, want := dev.Value, 150/wantMean-1; diffFloat(got, want) > 1e-12 {
		t.Errorf("deviation = %v, want %v", got, want)
	}

	short := ComputeSiliconIndexMADeviation([]float64{1, 2, 3}, SiliconIndexMAWindowSessions)
	if short.Available || short.Reason != SiliconIndexMAReasonInsufficient || short.Sessions != 3 {
		t.Errorf("short series: got %+v, want unavailable with reason %q and sessions=3", short, SiliconIndexMAReasonInsufficient)
	}

	zeros := make([]float64, SiliconIndexMAWindowSessions)
	if dev := ComputeSiliconIndexMADeviation(zeros, SiliconIndexMAWindowSessions); dev.Available {
		t.Errorf("zero mean: got available=true, want unavailable (ratio is undefined)")
	}
}

func diffFloat(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}

// TestSiliconIndexMA_DeviatingSeriesReachesOverheat is the reachability proof:
// a series that ends >20% above its 60-session mean must move the phase machine
// from ExpansionConfirmed into PhaseOverheat.
func TestSiliconIndexMA_DeviatingSeriesReachesOverheat(t *testing.T) {
	asOf := time.Date(2026, 9, 30, 6, 0, 0, 0, taipeiLocation())
	days := trailingTradingDays(SiliconIndexMAWindowSessions+10, asOf)
	dir := t.TempDir()
	writeMacroDays(t, dir, days, func(i int, _ time.Time) float64 {
		if i == len(days)-1 {
			return 1300 // last session 30% above a ~1000 baseline
		}
		return 1000
	}, false)
	useSeriesDir(t, dir)

	ind := ExtractSiliconIndicators(indexSnapshot(asOf, 0.5))
	if ind.TaiwanSemiconductorIndexMA <= defaultSiliconCycleParams().IndexMAPercentThreshold {
		t.Fatalf("index MA deviation = %v, want > threshold %v (the series is ~30%% above its mean)",
			ind.TaiwanSemiconductorIndexMA, defaultSiliconCycleParams().IndexMAPercentThreshold)
	}

	tracker := NewSiliconCycleTracker()
	now := time.Now()
	if got := tracker.DetectPhase(now, SiliconIndicators{
		TSMCMonthlyRevenueYoY: 0.25, GlobalSemiconductorBillingsYoY: 0.30, DRAMSpotPriceTrend: 0.05,
	}); got != PhaseExpansionConfirmed {
		t.Fatalf("premise: phase = %s, want %s", got, PhaseExpansionConfirmed)
	}
	if got := tracker.DetectPhase(now.Add(time.Hour), ind); got != PhaseOverheat {
		t.Fatalf("phase = %s, want %s: the 1→2 trigger must fire when the index is far above its MA", got, PhaseOverheat)
	}
}

// TestSiliconIndexMA_FlatSeriesDoesNotReachOverheat is the negative half.
func TestSiliconIndexMA_FlatSeriesDoesNotReachOverheat(t *testing.T) {
	asOf := time.Date(2026, 9, 30, 6, 0, 0, 0, taipeiLocation())
	days := trailingTradingDays(SiliconIndexMAWindowSessions+10, asOf)
	dir := t.TempDir()
	writeMacroDays(t, dir, days, func(int, time.Time) float64 { return 1000 }, false)
	useSeriesDir(t, dir)

	ind := ExtractSiliconIndicators(indexSnapshot(asOf, 0.5))
	if ind.TaiwanSemiconductorIndexMA != 0 {
		t.Fatalf("flat series: deviation = %v, want 0", ind.TaiwanSemiconductorIndexMA)
	}
	tracker := NewSiliconCycleTracker()
	now := time.Now()
	tracker.DetectPhase(now, SiliconIndicators{TSMCMonthlyRevenueYoY: 0.25, GlobalSemiconductorBillingsYoY: 0.30, DRAMSpotPriceTrend: 0.05})
	if got := tracker.DetectPhase(now.Add(time.Hour), ind); got != PhaseExpansionConfirmed {
		t.Errorf("phase = %s, want %s: a flat series must not trigger overheat", got, PhaseExpansionConfirmed)
	}
}

// TestSiliconIndexMA_KillSwitchRestoresLegacy pins the rollback path: with the
// switch off the extractor's value is the upstream single-day return again.
func TestSiliconIndexMA_KillSwitchRestoresLegacy(t *testing.T) {
	asOf := time.Date(2026, 9, 30, 6, 0, 0, 0, taipeiLocation())
	days := trailingTradingDays(SiliconIndexMAWindowSessions+10, asOf)
	dir := t.TempDir()
	writeMacroDays(t, dir, days, func(i int, _ time.Time) float64 {
		if i == len(days)-1 {
			return 1300
		}
		return 1000
	}, false)
	useSeriesDir(t, dir)

	dev := LoadSiliconIndexMADeviation(asOf)
	if !dev.Available {
		t.Fatalf("fixture premise: series not available: %+v", dev)
	}
	const legacy = 0.0099
	if got := siliconIndexMAForIndicator(legacy, dev, false); got != legacy {
		t.Errorf("kill switch off: got %v, want the legacy value %v", got, legacy)
	}
	if got := siliconIndexMAForIndicator(legacy, dev, true); got != dev.Value {
		t.Errorf("kill switch on: got %v, want the computed deviation %v", got, dev.Value)
	}
}

// TestSiliconIndexMA_AbsentSeriesFallsBackAndReportsWhy pins the honest-absence
// rule: no series means the legacy value plus a machine-readable reason, never a
// silent zero.
func TestSiliconIndexMA_AbsentSeriesFallsBackAndReportsWhy(t *testing.T) {
	asOf := time.Date(2026, 9, 30, 6, 0, 0, 0, taipeiLocation())

	// (a) no observation time at all
	useSeriesDir(t, t.TempDir())
	if dev := LoadSiliconIndexMADeviation(time.Time{}); dev.Available || dev.Reason != SiliconIndexMAReasonNoAsOf {
		t.Errorf("no as-of: got %+v, want unavailable with reason %q", dev, SiliconIndexMAReasonNoAsOf)
	}
	if got := SiliconIndexMA(marketdata.MacroDataPoint{Symbol: testTAISEMISymbol, ChangePct: 1.25}); got != 0.0125 {
		t.Errorf("no as-of: value = %v, want the legacy passthrough 0.0125", got)
	}

	// (b) archive present but too shallow for the window
	shallow := t.TempDir()
	writeMacroDays(t, shallow, trailingTradingDays(10, asOf), func(int, time.Time) float64 { return 1000 }, false)
	useSeriesDir(t, shallow)
	dev := LoadSiliconIndexMADeviation(asOf)
	if dev.Available || dev.Reason != SiliconIndexMAReasonInsufficient || dev.Sessions != 10 {
		t.Errorf("shallow archive: got %+v, want unavailable/insufficient with sessions=10", dev)
	}
	if got := SiliconIndexMA(indexSnapshot(asOf, -0.88).TaiwanSemiIndex); got != -0.0088 {
		t.Errorf("shallow archive: value = %v, want the legacy passthrough -0.0088", got)
	}
}

// TestSiliconIndexMA_TradingDayFilterDropsCarryForwardFiles proves weekends and
// holidays do not enter the average: the archive writes one file per calendar
// day, so a naive average would spread 60 sessions over 60 calendar days.
func TestSiliconIndexMA_TradingDayFilterDropsCarryForwardFiles(t *testing.T) {
	asOf := time.Date(2026, 9, 30, 6, 0, 0, 0, taipeiLocation())
	tradingDays := trailingTradingDays(SiliconIndexMAWindowSessions, asOf)

	// The same trading days, plus weekend files carrying an absurd level: if any
	// of them entered the window the mean — and therefore the deviation — would
	// change. Every calendar day from the first session to the as-of is written,
	// so the archive looks exactly like production (one file per UTC day).
	var allDays []time.Time
	for day := tradingDays[0]; !day.After(asOf); day = day.AddDate(0, 0, 1) {
		allDays = append(allDays, day)
	}
	lastTradingDay := tradingDays[len(tradingDays)-1]

	withWeekends := t.TempDir()
	writeMacroDays(t, withWeekends, allDays, func(_ int, day time.Time) float64 {
		switch {
		case !taiwanholidays.IsTradingDay(day):
			return 999999 // carry-forward files must never reach the average
		case day.Equal(lastTradingDay):
			return 1200
		default:
			return 1000
		}
	}, true)
	useSeriesDir(t, withWeekends)

	dev := LoadSiliconIndexMADeviation(asOf)
	if !dev.Available {
		t.Fatalf("expected an available reading: %+v", dev)
	}
	wantMean := (float64(SiliconIndexMAWindowSessions-1)*1000 + 1200) / float64(SiliconIndexMAWindowSessions)
	if want := 1200/wantMean - 1; diffFloat(dev.Value, want) > 1e-12 {
		t.Errorf("deviation = %v, want %v (trading-session average only)", dev.Value, want)
	}
	if dev.Sessions != SiliconIndexMAWindowSessions {
		t.Errorf("sessions = %d, want %d", dev.Sessions, SiliconIndexMAWindowSessions)
	}
}

// TestSiliconIndexMA_ReadingIsMemoisedPerDay pins the cache: the dashboard calls
// the extractor on every request, so the archive must be read once per session.
func TestSiliconIndexMA_ReadingIsMemoisedPerDay(t *testing.T) {
	asOf := time.Date(2026, 9, 30, 6, 0, 0, 0, taipeiLocation())
	days := trailingTradingDays(SiliconIndexMAWindowSessions, asOf)
	dir := t.TempDir()
	writeMacroDays(t, dir, days, func(i int, _ time.Time) float64 {
		if i == len(days)-1 {
			return 1200
		}
		return 1000
	}, false)
	useSeriesDir(t, dir)

	first := LoadSiliconIndexMADeviation(asOf)
	if !first.Available {
		t.Fatalf("expected an available reading: %+v", first)
	}
	// Rewrite the newest file; a cached read must be unaffected within the day.
	name := filepath.Join(dir, days[len(days)-1].In(taipeiLocation()).Format("2006-01-02")+".json")
	raw, _ := json.Marshal(map[string]any{"taiwan_semi_index": map[string]any{
		"symbol": testTAISEMISymbol, "value": float64(5000), "change_pct": 0.0, "timestamp": days[len(days)-1].Unix(),
	}})
	if err := os.WriteFile(name, raw, 0o600); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if second := LoadSiliconIndexMADeviation(asOf); second.Value != first.Value {
		t.Errorf("second read = %v, want the memoised %v", second.Value, first.Value)
	}
}

// TestSiliconIndexMA_ConsumptionPathObservable shows the downstream effect the
// phase flip has: the silicon layer contribution flips sign.
func TestSiliconIndexMA_ConsumptionPathObservable(t *testing.T) {
	expansionContribution := buildAdj("silicon", GetPhaseScore(PhaseExpansionConfirmed), 0.25, "").Contribution
	overheatContribution := buildAdj("silicon", GetPhaseScore(PhaseOverheat), 0.25, "").Contribution
	// (rawValue-0.5)*weight: expansion 1.0 -> +0.125, overheat 0.40 -> -0.025
	if diffFloat(expansionContribution, 0.125) > 1e-9 {
		t.Errorf("expansion contribution = %v, want 0.125", expansionContribution)
	}
	if diffFloat(overheatContribution, -0.025) > 1e-9 {
		t.Errorf("overheat contribution = %v, want -0.025", overheatContribution)
	}
	if expansionContribution <= 0 || overheatContribution >= 0 {
		t.Errorf("premise: the phase flip must move the silicon layer contribution across zero (got %v -> %v)",
			expansionContribution, overheatContribution)
	}
	if GetPhaseWeightMultiplier(PhaseOverheat) != 0.90 {
		t.Errorf("GetPhaseWeightMultiplier(overheat) = %v, want 0.90", GetPhaseWeightMultiplier(PhaseOverheat))
	}
	_ = fmt.Sprintf // keep fmt imported if the assertions above are trimmed
}
