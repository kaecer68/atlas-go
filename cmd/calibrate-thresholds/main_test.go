package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	configpkg "github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/industry"
)

func writeThresholdsFixture(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "configs", "parameters.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	doc := `{"industry":{"cycle_thresholds":{"value":{"consumer":{"expansion_revenue_pct":0.08,"expansion_profit_pct":0.1,"recovery_revenue_pct":0.03,"recovery_profit_pct":0.05,"mature_revenue_pct":0.01,"mature_profit_pct":0.02}},"todo":"Calibrate from historical revenue cycles"}}}`
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	return path, dir
}

func thresholdsResults() []industry.CalibrationResult {
	return []industry.CalibrationResult{
		{IndustryID: "semiconductor", SampleSize: 12, P25: 0.05, P50: 0.1, P75: 0.2},
	}
}

// TestWriteConfig_SSOTModeStillWritesFile pins the default (human on a checkout):
// the parameters file is rewritten, so the change is a reviewable git diff.
func TestWriteConfig_SSOTModeStillWritesFile(t *testing.T) {
	path, _ := writeThresholdsFixture(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := writeConfig(path, thresholdsResults(), configpkg.WritebackSSOT); err != nil {
		t.Fatalf("writeConfig(ssot): %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) == string(before) {
		t.Fatal("ssot mode did not rewrite the parameters file")
	}
	var doc map[string]any
	if err := json.Unmarshal(after, &doc); err != nil {
		t.Fatal(err)
	}
	ct := doc["industry"].(map[string]any)["cycle_thresholds"].(map[string]any)
	if ct["source"] != "percentile_based" {
		t.Errorf("source = %v, want percentile_based", ct["source"])
	}
}

// TestWriteConfig_OverlayModeDoesNotWriteSSOT is the FU-20260926-07 contract:
// in overlay mode the SSOT file stays byte-identical and the calibrated
// thresholds land in the overlay under data/.
func TestWriteConfig_OverlayModeDoesNotWriteSSOT(t *testing.T) {
	path, dir := writeThresholdsFixture(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	overlayPath := configpkg.CalibrationOverlayPath(dir)

	previous := configpkg.GetCalibratedOverlayPath()
	configpkg.SetCalibratedOverlayPath(overlayPath)
	defer configpkg.SetCalibratedOverlayPath(previous)

	if err := writeConfig(path, thresholdsResults(), configpkg.WritebackOverlay); err != nil {
		t.Fatalf("writeConfig(overlay): %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("SSOT document rewritten in overlay mode:\nbefore=%s\nafter=%s", before, after)
	}

	ov, err := configpkg.LoadCalibrationOverlay(overlayPath)
	if err != nil {
		t.Fatalf("LoadCalibrationOverlay: %v", err)
	}
	if ov == nil {
		t.Fatal("overlay was not written")
	}
	entry, ok := ov.Entries["industry.cycle_thresholds.value.semiconductor"]
	if !ok {
		t.Fatalf("overlay missing the calibrated industry: %+v", ov.Entries)
	}
	value, ok := entry.Value.(map[string]any)
	if !ok {
		t.Fatalf("entry value = %#v, want an object", entry.Value)
	}
	if _, ok := value["expansion_revenue_pct"]; !ok {
		t.Errorf("calibrated thresholds incomplete: %#v", value)
	}
	if _, ok := ov.Entries["industry.cycle_thresholds.source"]; !ok {
		t.Errorf("overlay missing the provenance entry: %+v", ov.Entries)
	}
}
