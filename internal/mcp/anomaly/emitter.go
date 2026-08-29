package anomaly

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/alerting"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// SeverityFunc maps an anomaly type + score to a severity label. Injectable
// so tests can pin a constant; production uses defaultSeverityFn.
type SeverityFunc func(anomalyType string, score float64) string

// AnomalyObserver is the metrics hook called once per emitted anomaly.
// Distinct from ScoreRecorder because the emitter also carries severity
// (for the per-severity emission counter).
type AnomalyObserver interface {
	ObserveAnomaly(tenantID, anomalyType, severity string, score float64)
}

// EmitterConfig wires the Emitter to its dependencies. All fields except
// Interval are required; missing required fields panic in NewEmitter
// because misconfiguration must surface at boot, not on the first anomaly.
type EmitterConfig struct {
	Detector   *Detector
	Publisher  alerting.AnomalyPublisher
	AckStore   AnomalyStore
	Bus        *eventbus.ChannelEventBus // optional; nil disables eventbus publish
	Observer   AnomalyObserver           // optional; nil disables metrics
	SeverityFn SeverityFunc              // optional; defaults to defaultSeverityFn
	Interval   time.Duration             // optional; default 1s
	BatchSize  int                       // optional; default 50
}

// Emitter fans an AnomalyEvent out from the detector's ring buffer to:
// (a) the alert publisher (Alertmanager webhook / noop / etc.)
// (b) the anomaly ack store (so the operator dashboard can list & ack)
// (c) the event bus (so SSE subscribers see the event)
// (d) the metrics registry (so the gauge + counter both move).
//
// The Emitter is decoupled from the detector by polling its Store at
// Interval. The detector is a hot path; this lets T1.4 land without
// touching detector.go (per the spec's "MUST NOT DO" rule).
type Emitter struct {
	cfg        EmitterConfig
	severityFn SeverityFunc
	dedup      map[string]struct{}
	mu         sync.Mutex
}

// NewEmitter constructs an Emitter. Detector / Publisher / AckStore are
// required; Bus / ScoreSink / SeverityFn / Interval / BatchSize have
// sensible defaults (nil-safe, see process).
func NewEmitter(cfg EmitterConfig) *Emitter {
	if cfg.Detector == nil {
		panic("anomaly: EmitterConfig.Detector is required")
	}
	if cfg.Publisher == nil {
		panic("anomaly: EmitterConfig.Publisher is required")
	}
	if cfg.AckStore == nil {
		panic("anomaly: EmitterConfig.AckStore is required")
	}
	if cfg.SeverityFn == nil {
		cfg.SeverityFn = defaultSeverityFn
	}
	if cfg.Interval <= 0 {
		cfg.Interval = time.Second
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 50
	}
	return &Emitter{
		cfg:        cfg,
		severityFn: cfg.SeverityFn,
		dedup:      make(map[string]struct{}),
	}
}

// ProcessOnce walks the most-recent detector events, dispatches any
// unseen ones, and updates the dedup set. Returns nil unless a
// programmer-error invariant fails (publisher errors are logged, not
// returned — see design notes in emitter_test.go).
func (e *Emitter) ProcessOnce(ctx context.Context) error {
	store := e.cfg.Detector.Store()
	if store == nil {
		return nil
	}
	recent := store.Recent(e.cfg.BatchSize)
	if len(recent) == 0 {
		return nil
	}

	// recent is newest-first; we want to process in arrival order so
	// the dedup map sees the earliest unseen entry first. Reverse walk.
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, ev := range slices.Backward(recent) {

		key := dedupKey(ev)
		if _, seen := e.dedup[key]; seen {
			continue
		}
		e.dispatch(ctx, ev)
		e.dedup[key] = struct{}{}
	}
	return nil
}

// Run blocks until ctx is cancelled, calling ProcessOnce every Interval.
// Intended to be launched in its own goroutine from server wiring.
func (e *Emitter) Run(ctx context.Context) {
	ticker := time.NewTicker(e.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = e.ProcessOnce(ctx)
		}
	}
}

