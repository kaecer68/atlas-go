package monitoring

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

// Wave9Observability wires the five Wave 9 observability detectors for live mode.
// It handles their startup order (RegimeDebouncer first, then parallel ingestion,
// channel-health and factor-regression detectors, then DriftDetector last) and
// shuts them down in LIFO order.
type Wave9Observability struct {
	bus eventbus.EventBus

	regimeDebouncer          service.RegimeDebouncer
	factorWeightRegression   service.FactorWeightRegressionDetector
	driftDetector            service.DriftDetector
	channelHealthSynthesizer service.ChannelHealthSynthesizer
	ingestionLagMonitor      service.IngestionLagMonitor

	weightProvider        service.WeightProvider
	targetWeightsProvider service.TargetWeightsProvider
	channelHealthProvider service.ChannelHealthProvider
	ingestionLagProvider  service.IngestionLagProvider

	factory detectorFactory

	started bool
	mu      sync.Mutex
}

// detectorFactory abstracts detector construction so tests can inject spies.
type detectorFactory interface {
	newRegimeDebouncer(bus eventbus.EventBus) service.RegimeDebouncer
	newFactorWeightRegressionDetector(bus eventbus.EventBus, provider service.WeightProvider) service.FactorWeightRegressionDetector
	newDriftDetector(bus eventbus.EventBus, provider service.TargetWeightsProvider) service.DriftDetector
	newChannelHealthSynthesizer(bus eventbus.EventBus, provider service.ChannelHealthProvider) service.ChannelHealthSynthesizer
	newIngestionLagMonitor(bus eventbus.EventBus, provider service.IngestionLagProvider) service.IngestionLagMonitor
}

type defaultDetectorFactory struct{}

func (defaultDetectorFactory) newRegimeDebouncer(bus eventbus.EventBus) service.RegimeDebouncer {
	return service.NewRegimeDebouncer(bus)
}

func (defaultDetectorFactory) newFactorWeightRegressionDetector(bus eventbus.EventBus, provider service.WeightProvider) service.FactorWeightRegressionDetector {
	return service.NewFactorWeightRegressionDetector(bus, provider)
}

func (defaultDetectorFactory) newDriftDetector(bus eventbus.EventBus, provider service.TargetWeightsProvider) service.DriftDetector {
	if provider != nil {
		return service.NewDriftDetectorWithTargets(bus, provider)
	}
	return service.NewDriftDetector(bus)
}

func (defaultDetectorFactory) newChannelHealthSynthesizer(bus eventbus.EventBus, provider service.ChannelHealthProvider) service.ChannelHealthSynthesizer {
	return service.NewChannelHealthSynthesizer(bus, provider)
}

func (defaultDetectorFactory) newIngestionLagMonitor(bus eventbus.EventBus, provider service.IngestionLagProvider) service.IngestionLagMonitor {
	return service.NewIngestionLagMonitor(bus, provider)
}

// Wave9Option configures a Wave9Observability instance.
type Wave9Option func(*Wave9Observability)

// WithWeightProvider provides the WeightProvider required by FactorWeightRegressionDetector.
func WithWeightProvider(p service.WeightProvider) Wave9Option {
	return func(w *Wave9Observability) { w.weightProvider = p }
}

// WithTargetWeightsProvider optionally provides target weights for DriftDetector v2.
// When nil, the drift detector falls back to v1 behavior.
func WithTargetWeightsProvider(p service.TargetWeightsProvider) Wave9Option {
	return func(w *Wave9Observability) { w.targetWeightsProvider = p }
}

// WithChannelHealthProvider provides the ChannelHealthProvider required by ChannelHealthSynthesizer.
func WithChannelHealthProvider(p service.ChannelHealthProvider) Wave9Option {
	return func(w *Wave9Observability) { w.channelHealthProvider = p }
}

// WithIngestionLagProvider provides the IngestionLagProvider required by IngestionLagMonitor.
func WithIngestionLagProvider(p service.IngestionLagProvider) Wave9Option {
	return func(w *Wave9Observability) { w.ingestionLagProvider = p }
}

