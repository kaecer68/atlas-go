package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/eventbus"
)

// stubTargetProvider is a minimal TargetWeightsProvider for tests.
type stubTargetProvider struct {
	weights map[string]float64
}

func (s *stubTargetProvider) GetTargetWeights(regime string) map[string]float64 {
	return s.weights
}

// TestDriftDetector_V2TargetDriftEmitted verifies that when actual portfolio
// weights deviate from target weights beyond 10%, the detector emits
// EventDriftDetected with target_drift reason and v2 payload fields.
func TestDriftDetector_V2TargetDriftEmitted(t *testing.T) {
	bus := &driftRecordingBus{}
	provider := &stubTargetProvider{
		weights: map[string]float64{
			"2330": 0.30,
			"2454": 0.25,
			"2317": 0.25,
			"2881": 0.20,
		},
	}
	d := NewDriftDetectorWithTargets(bus, provider).(*driftDetector)

	dispatchPosition(d, "2330", 700_000, "added")
	dispatchPosition(d, "2454", 100_000, "added")
	dispatchPosition(d, "2317", 100_000, "added")
	dispatchPosition(d, "2881", 100_000, "added")

	d.checkPeriod(time.Now())                      // establish baseline
	d.checkPeriod(time.Now().Add(5 * time.Minute)) // trigger check

	got := bus.snapshot()
	if len(got) != 1 {
		t.Fatalf("expected 1 drift event, got %d", len(got))
	}
	payload := got[0].Payload.(map[string]any)

	if got[0].SchemaVersion != 2 {
		t.Errorf("expected SchemaVersion=2, got %d", got[0].SchemaVersion)
	}

	reasons := payload["reasons"].([]string)
	hasTargetDrift := false
	for _, r := range reasons {
		if r == ReasonTargetDrift {
			hasTargetDrift = true
		}
	}
	if !hasTargetDrift {
		t.Errorf("expected reasons to include target_drift, got %v", reasons)
	}

	if got[0].Type != eventbus.EventDriftDetected {
		t.Errorf("expected event type EventDriftDetected, got %s", got[0].Type)
	}

	maxDrift, ok := payload["max_drift"].(float64)
	if !ok {
		t.Fatal("payload missing max_drift")
	}
	if maxDrift <= DriftTargetWeightThreshold {
		t.Errorf("expected max_drift > %f, got %f", DriftTargetWeightThreshold, maxDrift)
	}

	if payload["max_drift_symbol"] != "2330" {
		t.Errorf("expected max_drift_symbol=2330, got %v", payload["max_drift_symbol"])
	}

	if _, ok := payload["target_weights"].(map[string]float64); !ok {
		t.Error("payload missing target_weights as map[string]float64")
	}
	if _, ok := payload["actual_weights"].(map[string]float64); !ok {
		t.Error("payload missing actual_weights as map[string]float64")
	}

	thresholds := payload["thresholds"].(map[string]float64)
	if _, ok := thresholds["target_drift"]; !ok {
		t.Errorf("thresholds map missing target_drift key, got %v", thresholds)
	}
}

// TestDriftDetector_V2TargetDriftNoEmit verifies that when drift is below
// threshold and concentration/turnover are also below, NO event is emitted.
func TestDriftDetector_V2TargetDriftNoEmit(t *testing.T) {
	bus := &driftRecordingBus{}
	provider := &stubTargetProvider{
		weights: map[string]float64{
			"2330": 0.25, "2454": 0.25, "2317": 0.25, "2881": 0.25,
		},
	}
	d := NewDriftDetectorWithTargets(bus, provider).(*driftDetector)

	dispatchPosition(d, "2330", 200_000, "added")
	dispatchPosition(d, "2454", 200_000, "added")
	dispatchPosition(d, "2317", 200_000, "added")
	dispatchPosition(d, "2881", 200_000, "added")

	d.checkPeriod(time.Now())
	d.checkPeriod(time.Now().Add(5 * time.Minute))

	if got := bus.snapshot(); len(got) != 0 {
		t.Fatalf("balanced portfolio with aligned targets should not emit, got %d events", len(got))
	}
}

