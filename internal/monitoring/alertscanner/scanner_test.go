package alertscanner_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/monitoring/alertscanner"
)

// stubStore implements alertscanner.Store for tests.
type stubStore struct {
	records []domain.AlertRecord
	err     error // if set, all methods return this error
}

func (s *stubStore) LoadAll() ([]domain.AlertRecord, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.records, nil
}

func (s *stubStore) LoadUnacknowledged() ([]domain.AlertRecord, error) {
	if s.err != nil {
		return nil, s.err
	}
	var unack []domain.AlertRecord
	for _, r := range s.records {
		if r.Status == domain.AlertStatusTriggered || r.Status == domain.AlertStatusAcknowledged {
			unack = append(unack, r)
		}
	}
	return unack, nil
}

func makeAlert(id string, status domain.AlertStatus, severity string) domain.AlertRecord {
	return domain.AlertRecord{
		ID:        id,
		Timestamp: time.Now().UTC(),
		Status:    status,
		Severity:  severity,
		Rule:      "test-rule",
		Message:   "test message",
	}
}

func TestScanner_Scan_ReturnsUnacknowledged(t *testing.T) {
	store := &stubStore{records: []domain.AlertRecord{
		makeAlert("a1", domain.AlertStatusTriggered, "warning"),
		makeAlert("a2", domain.AlertStatusAcknowledged, "error"),
		makeAlert("a3", domain.AlertStatusResolved, "info"),
	}}
	s := alertscanner.New(store)

	records, err := s.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("got %d records, want 2 (triggered + acknowledged only)", len(records))
	}
}

func TestScanner_ScanActive_ExcludesResolved(t *testing.T) {
	store := &stubStore{records: []domain.AlertRecord{
		makeAlert("a1", domain.AlertStatusTriggered, "critical"),
		makeAlert("a2", domain.AlertStatusResolved, "error"),
		makeAlert("a3", domain.AlertStatusSilenced, "warning"),
	}}
	s := alertscanner.New(store)

	records, err := s.ScanActive(context.Background())
	if err != nil {
		t.Fatalf("ScanActive: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("got %d records, want 2 (triggered + silenced only)", len(records))
	}
}

func TestScanner_Scan_EmptyStore(t *testing.T) {
	store := &stubStore{}
	s := alertscanner.New(store)

	records, err := s.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("got %d records, want 0", len(records))
	}
}

func TestScanner_Scan_StoreError(t *testing.T) {
	store := &stubStore{err: errors.New("store down")}
	s := alertscanner.New(store)

	_, err := s.Scan(context.Background())
	if err == nil {
		t.Fatal("expected error from store failure")
	}
}

func TestScanner_CountBySeverity(t *testing.T) {
	store := &stubStore{records: []domain.AlertRecord{
		makeAlert("a1", domain.AlertStatusTriggered, "critical"),
		makeAlert("a2", domain.AlertStatusTriggered, "critical"),
		makeAlert("a3", domain.AlertStatusAcknowledged, "error"),
		makeAlert("a4", domain.AlertStatusAcknowledged, "warning"),
		makeAlert("a5", domain.AlertStatusResolved, "critical"), // excluded
	}}
	s := alertscanner.New(store)

	counts, err := s.CountBySeverity(context.Background())
	if err != nil {
		t.Fatalf("CountBySeverity: %v", err)
	}
	if counts["critical"] != 2 {
		t.Errorf("critical = %d, want 2", counts["critical"])
	}
	if counts["error"] != 1 {
		t.Errorf("error = %d, want 1", counts["error"])
	}
	if counts["warning"] != 1 {
		t.Errorf("warning = %d, want 1", counts["warning"])
	}
	if counts["info"] != 0 {
		t.Errorf("info = %d, want 0", counts["info"])
	}
}

func TestScanner_HasBlockers(t *testing.T) {
	store := &stubStore{records: []domain.AlertRecord{
		makeAlert("a1", domain.AlertStatusTriggered, "error"),
	}}
	s := alertscanner.New(store)

	blocked, err := s.HasBlockers(context.Background())
	if err != nil {
		t.Fatalf("HasBlockers: %v", err)
	}
	if !blocked {
		t.Error("expected blocked=true for error-severity alert")
	}
}

func TestScanner_HasBlockers_NoBlockers(t *testing.T) {
	store := &stubStore{records: []domain.AlertRecord{
		makeAlert("a1", domain.AlertStatusTriggered, "warning"),
		makeAlert("a2", domain.AlertStatusTriggered, "info"),
	}}
	s := alertscanner.New(store)

	blocked, err := s.HasBlockers(context.Background())
	if err != nil {
		t.Fatalf("HasBlockers: %v", err)
	}
	if blocked {
		t.Error("expected blocked=false for warning+info only")
	}
}

func TestScanner_Snapshot(t *testing.T) {
	store := &stubStore{records: []domain.AlertRecord{
		makeAlert("a1", domain.AlertStatusTriggered, "critical"),
		makeAlert("a2", domain.AlertStatusTriggered, "warning"),
	}}
	s := alertscanner.New(store)

	snap, err := s.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snap.Total != 2 {
		t.Errorf("Total = %d, want 2", snap.Total)
	}
	if !snap.Blocked {
		t.Error("Blocked should be true with critical alert")
	}
	if snap.ScannedAt.IsZero() {
		t.Error("ScannedAt should not be zero")
	}
	if snap.Alerts == nil {
		t.Error("Alerts should not be nil")
	}
}

func TestScanner_Scan_TimestampDesc(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)

	store := &stubStore{records: []domain.AlertRecord{
		{ID: "older", Timestamp: t1, Status: domain.AlertStatusTriggered, Severity: "info"},
		{ID: "newer", Timestamp: t2, Status: domain.AlertStatusTriggered, Severity: "info"},
	}}
	s := alertscanner.New(store)

	records, _ := s.Scan(context.Background())
	if len(records) != 2 {
		t.Fatalf("got %d records", len(records))
	}
	// Newest first
	if records[0].ID != "newer" {
		t.Errorf("first = %s, want newer", records[0].ID)
	}
	if records[1].ID != "older" {
		t.Errorf("second = %s, want older", records[1].ID)
	}
}
