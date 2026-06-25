package monitoring

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

// wave9CleanupSpy is a test double that records Start/Stop invocations.
// It implements the common Start(ctx) error / Stop() error shape shared by
// all 5 Wave 9 detector interfaces.
type wave9CleanupSpy struct {
	name       string
	startErr   error // returned by Start if non-nil
	started    atomic.Bool
	stopped    atomic.Bool
	startCount atomic.Int32
	stopCount  atomic.Int32
}

func (d *wave9CleanupSpy) Start(ctx context.Context) error {
	d.startCount.Add(1)
	if d.startErr != nil {
		return d.startErr
	}
	d.started.Store(true)
	return nil
}

func (d *wave9CleanupSpy) Stop() error {
	d.stopCount.Add(1)
	d.started.Store(false)
	d.stopped.Store(true)
	return nil
}

// spyFactory routes detector construction to a set of wave9CleanupSpys so tests
// can observe the Start/Stop lifecycle that Wave9Observability drives.
type spyFactory struct {
	regime        *wave9CleanupSpy
	ingestion     *wave9CleanupSpy
	channelHealth *wave9CleanupSpy
	factor        *wave9CleanupSpy
	drift         *wave9CleanupSpy
}

func (f *spyFactory) newRegimeDebouncer(_ eventbus.EventBus) service.RegimeDebouncer {
	return f.regime
}

func (f *spyFactory) newIngestionLagMonitor(_ eventbus.EventBus, _ service.IngestionLagProvider) service.IngestionLagMonitor {
	return f.ingestion
}

func (f *spyFactory) newChannelHealthSynthesizer(_ eventbus.EventBus, _ service.ChannelHealthProvider) service.ChannelHealthSynthesizer {
	return f.channelHealth
}

func (f *spyFactory) newFactorWeightRegressionDetector(_ eventbus.EventBus, _ service.WeightProvider) service.FactorWeightRegressionDetector {
	return f.factor
}

func (f *spyFactory) newDriftDetector(_ eventbus.EventBus, _ service.TargetWeightsProvider) service.DriftDetector {
	return f.drift
}

func newWave9CleanupFactory(channelHealthStartErr error) *spyFactory {
	return &spyFactory{
		regime:        &wave9CleanupSpy{name: "regime"},
		ingestion:     &wave9CleanupSpy{name: "ingestion"},
		channelHealth: &wave9CleanupSpy{name: "channelHealth", startErr: channelHealthStartErr},
		factor:        &wave9CleanupSpy{name: "factor"},
		drift:         &wave9CleanupSpy{name: "drift"},
	}
}

func newWave9ForTest(t *testing.T, bus eventbus.EventBus, factory detectorFactory) *Wave9Observability {
	t.Helper()
	w, err := NewWave9Observability(bus,
		WithWeightProvider(&wave9StaticWeightProvider{}),
		WithChannelHealthProvider(&wave9StaticChannelHealthProvider{}),
		WithIngestionLagProvider(&wave9StaticIngestionLagProvider{p99: 1.0}),
		withDetectorFactory(factory),
	)
	if err != nil {
		t.Fatalf("NewWave9Observability failed: %v", err)
	}
	return w
}

// TestWave9Integration_StartCleansUpParallelDetectorFailure verifies that when
// one of the three parallel-starting detectors fails, the other detectors
// that started successfully are Stopped before Start returns, so their
// goroutines and bus subscriptions are not leaked.
func TestWave9Integration_StartCleansUpParallelDetectorFailure(t *testing.T) {
	bus := eventbus.NewChannelEventBus(256)
	defer func() { _ = bus.Close() }()

	factory := newWave9CleanupFactory(errors.New("simulated channel-health start failure"))
	w := newWave9ForTest(t, bus, factory)

	err := w.Start(context.Background())
	if err == nil {
		t.Fatal("expected Start to return error when channelHealth fails")
	}

	// Regime debouncer started synchronously first → must be Stopped on cleanup
	if !factory.regime.stopped.Load() {
		t.Error("regime debouncer was started but not stopped after partial failure")
	}
	// The 2 parallel detectors that succeeded (ingestion, factor) must be Stopped
	if !factory.ingestion.stopped.Load() {
		t.Error("ingestion lag monitor was started but not stopped after partial failure")
	}
	if !factory.factor.stopped.Load() {
		t.Error("factor weight regression was started but not stopped after partial failure")
	}
	// The detector that failed to start must NOT be Stopped (it was never running)
	if factory.channelHealth.stopped.Load() {
		t.Error("failing channel health must not be stopped (Start never succeeded)")
	}
	// Drift detector is the LAST sequential start → must never be reached on parallel failure
	if got := factory.drift.startCount.Load(); got != 0 {
		t.Errorf("drift detector must not be started when earlier detector fails, got %d Start calls", got)
	}
}

