package calibration

import (
	"math"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// syntheticRegimeRecord creates a CalibrationRecord with specified VIX and foreign flow.
func syntheticRegimeRecord(vix, foreignNet float64) CalibrationRecord {
	return CalibrationRecord{
		Date:       time.Now(),
		ForeignNet: foreignNet,
		Outflow:    -foreignNet,
		Snapshot: marketdata.MacroDataSnapshot{
			VIX:                marketdata.MacroDataPoint{Value: vix},
			US10Y:              marketdata.MacroDataPoint{Value: 4.5},
			DXY:                marketdata.MacroDataPoint{ChangePct: 0.5},
			ForeignInvestorNet: marketdata.MacroDataPoint{Value: foreignNet},
			JPY:                marketdata.MacroDataPoint{ChangePct: -0.3},
			Oil:                marketdata.MacroDataPoint{ChangePct: 1.0},
			Gold:               marketdata.MacroDataPoint{ChangePct: 0.5},
		},
	}
}

func TestClassifyRegime_Bull(t *testing.T) {
	t.Parallel()
	if r := ClassifyRegime(12); r != RegimeBull {
		t.Fatalf("VIX=12: expected bull, got %s", r)
	}
}

func TestClassifyRegime_Normal(t *testing.T) {
	t.Parallel()
	if r := ClassifyRegime(20); r != RegimeNormal {
		t.Fatalf("VIX=20: expected normal, got %s", r)
	}
}

func TestClassifyRegime_Bear(t *testing.T) {
	t.Parallel()
	if r := ClassifyRegime(30); r != RegimeBear {
		t.Fatalf("VIX=30: expected bear, got %s", r)
	}
}

func TestClassifyRegime_Crisis(t *testing.T) {
	t.Parallel()
	if r := ClassifyRegime(40); r != RegimeCrisis {
		t.Fatalf("VIX=40: expected crisis, got %s", r)
	}
}

func TestClassifyRegime_Boundaries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		vix  float64
		want MarketRegime
	}{
		{0, RegimeBull},
		{14.9, RegimeBull},
		{15, RegimeNormal},
		{24.9, RegimeNormal},
		{25, RegimeBear},
		{34.9, RegimeBear},
		{35, RegimeCrisis},
		{100, RegimeCrisis},
	}
	for _, tt := range tests {
		if r := ClassifyRegime(tt.vix); r != tt.want {
			t.Errorf("VIX=%.1f: expected %s, got %s", tt.vix, tt.want, r)
		}
	}
}

func TestDefaultRegimeConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultRegimeConfig()
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	// All four regimes should use compile-time defaults
	for _, name := range []string{"bull", "normal", "bear", "crisis"} {
		var w StressIndexWeights
		switch name {
		case "bull":
			w = cfg.Bull.Weights
		case "normal":
			w = cfg.Normal.Weights
		case "bear":
			w = cfg.Bear.Weights
		case "crisis":
			w = cfg.Crisis.Weights
		}
		if w.DXY != StressWeightDXY || w.VIX != StressWeightVIX || w.ForeignFlow != StressWeightForeignFlow {
			t.Fatalf("%s: expected compile-time default weights, got DXY=%.3f VIX=%.3f Flow=%.3f",
				name, w.DXY, w.VIX, w.ForeignFlow)
		}
	}
}

func TestBoostWeights(t *testing.T) {
	t.Parallel()
	input := StressIndexWeights{
		DXY: 0.125, US10Y: 0.125, ForeignFlow: 0.125, VIX: 0.125,
		JPY: 0.125, Geopolitical: 0.125, Oil: 0.125, Gold: 0.125,
	}

	// Boost VIX + Geopolitical by 1.5x
	boosted := boostWeights(input, map[string]float64{"vix": 1.5, "geopolitical": 1.5})

	// VIX and Geo should be higher than before (0.125 * 1.5 = 0.1875 pre-normalize)
	// After normalization, they should be > 0.125 (the unboosted equal weight)
	if boosted.VIX <= 0.125 {
		t.Fatalf("expected VIX > 0.125 after boost, got %.4f", boosted.VIX)
	}
	if boosted.Geopolitical <= 0.125 {
		t.Fatalf("expected Geopolitical > 0.125 after boost, got %.4f", boosted.Geopolitical)
	}

	// Sum should be ~1.0
	sum := boosted.DXY + boosted.US10Y + boosted.ForeignFlow + boosted.VIX +
		boosted.JPY + boosted.Geopolitical + boosted.Oil + boosted.Gold
	if math.Abs(sum-1.0) > 0.001 {
		t.Fatalf("expected weights sum ~1.0, got %.6f", sum)
	}
}

