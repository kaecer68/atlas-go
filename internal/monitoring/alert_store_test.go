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

func TestAlertStore_FindByDedupKey(t *testing.T) {
	store := newTestStore(t)

	a1 := makeAlert("alert-1")
	a1.DedupKey = "dedup-abc"
	a2 := makeAlert("alert-2")
	a2.DedupKey = "dedup-xyz"

	if err := store.Save(a1); err != nil {
		t.Fatalf("Save(a1): %v", err)
	}
	if err := store.Save(a2); err != nil {
		t.Fatalf("Save(a2): %v", err)
	}

	found, err := store.FindByDedupKey("dedup-xyz")
	if err != nil {
		t.Fatalf("FindByDedupKey: %v", err)
	}
	if found == nil {
		t.Fatal("FindByDedupKey: expected match, got nil")
	}
	if found.ID != "alert-2" {
		t.Errorf("found.ID = %q, want alert-2", found.ID)
	}
}

func TestAlertStore_FindByDedupKey_NotFound(t *testing.T) {
	store := newTestStore(t)

	found, err := store.FindByDedupKey("nonexistent")
	if err != nil {
		t.Fatalf("FindByDedupKey: %v", err)
	}
	if found != nil {
		t.Errorf("FindByDedupKey = %v, want nil", found)
	}
}

