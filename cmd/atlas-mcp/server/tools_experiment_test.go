package server

import (
	"context"
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
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
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