// TestDriftDetector_V2NilProviderGraceful verifies that nil provider
// preserves v1 behavior exactly: concentration still triggers, but
// no target_drift reason and no v2 payload fields.
func TestDriftDetector_V2NilProviderGraceful(t *testing.T) {
	bus := &driftRecordingBus{}
	d := NewDriftDetectorWithTargets(bus, nil).(*driftDetector)

	dispatchPosition(d, "2330", 700_000, "added")
	dispatchPosition(d, "2454", 100_000, "added")
	dispatchPosition(d, "2317", 100_000, "added")
	dispatchPosition(d, "2881", 100_000, "added")

	d.checkPeriod(time.Now())
	d.checkPeriod(time.Now().Add(5 * time.Minute))

	got := bus.snapshot()
	if len(got) != 1 {
		t.Fatalf("concentration should trigger even with nil provider, got %d events", len(got))
	}
	payload := got[0].Payload.(map[string]any)
	reasons := payload["reasons"].([]string)
	for _, r := range reasons {
		if r == ReasonTargetDrift {
			t.Errorf("nil provider should NOT add target_drift reason, got %v", reasons)
		}
	}
	if _, hasMaxDrift := payload["max_drift"]; hasMaxDrift {
		t.Errorf("nil provider should NOT include max_drift in payload")
	}
	if _, hasTargetWeights := payload["target_weights"]; hasTargetWeights {
		t.Errorf("nil provider should NOT include target_weights in payload")
	}
	// thresholds map should still include target_drift constant (it's metadata, not provider-dependent)
	thresholds := payload["thresholds"].(map[string]float64)
	if _, ok := thresholds["target_drift"]; !ok {
		t.Errorf("thresholds map should always include target_drift constant, got %v", thresholds)
	}
}

// TestDriftDetector_V2EmptyTargetWeights verifies that empty target weights map
// behaves like nil provider: v1 behavior preserved, no target_drift reason.
func TestDriftDetector_V2EmptyTargetWeights(t *testing.T) {
	bus := &driftRecordingBus{}
	provider := &stubTargetProvider{weights: map[string]float64{}}
	d := NewDriftDetectorWithTargets(bus, provider).(*driftDetector)

	dispatchPosition(d, "2330", 700_000, "added")
	dispatchPosition(d, "2454", 100_000, "added")
	dispatchPosition(d, "2317", 100_000, "added")
	dispatchPosition(d, "2881", 100_000, "added")

	d.checkPeriod(time.Now())
	d.checkPeriod(time.Now().Add(5 * time.Minute))

	got := bus.snapshot()
	if len(got) != 1 {
		t.Fatalf("concentration should trigger even with empty target map, got %d", len(got))
	}
	payload := got[0].Payload.(map[string]any)
	reasons := payload["reasons"].([]string)
	for _, r := range reasons {
		if r == ReasonTargetDrift {
			t.Errorf("empty target map should NOT add target_drift reason, got %v", reasons)
		}
	}
}

// TestDriftDetector_V2RegimeChangeUpdatesCurrentRegime verifies that the
// onRegimeChangeConfirmed handler updates currentRegime.
func TestDriftDetector_V2RegimeChangeUpdatesCurrentRegime(t *testing.T) {
	bus := &driftRecordingBus{}
	d := NewDriftDetectorWithTargets(bus, nil).(*driftDetector)

	_ = d.onRegimeChangeConfirmed(context.Background(), eventbus.BusEvent{
		Type: eventbus.EventRegimeChangeConfirmed,
		Payload: map[string]any{
			"new_regime": "RISK_OFF",
			"old_regime": "RISK_ON",
		},
	})

	d.mu.Lock()
	got := d.currentRegime
	d.mu.Unlock()
	if got != "RISK_OFF" {
		t.Errorf("expected currentRegime=RISK_OFF, got %q", got)
	}
}

