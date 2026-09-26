package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeTestSSOT writes a minimal parameters file and returns its path.
func writeTestSSOT(t *testing.T, maxPosition, maxDailyLoss float64) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "parameters.json")
	body := map[string]any{
		"version": "test",
		"risk": map[string]any{
			"max_position_size":  map[string]any{"value": maxPosition},
			"max_daily_loss_pct": map[string]any{"value": maxDailyLoss},
		},
	}
	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		t.Fatalf("marshal ssot: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write ssot: %v", err)
	}
	return path
}

func overlayEntry(value, before, ssot float64) CalibrationOverlayEntry {
	return CalibrationOverlayEntry{
		Value:        value,
		Before:       before,
		SSOT:         ssot,
		CalibratedAt: time.Date(2026, 9, 26, 3, 8, 44, 0, time.UTC),
		Method:       "bayesian_optimization",
		Rationale:    "baseline_score=0.5000, optimized_score=0.6000 (+20.0% delta).",
	}
}

// TestCalibrationOverlayPath pins the overlay location: it must live under the
// bind-mounted data/ tree (FU-20260926-07), never under configs/.
func TestCalibrationOverlayPath(t *testing.T) {
	got := CalibrationOverlayPath("/app")
	want := "/app/data/state/parameters.calibrated.json"
	if got != want {
		t.Fatalf("CalibrationOverlayPath(/app) = %q, want %q", got, want)
	}
}

// TestApplyCalibratedOverlayLayer_AppliesOnTopOfSSOT is the core contract: the
// SSOT file keeps its reviewed value while the effective value is the overlay's.
func TestApplyCalibratedOverlayLayer_AppliesOnTopOfSSOT(t *testing.T) {
	ssotPath := writeTestSSOT(t, 0.15, 0.03)
	overlayPath := filepath.Join(t.TempDir(), "parameters.calibrated.json")

	if _, err := UpdateCalibrationOverlay(overlayPath, "risk_gate_calibrate", map[string]CalibrationOverlayEntry{
		"risk_max_position_size": overlayEntry(0.13, 0.15, 0.15),
	}); err != nil {
		t.Fatalf("UpdateCalibrationOverlay: %v", err)
	}

	SetCalibratedOverlayPath(overlayPath)
	defer SetCalibratedOverlayPath("")

	cfg, err := LoadEffectiveParametersConfig(ssotPath)
	if err != nil {
		t.Fatalf("LoadEffectiveParametersConfig: %v", err)
	}
	if got := cfg.Risk.MaxPositionSize.Value; got != 0.13 {
		t.Errorf("effective risk_max_position_size = %v, want 0.13 (overlay value)", got)
	}
	// A parameter with no overlay entry keeps the SSOT value.
	if got := cfg.Risk.MaxDailyLossPct.Value; got != 0.03 {
		t.Errorf("effective risk_max_daily_loss_pct = %v, want 0.03 (ssot value)", got)
	}

	// The SSOT file itself must be untouched: it is what a human reviews.
	ssotCfg, err := LoadParametersConfig(ssotPath)
	if err != nil {
		t.Fatalf("LoadParametersConfig: %v", err)
	}
	if got := ssotCfg.Risk.MaxPositionSize.Value; got != 0.15 {
		t.Errorf("ssot risk_max_position_size = %v, want 0.15 (overlay must not rewrite the SSOT)", got)
	}
}

// TestApplyCalibratedOverlayLayer_DisabledByDefault is the negative control for
// the whole overlay: with no path registered, the loader must behave exactly
// like a plain SSOT read even when an overlay file exists on disk.
func TestApplyCalibratedOverlayLayer_DisabledByDefault(t *testing.T) {
	ssotPath := writeTestSSOT(t, 0.15, 0.03)
	overlayPath := filepath.Join(t.TempDir(), "parameters.calibrated.json")
	if _, err := UpdateCalibrationOverlay(overlayPath, "risk_gate_calibrate", map[string]CalibrationOverlayEntry{
		"risk_max_position_size": overlayEntry(0.13, 0.15, 0.15),
	}); err != nil {
		t.Fatalf("UpdateCalibrationOverlay: %v", err)
	}

	SetCalibratedOverlayPath("") // default: overlay off
	cfg, err := LoadEffectiveParametersConfig(ssotPath)
	if err != nil {
		t.Fatalf("LoadEffectiveParametersConfig: %v", err)
	}
	if got := cfg.Risk.MaxPositionSize.Value; got != 0.15 {
		t.Errorf("effective risk_max_position_size = %v, want 0.15 (overlay disabled ⇒ SSOT only)", got)
	}
}

