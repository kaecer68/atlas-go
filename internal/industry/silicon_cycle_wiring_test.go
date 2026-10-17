package industry

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// withParametersFile points the config singleton at a temporary parameters
// JSON and restores the previous path when the test ends.
func withParametersFile(t *testing.T, body string) {
	t.Helper()
	prev := config.GetParametersConfigPath()
	p := filepath.Join(t.TempDir(), "parameters.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("write temp parameters: %v", err)
	}
	config.ResetParametersConfig()
	config.SetParametersConfigPath(p)
	t.Cleanup(func() {
		config.ResetParametersConfig()
		config.SetParametersConfigPath(prev)
	})
	if config.GetParametersConfig() == nil {
		t.Fatal("temp parameters config not loadable")
	}
}

// TestGetSiliconParams_ConsumesConfigFile proves the silicon cycle thresholds
// are read from ParametersConfig.Industry.SiliconCycle (issue #1944 Batch 2,
// Q6 I22): the function used to discard the config (`_ = cfg`) while the
// shipped configs/parameters.json carried a populated block.
func TestGetSiliconParams_ConsumesConfigFile(t *testing.T) {
	withParametersFile(t, `{"industry": {"silicon_cycle": {"value": {
		"revenue_yoy_threshold": 0.42,
		"capex_cut_threshold": 0.33,
		"min_confidence": 0.75,
		"history_window_size": 7
	}}}}`)

	p := getSiliconParams()
	if p.RevenueYoYThreshold != 0.42 {
		t.Errorf("RevenueYoYThreshold = %v, want 0.42 (config not consumed)", p.RevenueYoYThreshold)
	}
	if p.CapexCutThreshold != 0.33 {
		t.Errorf("CapexCutThreshold = %v, want 0.33 (config not consumed)", p.CapexCutThreshold)
	}
	if p.MinConfidence != 0.75 {
		t.Errorf("MinConfidence = %v, want 0.75 (config not consumed)", p.MinConfidence)
	}
	if p.HistoryWindowSize != 7 {
		t.Errorf("HistoryWindowSize = %v, want 7 (config not consumed)", p.HistoryWindowSize)
	}
	// Fields left zero in config keep the code default instead of collapsing to
	// 0 (a 0 threshold would make every transition fire).
	def := defaultSiliconCycleParams()
	if p.SOXExtremeThreshold != def.SOXExtremeThreshold {
		t.Errorf("SOXExtremeThreshold = %v, want default %v", p.SOXExtremeThreshold, def.SOXExtremeThreshold)
	}
	if p.BillingsYoYThreshold != def.BillingsYoYThreshold {
		t.Errorf("BillingsYoYThreshold = %v, want default %v", p.BillingsYoYThreshold, def.BillingsYoYThreshold)
	}
}

// TestGetSiliconParams_AllZeroConfigKeepsDefaults pins the legacy all-zero
// block: the phase machine must not run with zeroed thresholds.
func TestGetSiliconParams_AllZeroConfigKeepsDefaults(t *testing.T) {
	withParametersFile(t, `{"industry": {"silicon_cycle": {"value": {
		"revenue_yoy_threshold": 0, "billings_yoy_threshold": 0,
		"dram_stabilization_threshold": 0, "billings_stabilization_threshold": 0,
		"index_ma_percent_threshold": 0, "sox_extreme_threshold": 0,
		"capex_cut_threshold": 0, "min_confidence": 0, "history_window_size": 0
	}}}}`)

	p := getSiliconParams()
	def := defaultSiliconCycleParams()
	if p.RevenueYoYThreshold != def.RevenueYoYThreshold ||
		p.CapexCutThreshold != def.CapexCutThreshold ||
		p.HistoryWindowSize != def.HistoryWindowSize {
		t.Fatalf("all-zero config must keep defaults, got %+v", p)
	}
}

