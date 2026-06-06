package risk

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/eventbus"
)

func TestNewAuditSubscriber(t *testing.T) {
	bus := eventbus.NewChannelEventBus(16)
	a := NewAuditSubscriber(bus)
	if a == nil {
		t.Fatal("NewAuditSubscriber returned nil")
	}
	if a.bus != bus {
		t.Error("bus not set")
	}
}

func TestAuditSubscriber_LogDoesNotPanic(t *testing.T) {
	bus := eventbus.NewChannelEventBus(16)
	_ = NewAuditSubscriber(bus)

	events := []eventbus.BusEvent{
		{ID: "e1", Type: eventbus.EventStopLossTriggered, Timestamp: time.Now(), Payload: map[string]any{"symbol": "2330.TW"}},
		{ID: "e2", Type: eventbus.EventTakeProfitTriggered, Timestamp: time.Now()},
		{ID: "e3", Type: eventbus.EventRiskAlert, Timestamp: time.Now()},
		{ID: "e4", Type: eventbus.EventOrderFilled, Timestamp: time.Now()},
		{ID: "e5", Type: eventbus.EventOrderRejected, Timestamp: time.Now()},
		{ID: "e6", Type: eventbus.EventOrderPlaced, Timestamp: time.Now()},
	}
	for _, ev := range events {
		bus.Publish(ev)
	}
	time.Sleep(100 * time.Millisecond)
}

func TestAuditSubscriber_RegistersSubscribers(t *testing.T) {
	bus := eventbus.NewChannelEventBus(16)
	_ = NewAuditSubscriber(bus)

	stats := bus.Stats()
	total := stats["subscribers_total"].(int)
	if total == 0 {
		t.Error("expected subscribers to be registered")
	}
}

func TestAuditSubscriber_PersistsRiskEventsWhenSessionIDSet(t *testing.T) {
	t.Setenv("ATLAS_LEDGER_DIR", t.TempDir())

	bus := eventbus.NewChannelEventBus(16)
	t.Cleanup(func() { _ = bus.Close() })

	a := NewAuditSubscriber(bus)
	a.SetSessionID("session-20260530-daily")

	event := eventbus.BusEvent{
		ID:        "evt-risk-1",
		Type:      eventbus.EventRiskAlert,
		Timestamp: time.Now(),
		Payload:   map[string]any{"symbol": "2330.TW", "reason": "drawdown"},
	}
	bus.Publish(event)

	path := filepath.Join(os.Getenv("ATLAS_LEDGER_DIR"), "sessions", "session-20260530-daily", "risk_events.jsonl")
	stored := waitForRiskEvent(t, path)

	if stored.ID != event.ID {
		t.Fatalf("expected event id %q, got %q", event.ID, stored.ID)
	}
	if stored.Type != event.Type {
		t.Fatalf("expected event type %q, got %q", event.Type, stored.Type)
	}
	payload, ok := stored.Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected payload map, got %T", stored.Payload)
	}
	if payload["symbol"] != "2330.TW" {
		t.Fatalf("expected symbol 2330.TW, got %#v", payload["symbol"])
	}
}

func TestAuditSubscriber_DoesNotPersistWithoutSessionID(t *testing.T) {
	ledgerDir := t.TempDir()
	t.Setenv("ATLAS_LEDGER_DIR", ledgerDir)

	bus := eventbus.NewChannelEventBus(16)
	t.Cleanup(func() { _ = bus.Close() })

	_ = NewAuditSubscriber(bus)
	bus.Publish(eventbus.BusEvent{
		ID:        "evt-risk-no-session",
		Type:      eventbus.EventOrderPlaced,
		Timestamp: time.Now(),
		Payload:   map[string]any{"symbol": "2317.TW"},
	})

	time.Sleep(100 * time.Millisecond)

	matches, err := filepath.Glob(filepath.Join(ledgerDir, "sessions", "*", "risk_events.jsonl"))
	if err != nil {
		t.Fatalf("glob risk events: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no risk event artifacts, found %v", matches)
	}
}

func waitForRiskEvent(t *testing.T, path string) eventbus.BusEvent {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		f, err := os.Open(path)
		if err == nil {
			defer func() { _ = f.Close() }()
			scanner := bufio.NewScanner(f)
			if scanner.Scan() {
				var event eventbus.BusEvent
				if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
					t.Fatalf("unmarshal persisted event: %v", err)
				}
				return event
			}
			if err := scanner.Err(); err != nil {
				t.Fatalf("scan persisted event: %v", err)
			}
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for persisted risk event at %s", path)
	return eventbus.BusEvent{}
}
