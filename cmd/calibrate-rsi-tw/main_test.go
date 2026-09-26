package main

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
)

func TestPercentile_Empty(t *testing.T) {
	if got := percentile(nil, 0.5); got != 0 {
		t.Fatalf("expected 0 for empty, got %f", got)
	}
}

func TestPercentile_Boundaries(t *testing.T) {
	sorted := []float64{1, 2, 3, 4, 5}
	if got := percentile(sorted, 0); got != 1 {
		t.Fatalf("p=0 should be first element, got %f", got)
	}
	if got := percentile(sorted, 1); got != 5 {
		t.Fatalf("p=1 should be last element, got %f", got)
	}
}

func TestPercentile_Median(t *testing.T) {
	sorted := []float64{1, 2, 3, 4, 5}
	if got := percentile(sorted, 0.5); got != 3 {
		t.Fatalf("p=0.5 of [1..5] should be 3, got %f", got)
	}
}

func TestPercentile_Interpolation(t *testing.T) {
	sorted := []float64{0, 100}
	if got := percentile(sorted, 0.25); got != 25 {
		t.Fatalf("p=0.25 of [0,100] should be 25, got %f", got)
	}
}

func TestMean_Empty(t *testing.T) {
	if got := mean(nil); got != 0 {
		t.Fatalf("expected 0 for empty, got %f", got)
	}
}

func TestMean_Values(t *testing.T) {
	if got := mean([]float64{1, 2, 3, 4, 5}); got != 3 {
		t.Fatalf("expected mean 3, got %f", got)
	}
	if got := mean([]float64{-2, 2}); got != 0 {
		t.Fatalf("expected mean 0, got %f", got)
	}
}

func TestStddev_BelowTwoSamples(t *testing.T) {
	if got := stddev(nil); got != 0 {
		t.Fatalf("expected 0 for empty, got %f", got)
	}
	if got := stddev([]float64{5}); got != 0 {
		t.Fatalf("expected 0 for single sample, got %f", got)
	}
}

func TestStddev_PopulationFormula(t *testing.T) {
	vals := []float64{1, 2, 3, 4, 5}
	expected := math.Sqrt(2)
	if got := stddev(vals); math.Abs(got-expected) > 1e-9 {
		t.Fatalf("expected sqrt(2)=%f (population stddev), got %f", expected, got)
	}
}

func TestComputeDistribution_Empty(t *testing.T) {
	stats := computeDistribution(nil)
	if stats.Samples != nil {
		t.Fatalf("expected nil samples for empty input, got %v", stats.Samples)
	}
	if stats.Mean != 0 {
		t.Fatalf("expected mean 0 for empty input, got %f", stats.Mean)
	}
}

func TestComputeDistribution_FullStats(t *testing.T) {
	stats := computeDistribution([]float64{1, 2, 3, 4, 5})
	if stats.Min != 1 {
		t.Fatalf("expected min 1, got %f", stats.Min)
	}
	if stats.Max != 5 {
		t.Fatalf("expected max 5, got %f", stats.Max)
	}
	if stats.P50 != 3 {
		t.Fatalf("expected P50 3, got %f", stats.P50)
	}
	if math.Abs(stats.Mean-3) > 1e-9 {
		t.Fatalf("expected mean 3, got %f", stats.Mean)
	}
	if len(stats.Samples) != 5 {
		t.Fatalf("expected 5 samples, got %d", len(stats.Samples))
	}
}

// TestApplyCalibration_OverlayModeDoesNotWriteSSOT is the FU-20260926-07
// contract for this CLI: in overlay mode the SSOT file stays byte-identical and
// the calibrated RSI-tw VIX buckets land in the overlay under data/.
func TestApplyCalibration_OverlayModeDoesNotWriteSSOT(t *testing.T) {
	dir := t.TempDir()
	ssotPath := filepath.Join(dir, "configs", "parameters.json")
	if err := os.MkdirAll(filepath.Dir(ssotPath), 0o755); err != nil {
		t.Fatal(err)
	}
	ssotJSON := `{"rsi_tw":{"a4_vix_thresholds":{"value":[15,20,25]},"a4_vix_scores":{"value":[0.5,0.3,0.1]}}}`
	if err := os.WriteFile(ssotPath, []byte(ssotJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	overlayPath := config.CalibrationOverlayPath(dir)

	previous := config.GetCalibratedOverlayPath()
	config.SetCalibratedOverlayPath(overlayPath)
	defer config.SetCalibratedOverlayPath(previous)

	result := calibrationResult{
		SampleSize:      12,
		VIXThresholds:   []float64{12, 17, 22, 27},
		VIXScores:       []float64{0.9, 0.6, 0.3, 0.1},
		VIXDistribution: vixDistributionStats{Mean: 20, StdDev: 3},
	}
	if err := applyCalibration(dir, result, config.WritebackOverlay); err != nil {
		t.Fatalf("applyCalibration(overlay): %v", err)
	}

	after, err := os.ReadFile(ssotPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != ssotJSON {
		t.Errorf("SSOT document rewritten in overlay mode:\nbefore=%s\nafter=%s", ssotJSON, after)
	}

	ov, err := config.LoadCalibrationOverlay(overlayPath)
	if err != nil {
		t.Fatalf("LoadCalibrationOverlay: %v", err)
	}
	if ov == nil {
		t.Fatal("overlay was not written")
	}
	entry, ok := ov.Entries["rsi_tw.a4_vix_thresholds.value"]
	if !ok {
		t.Fatalf("overlay missing the calibrated VIX thresholds: %+v", ov.Entries)
	}
	values, ok := entry.Value.([]any)
	if !ok || len(values) != 4 {
		t.Fatalf("entry value = %#v, want the 4 calibrated thresholds", entry.Value)
	}
	if _, ok := ov.Entries["rsi_tw.a4_vix_scores.value"]; !ok {
		t.Errorf("overlay missing the calibrated VIX scores: %+v", ov.Entries)
	}
}

// TestApplyCalibration_SSOTModeStillWritesFile pins the default for a human on a
// checkout: the parameters file is rewritten.
func TestApplyCalibration_SSOTModeStillWritesFile(t *testing.T) {
	dir := t.TempDir()
	ssotPath := filepath.Join(dir, "configs", "parameters.json")
	if err := os.MkdirAll(filepath.Dir(ssotPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ssotPath, []byte(`{"rsi_tw":{"a4_vix_thresholds":{"value":[15,20,25]}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	previous := config.GetCalibratedOverlayPath()
	config.SetCalibratedOverlayPath("")
	defer config.SetCalibratedOverlayPath(previous)

	result := calibrationResult{SampleSize: 12, VIXThresholds: []float64{12, 17, 22}, VIXScores: []float64{0.9, 0.6, 0.3}}
	if err := applyCalibration(dir, result, config.WritebackSSOT); err != nil {
		t.Fatalf("applyCalibration(ssot): %v", err)
	}

	after, err := os.ReadFile(ssotPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "12") {
		t.Errorf("ssot mode did not persist the calibrated thresholds: %s", after)
	}
}
