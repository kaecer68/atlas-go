package config

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestEngineConfigLoadValidate(t *testing.T) {
	setProjectRoot(t)

	cfg, err := LoadEngineConfig()
	if err != nil {
		t.Fatalf("LoadEngineConfig() failed: %v\nCheck that configs/engine.json is valid.", err)
	}

	validateBaseAllocations(t, cfg.SectorRotation.BaseAllocations)
}

func TestDefaultEngineConfigValidate(t *testing.T) {
	cfg := defaultEngineConfig()

	if err := cfg.Validate(); err != nil {
		t.Fatalf("defaultEngineConfig().Validate() failed: %v", err)
	}

	validateBaseAllocations(t, cfg.SectorRotation.BaseAllocations)
}

func TestDefaultConfigIncludesLEOSatellite(t *testing.T) {
	cfg := defaultEngineConfig()

	if _, ok := cfg.SectorRotation.BaseAllocations["leo_satellite"]; !ok {
		t.Error("defaultEngineConfig().BaseAllocations missing leo_satellite")
	}

	found := false
	for _, id := range cfg.IndustryAnalysis.Classification.TopLevelIndustries {
		if id == "leo_satellite" {
			found = true
			break
		}
	}
	if !found {
		t.Error("defaultEngineConfig().TopLevelIndustries missing leo_satellite")
	}
}

func TestEngineJSONBaseAllocationsMatchDefault(t *testing.T) {
	setProjectRoot(t)

	jsonCfg, err := LoadEngineConfig()
	if err != nil {
		t.Skipf("Skipping comparison: engine.json failed to load: %v", err)
	}

	defaultCfg := defaultEngineConfig()
	jsonBA := jsonCfg.SectorRotation.BaseAllocations
	defaultBA := defaultCfg.SectorRotation.BaseAllocations

	if len(jsonBA) != len(defaultBA) {
		t.Errorf("base_allocations count mismatch: engine.json=%d, default=%d", len(jsonBA), len(defaultBA))
	}

	for k, jsonVal := range jsonBA {
		defaultVal, ok := defaultBA[k]
		if !ok {
			t.Errorf("engine.json has '%s' (%.2f) but defaultEngineConfig is missing it", k, jsonVal)
			continue
		}
		if math.Abs(jsonVal-defaultVal) > 0.001 {
			t.Errorf("base_allocations['%s'] mismatch: engine.json=%.2f, default=%.2f (diff=%.2f)",
				k, jsonVal, defaultVal, jsonVal-defaultVal)
		}
	}

	for k := range defaultBA {
		if _, ok := jsonBA[k]; !ok {
			t.Errorf("defaultEngineConfig has '%s' but engine.json is missing it", k)
		}
	}
}

func setProjectRoot(t *testing.T) {
	t.Helper()

	// Already at project root
	if _, err := os.Stat("configs/engine.json"); err == nil {
		return
	}

	// Try from package directory
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := filepath.Clean(filepath.Join(wd, "../.."))
	if _, err := os.Stat(filepath.Join(root, "configs/engine.json")); err != nil {
		t.Skipf("cannot find project root from %s: %v", wd, err)
		return
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir to project root (%s): %v", root, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})
}

func validateBaseAllocations(t *testing.T, ba map[string]float64) {
	t.Helper()

	var total float64
	for sector, alloc := range ba {
		if alloc < 0 {
			t.Errorf("base_allocations['%s'] = %.2f is negative", sector, alloc)
		}
		total += alloc
	}

	if math.Abs(total-1.0) > 0.01 {
		t.Errorf("base_allocations sum = %.2f, want 1.00 (±0.01)", total)
	}
}
