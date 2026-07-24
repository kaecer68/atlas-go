package calibration

import (
	"os"
	"path/filepath"
	"testing"
)

func writeStressWeightsFile(dir, content string) error {
	path := filepath.Join(dir, "configs", "stress_index_weights.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func TestIncrementalValidator_CompareModels_SucceedsWhenCandidateImprovesDirection(t *testing.T) {
	validator := IncrementalValidator{}
	baseline := []CalibrationRecord{
		{Outflow: 10, OutflowTarget: 10},
		{Outflow: -8, OutflowTarget: 10},
		{Outflow: 6, OutflowTarget: 6},
	}
	candidate := []CalibrationRecord{
		{Outflow: 11, OutflowTarget: 10},
		{Outflow: 9, OutflowTarget: 10},
		{Outflow: 7, OutflowTarget: 6},
	}

	result := validator.CompareModels(baseline, candidate)
	if !result.CandidateImproved {
		t.Fatalf("expected candidate to improve, got %+v", result)
	}
	if !validator.ShouldAddFactor(result) {
		t.Fatalf("expected factor addition to be recommended, got %+v", result)
	}
	if result.BaselineAccuracy >= result.CandidateAccuracy {
		t.Fatalf("expected candidate accuracy to exceed baseline, got baseline=%.3f candidate=%.3f", result.BaselineAccuracy, result.CandidateAccuracy)
	}
}

func TestIncrementalValidator_CompareModels_RejectsWhenCandidateDoesNotImprove(t *testing.T) {
	validator := IncrementalValidator{}
	baseline := []CalibrationRecord{
		{Outflow: 10, OutflowTarget: 10},
		{Outflow: 8, OutflowTarget: 10},
		{Outflow: 6, OutflowTarget: 6},
	}
	candidate := []CalibrationRecord{
		{Outflow: 9, OutflowTarget: 10},
		{Outflow: -2, OutflowTarget: 10},
		{Outflow: 5, OutflowTarget: 6},
	}

	result := validator.CompareModels(baseline, candidate)
	if result.CandidateImproved {
		t.Fatalf("expected candidate not to improve, got %+v", result)
	}
	if validator.ShouldAddFactor(result) {
		t.Fatalf("expected factor addition to be rejected, got %+v", result)
	}
}

func TestIncrementalValidator_ComputeDieboldMariano_ReturnsStatistic(t *testing.T) {
	validator := IncrementalValidator{}
	dm := validator.ComputeDieboldMariano([]float64{1, 2, 3}, []float64{0.5, 1.5, 2.5})
	if dm.N != 3 {
		t.Fatalf("expected n=3, got %+v", dm)
	}
	if dm.Stat == 0 {
		t.Fatalf("expected non-zero DM statistic, got %+v", dm)
	}
}

func TestLoadWeightsConfigAcceptsEightFactorSum(t *testing.T) {
	dir := t.TempDir()
	if err := writeStressWeightsFile(dir, `{
		"scaling":  {"dxy":5,"us10y":2,"foreign_flow":10,"vix":2.5,"jpy":10,"geopolitical":1,"oil":2,"gold":2},
		"weights":  {"dxy":0.13,"us10y":0.18,"foreign_flow":0.22,"vix":0.13,"jpy":0.08,"geopolitical":0.13,"oil":0.07,"gold":0.06},
		"thresholds":{"crisis":70,"high":50,"alert":30}
	}`); err != nil {
		t.Fatal(err)
	}
	cfg := LoadWeightsConfig(dir)
	if cfg == nil {
		t.Fatal("expected config to load for valid 8-factor sum")
	}
}
