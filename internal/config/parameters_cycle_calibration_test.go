package config

import (
	"path/filepath"
	"testing"
)

// TestMergeIndustryDefaults_CycleCalibrationAllZero pins issue #1944 Batch 2
// item I3: configs/parameters.json ships industry.cycle_calibration all-zero
// (min_samples=0, learning_rate=0, clamp 0..0, window_size=0). Before this
// ticket the all-zero block survived the merge, so the calibration tracker kept
// zero outcomes (window_size=0 cleared the buffer on every write) and the
// layer-accuracy loop wired in Batch 1 could never produce metrics.
func TestMergeIndustryDefaults_CycleCalibrationAllZero(t *testing.T) {
	cfg := &ParametersConfig{}
	mergeIndustryDefaults(cfg)

	def := DefaultParametersConfig().Industry.CycleCalibration.Value
	got := cfg.Industry.CycleCalibration.Value
	if got != def {
		t.Fatalf("CycleCalibration = %+v, want defaults %+v", got, def)
	}
	if got.MinSamples <= 0 || got.WindowSize <= 0 {
		t.Fatalf("merged CycleCalibration must have usable MinSamples/WindowSize, got %+v", got)
	}
}

// TestMergeIndustryDefaults_CycleCalibrationPartialOverride proves a
// deliberately hand-tuned block is not overwritten.
func TestMergeIndustryDefaults_CycleCalibrationPartialOverride(t *testing.T) {
	cfg := &ParametersConfig{}
	cfg.Industry.CycleCalibration.Value.MinSamples = 25
	cfg.Industry.CycleCalibration.Value.WindowSize = 60
	mergeIndustryDefaults(cfg)

	got := cfg.Industry.CycleCalibration.Value
	if got.MinSamples != 25 || got.WindowSize != 60 {
		t.Fatalf("hand-tuned CycleCalibration overwritten: %+v", got)
	}
}

// TestShippedConfigCycleCalibrationIsUsable loads the real
// configs/parameters.json and asserts the production values the calibration
// loop depends on. This is the runtime evidence for the I3 fix: the shipped
// all-zero block is merged up to the code defaults on load.
func TestShippedConfigCycleCalibrationIsUsable(t *testing.T) {
	path := filepath.Join("..", "..", "configs", "parameters.json")
	cfg, err := LoadParametersConfig(path)
	if err != nil {
		t.Fatalf("load shipped parameters: %v", err)
	}
	got := cfg.Industry.CycleCalibration.Value
	if got.WindowSize <= 0 {
		t.Fatalf("shipped config CycleCalibration.WindowSize = %d, want > 0 (zero clears the outcome window)", got.WindowSize)
	}
	if got.MinSamples <= 0 {
		t.Fatalf("shipped config CycleCalibration.MinSamples = %d, want > 0", got.MinSamples)
	}
	if got.WeightClampMax <= got.WeightClampMin {
		t.Fatalf("shipped config clamp window degenerate: [%v, %v]", got.WeightClampMin, got.WeightClampMax)
	}
	if got.LearningRate <= 0 {
		t.Fatalf("shipped config CycleCalibration.LearningRate = %v, want > 0", got.LearningRate)
	}
}
