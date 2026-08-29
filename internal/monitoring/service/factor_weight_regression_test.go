package service

import (
	"context"
	"maps"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
)

type stubWeightProvider struct {
	mu    sync.Mutex
	store map[string]map[string]float64
}

func (s *stubWeightProvider) GetWeights(regime string) map[string]float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	src, ok := s.store[regime]
	if !ok {
		return nil
	}
	cp := make(map[string]float64, len(src))
	maps.Copy(cp, src)
	return cp
}

func (s *stubWeightProvider) set(regime string, weights map[string]float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store == nil {
		s.store = make(map[string]map[string]float64)
	}
	s.store[regime] = weights
}

type factorRecordingBus struct {
	mu     sync.Mutex
	events []eventbus.BusEvent
}

func (b *factorRecordingBus) Publish(ev eventbus.BusEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, ev)
}

func (b *factorRecordingBus) Subscribe(eventbus.EventType, eventbus.EventHandler) eventbus.Subscription {
	return eventbus.Subscription{Cancel: func() {}}
}

func (b *factorRecordingBus) SubscribeAll(eventbus.EventHandler) eventbus.Subscription {
	return eventbus.Subscription{Cancel: func() {}}
}
func (b *factorRecordingBus) Close() error { return nil }
func (b *factorRecordingBus) snapshot() []eventbus.BusEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]eventbus.BusEvent, len(b.events))
	copy(out, b.events)
	return out
}

func dispatchRegime(d *factorWeightRegressionDetector, newR domain.Regime, ts time.Time) {
	_ = d.onRegimeChange(context.Background(), eventbus.BusEvent{
		Type:      eventbus.EventRegimeChange,
		Timestamp: ts,
		Payload: eventbus.RegimeEventPayload{
			OldRegime:    domain.Regime("previous"),
			NewRegime:    newR,
			Confidence:   0.7,
			DeterminedBy: "test",
		},
	})
}

func TestFactorWeightRegression_NoPriorWeightsNoEmit(t *testing.T) {
	bus := &factorRecordingBus{}
	provider := &stubWeightProvider{}
	provider.set("bull", map[string]float64{"momentum": 1.0, "value": 0.5})

	d := &factorWeightRegressionDetector{bus: bus, provider: provider}
	dispatchRegime(d, domain.Regime("bull"), time.Now())

	if got := bus.snapshot(); len(got) != 0 {
		t.Fatalf("first regime change should not emit (no prior), got %d", len(got))
	}
}

func TestFactorWeightRegression_SmallChangeNoEmit(t *testing.T) {
	bus := &factorRecordingBus{}
	provider := &stubWeightProvider{}
	provider.set("bull", map[string]float64{"momentum": 1.0, "value": 0.5})

	d := &factorWeightRegressionDetector{bus: bus, provider: provider}
	dispatchRegime(d, domain.Regime("bull"), time.Now())

	provider.set("bear", map[string]float64{"momentum": 1.1, "value": 0.55})
	dispatchRegime(d, domain.Regime("bear"), time.Now())

	if got := bus.snapshot(); len(got) != 0 {
		t.Fatalf("small weight change (score < 0.5) should not emit, got %d events", len(got))
	}
}

func TestFactorWeightRegression_LargeChangeEmit(t *testing.T) {
	bus := &factorRecordingBus{}
	provider := &stubWeightProvider{}
	provider.set("bull", map[string]float64{"momentum": 1.0, "value": 0.5})

	d := &factorWeightRegressionDetector{bus: bus, provider: provider}
	dispatchRegime(d, domain.Regime("bull"), time.Now())

	provider.set("bear", map[string]float64{"momentum": 2.5, "value": 0.0})
	dispatchRegime(d, domain.Regime("bear"), time.Now())

	got := bus.snapshot()
	if len(got) != 1 {
		t.Fatalf("large weight change (score >= 0.5) should emit, got %d events", len(got))
	}
	if got[0].Type != eventbus.EventFactorWeightRegression {
		t.Errorf("unexpected event type %s", got[0].Type)
	}
	payload := got[0].Payload.(map[string]any)
	if payload["regime"] != "bear" {
		t.Errorf("want regime=bear, got %v", payload["regime"])
	}
	score := payload["regression_score"].(float64)
	if score < 0.5 {
		t.Errorf("expected score >= 0.5, got %f", score)
	}
}

func TestFactorWeightRegression_NilProviderNoEmitNoPanic(t *testing.T) {
	bus := &factorRecordingBus{}
	d := &factorWeightRegressionDetector{bus: bus, provider: nil}
	dispatchRegime(d, domain.Regime("bull"), time.Now())
	if got := bus.snapshot(); len(got) != 0 {
		t.Fatalf("nil provider should not emit, got %d", len(got))
	}
}

func TestRegressionScore(t *testing.T) {
	if got := regressionScore(map[string]float64{"a": 1.0}, map[string]float64{"a": 1.0}); got != 0 {
		t.Errorf("identical should give 0, got %f", got)
	}
	if got := regressionScore(map[string]float64{"a": 1.0}, map[string]float64{"a": 1.5}); got != 0.5 {
		t.Errorf("diff 0.5 should give 0.5, got %f", got)
	}
	if got := regressionScore(map[string]float64{"a": 1.0}, map[string]float64{"b": 1.0}); got != 2.0 {
		t.Errorf("complete replacement a->b should give 2.0, got %f", got)
	}
}
