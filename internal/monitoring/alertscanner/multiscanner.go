package alertscanner

import (
	"context"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// AlertSource is a pluggable alert provider. MultiScanner aggregates
// results from all registered sources and produces a unified Snapshot.
//
// Implementations:
//   - StoreAdapter (wraps the existing Store interface)
//   - WebhookSource (Prometheus Alertmanager webhook receiver)
//   - Wave9Source (eventbus-driven Wave9 observability events)
type AlertSource interface {
	// Name returns a human-readable source identifier (e.g. "store", "webhook", "wave9").
	Name() string

	// ListActive returns all non-resolved alerts from this source.
	// Errors are non-fatal: MultiScanner logs them and continues with
	// the remaining sources (best-effort aggregation).
	ListActive(ctx context.Context) ([]domain.AlertRecord, error)
}

// StoreAdapter wraps a Store (the existing alert persistence contract)
// so it satisfies the AlertSource interface.
type StoreAdapter struct {
	store Store
	name  string
}

// NewStoreAdapter creates an AlertSource backed by a Store.
func NewStoreAdapter(store Store) *StoreAdapter {
	return &StoreAdapter{store: store, name: "store"}
}

func (a *StoreAdapter) Name() string { return a.name }

func (a *StoreAdapter) ListActive(ctx context.Context) ([]domain.AlertRecord, error) {
	// Reuse existing Scanner logic for filtering active alerts.
	s := New(a.store)
	return s.ScanActive(ctx)
}

// MultiScanner aggregates alerts from multiple AlertSource implementations.
// It is designed to coexist with the existing single-source Scanner:
// consumers that already depend on Scanner continue to work unchanged;
// new code opts in to MultiScanner for cross-source aggregation.
type MultiScanner struct {
	sources []AlertSource
}

// NewMultiScanner creates a scanner backed by the given alert sources.
// At least one source should be provided; an empty scanner returns
// empty results for all methods (no-op, never panics).
func NewMultiScanner(sources ...AlertSource) *MultiScanner {
	return &MultiScanner{sources: sources}
}

// Sources returns a copy of the registered alert sources.
func (m *MultiScanner) Sources() []AlertSource {
	out := make([]AlertSource, len(m.sources))
	copy(out, m.sources)
	return out
}

// Scan runs ListActive on every source, merges results, and returns
// unacknowledged alerts sorted by timestamp descending.
func (m *MultiScanner) Scan(ctx context.Context) ([]domain.AlertRecord, error) {
	all, err := m.ScanActive(ctx)
	if err != nil {
		return nil, err
	}
	var unacked []domain.AlertRecord
	for _, a := range all {
		if !a.Acknowledged {
			unacked = append(unacked, a)
		}
	}
	return unacked, nil
}

// ScanActive collects active (non-resolved) alerts from all sources.
// Source errors are collected; the method returns a combined error only
// when ALL sources fail. Partial success returns the available alerts.
func (m *MultiScanner) ScanActive(ctx context.Context) ([]domain.AlertRecord, error) {
	var all []domain.AlertRecord
	var errs []error
	for _, src := range m.sources {
		records, err := src.ListActive(ctx)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		all = append(all, records...)
	}
	sortByTimestampDesc(all)
	all = dedupByID(all)

	if len(errs) > 0 && len(all) == 0 {
		return nil, errs[0] // all sources failed
	}
	return all, nil
}

// CountBySeverity groups unacknowledged alerts by severity.
func (m *MultiScanner) CountBySeverity(ctx context.Context) (map[string]int, error) {
	alerts, err := m.Scan(ctx)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{"critical": 0, "error": 0, "warning": 0, "info": 0}
	for _, a := range alerts {
		counts[a.Severity]++
	}
	return counts, nil
}

// HasBlockers returns true when there are unacknowledged alerts at
// CRITICAL or ERROR severity.
func (m *MultiScanner) HasBlockers(ctx context.Context) (bool, error) {
	alerts, err := m.Scan(ctx)
	if err != nil {
		return false, err
	}
	for _, a := range alerts {
		if a.Severity == "critical" || a.Severity == "error" {
			return true, nil
		}
	}
	return false, nil
}

// Snapshot runs Scan + CountBySeverity + HasBlockers and returns a
// self-contained snapshot suitable for MCP tool output or agent context.
func (m *MultiScanner) Snapshot(ctx context.Context) (*Snapshot, error) {
	alerts, err := m.Scan(ctx)
	if err != nil {
		return nil, err
	}
	counts, err := m.CountBySeverity(ctx)
	if err != nil {
		return nil, err
	}
	blocked, err := m.HasBlockers(ctx)
	if err != nil {
		return nil, err
	}
	return &Snapshot{
		Total:      len(alerts),
		Blocked:    blocked,
		BySeverity: counts,
		Alerts:     alerts,
	}, nil
}

// dedupByID removes duplicate alerts (same ID) across sources,
// keeping the first occurrence. The input must be pre-sorted.
func dedupByID(records []domain.AlertRecord) []domain.AlertRecord {
	seen := make(map[string]bool, len(records))
	out := records[:0]
	for _, r := range records {
		if r.ID == "" || seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		out = append(out, r)
	}
	return out
}