// withDetectorFactory is an internal test hook that replaces detector construction.
func withDetectorFactory(f detectorFactory) Wave9Option {
	return func(w *Wave9Observability) { w.factory = f }
}

// NewWave9Observability creates a new Wave 9 observability coordinator.
// Required providers: ChannelHealthProvider, IngestionLagProvider.
// WeightProvider and TargetWeightsProvider are optional; when WeightProvider is nil,
// the factor-weight regression detector no-ops.
func NewWave9Observability(bus eventbus.EventBus, opts ...Wave9Option) (*Wave9Observability, error) {
	if bus == nil {
		return nil, errors.New("event bus is required")
	}

	w := &Wave9Observability{bus: bus}
	for _, opt := range opts {
		opt(w)
	}

	if w.channelHealthProvider == nil {
		return nil, errors.New("ChannelHealthProvider is required")
	}
	if w.ingestionLagProvider == nil {
		return nil, errors.New("IngestionLagProvider is required")
	}
	if w.factory == nil {
		w.factory = defaultDetectorFactory{}
	}

	return w, nil
}

// Start idempotently starts all five detectors in the prescribed order.
// On any error during startup, all detectors that started successfully are
// stopped before Start returns, and internal references are cleared so a
// subsequent retry creates fresh instances instead of reusing stale ones.
func (w *Wave9Observability) Start(ctx context.Context) (err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.started {
		return errors.New("wave9 observability already started")
	}

	// Track detectors that started successfully so we can stop them on partial
	// failure. Stops run in reverse of start order (LIFO) to mirror the normal
	// Stop() ordering semantics.
	var (
		startedMu sync.Mutex
		started   []func() error
	)
	addStarted := func(stop func() error) {
		startedMu.Lock()
		defer startedMu.Unlock()
		started = append(started, stop)
	}
	defer func() {
		if err == nil {
			return
		}
		// Stop started detectors in LIFO and aggregate any cleanup errors so
		// the caller can distinguish a clean partial-failure from one where
		// detectors leaked bus subscriptions (Stop returned an error).
		var cleanupErrs []error
		for _, s := range slices.Backward(started) {
			if stopErr := s(); stopErr != nil {
				cleanupErrs = append(cleanupErrs, stopErr)
				logging.Warn("wave9_observability", "cleanup_stop_failed", logging.Err(stopErr))
			}
		}
		if len(cleanupErrs) > 0 {
			err = fmt.Errorf("%w; cleanup failures: %w", err, errors.Join(cleanupErrs...))
		}
		// Clear references so a future Start() creates fresh instances
		// rather than reusing the failed-attempt detectors (which are
		// already stopped but still subscribed to the bus).
		w.regimeDebouncer = nil
		w.ingestionLagMonitor = nil
		w.channelHealthSynthesizer = nil
		w.factorWeightRegression = nil
		w.driftDetector = nil
		// Note: w.started is never true on the error path (it's set to
		// true only after all five Start calls succeed, line 229), so
		// the explicit reset below is a defensive no-op kept for clarity.
		w.started = false
	}()

	w.regimeDebouncer = w.factory.newRegimeDebouncer(w.bus)
	if err = w.regimeDebouncer.Start(ctx); err != nil {
		return fmt.Errorf("start regime debouncer: %w", err)
	}
	addStarted(w.regimeDebouncer.Stop)

	var wg sync.WaitGroup
	errs := make(chan error, 3)

	wg.Go(func() {
		w.ingestionLagMonitor = w.factory.newIngestionLagMonitor(w.bus, w.ingestionLagProvider)
		if startErr := w.ingestionLagMonitor.Start(ctx); startErr != nil {
			errs <- fmt.Errorf("start ingestion lag monitor: %w", startErr)
			return
		}
		addStarted(w.ingestionLagMonitor.Stop)
	})

	wg.Go(func() {
		w.channelHealthSynthesizer = w.factory.newChannelHealthSynthesizer(w.bus, w.channelHealthProvider)
		if startErr := w.channelHealthSynthesizer.Start(ctx); startErr != nil {
			errs <- fmt.Errorf("start channel health synthesizer: %w", startErr)
			return
		}
		addStarted(w.channelHealthSynthesizer.Stop)
	})

	wg.Go(func() {
		w.factorWeightRegression = w.factory.newFactorWeightRegressionDetector(w.bus, w.weightProvider)
		if startErr := w.factorWeightRegression.Start(ctx); startErr != nil {
			errs <- fmt.Errorf("start factor weight regression detector: %w", startErr)
			return
		}
		addStarted(w.factorWeightRegression.Stop)
	})

	wg.Wait()
	close(errs)
	// Collect ALL parallel-detector failures so the caller knows how many
	// subsystems were impacted, not just the first.  errs is buffered to 3
	// so close(errs) below the range never blocks.
	var startErrs []error
	for e := range errs {
		if e != nil {
			startErrs = append(startErrs, e)
		}
	}
	if len(startErrs) > 0 {
		err = errors.Join(startErrs...)
		return err
	}

	w.driftDetector = w.factory.newDriftDetector(w.bus, w.targetWeightsProvider)
	if startErr := w.driftDetector.Start(ctx); startErr != nil {
		err = fmt.Errorf("start drift detector: %w", startErr)
		return err
	}
	addStarted(w.driftDetector.Stop)

	w.started = true
	logging.Info("wave9_observability", "started")
	return nil
}

