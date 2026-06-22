package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/eventbus"
)

type fakeChannelHealthProvider struct {
	mu   sync.Mutex
	data map[string]string
}

func (f *fakeChannelHealthProvider) ChannelErrors() map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.data) == 0 {
		return nil
	}
	cp := make(map[string]string, len(f.data))
	for k, v := range f.data {
		cp[k] = v
	}
	return cp
}

func (f *fakeChannelHealthProvider) set(data map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data = data
}

type recordingBus struct {
	mu     sync.Mutex
	events []eventbus.BusEvent
	closed bool
}

func (b *recordingBus) Publish(ev eventbus.BusEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, ev)
}

func (b *recordingBus) Subscribe(eventbus.EventType, eventbus.EventHandler) eventbus.Subscription {
	return eventbus.Subscription{}
}

func (b *recordingBus) SubscribeAll(eventbus.EventHandler) eventbus.Subscription {
	return eventbus.Subscription{}
}

func (b *recordingBus) Close() error { b.closed = true; return nil }

func (b *recordingBus) snapshot() []eventbus.BusEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]eventbus.BusEvent, len(b.events))
	copy(out, b.events)
	return out
}

func TestChannelHealthSynthesizer_EmitsEventPerChannel(t *testing.T) {
	bus := &recordingBus{}
	provider := &fakeChannelHealthProvider{}
	syn := NewChannelHealthSynthesizer(bus, provider)

	now := time.Now()
	provider.set(map[string]string{
		"twse_capital_flow": "timeout after 5s",
		"sox_index":         "HTTP 503",
	})

	syn.(*channelHealthSynthesizer).check(now)

	events := bus.snapshot()
	if len(events) != 2 {
		t.Fatalf("want 2 events, got %d", len(events))
	}
	for _, ev := range events {
		if ev.Type != eventbus.EventChannelIndividualHealth {
			t.Errorf("unexpected event type %s", ev.Type)
		}
		if ev.Severity != "info" {
			t.Errorf("want severity info, got %s", ev.Severity)
		}
		if ev.SchemaVersion != 1 {
			t.Errorf("want schema version 1, got %d", ev.SchemaVersion)
		}
	}
}

func TestChannelHealthSynthesizer_DedupWithinWindow(t *testing.T) {
	bus := &recordingBus{}
	provider := &fakeChannelHealthProvider{}
	syn := NewChannelHealthSynthesizer(bus, provider)

	now := time.Now()
	provider.set(map[string]string{"twse_capital_flow": "timeout"})

	syn.(*channelHealthSynthesizer).check(now)
	syn.(*channelHealthSynthesizer).check(now.Add(2 * time.Second))
	syn.(*channelHealthSynthesizer).check(now.Add(4 * time.Second))

	events := bus.snapshot()
	if len(events) != 1 {
		t.Fatalf("want 1 event after dedup, got %d", len(events))
	}
}

func TestChannelHealthSynthesizer_EmitsAgainAfterDedupWindow(t *testing.T) {
	bus := &recordingBus{}
	provider := &fakeChannelHealthProvider{}
	syn := NewChannelHealthSynthesizer(bus, provider)

	t0 := time.Now()
	provider.set(map[string]string{"twse_capital_flow": "timeout"})

	syn.(*channelHealthSynthesizer).check(t0)
	syn.(*channelHealthSynthesizer).check(t0.Add(6 * time.Second))

	events := bus.snapshot()
	if len(events) != 2 {
		t.Fatalf("want 2 events after dedup window expires, got %d", len(events))
	}
}

func TestChannelHealthSynthesizer_NilProviderNoEmit(t *testing.T) {
	bus := &recordingBus{}
	syn := &channelHealthSynthesizer{
		bus:      bus,
		provider: nil,
		lastSeen: make(map[string]time.Time),
	}
	syn.check(time.Now())
	if got := bus.snapshot(); len(got) != 0 {
		t.Fatalf("want 0 events, got %d", len(got))
	}
}

func TestChannelHealthSynthesizer_StartStopLifecycle(t *testing.T) {
	bus := &recordingBus{}
	provider := &fakeChannelHealthProvider{data: map[string]string{"x": "err"}}
	syn := NewChannelHealthSynthesizer(bus, provider)
	syn.(*channelHealthSynthesizer).interval = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := syn.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	if err := syn.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if got := bus.snapshot(); len(got) == 0 {
		t.Fatal("want at least one event from background poll, got 0")
	}
}
