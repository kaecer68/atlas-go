package calibration

// darwinian_risk_vol_single_step_test.go — 2026-10-16 5.9x jump.
//
// On 2026-10-16 the deployed auto-calibration moved
// darwinian.risk_volatility_threshold from 0.0726591 to 0.012324 (ratio 0.170).
// The window that calls such a step unexplained — [1/3, 3] — already existed, but
// only on the READ side: ApplyCalibratedOverlayLayer warns and the drift metric
// pages on it, so the jump was written first and described afterwards.
//
// These tests pin the write side: a computed value more than 3x away from (or less
// than a third of) the previous one is NOT written, records why, and leaves the
// effective configuration untouched; a value inside the window, and a first write
// with no previous value, behave exactly as before.

import (
	"math"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
)

// newDarwinianCfgWith builds a darwinian test config whose previous
// risk_volatility_threshold is `before` (0 = no previous value yet).
func newDarwinianCfgWith(before float64) (*config.InferenceEngine, *config.ParametersConfig) {
	cfg := config.DefaultParametersConfig()
	cfg.Darwinian.SharpeMinSampleSize.Value = 2
	cfg.Darwinian.RiskVolatilityThreshold.Value = before
	return config.NewInferenceEngine(cfg), cfg
}

// writeDarwinianOutcomes(t, 0.08) (band test fixture) computes a daily
// volatility of about 0.01265 — the same order as the 2026-10-16 calibrated
// value, so the ratios below are the ones production saw.
const (
	jumpFixtureScale    = 0.08
	incidentPreviousVol = 0.0726591 // the value in force before the 2026-10-16 tick
)

// TestCalibrateDarwinian_RiskVolInsideWindowStillWrites pins ①: a step the window
// can explain (ratio 0.90) is written exactly as before.
func TestCalibrateDarwinian_RiskVolInsideWindowStillWrites(t *testing.T) {
	writeDarwinianOutcomes(t, jumpFixtureScale)
	ie, cfg := newDarwinianCfgWith(0.014)
	before := cfg.Darwinian.RiskVolatilityThreshold.Value

	res := CalibrateDarwinian(ie, 100, cfg)
	entry := findRiskVol(t, res)
	ratio := entry.After / before
	if ratio < config.CalibrationOverlaySingleStepWindowMin || ratio > config.CalibrationOverlaySingleStepWindowMax {
		t.Fatalf("fixture drift: ratio %.4f is outside the window, so this case cannot test the in-window path", ratio)
	}
	if entry.Method != "percentile_based" {
		t.Fatalf("method = %q, want percentile_based (inside the window the write path is unchanged)", entry.Method)
	}
	if entry.After == entry.Before {
		t.Fatalf("entry = %+v, want the computed value", entry)
	}
	if got, ok := ie.GetParameter("darwinian_risk_volatility_threshold"); !ok || got != entry.After {
		t.Errorf("effective value = %v (found=%v), want the computed %v", got, ok, entry.After)
	}
}

// TestCalibrateDarwinian_RiskVolOutsideWindowDoesNotWrite pins ②: the 2026-10-16
// jump (0.0726591 -> ~0.0126, ratio ~0.17) must NOT be written, and must say why.
func TestCalibrateDarwinian_RiskVolOutsideWindowDoesNotWrite(t *testing.T) {
	writeDarwinianOutcomes(t, jumpFixtureScale)
	ie, cfg := newDarwinianCfgWith(incidentPreviousVol)

	res := CalibrateDarwinian(ie, 100, cfg)
	entry := findRiskVol(t, res)
	if entry.Method != "skipped_single_step_window" {
		t.Fatalf("method = %q, want skipped_single_step_window (a single round cannot explain this jump)", entry.Method)
	}
	if entry.After != entry.Before {
		t.Errorf("entry.After = %v, want the unchanged %v (overlay persists only changed leaves)", entry.After, entry.Before)
	}
	if got, _ := ie.GetParameter("darwinian_risk_volatility_threshold"); got != incidentPreviousVol {
		t.Errorf("effective value = %v, want the untouched %v", got, incidentPreviousVol)
	}
	if entry.CalibrationNotes == "" || !strings.Contains(entry.CalibrationNotes, "single-step window") {
		t.Errorf("CalibrationNotes = %q, want an explanation naming the single-step window", entry.CalibrationNotes)
	}
	if entry.Confidence != 0 {
		t.Errorf("Confidence = %v, want 0 (nothing was published)", entry.Confidence)
	}
}

// TestCalibrateDarwinian_RiskVolFirstWriteIgnoresWindow pins ③: with no previous
// value there is nothing to compare, so the window must not block the first write
// (absence of a comparison is not evidence of an anomaly).
func TestCalibrateDarwinian_RiskVolFirstWriteIgnoresWindow(t *testing.T) {
	writeDarwinianOutcomes(t, jumpFixtureScale)
	ie, cfg := newDarwinianCfgWith(0)

	res := CalibrateDarwinian(ie, 100, cfg)
	entry := findRiskVol(t, res)
	if entry.Method != "percentile_based" {
		t.Fatalf("method = %q, want percentile_based (the first write has no previous value to compare)", entry.Method)
	}
	if entry.Before != 0 || entry.After <= 0 {
		t.Errorf("entry = %+v, want Before 0 and a positive computed value", entry)
	}
	if got, _ := ie.GetParameter("darwinian_risk_volatility_threshold"); got != entry.After {
		t.Errorf("effective value = %v, want the computed %v", got, entry.After)
	}
}

// TestSingleStepWindowRejects documents the boundary: inclusive at both edges, and
// no rejection when either side is non-positive (no ratio exists).
func TestSingleStepWindowRejects(t *testing.T) {
	minR := config.CalibrationOverlaySingleStepWindowMin // 1/3
	maxR := config.CalibrationOverlaySingleStepWindowMax // 3
	cases := []struct {
		name   string
		before float64
		after  float64
		want   bool
	}{
		{"inside", 1, 2, false},
		{"unchanged", 1, 1, false},
		{"upper_edge_inclusive", 1, maxR, false},
		{"above_upper_edge", 1, maxR * 1.0001, true},
		{"lower_edge_inclusive", 1, minR, false},
		{"below_lower_edge", 1, minR * 0.9999, true},
		{"incident_ratio", 0.0726591, 0.012324, true},
		{"no_previous_value", 0, 0.0123, false},
		{"negative_previous_value", -0.5, 0.0123, false},
		{"non_positive_new_value", 1, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := singleStepWindowRejects(tc.before, tc.after); got != tc.want {
				t.Errorf("singleStepWindowRejects(%v, %v) = %v, want %v", tc.before, tc.after, got, tc.want)
			}
		})
	}
	if math.Abs(minR-1.0/3.0) > 1e-15 {
		t.Fatalf("window lower edge = %v, want 1/3", minR)
	}
}