// TestDriftDetector_V2RegimeChangeRebaselinesPrevTotal verifies that
// regime change resets prevTotal to 0 (forces re-baseline on next checkPeriod).
func TestDriftDetector_V2RegimeChangeRebaselinesPrevTotal(t *testing.T) {
	bus := &driftRecordingBus{}
	d := NewDriftDetectorWithTargets(bus, nil).(*driftDetector)

	dispatchPosition(d, "2330", 200_000, "added")
	d.checkPeriod(time.Now()) // establishes prevTotal=200000

	d.mu.Lock()
	if d.prevTotal != 200_000 {
		d.mu.Unlock()
		t.Fatalf("expected prevTotal=200000 after first checkPeriod, got %f", d.prevTotal)
	}
	d.mu.Unlock()

	_ = d.onRegimeChangeConfirmed(context.Background(), eventbus.BusEvent{
		Type: eventbus.EventRegimeChangeConfirmed,
		Payload: map[string]any{
			"new_regime": "BEAR",
			"old_regime": "BULL",
		},
	})

	d.mu.Lock()
	prevAfter := d.prevTotal
	d.mu.Unlock()
	if prevAfter != 0 {
		t.Errorf("expected prevTotal=0 after regime change, got %f", prevAfter)
	}
}

// TestDriftDetector_V2SymbolNotInTargetMap verifies that symbols not present
// in the target map are treated as target=0, drift=actual_weight.
func TestDriftDetector_V2SymbolNotInTargetMap(t *testing.T) {
	bus := &driftRecordingBus{}
	provider := &stubTargetProvider{
		weights: map[string]float64{
			"2330": 0.50, "2454": 0.50, // only 2 symbols in target
		},
	}
	d := NewDriftDetectorWithTargets(bus, provider).(*driftDetector)

	dispatchPosition(d, "2330", 250_000, "added")
	dispatchPosition(d, "2454", 250_000, "added")
	dispatchPosition(d, "2317", 250_000, "added") // not in target → target=0
	dispatchPosition(d, "2881", 250_000, "added") // not in target → target=0

	d.checkPeriod(time.Now())
	d.checkPeriod(time.Now().Add(5 * time.Minute))

	got := bus.snapshot()
	if len(got) != 1 {
		t.Fatalf("expected 1 drift event, got %d", len(got))
	}
	payload := got[0].Payload.(map[string]any)
	reasons := payload["reasons"].([]string)
	hasTargetDrift := false
	for _, r := range reasons {
		if r == ReasonTargetDrift {
			hasTargetDrift = true
		}
	}
	if !hasTargetDrift {
		t.Errorf("expected target_drift reason when symbols not in target map, got %v", reasons)
	}

	maxDrift := payload["max_drift"].(float64)
	if maxDrift < 0.25 {
		t.Errorf("expected max_drift >= 0.25 (2317 actual=0.25, target=0), got %f", maxDrift)
	}
	maxSym := payload["max_drift_symbol"].(string)
	if maxSym != "2317" && maxSym != "2881" {
		t.Errorf("expected max_drift_symbol to be 2317 or 2881, got %s", maxSym)
	}
}

// TestDriftDetector_V2SchemaVersionBumped verifies that v2 events (with
// non-empty target weights from a non-nil provider) emit SchemaVersion=2.
func TestDriftDetector_V2SchemaVersionBumped(t *testing.T) {
	bus := &driftRecordingBus{}
	provider := &stubTargetProvider{
		weights: map[string]float64{
			"2330": 0.30, "2454": 0.25, "2317": 0.25, "2881": 0.20,
		},
	}
	d := NewDriftDetectorWithTargets(bus, provider).(*driftDetector)

	dispatchPosition(d, "2330", 700_000, "added")
	dispatchPosition(d, "2454", 100_000, "added")
	dispatchPosition(d, "2317", 100_000, "added")
	dispatchPosition(d, "2881", 100_000, "added")

	d.checkPeriod(time.Now())
	d.checkPeriod(time.Now().Add(5 * time.Minute))

	got := bus.snapshot()
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].SchemaVersion != 2 {
		t.Errorf("expected SchemaVersion=2, got %d", got[0].SchemaVersion)
	}
}

