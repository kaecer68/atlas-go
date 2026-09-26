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
	RiskChecked             *CounterVec
	RiskErrors              *CounterVec
	NarrativeEventsScraped  *CounterVec
	NarrativeErrors         *CounterVec
	SnapshotPersisted       *CounterVec
	PipelineDurationSeconds *CounterVec
	CoverageMapped          *CounterVec
	CoverageTotal           *CounterVec
	onInc                   OnInc
}

// SetOnInc installs a callback that is invoked on every counter increment.
// The callback receives the atlas_universe_* series name, the counter's real
// label name/value pairs and the per-event DELTA (see OnInc).
// Calling SetOnInc multiple times replaces the previous callback.
func (m *UniverseMetrics) SetOnInc(fn OnInc) {
	m.onInc = fn
	wireOnInc := func(cv *CounterVec, counterName string) {
		if cv == nil {
			return
		}
		cv.OnInc = func(_ string, labels map[string]string, value float64) {
			if m.onInc != nil {
				m.onInc(counterName, labels, value)
			}
		}
	}
	wireOnInc(m.SymbolsGathered, "atlas_universe_symbols_gathered_total")
	wireOnInc(m.SymbolsFiltered, "atlas_universe_symbols_filtered_total")
	wireOnInc(m.QuotesFetched, "atlas_universe_quotes_fetched_total")
	wireOnInc(m.QuotesErrors, "atlas_universe_quotes_errors_total")
	wireOnInc(m.SymbolsScreened, "atlas_universe_symbols_screened_total")
	wireOnInc(m.SymbolsRanked, "atlas_universe_symbols_ranked_total")
	wireOnInc(m.RiskChecked, "atlas_universe_risk_checked_total")
	wireOnInc(m.RiskErrors, "atlas_universe_risk_errors_total")
	wireOnInc(m.NarrativeEventsScraped, "atlas_universe_narrative_events_scraped_total")
	wireOnInc(m.NarrativeErrors, "atlas_universe_narrative_errors_total")
	wireOnInc(m.SnapshotPersisted, "atlas_universe_snapshot_persisted_total")
	wireOnInc(m.PipelineDurationSeconds, "atlas_universe_pipeline_duration_seconds")
	wireOnInc(m.CoverageMapped, "atlas_universe_coverage_mapped_total")
	wireOnInc(m.CoverageTotal, "atlas_universe_coverage_total")
}

// Universe stage label values. They are the first label of every series in
// this family and are part of the exposed contract: callers (WarmUp, the
// scheduler, the coverage check) must not invent new spellings.
const (
	UniverseStageDaily         = "daily"
	UniverseStageWeekly        = "weekly"
	UniverseStageCoverageCheck = "coverage_check"
)

// universeSeriesSpec is one labeled series of the atlas_universe_* family.
type universeSeriesSpec struct {
	counter *CounterVec
	values  []string
}