// Stop idempotently stops all detectors in LIFO order.
func (w *Wave9Observability) Stop() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.started {
		return nil
	}

	var errs []error
	if err := w.driftDetector.Stop(); err != nil {
		errs = append(errs, fmt.Errorf("stop drift detector: %w", err))
	}
	if err := w.factorWeightRegression.Stop(); err != nil {
		errs = append(errs, fmt.Errorf("stop factor weight regression detector: %w", err))
	}
	if err := w.channelHealthSynthesizer.Stop(); err != nil {
		errs = append(errs, fmt.Errorf("stop channel health synthesizer: %w", err))
	}
	if err := w.ingestionLagMonitor.Stop(); err != nil {
		errs = append(errs, fmt.Errorf("stop ingestion lag monitor: %w", err))
	}
	if err := w.regimeDebouncer.Stop(); err != nil {
		errs = append(errs, fmt.Errorf("stop regime debouncer: %w", err))
	}

	w.started = false
	if len(errs) > 0 {
		return errs[0]
	}
	logging.Info("wave9_observability", "stopped")
	return nil
}

// Close is a convenience alias for Stop.
func (w *Wave9Observability) Close() error {
	return w.Stop()
}

// ChannelHealthRecordStore is the minimal surface required to build a
// service.ChannelHealthProvider for the Wave 9 channel-health synthesizer.
type ChannelHealthRecordStore interface {
	ChannelIDs() []string
	Get(channelID string) *ChannelHealthRecord
}

type channelHealthStoreAdapter struct {
	store ChannelHealthRecordStore
}

// NewChannelHealthProviderFromStore adapts a channel health record store to the
// service.ChannelHealthProvider interface used by ChannelHealthSynthesizer.
func NewChannelHealthProviderFromStore(store ChannelHealthRecordStore) service.ChannelHealthProvider {
	return &channelHealthStoreAdapter{store: store}
}

func (a *channelHealthStoreAdapter) ChannelErrors() map[string]string {
	if a.store == nil {
		return nil
	}
	ids := a.store.ChannelIDs()
	if len(ids) == 0 {
		return nil
	}

	errs := make(map[string]string, len(ids))
	for _, id := range ids {
		rec := a.store.Get(id)
		if rec == nil {
			continue
		}
		if rec.Status == "ok" || rec.Status == "inactive" || rec.LastError == "" {
			continue
		}
		errs[id] = rec.LastError
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}
