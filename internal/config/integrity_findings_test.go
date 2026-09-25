package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// Issue #1944 Batch 3, item I31: cmd/calibration-validate reported OK=false while
// the nightly job stayed green (the workflow runs it with `set +e` and a non-final
// `cat`, and the Slack step is skipped when SLACK_WEBHOOK_URL is unset). This batch
// cannot touch .github/workflows, so the fix inside the code lane is to make the
// verdict self-describing: every failure now carries a stable code, a severity and
// the affected segment. These tests pin the codes and the shipped-config shape.

func writeIntegrityParams(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "parameters.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write params: %v", err)
	}
	return path
}

func segmentJSON(id, parent string, level int, reps []string) map[string]any {
	seg := map[string]any{
		"id":    id,
		"name":  id,
		"level": level,
	}
	if parent != "" {
		seg["parent_id"] = parent
	}
	if len(reps) > 0 {
		seg["representative_stocks"] = reps
	}
	return seg
}

func integrityParamsJSON(t *testing.T, updatedAt time.Time, segments []map[string]any) string {
	t.Helper()
	doc := map[string]any{
		"updated_at": updatedAt.Format(time.RFC3339),
		"industry": map[string]any{
			"classification_tree": map[string]any{
				"value": map[string]any{"segments": segments},
			},
		},
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return string(out)
}

func findingCodes(res *CalibrationValidationResult) []string {
	out := make([]string, 0, len(res.Findings))
	for _, f := range res.Findings {
		out = append(out, string(f.Code))
	}
	return out
}

func requireCode(t *testing.T, res *CalibrationValidationResult, code CalibrationFindingCode, segment string) {
	t.Helper()
	for _, f := range res.Findings {
		if f.Code != code {
			continue
		}
		if segment != "" && f.Segment != segment {
			continue
		}
		if f.Severity != CalibrationSeverityError {
			t.Fatalf("finding %s severity = %q, want %q", f.Code, f.Severity, CalibrationSeverityError)
		}
		return
	}
	t.Fatalf("missing finding %s (segment %q); got codes %v", code, segment, findingCodes(res))
}

func TestValidateCalibration_FindingsAreClassified(t *testing.T) {
	now := time.Now()

	t.Run("missing_file", func(t *testing.T) {
		res, err := ValidateCalibration(filepath.Join(t.TempDir(), "absent.json"), 48*time.Hour)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		requireCode(t, res, CalibrationFindingParamsStatFailed, "")
	})

	t.Run("invalid_json", func(t *testing.T) {
		path := writeIntegrityParams(t, "{not json")
		res, err := ValidateCalibration(path, 48*time.Hour)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		requireCode(t, res, CalibrationFindingParamsInvalidJSON, "")
	})

	t.Run("zero_updated_at", func(t *testing.T) {
		path := writeIntegrityParams(t, integrityParamsJSON(t, time.Time{},
			[]map[string]any{segmentJSON("semiconductor", "", 1, []string{"2330.TW"})}))
		res, err := ValidateCalibration(path, 48*time.Hour)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		requireCode(t, res, CalibrationFindingUpdatedAtZero, "")
	})

	t.Run("stale_updated_at", func(t *testing.T) {
		path := writeIntegrityParams(t, integrityParamsJSON(t, now.Add(-30*24*time.Hour),
			[]map[string]any{segmentJSON("semiconductor", "", 1, []string{"2330.TW"})}))
		res, err := ValidateCalibration(path, 48*time.Hour)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		requireCode(t, res, CalibrationFindingUpdatedAtStale, "")
	})

	t.Run("empty_segments", func(t *testing.T) {
		path := writeIntegrityParams(t, integrityParamsJSON(t, now, nil))
		res, err := ValidateCalibration(path, 48*time.Hour)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		requireCode(t, res, CalibrationFindingSegmentsEmpty, "")
	})

	t.Run("structural_segment_defects", func(t *testing.T) {
		segments := []map[string]any{
			segmentJSON("", "", 1, []string{"2330.TW"}),                       // empty L1 id
			segmentJSON("semiconductor", "", 1, []string{"2330.TW"}),          // ok
			segmentJSON("no_reps_l1", "", 1, nil),                             // L1 without reps
			segmentJSON("pcb", "semiconductor", 2, nil),                       // L2 without reps
			segmentJSON("orphan", "", 0, nil),                                 // level 0 → ignored
			segmentJSON("no_parent", "semiconductor", 2, []string{"3037.TW"}), // ok (has parent+reps)
		}
		// Force an L2 with empty parent and one with an unknown parent.
		segments = append(segments,
			map[string]any{"id": "empty_parent", "level": 2, "representative_stocks": []string{"1"}},
			map[string]any{"id": "unknown_parent", "level": 2, "parent_id": "ghost", "representative_stocks": []string{"2"}},
		)
		path := writeIntegrityParams(t, integrityParamsJSON(t, now, segments))
		res, err := ValidateCalibration(path, 48*time.Hour)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		requireCode(t, res, CalibrationFindingL1EmptyID, "")
		requireCode(t, res, CalibrationFindingL1NoRepresentatives, "no_reps_l1")
		requireCode(t, res, CalibrationFindingL2NoRepresentatives, "pcb")
		requireCode(t, res, CalibrationFindingL2EmptyParentID, "empty_parent")
		requireCode(t, res, CalibrationFindingL2UnknownParentID, "unknown_parent")
		if len(res.Issues) != len(res.Findings) {
			t.Fatalf("Issues (%d) must mirror Findings (%d) for backward compatibility", len(res.Issues), len(res.Findings))
		}
		for i, f := range res.Findings {
			if res.Issues[i] != f.Message {
				t.Fatalf("Issues[%d] = %q, want message %q", i, res.Issues[i], f.Message)
			}
		}
	})

	t.Run("clean_tree_passes", func(t *testing.T) {
		segments := []map[string]any{
			segmentJSON("electronics", "", 1, []string{"2317.TW"}),
			segmentJSON("pcb", "electronics", 2, []string{"3037.TW"}),
		}
		path := writeIntegrityParams(t, integrityParamsJSON(t, now, segments))
		res, err := ValidateCalibration(path, 48*time.Hour)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.OK {
			t.Fatalf("expected OK=true for a fully populated tree, findings=%v", findingCodes(res))
		}
		if len(res.Findings) != 0 {
			t.Fatalf("expected no findings, got %v", findingCodes(res))
		}
	})
}

// TestShippedConfigIntegrityFindingsAreClassified pins the exact failure classes of
// the shipped config. It is intentionally an exact-set assertion: refreshing
// updated_at, adding representatives or growing the taxonomy must fail here, so the
// inert-registry entry (I31) is updated in the same commit instead of drifting.
func TestShippedConfigIntegrityFindingsAreClassified(t *testing.T) {
	path := filepath.Join("..", "..", "configs", "parameters.json")
	res, err := ValidateCalibration(path, 48*time.Hour)
	if err != nil {
		t.Fatalf("ValidateCalibration: %v", err)
	}
	if res.OK {
		t.Fatalf("shipped config unexpectedly passes the nightly integrity check — update docs/reference/inert-registry.md (I31), which documents why it cannot pass in a fresh checkout")
	}

	want := []string{
		string(CalibrationFindingUpdatedAtStale),
		string(CalibrationFindingL1NoRepresentatives),
		string(CalibrationFindingL1NoRepresentatives),
		string(CalibrationFindingL1NoRepresentatives),
		string(CalibrationFindingL2NoRepresentatives),
		string(CalibrationFindingL1NoRepresentatives),
		string(CalibrationFindingL1NoRepresentatives),
		string(CalibrationFindingL2NoRepresentatives),
	}
	got := findingCodes(res)
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(want, got) {
		t.Fatalf("shipped-config finding codes changed:\n got %v\nwant %v", got, want)
	}

	// The five zero-weight strategy/asset-class buckets are the L1 entries without
	// representatives; pcb and thermal are the two L2 ones.
	segments := map[string]int{}
	for _, f := range res.Findings {
		if f.Code == CalibrationFindingL1NoRepresentatives || f.Code == CalibrationFindingL2NoRepresentatives {
			segments[f.Segment]++
		}
	}
	for _, id := range []string{"defensive", "etf_rotation", "high_dividend", "small_cap", "tech", "pcb", "thermal"} {
		if segments[id] != 1 {
			t.Fatalf("segment %q: got %d no-representative findings, want 1 (all=%v)", id, segments[id], segments)
		}
	}
}
