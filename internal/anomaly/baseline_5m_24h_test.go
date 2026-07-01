package anomaly

import (
	"context"
	"testing"
	"time"
)

func TestBaseline5m24hDetector_Name(t *testing.T) {
	d := NewBaseline5m24hDetector(DefaultConfig())
	if got := d.Name(); got != "baseline_5m_24h" {
		t.Errorf("Name = %q, want baseline_5m_24h", got)
	}
}

func TestBaseline5m24hDetector_Detect_returnsAnomalyWhenCurrentExceedsBaseline(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	d := NewBaseline5m24hDetector(DefaultConfig())
	d.now = func() time.Time { return now }

	// 24h baseline: 1 call per 5-minute bucket → median 1
	// 5m current: 10 calls → median 10
	entries := []AuditEntryV2{}
	for i := 0; i < 288; i++ {
		entries = append(entries, AuditEntryV2{TS: now.Add(-time.Duration(i*5+10) * time.Minute).Format(time.RFC3339), Tool: "tool_a", Status: "ok"})
	}
	for i := 0; i < 10; i++ {
		entries = append(entries, AuditEntryV2{TS: now.Add(-time.Duration(i) * time.Second).Format(time.RFC3339), Tool: "tool_a", Status: "ok"})
	}

	got, err := d.Detect(context.Background(), entries)
	if err != nil {
		t.Fatalf("Detect error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Detect = %d anomalies, want 1", len(got))
	}
	if got[0].Type != "baseline_5m_24h" {
		t.Errorf("Anomaly.Type = %q, want baseline_5m_24h", got[0].Type)
	}
	if got[0].Score <= 0 {
		t.Errorf("Anomaly.Score = %v, want positive", got[0].Score)
	}
}

func TestBaseline5m24hDetector_Detect_returnsEmptyWhenBaselineInsufficient(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	d := NewBaseline5m24hDetector(DefaultConfig())
	d.now = func() time.Time { return now }

	entries := []AuditEntryV2{
		{TS: now.Add(-1 * time.Minute).Format(time.RFC3339), Tool: "tool_a", Status: "ok"},
	}

	got, err := d.Detect(context.Background(), entries)
	if err != nil {
		t.Fatalf("Detect error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Detect = %d anomalies, want 0", len(got))
	}
}

func TestBaseline5m24hDetector_Detect_respectsContextCancellation(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	d := NewBaseline5m24hDetector(DefaultConfig())
	d.now = func() time.Time { return now }

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := d.Detect(ctx, []AuditEntryV2{})
	if err == nil {
		t.Error("Detect did not return context error")
	}
}
