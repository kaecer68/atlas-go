package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
)

func classificationTreeFixture() (*config.ParametersConfig, *config.ParametersConfig, error) {
	base := config.DefaultParametersConfig()
	base.Industry.ClassificationTree.Value.Segments = []config.IndustrySegmentConfig{
		{ID: "semiconductor", Name: "半導體", Level: 1, Weight: 0.4, GeographicExposure: "全球", Cyclicality: "cyclical", TechnologyIntensity: "high", CapitalIntensity: "high"},
		{ID: "shipping", Name: "航運", Level: 1, Weight: 0.6, GeographicExposure: "全球", Cyclicality: "cyclical", TechnologyIntensity: "low", CapitalIntensity: "high"},
	}
	updated, err := config.CloneParametersConfig(base)
	if err != nil {
		return nil, nil, err
	}
	updated.Industry.ClassificationTree.Value.Segments[0].Weight = 0.25
	updated.Industry.ClassificationTree.Value.Segments[1].Weight = 0.75
	updated.Industry.ClassificationTree.Rationale = "Auto-updated from TWSE trade-value proxy"
	return base, updated, nil
}

// TestPersistClassificationTree_OverlayModeDoesNotWriteSSOT is the
// FU-20260926-07 contract for this CLI: overlay mode persists the recomputed tree
// to the overlay under data/ and never writes the SSOT file.
func TestPersistClassificationTree_OverlayModeDoesNotWriteSSOT(t *testing.T) {
	dir := t.TempDir()
	ssotPath := filepath.Join(dir, "configs", "parameters.json")
	if err := os.MkdirAll(filepath.Dir(ssotPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ssotPath, []byte(`{"industry":{"classification_tree":{"value":{"segments":[]}}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(ssotPath)
	if err != nil {
		t.Fatal(err)
	}
	overlayPath := config.CalibrationOverlayPath(dir)

	previousPath := config.GetParametersConfigPath()
	previousOverlay := config.GetCalibratedOverlayPath()
	config.SetParametersConfigPath(ssotPath)
	config.SetCalibratedOverlayPath(overlayPath)
	defer func() {
		config.SetParametersConfigPath(previousPath)
		config.SetCalibratedOverlayPath(previousOverlay)
	}()

	base, updated, err := classificationTreeFixture()
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	if err := persistClassificationTree(updated, base, config.WritebackOverlay); err != nil {
		t.Fatalf("persistClassificationTree(overlay): %v", err)
	}

	after, err := os.ReadFile(ssotPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("SSOT document rewritten in overlay mode:\nbefore=%s\nafter=%s", before, after)
	}

	ov, err := config.LoadCalibrationOverlay(overlayPath)
	if err != nil {
		t.Fatalf("LoadCalibrationOverlay: %v", err)
	}
	if ov == nil {
		t.Fatal("overlay was not written")
	}
	entry, ok := ov.Entries["industry.classification_tree.value.segments"]
	if !ok {
		t.Fatalf("overlay missing the recomputed segments: %+v", ov.Entries)
	}
	segments, ok := entry.Value.([]any)
	if !ok || len(segments) != 2 {
		t.Fatalf("entry value = %#v, want the 2 recomputed segments", entry.Value)
	}
}

// TestPersistClassificationTree_OverlayModeWithoutPathFailsLoudly pins the
// fail-loud rule (no silent fallback to rewriting the SSOT).
func TestPersistClassificationTree_OverlayModeWithoutPathFailsLoudly(t *testing.T) {
	previousOverlay := config.GetCalibratedOverlayPath()
	config.SetCalibratedOverlayPath("")
	defer config.SetCalibratedOverlayPath(previousOverlay)

	base, updated, err := classificationTreeFixture()
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	if err := persistClassificationTree(updated, base, config.WritebackOverlay); err == nil {
		t.Fatal("overlay mode without a registered overlay path = nil error, want error")
	}
}
