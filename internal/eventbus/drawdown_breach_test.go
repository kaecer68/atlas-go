package eventbus

import (
	"context"
	"testing"
	"time"
)

// TestPublishDrawdownBreach verifies that ChannelEventBus.PublishDrawdownBreach
// dispatches an EventDrawdownBreach event with the supplied payload.
func TestPublishDrawdownBreach(t *testing.T) {
	bus := NewChannelEventBus(8)
	defer bus.Close()

	received := make(chan BusEvent, 4)
	sub := bus.SubscribeAll(func(_ context.Context, e BusEvent) error {
		select {
		case received <- e:
		default:
		}
		return nil
	})
	defer sub.Cancel()

	payload := DrawdownBreachPayload{
		CurrentDrawdown: 0.09,
		MaxDrawdownPct:  0.08,
		PortfolioValue:  91000,
		PeakValue:       100000,
		Timestamp:       time.Now(),
	}
	bus.PublishDrawdownBreach(payload)

	select {
	case e := <-received:
		if e.Type != EventDrawdownBreach {
			t.Errorf("expected EventDrawdownBreach, got type=%s", e.Type)
		}
		if e.Severity != "critical" {
			t.Errorf("expected severity=critical, got %s", e.Severity)
		}
		// Verify payload round-trips
		got, ok := e.Payload.(DrawdownBreachPayload)
		if !ok {
			t.Fatalf("expected payload type DrawdownBreachPayload, got %T", e.Payload)
		}
		if got.CurrentDrawdown != payload.CurrentDrawdown {
			t.Errorf("CurrentDrawdown mismatch: got %v, want %v", got.CurrentDrawdown, payload.CurrentDrawdown)
		}
		if got.MaxDrawdownPct != payload.MaxDrawdownPct {
			t.Errorf("MaxDrawdownPct mismatch: got %v, want %v", got.MaxDrawdownPct, payload.MaxDrawdownPct)
		}
		if got.PortfolioValue != payload.PortfolioValue {
			t.Errorf("PortfolioValue mismatch: got %v, want %v", got.PortfolioValue, payload.PortfolioValue)
		}
		if got.PeakValue != payload.PeakValue {
			t.Errorf("PeakValue mismatch: got %v, want %v", got.PeakValue, payload.PeakValue)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for EventDrawdownBreach event")
	}
}

// TestDrawdownBreachPayload_Fields verifies DrawdownBreachPayload has the
// expected JSON tags (snake_case per atlas-go conventions).
func TestDrawdownBreachPayload_Fields(t *testing.T) {
	p := DrawdownBreachPayload{}
	p.CurrentDrawdown = 0.09
	p.MaxDrawdownPct = 0.08
	p.PortfolioValue = 91000
	p.PeakValue = 100000
	p.Timestamp = time.Unix(0, 0)

	if p.CurrentDrawdown != 0.09 {
		t.Errorf("CurrentDrawdown not set correctly")
	}
	if p.MaxDrawdownPct != 0.08 {
		t.Errorf("MaxDrawdownPct not set correctly")
	}
}
