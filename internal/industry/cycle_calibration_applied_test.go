package industry

import (
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
