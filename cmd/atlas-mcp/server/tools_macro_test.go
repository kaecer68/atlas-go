package server

import (
	"context"
	"testing"
)

func TestHandleMacroGetSnapshotLatest_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"status":"healthy","chips":42}`)
	_, out, err := s.handleMacroGetSnapshotLatest(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/macro/snapshot/latest" {
		t.Fatalf("path=%s", rec.path)
	}
	if len(rec.query) != 0 {
		t.Fatalf("unexpected query: %v", rec.query)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleMacroGetSnapshotHistory_DefaultDays(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, _, err := s.handleMacroGetSnapshotHistory(context.Background(), nil, macroSnapshotHistoryInput{Days: 0})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/macro/snapshot/timeline" {
		t.Fatalf("path=%s, want /api/macro/snapshot/timeline (A02 fix: wrapper must point to timeline endpoint, not history)", rec.path)
	}
	if rec.query.Get("days") != "30" {
		t.Fatalf("expected days=30 default, got %q", rec.query.Get("days"))
	}
}

func TestHandleMacroGetSnapshotHistory_ClampedTo365(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, _, err := s.handleMacroGetSnapshotHistory(context.Background(), nil, macroSnapshotHistoryInput{Days: 9999})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/macro/snapshot/timeline" {
		t.Fatalf("path=%s, want /api/macro/snapshot/timeline", rec.path)
	}
	if got := rec.query.Get("days"); got != "365" {
		t.Fatalf("expected days=365 clamp, got %q", got)
	}
}

func TestHandleMacroGetStressIndexCurrent_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"score":-15}`)
	_, out, err := s.handleMacroGetStressIndexCurrent(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/narrative/stress-index/current" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleMacroGetStressIndexHistory_DefaultDays(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, _, err := s.handleMacroGetStressIndexHistory(context.Background(), nil, macroStressIndexHistoryInput{Days: 0})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/narrative/stress-index/history" {
		t.Fatalf("path=%s", rec.path)
	}
	if rec.query.Get("days") != "30" {
		t.Fatalf("expected days=30, got %q", rec.query.Get("days"))
	}
}

func TestHandleMacroGetCapitalFlowLatest_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"foreign":42}`)
	_, out, err := s.handleMacroGetCapitalFlowLatest(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/macro/capital-flow/latest" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleMacroGetIngestStatus_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"channels":[{"name":"fugle","status":"ok"}]}`)
	_, out, err := s.handleMacroGetIngestStatus(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/dashboard/macro-data-health" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}
