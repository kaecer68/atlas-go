package calibration

import (
	"math"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// syntheticScalingRecord creates a CalibrationRecord with specified macro values.
// DXY, JPY, Oil, Gold use ChangePct; US10Y, VIX, ForeignInvestorNet use Value.
func syntheticScalingRecord(dxyChange, us10y, vix, jpyChange, foreignNet, oilChange, goldChange float64) CalibrationRecord {
	snap := marketdata.MacroDataSnapshot{
		DXY:                marketdata.MacroDataPoint{ChangePct: dxyChange},
		US10Y:              marketdata.MacroDataPoint{Value: us10y},
		VIX:                marketdata.MacroDataPoint{Value: vix},
		JPY:                marketdata.MacroDataPoint{ChangePct: jpyChange},
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: foreignNet},
		Oil:                marketdata.MacroDataPoint{ChangePct: oilChange},
		Gold:               marketdata.MacroDataPoint{ChangePct: goldChange},
	}
	return CalibrationRecord{
		Date:       time.Now(),
		Snapshot:   snap,
		ForeignNet: foreignNet,
	}
}

func TestScaleCalibrator_New(t *testing.T) {
	t.Parallel()
	c := NewScaleCalibrator()
	if c == nil {
		t.Fatal("expected non-nil calibrator")
	}
	if c.TargetMedianContribution != 20.0 {
		t.Fatalf("expected target 20.0, got %f", c.TargetMedianContribution)
	}
}

func TestScaleCalibrator_WithTarget(t *testing.T) {
	t.Parallel()
	c := NewScaleCalibrator().WithTarget(10.0)
	if c.TargetMedianContribution != 10.0 {
		t.Fatalf("expected target 10.0, got %f", c.TargetMedianContribution)
	}
}

func TestScaleCalibrator_EmptyRecords(t *testing.T) {
	t.Parallel()
	c := NewScaleCalibrator()
	scaling := c.CalibrateScales(nil)

	for _, factor := range []struct {
		name  string
		value float64
	}{
		{"dxy", scaling.DXY},
		{"us10y", scaling.US10Y},
		{"foreign_flow", scaling.ForeignFlow},
		{"vix", scaling.VIX},
		{"jpy", scaling.JPY},
		{"geopolitical", scaling.Geopolitical},
		{"oil", scaling.Oil},
		{"gold", scaling.Gold},
	} {
		if factor.value != 1.0 {
			t.Errorf("expected %s scale 1.0 for empty records, got %f", factor.name, factor.value)
		}
	}
}

func TestScaleCalibrator_DXYScale(t *testing.T) {
	t.Parallel()
	// DXY change percentages: [0.1, 0.2, 0.3, 0.4, 0.5]
	// Median abs = 0.3
	// Scale = 20.0 / 0.3 = 66.67
	records := []CalibrationRecord{
		syntheticScalingRecord(0.1, 4.0, 15, 0.5, -5, 1.0, 0.5),
		syntheticScalingRecord(0.2, 4.0, 15, 0.5, -5, 1.0, 0.5),
		syntheticScalingRecord(0.3, 4.0, 15, 0.5, -5, 1.0, 0.5),
		syntheticScalingRecord(0.4, 4.0, 15, 0.5, -5, 1.0, 0.5),
		syntheticScalingRecord(0.5, 4.0, 15, 0.5, -5, 1.0, 0.5),
	}

	c := NewScaleCalibrator()
	scaling := c.CalibrateScales(records)

	expected := 20.0 / 0.3
	tolerance := 0.01
	if math.Abs(scaling.DXY-expected) > tolerance {
		t.Errorf("expected DXY scale ~%.2f, got %.4f", expected, scaling.DXY)
	}
}

func TestScaleCalibrator_AllFactors(t *testing.T) {
	t.Parallel()
	// Create records with reasonable signal magnitudes for all factors.
	records := make([]CalibrationRecord, 20)
	for i := range records {
		records[i] = syntheticScalingRecord(
			0.1+float64(i%5)*0.05, // DXY: 0.1-0.3
			3.5+float64(i%4)*0.5,  // US10Y: 3.5-5.0
			12+float64(i%6)*2,     // VIX: 12-22
			0.2+float64(i%4)*0.1,  // JPY: 0.2-0.5
			-3-float64(i%5),       // ForeignNet: -3 to -7
			0.5+float64(i%5)*0.3,  // Oil: 0.5-1.7
			0.3+float64(i%4)*0.2,  // Gold: 0.3-0.9
		)
	}

	c := NewScaleCalibrator()
	scaling := c.CalibrateScales(records)

	for _, factor := range []struct {
		name  string
		value float64
	}{
		{"dxy", scaling.DXY},
		{"us10y", scaling.US10Y},
		{"foreign_flow", scaling.ForeignFlow},
		{"vix", scaling.VIX},
		{"jpy", scaling.JPY},
		{"geopolitical", scaling.Geopolitical},
		{"oil", scaling.Oil},
		{"gold", scaling.Gold},
	} {
		if factor.value < 0.1 || factor.value > 100.0 {
			t.Errorf("expected %s scale in [0.1, 100], got %f", factor.name, factor.value)
		}
	}
}