// TestExtractSiliconIndicators_CapexReachesCutThreshold is the regression for
// the unreachable contraction transitions: the capex proxy is proportional to
// TSMC revenue YoY, so a real revenue decline must produce a signal below
// -CapexCutThreshold and actually move the state machine
// (1→3 / 2→3). The previous hardcoded ±0.05 could never cross the 0.10
// threshold.
func TestExtractSiliconIndicators_CapexReachesCutThreshold(t *testing.T) {
	e := NewSiliconCycleTracker()
	now := time.Now()

	// Realistic expansion snapshot: revenue +25% YoY, SOX +30%, DRAM +5%.
	boom := ExtractSiliconIndicators(marketdata.MacroDataSnapshot{
		TSMCRevenue:   marketdata.MacroDataPoint{ChangePct: 25.0},
		SOXIndex:      marketdata.MacroDataPoint{ChangePct: 30.0},
		DRAMSpotPrice: marketdata.MacroDataPoint{ChangePct: 5.0},
	})
	if got := e.DetectPhase(now, boom); got != PhaseExpansionConfirmed {
		t.Fatalf("expansion snapshot: phase = %s, want %s", got, PhaseExpansionConfirmed)
	}

	// Downturn snapshot: revenue -20% YoY (capex proxy -0.20),
	// SOX -5%, DRAM -5%.
	downturn := ExtractSiliconIndicators(marketdata.MacroDataSnapshot{
		TSMCRevenue:   marketdata.MacroDataPoint{ChangePct: -20.0},
		SOXIndex:      marketdata.MacroDataPoint{ChangePct: -5.0},
		DRAMSpotPrice: marketdata.MacroDataPoint{ChangePct: -5.0},
	})
	if downturn.TSMCCapexGuidance >= -defaultSiliconCycleParams().CapexCutThreshold {
		t.Fatalf("capex signal %.4f does not reach -CapexCutThreshold (%.2f)",
			downturn.TSMCCapexGuidance, defaultSiliconCycleParams().CapexCutThreshold)
	}
	if got := e.DetectPhase(now.Add(time.Hour), downturn); got != PhaseContraction {
		t.Fatalf("1→3 capex transition unreachable: phase = %s, want %s", got, PhaseContraction)
	}
}

// TestExtractSiliconIndicators_PrefersSectorDataCapex proves the capex input
// prefers the real sector_data figure (MacroDataSnapshot.CapexGrowth, written
// by marketdata.SectorDataProvider and previously consumed by nobody) over the
// revenue-derived proxy.
func TestExtractSiliconIndicators_PrefersSectorDataCapex(t *testing.T) {
	ind := ExtractSiliconIndicators(marketdata.MacroDataSnapshot{
		TSMCRevenue: marketdata.MacroDataPoint{ChangePct: 25.0},
		CapexGrowth: marketdata.MacroDataPoint{Symbol: "CAPEX_GROWTH", Value: -12.0},
	})
	if want := -0.12; ind.TSMCCapexGuidance != want {
		t.Fatalf("TSMCCapexGuidance = %v, want %v (sector_data value ignored)", ind.TSMCCapexGuidance, want)
	}
}

