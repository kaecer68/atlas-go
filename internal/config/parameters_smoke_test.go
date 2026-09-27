package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestCalibrationEvidence_Smoke verifies the real parameters.json in the repo
// has valid calibration evidence. This is the CI gate that prevents the bug
// where Save() silently drops calibration timestamps.
//
// If this test fails in CI, it means someone ran ParametersConfig.Save() without
// re-running the calibrator, or the calibrator didn't write both timestamp formats.
//
// The document is located through moduleRoot() (go.mod), NOT by probing for
// "configs/parameters.json" (issue #1944 Batch A4). A probe walks up from this
// package's directory, so it used to return the stale duplicate at
// internal/config/configs/parameters.json — version 1.2, updated_at 2026-06-26 —
// while the SSOT is configs/parameters.json, version 1.3, updated_at 2026-07-06.
// The gate therefore asserted against a 300 KB file that nothing reads.
func TestCalibrationEvidence_Smoke(t *testing.T) {
	root, err := moduleRoot()
	if err != nil {
		t.Skipf("module root (go.mod) not found: %v", err)
	}
	path := filepath.Join(root, "configs", "parameters.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("%s not readable: %v", path, err)
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// Print what was actually judged: the resolved path and the document version
	// are the two things that made this gate read the wrong file for months.
	t.Logf("calibration evidence source: %s (%d bytes, version=%v, updated_at=%v)",
		path, len(data), doc["version"], doc["updated_at"])

	industryCfg, ok := doc["industry"].(map[string]any)
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

// TestNoShadowParametersCopy keeps the duplicate document from coming back
// (issue #1944 Batch A4). A second copy of parameters.json inside
// internal/config/ is worse than dead weight: it makes every
// "walk up until configs/parameters.json exists" probe — findRepoRoot and
// friends — resolve to internal/config instead of the repository root, so tests
// and tools silently judge a stale document while the SSOT moves on.
func TestNoShadowParametersCopy(t *testing.T) {
	root, err := moduleRoot()
	if err != nil {
		t.Skipf("module root (go.mod) not found: %v", err)
	}
	shadow := filepath.Join(root, "internal", "config", "configs", "parameters.json")
	if _, statErr := os.Stat(shadow); statErr == nil {
		t.Fatalf("shadow copy exists again: %s\n\nRemove it. It shadows the SSOT (%s) for every walk-up probe (issue #1944 Batch A4).",
			shadow, filepath.Join(root, "configs", "parameters.json"))
	}
}
