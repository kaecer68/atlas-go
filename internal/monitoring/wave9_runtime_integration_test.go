package monitoring

import (
	"context"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

// wave9BusRecorder captures events published by a Wave9Observability instance.
type wave9BusRecorder struct {
	mu     sync.Mutex
	events map[eventbus.EventType][]eventbus.BusEvent
}

func newWave9BusRecorder() *wave9BusRecorder {
	return &wave9BusRecorder{events: make(map[eventbus.EventType][]eventbus.BusEvent)}
}

func (r *wave9BusRecorder) Handle(_ context.Context, ev eventbus.BusEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events[ev.Type] = append(r.events[ev.Type], ev)
	return nil
}

func (r *wave9BusRecorder) waitFor(t *testing.T, evType eventbus.EventType, timeout time.Duration) eventbus.BusEvent {
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
	t.Fatalf("timeout waiting for event %s", evType)
	return eventbus.BusEvent{}
}

type wave9StaticWeightProvider struct {
	byRegime map[string]map[string]float64
}

func (p *wave9StaticWeightProvider) GetWeights(regime string) map[string]float64 {
	return p.byRegime[regime]
}

type wave9StaticChannelHealthProvider struct {
	errors map[string]string
}

func (p *wave9StaticChannelHealthProvider) ChannelErrors() map[string]string {
	return p.errors
}

type wave9StaticIngestionLagProvider struct {
	p99 float64
}

func (p *wave9StaticIngestionLagProvider) P99LatencySeconds() float64 {
	return p.p99
}

func setIngestionLagInterval(d service.IngestionLagMonitor, interval time.Duration) {
	dv := reflect.ValueOf(d).Elem()
	field := dv.FieldByName("interval")
	ptr := (*time.Duration)(unsafe.Pointer(field.UnsafeAddr()))
	*ptr = interval
}

type shortIntervalFactory struct {
	defaultDetectorFactory
}

func (shortIntervalFactory) newIngestionLagMonitor(bus eventbus.EventBus, provider service.IngestionLagProvider) service.IngestionLagMonitor {
	d := service.NewIngestionLagMonitor(bus, provider)
	setIngestionLagInterval(d, 50*time.Millisecond)
	return d
}

// requireWired checks that a private detector field on Wave9Observability is
// non-nil and has the expected concrete type name.  Concrete types live in
// package service and are unexported from monitoring, so we compare type-name
// strings rather than reflect.Type values.
func requireWired(t *testing.T, w *Wave9Observability, fieldName, wantTypeName string) {
	t.Helper()
	v := reflect.ValueOf(w).Elem().FieldByName(fieldName)
	if !v.IsValid() || v.IsNil() {
		t.Fatalf("field %s is nil or missing", fieldName)
	}
	got := v.Elem().Type().String()
	if got != wantTypeName {
		t.Fatalf("field %s want type %s, got %s", fieldName, wantTypeName, got)
	}
}

func TestWave9Integration_AllFiveDetectorsWired(t *testing.T) {
	bus := eventbus.NewChannelEventBus(256)
	defer func() { _ = bus.Close() }()

	w, err := NewWave9Observability(
		bus,
		WithWeightProvider(&wave9StaticWeightProvider{}),
		WithChannelHealthProvider(&wave9StaticChannelHealthProvider{}),
		WithIngestionLagProvider(&wave9StaticIngestionLagProvider{p99: 1.0}),
	)
	if err != nil {
		t.Fatalf("NewWave9Observability failed: %v", err)
	}

	ctx := context.Background()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = w.Stop() }()

	requireWired(t, w, "regimeDebouncer", "*service.regimeDebouncer")
	requireWired(t, w, "factorWeightRegression", "*service.factorWeightRegressionDetector")
	requireWired(t, w, "driftDetector", "*service.driftDetector")
	requireWired(t, w, "channelHealthSynthesizer", "*service.channelHealthSynthesizer")
	requireWired(t, w, "ingestionLagMonitor", "*service.ingestionLagMonitor")
}

// countWave9Goroutines returns the count of running Wave 9 detector run loops.
// FactorWeightRegressionDetector is event-driven and excluded.
func countWave9Goroutines() int {
	buf := make([]byte, 1<<16)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	stacks := string(buf)
	markers := []string{
		"(*regimeDebouncer).run",
		"(*ingestionLagMonitor).run",
		"(*channelHealthSynthesizer).run",
		"(*driftDetector).run",
	}
	n := 0
	for _, m := range markers {
		n += strings.Count(stacks, m)
	}
	return n
}

func TestWave9Integration_StartStopNoLeak(t *testing.T) {
	bus := eventbus.NewChannelEventBus(256)
	defer func() { _ = bus.Close() }()

	w, err := NewWave9Observability(
		bus,
		WithWeightProvider(&wave9StaticWeightProvider{}),
		WithChannelHealthProvider(&wave9StaticChannelHealthProvider{}),
		WithIngestionLagProvider(&wave9StaticIngestionLagProvider{p99: 1.0}),
	)
	if err != nil {
		t.Fatalf("NewWave9Observability failed: %v", err)
	}

	before := countWave9Goroutines()
	ctx := context.Background()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if err := w.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	runtime.GC()
	time.Sleep(200 * time.Millisecond)
	after := countWave9Goroutines()
	if after > before {
		t.Fatalf("Wave9 detector goroutine leak: before=%d after=%d (markers: regimeDebouncer.run / ingestionLagMonitor.run / channelHealthSynthesizer.run / driftDetector.run)", before, after)
	}
}

func TestWave9Integration_Wave9ObservabilityEndToEnd(t *testing.T) {
	bus := eventbus.NewChannelEventBus(256)
	defer func() { _ = bus.Close() }()

	rec := newWave9BusRecorder()
	for _, et := range []eventbus.EventType{
		eventbus.EventRegimeChangeConfirmed,
		eventbus.EventFactorWeightRegression,
		eventbus.EventIngestionLagSpike,
	} {
		bus.Subscribe(et, rec.Handle)
	}

	weightProvider := &wave9StaticWeightProvider{
		byRegime: map[string]map[string]float64{
			"regime_a": {"momentum": 0.3, "value": 0.4, "quality": 0.3},
			"regime_b": {"momentum": 0.6, "value": 0.2, "quality": 0.2},
		},
	}

	w, err := NewWave9Observability(
		bus,
		WithWeightProvider(weightProvider),
		WithChannelHealthProvider(&wave9StaticChannelHealthProvider{errors: map[string]string{"twse": "timeout"}}),
		WithIngestionLagProvider(&wave9StaticIngestionLagProvider{p99: 10.0}),
		withDetectorFactory(shortIntervalFactory{}),
	)
	if err != nil {
		t.Fatalf("NewWave9Observability failed: %v", err)
	}

	ctx := context.Background()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = w.Stop() }()

	// Drive factor-weight regression through two regime changes.
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
	time.Sleep(50 * time.Millisecond)

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

	rec.waitFor(t, eventbus.EventFactorWeightRegression, 2*time.Second)

	// The regime debouncer and drift detector use production tickers that are
	// too slow for CI, so this test verifies the wrapper wiring and the
	// detectors that respond immediately to input events.
	rec.waitFor(t, eventbus.EventIngestionLagSpike, 2*time.Second)
}
