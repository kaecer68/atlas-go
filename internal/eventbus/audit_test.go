package eventbus

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAuditTrailSubscriber_WritesToJSONL(t *testing.T) {
	bus := NewChannelEventBus(8)
	defer bus.Close()

	ledgerDir := t.TempDir()
	sub := NewAuditTrailSubscriber(bus, ledgerDir)
	sub.Enable()

	bus.Publish(BusEvent{
		ID:        "evt-1",
		Type:      EventSimulationStart,
		Timestamp: time.Now(),
		Payload: map[string]any{
			"session_id": "sess-1",
			"as_of":      "2026-06-21",
		},
		SchemaVersion: 1,
	})
	bus.Publish(BusEvent{
		ID:        "evt-2",
		Type:      EventRegimeChange,
		Timestamp: time.Now(),
		Payload: RegimeEventPayload{
			OldRegime:    "bull",
			NewRegime:    "bear",
			Confidence:   0.85,
			DeterminedBy: "test",
		},
		SchemaVersion: 1,
	})
	bus.Publish(BusEvent{
		ID:        "evt-3",
		Type:      EventSimulationComplete,
		Timestamp: time.Now(),
		Payload: map[string]any{
			"session_id":      "sess-1",
			"portfolio_value": 1000000.0,
			"order_count":     5,
			"position_count":  3,
		},
		SchemaVersion: 1,
	})

	path := filepath.Join(ledgerDir, "events.jsonl")
	waitForLines(t, path, 3)

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open events.jsonl: %v", err)
	}
	defer func() { _ = file.Close() }()

	var events []BusEvent
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var ev BusEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		events = append(events, ev)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan events.jsonl: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	wantTypes := map[EventType]bool{
		EventSimulationStart:    true,
		EventRegimeChange:       true,
		EventSimulationComplete: true,
	}
	gotTypes := make(map[EventType]bool)
	for _, ev := range events {
		gotTypes[ev.Type] = true
	}
	for want := range wantTypes {
		if !gotTypes[want] {
			t.Errorf("missing event type %q in audit trail", want)
		}
	}
}

func TestAuditTrailSubscriber_DisabledSkipsWrite(t *testing.T) {
	bus := NewChannelEventBus(8)
	defer bus.Close()

	ledgerDir := t.TempDir()
	_ = NewAuditTrailSubscriber(bus, ledgerDir)
	// intentionally left disabled

	bus.Publish(BusEvent{
		ID:            "evt-1",
		Type:          EventSimulationStart,
		Timestamp:     time.Now(),
		Payload:       map[string]any{"session_id": "sess-1"},
		SchemaVersion: 1,
	})
	bus.Publish(BusEvent{
		ID:            "evt-2",
		Type:          EventRegimeChange,
		Timestamp:     time.Now(),
		Payload:       RegimeEventPayload{OldRegime: "bull", NewRegime: "bear", Confidence: 0.85, DeterminedBy: "test"},
		SchemaVersion: 1,
	})

	path := filepath.Join(ledgerDir, "events.jsonl")
	for range 50 {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected events.jsonl to not exist when disabled, got err=%v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAuditTrailSubscriber_Close(t *testing.T) {
	bus := NewChannelEventBus(8)
	ledgerDir := t.TempDir()
	_ = NewAuditTrailSubscriber(bus, ledgerDir)

	if err := bus.Close(); err != nil {
		t.Fatalf("bus.Close() returned error: %v", err)
	}
}

func waitForLines(t *testing.T, path string, want int) {
	t.Helper()
	for range 100 {
		if count, _ := countLines(path); count >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d lines in %s", want, path)
}

func countLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}
