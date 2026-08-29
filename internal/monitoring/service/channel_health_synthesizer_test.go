package service

import (
	"context"
	"maps"
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
	maps.Copy(cp, f.data)
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

// Regression test: first_seen_at 必須等於首次偵測時間，而非 time.Time 零值。
// Code review PR #632 issue #1 指出修補前 `firstSeen := last` 在 `seen=false`
// 時會把 `0001-01-01T00:00:00Z` 寫入 payload。
func TestChannelHealthSynthesizer_FirstSeenAtEqualsNowOnFirstEmit(t *testing.T) {
	bus := &recordingBus{}
	provider := &fakeChannelHealthProvider{}
	syn := NewChannelHealthSynthesizer(bus, provider)

	t0 := time.Now()
	provider.set(map[string]string{"twse_capital_flow": "timeout"})

	syn.(*channelHealthSynthesizer).check(t0)
	events := bus.snapshot()
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	payload, ok := events[0].Payload.(map[string]any)
	if !ok {
		t.Fatalf("payload type = %T, want map[string]any", events[0].Payload)
	}
	firstSeen, ok := payload["first_seen_at"].(time.Time)
	if !ok {
		t.Fatalf("first_seen_at type = %T, want time.Time", payload["first_seen_at"])
	}
	if firstSeen.IsZero() {
		t.Fatalf("first_seen_at is zero time on first emit; want t0 = %v", t0)
	}
	if !firstSeen.Equal(t0) {
		t.Errorf("first_seen_at = %v, want %v", firstSeen, t0)
	}
	if detectedAt, _ := payload["detected_at"].(time.Time); !detectedAt.Equal(t0) {
		t.Errorf("detected_at = %v, want %v", detectedAt, t0)
	}
}

// Regression test: 第二次觸發（超出 dedup window）時，first_seen_at 必須
// 保留首次偵測時間（不可被本次時間覆蓋），確保下游能區分「首次失敗」與「復發」。
func TestChannelHealthSynthesizer_FirstSeenAtPreservedAcrossRecurrence(t *testing.T) {
	bus := &recordingBus{}
	provider := &fakeChannelHealthProvider{}
	syn := NewChannelHealthSynthesizer(bus, provider)

	t0 := time.Now()
	t1 := t0.Add(2 * ChannelHealthDedupWindow)
	provider.set(map[string]string{"twse_capital_flow": "timeout"})

	syn.(*channelHealthSynthesizer).check(t0)
	syn.(*channelHealthSynthesizer).check(t1)

	events := bus.snapshot()
	if len(events) != 2 {
		t.Fatalf("want 2 events, got %d", len(events))
	}
	p0, _ := events[0].Payload.(map[string]any)
	p1, _ := events[1].Payload.(map[string]any)
	fs0, _ := p0["first_seen_at"].(time.Time)
	fs1, _ := p1["first_seen_at"].(time.Time)
	if !fs0.Equal(t0) {
		t.Errorf("first event first_seen_at = %v, want %v", fs0, t0)
	}
	if !fs1.Equal(t0) {
		t.Errorf("second event first_seen_at = %v, want %v (must preserve original)", fs1, t0)
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