// TestWave9Integration_StartCleansUpDriftDetectorFailure verifies that when
// the sequentially-started drift detector fails, all 4 previously-started
// detectors are Stopped before Start returns.
func TestWave9Integration_StartCleansUpDriftDetectorFailure(t *testing.T) {
	bus := eventbus.NewChannelEventBus(256)
	defer func() { _ = bus.Close() }()

	// All 4 succeed, but drift fails
	factory := &spyFactory{
		regime:        &wave9CleanupSpy{name: "regime"},
		ingestion:     &wave9CleanupSpy{name: "ingestion"},
		channelHealth: &wave9CleanupSpy{name: "channelHealth"},
		factor:        &wave9CleanupSpy{name: "factor"},
		drift:         &wave9CleanupSpy{name: "drift", startErr: errors.New("simulated drift start failure")},
	}
	w := newWave9ForTest(t, bus, factory)

	err := w.Start(context.Background())
	if err == nil {
		t.Fatal("expected Start to return error when drift detector fails")
	}

	// All 4 must be stopped
	for name, d := range map[string]*wave9CleanupSpy{
		"regime":        factory.regime,
		"ingestion":     factory.ingestion,
		"channelHealth": factory.channelHealth,
		"factor":        factory.factor,
	} {
		if !d.stopped.Load() {
			t.Errorf("%s was started but not stopped after drift failure", name)
		}
	}
	// Drift never started successfully → must not be stopped
	if factory.drift.stopped.Load() {
		t.Error("failing drift detector must not be stopped (Start never succeeded)")
	}
}

// TestWave9Integration_StartClearsReferencesOnFailure verifies that after a
// failed Start, internal detector references are cleared so a subsequent
// retry creates fresh instances (not stale ones from the failed attempt).
func TestWave9Integration_StartClearsReferencesOnFailure(t *testing.T) {
	bus := eventbus.NewChannelEventBus(256)
	defer func() { _ = bus.Close() }()

	factory := newWave9CleanupFactory(errors.New("simulated failure"))
	w := newWave9ForTest(t, bus, factory)

	if err := w.Start(context.Background()); err == nil {
		t.Fatal("expected Start to fail")
	}

	// After failure, internal fields should be nil so a retry uses fresh detectors.
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.regimeDebouncer != nil {
		t.Error("regimeDebouncer should be nil after failed Start")
	}
	if w.ingestionLagMonitor != nil {
		t.Error("ingestionLagMonitor should be nil after failed Start")
	}
	if w.channelHealthSynthesizer != nil {
		t.Error("channelHealthSynthesizer should be nil after failed Start")
	}
	if w.factorWeightRegression != nil {
		t.Error("factorWeightRegression should be nil after failed Start")
	}
	if w.driftDetector != nil {
		t.Error("driftDetector should be nil after failed Start")
	}
	if w.started {
		t.Error("started should be false after failed Start")
	}
}

// TestWave9Integration_StartCanBeRetriedAfterFailure verifies that after a
// failed Start (with cleanup), a subsequent Start with a working factory
// succeeds and brings the system to a normal running state.
func TestWave9Integration_StartCanBeRetriedAfterFailure(t *testing.T) {
	bus := eventbus.NewChannelEventBus(256)
	defer func() { _ = bus.Close() }()

	// First attempt fails
	failing := newWave9CleanupFactory(errors.New("first attempt fail"))
	w := newWave9ForTest(t, bus, failing)

	if err := w.Start(context.Background()); err == nil {
		t.Fatal("expected first Start to fail")
	}

	// Replace factory with all-working detectors
	working := &spyFactory{
		regime:        &wave9CleanupSpy{name: "regime2"},
		ingestion:     &wave9CleanupSpy{name: "ingestion2"},
		channelHealth: &wave9CleanupSpy{name: "channelHealth2"},
		factor:        &wave9CleanupSpy{name: "factor2"},
		drift:         &wave9CleanupSpy{name: "drift2"},
	}
	w.factory = working

	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("retry Start failed: %v", err)
	}
	defer func() { _ = w.Stop() }()

	// Verify the NEW detectors are running, not the old (already-stopped) ones
	for name, d := range map[string]*wave9CleanupSpy{
		"regime2":        working.regime,
		"ingestion2":     working.ingestion,
		"channelHealth2": working.channelHealth,
		"factor2":        working.factor,
		"drift2":         working.drift,
	} {
		if !d.started.Load() {
			t.Errorf("%s should be started after retry", name)
		}
		if d.stopped.Load() {
			t.Errorf("%s should not be stopped after retry", name)
		}
	}

	// Sanity: the OLD detectors were stopped by the first cleanup, and the
	// retry must not call Stop on them a second time.
	if !failing.regime.stopped.Load() {
		t.Error("old regime detector should be stopped after first failure")
	}
	if got := failing.regime.stopCount.Load(); got != 1 {
		t.Errorf("old regime detector must be Stopped exactly once across both attempts, got %d", got)
	}
}