func TestBoostWeights_NoBoostIsIdentity(t *testing.T) {
	t.Parallel()
	input := StressIndexWeights{
		DXY: 0.13, US10Y: 0.18, ForeignFlow: 0.22, VIX: 0.13,
		JPY: 0.08, Geopolitical: 0.13, Oil: 0.07, Gold: 0.06,
	}
	result := boostWeights(input, nil)
	sum := result.DXY + result.US10Y + result.ForeignFlow + result.VIX +
		result.JPY + result.Geopolitical + result.Oil + result.Gold
	if math.Abs(sum-1.0) > 0.001 {
		t.Fatalf("no-boost should preserve sum ~1.0, got %.6f", sum)
	}
}

func TestSelectConfig(t *testing.T) {
	t.Parallel()
	cfg := &RegimeCalibratedConfig{
		Bull:   StressIndexWeightsConfig{Weights: StressIndexWeights{DXY: 0.20}},
		Normal: StressIndexWeightsConfig{Weights: StressIndexWeights{DXY: 0.13}},
		Bear:   StressIndexWeightsConfig{Weights: StressIndexWeights{DXY: 0.10}},
		Crisis: StressIndexWeightsConfig{Weights: StressIndexWeights{DXY: 0.125}},
	}

	tests := []struct {
		regime MarketRegime
		want   float64
	}{
		{RegimeBull, 0.20},
		{RegimeNormal, 0.13},
		{RegimeBear, 0.10},
		{RegimeCrisis, 0.125},
	}
	for _, tt := range tests {
		got := cfg.SelectConfig(tt.regime)
		if got.Weights.DXY != tt.want {
			t.Errorf("%s: expected DXY=%.2f, got %.4f", tt.regime, tt.want, got.Weights.DXY)
		}
	}
}

func TestSelectConfig_UnknownRegime(t *testing.T) {
	t.Parallel()
	cfg := &RegimeCalibratedConfig{
		Normal: StressIndexWeightsConfig{Weights: StressIndexWeights{DXY: 0.13}},
	}
	got := cfg.SelectConfig(MarketRegime("unknown"))
	if got.Weights.DXY != 0.13 {
		t.Fatalf("unknown regime should fall back to Normal, got DXY=%.4f", got.Weights.DXY)
	}
}

func TestSelectConfig_NilReceiver(t *testing.T) {
	t.Parallel()
	var cfg *RegimeCalibratedConfig
	got := cfg.SelectConfig(RegimeBull)
	// Should return defaults
	if got.Weights.DXY != StressWeightDXY {
		t.Fatalf("nil receiver should return defaults, got DXY=%.4f", got.Weights.DXY)
	}
}

