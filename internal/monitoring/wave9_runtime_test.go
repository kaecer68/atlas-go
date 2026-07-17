package monitoring

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

type wave9RecordingBus struct {
	mu     sync.Mutex
	events []eventbus.BusEvent
}

func (b *wave9RecordingBus) Publish(ev eventbus.BusEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, ev)
}

func (b *wave9RecordingBus) Subscribe(eventbus.EventType, eventbus.EventHandler) eventbus.Subscription {
	return eventbus.Subscription{Cancel: func() {}}
}

func (b *wave9RecordingBus) SubscribeAll(eventbus.EventHandler) eventbus.Subscription {
	return eventbus.Subscription{Cancel: func() {}}
}

func (b *wave9RecordingBus) Close() error { return nil }

type wave9StubProvider struct{}

func (wave9StubProvider) GetWeights(string) map[string]float64       { return nil }
func (wave9StubProvider) ChannelErrors() map[string]string           { return nil }
func (wave9StubProvider) P99LatencySeconds() float64                 { return 0 }
func (wave9StubProvider) GetTargetWeights(string) map[string]float64 { return nil }

// spyDetector satisfies all five detector interfaces and records Start/Stop calls.
type spyDetector struct {
	name       string
	startOrder int
	stopOrder  int
	mu         sync.Mutex
}

func (d *spyDetector) Start(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.startOrder != 0 {
		return errors.New("already started")
	}
	d.startOrder = nextStartCounter()
	return nil
}

func (d *spyDetector) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopOrder != 0 {
		return nil
	}
	d.stopOrder = nextStopCounter()
	return nil
}

func (d *spyDetector) started() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.startOrder > 0
}

func (d *spyDetector) stopped() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.stopOrder > 0
}

var (
	startCounter int
	stopCounter  int
	counterMu    sync.Mutex
)

func nextStartCounter() int {
	counterMu.Lock()
	defer counterMu.Unlock()
	startCounter++
	return startCounter
}

func nextStopCounter() int {
	counterMu.Lock()
	defer counterMu.Unlock()
	stopCounter++
	return stopCounter
}

func resetCounters() {
	counterMu.Lock()
	defer counterMu.Unlock()
	startCounter = 0
	stopCounter = 0
}

type wave9SpyFactory struct {
	regimeDebouncer          *spyDetector
	factorWeightRegression   *spyDetector
	driftDetector            *spyDetector
	channelHealthSynthesizer *spyDetector
	ingestionLagMonitor      *spyDetector
}

func newWave9SpyFactory() *wave9SpyFactory {
	return &wave9SpyFactory{
		regimeDebouncer:          &spyDetector{name: "regime_debouncer"},
		factorWeightRegression:   &spyDetector{name: "factor_weight_regression"},
		driftDetector:            &spyDetector{name: "drift_detector"},
		channelHealthSynthesizer: &spyDetector{name: "channel_health_synthesizer"},
		ingestionLagMonitor:      &spyDetector{name: "ingestion_lag_monitor"},
	}
}

func (f *wave9SpyFactory) newRegimeDebouncer(eventbus.EventBus) service.RegimeDebouncer {
	return f.regimeDebouncer
}

func (f *wave9SpyFactory) newFactorWeightRegressionDetector(eventbus.EventBus, service.WeightProvider) service.FactorWeightRegressionDetector {
	return f.factorWeightRegression
}

func (f *wave9SpyFactory) newDriftDetector(eventbus.EventBus, service.TargetWeightsProvider) service.DriftDetector {
	return f.driftDetector
}

func (f *wave9SpyFactory) newChannelHealthSynthesizer(eventbus.EventBus, service.ChannelHealthProvider) service.ChannelHealthSynthesizer {
	return f.channelHealthSynthesizer
}

func (f *wave9SpyFactory) newIngestionLagMonitor(eventbus.EventBus, service.IngestionLagProvider) service.IngestionLagMonitor {
	return f.ingestionLagMonitor
}

func newWave9WithSpyFactory(t *testing.T) (*Wave9Observability, *wave9SpyFactory) {
	t.Helper()
	bus := &wave9RecordingBus{}
	stub := wave9StubProvider{}
	spy := newWave9SpyFactory()
	w, err := NewWave9Observability(
		bus,
		WithWeightProvider(stub),
		WithChannelHealthProvider(stub),
		WithIngestionLagProvider(stub),
		withDetectorFactory(spy),
	)
	require.NoError(t, err)
	return w, spy
}

