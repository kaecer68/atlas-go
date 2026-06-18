package live

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/eventbus"
)

func TestRiskAuditLog_RecordsRiskGateBlocked(t *testing.T) {
	bus := NewChannelEventBus(16)
	t.Cleanup(func() { _ = bus.Close() })

	log := NewRiskAuditLog()
	sub := log.Subscribe(bus)
	t.Cleanup(sub.Cancel)

	bus.Publish(eventbus.BusEvent{
		Type: eventbus.EventOrderError,
		Payload: eventbus.OrderErrorEventPayload{
			OrderID:      "oid-1",
			Symbol:       "2330",
			Side:         "buy",
			Price:        100.0,
			Quantity:     10,
			ErrorCode:    "risk_gate_blocked",
			ErrorMessage: "risk gate: trading halted",
			Attempts:     1,
			LastStatus:   "blocked",
			Timestamp:    time.Now(),
		},
	})

	time.Sleep(50 * time.Millisecond)

	entries := log.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].OrderID != "oid-1" {
		t.Fatalf("expected OrderID oid-1, got %s", entries[0].OrderID)
	}
	if entries[0].Symbol != "2330" {
		t.Fatalf("expected Symbol 2330, got %s", entries[0].Symbol)
	}
}

func TestRiskAuditLog_IgnoresNonBlockedErrors(t *testing.T) {
	bus := NewChannelEventBus(16)
	t.Cleanup(func() { _ = bus.Close() })

	log := NewRiskAuditLog()
	sub := log.Subscribe(bus)
	t.Cleanup(sub.Cancel)

	bus.Publish(eventbus.BusEvent{
		Type: eventbus.EventOrderError,
		Payload: eventbus.OrderErrorEventPayload{
			OrderID:      "oid-2",
			Symbol:       "2603",
			ErrorCode:    "rejected",
			ErrorMessage: "broker rejected",
			Attempts:     1,
		},
	})

	time.Sleep(50 * time.Millisecond)

	entries := log.Entries()
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries for non-blocked error, got %d", len(entries))
	}
}
