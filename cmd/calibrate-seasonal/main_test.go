package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/industry"
)

// writeTempParameters builds a minimal parameters.json holding four seasonal
// patterns and returns its path. Only the shape SummarizeCalibrationHealth and
// updateParametersFileAt read is populated.
func writeTempParameters(t *testing.T) string {
	t.Helper()
	patterns := []map[string]any{
		{"id": "p_calibrated", "adjustment_factor": 1.1, "historical_accuracy": 0.5, "avg_market_return": 0.02},
		{"id": "p_insufficient", "adjustment_factor": 1.0, "historical_accuracy": 0.5, "avg_market_return": 0.02},
		{"id": "p_no_observations", "adjustment_factor": 1.0, "historical_accuracy": 0.5, "avg_market_return": 0.02},
		{"id": "p_out_of_range", "adjustment_factor": 1.0, "historical_accuracy": 0.5, "avg_market_return": 0.02},
	}
	doc := map[string]any{
		"industry": map[string]any{
			"seasonal_patterns": map[string]any{
				"value": patterns,
			},
		},
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "parameters.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func readPatterns(t *testing.T, path string) map[string]map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal back: %v", err)
	}
	sp := doc["industry"].(map[string]any)["seasonal_patterns"].(map[string]any)
	out := map[string]map[string]any{}
	for _, item := range sp["value"].([]any) {
		p := item.(map[string]any)
		out[p["id"].(string)] = p
	}
	return out
}

// TestUpdateParametersFileAt_WritesPerPatternCalibrationEvidence is the
// producer-side half of #1944 item I17: before this, nothing in the repo ever
// wrote calibration_observations / calibration_verdict / calibration_timestamp,
// so the seasonal health block could only report "unknown / no_observations".
func TestUpdateParametersFileAt_WritesPerPatternCalibrationEvidence(t *testing.T) {
	path := writeTempParameters(t)

	results := []industry.SeasonalCalibration{
		{PatternID: "p_calibrated", ObservationCount: 7, ObservedAccuracy: 0.71, ObservedAvgReturn: 0.032, ObservedAdjustment: 1.4},
		{PatternID: "p_insufficient", ObservationCount: 2, ObservedAccuracy: 0.4, ObservedAvgReturn: 0.01, ObservedAdjustment: 0.9},
		{PatternID: "p_no_observations", ObservationCount: 0},
		{PatternID: "p_out_of_range", ObservationCount: 6, ObservedAccuracy: 0.6, ObservedAvgReturn: 0.02, ObservedAdjustment: 9.0},
	}

	// threshold 3 → p_insufficient (2 < 3) must be skipped, p_out_of_range is
	// rejected by the range guard, only p_calibrated may be written back.
	if err := updateParametersFileAt(path, results, 3, "/tmp/replay.csv", config.WritebackSSOT); err != nil {
		t.Fatalf("updateParametersFileAt: %v", err)
	}

	patterns := readPatterns(t, path)

	// 1) calibrated pattern: verdict + applied values written.
	got := patterns["p_calibrated"]
	if got["calibration_verdict"] != industry.SeasonalVerdictCalibrated {
		t.Errorf("p_calibrated verdict = %v, want %s", got["calibration_verdict"], industry.SeasonalVerdictCalibrated)
	}
	if got["calibration_observations"] != float64(7) {
		t.Errorf("p_calibrated observations = %v, want 7", got["calibration_observations"])
	}
	if got["calibration_timestamp"] == nil || got["calibration_timestamp"] == "" {
		t.Error("p_calibrated calibration_timestamp missing")
	}
	if got["adjustment_factor"] != 1.4 {
		t.Errorf("p_calibrated adjustment_factor = %v, want 1.4 (applied)", got["adjustment_factor"])
	}

	// 2) below-threshold pattern: evidence written, values NOT applied.
	got = patterns["p_insufficient"]
	if got["calibration_verdict"] != industry.SeasonalVerdictInsufficientSamples {
		t.Errorf("p_insufficient verdict = %v, want %s", got["calibration_verdict"], industry.SeasonalVerdictInsufficientSamples)
	}
	if got["calibration_observations"] != float64(2) {
		t.Errorf("p_insufficient observations = %v, want 2", got["calibration_observations"])
	}
	if got["adjustment_factor"] != 1.0 {
		t.Errorf("p_insufficient adjustment_factor = %v, want 1.0 (unchanged)", got["adjustment_factor"])
	}

	// 3) zero-observation pattern: explicit no_observations, values unchanged.
	got = patterns["p_no_observations"]
	if got["calibration_verdict"] != industry.SeasonalVerdictNoObservations {
		t.Errorf("p_no_observations verdict = %v, want %s", got["calibration_verdict"], industry.SeasonalVerdictNoObservations)
	}
	if got["adjustment_factor"] != 1.0 {
		t.Errorf("p_no_observations adjustment_factor = %v, want 1.0 (unchanged)", got["adjustment_factor"])
	}

	// 4) range-guard rejection: verdict says why, values unchanged.
	got = patterns["p_out_of_range"]
	if got["calibration_verdict"] != industry.SeasonalVerdictOutOfRange {
		t.Errorf("p_out_of_range verdict = %v, want %s", got["calibration_verdict"], industry.SeasonalVerdictOutOfRange)
	}
	if got["adjustment_factor"] != 1.0 {
		t.Errorf("p_out_of_range adjustment_factor = %v, want 1.0 (guard rejected it)", got["adjustment_factor"])
	}
}