// TestSiliconIndicatorProvenance pins what the two mis-declared silicon inputs
// actually are, so a steady phase is never misread as "no signal" (issue #1944
// Batch 2, Q6 I22; corrected by Batch B, 2026-09-27).
//
// Batch 2 asserted SiliconTWIndexProducerAvailable == false, i.e. "no production
// provider writes MacroDataSnapshot.TaiwanSemiIndex". That was false: the
// twse_sector_index channel writes it from a real TWSE provider and runs on a
// 15-minute production task. The 1→2 trigger is still unreachable, for a
// different reason — the written value is a single-day return while the
// threshold means "this far above the moving average" — which is what this test
// now pins, including the executable consequence.
func TestSiliconIndicatorProvenance(t *testing.T) {
	if !SiliconTWIndexWriterWired {
		t.Error("SiliconTWIndexWriterWired flipped to false: the twse_sector_index channel writes MacroDataSnapshot.TaiwanSemiIndex (gateway_adapter.applyTWSESectorIndex) — update the doc block and this test if that channel is really gone")
	}
	if !SiliconTWIndexIsMADeviation {
		t.Error("SiliconTWIndexIsMADeviation flipped back to false: TaiwanSemiconductorIndexMA is computed by SiliconIndexMA from the archived daily index levels, so the 1→2 overheat trigger is genuinely reachable. If this is an intentional rollback (I22-overheat), revert the doc block, ExtractSiliconIndicators and the SiliconIndexMA* tests together")
	}
	if SiliconSOXIndicatorIsYoY {
		t.Error("SiliconSOXIndicatorIsYoY flipped to true: update the doc block and the audit spec")
	}

	// The upstream single-day return is STILL the wire format, but it is now only
	// a declared fallback: with no observation time there is no as-of window, so
	// the legacy passthrough is returned unchanged and the absence is named.
	ind := ExtractSiliconIndicators(marketdata.MacroDataSnapshot{
		TaiwanSemiIndex: marketdata.MacroDataPoint{ChangePct: 99.0},
	})
	if ind.TaiwanSemiconductorIndexMA != 0.99 {
		t.Fatalf("TaiwanSemiconductorIndexMA = %v, want the legacy passthrough 0.99 when no series is available", ind.TaiwanSemiconductorIndexMA)
	}
	if dev := LoadSiliconIndexMADeviation(time.Time{}); dev.Available || dev.Reason != SiliconIndexMAReasonNoAsOf {
		t.Fatalf("fallback reason = %+v, want an unavailable reading with reason %q", dev, SiliconIndexMAReasonNoAsOf)
	}

	// The fix computes the statistic the threshold was written for; it does not
	// lower the threshold to fit a one-day move. Reachability itself (a series
	// >20% above its 60-session mean moving the phase into PhaseOverheat) is
	// pinned by TestSiliconIndexMA_DeviatingSeriesReachesOverheat, and the
	// negative half by TestSiliconIndexMA_FlatSeriesDoesNotReachOverheat.
	if got, want := SiliconIndexMAWindowSessions, 60; got != want {
		t.Fatalf("SiliconIndexMAWindowSessions = %d, want %d (季線)", got, want)
	}

	// Executable consequence, kept from the pre-fix guard: the SOX line still
	// compares a single-day change against a "deviation/period" threshold, so the
	// 1→2 SOX trigger remains unreachable (registered, NOT fixed here).
	p := defaultSiliconCycleParams()
	const dailyIndexMove, dailySOXMove = 0.09, 0.35
	if p.IndexMAPercentThreshold <= dailyIndexMove || p.SOXExtremeThreshold <= dailySOXMove {
		t.Fatalf("premise: thresholds = %v/%v, want both above the single-day moves used here (%v/%v)",
			p.IndexMAPercentThreshold, p.SOXExtremeThreshold, dailyIndexMove, dailySOXMove)
	}

	e := NewSiliconCycleTracker()
	now := time.Now()
	if got := e.DetectPhase(now, SiliconIndicators{
		TSMCMonthlyRevenueYoY: 0.25, GlobalSemiconductorBillingsYoY: 0.30, DRAMSpotPriceTrend: 0.05,
	}); got != PhaseExpansionConfirmed {
		t.Fatalf("premise: phase = %s, want %s after a recovery snapshot", got, PhaseExpansionConfirmed)
	}
	if got := e.DetectPhase(now.Add(time.Hour), SiliconIndicators{
		TSMCMonthlyRevenueYoY:          0.25,
		GlobalSemiconductorBillingsYoY: 0.30,
		DRAMSpotPriceTrend:             0.05,
		TaiwanSemiconductorIndexMA:     dailyIndexMove,
		PhiladelphiaSOXIndexYoY:        dailySOXMove,
	}); got != PhaseExpansionConfirmed {
		t.Fatalf("phase = %s, want %s while both 1→2 triggers stay below their thresholds", got, PhaseExpansionConfirmed)
	}
}