// TestApplyCalibratedOverlayLayer_InvalidatesWhenSSOTMoved covers the guard that
// keeps a reviewed charter edit authoritative: an overlay entry reconciled
// against 0.15 is dropped (and removed from disk) once the SSOT says 0.20.
func TestApplyCalibratedOverlayLayer_InvalidatesWhenSSOTMoved(t *testing.T) {
	ssotPath := writeTestSSOT(t, 0.20, 0.03)
	overlayPath := filepath.Join(t.TempDir(), "parameters.calibrated.json")
	if _, err := UpdateCalibrationOverlay(overlayPath, "risk_gate_calibrate", map[string]CalibrationOverlayEntry{
		"risk_max_position_size": overlayEntry(0.13, 0.15, 0.15),
	}); err != nil {
		t.Fatalf("UpdateCalibrationOverlay: %v", err)
	}

	SetCalibratedOverlayPath(overlayPath)
	defer SetCalibratedOverlayPath("")

	cfg, err := LoadEffectiveParametersConfig(ssotPath)
	if err != nil {
		t.Fatalf("LoadEffectiveParametersConfig: %v", err)
	}
	if got := cfg.Risk.MaxPositionSize.Value; got != 0.20 {
		t.Errorf("effective risk_max_position_size = %v, want 0.20 (stale overlay must not override a moved SSOT)", got)
	}

	ov, err := LoadCalibrationOverlay(overlayPath)
	if err != nil {
		t.Fatalf("LoadCalibrationOverlay: %v", err)
	}
	if _, ok := ov.Entries["risk_max_position_size"]; ok {
		t.Errorf("stale entry survived reconciliation: %+v", ov.Entries)
	}
}

// TestApplyCalibratedOverlayLayer_DropsUnknownParameter covers a typo'd entry:
// it cannot be applied, so it must be reported and removed rather than kept.
func TestApplyCalibratedOverlayLayer_DropsUnknownParameter(t *testing.T) {
	ssotPath := writeTestSSOT(t, 0.15, 0.03)
	overlayPath := filepath.Join(t.TempDir(), "parameters.calibrated.json")
	if _, err := UpdateCalibrationOverlay(overlayPath, "risk_gate_calibrate", map[string]CalibrationOverlayEntry{
		"risk_max_position_sixe": overlayEntry(0.13, 0.15, 0.15), // typo on purpose
	}); err != nil {
		t.Fatalf("UpdateCalibrationOverlay: %v", err)
	}

	SetCalibratedOverlayPath(overlayPath)
	defer SetCalibratedOverlayPath("")

	// A fresh config instance, so the assertion does not depend on the singleton.
	cfg, err := LoadParametersConfig(ssotPath)
	if err != nil {
		t.Fatalf("LoadParametersConfig: %v", err)
	}
	report := ApplyCalibratedOverlayLayer(cfg)
	if len(report.Unknown) != 1 || report.Unknown[0] != "risk_max_position_sixe" {
		t.Fatalf("Unknown = %v, want [risk_max_position_sixe]", report.Unknown)
	}
	if len(report.Applied) != 0 {
		t.Errorf("Applied = %v, want none (unknown parameter)", report.AppliedNames())
	}
	ov, err := LoadCalibrationOverlay(overlayPath)
	if err != nil {
		t.Fatalf("LoadCalibrationOverlay: %v", err)
	}
	if len(ov.Entries) != 0 {
		t.Errorf("unknown entry survived reconciliation: %+v", ov.Entries)
	}
}

// TestApplyCalibratedOverlayLayer_ReconcilesBaseline pins the first-startup
// bootstrap: an entry written with no reconciliation baseline gets one, so a
// later SSOT edit can invalidate it.
func TestApplyCalibratedOverlayLayer_ReconcilesBaseline(t *testing.T) {
	ssotPath := writeTestSSOT(t, 0.15, 0.03)
	overlayPath := filepath.Join(t.TempDir(), "parameters.calibrated.json")
	if _, err := UpdateCalibrationOverlay(overlayPath, "risk_gate_calibrate", map[string]CalibrationOverlayEntry{
		"risk_max_position_size": overlayEntry(0.13, 0.15, 0), // written but never restarted
	}); err != nil {
		t.Fatalf("UpdateCalibrationOverlay: %v", err)
	}

	SetCalibratedOverlayPath(overlayPath)
	defer SetCalibratedOverlayPath("")

	cfg, err := LoadParametersConfig(ssotPath)
	if err != nil {
		t.Fatalf("LoadParametersConfig: %v", err)
	}
	report := ApplyCalibratedOverlayLayer(cfg)
	if len(report.Applied) != 1 || report.Applied[0].SSOT != 0.15 || report.Applied[0].Effective != 0.13 {
		t.Fatalf("Applied = %+v, want one entry ssot=0.15 effective=0.13", report.Applied)
	}
	if !report.Reconciled {
		t.Errorf("Reconciled = false, want true (baseline had to be recorded)")
	}
	ov, err := LoadCalibrationOverlay(overlayPath)
	if err != nil {
		t.Fatalf("LoadCalibrationOverlay: %v", err)
	}
	if got := ov.Entries["risk_max_position_size"].SSOT; got != 0.15 {
		t.Errorf("reconciled ssot = %v, want 0.15", got)
	}
}

