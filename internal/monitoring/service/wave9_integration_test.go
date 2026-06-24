package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
)

// integrationBusRecorder captures BusEvent instances keyed by EventType.
// It is safe for concurrent use because eventbus handlers are invoked in
// per-subscriber goroutines.
type integrationBusRecorder struct {
	mu     sync.Mutex
	events map[eventbus.EventType][]eventbus.BusEvent
}

func newIntegrationBusRecorder() *integrationBusRecorder {
	return &integrationBusRecorder{events: make(map[eventbus.EventType][]eventbus.BusEvent)}
}

func (r *integrationBusRecorder) Handle(_ context.Context, ev eventbus.BusEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events[ev.Type] = append(r.events[ev.Type], ev)
	return nil
}

func (r *integrationBusRecorder) WaitForEvent(t *testing.T, evType eventbus.EventType, timeout time.Duration) eventbus.BusEvent {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		events := r.events[evType]
		r.mu.Unlock()
		if len(events) > 0 {
			return events[0]
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for event %s after %v (got events: %v)", evType, timeout, r.snapshot())
	return eventbus.BusEvent{}
}

func (r *integrationBusRecorder) snapshot() map[eventbus.EventType]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[eventbus.EventType]int, len(r.events))
	for k, v := range r.events {
		out[k] = len(v)
	}
	return out
}

// Mock providers used to drive the detectors without external infrastructure.

type staticWeightProvider struct {
	byRegime map[string]map[string]float64
}

func (p *staticWeightProvider) GetWeights(regime string) map[string]float64 {
	return p.byRegime[regime]
}

type staticChannelHealthProvider struct {
	errors map[string]string
}

func (p *staticChannelHealthProvider) ChannelErrors() map[string]string {
	return p.errors
}

type staticIngestionLagProvider struct {
	p99 float64
}

func (p *staticIngestionLagProvider) P99LatencySeconds() float64 {
	return p.p99
}

// waitForHandlers yields briefly so that the real ChannelEventBus dispatcher
// and per-handler goroutines can process published events.  It avoids the
// long real-time waits that the production tickers would impose.
func waitForHandlers() {
	time.Sleep(100 * time.Millisecond)
}

func TestWave9Integration_RegimeDebouncerFlow(t *testing.T) {
	bus := eventbus.NewChannelEventBus(256)
	defer func() { _ = bus.Close() }()

	rec := newIntegrationBusRecorder()
	bus.Subscribe(eventbus.EventRegimeChangeConfirmed, rec.Handle)

	d := NewRegimeDebouncer(bus)
	ctx := context.Background()
	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = d.Stop() }()

	bus.Publish(eventbus.BusEvent{
		Type:      eventbus.EventRegimeChange,
		Timestamp: time.Now(),
		Payload: eventbus.RegimeEventPayload{
			OldRegime:    domain.Regime("bear"),
			NewRegime:    domain.Regime("bull"),
			Confidence:   0.95,
			DeterminedBy: "integration-test",
		},
		SchemaVersion: 1,
	})

	waitForHandlers()

	// Bypass the 30s production debounce window by evaluating stability at a
	// future instant.  The test is in package service, so the private check
	// method is accessible.
	rd := d.(*regimeDebouncer)
	rd.check(time.Now().Add(60 * time.Second))

	ev := rec.WaitForEvent(t, eventbus.EventRegimeChangeConfirmed, 2*time.Second)
	if ev.Type != eventbus.EventRegimeChangeConfirmed {
		t.Fatalf("want EventRegimeChangeConfirmed, got %s", ev.Type)
	}
	payload, ok := ev.Payload.(map[string]any)
	if !ok {
		t.Fatalf("want map[string]any payload, got %T", ev.Payload)
	}
	if payload["new_regime"] != "bull" {
		t.Errorf("want new_regime=bull, got %v", payload["new_regime"])
	}
	if payload["old_regime"] != "bear" {
		t.Errorf("want old_regime=bear, got %v", payload["old_regime"])
	}
}

