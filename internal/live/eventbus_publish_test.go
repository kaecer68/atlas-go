package live

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestPublishMarketSnapshot(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var received atomic.Int32
	sub := bus.Subscribe(EventMarketSnapshot, func(ctx context.Context, event BusEvent) error {
		received.Add(1)
		return nil
	})
	defer sub.Cancel()

	quote := domain.Quote{Symbol: "2330", Open: 500, High: 510, Low: 495, Last: 505}
	bus.PublishMarketSnapshot(quote)

	time.Sleep(100 * time.Millisecond)
	if received.Load() != 1 {
		t.Fatalf("expected 1 market snapshot event, got %d", received.Load())
	}
}

func TestPublishRegimeChange(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var received atomic.Int32
	bus.Subscribe(EventRegimeChange, func(ctx context.Context, event BusEvent) error {
		received.Add(1)
		payload := event.Payload.(RegimeEventPayload)
		if payload.NewRegime != domain.RegimeRiskOn {
			t.Errorf("unexpected new regime: %v", payload.NewRegime)
		}
		return nil
	})

	bus.PublishRegimeChange(domain.RegimeNeutral, domain.RegimeRiskOn, 0.85, "prism")

	time.Sleep(100 * time.Millisecond)
	if received.Load() != 1 {
		t.Fatalf("expected 1 regime change event, got %d", received.Load())
	}
}

func TestPublishPositionUpdate(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var received atomic.Int32
	bus.Subscribe(EventPositionUpdate, func(ctx context.Context, event BusEvent) error {
		received.Add(1)
		return nil
	})

	position := domain.Position{Symbol: "2330", Quantity: 100, AverageCost: 500}
	bus.PublishPositionUpdate("2330", position, "added")

	time.Sleep(100 * time.Millisecond)
	if received.Load() != 1 {
		t.Fatalf("expected 1 position update event, got %d", received.Load())
	}
}

func TestPublishRecommendation(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var received atomic.Int32
	bus.Subscribe(EventAgentRecommendation, func(ctx context.Context, event BusEvent) error {
		received.Add(1)
		return nil
	})

	recs := []domain.Recommendation{{Symbol: "2330", Conviction: 80, Side: "buy"}}
	bus.PublishRecommendation("growth-momentum-01", recs)

	time.Sleep(100 * time.Millisecond)
	if received.Load() != 1 {
		t.Fatalf("expected 1 recommendation event, got %d", received.Load())
	}
}

func TestSubscribeAndUnsubscribe(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var received atomic.Int32
	sub := bus.Subscribe(EventMarketSnapshot, func(ctx context.Context, event BusEvent) error {
		received.Add(1)
		return nil
	})

	bus.Publish(BusEvent{ID: "1", Type: EventMarketSnapshot, Timestamp: time.Now()})
	time.Sleep(50 * time.Millisecond)
	if received.Load() != 1 {
		t.Fatalf("expected 1 event before unsubscribe, got %d", received.Load())
	}

	sub.Cancel()

	bus.Publish(BusEvent{ID: "2", Type: EventMarketSnapshot, Timestamp: time.Now()})
	time.Sleep(50 * time.Millisecond)
	if received.Load() != 1 {
		t.Fatalf("expected no additional events after unsubscribe, got %d", received.Load())
	}
}

func TestSubscribeAll(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	var received atomic.Int32
	sub := bus.SubscribeAll(func(ctx context.Context, event BusEvent) error {
		received.Add(1)
		return nil
	})
	defer sub.Cancel()

	bus.Publish(BusEvent{ID: "1", Type: EventSystemStart, Timestamp: time.Now()})
	bus.Publish(BusEvent{ID: "2", Type: EventSystemError, Timestamp: time.Now()})

	time.Sleep(100 * time.Millisecond)
	if received.Load() != 2 {
		t.Fatalf("expected 2 events from SubscribeAll, got %d", received.Load())
	}
}

func TestEventBusStats(t *testing.T) {
	bus := NewChannelEventBus(64)
	defer bus.Close()

	bus.Subscribe(EventMarketSnapshot, func(ctx context.Context, event BusEvent) error { return nil })
	bus.Subscribe(EventMarketSnapshot, func(ctx context.Context, event BusEvent) error { return nil })
	bus.Subscribe(EventRegimeChange, func(ctx context.Context, event BusEvent) error { return nil })
	bus.SubscribeAll(func(ctx context.Context, event BusEvent) error { return nil })

	stats := bus.Stats()
	if stats["subscribers_total"] != 4 {
		t.Fatalf("expected 4 total subscribers, got %v", stats["subscribers_total"])
	}
	if stats["subscribers_by_type"] != 2 {
		t.Fatalf("expected 2 subscriber types, got %v", stats["subscribers_by_type"])
	}
	if stats["channel_capacity"] != 64 {
		t.Fatalf("expected channel capacity 64, got %v", stats["channel_capacity"])
	}
}
