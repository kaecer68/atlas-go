package industry

import (
	"math"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

func testCalibrationConfig() config.CycleCalibrationConfig {
	return config.CycleCalibrationConfig{
		MinSamples:     10,
		LearningRate:   0.05,
		HitRateHigh:    0.55,
		HitRateLow:     0.45,
		WeightClampMin: 0.05,
		WeightClampMax: 0.40,
		WindowSize:     30,
	}
}

func TestNewCycleCalibration(t *testing.T) {
	cal := NewCycleCalibration(testCalibrationConfig())
	if cal == nil {
		t.Fatal("expected non-nil calibration")
	}
	if cal.GetOutcomeCount() != 0 {
		t.Errorf("expected 0 outcomes, got %d", cal.GetOutcomeCount())
	}
}

func TestRecordOutcome_AccuracyTracking(t *testing.T) {
	cal := NewCycleCalibration(testCalibrationConfig())
	baseSignals := map[string]float64{
		"silicon":        0.75,
		"business_cycle": 0.60,
		"seasonal":       1.10,
		"events":         0.95,
		"supply_chain":   0.05,
	}

	for range 10 {
		cal.RecordOutcome("sess-01", time.Now(), baseSignals, 0.01)
	}

	metrics := cal.GetMetrics()

	if metrics["silicon"].TotalSignals != 10 {
		t.Errorf("silicon signals: expected 10, got %d", metrics["silicon"].TotalSignals)
	}
	if metrics["silicon"].CorrectSignals != 10 {
		t.Errorf("silicon correct: expected 10, got %d — 0.75 signal is bullish and return +0.01 is bullish",
			metrics["silicon"].CorrectSignals)
	}

	if metrics["events"].CorrectSignals != 0 {
		t.Errorf("events correct: expected 0, got %d — 0.95 signal (below 1.0) is bearish but return +0.01 is bullish",
			metrics["events"].CorrectSignals)
	}

	if metrics["supply_chain"].CorrectSignals != 10 {
		t.Errorf("supply_chain correct: expected 10, got %d — 0.05 > 0 is bullish",
			metrics["supply_chain"].CorrectSignals)
	}
}

func TestRecordOutcome_MixedDirections(t *testing.T) {
	cal := NewCycleCalibration(testCalibrationConfig())

	bullishSignals := map[string]float64{"silicon": 0.75, "business_cycle": 0.60, "seasonal": 1.10, "events": 1.05, "supply_chain": 0.05}
	bearishSignals := map[string]float64{"silicon": 0.25, "business_cycle": 0.50, "seasonal": 0.95, "events": 0.90, "supply_chain": -0.05}

	for range 5 {
		cal.RecordOutcome("sess-bull", time.Now(), bullishSignals, 0.02)
	}
	for range 5 {
		cal.RecordOutcome("sess-bear", time.Now(), bearishSignals, -0.02)
	}

	metrics := cal.GetMetrics()

	if metrics["silicon"].TotalSignals != 10 {
		t.Errorf("silicon total: expected 10, got %d", metrics["silicon"].TotalSignals)
	}
	if metrics["silicon"].CorrectSignals != 10 {
		t.Errorf("silicon correct: expected 10, got %d", metrics["silicon"].CorrectSignals)
	}

	if metrics["business_cycle"].CorrectSignals != 10 {
		t.Errorf("business_cycle correct: expected 10, got %d — 0.50 is not > 0.5 (bearish), matches 5 bearish returns; 0.60 > 0.5 (bullish), matches 5 bullish returns",
			metrics["business_cycle"].CorrectSignals)
	}

	if metrics["supply_chain"].CorrectSignals != 10 {
		t.Errorf("supply_chain correct: expected 10, got %d", metrics["supply_chain"].CorrectSignals)
	}
}

func TestRecordOutcome_NeutralReturn(t *testing.T) {
	cal := NewCycleCalibration(testCalibrationConfig())
	signals := map[string]float64{"silicon": 0.75}

	cal.RecordOutcome("sess-zero", time.Now(), signals, 0.0)

	metrics := cal.GetMetrics()
	if metrics["silicon"].CorrectSignals != 0 {
		t.Errorf("expected 0 correct for zero return, got %d", metrics["silicon"].CorrectSignals)
	}
}

func TestCalibrateWeights_InsufficientSamples(t *testing.T) {
	cal := NewCycleCalibration(testCalibrationConfig())

	baseWeights := map[string]float64{"silicon": 0.25, "business_cycle": 0.20, "seasonal": 0.15}

	for range 5 {
		cal.RecordOutcome("sess", time.Now(), map[string]float64{
			"silicon":        0.75,
			"business_cycle": 0.60,
			"seasonal":       1.10,
		}, 0.01)
	}

	calibrated := cal.CalibrateWeights(baseWeights)

	if calibrated["silicon"] != baseWeights["silicon"] {
		t.Errorf("expected unchanged weight (insufficient samples), got %v vs %v",
			calibrated["silicon"], baseWeights["silicon"])
	}
}

func TestCalibrateWeights_UpweightHighAccuracy(t *testing.T) {
	cal := NewCycleCalibration(testCalibrationConfig())

	baseWeights := map[string]float64{"silicon": 0.25, "business_cycle": 0.20, "seasonal": 0.15}

	for range 15 {
		cal.RecordOutcome("sess", time.Now(), map[string]float64{
			"silicon":        0.75,
			"business_cycle": 0.60,
			"seasonal":       1.10,
		}, 0.01)
	}

	calibrated := cal.CalibrateWeights(baseWeights)

	metrics := cal.GetMetrics()
	if metrics["silicon"].Accuracy != 1.0 {
		t.Fatalf("expected silicon accuracy 1.0, got %.2f", metrics["silicon"].Accuracy)
	}

	expectedSilicon := (baseWeights["silicon"] + 0.05) / ((baseWeights["silicon"] + 0.05) + (baseWeights["business_cycle"] + 0.05) + (baseWeights["seasonal"] + 0.05))
	if math.Abs(calibrated["silicon"]-expectedSilicon) > 1e-3 {
		t.Errorf("expected silicon weight %.4f (upweighted +0.05 then normalized), got %.4f",
			expectedSilicon, calibrated["silicon"])
	}
}

func TestCalibrateWeights_DownweightLowAccuracy(t *testing.T) {
	cal := NewCycleCalibration(testCalibrationConfig())

	baseWeights := map[string]float64{"silicon": 0.25, "business_cycle": 0.20, "seasonal": 0.15}

	highAccuracySignals := map[string]float64{"silicon": 0.75, "business_cycle": 0.25, "seasonal": 1.10}

	for range 15 {
		cal.RecordOutcome("sess", time.Now(), highAccuracySignals, 0.01)
	}

	calibrated := cal.CalibrateWeights(baseWeights)

	rawSilicon := baseWeights["silicon"] + 0.05
	rawBusiness := baseWeights["business_cycle"] - 0.05
	rawSeasonal := baseWeights["seasonal"] + 0.05
	total := rawSilicon + rawBusiness + rawSeasonal
	expectedBusiness := rawBusiness / total
	if math.Abs(calibrated["business_cycle"]-expectedBusiness) > 1e-3 {
		t.Errorf("expected business_cycle weight %.4f (downweighted -0.05 then normalized), got %.4f",
			expectedBusiness, calibrated["business_cycle"])
	}
}

func TestCalibrateWeights_WeightClamping(t *testing.T) {
	cal := NewCycleCalibration(testCalibrationConfig())

	baseWeights := map[string]float64{"silicon": 0.03, "business_cycle": 0.45}

	for range 15 {
		cal.RecordOutcome("sess", time.Now(), map[string]float64{
			"silicon":        0.75,
			"business_cycle": 0.25,
		}, 0.01)
	}

	calibrated := cal.CalibrateWeights(baseWeights)

	var sum float64
	for _, w := range calibrated {
		sum += w
	}
	sumRounded := math.Round(sum*100) / 100
	if sumRounded != 1.0 {
		t.Errorf("weights sum to %.4f, expected 1.0", sum)
	}

	if len(calibrated) != 2 {
		t.Errorf("expected 2 layers, got %d", len(calibrated))
	}
}

func TestCalibrateWeights_NormalizesToOne(t *testing.T) {
	cal := NewCycleCalibration(testCalibrationConfig())

	baseWeights := map[string]float64{"silicon": 0.3, "business_cycle": 0.3, "seasonal": 0.2}

	for range 15 {
		cal.RecordOutcome("sess", time.Now(), map[string]float64{
			"silicon":        0.75,
			"business_cycle": 0.25,
			"seasonal":       1.10,
		}, 0.01)
	}

	calibrated := cal.CalibrateWeights(baseWeights)

	var sum float64
	for _, w := range calibrated {
		sum += w
	}
	sumRounded := math.Round(sum*100) / 100

	if sumRounded != 1.0 {
		t.Errorf("weights sum to %.4f, expected 1.0; weights: %v", sum, calibrated)
	}
}

func TestRecordOutcome_RollingWindowEviction(t *testing.T) {
	cfg := testCalibrationConfig()
	cfg.WindowSize = 5
	cfg.MinSamples = 3
	cal := NewCycleCalibration(cfg)

	for range 10 {
		cal.RecordOutcome("sess", time.Now(), map[string]float64{"silicon": 0.75}, 0.01)
	}

	if cal.GetOutcomeCount() != 5 {
		t.Errorf("expected 5 outcomes in window, got %d", cal.GetOutcomeCount())
	}

	metrics := cal.GetMetrics()
	if metrics["silicon"].TotalSignals != 5 {
		t.Errorf("expected 5 total signals (from latest 5 outcomes), got %d", metrics["silicon"].TotalSignals)
	}
}

func TestLayerSignalMatchesReturn(t *testing.T) {
	tests := []struct {
		layer        string
		signal       float64
		actualReturn float64
		want         bool
	}{
		{"silicon", 0.75, 0.01, true},
		{"silicon", 0.25, 0.01, false},
		{"silicon", 0.75, -0.01, false},
		{"silicon", 0.25, -0.01, true},
		{"business_cycle", 0.60, 0.01, true},
		{"business_cycle", 0.50, 0.01, false},
		{"business_cycle", 0.50, -0.01, true},
		{"seasonal", 1.10, 0.01, true},
		{"seasonal", 1.00, 0.01, false},
		{"seasonal", 0.95, 0.01, false},
		{"seasonal", 0.95, -0.01, true},
		{"events", 1.05, 0.01, true},
		{"events", 1.00, -0.01, true},
		{"events", 0.90, 0.01, false},
		{"supply_chain", 0.05, 0.01, true},
		{"supply_chain", -0.05, 0.01, false},
		{"supply_chain", 0.05, -0.01, false},
		{"supply_chain", -0.05, -0.01, true},
		{"unknown_layer", 0.75, 0.01, true},
		{"unknown_layer", 0.25, 0.01, false},
	}

	for _, tt := range tests {
		got := layerSignalMatchesReturn(tt.layer, tt.signal, tt.actualReturn)
		if got != tt.want {
			t.Errorf("layerSignalMatchesReturn(%s, %.2f, %.2f) = %v, want %v",
				tt.layer, tt.signal, tt.actualReturn, got, tt.want)
		}
	}
}

func TestGetMetrics_EmptyCalibration(t *testing.T) {
	cal := NewCycleCalibration(testCalibrationConfig())
	metrics := cal.GetMetrics()
	if len(metrics) != 0 {
		t.Errorf("expected empty metrics, got %d entries", len(metrics))
	}
}

func TestSetConfig_PropagatesChanges(t *testing.T) {
	cal := NewCycleCalibration(testCalibrationConfig())

	cfg := testCalibrationConfig()
	cfg.LearningRate = 0.10
	cal.SetConfig(cfg)

	baseWeights := map[string]float64{"silicon": 0.25}
	for range 15 {
		cal.RecordOutcome("sess", time.Now(), map[string]float64{"silicon": 0.75}, 0.01)
	}

	calibrated := cal.CalibrateWeights(baseWeights)
	if math.Abs(calibrated["silicon"]-1.0) > 1e-6 {
		t.Errorf("expected silicon weight 1.0000 (only layer, normalized), got %.4f",
			calibrated["silicon"])
	}
}