// TestDriftDetector_V1ConstructorEmitsSchemaVersion1 verifies that the
// legacy NewDriftDetector constructor (no provider) emits SchemaVersion=1,
// preserving the v1 contract. Consumers dispatching on data.schema_version
// can correctly route v1-shape events to the v1 parser.
func TestDriftDetector_V1ConstructorEmitsSchemaVersion1(t *testing.T) {
	bus := &driftRecordingBus{}
	d := NewDriftDetector(bus).(*driftDetector)

	dispatchPosition(d, "2330", 700_000, "added")
	dispatchPosition(d, "2454", 100_000, "added")
	dispatchPosition(d, "2317", 100_000, "added")
	dispatchPosition(d, "2881", 100_000, "added")

	d.checkPeriod(time.Now())
	d.checkPeriod(time.Now().Add(5 * time.Minute))

	got := bus.snapshot()
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].SchemaVersion != 1 {
		t.Errorf("expected SchemaVersion=1 for v1 constructor, got %d", got[0].SchemaVersion)
	}
	payload := got[0].Payload.(map[string]any)
	if _, hasTargetWeights := payload["target_weights"]; hasTargetWeights {
		t.Error("v1 constructor event should not include target_weights field")
	}
	if _, hasMaxDrift := payload["max_drift"]; hasMaxDrift {
		t.Error("v1 constructor event should not include max_drift field")
	}
}

// TestDriftDetector_V2ConcurrentProviderAccess exercises concurrent reads
// of provider.GetTargetWeights under d.mu to ensure no data race.
// This test runs only with -race flag.
func TestDriftDetector_V2ConcurrentProviderAccess(t *testing.T) {
	bus := &driftRecordingBus{}
	var callCount sync.Map
	provider := &concurrentCountingProvider{counter: &callCount}
	d := NewDriftDetectorWithTargets(bus, provider).(*driftDetector)

	dispatchPosition(d, "2330", 700_000, "added")
	dispatchPosition(d, "2454", 100_000, "added")
	dispatchPosition(d, "2317", 100_000, "added")
	dispatchPosition(d, "2881", 100_000, "added")

	// Run checkPeriod concurrently with regime changes to verify no race.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 5 {
			d.checkPeriod(time.Now())
		}
	}()
	go func() {
		defer wg.Done()
		for range 5 {
			_ = d.onRegimeChangeConfirmed(context.Background(), eventbus.BusEvent{
				Type:    eventbus.EventRegimeChangeConfirmed,
				Payload: map[string]any{"new_regime": "TEST"},
			})
		}
	}()
	wg.Wait()

	_ = d.onRegimeChangeConfirmed(context.Background(), eventbus.BusEvent{
		Type:    eventbus.EventRegimeChangeConfirmed,
		Payload: map[string]any{"new_regime": "TEST"},
	})

	d.mu.Lock()
	gotRegime := d.currentRegime
	gotPrevTotal := d.prevTotal
	d.mu.Unlock()
	if gotRegime != "TEST" {
		t.Errorf("expected currentRegime=TEST after regime change, got %q", gotRegime)
	}
	if gotPrevTotal != 0 {
		t.Errorf("expected prevTotal=0 after regime change reset, got %f", gotPrevTotal)
	}
}

// concurrentCountingProvider counts calls for race detection.
type concurrentCountingProvider struct {
	counter *sync.Map
}

func (c *concurrentCountingProvider) GetTargetWeights(regime string) map[string]float64 {
	if v, ok := c.counter.Load(regime); ok {
		n := v.(int)
		c.counter.Store(regime, n+1)
	} else {
		c.counter.Store(regime, 1)
	}
	return map[string]float64{"2330": 0.30, "2454": 0.25}
}

type regimeRecordingProvider struct {
	mu    sync.Mutex
	calls []string
}

func (p *regimeRecordingProvider) GetTargetWeights(regime string) map[string]float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, regime)
	return map[string]float64{"2330": 0.5}
}

