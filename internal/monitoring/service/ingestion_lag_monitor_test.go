package service

import (
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/eventbus"
)

type lagRecordingBus struct {
	mu     sync.Mutex
	events []eventbus.BusEvent
}

func (b *lagRecordingBus) Publish(ev eventbus.BusEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, ev)
}

func (b *lagRecordingBus) Subscribe(eventbus.EventType, eventbus.EventHandler) eventbus.Subscription {
	return eventbus.Subscription{Cancel: func() {}}
}

func (b *lagRecordingBus) SubscribeAll(eventbus.EventHandler) eventbus.Subscription {
	return eventbus.Subscription{Cancel: func() {}}
}
func (b *lagRecordingBus) Close() error { return nil }
func (b *lagRecordingBus) snapshot() []eventbus.BusEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]eventbus.BusEvent, len(b.events))
	copy(out, b.events)
	return out
}

type stubLagProvider struct {
	mu  sync.Mutex
	p99 float64
}

func (s *stubLagProvider) P99LatencySeconds() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.p99
}

func TestIngestionLagMonitor_NoEmitBelowThreshold(t *testing.T) {
	bus := &lagRecordingBus{}
	provider := &stubLagProvider{p99: 2.0}
	m := &ingestionLagMonitor{bus: bus, provider: provider}

	m.check(time.Now())
	if got := bus.snapshot(); len(got) != 0 {
		t.Fatalf("2s latency (below 5s threshold) should not emit, got %d", len(got))
	}
}

func TestIngestionLagMonitor_EmitsAboveThreshold(t *testing.T) {
	bus := &lagRecordingBus{}
	provider := &stubLagProvider{p99: 7.5}
	m := &ingestionLagMonitor{bus: bus, provider: provider}

	m.check(time.Now())
	got := bus.snapshot()
	if len(got) != 1 {
		t.Fatalf("7.5s latency should emit, got %d events", len(got))
	}
	if got[0].Type != eventbus.EventIngestionLagSpike {
		t.Errorf("unexpected event type %s", got[0].Type)
	}
	if got[0].Severity != "warning" {
		t.Errorf("want severity warning, got %s", got[0].Severity)
	}
	payload := got[0].Payload.(map[string]any)
	if v := payload["p99_latency_seconds"].(float64); v != 7.5 {
		t.Errorf("want p99=7.5, got %f", v)
	}
}

func TestIngestionLagMonitor_DedupWithinWindow(t *testing.T) {
	bus := &lagRecordingBus{}
	provider := &stubLagProvider{p99: 8.0}
	m := &ingestionLagMonitor{bus: bus, provider: provider}

	t0 := time.Now()
	m.check(t0)
	m.check(t0.Add(10 * time.Second))
	m.check(t0.Add(30 * time.Second))
	m.check(t0.Add(50 * time.Second))

	got := bus.snapshot()
	if len(got) != 1 {
		t.Fatalf("within 60s dedup window, should emit once, got %d", len(got))
	}
}

func TestIngestionLagMonitor_EmitsAgainAfterDedupWindow(t *testing.T) {
	bus := &lagRecordingBus{}
	provider := &stubLagProvider{p99: 8.0}
	m := &ingestionLagMonitor{bus: bus, provider: provider}

	t0 := time.Now()
	m.check(t0)
	m.check(t0.Add(61 * time.Second))

	got := bus.snapshot()
	if len(got) != 2 {
		t.Fatalf("after dedup window expires, should emit again, got %d", len(got))
	}
}

func TestIngestionLagMonitor_NilProviderNoEmit(t *testing.T) {
	bus := &lagRecordingBus{}
	m := &ingestionLagMonitor{bus: bus, provider: nil}
	m.check(time.Now())
	if got := bus.snapshot(); len(got) != 0 {
		t.Fatalf("nil provider should not emit, got %d", len(got))
	}
}

func TestIngestionLagMonitor_StartStopLifecycle(t *testing.T) {
	bus := &lagRecordingBus{}
	provider := &stubLagProvider{p99: 8.0}
	m := NewIngestionLagMonitor(bus, provider)
	m.(*ingestionLagMonitor).interval = 10 * time.Millisecond

	ctx := t.Context()
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	if err := m.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if got := bus.snapshot(); len(got) == 0 {
		t.Fatal("want at least one event from background poll, got 0")
	}
}
