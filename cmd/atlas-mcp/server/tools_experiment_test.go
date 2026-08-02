package server

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestHandleExperimentDiff_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleExperimentDiff(context.Background(), nil, experimentIDInput{ExperimentID: "exp-42"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/experiment/diff" {
		t.Fatalf("path=%s", rec.path)
	}
	if got := rec.query.Get("experiment_id"); got != "exp-42" {
		t.Fatalf("experiment_id query not propagated, got=%q", got)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleExperimentDiff_EmptyID(t *testing.T) {
	s, _, done := newTestHarness(t)
	defer done()

	_, _, err := s.handleExperimentDiff(context.Background(), nil, experimentIDInput{
		ExperimentID: "",
	})
	if err == nil {
		t.Fatal("expected error for empty experiment_id")
	}
	if !strings.Contains(err.Error(), "experiment_id is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleExperimentHistory_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleExperimentHistory(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/experiment/history" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

// --- experiment_judge ----------------------------------------------------------

func TestHandleExperimentJudge_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"verdict":"accept","confidence":0.85}`)

	_, out, err := s.handleExperimentJudge(context.Background(), nil, ExperimentJudgeInput{
		ExperimentID: "exp-42",
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/experiment/judge" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
	if out.Result["verdict"] != "accept" {
		t.Fatalf("verdict=%v", out.Result["verdict"])
	}
}

func TestHandleExperimentJudge_EmptyID(t *testing.T) {
	s, _, done := newTestHarness(t)
	defer done()

	_, _, err := s.handleExperimentJudge(context.Background(), nil, ExperimentJudgeInput{
		ExperimentID: "",
	})
	if err == nil {
		t.Fatal("expected error for empty experiment_id")
	}
	if !strings.Contains(err.Error(), "experiment_id is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleExperimentJudge_ServerError(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`not json`)
	_, _, err := s.handleExperimentJudge(context.Background(), nil, ExperimentJudgeInput{
		ExperimentID: "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error for malformed JSON response")
	}
	if rec.path != "/api/experiment/judge" {
		t.Fatalf("path=%s", rec.path)
	}
}

func TestHandleExperimentJudge_AuditRoundTrip(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"verdict":"reject","confidence":0.3}`)

	_, _, err := s.handleExperimentJudge(context.Background(), nil, ExperimentJudgeInput{
		ExperimentID: "exp-99",
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	// Read the audit log and verify it contains the experiment_judge entry.
	auditBytes, err := os.ReadFile(s.cfg.AuditLogPath)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if len(auditBytes) == 0 {
		t.Fatal("audit log is empty")
	}
	if !strings.Contains(string(auditBytes), `"tool":"experiment_judge"`) {
		t.Fatal("audit log missing experiment_judge entry")
	}
	if !strings.Contains(string(auditBytes), `"experiment_id"`) {
		t.Fatal("audit log missing experiment_id arg key")
	}
}