func TestWave9Observability_StartStop(t *testing.T) {
	resetCounters()
	w, spy := newWave9WithSpyFactory(t)
	ctx := context.Background()

	require.NoError(t, w.Start(ctx))
	assert.True(t, spy.regimeDebouncer.started())
	assert.True(t, spy.driftDetector.started())

	// Double Start returns an error.
	err := w.Start(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already started")

	require.NoError(t, w.Stop())
	assert.True(t, spy.regimeDebouncer.stopped())
	assert.True(t, spy.driftDetector.stopped())

	// Double Stop is a no-op.
	require.NoError(t, w.Stop())
}

func TestWave9Observability_StartOrder(t *testing.T) {
	resetCounters()
	w, spy := newWave9WithSpyFactory(t)

	require.NoError(t, w.Start(context.Background()))

	// Regime debouncer must start first; drift detector must start last.
	assert.Less(t, spy.regimeDebouncer.startOrder, spy.ingestionLagMonitor.startOrder)
	assert.Less(t, spy.regimeDebouncer.startOrder, spy.channelHealthSynthesizer.startOrder)
	assert.Less(t, spy.regimeDebouncer.startOrder, spy.factorWeightRegression.startOrder)
	assert.Less(t, spy.ingestionLagMonitor.startOrder, spy.driftDetector.startOrder)
	assert.Less(t, spy.channelHealthSynthesizer.startOrder, spy.driftDetector.startOrder)
	assert.Less(t, spy.factorWeightRegression.startOrder, spy.driftDetector.startOrder)

	_ = w.Stop()
}

func TestWave9Observability_StopOrder(t *testing.T) {
	resetCounters()
	w, spy := newWave9WithSpyFactory(t)

	require.NoError(t, w.Start(context.Background()))
	require.NoError(t, w.Stop())

	// LIFO order: drift first, then factor/channel/ingestion, then regime last.
	assert.Less(t, spy.driftDetector.stopOrder, spy.factorWeightRegression.stopOrder)
	assert.Less(t, spy.driftDetector.stopOrder, spy.channelHealthSynthesizer.stopOrder)
	assert.Less(t, spy.driftDetector.stopOrder, spy.ingestionLagMonitor.stopOrder)
	assert.Less(t, spy.factorWeightRegression.stopOrder, spy.regimeDebouncer.stopOrder)
	assert.Less(t, spy.channelHealthSynthesizer.stopOrder, spy.regimeDebouncer.stopOrder)
	assert.Less(t, spy.ingestionLagMonitor.stopOrder, spy.regimeDebouncer.stopOrder)
}

func TestWave9Observability_RequiresProviders(t *testing.T) {
	bus := &wave9RecordingBus{}
	stub := wave9StubProvider{}

	cases := []struct {
		name string
		opts []Wave9Option
	}{
		{
			name: "missing ChannelHealthProvider",
			opts: []Wave9Option{
				WithWeightProvider(stub),
				WithIngestionLagProvider(stub),
			},
		},
		{
			name: "missing IngestionLagProvider",
			opts: []Wave9Option{
				WithWeightProvider(stub),
				WithChannelHealthProvider(stub),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewWave9Observability(bus, tc.opts...)
			require.Error(t, err)
			field := strings.TrimPrefix(tc.name, "missing ")
			assert.Contains(t, err.Error(), field+" is required")
		})
	}
}

func TestWave9Observability_NilWeightProvider(t *testing.T) {
	bus := &wave9RecordingBus{}
	stub := wave9StubProvider{}
	spy := newWave9SpyFactory()

	w, err := NewWave9Observability(
		bus,
		WithChannelHealthProvider(stub),
		WithIngestionLagProvider(stub),
		withDetectorFactory(spy),
	)
	require.NoError(t, err)
	require.NoError(t, w.Start(context.Background()))
	assert.True(t, spy.factorWeightRegression.started())
	require.NoError(t, w.Stop())
}

func TestWave9Observability_NilBus(t *testing.T) {
	stub := wave9StubProvider{}
	_, err := NewWave9Observability(
		nil,
		WithWeightProvider(stub),
		WithChannelHealthProvider(stub),
		WithIngestionLagProvider(stub),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "event bus is required")
}

func TestWave9Observability_TargetWeightsProviderOptional(t *testing.T) {
	bus := &wave9RecordingBus{}
	stub := wave9StubProvider{}
	spy := newWave9SpyFactory()
	w, err := NewWave9Observability(
		bus,
		WithWeightProvider(stub),
		WithChannelHealthProvider(stub),
		WithIngestionLagProvider(stub),
		withDetectorFactory(spy),
	)
	require.NoError(t, err)
	require.NoError(t, w.Start(context.Background()))
	// With nil target provider, default factory should use NewDriftDetector (v1).
	assert.True(t, spy.driftDetector.started())
	require.NoError(t, w.Stop())
}

func TestWave9Observability_CloseAliasesStop(t *testing.T) {
	resetCounters()
	w, spy := newWave9WithSpyFactory(t)

	require.NoError(t, w.Start(context.Background()))
	require.NoError(t, w.Close())
	assert.True(t, spy.regimeDebouncer.stopped())
	assert.True(t, spy.driftDetector.stopped())
}

func TestNewChannelHealthProviderFromStore(t *testing.T) {
	store := &wave9StubHealthStore{
		ids: []string{"ok_channel", "error_channel"},
		records: map[string]*ChannelHealthRecord{
			"ok_channel":    {Status: "ok", LastError: ""},
			"error_channel": {Status: "error", LastError: "rate limited"},
		},
	}

	provider := NewChannelHealthProviderFromStore(store)
	errs := provider.ChannelErrors()
	require.Len(t, errs, 1)
	assert.Equal(t, "rate limited", errs["error_channel"])
}

func TestNewChannelHealthProviderFromStore_NoErrors(t *testing.T) {
	store := &wave9StubHealthStore{
		ids: []string{"ok_channel"},
		records: map[string]*ChannelHealthRecord{
			"ok_channel": {Status: "ok", LastError: ""},
		},
	}

	provider := NewChannelHealthProviderFromStore(store)
	assert.Nil(t, provider.ChannelErrors())
}

type wave9StubHealthStore struct {
	ids     []string
	records map[string]*ChannelHealthRecord
}

func (s *wave9StubHealthStore) ChannelIDs() []string { return s.ids }
func (s *wave9StubHealthStore) Get(id string) *ChannelHealthRecord {
	return s.records[id]
}

func TestWave9Observability_ParallelStart(t *testing.T) {
	resetCounters()
	w, spy := newWave9WithSpyFactory(t)

	// Use a short deadline to prove parallel start does not serialize too slowly.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, w.Start(ctx))

	// All five detectors should have started.
	assert.True(t, spy.regimeDebouncer.started())
	assert.True(t, spy.factorWeightRegression.started())
	assert.True(t, spy.channelHealthSynthesizer.started())
	assert.True(t, spy.ingestionLagMonitor.started())
	assert.True(t, spy.driftDetector.started())

	require.NoError(t, w.Stop())
}