func TestWave9Integration_FactorWeightRegressionFlow(t *testing.T) {
	bus := eventbus.NewChannelEventBus(256)
	defer func() { _ = bus.Close() }()

	rec := newIntegrationBusRecorder()
	bus.Subscribe(eventbus.EventFactorWeightRegression, rec.Handle)

	provider := &staticWeightProvider{
		byRegime: map[string]map[string]float64{
			"bear": {"momentum": 0.3, "value": 0.4, "quality": 0.3},
			"bull": {"momentum": 0.6, "value": 0.2, "quality": 0.2},
		},
	}

	d := NewFactorWeightRegressionDetector(bus, provider)
	ctx := context.Background()
	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = d.Stop() }()

	bus.Publish(eventbus.BusEvent{
		Type:      eventbus.EventRegimeChange,
		Timestamp: time.Now(),
		Payload: eventbus.RegimeEventPayload{
			OldRegime:    domain.Regime("init"),
			NewRegime:    domain.Regime("bear"),
			Confidence:   0.9,
			DeterminedBy: "integration-test",
		},
		SchemaVersion: 1,
	})
	waitForHandlers()

	bus.Publish(eventbus.BusEvent{
		Type:      eventbus.EventRegimeChange,
		Timestamp: time.Now(),
		Payload: eventbus.RegimeEventPayload{
			OldRegime:    domain.Regime("bear"),
			NewRegime:    domain.Regime("bull"),
			Confidence:   0.9,
			DeterminedBy: "integration-test",
		},
		SchemaVersion: 1,
	})

	ev := rec.WaitForEvent(t, eventbus.EventFactorWeightRegression, 2*time.Second)
	if ev.Type != eventbus.EventFactorWeightRegression {
		t.Fatalf("want EventFactorWeightRegression, got %s", ev.Type)
	}
	payload, ok := ev.Payload.(map[string]any)
	if !ok {
		t.Fatalf("want map[string]any payload, got %T", ev.Payload)
	}
	if payload["regime"] != "bull" {
		t.Errorf("want regime=bull, got %v", payload["regime"])
	}
	score, ok := payload["regression_score"].(float64)
	if !ok || score < FactorWeightRegressionThreshold {
		t.Errorf("want regression_score >= %f, got %v", FactorWeightRegressionThreshold, payload["regression_score"])
	}
}

func TestWave9Integration_DriftDetectorFlow(t *testing.T) {
	bus := eventbus.NewChannelEventBus(256)
	defer func() { _ = bus.Close() }()

	rec := newIntegrationBusRecorder()
	bus.Subscribe(eventbus.EventDriftDetected, rec.Handle)

	d := NewDriftDetector(bus)
	ctx := context.Background()
	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = d.Stop() }()

	bus.Publish(eventbus.BusEvent{
		Type:      eventbus.EventPositionUpdate,
		Timestamp: time.Now(),
		Payload: eventbus.PositionEventPayload{
			Symbol:     "2330",
			ChangeType: "added",
			Position: domain.Position{
				Symbol:      "2330",
				Quantity:    1000,
				MarketValue: 700_000,
			},
		},
		SchemaVersion: 1,
	})
	for _, sym := range []string{"2454", "2317", "2881"} {
		bus.Publish(eventbus.BusEvent{
			Type:      eventbus.EventPositionUpdate,
			Timestamp: time.Now(),
			Payload: eventbus.PositionEventPayload{
				Symbol:     sym,
				ChangeType: "added",
				Position: domain.Position{
					Symbol:      sym,
					Quantity:    1000,
					MarketValue: 100_000,
				},
			},
			SchemaVersion: 1,
		})
	}
	waitForHandlers()

	// The first check establishes a baseline; the second detects the
	// concentration drift without waiting for the production 5m ticker.
	dd := d.(*driftDetector)
	dd.checkPeriod(time.Now())
	dd.checkPeriod(time.Now().Add(time.Minute))

	ev := rec.WaitForEvent(t, eventbus.EventDriftDetected, 2*time.Second)
	if ev.Type != eventbus.EventDriftDetected {
		t.Fatalf("want EventDriftDetected, got %s", ev.Type)
	}
	payload, ok := ev.Payload.(map[string]any)
	if !ok {
		t.Fatalf("want map[string]any payload, got %T", ev.Payload)
	}
	if payload["max_symbol"] != "2330" {
		t.Errorf("want max_symbol=2330, got %v", payload["max_symbol"])
	}
	maxConcentration, ok := payload["max_concentration"].(float64)
	if !ok || maxConcentration <= DriftMaxConcentrationThreshold {
		t.Errorf("want max_concentration > %f, got %v", DriftMaxConcentrationThreshold, payload["max_concentration"])
	}
}

func TestWave9Integration_ChannelHealthSynthesizerFlow(t *testing.T) {
	bus := eventbus.NewChannelEventBus(256)
	defer func() { _ = bus.Close() }()

	rec := newIntegrationBusRecorder()
	bus.Subscribe(eventbus.EventChannelIndividualHealth, rec.Handle)

	provider := &staticChannelHealthProvider{
		errors: map[string]string{"twse": "timeout", "finmind": "rate_limited"},
	}

	d := NewChannelHealthSynthesizer(bus, provider)
	ctx := context.Background()
	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = d.Stop() }()

	ch := d.(*channelHealthSynthesizer)
	ch.check(time.Now())

	ev := rec.WaitForEvent(t, eventbus.EventChannelIndividualHealth, 2*time.Second)
	if ev.Type != eventbus.EventChannelIndividualHealth {
		t.Fatalf("want EventChannelIndividualHealth, got %s", ev.Type)
	}
	payload, ok := ev.Payload.(map[string]any)
	if !ok {
		t.Fatalf("want map[string]any payload, got %T", ev.Payload)
	}
	if payload["channel_id"] != "finmind" && payload["channel_id"] != "twse" {
		t.Errorf("unexpected channel_id %v", payload["channel_id"])
	}
}

