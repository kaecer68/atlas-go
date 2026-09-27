package server

import (
	"context"
	"strings"
	"testing"
)

func TestHandleNarrativeGetEvents_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleNarrativeGetEvents(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	// rec.path reflects the LAST HTTP call (fetchCurrentPeriod enrichment).
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleNarrativeGetChains_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleNarrativeGetChains(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/narrative/chains" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleNarrativeGetModels_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleNarrativeGetModels(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/narrative/models" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleNarrativeGetTemplates_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleNarrativeGetTemplates(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/narrative/templates" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleNarrativeGetSeasonal_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleNarrativeGetSeasonal(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/narrative/seasonal" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleNarrativeGetBundle_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleNarrativeGetBundle(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/narrative/bundle" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleNarrativeStressIndexThresholds_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleNarrativeStressIndexThresholds(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/narrative/stress-index/thresholds" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

// TestHandleNarrativeGetEvents_PeriodWeightNotApplied pins the contract of the
// `period_weight_applied` field (issue #1944 Batch 2, hardcoded-"applied" pass;
// Batch A added the missing test).
//
// The handler used to publish `period_weight_applied: true` unconditionally for
// any event payload that carried a period. Period weighting lives in
// internal/narrative/detector.go, whose DetectEvents takes no period, and its
// only production caller (internal/scheduler/template_detector_scan.go) never
// sets CurrentPeriod — while this handler reads /api/narrative/events, i.e.
// narrative_engine.DetectEvents. The events returned here are therefore NOT
// period-weighted, and the field must say false. Flipping it back to true is a
// false claim that no other test would catch: the value is a constant, so only
// this assertion can hold it down.
func TestHandleNarrativeGetEvents_PeriodWeightNotApplied(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	// fetchCurrentPeriod reads /api/regime/history; the harness answers every
	// path with responseBody, so one body drives both calls and the period is
	// non-empty — the exact condition that used to force the field to true.
	rec.responseBody = []byte(`{"current_period":"2026Q3","sessions":[{"period_name_zh":"第三季"}]}`)

	_, out, err := s.handleNarrativeGetEvents(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Result == nil || *out.Result == nil {
		t.Fatal("precondition: handler must enrich the event payload with the current period")
	}
	if _, ok := (*out.Result)["current_period"]; !ok {
		t.Fatal("precondition: current_period enrichment missing, the field under test would not be emitted")
	}

	applied, ok := (*out.Result)["period_weight_applied"]
	if !ok {
		t.Fatal("period_weight_applied missing: the payload must state whether the events are period-weighted")
	}
	if applied != false {
		t.Errorf("period_weight_applied = %v, want false: /api/narrative/events runs DetectEvents, which takes no period", applied)
	}

	note, _ := (*out.Result)["period_weight_note"].(string)
	if !strings.Contains(note, "DetectEvents") {
		t.Errorf("period_weight_note = %q, want the reason the flag is false (must name DetectEvents)", note)
	}
}
