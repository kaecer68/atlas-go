package monitoring

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func newTestAutoHandler(t *testing.T) (*AutoHandler, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := NewAlertStore(dir)
	if err != nil {
		t.Fatalf("NewAlertStore: %v", err)
	}
	h := NewAutoHandler(store, nil)
	return h, dir
}

func TestAutoHandler_Suppress(t *testing.T) {
	h, _ := newTestAutoHandler(t)
	h.Suppress("cat", 5*time.Minute)

	alert := Alert{Level: AlertLevelWarning, Category: "cat"}
	if !h.isSuppressed(alert) {
		t.Error("expected alert to be suppressed after Suppress")
	}
}

func TestAutoHandler_Recover_NoStore(t *testing.T) {
	h := NewAutoHandler(nil, nil)
	// Should not panic and return early.
	h.Recover("cat")
}

func TestAutoHandler_Recover_ResolvesTriggered(t *testing.T) {
	h, _ := newTestAutoHandler(t)
	rec := domain.AlertRecord{
		ID:     "a1",
		Rule:   "cat",
		Status: domain.AlertStatusTriggered,
	}
	if err := h.alertStore.Save(rec); err != nil {
		t.Fatalf("Save: %v", err)
	}

	h.Recover("cat")

	all, err := h.alertStore.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 record, got %d", len(all))
	}
	if all[0].Status != domain.AlertStatusResolved {
		t.Errorf("status = %v, want resolved", all[0].Status)
	}
	if all[0].ResolvedBy != "auto-recovery" {
		t.Errorf("resolved by = %q, want auto-recovery", all[0].ResolvedBy)
	}
}

func TestAutoHandler_Handle_InfoAcknowledged(t *testing.T) {
	h, dir := newTestAutoHandler(t)
	if err := h.alertStore.Save(domain.AlertRecord{ID: "i1", Rule: "info-rule", Status: domain.AlertStatusTriggered}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	h.Handle(Alert{ID: "i1", Level: AlertLevelInfo, Category: "info-rule"})

	all, _ := h.alertStore.LoadAll()
	if len(all) != 1 || !all[0].Acknowledged {
		t.Errorf("expected INFO alert to be acknowledged: %+v", all)
	}
	_ = dir
}

func TestAutoHandler_Handle_NonInfoSuppressed(t *testing.T) {
	h, _ := newTestAutoHandler(t)
	h.Suppress("warn-cat", 5*time.Minute)

	called := false
	_ = called
	h.Handle(Alert{Level: AlertLevelWarning, Category: "warn-cat"})
}

func TestAutoHandler_StaticRulesSuppress(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewAlertStore(dir)
	h := NewAutoHandler(store, []SuppressRule{{Category: "s1", Duration: 5 * time.Minute}})

	if !h.isSuppressed(Alert{Category: "s1"}) {
		t.Error("expected static rule to suppress matching category")
	}
}

func TestAutoHandler_StaticRuleEmptyCategoryMatchesAll(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewAlertStore(dir)
	h := NewAutoHandler(store, []SuppressRule{{Category: "", Duration: 5 * time.Minute}})

	if !h.isSuppressed(Alert{Category: "anything"}) {
		t.Error("expected empty-category rule to match all categories")
	}
}

func TestAutoHandler_NewRulesNilDefaults(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewAlertStore(dir)
	h := NewAutoHandler(store, nil)
	if h.rules == nil {
		t.Error("expected non-nil rules slice")
	}
}

func TestAutoHandler_Recover_NoTriggered(t *testing.T) {
	h, _ := newTestAutoHandler(t)
	_ = h.alertStore.Save(domain.AlertRecord{ID: "r1", Rule: "cat", Status: domain.AlertStatusResolved})
	h.Recover("cat")
	all, _ := h.alertStore.LoadAll()
	if len(all) != 1 || all[0].ResolvedBy != "" {
		t.Error("recover should not modify already-resolved records")
	}
}

func TestAutoHandler_Suppress_OverridesExpiry(t *testing.T) {
	h, _ := newTestAutoHandler(t)
	h.Suppress("cat", 1*time.Minute)
	if !h.isSuppressed(Alert{Category: "cat"}) {
		t.Fatal("initial suppression failed")
	}
	// Override with an already-expired time.
	h.Suppress("cat", -1*time.Second)
	if h.isSuppressed(Alert{Category: "cat"}) {
		t.Error("expected override to expire suppression")
	}
}

var _ = filepath.Join // keep filepath import used
