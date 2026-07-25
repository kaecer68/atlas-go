package alertscanner

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// stubSource is a test AlertSource with configurable results.
type stubSource struct {
	name    string
	records []domain.AlertRecord
	err     error
}

func (s *stubSource) Name() string { return s.name }
func (s *stubSource) ListActive(_ context.Context) ([]domain.AlertRecord, error) {
	return s.records, s.err
}

func TestMultiScanner_Scan_MergesSources(t *testing.T) {
	now := time.Now()
	s1 := &stubSource{
		name: "source-a",
		records: []domain.AlertRecord{
			{ID: "a1", Timestamp: now, Severity: "warning", Acknowledged: false},
			{ID: "a2", Timestamp: now.Add(-time.Hour), Severity: "info", Acknowledged: true},
		},
	}
	s2 := &stubSource{
		name: "source-b",
		records: []domain.AlertRecord{
			{ID: "b1", Timestamp: now.Add(-30 * time.Minute), Severity: "critical", Acknowledged: false},
		},
	}

	ms := NewMultiScanner(s1, s2)
	alerts, err := ms.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(alerts) != 2 {
		t.Fatalf("got %d alerts, want 2: %+v", len(alerts), alerts)
	}
	if alerts[0].ID != "a1" {
		t.Errorf("first alert should be a1 (newest), got %s", alerts[0].ID)
	}
}

func TestMultiScanner_ScanActive_IncludesAcknowledged(t *testing.T) {
	now := time.Now()
	s1 := &stubSource{
		name: "s1",
		records: []domain.AlertRecord{
			{ID: "a1", Timestamp: now, Severity: "info", Acknowledged: true},
		},
	}

	ms := NewMultiScanner(s1)
	alerts, err := ms.ScanActive(context.Background())
	if err != nil {
		t.Fatalf("ScanActive: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("ScanActive should include acknowledged alerts, got %d", len(alerts))
	}
}

func TestMultiScanner_DedupByID(t *testing.T) {
	now := time.Now()
	s1 := &stubSource{
		name: "s1",
		records: []domain.AlertRecord{
			{ID: "dup-1", Timestamp: now, Severity: "warning", Acknowledged: false},
		},
	}
	s2 := &stubSource{
		name: "s2",
		records: []domain.AlertRecord{
			{ID: "dup-1", Timestamp: now.Add(-time.Hour), Severity: "warning", Acknowledged: false},
		},
	}

	ms := NewMultiScanner(s1, s2)
	alerts, err := ms.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("got %d alerts, want 1 after dedup", len(alerts))
	}
}

func TestMultiScanner_PartialFailure(t *testing.T) {
	now := time.Now()
	s1 := &stubSource{
		name: "ok-source",
		records: []domain.AlertRecord{
			{ID: "a1", Timestamp: now, Severity: "warning", Acknowledged: false},
		},
	}
	s2 := &stubSource{
		name: "bad-source",
		err:  errors.New("connection refused"),
	}

	ms := NewMultiScanner(s1, s2)
	alerts, err := ms.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan should tolerate partial failure: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("got %d alerts, want 1 from healthy source", len(alerts))
	}
}

func TestMultiScanner_AllSourcesFail(t *testing.T) {
	s1 := &stubSource{name: "s1", err: errors.New("dead")}
	s2 := &stubSource{name: "s2", err: errors.New("also dead")}

	ms := NewMultiScanner(s1, s2)
	_, err := ms.Scan(context.Background())
	if err == nil {
		t.Fatal("expected error when all sources fail")
	}
}

func TestMultiScanner_Snapshot(t *testing.T) {
	now := time.Now()
	s1 := &stubSource{
		name: "s1",
		records: []domain.AlertRecord{
			{ID: "a1", Timestamp: now, Severity: "critical", Acknowledged: false},
			{ID: "a2", Timestamp: now, Severity: "warning", Acknowledged: false},
		},
	}

	ms := NewMultiScanner(s1)
	snap, err := ms.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snap.Total != 2 {
		t.Errorf("Total = %d, want 2", snap.Total)
	}
	if !snap.Blocked {
		t.Error("Blocked should be true (has critical)")
	}
	if snap.BySeverity["critical"] != 1 {
		t.Errorf("critical count = %d, want 1", snap.BySeverity["critical"])
	}
}