// TestPhaseHistoryWindowZeroDoesNotWipe proves a zero HistoryWindowSize keeps
// the phase history instead of clearing it on every transition.
func TestPhaseHistoryWindowZeroDoesNotWipe(t *testing.T) {
	withParametersFile(t, `{"industry": {"silicon_cycle": {"value": {"history_window_size": 0}}}}`)

	e := NewSiliconCycleTracker()
	now := time.Now()
	e.DetectPhase(now, SiliconIndicators{TSMCMonthlyRevenueYoY: 0.25, GlobalSemiconductorBillingsYoY: 0.30, DRAMSpotPriceTrend: 0.05})
	e.DetectPhase(now.Add(time.Hour), SiliconIndicators{TSMCMonthlyRevenueYoY: -0.20, GlobalSemiconductorBillingsYoY: -0.05, DRAMSpotPriceTrend: -0.05})
	e.DetectPhase(now.Add(2*time.Hour), SiliconIndicators{TSMCMonthlyRevenueYoY: -0.20, GlobalSemiconductorBillingsYoY: 0.02, DRAMSpotPriceTrend: 0.01})
	if got := e.GetTransitionCount(); got == 0 {
		t.Fatal("zero HistoryWindowSize wiped the phase history")
	}
}

// moduleRootForScan walks up from the test working directory until it finds
// go.mod. Production-consumer scans must run against the module root, not
// against a directory that merely looks close to it (issue #1944 Batch A4).
func moduleRootForScan(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("abs working dir: %v", err)
	}
	for range 10 {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("module root not found above %s", dir)
	return ""
}

// TestSiliconPhaseWeightMultiplierHasNoConsumer is the machine-checkable
// contract for the FU-20260930-01 declaration (DECLARED ONLY / NOT ENFORCED on
// GetPhaseWeightMultiplier): the per-phase multipliers must not be presented as
// a live risk control while nothing consumes them.
//
// It couples the declaration flag with the code scan in BOTH directions, so the
// two cannot drift apart:
//
//   - consumer(s) found while SiliconPhaseWeightMultiplierWired is false ⇒ red.
//     Wiring the multiplier is a value change: update the doc comment, the flag,
//     the end-to-end test and FU-20260930-01 in the same commit.
//   - flag true with no consumer ⇒ red. Claiming enforcement that does not exist
//     is the defect this guard exists for.
//
// The scan is deliberately textual and deliberately over-triggering (the N-C1
// precedent in internal/config/parameters_inert_declarations_test.go): any
// non-test production reference counts as a consumer, including one that only
// mentions the name. False-red is the safe direction here.
func TestSiliconPhaseWeightMultiplierHasNoConsumer(t *testing.T) {
	const (
		symbol       = "GetPhaseWeightMultiplier"
		definingFile = "internal/industry/silicon_cycle.go"
	)

	root := moduleRootForScan(t)

	var consumers []string
	for _, sub := range []string{"internal", "cmd"} {
		walkErr := filepath.WalkDir(filepath.Join(root, sub), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				rel = path
			}
			rel = filepath.ToSlash(rel)
			if rel == definingFile {
				return nil // the declaration itself
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if strings.Contains(string(data), symbol) {
				consumers = append(consumers, rel)
			}
			return nil
		})
		if walkErr != nil {
			t.Fatalf("walk %s: %v", sub, walkErr)
		}
	}

	if SiliconPhaseWeightMultiplierWired {
		if len(consumers) == 0 {
			t.Fatalf("%s is declared wired but no non-test production file references %s — wire it, or set the constant back to false together with the DECLARED ONLY comment (FU-20260930-01)",
				"SiliconPhaseWeightMultiplierWired", symbol)
		}
		t.Logf("%s: declared wired, consumers=%v", symbol, consumers)
		return
	}

	if len(consumers) > 0 {
		t.Fatalf("%s now has production consumer(s): %v — it is declared DECLARED ONLY / NOT ENFORCED, so a wiring change must update the doc comment and SiliconPhaseWeightMultiplierWired (silicon_cycle.go), add the end-to-end test it needs, satisfy the re-certification conditions listed there, and close FU-20260930-01 in the same commit",
			symbol, consumers)
	}
	t.Logf("%s: 0 production consumers (test-only), constant=false — declaration holds", symbol)
}