func TestScaleCalibrator_MedianComputation(t *testing.T) {
	t.Parallel()
	// Known median from small set: signals [1, 2, 3, 4, 5] => median=3
	// Target = 20 => scale = 20/3 ≈ 6.67
	// Use VIX since it takes Value directly (not ChangePct)
	records := []CalibrationRecord{
		syntheticScalingRecord(0, 0, 1, 0, 0, 0, 0),
		syntheticScalingRecord(0, 0, 2, 0, 0, 0, 0),
		syntheticScalingRecord(0, 0, 3, 0, 0, 0, 0),
		syntheticScalingRecord(0, 0, 4, 0, 0, 0, 0),
		syntheticScalingRecord(0, 0, 5, 0, 0, 0, 0),
	}

	c := NewScaleCalibrator()
	scaling := c.CalibrateScales(records)

	expected := 20.0 / 3.0
	tolerance := 0.01
	if math.Abs(scaling.VIX-expected) > tolerance {
		t.Errorf("expected VIX scale ~%.4f, got %.4f", expected, scaling.VIX)
	}
}

func TestSignalForScaling_CollectsSignals(t *testing.T) {
	t.Parallel()
	// Create 10 records with non-zero VIX values
	records := make([]CalibrationRecord, 10)
	for i := range records {
		records[i] = syntheticScalingRecord(0.1, 4.0, float64(10+i), 0, 0, 0, 0)
	}

	signals := signalForScaling("vix", records)
	if len(signals) != 10 {
		t.Errorf("expected 10 VIX signals, got %d", len(signals))
	}

	// DXY should also produce 10 signals (all 0.1 change)
	signals = signalForScaling("dxy", records)
	if len(signals) != 10 {
		t.Errorf("expected 10 DXY signals, got %d", len(signals))
	}

	// Records with all-zero signals should produce empty slice
	zeroRecords := make([]CalibrationRecord, 5)
	for i := range zeroRecords {
		zeroRecords[i] = syntheticScalingRecord(0, 0, 0, 0, 0, 0, 0)
	}
	signals = signalForScaling("vix", zeroRecords)
	if len(signals) != 0 {
		t.Errorf("expected 0 VIX signals for zero records, got %d", len(signals))
	}
}

func TestScaleCalibrator_Clamping(t *testing.T) {
	t.Parallel()
	// Test lower bound: tiny signals => huge scale => clamped to 100
	// DXY changes of 0.001 => median 0.001 => scale = 20/0.001 = 20000 => clamped to 100
	tinyRecords := make([]CalibrationRecord, 5)
	for i := range tinyRecords {
		tinyRecords[i] = syntheticScalingRecord(0.001, 4.0, 15, 0, 0, 0, 0)
	}
	c := NewScaleCalibrator()
	scaling := c.CalibrateScales(tinyRecords)
	if scaling.DXY != 100.0 {
		t.Errorf("expected DXY clamped to 100.0, got %f", scaling.DXY)
	}

	// Test upper bound: huge signals => tiny scale => clamped to 0.1
	// VIX values of 1000 => scale = 20/1000 = 0.02 => clamped to 0.1
	hugeRecords := make([]CalibrationRecord, 5)
	for i := range hugeRecords {
		hugeRecords[i] = syntheticScalingRecord(0, 0, 1000, 0, 0, 0, 0)
	}
	scaling = c.CalibrateScales(hugeRecords)
	if scaling.VIX != 0.1 {
		t.Errorf("expected VIX clamped to 0.1, got %f", scaling.VIX)
	}
}

func TestScaleCalibrator_GeopoliticalZeroRecords(t *testing.T) {
	t.Parallel()
	// Geopolitical uses 0 signal since CalibrationRecord has no geo score
	// With all-zero signals, scale should default to 1.0
	records := []CalibrationRecord{
		syntheticScalingRecord(0.1, 4.0, 15, 0.5, -5, 1.0, 0.5),
	}
	c := NewScaleCalibrator()
	scaling := c.CalibrateScales(records)
	if scaling.Geopolitical != 1.0 {
		t.Errorf("expected Geopolitical scale 1.0 (no signal data), got %f", scaling.Geopolitical)
	}
}
