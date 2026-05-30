package config

import (
	"encoding/json"
	"os"
	"testing"
)

// TestCalibrationEvidence_Smoke verifies the real parameters.json in the repo
// has valid calibration evidence. This is the CI gate that prevents the bug
// where Save() silently drops calibration timestamps.
//
// If this test fails in CI, it means someone ran ParametersConfig.Save() without
// re-running the calibrator, or the calibrator didn't write both timestamp formats.
func TestCalibrationEvidence_Smoke(t *testing.T) {
	// Test runs from internal/config/ — navigate to repo root.
	path, err := findRepoRoot("configs/parameters.json")
	if err != nil {
		t.Skipf("configs/parameters.json not found: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("configs/parameters.json not readable: %v", err)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	industryCfg, ok := config["industry"].(map[string]any)
	if !ok {
		t.Fatal("industry section missing")
	}

	sp, ok := industryCfg["seasonal_patterns"].(map[string]any)
	if !ok {
		t.Fatal("industry.seasonal_patterns section missing")
	}

	// Check citation evidence quality — if it says "high", we MUST have calibration data
	cite, ok := sp["citation"].(map[string]any)
	if !ok {
		t.Skip("no citation in seasonal_patterns — skipping smoke check")
	}

	eq, _ := cite["evidence_quality"].(string)
	switch eq {
	case "high", "medium":
		// Evidence claims calibration was done — timestamp must exist
		hasTimestamp := false
		if ts, ok := sp["last_calibrated"]; ok && ts != nil && ts != "" {
			hasTimestamp = true
			t.Logf("last_calibrated found: %v", ts)
		}
		if ts, ok := sp["calibration_timestamp"]; ok && ts != nil && ts != "" {
			hasTimestamp = true
			t.Logf("calibration_timestamp found: %v", ts)
		}

		if !hasTimestamp {
			t.Error(`CALIBRATION EVIDENCE LOST:
industry.seasonal_patterns.citation.evidence_quality is "` + eq + `" but no
calibration timestamp exists (neither last_calibrated nor calibration_timestamp).

This means ParametersConfig.Save() overwrote the calibrator's timestamps.
Fix: re-run 'go run ./cmd/calibrate-seasonal --update --update-threshold 0'
`)
		} else {
			t.Logf("calibration evidence intact: evidence_quality=%s, timestamp present", eq)
		}

	case "low", "heuristic", "":
		// Not calibrated — no timestamp needed, but shouldn't claim otherwise
		calMethod, hasMethod := sp["calibration_method"].(string)
		if hasMethod && calMethod != "" {
			t.Errorf(`INCONSISTENT STATE:
industry.seasonal_patterns.citation.evidence_quality is %q but
calibration_method is %q — evidence_quality should be "high" or "medium"
after calibration.`, eq, calMethod)
		}
	}
}

// findRepoRoot walks up from the current directory until it finds
// the given relative path, or returns an error after 10 levels.
func findRepoRoot(rel string) (string, error) {
	dir := "."
	for range 10 {
		abs := dir + "/" + rel
		if _, err := os.Stat(abs); err == nil {
			return abs, nil
		}
		dir = dir + "/.."
	}
	return "", os.ErrNotExist
}