// TestUpdateCalibrationOverlay_MergesEntries guards the merge: a later round
// that only reports one tunable must not drop the other one.
func TestUpdateCalibrationOverlay_MergesEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "parameters.calibrated.json")
	if _, err := UpdateCalibrationOverlay(path, "risk_gate_calibrate", map[string]CalibrationOverlayEntry{
		"risk_max_position_size":  overlayEntry(0.13, 0.15, 0.15),
		"risk_max_daily_loss_pct": overlayEntry(0.031, 0.03, 0.03),
	}); err != nil {
		t.Fatalf("first update: %v", err)
	}
	if _, err := UpdateCalibrationOverlay(path, "risk_gate_calibrate", map[string]CalibrationOverlayEntry{
		"risk_max_position_size": overlayEntry(0.14, 0.13, 0.15),
	}); err != nil {
		t.Fatalf("second update: %v", err)
	}

	ov, err := LoadCalibrationOverlay(path)
	if err != nil {
		t.Fatalf("LoadCalibrationOverlay: %v", err)
	}
	if len(ov.Entries) != 2 {
		t.Fatalf("entries = %d, want 2 (merge must keep unmentioned parameters): %+v", len(ov.Entries), ov.Entries)
	}
	if got := ov.Entries["risk_max_daily_loss_pct"].Value; got != 0.031 {
		t.Errorf("daily loss entry = %v, want 0.031 (must survive)", got)
	}
	if got := ov.Entries["risk_max_position_size"].Value; got != 0.14 {
		t.Errorf("position entry = %v, want 0.14 (updated)", got)
	}
	if got := ov.Entries["risk_max_position_size"].SSOT; got != 0.15 {
		t.Errorf("position ssot baseline = %v, want 0.15 (preserved across updates)", got)
	}
}

// TestUpdateCalibrationOverlay_EmptyPathIsError pins the fail-loud path: writing
// without a registered overlay path must not silently succeed.
func TestUpdateCalibrationOverlay_EmptyPathIsError(t *testing.T) {
	if _, err := UpdateCalibrationOverlay("", "risk_gate_calibrate", nil); err == nil {
		t.Fatal(`UpdateCalibrationOverlay("", ...) = nil error, want error`)
	}
}

// TestLoadCalibrationOverlay_MissingFileIsNotAnError pins the fail-safe read.
func TestLoadCalibrationOverlay_MissingFileIsNotAnError(t *testing.T) {
	ov, err := LoadCalibrationOverlay(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("LoadCalibrationOverlay(missing) = %v, want nil error", err)
	}
	if ov != nil {
		t.Fatalf("LoadCalibrationOverlay(missing) = %+v, want nil overlay", ov)
	}
}

// TestSSOTParameterValue_IgnoresOverlay is the reason the writer can record an
// exact reconciliation baseline: the SSOT read must not be the overlaid value.
func TestSSOTParameterValue_IgnoresOverlay(t *testing.T) {
	ssotPath := writeTestSSOT(t, 0.15, 0.03)
	overlayPath := filepath.Join(t.TempDir(), "parameters.calibrated.json")
	oldPath := GetParametersConfigPath()
	SetParametersConfigPath(ssotPath)
	defer SetParametersConfigPath(oldPath)

	if _, err := UpdateCalibrationOverlay(overlayPath, "risk_gate_calibrate", map[string]CalibrationOverlayEntry{
		"risk_max_position_size": overlayEntry(0.13, 0.15, 0.15),
	}); err != nil {
		t.Fatalf("UpdateCalibrationOverlay: %v", err)
	}
	SetCalibratedOverlayPath(overlayPath)
	defer SetCalibratedOverlayPath("")

	// Warm the singleton through the overlay, then read the SSOT value.
	*GetParametersConfig() = *mustLoad(t, ssotPath)

	got, ok := SSOTParameterValue("risk_max_position_size")
	if !ok {
		t.Fatal("SSOTParameterValue returned ok=false")
	}
	if got != 0.15 {
		t.Errorf("SSOTParameterValue = %v, want 0.15 (must ignore the overlay value 0.13)", got)
	}
	if _, ok := SSOTParameterValue("not_a_parameter"); ok {
		t.Error("SSOTParameterValue(unknown) = ok, want !ok")
	}
}

func mustLoad(t *testing.T, path string) *ParametersConfig {
	t.Helper()
	cfg, err := LoadEffectiveParametersConfig(path)
	if err != nil {
		t.Fatalf("LoadEffectiveParametersConfig: %v", err)
	}
	return cfg
}
