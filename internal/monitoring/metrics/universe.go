package metrics

import (
	"time"
)

// UniverseMetrics exposes Prometheus-style counters for the universe pipeline.
// It mirrors CounterVec semantics using the in-memory counter store defined
// in degraded.go so it can be used from any package without import cycles.
type UniverseMetrics struct {
	SymbolsGathered         *CounterVec
	SymbolsFiltered         *CounterVec
	QuotesFetched           *CounterVec
	QuotesErrors            *CounterVec
	SymbolsScreened         *CounterVec
	SymbolsRanked           *CounterVec
	ScoringErrors           *CounterVec
	RiskChecked             *CounterVec
	RiskErrors              *CounterVec
	NarrativeEventsScraped  *CounterVec
	NarrativeErrors         *CounterVec
	SnapshotPersisted       *CounterVec
	SnapshotSizeBytes       *CounterVec
	PipelineDurationSeconds *CounterVec
	CoverageMapped          *CounterVec
	CoverageTotal           *CounterVec
	D6ExpiredSymbols        *CounterVec
	onInc                   OnInc
}

// SetOnInc installs a callback that is invoked on every counter increment.
// Calling SetOnInc multiple times replaces the previous callback.
func (m *UniverseMetrics) SetOnInc(fn OnInc) {
	m.onInc = fn
	wireOnInc := func(cv *CounterVec, counterName string) {
		if cv == nil {
			return
		}
		cv.OnInc = func(_ string, labels map[string]string, value float64) {
			if m.onInc != nil {
				m.onInc(counterName, orderedLabelValues(labels, cv.labelNames), value)
			}
		}
	}
	wireOnInc(m.SymbolsGathered, "atlas_universe_symbols_gathered_total")
	wireOnInc(m.SymbolsFiltered, "atlas_universe_symbols_filtered_total")
	wireOnInc(m.QuotesFetched, "atlas_universe_quotes_fetched_total")
	wireOnInc(m.QuotesErrors, "atlas_universe_quotes_errors_total")
	wireOnInc(m.SymbolsScreened, "atlas_universe_symbols_screened_total")
	wireOnInc(m.SymbolsRanked, "atlas_universe_symbols_ranked_total")
	wireOnInc(m.ScoringErrors, "atlas_universe_scoring_errors_total")
	wireOnInc(m.RiskChecked, "atlas_universe_risk_checked_total")
	wireOnInc(m.RiskErrors, "atlas_universe_risk_errors_total")
	wireOnInc(m.NarrativeEventsScraped, "atlas_universe_narrative_events_scraped_total")
	wireOnInc(m.NarrativeErrors, "atlas_universe_narrative_errors_total")
	wireOnInc(m.SnapshotPersisted, "atlas_universe_snapshot_persisted_total")
	wireOnInc(m.SnapshotSizeBytes, "atlas_universe_snapshot_size_bytes")
	wireOnInc(m.PipelineDurationSeconds, "atlas_universe_pipeline_duration_seconds")
	wireOnInc(m.CoverageMapped, "atlas_universe_coverage_mapped_total")
	wireOnInc(m.CoverageTotal, "atlas_universe_coverage_total")
	wireOnInc(m.D6ExpiredSymbols, "atlas_universe_d6_expired_symbols_total")
}

// UniverseSnapshot is a point-in-time view of all universe pipeline counters.
type UniverseSnapshot struct {
	Timestamp               time.Time
	SymbolsGathered         []Sample
	SymbolsFiltered         []Sample
	QuotesFetched           []Sample
	QuotesErrors            []Sample
	SymbolsScreened         []Sample
	SymbolsRanked           []Sample
	ScoringErrors           []Sample
	RiskChecked             []Sample
	RiskErrors              []Sample
	NarrativeEventsScraped  []Sample
	NarrativeErrors         []Sample
	SnapshotPersisted       []Sample
	SnapshotSizeBytes       []Sample
	PipelineDurationSeconds []Sample
	CoverageMapped          []Sample
	CoverageTotal           []Sample
	D6ExpiredSymbols        []Sample
}

