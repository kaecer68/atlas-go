package industry

import (
	"os"
	"path/filepath"
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

// TestSiliconIndicatorProvenance pins the two indicator inputs that have no
// production producer at the period their names declare, so the state machine
// cannot be read as "no signal" (issue #1944 Batch 2, Q6 I22).
func TestSiliconIndicatorProvenance(t *testing.T) {
	if SiliconTWIndexProducerAvailable {
		t.Error("SiliconTWIndexProducerAvailable flipped to true: update the doc block and the audit spec")
	}
	if SiliconSOXIndicatorIsYoY {
		t.Error("SiliconSOXIndicatorIsYoY flipped to true: update the doc block and the audit spec")
	}
	// TaiwanSemiconductorIndexMA is a passthrough of a field no provider
	// populates, so it stays 0 even for an extreme input: the 1→2 overheat
	// branch that reads it is unreachable today.
	ind := ExtractSiliconIndicators(marketdata.MacroDataSnapshot{
		TaiwanSemiIndex: marketdata.MacroDataPoint{ChangePct: 99.0},
	})
	if ind.TaiwanSemiconductorIndexMA != 0.99 {
		t.Fatalf("TaiwanSemiconductorIndexMA = %v, want a 1:1 passthrough of ChangePct", ind.TaiwanSemiconductorIndexMA)
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