// TestUpdateParametersFileAt_ClosesTheEvidenceLoop proves the producer and the
// consumer are actually connected: after one write-back the seasonal health
// summary reports measured evidence instead of "unknown / no_observations".
func TestUpdateParametersFileAt_ClosesTheEvidenceLoop(t *testing.T) {
	path := writeTempParameters(t)

	before, err := industry.SummarizeCalibrationHealth(path)
	if err != nil {
		t.Fatalf("SummarizeCalibrationHealth (before): %v", err)
	}
	if before.TotalObservations != 0 || before.CalibrationEvidence != industry.CalibrationEvidenceNone {
		t.Fatalf("fixture should start with no calibration evidence, got total=%d evidence=%s",
			before.TotalObservations, before.CalibrationEvidence)
	}

	results := []industry.SeasonalCalibration{
		{PatternID: "p_calibrated", ObservationCount: 7, ObservedAccuracy: 0.71, ObservedAvgReturn: 0.032, ObservedAdjustment: 1.4},
		{PatternID: "p_insufficient", ObservationCount: 2},
		{PatternID: "p_no_observations", ObservationCount: 0},
		{PatternID: "p_out_of_range", ObservationCount: 6, ObservedAdjustment: 9.0},
	}
	if err := updateParametersFileAt(path, results, 3, "/tmp/replay.csv", config.WritebackSSOT); err != nil {
		t.Fatalf("updateParametersFileAt: %v", err)
	}

	after, err := industry.SummarizeCalibrationHealth(path)
	if err != nil {
		t.Fatalf("SummarizeCalibrationHealth (after): %v", err)
	}
	if after.TotalObservations != 15 {
		t.Errorf("TotalObservations = %d, want 15 (7+2+0+6)", after.TotalObservations)
	}
	if after.CalibrationEvidence != industry.CalibrationEvidencePresent {
		t.Errorf("CalibrationEvidence = %s, want %s", after.CalibrationEvidence, industry.CalibrationEvidencePresent)
	}
	if after.LastCalibratedAt == nil {
		t.Error("LastCalibratedAt is nil — per-pattern calibration_timestamp was not written")
	}
	if after.VerdictCounts[industry.SeasonalVerdictCalibrated] != 1 {
		t.Errorf("VerdictCounts[calibrated] = %d, want 1", after.VerdictCounts[industry.SeasonalVerdictCalibrated])
	}
	if after.VerdictCounts[industry.SeasonalVerdictNoObservations] != 1 {
		t.Errorf("VerdictCounts[no_observations] = %d, want 1", after.VerdictCounts[industry.SeasonalVerdictNoObservations])
	}
}