func TestAlertStore_Update(t *testing.T) {
	store := newTestStore(t)

	a1 := makeAlert("alert-1")
	if err := store.Save(a1); err != nil {
		t.Fatalf("Save: %v", err)
	}

	now := time.Now()
	err := store.Update("alert-1", func(rec *domain.AlertRecord) {
		rec.Acknowledged = true
		rec.AcknowledgedAt = &now
		rec.AcknowledgedBy = "tester"
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	records, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("LoadAll len = %d, want 1", len(records))
	}
	if !records[0].Acknowledged {
		t.Error("record should be acknowledged")
	}
	if records[0].AcknowledgedBy != "tester" {
		t.Errorf("AcknowledgedBy = %q, want tester", records[0].AcknowledgedBy)
	}
}

func TestAlertStore_Update_NotFound(t *testing.T) {
	store := newTestStore(t)

	err := store.Update("nonexistent", func(rec *domain.AlertRecord) {
		rec.Acknowledged = true
	})
	if err == nil {
		t.Fatal("Update nonexistent: expected error, got nil")
	}
}

func TestAlertStore_Resolve(t *testing.T) {
	store := newTestStore(t)

	a1 := makeAlert("alert-1")
	if err := store.Save(a1); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := store.Resolve("alert-1", "user1"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	records, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("LoadAll len = %d, want 1", len(records))
	}
	if records[0].Status != domain.AlertStatusResolved {
		t.Errorf("Status = %q, want resolved", records[0].Status)
	}
	if records[0].ResolvedBy != "user1" {
		t.Errorf("ResolvedBy = %q, want user1", records[0].ResolvedBy)
	}
	if records[0].ResolvedAt == nil {
		t.Error("ResolvedAt should not be nil")
	}
}

func TestAlertStore_Resolve_NotFound(t *testing.T) {
	store := newTestStore(t)

	err := store.Resolve("nonexistent", "user1")
	if err == nil {
		t.Fatal("Resolve nonexistent: expected error, got nil")
	}
}

func TestAlertStore_DeleteWhere(t *testing.T) {
	store := newTestStore(t)
	a1 := makeAlert("alert-1")
	a1.Rule = "cpu"
	a2 := makeAlert("alert-2")
	a2.Rule = "memory"
	if err := store.Save(a1); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(a2); err != nil {
		t.Fatal(err)
	}

	deleted, err := store.DeleteWhere(func(r *domain.AlertRecord) bool { return r.Rule == "cpu" })
	if err != nil {
		t.Fatalf("DeleteWhere: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}
	all, _ := store.LoadAll()
	if len(all) != 1 || all[0].ID != "alert-2" {
		t.Errorf("expected only alert-2, got %+v", all)
	}
}

func TestAlertStore_DeleteWhere_NoMatch(t *testing.T) {
	store := newTestStore(t)
	if err := store.Save(makeAlert("alert-1")); err != nil {
		t.Fatal(err)
	}
	deleted, err := store.DeleteWhere(func(r *domain.AlertRecord) bool { return false })
	if err != nil {
		t.Fatalf("DeleteWhere: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0", deleted)
	}
}

func TestAlertStore_AcknowledgeWhere(t *testing.T) {
	store := newTestStore(t)
	a1 := makeAlert("alert-1")
	a1.Rule = "cpu"
	a2 := makeAlert("alert-2")
	a2.Rule = "memory"
	if err := store.Save(a1); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(a2); err != nil {
		t.Fatal(err)
	}

	acked, err := store.AcknowledgeWhere(func(r *domain.AlertRecord) bool { return r.Rule == "cpu" }, "bot")
	if err != nil {
		t.Fatalf("AcknowledgeWhere: %v", err)
	}
	if acked != 1 {
		t.Errorf("acked = %d, want 1", acked)
	}
	all, _ := store.LoadAll()
	if !all[0].Acknowledged {
		t.Error("expected alert-1 acknowledged")
	}
}

func TestAlertStore_AcknowledgeWhere_NoMatch(t *testing.T) {
	store := newTestStore(t)
	if err := store.Save(makeAlert("alert-1")); err != nil {
		t.Fatal(err)
	}
	acked, err := store.AcknowledgeWhere(func(r *domain.AlertRecord) bool { return false }, "bot")
	if err != nil {
		t.Fatalf("AcknowledgeWhere: %v", err)
	}
	if acked != 0 {
		t.Errorf("acked = %d, want 0", acked)
	}
}

func TestAlertStore_ResolveWhere(t *testing.T) {
	store := newTestStore(t)
	a1 := makeAlert("alert-1")
	a1.Rule = "cpu"
	a1.Status = domain.AlertStatusTriggered
	a2 := makeAlert("alert-2")
	a2.Rule = "cpu"
	a2.Status = domain.AlertStatusAcknowledged
	if err := store.Save(a1); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(a2); err != nil {
		t.Fatal(err)
	}

	resolved, err := store.ResolveWhere(func(r *domain.AlertRecord) bool {
		return r.Rule == "cpu" && r.Status == domain.AlertStatusTriggered
	}, "bot")
	if err != nil {
		t.Fatalf("ResolveWhere: %v", err)
	}
	if resolved != 1 {
		t.Errorf("resolved = %d, want 1", resolved)
	}
	all, _ := store.LoadAll()
	if all[0].Status != domain.AlertStatusResolved {
		t.Errorf("expected alert-1 resolved, got %s", all[0].Status)
	}
}

func TestAlertStore_ResolveWhere_NoMatch(t *testing.T) {
	store := newTestStore(t)
	if err := store.Save(makeAlert("alert-1")); err != nil {
		t.Fatal(err)
	}
	resolved, err := store.ResolveWhere(func(r *domain.AlertRecord) bool { return false }, "bot")
	if err != nil {
		t.Fatalf("ResolveWhere: %v", err)
	}
	if resolved != 0 {
		t.Errorf("resolved = %d, want 0", resolved)
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

func TestAlertStore_DeleteWhere(t *testing.T) {
	store := newTestStore(t)

	a1 := makeAlert("alert-1")
	a1.DedupKey = "keep"
	a2 := makeAlert("alert-2")
	a2.DedupKey = "delete-me"
	a3 := makeAlert("alert-3")
	a3.DedupKey = "delete-me-too"

	for _, a := range []domain.AlertRecord{a1, a2, a3} {
		if err := store.Save(a); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	deleted, err := store.DeleteWhere(func(r *domain.AlertRecord) bool {
		return r.DedupKey == "delete-me" || r.DedupKey == "delete-me-too"
	})
	if err != nil {
		t.Fatalf("DeleteWhere: %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}

	records, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("LoadAll len = %d, want 1", len(records))
	}
	if records[0].ID != "alert-1" {
		t.Errorf("remaining record ID = %q, want alert-1", records[0].ID)
	}
}

func TestAlertStore_DeleteWhere_NoMatch(t *testing.T) {
	store := newTestStore(t)

	a1 := makeAlert("alert-1")
	if err := store.Save(a1); err != nil {
		t.Fatalf("Save: %v", err)
	}

	deleted, err := store.DeleteWhere(func(r *domain.AlertRecord) bool {
		return false
	})
	if err != nil {
		t.Fatalf("DeleteWhere: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0", deleted)
	}

	records, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("LoadAll len = %d, want 1", len(records))
	}
}

func TestAlertStore_AcknowledgeWhere(t *testing.T) {
	store := newTestStore(t)

	a1 := makeAlert("alert-1")
	a1.DedupKey = "group-a"
	a2 := makeAlert("alert-2")
	a2.DedupKey = "group-a"
	a3 := makeAlert("alert-3")
	a3.DedupKey = "group-b"

	for _, a := range []domain.AlertRecord{a1, a2, a3} {
		if err := store.Save(a); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	acked, err := store.AcknowledgeWhere(func(r *domain.AlertRecord) bool {
		return r.DedupKey == "group-a"
	}, "admin")
	if err != nil {
		t.Fatalf("AcknowledgeWhere: %v", err)
	}
	if acked != 2 {
		t.Errorf("acked = %d, want 2", acked)
	}

	records, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("LoadAll len = %d, want 3", len(records))
	}
	for _, r := range records {
		if r.DedupKey == "group-a" {
			if !r.Acknowledged {
				t.Errorf("alert %s should be acknowledged", r.ID)
			}
			if r.AcknowledgedBy != "admin" {
				t.Errorf("AcknowledgedBy = %q, want admin", r.AcknowledgedBy)
			}
		} else {
			if r.Acknowledged {
				t.Errorf("alert %s should NOT be acknowledged", r.ID)
			}
		}
	}
}

func TestAlertStore_AcknowledgeWhere_NoMatch(t *testing.T) {
	store := newTestStore(t)

	a1 := makeAlert("alert-1")
	if err := store.Save(a1); err != nil {
		t.Fatalf("Save: %v", err)
	}

	acked, err := store.AcknowledgeWhere(func(r *domain.AlertRecord) bool {
		return false
	}, "admin")
	if err != nil {
		t.Fatalf("AcknowledgeWhere: %v", err)
	}
	if acked != 0 {
		t.Errorf("acked = %d, want 0", acked)
	}
}

func TestAlertStore_ResolveWhere(t *testing.T) {
	store := newTestStore(t)

	a1 := makeAlert("alert-1")
	a1.DedupKey = "resolve-group"
	a2 := makeAlert("alert-2")
	a2.DedupKey = "resolve-group"
	a3 := makeAlert("alert-3")
	a3.DedupKey = "keep-group"

	for _, a := range []domain.AlertRecord{a1, a2, a3} {
		if err := store.Save(a); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	resolved, err := store.ResolveWhere(func(r *domain.AlertRecord) bool {
		return r.DedupKey == "resolve-group"
	}, "auto-recovery")
	if err != nil {
		t.Fatalf("ResolveWhere: %v", err)
	}
	if resolved != 2 {
		t.Errorf("resolved = %d, want 2", resolved)
	}

	records, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("LoadAll len = %d, want 3", len(records))
	}
	for _, r := range records {
		if r.DedupKey == "resolve-group" {
			if r.Status != domain.AlertStatusResolved {
				t.Errorf("alert %s status = %q, want resolved", r.ID, r.Status)
			}
			if r.ResolvedBy != "auto-recovery" {
				t.Errorf("ResolvedBy = %q, want auto-recovery", r.ResolvedBy)
			}
			if r.ResolvedAt == nil {
				t.Error("ResolvedAt should not be nil")
			}
		} else {
			if r.Status == domain.AlertStatusResolved {
				t.Errorf("alert %s should NOT be resolved", r.ID)
			}
		}
	}
}

func TestAlertStore_ResolveWhere_NoMatch(t *testing.T) {
	store := newTestStore(t)

	a1 := makeAlert("alert-1")
	if err := store.Save(a1); err != nil {
		t.Fatalf("Save: %v", err)
	}

	resolved, err := store.ResolveWhere(func(r *domain.AlertRecord) bool {
		return false
	}, "auto-recovery")
	if err != nil {
		t.Fatalf("ResolveWhere: %v", err)
	}
	if resolved != 0 {
		t.Errorf("resolved = %d, want 0", resolved)
	}
}
