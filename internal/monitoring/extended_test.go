package monitoring

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// ─── AlertStore Where methods ──────────────────────────────────────────────

func TestDeleteWhere_HappyPath(t *testing.T) {
	dir := t.TempDir()
	s, err := NewAlertStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Save(domain.AlertRecord{ID: "id-1", Rule: "test-1", Status: domain.AlertStatusTriggered})
	s.Save(domain.AlertRecord{ID: "id-2", Rule: "test-2", Status: domain.AlertStatusTriggered})

	count, err := s.DeleteWhere(func(r *domain.AlertRecord) bool { return r.Rule == "test-1" })
	if err != nil {
		t.Fatalf("DeleteWhere error: %v", err)
	}
	if count != 1 {
		t.Fatalf("DeleteWhere count = %d, want 1", count)
	}
}

func TestDeleteWhere_NoMatches(t *testing.T) {
	dir := t.TempDir()
	s, err := NewAlertStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Save(domain.AlertRecord{ID: "id-1", Rule: "test-1", Status: domain.AlertStatusTriggered})

	count, err := s.DeleteWhere(func(r *domain.AlertRecord) bool { return r.Rule == "nonexistent" })
	if err != nil {
		t.Fatalf("DeleteWhere error: %v", err)
	}
	if count != 0 {
		t.Fatalf("DeleteWhere count = %d, want 0", count)
	}
}

func TestAcknowledgeWhere_HappyPath(t *testing.T) {
	dir := t.TempDir()
	s, err := NewAlertStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Save(domain.AlertRecord{ID: "id-1", Rule: "test-1", Status: domain.AlertStatusTriggered})
	s.Save(domain.AlertRecord{ID: "id-2", Rule: "test-2", Status: domain.AlertStatusTriggered})

	count, err := s.AcknowledgeWhere(func(r *domain.AlertRecord) bool { return r.Rule == "test-1" }, "tester")
	if err != nil {
		t.Fatalf("AcknowledgeWhere error: %v", err)
	}
	if count != 1 {
		t.Fatalf("AcknowledgeWhere count = %d, want 1", count)
	}
}

func TestAcknowledgeWhere_NoMatches(t *testing.T) {
	dir := t.TempDir()
	s, err := NewAlertStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Save(domain.AlertRecord{ID: "id-1", Rule: "test-1", Status: domain.AlertStatusTriggered})

	count, err := s.AcknowledgeWhere(func(r *domain.AlertRecord) bool { return r.Rule == "nonexistent" }, "tester")
	if err != nil {
		t.Fatalf("AcknowledgeWhere error: %v", err)
	}
	if count != 0 {
		t.Fatalf("AcknowledgeWhere count = %d, want 0", count)
	}
}

func TestResolveWhere_HappyPath(t *testing.T) {
	dir := t.TempDir()
	s, err := NewAlertStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Save(domain.AlertRecord{ID: "id-1", Rule: "test-1", Status: domain.AlertStatusTriggered})
	s.Save(domain.AlertRecord{ID: "id-2", Rule: "test-2", Status: domain.AlertStatusTriggered})

	count, err := s.ResolveWhere(func(r *domain.AlertRecord) bool { return r.Rule == "test-2" }, "tester")
	if err != nil {
		t.Fatalf("ResolveWhere error: %v", err)
	}
	if count != 1 {
		t.Fatalf("ResolveWhere count = %d, want 1", count)
	}
}

func TestResolveWhere_NoMatches(t *testing.T) {
	dir := t.TempDir()
	s, err := NewAlertStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Save(domain.AlertRecord{ID: "id-1", Rule: "test-1", Status: domain.AlertStatusTriggered})

	count, err := s.ResolveWhere(func(r *domain.AlertRecord) bool { return r.Rule == "nonexistent" }, "tester")
	if err != nil {
		t.Fatalf("ResolveWhere error: %v", err)
	}
	if count != 0 {
		t.Fatalf("ResolveWhere count = %d, want 0", count)
	}
}

// ─── AutoHandler ───────────────────────────────────────────────────────────

func TestAutoHandler_Suppress_NilStore(t *testing.T) {
	h := NewAutoHandler(nil, nil)
	h.Suppress("test-category", 1*time.Hour)
	// Verify suppression took effect by checking isSuppressed.
	if !h.isSuppressed(Alert{Category: "test-category"}) {
		t.Error("expected test-category to be suppressed")
	}
}

func TestAutoHandler_Suppress_OverridesExisting(t *testing.T) {
	h := NewAutoHandler(nil, nil)
	h.Suppress("test-category", 1*time.Millisecond)
	time.Sleep(2 * time.Millisecond)
	h.Suppress("test-category", 1*time.Hour)
	if !h.isSuppressed(Alert{Category: "test-category"}) {
		t.Error("expected test-category to be suppressed after override")
	}
}