// warmUpSeries returns every series the universe pipeline can produce: both
// scheduled stages (daily/weekly, see internal/monitoring/universe_scheduler.go)
// plus the coverage check task (cmd/atlas/main.go). The second label of the
// two-label vectors enumerates the buckets the pipeline adds to, including the
// error buckets that only appear on a bad run — a series that can exist must be
// listed here, otherwise it stays invisible after a restart (see WarmUp).
func (m *UniverseMetrics) warmUpSeries() []universeSeriesSpec {
	specs := make([]universeSeriesSpec, 0, 64)
	for _, stage := range []string{UniverseStageDaily, UniverseStageWeekly} {
		specs = append(specs,
			universeSeriesSpec{m.SymbolsGathered, []string{stage}},
			universeSeriesSpec{m.SymbolsFiltered, []string{stage, "industry_filter"}},
			universeSeriesSpec{m.SymbolsFiltered, []string{stage, "dropped"}},
			universeSeriesSpec{m.QuotesFetched, []string{stage}},
			universeSeriesSpec{m.QuotesErrors, []string{stage, "provider_unavailable"}},
			universeSeriesSpec{m.QuotesErrors, []string{stage, "fetch_error"}},
			universeSeriesSpec{m.QuotesErrors, []string{stage, "chunk_error"}},
			universeSeriesSpec{m.QuotesErrors, []string{stage, "unresolved"}},
			universeSeriesSpec{m.SymbolsScreened, []string{stage, "passed"}},
			universeSeriesSpec{m.SymbolsScreened, []string{stage, "failed"}},
			universeSeriesSpec{m.SymbolsRanked, []string{stage}},
			universeSeriesSpec{m.RiskChecked, []string{stage, "passed"}},
			universeSeriesSpec{m.RiskChecked, []string{stage, "excluded"}},
			universeSeriesSpec{m.RiskErrors, []string{stage, "filter_error"}},
			universeSeriesSpec{m.NarrativeEventsScraped, []string{stage}},
			universeSeriesSpec{m.NarrativeErrors, []string{stage, "scrape_error"}},
			universeSeriesSpec{m.NarrativeErrors, []string{stage, "cache_save_error"}},
			universeSeriesSpec{m.SnapshotPersisted, []string{stage}},
			universeSeriesSpec{m.PipelineDurationSeconds, []string{stage}},
			universeSeriesSpec{m.CoverageMapped, []string{stage, "all"}},
			universeSeriesSpec{m.CoverageTotal, []string{stage, "all"}},
		)
	}
	specs = append(specs,
		universeSeriesSpec{m.CoverageMapped, []string{UniverseStageCoverageCheck, "all"}},
		universeSeriesSpec{m.CoverageTotal, []string{UniverseStageCoverageCheck, "all"}},
	)
	return specs
}

// WarmUp materializes every series of this family with value 0, so /metrics
// exposes atlas_universe_* from process start instead of only after the first
// pipeline run (issue #1995).
//
// Why it is needed: a series appears on /metrics only when its counter is first
// incremented. CounterVec.WithLabelValues just builds the in-memory counter;
// the series is created by the OnInc callback reaching the MetricsCollector
// that PrometheusHandler scrapes (see CollectorOnInc). The collector is
// in-memory (bootstrap.InitMetrics -> NewMetricsCollector, no persistence) and
// the pipeline runs at most once per trading day (06:00 UTC, Tue-Fri; the
// weekly rebuild covers Monday). Production measurement 2026-09-25: the
// container restarted at 07:14Z and the whole family stayed absent from
// /metrics for the following ~71h, until the next scheduled run. During that
// window `grep -c '^atlas_universe_' /metrics` is 0 on a healthy system, which
// makes "the family is missing" mean "a restart happened", not "the metric was
// renamed or the pipeline stopped".
//
// Add(0) is deliberate: it reports a zero DELTA to OnInc (see Counter.Add), so
// the series materializes in the sink without fabricating an increment.
//
// Call it *after* SetOnInc; before SetOnInc the zero deltas are not forwarded
// anywhere and no series is created.
func (m *UniverseMetrics) WarmUp() {
	if m == nil {
		return
	}
	for _, s := range m.warmUpSeries() {
		s.counter.WithLabelValues(s.values...).Add(0)
	}
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
	RiskChecked             []Sample
	RiskErrors              []Sample
	NarrativeEventsScraped  []Sample
	NarrativeErrors         []Sample
	SnapshotPersisted       []Sample
	PipelineDurationSeconds []Sample
	CoverageMapped          []Sample
	CoverageTotal           []Sample
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
		RiskChecked:             m.RiskChecked.snapshotSamplesAt(now),
		RiskErrors:              m.RiskErrors.snapshotSamplesAt(now),
		NarrativeEventsScraped:  m.NarrativeEventsScraped.snapshotSamplesAt(now),
		NarrativeErrors:         m.NarrativeErrors.snapshotSamplesAt(now),
		SnapshotPersisted:       m.SnapshotPersisted.snapshotSamplesAt(now),
		PipelineDurationSeconds: m.PipelineDurationSeconds.snapshotSamplesAt(now),
		CoverageMapped:          m.CoverageMapped.snapshotSamplesAt(now),
		CoverageTotal:           m.CoverageTotal.snapshotSamplesAt(now),
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
	}
}
