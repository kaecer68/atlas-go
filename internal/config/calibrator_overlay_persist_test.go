package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeCalibrator is a minimal ParameterCalibrator for the persistence tests.
type fakeCalibrator struct{ names []string }

func (f fakeCalibrator) ParamNames() []string { return f.names }

// TestCalibrateParameters_PersistsToOverlayNotSSOT is the FU-20260926-07
// contract for the generic calibrator: accepted changes land in the
// calibrated-parameters overlay and configs/parameters.json stays byte-identical.
func TestCalibrateParameters_PersistsToOverlayNotSSOT(t *testing.T) {
	dir := t.TempDir()
	ssotPath := writeTestSSOT(t, 0.15, 0.03)
	overlayPath := filepath.Join(dir, "data", "state", "parameters.calibrated.json")
	ssotBefore := readFile(t, ssotPath)

	previousPath := GetParametersConfigPath()
	previousOverlay := GetCalibratedOverlayPath()
	SetParametersConfigPath(ssotPath)
	SetCalibratedOverlayPath(overlayPath)
	ResetParametersConfig()
	defer func() {
		SetParametersConfigPath(previousPath)
		SetCalibratedOverlayPath(previousOverlay)
		ResetParametersConfig()
	}()

	const param = "darwinian_weight_max"
	// The evaluator is maximized by a larger weight, so the optimizer must move
	// the parameter away from the SSOT value and the change must be applied.
	evaluator := func(cfg *ParametersConfig) (float64, error) {
		return cfg.Darwinian.WeightMax.Value * 100, nil
	}
	cfg := DefaultCalibrateConfig()
	cfg.InitialPoints, cfg.Iterations, cfg.MinImprovement = 6, 8, 0

	result, err := CalibrateParameters(context.Background(), fakeCalibrator{names: []string{param}}, evaluator, cfg)
	if err != nil {
		t.Fatalf("CalibrateParameters: %v", err)
	}
	if len(result.Changes) == 0 {
		t.Fatalf("no changes applied, cannot assert persistence (result: %+v)", result)
	}

	ov, err := LoadCalibrationOverlay(overlayPath)
	if err != nil {
		t.Fatalf("LoadCalibrationOverlay: %v", err)
	}
	if ov == nil {
		t.Fatal("overlay was not written")
	}
	entry, ok := ov.Entries[param]
	if !ok {
		t.Fatalf("overlay has no entry for %s: %+v", param, ov.Entries)
	}
	if entry.Method != "bayesian_optimization" {
		t.Errorf("entry method = %q, want bayesian_optimization", entry.Method)
	}
	if got := numericOf(t, entry.Value); got != result.Changes[0].After {
		t.Errorf("overlay value = %v, want the applied value %v", got, result.Changes[0].After)
	}
	if entry.SSOT == nil || !entry.SSOT.Present {
		t.Errorf("entry baseline = %+v, want the SSOT value", entry.SSOT)
	}
	if ov.Source != "calibrate_parameters" {
		t.Errorf("overlay source = %q, want calibrate_parameters", ov.Source)
	}

	// The SSOT file must be untouched: that is the whole point of the overlay.
	if after := readFile(t, ssotPath); after != ssotBefore {
		t.Errorf("SSOT document rewritten by the calibrator:\nbefore=%s\nafter=%s", ssotBefore, after)
	}
}

// TestPersistCalibratorChanges_NoOverlayPathIsNotFatal pins the disabled-overlay
// behaviour: nothing is written, nothing panics, and the caller still gets its
// in-memory result.
func TestPersistCalibratorChanges_NoOverlayPathIsNotFatal(t *testing.T) {
	previousOverlay := GetCalibratedOverlayPath()
	SetCalibratedOverlayPath("")
	defer SetCalibratedOverlayPath(previousOverlay)

	persistCalibratorChanges([]CalibratorChange{
		{ParamName: "darwinian_weight_max", Before: 0.5, After: 0.6},
	}, time.Now())
}

// TestPersistCalibratorChanges_WritesEveryAcceptedChange keeps the entry set in
// sync with the report: every accepted change must be reproducible from the
// overlay alone.
func TestPersistCalibratorChanges_WritesEveryAcceptedChange(t *testing.T) {
	ssotPath := writeTestSSOT(t, 0.15, 0.03)
	overlayPath := filepath.Join(t.TempDir(), "parameters.calibrated.json")

	previousSSOT := GetParametersConfigPath()
	previousOverlay := GetCalibratedOverlayPath()
	SetParametersConfigPath(ssotPath)
	SetCalibratedOverlayPath(overlayPath)
	defer func() {
		SetParametersConfigPath(previousSSOT)
		SetCalibratedOverlayPath(previousOverlay)
	}()

	at := time.Date(2026, 9, 26, 3, 8, 44, 0, time.UTC)
	persistCalibratorChanges([]CalibratorChange{
		{ParamName: "risk_max_position_size", Before: 0.15, After: 0.13, DeltaPct: -13.3, Confidence: "high"},
		{ParamName: "risk_max_daily_loss_pct", Before: 0.03, After: 0.04, DeltaPct: 33.3, Confidence: "medium"},
	}, at)

	ov, err := LoadCalibrationOverlay(overlayPath)
	if err != nil {
		t.Fatalf("LoadCalibrationOverlay: %v", err)
	}
	if ov == nil || len(ov.Entries) != 2 {
		t.Fatalf("overlay entries = %+v, want 2", ov)
	}
	for name, want := range map[string]float64{"risk_max_position_size": 0.13, "risk_max_daily_loss_pct": 0.04} {
		entry, ok := ov.Entries[name]
		if !ok {
			t.Fatalf("missing entry %s: %+v", name, ov.Entries)
		}
		if got := numericOf(t, entry.Value); got != want {
			t.Errorf("entry %s value = %v, want %v", name, got, want)
		}
		if !entry.CalibratedAt.Equal(at) {
			t.Errorf("entry %s calibrated_at = %v, want %v", name, entry.CalibratedAt, at)
		}
		if entry.SSOT == nil || !entry.SSOT.Present {
			t.Errorf("entry %s baseline missing: %+v", name, entry.SSOT)
		}
	}
}

// TestPersistCalibratorChanges_EmptyChangesWritesNothing pins that a run with no
// accepted change does not touch the overlay file at all.
func TestPersistCalibratorChanges_EmptyChangesWritesNothing(t *testing.T) {
	overlayPath := filepath.Join(t.TempDir(), "parameters.calibrated.json")
	previousOverlay := GetCalibratedOverlayPath()
	SetCalibratedOverlayPath(overlayPath)
	defer SetCalibratedOverlayPath(previousOverlay)

	persistCalibratorChanges(nil, time.Now())

	if _, err := os.Stat(overlayPath); !os.IsNotExist(err) {
		t.Fatalf("overlay file created for an empty change set (stat err = %v)", err)
	}
}