// TestUpdateParametersFileAt_OverlayModeDoesNotWriteSSOT is the FU-20260926-07
// contract for this CLI: the daemon spawns it *inside the container*, where
// configs/ is not bind-mounted. In overlay mode the SSOT file must be left
// byte-identical and the calibrated leaves must land in the overlay under data/.
func TestUpdateParametersFileAt_OverlayModeDoesNotWriteSSOT(t *testing.T) {
	path := writeTempParameters(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	overlayPath := filepath.Join(t.TempDir(), "data", "state", "parameters.calibrated.json")

	previous := config.GetCalibratedOverlayPath()
	config.SetCalibratedOverlayPath(overlayPath)
	defer config.SetCalibratedOverlayPath(previous)

	results := []industry.SeasonalCalibration{
		{PatternID: "p_calibrated", ObservationCount: 7, ObservedAccuracy: 0.71, ObservedAvgReturn: 0.032, ObservedAdjustment: 1.4},
	}
	if err := updateParametersFileAt(path, results, 3, "/tmp/replay.csv", config.WritebackOverlay); err != nil {
		t.Fatalf("updateParametersFileAt(overlay): %v", err)
	}

	// (1) SSOT untouched.
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("SSOT document rewritten in overlay mode:\nbefore=%s\nafter=%s", before, after)
	}

	// (2) The calibrated array landed in the overlay.
	ov, err := config.LoadCalibrationOverlay(overlayPath)
	if err != nil {
		t.Fatalf("LoadCalibrationOverlay: %v", err)
	}
	if ov == nil {
		t.Fatal("overlay was not written")
	}
	entry, ok := ov.Entries["industry.seasonal_patterns.value"]
	if !ok {
		t.Fatalf("overlay has no seasonal_patterns entry: %+v", ov.Entries)
	}
	patterns, ok := entry.Value.([]any)
	if !ok {
		t.Fatalf("entry value = %#v, want an array", entry.Value)
	}
	found := false
	for _, item := range patterns {
		pattern, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if pattern["id"] == "p_calibrated" && pattern["adjustment_factor"] == 1.4 {
			found = true
		}
	}
	if !found {
		t.Errorf("calibrated pattern not present in the overlay value: %#v", patterns)
	}

	// (3) The entry records the SSOT it was diffed against, so a later reviewed
	// edit of the SSOT invalidates it instead of being overridden.
	if entry.SSOT == nil || !entry.SSOT.Present {
		t.Errorf("entry baseline = %+v, want the SSOT array", entry.SSOT)
	}
	if entry.Method != "calibrate_seasonal" {
		t.Errorf("entry method = %q, want calibrate_seasonal", entry.Method)
	}
	// (Round-tripping the overlay through the loader is covered by the config
	// package tests; this fixture is deliberately too small to pass the full
	// parameters validation.)
}

// TestUpdateParametersFileAt_OverlayModeWithoutPathFailsLoudly pins the
// fail-loud rule: asking for overlay mode without a registered overlay path must
// not silently fall back to rewriting the SSOT.
func TestUpdateParametersFileAt_OverlayModeWithoutPathFailsLoudly(t *testing.T) {
	path := writeTempParameters(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	previous := config.GetCalibratedOverlayPath()
	config.SetCalibratedOverlayPath("")
	defer config.SetCalibratedOverlayPath(previous)

	results := []industry.SeasonalCalibration{
		{PatternID: "p_calibrated", ObservationCount: 7, ObservedAccuracy: 0.71, ObservedAvgReturn: 0.032, ObservedAdjustment: 1.4},
	}
	if err := updateParametersFileAt(path, results, 3, "/tmp/replay.csv", config.WritebackOverlay); err == nil {
		t.Fatal("overlay mode without a registered overlay path = nil error, want error")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Error("SSOT document was rewritten even though overlay mode failed")
	}
}
