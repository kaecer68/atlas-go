package experiment

import (
	"encoding/json"
	"testing"
)

// TestPromptExperimentResult_UnmarshalLegacyStringNotes verifies a legacy
// result file whose "notes" field is a plain string decodes without error
// (audit A2: schema drift caused parse_experiment_file_failed on production).
func TestPromptExperimentResult_UnmarshalLegacyStringNotes(t *testing.T) {
	raw := `{"experiment":{"id":"exp-1","status":"running"},"notes":"replay judge accepted the candidate"}`
	var r PromptExperimentResult
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatalf("unmarshal legacy string notes: %v", err)
	}
	if len(r.Notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(r.Notes))
	}
	if r.Notes[0] != "replay judge accepted the candidate" {
		t.Errorf("unexpected note: %q", r.Notes[0])
	}
}

// TestPromptExperimentResult_UnmarshalStringSliceNotes verifies the current
// []string schema still decodes.
func TestPromptExperimentResult_UnmarshalStringSliceNotes(t *testing.T) {
	raw := `{"experiment":{"id":"exp-2"},"notes":["note a","note b"]}`
	var r PromptExperimentResult
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatalf("unmarshal []string notes: %v", err)
	}
	if len(r.Notes) != 2 || r.Notes[0] != "note a" || r.Notes[1] != "note b" {
		t.Fatalf("unexpected notes: %#v", r.Notes)
	}
}

// TestPromptExperimentResult_NotesRoundTrip verifies marshal → unmarshal keeps
// the []string shape (the field is always emitted as an array, never as a
// legacy string) and survives the custom decoder.
func TestPromptExperimentResult_NotesRoundTrip(t *testing.T) {
	in := PromptExperimentResult{
		Experiment: ExperimentRecord{ID: "exp-3"},
		Notes:      []string{"alpha", "beta"},
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out PromptExperimentResult
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Notes) != 2 || out.Notes[0] != "alpha" || out.Notes[1] != "beta" {
		t.Fatalf("round-trip mismatch: %#v", out.Notes)
	}
}

// TestPromptExperimentResult_UnmarshalOtherFieldsIntact verifies the custom
// decoder preserves sibling fields (the alias-type decode must not drop data).
func TestPromptExperimentResult_UnmarshalOtherFieldsIntact(t *testing.T) {
	raw := `{"experiment":{"id":"exp-5","target_agent_id":"ag-1","status":"accepted","baseline_value":1.5},"candidate_prompt":"cmd/p.txt","notes":"legacy","recorded_at":"2026-06-01T08:00:00Z"}`
	var r PromptExperimentResult
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if r.Experiment.ID != "exp-5" || r.Experiment.TargetAgentID != "ag-1" ||
		r.Experiment.Status != ExperimentAccepted || r.Experiment.BaselineValue != 1.5 {
		t.Fatalf("experiment fields lost: %#v", r.Experiment)
	}
	if r.CandidatePrompt != "cmd/p.txt" {
		t.Fatalf("candidate_prompt lost: %q", r.CandidatePrompt)
	}
	if r.RecordedAt.IsZero() {
		t.Fatal("recorded_at lost")
	}
	if len(r.Notes) != 1 || r.Notes[0] != "legacy" {
		t.Fatalf("notes mismatch: %#v", r.Notes)
	}
}

// TestPromptExperimentResult_UnmarshalNullNotes verifies absent/null notes
// decode cleanly and stay nil.
func TestPromptExperimentResult_UnmarshalNullNotes(t *testing.T) {
	for _, raw := range []string{`{"experiment":{"id":"exp-4"}}`, `{"notes":null}`} {
		var r PromptExperimentResult
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			t.Fatalf("unmarshal %s: %v", raw, err)
		}
		if r.Notes != nil {
			t.Fatalf("expected nil notes for %s, got %#v", raw, r.Notes)
		}
	}
}
