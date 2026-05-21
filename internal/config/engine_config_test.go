package config

import (
	"math"
	"os"
	"testing"
)

// TestEngineConfigLoadValidate validates that engine.json loads and passes Validate().
// This prevents silent fallback to defaultEngineConfig() when base_allocations drift.
func TestEngineConfigLoadValidate(t *testing.T) {
	cfg, err := LoadEngineConfig()
	if err != nil {
		t.Fatalf("LoadEngineConfig() failed: %v\nCheck that configs/engine.json is valid.", err)
	}

	// base_allocations must sum to 1.0 (enforced by Validate)
	ValidateBaseAllocations(t, cfg.SectorRotation.BaseAllocations)
}

// TestDefaultEngineConfigValidate ensures the hardcoded default config passes Validate().
// This is critical because defaultEngineConfig() is the silent fallback when engine.json
// fails validation - if the default itself is broken, leo_satellite and other new industries
// silently disappear from sector rotation.
func TestDefaultEngineConfigValidate(t *testing.T) {
	cfg := defaultEngineConfig()

	if err := cfg.Validate(); err != nil {
		t.Fatalf("defaultEngineConfig().Validate() failed: %v\nThis means the fallback config is also broken.", err)
	}

	ValidateBaseAllocations(t, cfg.SectorRotation.BaseAllocations)
}

// TestDefaultConfigIncludesLEOSatellite ensures leo_satellite exists in the default config.
// Regression test: without this, SectorRotator silently drops leo_satellite allocations.
func TestDefaultConfigIncludesLEOSatellite(t *testing.T) {
	cfg := defaultEngineConfig()

	// Check base_allocations
	if _, ok := cfg.SectorRotation.BaseAllocations["leo_satellite"]; !ok {
		t.Error("defaultEngineConfig().SectorRotation.BaseAllocations missing leo_satellite")
	}

	// Check top_level_industries
	found := false
	for _, id := range cfg.IndustryAnalysis.Classification.TopLevelIndustries {
		if id == "leo_satellite" {
			found = true
			break
		}
	}
	if !found {
		t.Error("defaultEngineConfig().IndustryAnalysis.Classification.TopLevelIndustries missing leo_satellite")
	}
}

// TestEngineJSONBaseAllocationsMatchDefault ensures the JSON config and the Go default
// have identical base_allocations. If they diverge, the system behaves differently
// depending on whether engine.json loads successfully.
func TestEngineJSONBaseAllocationsMatchDefault(t *testing.T) {
	// Only run if engine.json is accessible (not in CI without the file)
	if _, err := os.Stat("configs/engine.json"); os.IsNotExist(err) {
		t.Skip("configs/engine.json not found, skipping comparison test")
	}

	jsonCfg, err := LoadEngineConfig()
	if err != nil {
		t.Skipf("Skipping comparison: engine.json failed to load: %v", err)
	}

	defaultCfg := defaultEngineConfig()

	jsonBA := jsonCfg.SectorRotation.BaseAllocations
	defaultBA := defaultCfg.SectorRotation.BaseAllocations

	// Both must have the same keys
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

	// Check the reverse direction
	for k := range defaultBA {
		if _, ok := jsonBA[k]; !ok {
			t.Errorf("defaultEngineConfig has '%s' but engine.json is missing it", k)
		}
	}
}

// ValidateBaseAllocations is a helper that checks base_allocations sum to 1.0
// and all values are non-negative.
func ValidateBaseAllocations(t *testing.T, ba map[string]float64) {
	t.Helper()

	var total float64
	for sector, alloc := range ba {
		if alloc < 0 {
			t.Errorf("base_allocations['%s'] = %.2f is negative", sector, alloc)
		}
		total += alloc
	}

	if math.Abs(total-1.0) > 0.01 {
		t.Errorf("base_allocations sum = %.2f, want 1.00 (±0.01). Sectors: %v", total, keys(ba))
	}
}

func keys(m map[string]float64) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