func TestWave9Integration_IngestionLagMonitorFlow(t *testing.T) {
	bus := eventbus.NewChannelEventBus(256)
	defer func() { _ = bus.Close() }()

	rec := newIntegrationBusRecorder()
	bus.Subscribe(eventbus.EventIngestionLagSpike, rec.Handle)

	provider := &staticIngestionLagProvider{p99: 10.0}
	d := NewIngestionLagMonitor(bus, provider)
	ctx := context.Background()
	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = d.Stop() }()

	m := d.(*ingestionLagMonitor)
	m.check(time.Now())

	ev := rec.WaitForEvent(t, eventbus.EventIngestionLagSpike, 2*time.Second)
	if ev.Type != eventbus.EventIngestionLagSpike {
		t.Fatalf("want EventIngestionLagSpike, got %s", ev.Type)
	}
	payload, ok := ev.Payload.(map[string]any)
	if !ok {
		t.Fatalf("want map[string]any payload, got %T", ev.Payload)
	}
	p99, ok := payload["p99_latency_seconds"].(float64)
	if !ok || p99 < IngestionLagP99Threshold.Seconds() {
		t.Errorf("want p99_latency_seconds >= %f, got %v", IngestionLagP99Threshold.Seconds(), payload["p99_latency_seconds"])
	}
}

func TestWave9Integration_EndToEndEventFlow(t *testing.T) {
	bus := eventbus.NewChannelEventBus(256)
	defer func() { _ = bus.Close() }()

	rec := newIntegrationBusRecorder()
	for _, et := range []eventbus.EventType{
		eventbus.EventRegimeChangeConfirmed,
		eventbus.EventFactorWeightRegression,
		eventbus.EventDriftDetected,
		eventbus.EventIngestionLagSpike,
	} {
		bus.Subscribe(et, rec.Handle)
	}

	weightProvider := &staticWeightProvider{
		byRegime: map[string]map[string]float64{
			"regime_a": {"momentum": 0.3, "value": 0.4, "quality": 0.3},
			"regime_b": {"momentum": 0.6, "value": 0.2, "quality": 0.2},
		},
	}
	channelHealthProvider := &staticChannelHealthProvider{errors: map[string]string{"twse": "timeout"}}
	ingestionLagProvider := &staticIngestionLagProvider{p99: 10.0}

	rd := NewRegimeDebouncer(bus)
	fw := NewFactorWeightRegressionDetector(bus, weightProvider)
	dd := NewDriftDetector(bus)
	ch := NewChannelHealthSynthesizer(bus, channelHealthProvider)
	il := NewIngestionLagMonitor(bus, ingestionLagProvider)

	ctx := context.Background()
	for _, d := range []interface {
		Start(context.Context) error
		Stop() error
	}{rd, fw, dd, ch, il} {
		if err := d.Start(ctx); err != nil {
			t.Fatalf("Start failed: %v", err)
		}
		defer func(d interface{ Stop() error }) { _ = d.Stop() }(d)
	}

	// Factor-weight regression: two regime changes.
	bus.Publish(eventbus.BusEvent{
		Type:      eventbus.EventRegimeChange,
		Timestamp: time.Now(),
		Payload: eventbus.RegimeEventPayload{
			OldRegime:    domain.Regime("init"),
			NewRegime:    domain.Regime("regime_a"),
			Confidence:   0.9,
			DeterminedBy: "integration-test",
		},
		SchemaVersion: 1,
	})
	waitForHandlers()

	bus.Publish(eventbus.BusEvent{
		Type:      eventbus.EventRegimeChange,
		Timestamp: time.Now(),
		Payload: eventbus.RegimeEventPayload{
			OldRegime:    domain.Regime("regime_a"),
			NewRegime:    domain.Regime("regime_b"),
			Confidence:   0.9,
			DeterminedBy: "integration-test",
		},
		SchemaVersion: 1,
	})
	waitForHandlers()
	rec.WaitForEvent(t, eventbus.EventFactorWeightRegression, 2*time.Second)

	// Regime confirmation via private check.
	rd.(*regimeDebouncer).check(time.Now().Add(60 * time.Second))
	rec.WaitForEvent(t, eventbus.EventRegimeChangeConfirmed, 2*time.Second)

	// Drift detection via private checkPeriod.
	for _, sym := range []string{"2330", "2454", "2317", "2881"} {
		mv := 100_000.0
		if sym == "2330" {
			mv = 700_000
		}
		bus.Publish(eventbus.BusEvent{
			Type:      eventbus.EventPositionUpdate,
			Timestamp: time.Now(),
			Payload: eventbus.PositionEventPayload{
				Symbol:     sym,
				ChangeType: "added",
				Position:   domain.Position{Symbol: sym, Quantity: 1000, MarketValue: mv},
			},
			SchemaVersion: 1,
		})
	}
	waitForHandlers()
	dd.(*driftDetector).checkPeriod(time.Now())
	dd.(*driftDetector).checkPeriod(time.Now().Add(time.Minute))
	rec.WaitForEvent(t, eventbus.EventDriftDetected, 2*time.Second)

	// Ingestion-lag spike via private check.
	il.(*ingestionLagMonitor).check(time.Now())
	rec.WaitForEvent(t, eventbus.EventIngestionLagSpike, 2*time.Second)
}
