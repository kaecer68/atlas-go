package monitoring

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestMonitor_AlertPersistedToStore(t *testing.T) {
	dir := t.TempDir()
	store, err := NewAlertStore(dir)
	if err != nil {
		t.Fatalf("NewAlertStore: %v", err)
	}

	m := NewMonitor()
	m.SetAlertStore(store)

	m.Alert(AlertLevelWarning, "test_cat", "test msg", nil)
	time.Sleep(50 * time.Millisecond)

	records, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("LoadAll len = %d, want 1", len(records))
	}
	if records[0].Rule != "test_cat" {
		t.Errorf("Rule = %q, want test_cat", records[0].Rule)
	}
	if records[0].Severity != "WARNING" {
		t.Errorf("Severity = %q, want WARNING", records[0].Severity)
	}
	if records[0].Message != "test msg" {
		t.Errorf("Message = %q, want test msg", records[0].Message)
	}
}

func TestMonitor_AlertDispatchedToNotifier(t *testing.T) {
	var mu sync.Mutex
	var received []domain.AlertRecord

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	n := NewWebhookNotifier(server.URL, nil)

	captureNotifier := &captureNotifier{
		inner:    n,
		mu:       &mu,
		received: &received,
	}

	m := NewMonitor()
	m.AddNotifier(captureNotifier)

	m.Alert(AlertLevelError, "err_cat", "err msg", nil)
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("received %d alerts, want 1", len(received))
	}
	if received[0].Rule != "err_cat" {
		t.Errorf("Rule = %q, want err_cat", received[0].Rule)
	}
	if received[0].Severity != "ERROR" {
		t.Errorf("Severity = %q, want ERROR", received[0].Severity)
	}
}

func TestMonitor_AlertSkipsUnconfiguredNotifier(t *testing.T) {
	m := NewMonitor()
	m.AddNotifier(NewWebhookNotifier("", nil))

	m.Alert(AlertLevelInfo, "cat", "msg", nil)
	time.Sleep(50 * time.Millisecond)
}

func TestMonitor_MultipleNotifiersAllDispatched(t *testing.T) {
	var mu sync.Mutex
	var count int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		count++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	m := NewMonitor()
	m.AddNotifier(NewWebhookNotifier(server.URL, nil))
	m.AddNotifier(NewWebhookNotifier(server.URL, nil))

	m.Alert(AlertLevelWarning, "cat", "msg", nil)

	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		if count == 2 {
			mu.Unlock()
			break
		}
		mu.Unlock()
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if count != 2 {
		t.Errorf("dispatch count = %d, want 2", count)
	}
}

func TestMonitor_ConvenienceMethodsPersistAlerts(t *testing.T) {
	dir := t.TempDir()
	store, err := NewAlertStore(dir)
	if err != nil {
		t.Fatalf("NewAlertStore: %v", err)
	}

	m := NewMonitor()
	m.SetAlertStore(store)

	m.Info("info_cat", "info msg", nil)
	m.Warning("warn_cat", "warn msg", nil)
	m.Error("err_cat", "err msg", nil)
	m.Critical("crit_cat", "crit msg", nil)
	time.Sleep(100 * time.Millisecond)

	records, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(records) != 4 {
		t.Fatalf("LoadAll len = %d, want 4", len(records))
	}

	wantSeverities := map[string]bool{"INFO": true, "WARNING": true, "ERROR": true, "CRITICAL": true}
	for _, r := range records {
		delete(wantSeverities, r.Severity)
	}
	if len(wantSeverities) != 0 {
		t.Errorf("missing severities: %v", wantSeverities)
	}
}

// captureNotifier wraps a Notifier to capture alerts for testing.
type captureNotifier struct {
	inner    Notifier
	mu       *sync.Mutex
	received *[]domain.AlertRecord
}

func (c *captureNotifier) Name() string       { return c.inner.Name() }
func (c *captureNotifier) IsConfigured() bool { return true }
func (c *captureNotifier) Notify(a domain.AlertRecord) error {
	c.mu.Lock()
	*c.received = append(*c.received, a)
	c.mu.Unlock()
	return c.inner.Notify(a)
}
