package calibration

import (
	"math"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// validationTestRecords creates n synthetic calibration records for testing.
// Every 3rd record has positive outflow (Outflow=10), the rest have Outflow=0.
func validationTestRecords(n int) []CalibrationRecord {
	records := make([]CalibrationRecord, n)
	for i := 0; i < n; i++ {
		outflow := 0.0
		if i%3 == 0 { // every 3rd record has outflow
			outflow = 10.0
		}
		records[i] = CalibrationRecord{
			Date: time.Now().AddDate(0, 0, -n+i),
			Snapshot: marketdata.MacroDataSnapshot{
				DXY:                marketdata.MacroDataPoint{ChangePct: 0.1 * float64(i%5)},
				US10Y:              marketdata.MacroDataPoint{Value: 4.0 + float64(i%3)*0.5},
				VIX:                marketdata.MacroDataPoint{Value: 15.0 + float64(i%4)},
				JPY:                marketdata.MacroDataPoint{Value: 145.0 + float64(i%5), ChangePct: 0.05 * float64(i%4)},
				Oil:                marketdata.MacroDataPoint{ChangePct: 0.2 * float64(i%3)},
				Gold:               marketdata.MacroDataPoint{ChangePct: 0.1 * float64(i%3)},
				ForeignInvestorNet: marketdata.MacroDataPoint{Value: -float64(i % 5)},
			},
			Outflow: outflow,
		}
	}
	return records
}

// defaultTestConfig returns a simple config with equal weights (0.125 each) and scale=1.
func defaultTestConfig() StressIndexWeightsConfig {
	return StressIndexWeightsConfig{
		Scaling: StressIndexScaling{
			DXY: 1, US10Y: 1, ForeignFlow: 1, VIX: 1,
			JPY: 1, Geopolitical: 1, Oil: 1, Gold: 1,
		},
		Weights: StressIndexWeights{
			DXY: 0.125, US10Y: 0.125, ForeignFlow: 0.125, VIX: 0.125,
			JPY: 0.125, Geopolitical: 0.125, Oil: 0.125, Gold: 0.125,
		},
		Thresholds: StressIndexThresholds{Crisis: 80, High: 60, Alert: 40},
	}
}

func TestValidateCalibration_SufficientSamples(t *testing.T) {
	t.Parallel()

	records := validationTestRecords(20)
	cfg := defaultTestConfig()

	result := ValidateCalibration(records, cfg, cfg)

	if result.TrainingSize != 16 {
		t.Errorf("TrainingSize = %d, want 16", result.TrainingSize)
	}
	if result.ValidationSize != 4 {
		t.Errorf("ValidationSize = %d, want 4", result.ValidationSize)
	}
	if result.Improvement != 0 {
		t.Errorf("Improvement = %v, want 0 (identical configs)", result.Improvement)
	}
}

func TestValidateCalibration_InsufficientSamples(t *testing.T) {
	t.Parallel()

	records := validationTestRecords(5)
	cfg := defaultTestConfig()

	result := ValidateCalibration(records, cfg, cfg)

	if !result.IsDegradation {
		t.Error("expected IsDegradation=true for insufficient data")
	}
	if result.DegradationMsg == "" {
		t.Error("expected non-empty DegradationMsg for insufficient data")
	}
	if result.ValidationSize != 0 {
		t.Errorf("ValidationSize = %d, want 0", result.ValidationSize)
	}
	if result.TrainingSize != 5 {
		t.Errorf("TrainingSize = %d, want 5", result.TrainingSize)
	}
}

func TestValidateCalibration_NewConfigBetter(t *testing.T) {
	t.Parallel()

	// Construct 30 records where outflow is perfectly correlated with VIX level.
	// Even-indexed records: VIX=40, outflow=10 (stress event).
	// Odd-indexed records: VIX=12, outflow=0 (calm).
	// All other factors are near-zero to isolate the VIX signal.
	records := make([]CalibrationRecord, 30)
	for i := range records {
		vixValue := 12.0
		outflow := 0.0
		if i%2 == 0 {
			vixValue = 40.0
			outflow = 10.0
		}
		records[i] = CalibrationRecord{
			Date: time.Now().AddDate(0, 0, -30+i),
			Snapshot: marketdata.MacroDataSnapshot{
				DXY:                marketdata.MacroDataPoint{ChangePct: 0.001},
				US10Y:              marketdata.MacroDataPoint{Value: 0.1},
				VIX:                marketdata.MacroDataPoint{Value: vixValue},
				JPY:                marketdata.MacroDataPoint{Value: 150.0, ChangePct: 0.001},
				Oil:                marketdata.MacroDataPoint{ChangePct: 0.001},
				Gold:               marketdata.MacroDataPoint{ChangePct: 0.001},
				ForeignInvestorNet: marketdata.MacroDataPoint{Value: -0.1},
			},
			Outflow: outflow,
		}
	}

	// Old config: VIX barely contributes (low scale and weight).
	// All scores will be below 50 → predicted=false for every record.
	// Accuracy = only non-outflow records matched ≈ 50%.
	oldConfig := StressIndexWeightsConfig{
		Scaling: StressIndexScaling{
			DXY: 0.1, US10Y: 0.1, ForeignFlow: 0.1, VIX: 0.1,
			JPY: 0.1, Geopolitical: 0.1, Oil: 0.1, Gold: 0.1,
		},
		Weights: StressIndexWeights{
			DXY: 0.1, US10Y: 0.1, ForeignFlow: 0.1, VIX: 0.05,
			JPY: 0.1, Geopolitical: 0.1, Oil: 0.15, Gold: 0.2,
		},
	}

	// New config: VIX drives the score past 50 for stress records.
	// VIX scale=5, weight=0.5 → VIX=40 contributes min(|40*5|,100)*0.5=50.
	// With other small factors adding ~1-2, outflow records score ~52 (>50), non-outflow ~33 (<50).
	newConfig := StressIndexWeightsConfig{
		Scaling: StressIndexScaling{
			DXY: 0.1, US10Y: 0.1, ForeignFlow: 0.1, VIX: 5.0,
			JPY: 0.1, Geopolitical: 0.1, Oil: 0.1, Gold: 0.1,
		},
		Weights: StressIndexWeights{
			DXY: 0.05, US10Y: 0.05, ForeignFlow: 0.05, VIX: 0.5,
			JPY: 0.05, Geopolitical: 0.05, Oil: 0.1, Gold: 0.1,
		},
	}

	result := ValidateCalibration(records, oldConfig, newConfig)

	if result.NewAccuracy <= result.OldAccuracy {
		t.Errorf("expected NewAccuracy (%.2f) > OldAccuracy (%.2f)", result.NewAccuracy, result.OldAccuracy)
	}
	if result.Improvement <= 0 {
		t.Errorf("expected positive Improvement, got %.4f", result.Improvement)
	}
	if result.IsDegradation {
		t.Error("expected IsDegradation=false when new config is better")
	}
}

func TestValidateCalibration_NewConfigWorse(t *testing.T) {
	t.Parallel()

	// Same VIX-correlated data as NewConfigBetter, but configs are flipped:
	// the "new" config has low VIX sensitivity, missing the outflow pattern.
	records := make([]CalibrationRecord, 30)
	for i := range records {
		vixValue := 12.0
		outflow := 0.0
		if i%2 == 0 {
			vixValue = 40.0
			outflow = 10.0
		}
		records[i] = CalibrationRecord{
			Date: time.Now().AddDate(0, 0, -30+i),
			Snapshot: marketdata.MacroDataSnapshot{
				DXY:                marketdata.MacroDataPoint{ChangePct: 0.001},
				US10Y:              marketdata.MacroDataPoint{Value: 0.1},
				VIX:                marketdata.MacroDataPoint{Value: vixValue},
				JPY:                marketdata.MacroDataPoint{Value: 150.0, ChangePct: 0.001},
				Oil:                marketdata.MacroDataPoint{ChangePct: 0.001},
				Gold:               marketdata.MacroDataPoint{ChangePct: 0.001},
				ForeignInvestorNet: marketdata.MacroDataPoint{Value: -0.1},
			},
			Outflow: outflow,
		}
	}

	// Old config captures the VIX pattern (high scale+weight → score crosses 50).
	oldConfig := StressIndexWeightsConfig{
		Scaling: StressIndexScaling{
			DXY: 0.1, US10Y: 0.1, ForeignFlow: 0.1, VIX: 5.0,
			JPY: 0.1, Geopolitical: 0.1, Oil: 0.1, Gold: 0.1,
		},
		Weights: StressIndexWeights{
			DXY: 0.05, US10Y: 0.05, ForeignFlow: 0.05, VIX: 0.5,
			JPY: 0.05, Geopolitical: 0.05, Oil: 0.1, Gold: 0.1,
		},
	}

	// New config: VIX barely contributes → all scores below 50 → accuracy drops.
	newConfig := StressIndexWeightsConfig{
		Scaling: StressIndexScaling{
			DXY: 0.1, US10Y: 0.1, ForeignFlow: 0.1, VIX: 0.1,
			JPY: 0.1, Geopolitical: 0.1, Oil: 0.1, Gold: 0.1,
		},
		Weights: StressIndexWeights{
			DXY: 0.1, US10Y: 0.1, ForeignFlow: 0.1, VIX: 0.05,
			JPY: 0.1, Geopolitical: 0.1, Oil: 0.15, Gold: 0.2,
		},
	}

	result := ValidateCalibration(records, oldConfig, newConfig)

	if !result.IsDegradation {
		t.Errorf("expected IsDegradation=true when new config is worse (improvement=%.4f)", result.Improvement)
	}
	if result.DegradationMsg == "" {
		t.Error("expected non-empty DegradationMsg when degraded")
	}
	if result.NewAccuracy >= result.OldAccuracy {
		t.Errorf("expected NewAccuracy (%.2f) < OldAccuracy (%.2f)", result.NewAccuracy, result.OldAccuracy)
	}
}

func TestValidateCalibration_IdenticalConfigs(t *testing.T) {
	t.Parallel()

	records := validationTestRecords(20)
	cfg := defaultTestConfig()

	result := ValidateCalibration(records, cfg, cfg)

	// Same config => both should agree on every record
	if result.OldAccuracy != result.NewAccuracy {
		t.Errorf("OldAccuracy=%.4f != NewAccuracy=%.4f for identical configs",
			result.OldAccuracy, result.NewAccuracy)
	}
	if math.Abs(result.Improvement) > 1e-9 {
		t.Errorf("Improvement=%.9f, want ~0 for identical configs", result.Improvement)
	}
	if result.IsDegradation {
		t.Error("expected IsDegradation=false for identical configs")
	}
}

func TestComputeStressFromConfig(t *testing.T) {
	t.Parallel()

	record := CalibrationRecord{
		Snapshot: marketdata.MacroDataSnapshot{
			DXY:                marketdata.MacroDataPoint{ChangePct: 0.1},
			US10Y:              marketdata.MacroDataPoint{Value: 4.0},
			VIX:                marketdata.MacroDataPoint{Value: 20.0},
			JPY:                marketdata.MacroDataPoint{Value: 152.0, ChangePct: 0.05},
			Oil:                marketdata.MacroDataPoint{ChangePct: 0.2},
			Gold:               marketdata.MacroDataPoint{ChangePct: 0.1},
			ForeignInvestorNet: marketdata.MacroDataPoint{Value: -3.0},
		},
		ForeignNet: -3.0,
	}

	config := StressIndexWeightsConfig{
		Scaling: StressIndexScaling{
			DXY: 10, US10Y: 1, ForeignFlow: 1, VIX: 1,
			JPY: 10, Geopolitical: 0, Oil: 10, Gold: 10,
		},
		Weights: StressIndexWeights{
			DXY: 0.2, US10Y: 0.1, ForeignFlow: 0.2, VIX: 0.1,
			JPY: 0.1, Geopolitical: 0, Oil: 0.1, Gold: 0.2,
		},
	}

	// Without baselines: uses absolute factorSignal values.
	//   dxy:    |0.1| * 10 = 1.0       * 0.2 = 0.2
	//   us10y:  |4.0| * 1  = 4.0       * 0.1 = 0.4
	//   fflow:  |-(-3.0)| * 1 = 3.0    * 0.2 = 0.6
	//   vix:    20.0 * 1 = 20.0        * 0.1 = 2.0
	//   jpy:    |152.0| * 10 = 1520 → clamp 100 * 0.1 = 10.0
	//   geo:    0
	//   oil:    |0.2| * 10 = 2.0       * 0.1 = 0.2
	//   gold:   |0.1| * 10 = 1.0       * 0.2 = 0.2
	//   total = 0.2 + 0.4 + 0.6 + 2.0 + 10.0 + 0 + 0.2 + 0.2 = 13.6
	score := computeStressFromConfig(record, config, nil)

	expected := 13.6
	if math.Abs(score-expected) > 0.01 {
		t.Errorf("score = %.4f, want %.2f", score, expected)
	}
}

func TestComputeStressFromConfig_WithBaselines(t *testing.T) {
	t.Parallel()

	records := validationTestRecords(20)
	baselines := ComputeBaselines(records, &BaselineConfig{Window: 60})

	record := records[len(records)-1] // pick last record
	cfg := defaultTestConfig()

	// With baselines, DXY/JPY/Oil/Gold use hybrid signal which may differ
	// from the simple change-based signal.
	scoreWithBaselines := computeStressFromConfig(record, cfg, baselines)
	scoreWithoutBaselines := computeStressFromConfig(record, cfg, nil)

	// The scores may differ — the important thing is no panic/crash.
	// They can be equal if hybrid falls back to change signal.
	_ = scoreWithBaselines
	_ = scoreWithoutBaselines
}

func TestComputeStressFromConfig_VariousConfigs(t *testing.T) {
	t.Parallel()

	record := CalibrationRecord{
		Snapshot: marketdata.MacroDataSnapshot{
			DXY:                marketdata.MacroDataPoint{ChangePct: 0.5},
			US10Y:              marketdata.MacroDataPoint{Value: 4.5},
			VIX:                marketdata.MacroDataPoint{Value: 25.0},
			JPY:                marketdata.MacroDataPoint{Value: 155.0, ChangePct: 0.3},
			Oil:                marketdata.MacroDataPoint{ChangePct: 0.4},
			Gold:               marketdata.MacroDataPoint{ChangePct: 0.2},
			ForeignInvestorNet: marketdata.MacroDataPoint{Value: -5.0},
		},
		ForeignNet: -5.0,
	}

	lowScale := StressIndexWeightsConfig{
		Scaling: StressIndexScaling{
			DXY: 1, US10Y: 1, ForeignFlow: 1, VIX: 1,
			JPY: 1, Geopolitical: 1, Oil: 1, Gold: 1,
		},
		Weights: StressIndexWeights{
			DXY: 0.125, US10Y: 0.125, ForeignFlow: 0.125, VIX: 0.125,
			JPY: 0.125, Geopolitical: 0.125, Oil: 0.125, Gold: 0.125,
		},
	}

	highScale := StressIndexWeightsConfig{
		Scaling: StressIndexScaling{
			DXY: 100, US10Y: 100, ForeignFlow: 100, VIX: 100,
			JPY: 100, Geopolitical: 100, Oil: 100, Gold: 100,
		},
		Weights: StressIndexWeights{
			DXY: 0.125, US10Y: 0.125, ForeignFlow: 0.125, VIX: 0.125,
			JPY: 0.125, Geopolitical: 0.125, Oil: 0.125, Gold: 0.125,
		},
	}

	lowScore := computeStressFromConfig(record, lowScale, nil)
	highScore := computeStressFromConfig(record, highScale, nil)

	// Higher scales must produce a higher score.
	if highScore <= lowScore {
		t.Errorf("highScale score (%.4f) should be > lowScale score (%.4f)", highScore, lowScore)
	}

	// The high-scale score should be capped — components are capped at 100.
	// Verify the score is finite and non-negative.
	if math.IsInf(highScore, 0) || math.IsNaN(highScore) {
		t.Errorf("highScore is invalid: %.4f", highScore)
	}
	if highScore < 0 {
		t.Errorf("highScore should be non-negative: %.4f", highScore)
	}
}
