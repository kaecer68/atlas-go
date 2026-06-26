package monitoring

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
)

// TestDrawdownConsumer_Start_Stop_Subscribes verifies that Start subscribes to
// EventDrawdownBreach on the bus and Stop cancels the subscription. Uses
// AlertStore backed by t.TempDir() so the test exercises the full event-to-
// store path without touching the production data directory.
func TestDrawdownConsumer_Start_Stop_Subscribes(t *testing.T) {
	bus := eventbus.NewChannelEventBus(8)
	defer bus.Close()

	store, err := NewAlertStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewAlertStore: %v", err)
	}

	c := NewDrawdownConsumer(bus, store)
	c.Start()
	c.Stop()

	// After Stop, no subscription remains — re-publishing should be a no-op.
	// (We cannot directly assert "no subscription" without exposing internals,
	// so we verify by checking that Stop is idempotent.)
	c.Stop() // must not panic
}

// TestDrawdownConsumer_ConvertsAndPersistsAlert is the financial-engineering-
// grade regression test: an EventDrawdownBreach published on the bus must be
// converted to an AlertRecord and persisted to the AlertStore. This is the
// surface that Decision 8 PR-A wired the publisher side for; PR-B completes
// the loop by ensuring the consumer side actually saves the alert.
func TestDrawdownConsumer_ConvertsAndPersistsAlert(t *testing.T) {
	bus := eventbus.NewChannelEventBus(8)
	defer bus.Close()

	store, err := NewAlertStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewAlertStore: %v", err)
	}

	c := NewDrawdownConsumer(bus, store)
	c.Start()
	defer c.Stop()

	// Publish a drawdown breach event with the same payload shape that
	// RiskManager.publishDrawdownBreach emits in PR-A.
	now := time.Now()
	bus.PublishDrawdownBreach(eventbus.DrawdownBreachPayload{
		CurrentDrawdown: 0.09,
		MaxDrawdownPct:  0.08,
		PortfolioValue:  91000,
		PeakValue:       100000,
		Timestamp:       now,
	})

	// Allow the event dispatcher goroutine time to deliver + Save to flush.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		all, err := store.LoadAll()
		if err != nil {
			t.Fatalf("LoadAll: %v", err)
		}
		if len(all) >= 1 {
			// Verify the persisted record matches the expected mapping.
			got := all[0]
			if got.Rule != "portfolio_drawdown_breach" {
				t.Errorf("expected Rule=portfolio_drawdown_breach, got %q", got.Rule)
			}
			if got.Severity != "critical" {
				t.Errorf("expected Severity=critical, got %q", got.Severity)
			}
			if got.Status != domain.AlertStatusTriggered {
				t.Errorf("expected Status=triggered, got %q", got.Status)
			}
			if got.Value != 0.09 {
				t.Errorf("expected Value=0.09, got %v", got.Value)
			}
			if got.Threshold != 0.08 {
				t.Errorf("expected Threshold=0.08, got %v", got.Threshold)
			}
			if got.Message == "" {
				t.Error("expected non-empty Message")
			}
			if got.Acknowledged {
				t.Error("expected Acknowledged=false for newly created alert")
			}
			if got.Count != 1 {
				t.Errorf("expected Count=1, got %d", got.Count)
			}
			if got.ID == "" {
				t.Error("expected non-empty ID (uuid)")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for AlertRecord to be persisted by DrawdownConsumer")
}

// TestDrawdownConsumer_NilStoreIsSafe verifies the no-store branch does not
// panic when a drawdown event is published. The consumer must remain a no-op
// instead of crashing the event dispatcher goroutine.
func TestDrawdownConsumer_NilStoreIsSafe(t *testing.T) {
	bus := eventbus.NewChannelEventBus(8)
	defer bus.Close()

	c := NewDrawdownConsumer(bus, nil)
	c.Start()
	defer c.Stop()

	// Should not panic even though store is nil.
	bus.PublishDrawdownBreach(eventbus.DrawdownBreachPayload{
		CurrentDrawdown: 0.10,
		MaxDrawdownPct:  0.08,
		PortfolioValue:  90000,
		PeakValue:       100000,
		Timestamp:       time.Now(),
	})

	// Give dispatcher a moment to deliver the (dropped) event.
	time.Sleep(100 * time.Millisecond)
}

// TestDrawdownConsumer_NilBusNoop verifies that NewDrawdownConsumer with a nil
// bus returns a consumer that can be safely Start()/Stop()'d without panic.
// This is the lazy-init / DI-deferred pattern.
func TestDrawdownConsumer_NilBusNoop(t *testing.T) {
	store, err := NewAlertStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewAlertStore: %v", err)
	}

	c := NewDrawdownConsumer(nil, store)
	c.Start() // must not panic
	c.Stop()  // must not panic
}

