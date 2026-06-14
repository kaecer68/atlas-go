package monitoring

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func newTestAlertStore(t *testing.T) *AlertStore {
	t.Helper()
	store, err := NewAlertStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewAlertStore: %v", err)
	}
	return store
}

func TestAutoHandler_isSuppressed_Expired(t *testing.T) {
	store := newTestAlertStore(t)
	h := NewAutoHandler(store, []SuppressRule{{Category: "cpu", Duration: -time.Hour}})
	if h.isSuppressed(Alert{Category: "cpu"}) {
		t.Error("expected expired suppression to be cleared")
	}
}

func TestAutoHandler_isSuppressed_StaticWildcard(t *testing.T) {
	store := newTestAlertStore(t)
	h := NewAutoHandler(store, []SuppressRule{{Category: "", Duration: time.Hour}})
	if !h.isSuppressed(Alert{Category: "anything"}) {
		t.Error("expected wildcard suppression to match")
	}
}

func TestAutoHandler_Handle_InfoAutoAcknowledge(t *testing.T) {
	store := newTestAlertStore(t)
	if err := store.Save(domain.AlertRecord{ID: "i1", Rule: "r1", Status: domain.AlertStatusTriggered}); err != nil {
		t.Fatal(err)
	}
	h := NewAutoHandler(store, nil)
	h.Handle(Alert{ID: "i1", Level: AlertLevelInfo, Category: "r1"})
	all, err := store.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || !all[0].Acknowledged {
		t.Error("expected INFO alert to be auto-acknowledged")
	}
}

func TestAutoHandler_Handle_Suppressed(t *testing.T) {
	store := newTestAlertStore(t)
	h := NewAutoHandler(store, []SuppressRule{{Category: "cpu", Duration: time.Hour}})
	h.Handle(Alert{ID: "a1", Level: AlertLevelWarning, Category: "cpu"})
	all, err := store.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Error("expected suppressed alert not to be stored")
	}
}

func TestAutoHandler_Recover(t *testing.T) {
	store := newTestAlertStore(t)
	if err := store.Save(domain.AlertRecord{ID: "r1", Rule: "cpu", Status: domain.AlertStatusTriggered}); err != nil {
		t.Fatal(err)
	}
	h := NewAutoHandler(store, nil)
	h.Recover("cpu")
	all, err := store.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Status != domain.AlertStatusResolved {
		t.Errorf("expected resolved, got %v", all[0].Status)
	}
}
