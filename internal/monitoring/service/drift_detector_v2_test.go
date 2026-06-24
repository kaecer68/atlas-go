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

// TestDriftDetector_V2SchemaVersionBumped verifies that emitted events
// always have SchemaVersion=2.
func TestDriftDetector_V2SchemaVersionBumped(t *testing.T) {
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
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].SchemaVersion != 2 {
		t.Errorf("expected SchemaVersion=2, got %d", got[0].SchemaVersion)
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
		for i := 0; i < 5; i++ {
			d.checkPeriod(time.Now())
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			_ = d.onRegimeChangeConfirmed(context.Background(), eventbus.BusEvent{
				Type:    eventbus.EventRegimeChangeConfirmed,
				Payload: map[string]any{"new_regime": "TEST"},
			})
		}
	}()
	wg.Wait()
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