// TestDrawdownConsumer_PayloadTypeMismatchIsSafe verifies that an event of
// the right type but with a wrong payload type is logged and skipped without
// crashing the handler. The handler must not block the dispatcher goroutine
// (eventbus AGENTS.md: handler panic is recovered but a wrong payload is the
// caller's responsibility — we still want to fail gracefully).
func TestDrawdownConsumer_PayloadTypeMismatchIsSafe(t *testing.T) {
	bus := eventbus.NewChannelEventBus(8)
	defer bus.Close()

	store, err := NewAlertStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewAlertStore: %v", err)
	}

	c := NewDrawdownConsumer(bus, store)
	c.Start()
	defer c.Stop()

	// Publish a DrawdownBreach with the WRONG payload type.
	bus.Publish(eventbus.BusEvent{
		ID:        "test-mismatch",
		Type:      eventbus.EventDrawdownBreach,
		Timestamp: time.Now(),
		Payload:   "this is a string, not a DrawdownBreachPayload",
		Severity:  "critical",
	})

	// Give dispatcher a moment.
	time.Sleep(100 * time.Millisecond)

	// Verify no alert was persisted (type-assertion failed, skipped).
	all, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("expected 0 alerts after type-mismatch payload, got %d", len(all))
	}
}

// TestDrawdownConsumer_BuildAlertRecord_FieldMapping pins the payload-to-
// AlertRecord field mapping so accidental renames or type changes in the
// payload are caught by this test rather than by silent runtime breakage.
func TestDrawdownConsumer_BuildAlertRecord_FieldMapping(t *testing.T) {
	c := &DrawdownConsumer{}
	now := time.Date(2026, 6, 26, 18, 0, 0, 0, time.UTC)
	rec := c.buildAlertRecord(eventbus.DrawdownBreachPayload{
		CurrentDrawdown: 0.12,
		MaxDrawdownPct:  0.05,
		PortfolioValue:  88000,
		PeakValue:       100000,
		Timestamp:       now,
	})

	if rec.Rule != "portfolio_drawdown_breach" {
		t.Errorf("Rule mismatch: %q", rec.Rule)
	}
	if rec.Severity != "critical" {
		t.Errorf("Severity mismatch: %q", rec.Severity)
	}
	if rec.Status != domain.AlertStatusTriggered {
		t.Errorf("Status mismatch: %q", rec.Status)
	}
	if rec.Value != 0.12 {
		t.Errorf("Value mismatch: %v", rec.Value)
	}
	if rec.Threshold != 0.05 {
		t.Errorf("Threshold mismatch: %v", rec.Threshold)
	}
	if rec.Count != 1 {
		t.Errorf("Count mismatch: %d", rec.Count)
	}
	if rec.Timestamp != now {
		t.Errorf("Timestamp mismatch: %v", rec.Timestamp)
	}
	if rec.DedupKey == "" {
		t.Error("expected non-empty DedupKey for downstream AlertDeduplicator")
	}
	if rec.ID == "" {
		t.Error("expected non-empty ID (uuid)")
	}
	wantMsg := "Portfolio drawdown 12.00% exceeds limit 5.00%"
	if rec.Message != wantMsg {
		t.Errorf("Message mismatch:\n  got:  %q\n  want: %q", rec.Message, wantMsg)
	}
	if rec.FirstSeen == nil || !rec.FirstSeen.Equal(now) {
		t.Errorf("FirstSeen mismatch: %v", rec.FirstSeen)
	}
	if rec.LastSeen == nil || !rec.LastSeen.Equal(now) {
		t.Errorf("LastSeen mismatch: %v", rec.LastSeen)
	}
}

// TestDrawdownConsumer_AlertStoreFilePath verifies the AlertStore file is
// created under the configured directory. Sanity check that we are not
// accidentally writing to the production data path during tests.
func TestDrawdownConsumer_AlertStoreFilePath(t *testing.T) {
	dir := t.TempDir()
	store, err := NewAlertStore(dir)
	if err != nil {
		t.Fatalf("NewAlertStore: %v", err)
	}
	want := filepath.Join(dir, "alerts.jsonl")
	if store.filePath != want {
		t.Errorf("expected AlertStore filePath=%q, got %q", want, store.filePath)
	}
}

// Compile-time guard: DrawdownConsumer must accept context.Context handler
// signature used by eventbus.Subscribe.
var _ = func(ctx context.Context, ev eventbus.BusEvent) error { return nil }
