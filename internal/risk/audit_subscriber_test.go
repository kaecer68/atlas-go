package risk

import (
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