func TestCalibrateWeightsByRegime(t *testing.T) {
	t.Parallel()
	cal := NewRegimeAwareCalibrator()

	// Create records spanning all regimes with enough per-regime for calibration
	var records []CalibrationRecord
	// Bull (VIX=12): 5 records
	for i := range 5 {
		r := syntheticRegimeRecord(12, -float64(i+1))
		r.OutflowTarget = r.Outflow
		records = append(records, r)
	}
	// Normal (VIX=20): 5 records
	for i := range 5 {
		r := syntheticRegimeRecord(20, -float64(i+1))
		r.OutflowTarget = r.Outflow
		records = append(records, r)
	}
	// Bear (VIX=30): 5 records
	for i := range 5 {
		r := syntheticRegimeRecord(30, -float64(i+1))
		r.OutflowTarget = r.Outflow
		records = append(records, r)
	}
	// Crisis (VIX=40): 5 records
	for i := range 5 {
		r := syntheticRegimeRecord(40, -float64(i+1))
		r.OutflowTarget = r.Outflow
		records = append(records, r)
	}

	cfg := cal.CalibrateWeightsByRegime(records)
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	// Crisis should have equal weights (0.125 each)
	crisisW := cfg.Crisis.Weights
	if math.Abs(crisisW.DXY-0.125) > 0.001 {
		t.Fatalf("crisis DXY should be 0.125, got %.4f", crisisW.DXY)
	}

	bullW := cfg.Bull.Weights
	bearW := cfg.Bear.Weights

	sum := bullW.DXY + bullW.US10Y + bullW.ForeignFlow + bullW.VIX + bullW.JPY + bullW.Geopolitical + bullW.Oil + bullW.Gold
	if math.Abs(sum-1.0) > 0.01 {
		t.Fatalf("bull weights sum=%.4f, expected ~1.0", sum)
	}
	sum = bearW.DXY + bearW.US10Y + bearW.ForeignFlow + bearW.VIX + bearW.JPY + bearW.Geopolitical + bearW.Oil + bearW.Gold
	if math.Abs(sum-1.0) > 0.01 {
		t.Fatalf("bear weights sum=%.4f, expected ~1.0", sum)
	}

	for _, name := range []string{"bull", "normal", "bear", "crisis"} {
		var w StressIndexWeights
		switch name {
		case "bull":
			w = cfg.Bull.Weights
		case "normal":
			w = cfg.Normal.Weights
		case "bear":
			w = cfg.Bear.Weights
		case "crisis":
			w = cfg.Crisis.Weights
		}
		sum := w.DXY + w.US10Y + w.ForeignFlow + w.VIX + w.JPY + w.Geopolitical + w.Oil + w.Gold
		if math.Abs(sum-1.0) > 0.01 {
			t.Fatalf("%s weights sum=%.4f, expected ~1.0", name, sum)
		}
	}
}

func TestCalibrateWeightsByRegime_InsufficientRecords(t *testing.T) {
	t.Parallel()
	cal := NewRegimeAwareCalibrator()

	// Only 2 bull records — insufficient for calibration (< 3)
	records := []CalibrationRecord{
		syntheticRegimeRecord(12, -1),
		syntheticRegimeRecord(12, -2),
	}

	cfg := cal.CalibrateWeightsByRegime(records)
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	// Bull should fall back to defaults (insufficient records)
	if cfg.Bull.Weights.DXY != StressWeightDXY {
		t.Fatalf("insufficient bull records should use defaults, got DXY=%.4f", cfg.Bull.Weights.DXY)
	}
}

func TestCalibrateWeightsByRegime_EmptyRecords(t *testing.T) {
	t.Parallel()
	cal := NewRegimeAwareCalibrator()
	cfg := cal.CalibrateWeightsByRegime(nil)
	if cfg == nil {
		t.Fatal("expected non-nil config for empty records")
	}
	// All regimes should be defaults
	if cfg.Bull.Weights.DXY != StressWeightDXY {
		t.Fatalf("empty records should produce defaults, got DXY=%.4f", cfg.Bull.Weights.DXY)
	}
}

func TestEqualWeights(t *testing.T) {
	t.Parallel()
	w := equalWeights()
	sum := w.DXY + w.US10Y + w.ForeignFlow + w.VIX + w.JPY + w.Geopolitical + w.Oil + w.Gold
	if math.Abs(sum-1.0) > 0.001 {
		t.Fatalf("equal weights should sum to 1.0, got %.6f", sum)
	}
	if w.DXY != 0.125 {
		t.Fatalf("expected 0.125, got %.4f", w.DXY)
	}
}

func TestNewRegimeAwareCalibrator(t *testing.T) {
	t.Parallel()
	c := NewRegimeAwareCalibrator()
	if c == nil {
		t.Fatal("expected non-nil calibrator")
	}
	if c.TargetMedianContribution != 20.0 {
		t.Fatalf("expected target 20.0, got %f", c.TargetMedianContribution)
	}
}

func TestFilterByRegime(t *testing.T) {
	t.Parallel()
	records := []CalibrationRecord{
		syntheticRegimeRecord(10, -1), // bull
		syntheticRegimeRecord(20, -2), // normal
		syntheticRegimeRecord(12, -3), // bull
		syntheticRegimeRecord(30, -4), // bear
		syntheticRegimeRecord(40, -5), // crisis
	}

	bullRecords := filterByRegime(records, RegimeBull)
	if len(bullRecords) != 2 {
		t.Fatalf("expected 2 bull records, got %d", len(bullRecords))
	}

	crisisRecords := filterByRegime(records, RegimeCrisis)
	if len(crisisRecords) != 1 {
		t.Fatalf("expected 1 crisis record, got %d", len(crisisRecords))
	}
}
