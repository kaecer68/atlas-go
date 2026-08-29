package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
)

type regimeRecordingBus struct {
	mu     sync.Mutex
	events []eventbus.BusEvent
	subs   map[eventbus.EventType][]eventbus.EventHandler
}

func newRegimeRecordingBus() *regimeRecordingBus {
	return &regimeRecordingBus{subs: make(map[eventbus.EventType][]eventbus.EventHandler)}
}

func (b *regimeRecordingBus) Publish(ev eventbus.BusEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, ev)
}

func (b *regimeRecordingBus) Subscribe(t eventbus.EventType, h eventbus.EventHandler) eventbus.Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[t] = append(b.subs[t], h)
	return eventbus.Subscription{
		ID:        string(t),
		EventType: t,
		Cancel:    func() {},
	}
}

func (b *regimeRecordingBus) SubscribeAll(h eventbus.EventHandler) eventbus.Subscription {
	return eventbus.Subscription{Cancel: func() {}}
}

func (b *regimeRecordingBus) Close() error { return nil }

func (b *regimeRecordingBus) dispatch(t eventbus.EventType, ev eventbus.BusEvent) {
	b.mu.Lock()
	handlers := b.subs[t]
	b.mu.Unlock()
	for _, h := range handlers {
		_ = h(context.Background(), ev)
	}
}

func (b *regimeRecordingBus) snapshot() []eventbus.BusEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]eventbus.BusEvent, len(b.events))
	copy(out, b.events)
	return out
}

func publishRegimeChange(bus *regimeRecordingBus, oldR, newR domain.Regime, confidence float64, ts time.Time) {
	ev := eventbus.BusEvent{
		Type:      eventbus.EventRegimeChange,
		Timestamp: ts,
		Payload: eventbus.RegimeEventPayload{
			OldRegime:    oldR,
			NewRegime:    newR,
			Confidence:   confidence,
			DeterminedBy: "test",
		},
	}
	bus.dispatch(eventbus.EventRegimeChange, ev)
}

func dispatchRegimeChangeToHandler(d *regimeDebouncer, oldR, newR domain.Regime, confidence float64, ts time.Time) {
	_ = d.onRegimeChange(context.Background(), eventbus.BusEvent{
		Type:      eventbus.EventRegimeChange,
		Timestamp: ts,
		Payload: eventbus.RegimeEventPayload{
			OldRegime:    oldR,
			NewRegime:    newR,
			Confidence:   confidence,
			DeterminedBy: "test",
		},
	})
}

func TestRegimeDebouncer_NoEmitWithoutEvent(t *testing.T) {
	bus := newRegimeRecordingBus()
	d := &regimeDebouncer{bus: bus}
	d.check(time.Now())
	if got := bus.snapshot(); len(got) != 0 {
		t.Fatalf("want 0 events, got %d", len(got))
	}
}

func TestRegimeDebouncer_EmitsAfterStabilityWindow(t *testing.T) {
	bus := newRegimeRecordingBus()
	d := &regimeDebouncer{bus: bus}

	t0 := time.Now()
	dispatchRegimeChangeToHandler(d, domain.Regime("bull"), domain.Regime("bear"), 0.8, t0)

	d.check(t0.Add(10 * time.Second))
	if got := bus.snapshot(); len(got) != 0 {
		t.Fatalf("10s after change should not emit, got %d events", len(got))
	}

	d.check(t0.Add(31 * time.Second))
	got := bus.snapshot()
	if len(got) != 1 {
		t.Fatalf("31s after change should emit, got %d events", len(got))
	}
	if got[0].Type != eventbus.EventRegimeChangeConfirmed {
		t.Errorf("unexpected event type %s", got[0].Type)
	}
	payload, ok := got[0].Payload.(map[string]any)
	if !ok {
		t.Fatalf("unexpected payload type %T", got[0].Payload)
	}
	if payload["new_regime"] != "bear" {
		t.Errorf("want new_regime=bear, got %v", payload["new_regime"])
	}
	if payload["old_regime"] != "bull" {
		t.Errorf("want old_regime=bull, got %v", payload["old_regime"])
	}
}

func TestRegimeDebouncer_ResetsWindowOnNewChange(t *testing.T) {
	bus := newRegimeRecordingBus()
	d := &regimeDebouncer{bus: bus}

	t0 := time.Now()
	dispatchRegimeChangeToHandler(d, domain.Regime("bull"), domain.Regime("bear"), 0.8, t0)

	d.check(t0.Add(25 * time.Second))
	dispatchRegimeChangeToHandler(d, domain.Regime("bear"), domain.Regime("sideways"), 0.6, t0.Add(25*time.Second))

	d.check(t0.Add(45 * time.Second))
	got := bus.snapshot()
	if len(got) != 0 {
		t.Fatalf("only 20s since latest change, should not emit, got %d", len(got))
	}

	d.check(t0.Add(56 * time.Second))
	got = bus.snapshot()
	if len(got) != 1 {
		t.Fatalf("31s after latest change should emit, got %d", len(got))
	}
	payload := got[0].Payload.(map[string]any)
	if payload["new_regime"] != "sideways" {
		t.Errorf("want new_regime=sideways (latest), got %v", payload["new_regime"])
	}
}

func TestRegimeDebouncer_DedupSameRegime(t *testing.T) {
	bus := newRegimeRecordingBus()
	d := &regimeDebouncer{bus: bus}

	t0 := time.Now()
	dispatchRegimeChangeToHandler(d, domain.Regime("bull"), domain.Regime("bear"), 0.8, t0)
	d.check(t0.Add(31 * time.Second))

	d.check(t0.Add(40 * time.Second))
	d.check(t0.Add(50 * time.Second))

	got := bus.snapshot()
	if len(got) != 1 {
		t.Fatalf("same regime should only emit once, got %d", len(got))
	}
}

func TestRegimeDebouncer_StartStopLifecycle(t *testing.T) {
	bus := newRegimeRecordingBus()
	d := NewRegimeDebouncer(bus)

	ctx := t.Context()
	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	t0 := time.Now()
	publishRegimeChange(bus, domain.Regime("bull"), domain.Regime("bear"), 0.8, t0)

	time.Sleep(50 * time.Millisecond)
	if err := d.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}
