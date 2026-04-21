package monitoring

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func newTestStore(t *testing.T) *AlertStore {
	t.Helper()
	dir := t.TempDir()
	store, err := NewAlertStore(dir)
	if err != nil {
		t.Fatalf("NewAlertStore: %v", err)
	}
	return store
}

func makeAlert(id string) domain.AlertRecord {
	return domain.AlertRecord{
		ID:        id,
		Timestamp: time.Now(),
		Rule:      "test_rule",
		Severity:  "WARNING",
		Message:   "test message",
		Value:     42.0,
	}
}

func TestAlertStore_SaveAndLoadAll(t *testing.T) {
	store := newTestStore(t)

	a1 := makeAlert("alert-1")
	a2 := makeAlert("alert-2")

	if err := store.Save(a1); err != nil {
		t.Fatalf("Save(a1): %v", err)
	}
	if err := store.Save(a2); err != nil {
		t.Fatalf("Save(a2): %v", err)
	}

	records, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("LoadAll len = %d, want 2", len(records))
	}
	if records[0].ID != "alert-1" {
		t.Errorf("records[0].ID = %q, want alert-1", records[0].ID)
	}
	if records[1].ID != "alert-2" {
		t.Errorf("records[1].ID = %q, want alert-2", records[1].ID)
	}
}

func TestAlertStore_LoadAll_EmptyFile(t *testing.T) {
	store := newTestStore(t)

	records, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll on empty: %v", err)
	}
	if records != nil {
		t.Errorf("LoadAll on empty = %v, want nil", records)
	}
}

func TestAlertStore_LoadAll_NonExistentFile(t *testing.T) {
	dir := t.TempDir()
	store := &AlertStore{filePath: filepath.Join(dir, "nonexistent.jsonl")}

	records, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll nonexistent: %v", err)
	}
	if records != nil {
		t.Errorf("LoadAll nonexistent = %v, want nil", records)
	}
}

func TestAlertStore_LoadUnacknowledged(t *testing.T) {
	store := newTestStore(t)

	a1 := makeAlert("alert-1")
	a2 := makeAlert("alert-2")
	a3 := makeAlert("alert-3")
	a3.Acknowledged = true

	if err := store.Save(a1); err != nil {
		t.Fatalf("Save(a1): %v", err)
	}
	if err := store.Save(a2); err != nil {
		t.Fatalf("Save(a2): %v", err)
	}
	if err := store.Save(a3); err != nil {
		t.Fatalf("Save(a3): %v", err)
	}

	unacked, err := store.LoadUnacknowledged()
	if err != nil {
		t.Fatalf("LoadUnacknowledged: %v", err)
	}
	if len(unacked) != 2 {
		t.Fatalf("LoadUnacknowledged len = %d, want 2", len(unacked))
	}
	for _, a := range unacked {
		if a.Acknowledged {
			t.Errorf("unacknowledged alert %s should not be acknowledged", a.ID)
		}
	}
}

func TestAlertStore_LoadUnacknowledged_AllAcked(t *testing.T) {
	store := newTestStore(t)

	a1 := makeAlert("alert-1")
	a1.Acknowledged = true
	if err := store.Save(a1); err != nil {
		t.Fatalf("Save: %v", err)
	}

	unacked, err := store.LoadUnacknowledged()
	if err != nil {
		t.Fatalf("LoadUnacknowledged: %v", err)
	}
	if len(unacked) != 0 {
		t.Errorf("LoadUnacknowledged len = %d, want 0", len(unacked))
	}
}

func TestAlertStore_Acknowledge(t *testing.T) {
	store := newTestStore(t)

	a1 := makeAlert("alert-1")
	if err := store.Save(a1); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := store.Acknowledge("alert-1", "user1"); err != nil {
		t.Fatalf("Acknowledge: %v", err)
	}

	records, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("LoadAll len = %d, want 1", len(records))
	}
	if !records[0].Acknowledged {
		t.Error("alert should be acknowledged")
	}
	if records[0].AcknowledgedBy != "user1" {
		t.Errorf("AcknowledgedBy = %q, want user1", records[0].AcknowledgedBy)
	}
	if records[0].AcknowledgedAt == nil {
		t.Error("AcknowledgedAt should not be nil")
	}
}

func TestAlertStore_Acknowledge_NotFound(t *testing.T) {
	store := newTestStore(t)

	err := store.Acknowledge("nonexistent", "user1")
	if err == nil {
		t.Fatal("Acknowledge nonexistent: expected error, got nil")
	}
}

func TestAlertStore_Acknowledge_MultipleRecords(t *testing.T) {
	store := newTestStore(t)

	a1 := makeAlert("alert-1")
	a2 := makeAlert("alert-2")
	if err := store.Save(a1); err != nil {
		t.Fatalf("Save(a1): %v", err)
	}
	if err := store.Save(a2); err != nil {
		t.Fatalf("Save(a2): %v", err)
	}

	if err := store.Acknowledge("alert-2", "user2"); err != nil {
		t.Fatalf("Acknowledge: %v", err)
	}

	records, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("LoadAll len = %d, want 2", len(records))
	}
	if records[0].Acknowledged {
		t.Error("alert-1 should NOT be acknowledged")
	}
	if !records[1].Acknowledged {
		t.Error("alert-2 should be acknowledged")
	}
	if records[1].AcknowledgedBy != "user2" {
		t.Errorf("AcknowledgedBy = %q, want user2", records[1].AcknowledgedBy)
	}
}

func TestAlertStore_DirCreation(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub", "alerts")
	store, err := NewAlertStore(dir)
	if err != nil {
		t.Fatalf("NewAlertStore: %v", err)
	}
	if store.filePath != filepath.Join(dir, "alerts.jsonl") {
		t.Errorf("filePath = %q, want %q", store.filePath, filepath.Join(dir, "alerts.jsonl"))
	}
}

func TestAlertStore_DirCreationError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping as root")
	}
	_, err := NewAlertStore("/proc/impossible/path")
	if err == nil {
		t.Fatal("expected error creating in /proc, got nil")
	}
}
