package industry

import (
	"math"
	"reflect"
	"testing"
	"time"
)

// Issue #1944 Batch 1: resolveCardConfig() used to return defaults
// unconditionally, so the calibration tracker injected via
// SetGlobalCycleCalibration (production: monitoring.IndustryService.
// SetCycleCalibration) never reached the cycle status card, while
// IndustryService documented the opposite. These tests pin the consumption
// contract: evidence-driven, and scale-preserving.

func withCalibration(t *testing.T, cal *CycleCalibration) {
	t.Helper()
	old := GetGlobalCycleCalibration()
	SetGlobalCycleCalibration(cal)
	t.Cleanup(func() { SetGlobalCycleCalibration(old) })
}

func TestResolveCardConfig_NoCalibrationUsesDefaults(t *testing.T) {
	withCalibration(t, nil)

	got := resolveCardConfig()
	if !reflect.DeepEqual(got, defaultCardConfig()) {
		t.Fatalf("nil calibration: got %+v, want defaults", got.LayerWeights)
	}
}

// TestResolveCardConfig_NoEvidenceIsIgnored guards rule 1 of the consumption
// contract: with zero recorded outcomes the injected tracker must not change
// the weights. CalibrateWeights() normalises to sum=1, so an unconditional
// call would rescale the composite coefficient by 1/0.85 while nothing was
// learned.
func TestResolveCardConfig_NoEvidenceIsIgnored(t *testing.T) {
	withCalibration(t, NewCycleCalibration(testCalibrationConfig()))

	got := resolveCardConfig()
	if !reflect.DeepEqual(got, defaultCardConfig()) {
		t.Fatalf("no evidence: got %+v, want defaults", got.LayerWeights)
	}
	if math.Abs(weightSum(got.LayerWeights)-0.85) > 1e-9 {
		t.Fatalf("funded weight sum = %v, want 0.85", weightSum(got.LayerWeights))
	}
}

// TestResolveCardConfig_BelowMinSamplesIsIgnored covers the tracker's own
// MinSamples gate: too few outcomes => no opinion => defaults.
func TestResolveCardConfig_BelowMinSamplesIsIgnored(t *testing.T) {
	cal := NewCycleCalibration(testCalibrationConfig()) // MinSamples = 10
	for i := range 3 {
		cal.RecordOutcome("s", time.Now(), map[string]float64{"silicon": 0.9}, -0.01-adjust(i))
	}
	withCalibration(t, cal)

	got := resolveCardConfig()
	if !reflect.DeepEqual(got, defaultCardConfig()) {
		t.Fatalf("below MinSamples: got %+v, want defaults", got.LayerWeights)
	}
}

func adjust(i int) float64 { return float64(i) * 1e-6 }

// TestResolveCardConfig_EvidenceRedistributesWeights pins the actual wiring:
// a layer whose signal matched the market direction (bullish signal on a
// down day => mismatch) is downweighted, a layer that matched is upweighted,
// layers with no metrics keep their default, and the funded sum is preserved.
func TestResolveCardConfig_EvidenceRedistributesWeights(t *testing.T) {
	cfg := testCalibrationConfig()
	cal := NewCycleCalibration(cfg)
	now := time.Now()
	for i := range cfg.MinSamples {
		// Down day: silicon is bullish (>0.5) => wrong; business_cycle is
		// bearish (<=0.5) => right.
		cal.RecordOutcome("sess", now.AddDate(0, 0, -i), map[string]float64{
			"silicon":        0.9,
			"business_cycle": 0.1,
		}, -0.01-float64(i)*1e-6)
	}
	withCalibration(t, cal)

	got := resolveCardConfig().LayerWeights
	if got["silicon"] >= 0.25 {
		t.Errorf("silicon (accuracy 0) should be downweighted below 0.25, got %v", got["silicon"])
	}
	if got["business_cycle"] <= 0.20 {
		t.Errorf("business_cycle (accuracy 1.0) should be upweighted above 0.20, got %v", got["business_cycle"])
	}
	if math.Abs(got["seasonal"]-0.15) > 1e-9 ||
		math.Abs(got["events"]-0.15) > 1e-9 ||
		math.Abs(got["supply_chain"]-0.10) > 1e-9 {
		t.Errorf("layers without metrics must keep defaults, got %+v", got)
	}
	if sum := weightSum(got); math.Abs(sum-0.85) > 1e-9 {
		t.Errorf("funded weight sum = %v, want 0.85 (residual preserved)", sum)
	}
}

// TestApplyCycleCalibration_AllZeroWeightsFallsBackToDefaults documents the
// guard for the current production config (weight_clamp_min/max = 0, i.e. the
// clamp window collapses to zero): a tracker that clamps every layer to 0
// yields no usable weights, so the defaults are kept instead of blanking the
// card. Fixing the all-zero config itself is issue #1944 Batch 2 (I3).
func TestApplyCycleCalibration_AllZeroWeightsFallsBackToDefaults(t *testing.T) {
	cfg := testCalibrationConfig()
	cfg.WeightClampMin = 0
	cfg.WeightClampMax = 0
	cfg.MinSamples = 1
	cal := NewCycleCalibration(cfg)
	// Bullish signal per layer on an up day => every layer "correct" =>
	// upweighted => clamped to the [0,0] window => all weights 0.
	cal.RecordOutcome("s", time.Now(), map[string]float64{
		"silicon":        0.9,
		"business_cycle": 0.9,
		"seasonal":       1.2,
		"events":         1.2,
		"supply_chain":   0.5,
	}, 0.01)

	base := defaultCardConfig()
	if got := cal.CalibrateWeights(base.LayerWeights); weightSum(got) != 0 {
		t.Fatalf("expected the zero clamp window to blank calibrated weights, got %+v", got)
	}
	if got := applyCycleCalibration(base, cal); !reflect.DeepEqual(got, base) {
		t.Fatalf("all-zero calibration must fall back to defaults, got %+v", got.LayerWeights)
	}
}

// TestBuildCard_WithCalibrationStaysInClamp proves the wired path end-to-end:
// a card built with a calibration tracker present keeps the documented
// composite clamp (0.80-1.20).
func TestBuildCard_WithCalibrationStaysInClamp(t *testing.T) {
	cfg := testCalibrationConfig()
	cal := NewCycleCalibration(cfg)
	for i := range cfg.MinSamples {
		cal.RecordOutcome("sess", time.Now().AddDate(0, 0, -i), map[string]float64{
			"silicon":        0.9,
			"business_cycle": 0.1,
		}, -0.01-float64(i)*1e-6)
	}
	withCalibration(t, cal)

	builder := NewCycleStatusCardBuilder(NewSiliconCycleTracker(), NewCycleTracker(), NewSeasonalEngineFromConfig(nil), NewEventCalendar(), NewLinkageAnalyzer())
	card, err := builder.BuildCard(time.Now(), "semiconductor")
	if err != nil {
		t.Fatal(err)
	}
	if card.CompositeCoefficient < 0.80 || card.CompositeCoefficient > 1.20 {
		t.Fatalf("composite coefficient out of clamp range: %v", card.CompositeCoefficient)
	}
}
