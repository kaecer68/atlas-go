package config

import "testing"

// TestSectorPredictionEnabledDefaultsOff pins item I5 (#1944 Batch 3): the
// shipped default of SECTOR_PREDICTION_ENABLED is false and no deployment
// configuration sets it, so the event-driven sector predictor is never built in
// production (cmd/atlas only wires the macro provider when the flag is true).
//
// Flipping the default requires: a real deployment setting, an update to
// docs/reference/inert-registry.md, and the removal of this test.
func TestSectorPredictionEnabledDefaultsOff(t *testing.T) {
	t.Setenv("SECTOR_PREDICTION_ENABLED", "")
	cfg := Normalize(Load())
	if cfg.SectorPredictionEnabled {
		t.Fatal("SectorPredictionEnabled must default to false")
	}
	if got := cfg.SectorPredictionEnabled; got {
		t.Fatalf("SectorPredictionEnabled = %v, want false", got)
	}
}