func TestDriftDetector_V2RegimeChangeTriggersNewProviderQuery(t *testing.T) {
	bus := &driftRecordingBus{}
	provider := &regimeRecordingProvider{}
	d := NewDriftDetectorWithTargets(bus, provider).(*driftDetector)

	dispatchPosition(d, "2330", 200_000, "added")
	d.checkPeriod(time.Now())
	d.checkPeriod(time.Now().Add(5 * time.Minute))

	_ = d.onRegimeChangeConfirmed(context.Background(), eventbus.BusEvent{
		Type:    eventbus.EventRegimeChangeConfirmed,
		Payload: map[string]any{"new_regime": "BULL"},
	})

	d.checkPeriod(time.Now().Add(10 * time.Minute))
	d.checkPeriod(time.Now().Add(15 * time.Minute))

	provider.mu.Lock()
	afterCalls := len(provider.calls)
	lastAfter := ""
	if afterCalls > 0 {
		lastAfter = provider.calls[afterCalls-1]
	}
	provider.mu.Unlock()

	if afterCalls == 0 {
		t.Fatal("expected provider to be queried after regime change, got 0 calls")
	}
	if lastAfter != "BULL" {
		t.Errorf("expected last provider query to use new regime BULL, got %q", lastAfter)
	}
}

func TestDriftDetector_V2EmptyRegimeStringPassesToProvider(t *testing.T) {
	bus := &driftRecordingBus{}
	provider := &regimeRecordingProvider{}
	d := NewDriftDetectorWithTargets(bus, provider).(*driftDetector)

	dispatchPosition(d, "2330", 200_000, "added")
	d.checkPeriod(time.Now())

	provider.mu.Lock()
	firstRegime := ""
	if len(provider.calls) > 0 {
		firstRegime = provider.calls[0]
	}
	provider.mu.Unlock()
	if firstRegime != "" {
		t.Errorf("expected first provider query to use empty regime (no regime change yet), got %q", firstRegime)
	}
}

func TestDriftDetector_V2StopCancelsBothSubscriptions(t *testing.T) {
	bus := &regimeSpyBus{}
	provider := &stubTargetProvider{weights: map[string]float64{"2330": 0.5}}
	d := NewDriftDetectorWithTargets(bus, provider).(*driftDetector)

	if err := d.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if err := d.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

type regimeSpyBus struct {
	driftRecordingBus
	subscribeCount map[eventbus.EventType]int
}

func (b *regimeSpyBus) Subscribe(eventType eventbus.EventType, _ eventbus.EventHandler) eventbus.Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.subscribeCount == nil {
		b.subscribeCount = make(map[eventbus.EventType]int)
	}
	b.subscribeCount[eventType]++
	return eventbus.Subscription{Cancel: func() {}}
}

func (b *regimeSpyBus) SubscribeAll(_ eventbus.EventHandler) eventbus.Subscription {
	return eventbus.Subscription{Cancel: func() {}}
}

func TestDriftDetector_V1StartDoesNotSubscribeToRegime(t *testing.T) {
	bus := &regimeSpyBus{}
	d := NewDriftDetector(bus).(*driftDetector)

	if err := d.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	t.Cleanup(func() { _ = d.Stop() })

	bus.mu.Lock()
	defer bus.mu.Unlock()
	if bus.subscribeCount[eventbus.EventRegimeChangeConfirmed] != 0 {
		t.Errorf("v1 detector must not subscribe to EventRegimeChangeConfirmed, got %d subscriptions",
			bus.subscribeCount[eventbus.EventRegimeChangeConfirmed])
	}
	if bus.subscribeCount[eventbus.EventPositionUpdate] != 1 {
		t.Errorf("v1 detector must subscribe to EventPositionUpdate exactly once, got %d",
			bus.subscribeCount[eventbus.EventPositionUpdate])
	}
}

func TestDriftDetector_V2StartSubscribesToBoth(t *testing.T) {
	bus := &regimeSpyBus{}
	provider := &stubTargetProvider{weights: map[string]float64{"2330": 0.5}}
	d := NewDriftDetectorWithTargets(bus, provider).(*driftDetector)

	if err := d.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	t.Cleanup(func() { _ = d.Stop() })

	bus.mu.Lock()
	defer bus.mu.Unlock()
	if bus.subscribeCount[eventbus.EventRegimeChangeConfirmed] != 1 {
		t.Errorf("v2 detector must subscribe to EventRegimeChangeConfirmed, got %d",
			bus.subscribeCount[eventbus.EventRegimeChangeConfirmed])
	}
	if bus.subscribeCount[eventbus.EventPositionUpdate] != 1 {
		t.Errorf("v2 detector must subscribe to EventPositionUpdate exactly once, got %d",
			bus.subscribeCount[eventbus.EventPositionUpdate])
	}
}