// Snapshot returns the current values of all universe pipeline counters,
// stamped with the wall-clock time at which the snapshot was captured.
func (m *UniverseMetrics) Snapshot() UniverseSnapshot {
	now := time.Now()
	return UniverseSnapshot{
		Timestamp:               now,
		SymbolsGathered:         m.SymbolsGathered.snapshotSamplesAt(now),
		SymbolsFiltered:         m.SymbolsFiltered.snapshotSamplesAt(now),
		QuotesFetched:           m.QuotesFetched.snapshotSamplesAt(now),
		QuotesErrors:            m.QuotesErrors.snapshotSamplesAt(now),
		SymbolsScreened:         m.SymbolsScreened.snapshotSamplesAt(now),
		SymbolsRanked:           m.SymbolsRanked.snapshotSamplesAt(now),
		ScoringErrors:           m.ScoringErrors.snapshotSamplesAt(now),
		RiskChecked:             m.RiskChecked.snapshotSamplesAt(now),
		RiskErrors:              m.RiskErrors.snapshotSamplesAt(now),
		NarrativeEventsScraped:  m.NarrativeEventsScraped.snapshotSamplesAt(now),
		NarrativeErrors:         m.NarrativeErrors.snapshotSamplesAt(now),
		SnapshotPersisted:       m.SnapshotPersisted.snapshotSamplesAt(now),
		SnapshotSizeBytes:       m.SnapshotSizeBytes.snapshotSamplesAt(now),
		PipelineDurationSeconds: m.PipelineDurationSeconds.snapshotSamplesAt(now),
		CoverageMapped:          m.CoverageMapped.snapshotSamplesAt(now),
		CoverageTotal:           m.CoverageTotal.snapshotSamplesAt(now),
		D6ExpiredSymbols:        m.D6ExpiredSymbols.snapshotSamplesAt(now),
	}
}

// NewUniverseMetrics creates a new UniverseMetrics instance backed by an
// in-memory counter store.
func NewUniverseMetrics() *UniverseMetrics {
	return &UniverseMetrics{
		SymbolsGathered: &CounterVec{
			name:       "atlas_universe_symbols_gathered_total",
			labelNames: []string{"stage"},
		},
		SymbolsFiltered: &CounterVec{
			name:       "atlas_universe_symbols_filtered_total",
			labelNames: []string{"stage", "reason"},
		},
		QuotesFetched: &CounterVec{
			name:       "atlas_universe_quotes_fetched_total",
			labelNames: []string{"stage"},
		},
		QuotesErrors: &CounterVec{
			name:       "atlas_universe_quotes_errors_total",
			labelNames: []string{"stage", "error_type"},
		},
		SymbolsScreened: &CounterVec{
			name:       "atlas_universe_symbols_screened_total",
			labelNames: []string{"stage", "result"},
		},
		SymbolsRanked: &CounterVec{
			name:       "atlas_universe_symbols_ranked_total",
			labelNames: []string{"stage"},
		},
		ScoringErrors: &CounterVec{
			name:       "atlas_universe_scoring_errors_total",
			labelNames: []string{"stage", "error_type"},
		},
		RiskChecked: &CounterVec{
			name:       "atlas_universe_risk_checked_total",
			labelNames: []string{"stage", "result"},
		},
		RiskErrors: &CounterVec{
			name:       "atlas_universe_risk_errors_total",
			labelNames: []string{"stage", "error_type"},
		},
		NarrativeEventsScraped: &CounterVec{
			name:       "atlas_universe_narrative_events_scraped_total",
			labelNames: []string{"stage"},
		},
		NarrativeErrors: &CounterVec{
			name:       "atlas_universe_narrative_errors_total",
			labelNames: []string{"stage", "error_type"},
		},
		SnapshotPersisted: &CounterVec{
			name:       "atlas_universe_snapshot_persisted_total",
			labelNames: []string{"stage"},
		},
		SnapshotSizeBytes: &CounterVec{
			name:       "atlas_universe_snapshot_size_bytes",
			labelNames: []string{"stage"},
		},
		PipelineDurationSeconds: &CounterVec{
			name:       "atlas_universe_pipeline_duration_seconds",
			labelNames: []string{"stage"},
		},
		CoverageMapped: &CounterVec{
			name:       "atlas_universe_coverage_mapped_total",
			labelNames: []string{"stage", "industry"},
		},
		CoverageTotal: &CounterVec{
			name:       "atlas_universe_coverage_total",
			labelNames: []string{"stage", "industry"},
		},
		D6ExpiredSymbols: &CounterVec{
			name:       "atlas_universe_d6_expired_symbols_total",
			labelNames: []string{"stage"},
		},
	}
}
