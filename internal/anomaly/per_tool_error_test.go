package anomaly

import (
	"context"
	"testing"
	"time"
)

func TestPerToolErrorDetector_Name(t *testing.T) {
	d := NewPerToolErrorDetector(DefaultConfig())
	if got := d.Name(); got != "per_tool_error" {
		t.Errorf("Name = %q, want per_tool_error", got)
	}
}

func TestPerToolErrorDetector_Detect_returnsAnomalyForSpikingTool(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	d := NewPerToolErrorDetector(DefaultConfig())
	d.now = func() time.Time { return now }

	entries := []AuditEntryV2{}
	// Baseline: tool_a has 1 error every 5 minutes.
	for i := 0; i < 288; i++ {
		entries = append(entries, AuditEntryV2{
			TS:     now.Add(-time.Duration(i*5+10) * time.Minute).Format(time.RFC3339),
			Tool:   "tool_a",
			Status: "error",
		})
	}
	// Current window: tool_a has 10 errors.
	for i := 0; i < 10; i++ {
		entries = append(entries, AuditEntryV2{
			TS:     now.Add(-time.Duration(i) * time.Second).Format(time.RFC3339),
			Tool:   "tool_a",
			Status: "error",
		})
	}

	got, err := d.Detect(context.Background(), entries)
	if err != nil {
		t.Fatalf("Detect error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Detect = %d anomalies, want 1", len(got))
	}
	if got[0].Type != "per_tool_error" {
		t.Errorf("Anomaly.Type = %q, want per_tool_error", got[0].Type)
	}
	if got[0].Tool != "tool_a" {
		t.Errorf("Anomaly.Tool = %q, want tool_a", got[0].Tool)
	}
}

func TestPerToolErrorDetector_Detect_skipsToolsWithInsufficientBaseline(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	d := NewPerToolErrorDetector(DefaultConfig())
	d.now = func() time.Time { return now }

	entries := []AuditEntryV2{
		{TS: now.Add(-1 * time.Minute).Format(time.RFC3339), Tool: "tool_a", Status: "error"},
	}

	got, err := d.Detect(context.Background(), entries)
	if err != nil {
		t.Fatalf("Detect error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Detect = %d anomalies, want 0", len(got))
	}
}

func TestPerToolErrorDetector_Detect_returnsEmptyWhenNoSpike(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	d := NewPerToolErrorDetector(DefaultConfig())
	d.now = func() time.Time { return now }

	entries := []AuditEntryV2{}
	// Baseline and current both have 1 error per 5 minutes.
	for i := 0; i < 289; i++ {
		entries = append(entries, AuditEntryV2{
			TS:     now.Add(-time.Duration(i*5) * time.Minute).Format(time.RFC3339),
			Tool:   "tool_a",
			Status: "error",
		})
	}

	got, err := d.Detect(context.Background(), entries)
	if err != nil {
		t.Fatalf("Detect error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Detect = %d anomalies, want 0", len(got))
	}
}

func TestPerToolErrorDetector_Detect_respectsContextCancellation(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	d := NewPerToolErrorDetector(DefaultConfig())
	d.now = func() time.Time { return now }

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := d.Detect(ctx, []AuditEntryV2{})
	if err == nil {
		t.Error("Detect did not return context error")
	}
}
