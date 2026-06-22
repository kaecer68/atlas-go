package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
)

type driftRecordingBus struct {
	mu     sync.Mutex
	events []eventbus.BusEvent
}

func (b *driftRecordingBus) Publish(ev eventbus.BusEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, ev)
}
func (b *driftRecordingBus) Subscribe(eventbus.EventType, eventbus.EventHandler) eventbus.Subscription {
	return eventbus.Subscription{Cancel: func() {}}
}
func (b *driftRecordingBus) SubscribeAll(eventbus.EventHandler) eventbus.Subscription {
	return eventbus.Subscription{Cancel: func() {}}
}
func (b *driftRecordingBus) Close() error { return nil }
func (b *driftRecordingBus) snapshot() []eventbus.BusEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]eventbus.BusEvent, len(b.events))
	copy(out, b.events)
	return out
}

func dispatchPosition(d *driftDetector, symbol string, value float64, changeType string) {
	_ = d.onPositionUpdate(context.Background(), eventbus.BusEvent{
		Type:      eventbus.EventPositionUpdate,
		Timestamp: time.Now(),
		Payload: eventbus.PositionEventPayload{
			Symbol:     symbol,
			ChangeType: changeType,
			Position: domain.Position{
				Symbol:       symbol,
				Quantity:     1000,
				MarketValue:  value,
				CurrentPrice: value / 1000,
			},
		},
	})
}

func TestDriftDetector_NoEmitOnLowConcentration(t *testing.T) {
	bus := &driftRecordingBus{}
	d := &driftDetector{
		bus:       bus,
		snapshots: make(map[string]*driftSnapshot),
	}
	dispatchPosition(d, "2330", 200_000, "added")
	dispatchPosition(d, "2454", 200_000, "added")
	dispatchPosition(d, "2317", 200_000, "added")
	dispatchPosition(d, "2881", 200_000, "added")

	d.checkPeriod(time.Now())
	if got := bus.snapshot(); len(got) != 0 {
		t.Fatalf("balanced 4-symbol portfolio (25%% each, threshold 25%% exclusive) should not emit, got %d", len(got))
	}
}

func TestDriftDetector_EmitsOnHighConcentration(t *testing.T) {
	bus := &driftRecordingBus{}
	d := &driftDetector{
		bus:       bus,
		snapshots: make(map[string]*driftSnapshot),
	}
	dispatchPosition(d, "2330", 700_000, "added")
	dispatchPosition(d, "2454", 100_000, "added")
	dispatchPosition(d, "2317", 100_000, "added")
	dispatchPosition(d, "2881", 100_000, "added")

	d.checkPeriod(time.Now())
	d.checkPeriod(time.Now().Add(5 * time.Minute))
	got := bus.snapshot()
	if len(got) != 1 {
		t.Fatalf("concentrated portfolio (70%% in 2330) should emit on second check, got %d", len(got))
	}
	if got[0].Type != eventbus.EventDriftDetected {
		t.Errorf("unexpected event type %s", got[0].Type)
	}
	payload := got[0].Payload.(map[string]any)
	if payload["max_symbol"] != "2330" {
		t.Errorf("want max_symbol=2330, got %v", payload["max_symbol"])
	}
	maxW := payload["max_concentration"].(float64)
	if maxW < 0.25 {
		t.Errorf("expected concentration >= 0.25, got %f", maxW)
	}
	reasons := payload["reasons"].([]string)
	if len(reasons) != 1 || reasons[0] != "concentration" {
		t.Errorf("expected reasons=[concentration], got %v", reasons)
	}
}

func TestDriftDetector_EmitsOnHighTurnover(t *testing.T) {
	bus := &driftRecordingBus{}
	d := &driftDetector{
		bus:       bus,
		snapshots: make(map[string]*driftSnapshot),
	}
	dispatchPosition(d, "2330", 200_000, "added")
	d.checkPeriod(time.Now())
	if got := bus.snapshot(); len(got) != 0 {
		t.Fatalf("first check should establish baseline, got %d events", len(got))
	}

	dispatchPosition(d, "2330", 300_000, "updated")
	dispatchPosition(d, "2454", 200_000, "added")
	dispatchPosition(d, "2317", 200_000, "added")

	d.checkPeriod(time.Now())
	got := bus.snapshot()
	if len(got) != 1 {
		t.Fatalf("50%% turnover should emit, got %d", len(got))
	}
	payload := got[0].Payload.(map[string]any)
	reasons := payload["reasons"].([]string)
	hasTurnover := false
	for _, r := range reasons {
		if r == "turnover" {
			hasTurnover = true
		}
	}
	if !hasTurnover {
		t.Errorf("expected reasons include turnover, got %v", reasons)
	}
}

func TestDriftDetector_RemovedSymbolCleared(t *testing.T) {
	bus := &driftRecordingBus{}
	d := &driftDetector{
		bus:       bus,
		snapshots: make(map[string]*driftSnapshot),
	}
	dispatchPosition(d, "2330", 200_000, "added")
	dispatchPosition(d, "2454", 200_000, "added")
	dispatchPosition(d, "2330", 0, "removed")

	d.mu.Lock()
	count := len(d.snapshots)
	d.mu.Unlock()
	if count != 1 {
		t.Fatalf("after remove, want 1 symbol, got %d", count)
	}
}

func TestDriftDetector_EmptyPortfolioNoEmit(t *testing.T) {
	bus := &driftRecordingBus{}
	d := &driftDetector{
		bus:       bus,
		snapshots: make(map[string]*driftSnapshot),
	}
	d.checkPeriod(time.Now())
	if got := bus.snapshot(); len(got) != 0 {
		t.Fatalf("empty portfolio should not emit, got %d", len(got))
	}
}

func TestAbsDiff(t *testing.T) {
	if got := absDiff(10, 5); got != 5 {
		t.Errorf("absDiff(10,5)=%f want 5", got)
	}
	if got := absDiff(5, 10); got != 5 {
		t.Errorf("absDiff(5,10)=%f want 5", got)
	}
	if got := absDiff(7, 7); got != 0 {
		t.Errorf("absDiff(7,7)=%f want 0", got)
	}
}
