package industry

import (
	"math"
	"reflect"
	"testing"
	"time"
)

// TestCalibrationApplied_DerivedFromEvidence pins the outward "calibrated" flag
// derivation (issue #1944 Batch 2, E-item): it must be false without evidence
// and true only when calibration evidence actually redistributes the funded
// layer weights — not merely because a metrics map exists.
func TestCalibrationApplied_DerivedFromEvidence(t *testing.T) {
	withCalibration(t, nil)
	if CalibrationApplied() {
		t.Fatal("no calibration tracker: CalibrationApplied() must be false")
	}

	cfg := testCalibrationConfig()
	cal := NewCycleCalibration(cfg)
	withCalibration(t, cal)
	if CalibrationApplied() {
		t.Fatal("tracker without outcomes: CalibrationApplied() must be false")
	}

	// Evidence for layers with opposite accuracy (silicon wrong, business_cycle
	// right) redistributes the weights, so the outward flag must flip.
	for i := range cfg.MinSamples {
		cal.RecordOutcome("sess", time.Now().AddDate(0, 0, -i), map[string]float64{
			"silicon":        0.9,
			"business_cycle": 0.1,
		}, -0.01-float64(i)*1e-6)
	}
	if !CalibrationApplied() {
		t.Fatal("tracker with redistributing evidence: CalibrationApplied() must be true")
	}

	base := defaultCardConfig().LayerWeights
	effective := EffectiveCardConfig().LayerWeights
	if effective["silicon"] >= base["silicon"] {
		t.Errorf("silicon (accuracy 0) should be downweighted: %v → %v", base["silicon"], effective["silicon"])
	}
}

// TestCalibrationApplied_NoNudgedLayerIsNotCalibration is the A1 contract of
// issue #1944 Batch A: evidence that crosses no hit-rate threshold moves no
// weight, so the outward "calibrated" flag must stay false.
//
// Regression it pins: CalibrateWeights normalises through normalizeWeights
// (rounds every layer to 4 dp) and the caller rescales back to the funded sum,
// so rebuilding the weights with *no* nudged layer still shifted them by ~1e-5
// — five orders of magnitude above the 1e-12 tolerance in CalibrationApplied.
// A tracker whose evidence changed nothing therefore reported itself as
// calibrated, which is exactly the "hardcoded applied" class of defect this
// flag was introduced to remove.
func TestCalibrationApplied_NoNudgedLayerIsNotCalibration(t *testing.T) {
	cfg := testCalibrationConfig() // MinSamples=10, HitRateHigh=0.55, HitRateLow=0.45
	cal := NewCycleCalibration(cfg)
	for i := range cfg.MinSamples {
		// silicon 0.9 is bullish; alternating return signs make it right on
		// exactly half the outcomes => accuracy 0.50, which is neither
		// > HitRateHigh nor < HitRateLow, so no layer is nudged.
		ret := 0.01
		if i%2 == 1 {
			ret = -0.01
		}
		cal.RecordOutcome("sess", time.Now().AddDate(0, 0, -i),
			map[string]float64{"silicon": 0.9}, ret)
	}
	if got := cal.GetMetrics()["silicon"].Accuracy; math.Abs(got-0.5) > 1e-12 {
		t.Fatalf("precondition: silicon accuracy = %v, want 0.50 (an accuracy no layer threshold acts on)", got)
	}
	withCalibration(t, cal)

	if CalibrationApplied() {
		t.Fatal("accuracy 0.50 crosses no threshold: CalibrationApplied() must be false")
	}
	base := defaultCardConfig().LayerWeights
	if got := EffectiveCardConfig().LayerWeights; !reflect.DeepEqual(got, base) {
		t.Errorf("no layer was nudged, effective weights must be the baseline unchanged: got %v, want %v", got, base)
	}
}

// TestCalibrationApplied_NudgedLayerIsCalibration is the other half of the A1
// pair: a layer whose accuracy really crosses a threshold must still flip the
// flag. It keeps the no-op fix from being implemented by muting calibration.
func TestCalibrationApplied_NudgedLayerIsCalibration(t *testing.T) {
	cfg := testCalibrationConfig()
	cal := NewCycleCalibration(cfg)
	for i := range cfg.MinSamples {
		// silicon 0.9 is bullish and every return is positive => accuracy 1.0
		// > HitRateHigh (0.55) => the layer is upweighted.
		cal.RecordOutcome("sess", time.Now().AddDate(0, 0, -i),
			map[string]float64{"silicon": 0.9}, 0.01+float64(i)*1e-6)
	}
	if got := cal.GetMetrics()["silicon"].Accuracy; got != 1.0 {
		t.Fatalf("precondition: silicon accuracy = %v, want 1.0", got)
	}
	withCalibration(t, cal)

	if !CalibrationApplied() {
		t.Fatal("silicon accuracy 1.0 > HitRateHigh 0.55 upweights a funded layer: CalibrationApplied() must be true")
	}
	base := defaultCardConfig().LayerWeights
	if got := EffectiveCardConfig().LayerWeights; got["silicon"] <= base["silicon"] {
		t.Errorf("silicon should be upweighted: %v → %v", base["silicon"], got["silicon"])
	}
}