func TestNewAutoHandler_WithRules(t *testing.T) {
	rules := []SuppressRule{{Category: "test-cat", Duration: 1 * time.Hour}}
	h := NewAutoHandler(nil, rules)
	if !h.isSuppressed(Alert{Category: "test-cat"}) {
		t.Error("expected test-cat suppressed by rule at construction")
	}
}

// ─── MultiNotifier ─────────────────────────────────────────────────────────

type stubNotifier struct {
	name          string
	configured    bool
	notifyErr     error
	notifications []domain.AlertRecord
}

func (s *stubNotifier) Name() string       { return s.name }
func (s *stubNotifier) IsConfigured() bool { return s.configured }
func (s *stubNotifier) Notify(a domain.AlertRecord) error {
	s.notifications = append(s.notifications, a)
	return s.notifyErr
}

func TestMultiNotifier_AddNotifier(t *testing.T) {
	m := NewMultiNotifier()
	m.AddNotifier(&stubNotifier{name: "test1", configured: true})
	if len(m.notifiers) != 1 {
		t.Fatalf("notifiers len = %d, want 1", len(m.notifiers))
	}
}

func TestMultiNotifier_Notify(t *testing.T) {
	n1 := &stubNotifier{name: "n1", configured: true}
	n2 := &stubNotifier{name: "n2", configured: false}
	m := NewMultiNotifier(n1, n2)

	alert := domain.AlertRecord{ID: "alert-1", Rule: "test"}
	errs := m.Notify(alert)

	if len(errs) != 0 {
		t.Fatalf("Notify returned errors: %v", errs)
	}
	if len(n1.notifications) != 1 {
		t.Errorf("n1 should have received 1 notification, got %d", len(n1.notifications))
	}
	if len(n2.notifications) != 0 {
		t.Errorf("n2 (unconfigured) should have received 0 notifications, got %d", len(n2.notifications))
	}
}

// ─── MultiNotifier NotifierNames ───────────────────────────────────────────

func TestMultiNotifier_NotifierNames(t *testing.T) {
	n1 := &stubNotifier{name: "n1", configured: true}
	n2 := &stubNotifier{name: "n2", configured: false}
	n3 := &stubNotifier{name: "n3", configured: true}
	m := NewMultiNotifier(n1, n2, n3)

	names := m.NotifierNames()
	if len(names) != 2 {
		t.Fatalf("NotifierNames len = %d, want 2 (only configured)", len(names))
	}
	if names[0] != "n1" || names[1] != "n3" {
		t.Errorf("NotifierNames = %v, want [n1 n3]", names)
	}
}

// ─── ChannelHealthStore Alerts ─────────────────────────────────────────────

func TestChannelHealthStore_Alerts_Empty(t *testing.T) {
	s := NewChannelHealthStore(t.TempDir())
	alerts := s.Alerts()
	if alerts != nil {
		t.Errorf("Alerts on empty store should return nil, got %v", alerts)
	}
}

func TestChannelHealthStore_Alerts_WithErrors(t *testing.T) {
	s := NewChannelHealthStore(t.TempDir())
	s.Record("ch-ok", "ok", "")
	s.Record("ch-err", "error", "connection refused")
	s.Record("ch-warn", "warn", "")

	alerts := s.Alerts()
	if len(alerts) == 0 {
		t.Fatal("expected alerts for channels with errors")
	}
	for _, a := range alerts {
		if a.ChannelID != "ch-err" && a.ChannelID != "ch-warn" {
			t.Errorf("unexpected alert for channel %q with status %q", a.ChannelID, a.Status)
		}
	}
}

func TestNewChannelHealthStore_WithoutPool(t *testing.T) {
	s := NewChannelHealthStore(t.TempDir())
	if s == nil {
		t.Fatal("NewChannelHealthStore without pool returned nil")
	}
}

func TestChannelHealthStore_WithRecordsFetched(t *testing.T) {
	s := NewChannelHealthStore(t.TempDir())
	s.Record("ch-test", "ok", "", WithRecordsFetched(42))
	rec := s.Get("ch-test")
	if rec != nil && rec.RecordsFetched != 42 {
		t.Errorf("RecordsFetched = %d, want 42", rec.RecordsFetched)
	}
}

func TestChannelHealthStore_WithSymbolsProcessed(t *testing.T) {
	s := NewChannelHealthStore(t.TempDir())
	s.Record("ch-test", "ok", "", WithSymbolsProcessed(100))
	rec := s.Get("ch-test")
	if rec != nil && rec.SymbolsProcessed != 100 {
		t.Errorf("SymbolsProcessed = %d, want 100", rec.SymbolsProcessed)
	}
}
