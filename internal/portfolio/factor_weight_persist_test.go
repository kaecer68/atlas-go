package portfolio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
)

// writeFactorWeightSSOT writes a parameters.json fixture carrying the complete
// eight-factor weight map (the SSOT validation requires the full set).
func writeFactorWeightSSOT(t *testing.T, weights map[string]float64) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "configs", "parameters.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := map[string]any{
		"version": "test",
		"factor_weight": map[string]any{
			"base_weights": map[string]any{"value": weights},
		},
	}
	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func defaultWeightFixture() map[string]float64 {
	return map[string]float64{
		"momentum": 0.25, "value": 0.2, "quality": 0.2, "agent": 0.05,
		"inst_sent": 0.1, "liquidity": 0.05, "narrative": 0.05, "industry_cycle": 0.0,
	}
}

// TestApplyFactorWeights_PersistsToOverlayNotSSOT is the FU-20260926-07 contract
// for the factor-weight calibrator: the adapted weights land in the
// calibrated-parameters overlay under data/, and configs/parameters.json (the
// reviewed SSOT) stays byte-identical.
func TestApplyFactorWeights_PersistsToOverlayNotSSOT(t *testing.T) {
	dir := t.TempDir()
	ssotPath := writeFactorWeightSSOT(t, defaultWeightFixture())
	overlayPath := filepath.Join(dir, "data", "state", "parameters.calibrated.json")
	ssotBefore, err := os.ReadFile(ssotPath)
	if err != nil {
		t.Fatal(err)
	}

	previousSSOT := config.GetParametersConfigPath()
	previousOverlay := config.GetCalibratedOverlayPath()
	config.SetParametersConfigPath(ssotPath)
	config.SetCalibratedOverlayPath(overlayPath)
	config.ResetParametersConfig()
	defer func() {
		config.SetParametersConfigPath(previousSSOT)
		config.SetCalibratedOverlayPath(previousOverlay)
		config.ResetParametersConfig()
	}()

	applyFactorWeights(map[FactorType]float64{"momentum": 0.42, "quality": 0.11})

	ov, err := config.LoadCalibrationOverlay(overlayPath)
	if err != nil {
		t.Fatalf("LoadCalibrationOverlay: %v", err)
	}
	if ov == nil {
		t.Fatal("overlay was not written")
	}
	if ov.Source != "factor_weight_calibrate" {
		t.Errorf("overlay source = %q, want factor_weight_calibrate", ov.Source)
	}
	entry, ok := ov.Entries["factor_weight.base_weights.value"]
	if !ok {
		t.Fatalf("overlay has no weight entry: %+v", ov.Entries)
	}
	if entry.Path != "factor_weight.base_weights.value" {
		t.Errorf("entry path = %q", entry.Path)
	}
	weights, ok := entry.Value.(map[string]any)
	if !ok {
		t.Fatalf("entry value = %#v, want an object", entry.Value)
	}
	if got := weights["momentum"]; got != 0.42 {
		t.Errorf("persisted momentum = %v, want 0.42", got)
	}
	if got := weights["quality"]; got != 0.11 {
		t.Errorf("persisted quality = %v, want 0.11", got)
	}
	if len(weights) != 8 {
		t.Errorf("persisted weight count = %d, want 8 (the SSOT validation requires the full set)", len(weights))
	}
	for _, leaf := range []string{
		"factor_weight.base_weights.last_calibrated",
		"factor_weight.base_weights.calibration_method",
	} {
		if _, ok := ov.Entries[leaf]; !ok {
			t.Errorf("overlay missing provenance entry %s: %+v", leaf, ov.Entries)
		}
	}

	// The SSOT file must be untouched.
	ssotAfter, err := os.ReadFile(ssotPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(ssotAfter) != string(ssotBefore) {
		t.Errorf("SSOT document rewritten by the factor weight calibrator:\nbefore=%s\nafter=%s", ssotBefore, ssotAfter)
	}

	// The in-memory configuration must still carry the adapted weights, so the
	// running process uses them immediately.
	live := config.GetParametersConfig()
	if got := live.FactorWeight.BaseWeights.Value["momentum"]; got != 0.42 {
		t.Errorf("in-memory momentum = %v, want 0.42", got)
	}
}

// TestApplyFactorWeights_OverlayRoundTrips proves the persisted entry is usable:
// layering it back on the SSOT reproduces the adapted weights.
func TestApplyFactorWeights_OverlayRoundTrips(t *testing.T) {
	dir := t.TempDir()
	ssotPath := writeFactorWeightSSOT(t, defaultWeightFixture())
	overlayPath := filepath.Join(dir, "data", "state", "parameters.calibrated.json")

	previousSSOT := config.GetParametersConfigPath()
	previousOverlay := config.GetCalibratedOverlayPath()
	config.SetParametersConfigPath(ssotPath)
	config.SetCalibratedOverlayPath(overlayPath)
	config.ResetParametersConfig()
	defer func() {
		config.SetParametersConfigPath(previousSSOT)
		config.SetCalibratedOverlayPath(previousOverlay)
		config.ResetParametersConfig()
	}()

	applyFactorWeights(map[FactorType]float64{"momentum": 0.42})

	config.ResetParametersConfig()
	effective := config.GetParametersConfig()
	if got := effective.FactorWeight.BaseWeights.Value["momentum"]; got != 0.42 {
		t.Errorf("effective momentum after reload = %v, want 0.42", got)
	}
	if got := effective.FactorWeight.BaseWeights.Value["quality"]; got != 0.2 {
		t.Errorf("effective quality after reload = %v, want 0.2 (SSOT value)", got)
	}
}

// TestApplyFactorWeights_NoOverlayPathWritesNothing pins the disabled-overlay
// behaviour: the weights still apply in memory, nothing is persisted.
func TestApplyFactorWeights_NoOverlayPathWritesNothing(t *testing.T) {
	dir := t.TempDir()
	ssotPath := writeFactorWeightSSOT(t, defaultWeightFixture())

	previousSSOT := config.GetParametersConfigPath()
	previousOverlay := config.GetCalibratedOverlayPath()
	config.SetParametersConfigPath(ssotPath)
	config.SetCalibratedOverlayPath("")
	config.ResetParametersConfig()
	defer func() {
		config.SetParametersConfigPath(previousSSOT)
		config.SetCalibratedOverlayPath(previousOverlay)
		config.ResetParametersConfig()
	}()

	applyFactorWeights(map[FactorType]float64{"momentum": 0.42})

	if got := config.GetParametersConfig().FactorWeight.BaseWeights.Value["momentum"]; got != 0.42 {
		t.Errorf("in-memory momentum = %v, want 0.42", got)
	}
	if _, err := os.Stat(config.CalibrationOverlayPath(dir)); !os.IsNotExist(err) {
		t.Errorf("overlay file created although no overlay path is registered (stat err = %v)", err)
	}
}