// dispatch performs the fan-out: save -> publish (alert) -> publish (bus) ->
// metrics. Publisher errors are logged but do not abort the dispatch loop
// so one bad webhook doesn't take the rest down.
func (e *Emitter) dispatch(ctx context.Context, ev AnomalyEvent) {
	severity := e.severityFn(ev.AnomalyType, ev.Score)

	// (a) Persist into the ack store first so a webhook failure still
	// leaves the operator dashboard seeing the event.
	sa, err := e.cfg.AckStore.Save(ev)
	if err != nil {
		logging.Warn("anomaly_emitter", "ack_store_save_failed",
			logging.FStr("anomaly_type", ev.AnomalyType),
			logging.FStr("tenant_id", ev.TenantID),
			logging.Err(err))
		return
	}

	// (b) Publish to the alert sink.
	alertEv := alerting.AnomalyEvent{
		AnomalyID:  sa.AnomalyID,
		Type:       ev.AnomalyType,
		TenantID:   ev.TenantID,
		Tool:       ev.Tool,
		Score:      ev.Score,
		DetectedAt: parseAnomalyTS(ev.TS, e.cfg.Detector.now()),
		Severity:   severity,
	}
	if err := e.cfg.Publisher.PublishAnomaly(ctx, alertEv); err != nil {
		logging.Warn("anomaly_emitter", "publisher_failed",
			logging.FStr("anomaly_id", sa.AnomalyID),
			logging.FStr("anomaly_type", ev.AnomalyType),
			logging.Err(err))
		// Continue — the event is already in the ack store and
		// metrics will still update.
	}

	// (c) Publish to the event bus (for SSE subscribers / dashboards).
	if e.cfg.Bus != nil {
		e.cfg.Bus.Publish(eventbus.BusEvent{
			ID:        sa.AnomalyID,
			Type:      eventbus.EventMCPAnomalyDetected,
			Timestamp: alertEv.DetectedAt,
			Payload: eventbus.MCPAnomalyEventPayload{
				AnomalyID:   sa.AnomalyID,
				TenantID:    ev.TenantID,
				AnomalyType: ev.AnomalyType,
				Tool:        ev.Tool,
				Score:       ev.Score,
				Severity:    severity,
				DetectedAt:  alertEv.DetectedAt.UTC().Format(time.RFC3339),
			},
			SchemaVersion: 1,
		})
	}

	// (d) Update metrics. ObserveAnomaly updates both the gauge and the
	// severity-keyed emission counter (see cmd/atlas-mcp/server/metrics.go).
	if e.cfg.Observer != nil {
		e.cfg.Observer.ObserveAnomaly(ev.TenantID, ev.AnomalyType, severity, ev.Score)
	}
}

// dedupKey returns the key used to skip already-dispatched anomalies. We
// include TS + type + tenant + tool to handle the rare case where two
// distinct anomalies share a millisecond (e.g., a burst and a tool-error
// spike in the same audit batch).
func dedupKey(ev AnomalyEvent) string {
	return fmt.Sprintf("%s|%s|%s|%s", ev.TS, ev.AnomalyType, ev.TenantID, ev.Tool)
}

// defaultSeverityFn maps a score to a severity label. The thresholds are
// tuned for the three detector baselines:
//   - burst: z-score (3.0 = 3σ above baseline)
//   - tool_error_spike / tenant_error_anomaly: error rate (0.5 = 50%)
//
// score < 3.0  -> low
// 3.0 <= score < 5.0 -> medium
// score >= 5.0  -> high
//
// Centralized so SREs have one place to tune the mapping.
func defaultSeverityFn(_ string, score float64) string {
	switch {
	case score >= 5.0:
		return "high"
	case score >= 3.0:
		return "medium"
	default:
		return "low"
	}
}

// parseAnomalyTS decodes the RFC3339 TS field of an AnomalyEvent, falling
// back to the supplied fallback time when the field is empty or invalid.
// The detector always sets TS via time.Format(RFC3339) so the fallback
// path is only hit in tests or programmatic construction.
func parseAnomalyTS(ts string, fallback time.Time) time.Time {
	if ts == "" {
		return fallback.UTC()
	}
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t.UTC()
	}
	return fallback.UTC()
}
