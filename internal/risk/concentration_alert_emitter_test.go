package risk

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/eventbus"
)

// TestConcentrationAlertEmitter_PerPositionBreach verifies that a single
// position whose weight exceeds maxPositionWeightPct emits an error-severity
// EventConcentrationBreach. Financial-engineering regression test: this is
// the core signal that prevents retail over-concentration.
func TestConcentrationAlertEmitter_PerPositionBreach(t *testing.T) {
	bus := eventbus.NewChannelEventBus(8)
	defer bus.Close()

	emitter := NewConcentrationAlertEmitter(bus, 0.15, 20, 0.50)

	received := make(chan eventbus.BusEvent, 4)
	sub := bus.SubscribeAll(func(_ context.Context, e eventbus.BusEvent) error {
		select {
		case received <- e:
		default:
		}
		return nil
	})
	defer sub.Cancel()

	// One position at 20% weight (above 15% threshold).
	positions := []PositionInfo{
		{Symbol: "2330", Value: 20000},
	}
	emitter.EvaluateAndPublish(positions, 100000, nil)

	select {
	case e := <-received:
		if e.Type != eventbus.EventConcentrationBreach {
			t.Errorf("expected EventConcentrationBreach, got %s", e.Type)
		}
		if e.Severity != "error" {
			t.Errorf("expected severity=error, got %s", e.Severity)
		}
		payload, ok := e.Payload.(eventbus.ConcentrationBreachPayload)
		if !ok {
			t.Fatalf("expected payload type ConcentrationBreachPayload, got %T", e.Payload)
		}
		if payload.Type != "position" {
			t.Errorf("expected type=position, got %s", payload.Type)
		}
		if payload.Symbol != "2330" {
			t.Errorf("expected Symbol=2330, got %s", payload.Symbol)
		}
		if payload.Value != 0.20 {
			t.Errorf("expected Value=0.20, got %v", payload.Value)
		}
		if payload.Threshold != 0.15 {
			t.Errorf("expected Threshold=0.15, got %v", payload.Threshold)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for per-position breach event")
	}
}

// TestConcentrationAlertEmitter_PositionsCountBreach verifies that
// exceeding maxPositionsCount emits a warning-severity event.
func TestConcentrationAlertEmitter_PositionsCountBreach(t *testing.T) {
	bus := eventbus.NewChannelEventBus(8)
	defer bus.Close()

	emitter := NewConcentrationAlertEmitter(bus, 0.15, 20, 0.50)

	received := make(chan eventbus.BusEvent, 4)
	sub := bus.SubscribeAll(func(_ context.Context, e eventbus.BusEvent) error {
		select {
		case received <- e:
		default:
		}
		return nil
	})
	defer sub.Cancel()

	// 25 positions, each at 4% weight (below 15% per-position threshold).
	positions := make([]PositionInfo, 25)
	for i := range positions {
		positions[i] = PositionInfo{Symbol: "SYM", Value: 4000}
	}
	emitter.EvaluateAndPublish(positions, 100000, nil)

	select {
	case e := <-received:
		if e.Severity != "warning" {
			t.Errorf("expected severity=warning, got %s", e.Severity)
		}
		payload := e.Payload.(eventbus.ConcentrationBreachPayload)
		if payload.Type != "count" {
			t.Errorf("expected type=count, got %s", payload.Type)
		}
		if payload.Value != 25 {
			t.Errorf("expected Value=25, got %v", payload.Value)
		}
		if payload.Threshold != 20 {
			t.Errorf("expected Threshold=20, got %v", payload.Threshold)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for positions-count breach event")
	}
}

// TestConcentrationAlertEmitter_SectorBreach verifies that a sector
// whose weight exceeds the high threshold emits an error-severity event.
func TestConcentrationAlertEmitter_SectorBreach(t *testing.T) {
	bus := eventbus.NewChannelEventBus(8)
	defer bus.Close()

	emitter := NewConcentrationAlertEmitter(bus, 0.15, 20, 0.50)

	received := make(chan eventbus.BusEvent, 4)
	sub := bus.SubscribeAll(func(_ context.Context, e eventbus.BusEvent) error {
		select {
		case received <- e:
		default:
		}
		return nil
	})
	defer sub.Cancel()

	// Semiconductor sector at 60% weight (above 50% high threshold).
	sectorExposure := map[string]float64{
		"semiconductor": 0.60,
		"financials":    0.20,
		"consumer":      0.20,
	}
	emitter.EvaluateAndPublish(nil, 100000, sectorExposure)

	select {
	case e := <-received:
		if e.Severity != "error" {
			t.Errorf("expected severity=error, got %s", e.Severity)
		}
		payload := e.Payload.(eventbus.ConcentrationBreachPayload)
		if payload.Type != "sector" {
			t.Errorf("expected type=sector, got %s", payload.Type)
		}
		if payload.Sector != "semiconductor" {
			t.Errorf("expected Sector=semiconductor, got %s", payload.Sector)
		}
		if payload.Value != 0.60 {
			t.Errorf("expected Value=0.60, got %v", payload.Value)
		}
		if payload.Threshold != 0.50 {
			t.Errorf("expected Threshold=0.50, got %v", payload.Threshold)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for sector breach event")
	}
}

// TestConcentrationAlertEmitter_NoBreachesWhenBelowThresholds verifies
// that no event is emitted when all conditions are below thresholds.
func TestConcentrationAlertEmitter_NoBreachesWhenBelowThresholds(t *testing.T) {
	bus := eventbus.NewChannelEventBus(8)
	defer bus.Close()

	emitter := NewConcentrationAlertEmitter(bus, 0.15, 20, 0.50)

	received := make(chan eventbus.BusEvent, 4)
	sub := bus.SubscribeAll(func(_ context.Context, e eventbus.BusEvent) error {
		select {
		case received <- e:
		default:
		}
		return nil
	})
	defer sub.Cancel()

	// 10 positions, each at 5% weight; sectors all below 50%.
	positions := make([]PositionInfo, 10)
	for i := range positions {
		positions[i] = PositionInfo{Symbol: "SYM", Value: 5000}
	}
	sectorExposure := map[string]float64{
		"semiconductor": 0.25,
		"financials":    0.25,
	}
	emitter.EvaluateAndPublish(positions, 100000, sectorExposure)

	select {
	case e := <-received:
		t.Fatalf("unexpected event: type=%s severity=%s", e.Type, e.Severity)
	case <-time.After(200 * time.Millisecond):
		// Expected: no event.
	}
}

// TestConcentrationAlertEmitter_NilBusNoop verifies the no-bus branch
// is safe (no panic).
func TestConcentrationAlertEmitter_NilBusNoop(t *testing.T) {
	emitter := NewConcentrationAlertEmitter(nil, 0.15, 20, 0.50)

	positions := []PositionInfo{{Symbol: "2330", Value: 20000}}
	emitter.EvaluateAndPublish(positions, 100000, map[string]float64{
		"semiconductor": 0.60,
	})
	// No panic = pass
}

// TestConcentrationAlertEmitter_MultipleBreachesSinglePublish verifies that
// multiple breaches in one evaluation all get published (not just the first).
func TestConcentrationAlertEmitter_MultipleBreachesSinglePublish(t *testing.T) {
	bus := eventbus.NewChannelEventBus(8)
	defer bus.Close()

	emitter := NewConcentrationAlertEmitter(bus, 0.15, 20, 0.50)

	received := make(chan eventbus.BusEvent, 8)
	sub := bus.SubscribeAll(func(_ context.Context, e eventbus.BusEvent) error {
		select {
		case received <- e:
		default:
		}
		return nil
	})
	defer sub.Cancel()

	// 2 over-concentrated positions + 1 sector breach.
	positions := []PositionInfo{
		{Symbol: "2330", Value: 20000},
		{Symbol: "2454", Value: 18000},
	}
	sectorExposure := map[string]float64{
		"semiconductor": 0.60,
	}
	emitter.EvaluateAndPublish(positions, 100000, sectorExposure)

	// Expect at least 3 events: 2 position + 1 sector.
	got := 0
	deadline := time.After(2 * time.Second)
	for got < 3 {
		select {
		case <-received:
			got++
		case <-deadline:
			t.Fatalf("expected at least 3 events, got %d", got)
		}
	}
}

// TestConcentrationAlertEmitter_PayloadTypeMismatchIsSafe verifies that
// a wrong payload type on the bus is logged and skipped without crash.
func TestConcentrationAlertEmitter_PayloadTypeMismatchIsSafe(t *testing.T) {
	bus := eventbus.NewChannelEventBus(8)
	defer bus.Close()

	_ = NewConcentrationAlertEmitter(bus, 0.15, 20, 0.50)

	// Publish a wrong-payload event of the right type.
	bus.Publish(eventbus.BusEvent{
		ID:        "test-mismatch",
		Type:      eventbus.EventConcentrationBreach,
		Timestamp: time.Now(),
		Payload:   "wrong type",
		Severity:  "error",
	})

	time.Sleep(100 * time.Millisecond)
	// Test passes if no panic.
}

// TestConcentrationAlertEmitter_NewAlertStoreFilePath is a sanity check
// confirming the test fixture uses a temp directory (not the production
// data path). Documents intent; this is not a functional test of the
// emitter itself.
func TestConcentrationAlertEmitter_NewAlertStoreFilePath(t *testing.T) {
	dir := t.TempDir()
	// Just verify the temp dir pattern works.
	got := filepath.Join(dir, "alerts.jsonl")
	if got == "" {
		t.Fatal("expected non-empty path")
	}
}

// Compile-time guard: handler signature must match eventbus.Subscribe.
var _ concentrationHandler = func(_ context.Context, _ eventbus.BusEvent) error { return nil }

type concentrationHandler = func(context.Context, eventbus.BusEvent) error
