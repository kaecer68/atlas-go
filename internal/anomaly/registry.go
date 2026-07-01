package anomaly

import (
	"context"
	"fmt"
	"time"
)

// EventPublisher publishes anomaly events to a downstream bus. It is a narrow
// interface so internal/anomaly does not depend on the concrete eventbus
// package; cmd/atlas-mcp/server wires a real eventbus adapter here.
type EventPublisher interface {
	Publish(ctx context.Context, eventType string, payload any) error
}

// Alerter is the alerting integration seam. T1.2 wires a no-op implementation;
// a future change can route anomalies to internal/alerting or Alertmanager.
type Alerter interface {
	PublishAnomalies(ctx context.Context, anomalies []Anomaly) error
}

// FetchEntriesFunc returns the audit entries available for the current detect
// pass. The registry does not know how entries are stored.
type FetchEntriesFunc func(ctx context.Context) ([]AuditEntryV2, error)

// Registry holds configured detectors and routes detected anomalies to the
// event publisher and alerter.
type Registry struct {
	cfg       Config
	detectors []Detector
	publisher EventPublisher
	alerter   Alerter
	now       func() time.Time
}

// NewRegistry creates a registry with the given configuration and sinks.
func NewRegistry(cfg Config, publisher EventPublisher, alerter Alerter) *Registry {
	if publisher == nil {
		publisher = NoopEventPublisher{}
	}
	if alerter == nil {
		alerter = NoopAlerter{}
	}
	return &Registry{
		cfg:       cfg,
		publisher: publisher,
		alerter:   alerter,
		now:       time.Now,
	}
}

// Register adds a detector to the registry.
func (r *Registry) Register(d Detector) {
	if d == nil {
		return
	}
	r.detectors = append(r.detectors, d)
}

// Detect runs all registered detectors and aggregates their anomalies.
// Non-empty results are forwarded to the publisher and alerter.
func (r *Registry) Detect(ctx context.Context, entries []AuditEntryV2) ([]Anomaly, error) {
	var all []Anomaly
	for _, d := range r.detectors {
		found, err := d.Detect(ctx, entries)
		if err != nil {
			return nil, fmt.Errorf("detector %s: %w", d.Name(), err)
		}
		all = append(all, found...)
	}
	if len(all) == 0 {
		return nil, nil
	}

	if err := r.publisher.Publish(ctx, "mcp.anomaly.detected", all); err != nil {
		return all, fmt.Errorf("publish anomalies: %w", err)
	}
	if err := r.alerter.PublishAnomalies(ctx, all); err != nil {
		return all, fmt.Errorf("alert anomalies: %w", err)
	}
	return all, nil
}

// RunLoop runs detection at cfg.DetectIntervalSec until ctx is cancelled.
// Each iteration fetches entries, runs Detect, and waits for the next tick.
func (r *Registry) RunLoop(ctx context.Context, fetch FetchEntriesFunc) {
	if fetch == nil {
		return
	}
	interval := time.Duration(r.cfg.DetectIntervalSec) * time.Second
	if interval <= 0 {
		interval = 60 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run once immediately so startup behavior is observable in tests.
	r.runOnce(ctx, fetch)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.runOnce(ctx, fetch)
		}
	}
}

func (r *Registry) runOnce(ctx context.Context, fetch FetchEntriesFunc) {
	entries, err := fetch(ctx)
	if err != nil {
		return
	}
	_, _ = r.Detect(ctx, entries)
}

// NoopEventPublisher discards published events. It is the default publisher
// when none is supplied, keeping the registry safe to construct in tests.
type NoopEventPublisher struct{}

// Publish implements EventPublisher as a no-op.
func (NoopEventPublisher) Publish(_ context.Context, _ string, _ any) error { return nil }

// NoopAlerter discards alert calls. It is the default alerter when none is
// supplied, satisfying the alerting seam without emitting alerts in T1.2.
type NoopAlerter struct{}

// PublishAnomalies implements Alerter as a no-op.
func (NoopAlerter) PublishAnomalies(_ context.Context, _ []Anomaly) error { return nil }
