package metrics

import (
	"time"
)

// ── atlas_universe_last_run_* : the pipeline's own verdict ────────────────
//
// Why this family exists (2026-09-27, incident universe-scoring-gap-20260925)
//
// The atlas_universe_*_total counters describe *how much work the pipeline
// recorded*, not *what it produced*. Alerting that reads only counter
// increments can therefore not tell three worlds apart:
//
//	(i)   the pipeline did not run at all (holiday, broken schedule);
//	(ii)  it ran but produced nothing usable (empty universe, dead quote
//	      provider, thresholds that reject everything);
//	(iii) it ran and produced a healthy snapshot, but a counter never made it
//	      into the sink (this is exactly what production showed on
//	      2026-09-25: symbols_ranked_total and quotes_fetched_total stayed at
//	      zero for four days while the snapshot held ranked=150,
//	      quotes_returned=1581).
//
// In world (iii) every counter-increment rule fires on a healthy pipeline. The
// fix is to publish the run's verdict as an OUTPUT measurement, once per run,
// on a path that does not share the counter wiring: ReportRun is called from a
// single defer in BuildUniverse, so no early return or error path can skip it,
// and the gauges reach the collector through the sink below (RecordGauge),
// never through CounterVec/OnInc.
//
// Semantics that consumers rely on:
//
//   - last_run_finished_timestamp_seconds / last_run_valid are warmed at
//     process start: the timestamp is the process start instant and valid is 0.
//     So "valid == 0" means "no run has completed since this process started",
//     and it is never a claim about the pipeline's output.
//   - Every other gauge describes the LAST COMPLETED RUN of that stage, and may
//     be arbitrarily old (a weekly verdict stays valid for six days). Alerts
//     must pair a verdict gauge with a freshness or a "did it run" rule; the
//     verdict itself never expires on its own.
//   - last_run_outcome is one-hot: ReportRun zeroes the outcome value it
//     published for that stage last time before publishing the new one, so
//     "the current outcome" is always the series with value 1. (The sink is an
//     append-only map: a series is never removed, only overwritten.)
const (
	// UniverseMetricLastRunValid is 0 until a run of that stage completes, then 1.
	UniverseMetricLastRunValid = "atlas_universe_last_run_valid"
	// UniverseMetricLastRunFinished is the unix time of the last completed run
	// (warmed to the process start instant).
	UniverseMetricLastRunFinished = "atlas_universe_last_run_finished_timestamp_seconds"
	// UniverseMetricLastRunGathered is symbols_gathered of the last run.
	UniverseMetricLastRunGathered = "atlas_universe_last_run_symbols_gathered"
	// UniverseMetricLastRunFiltered is symbols_filtered of the last run.
	UniverseMetricLastRunFiltered = "atlas_universe_last_run_symbols_filtered"
	// UniverseMetricLastRunScreenedPassed is the number of symbols the last run
	// let through screening (the `passed` bucket of symbols_screened_total).
	UniverseMetricLastRunScreenedPassed = "atlas_universe_last_run_screened_passed"
	// UniverseMetricLastRunScreenedFailed is the `failed` bucket of the last run.
	UniverseMetricLastRunScreenedFailed = "atlas_universe_last_run_screened_failed"
	// UniverseMetricLastRunRanked is the size of the ranked list of the last run.
	UniverseMetricLastRunRanked = "atlas_universe_last_run_symbols_ranked"
	// UniverseMetricLastRunTrustworthy is 1 when the last run's ranked list is a
	// market verdict, 0 when it is not (see RankedFallbackReason).
	UniverseMetricLastRunTrustworthy = "atlas_universe_last_run_ranked_trustworthy"
	// UniverseMetricLastRunQuotesRequested / Returned describe the quote input of
	// the last run.
	UniverseMetricLastRunQuotesRequested = "atlas_universe_last_run_quotes_requested"
	UniverseMetricLastRunQuotesReturned  = "atlas_universe_last_run_quotes_returned"
	// UniverseMetricLastRunOutcome is a one-hot gauge (labels stage, outcome)
	// naming *why* the last run's ranking is or is not a market verdict. The
	// vocabulary is owned by the caller (internal/monitoring's RunOutcome*
	// constants); this package treats it as an opaque string.
	UniverseMetricLastRunOutcome = "atlas_universe_last_run_outcome"
	// UniverseMetricNextRun is the instant (unix time) at which the scheduler is
	// next expected to run the pipeline, computed from the Taiwan trading
	// calendar. It is emitted at process start and after every run, so a missed
	// trigger is visible without any holiday-blind window arithmetic.
	UniverseMetricNextRun = "atlas_universe_next_run_timestamp_seconds"
	// UniverseStageLabel is the label every gauge of this family carries, except
	// UniverseMetricNextRun (whose subject is the schedule, not a stage).
	UniverseStageLabel = "stage"
)

// UniverseRunVerdict is the machine-readable outcome of one completed
// BuildUniverse run. It mirrors the fields the snapshot already persists
// (UniverseBuildResult) so the alerting layer can read the same verdict
// without filesystem access.
type UniverseRunVerdict struct {
	// Stage is "daily" or "weekly" (see the UniverseStage* constants).
	Stage string
	// Outcome names the verdict bucket; one of the caller's RunOutcome* values.
	Outcome string
	// Gathered / Filtered / ScreenedPassed / ScreenedFailed / Ranked are the
	// counts the run reached at each stage.
	Gathered       int
	Filtered       int
	ScreenedPassed int
	ScreenedFailed int
	Ranked         int
	// QuotesRequested / QuotesReturned are the quote-fetch counts.
	QuotesRequested int
	QuotesReturned  int
	// Trustworthy is true when Ranked reflects real quote input.
	Trustworthy bool
	// FinishedAt is when the run finished; zero means "now".
	FinishedAt time.Time
}

// UniverseRunVerdictSink receives one gauge sample: the metric name, its
// value, and its labels (excluding the stage label, added by ReportRun).
type UniverseRunVerdictSink func(name string, value float64, labels map[string]string)

// SetRunVerdictSink installs the sink that publishes run verdicts. A nil sink
// (or nil receiver) makes ReportRun a no-op. Calling it again replaces the sink.
func (m *UniverseMetrics) SetRunVerdictSink(sink UniverseRunVerdictSink) {
	if m == nil {
		return
	}
	m.runVerdictSink = sink
}

// reportGauge forwards one gauge sample to the sink if one is installed.
func (m *UniverseMetrics) reportGauge(name string, value float64, labels map[string]string) {
	if m == nil || m.runVerdictSink == nil {
		return
	}
	m.runVerdictSink(name, value, labels)
}

// ReportRun publishes the verdict of one completed run. Every gauge it writes
// carries the stage label; the previous outcome value of that stage is zeroed
// first so the one-hot metric stays one-hot.
func (m *UniverseMetrics) ReportRun(v UniverseRunVerdict) {
	if m == nil || m.runVerdictSink == nil {
		return
	}
	finished := v.FinishedAt
	if finished.IsZero() {
		finished = time.Now()
	}
	stage := map[string]string{UniverseStageLabel: v.Stage}

	m.reportGauge(UniverseMetricLastRunValid, 1, stage)
	m.reportGauge(UniverseMetricLastRunFinished, float64(finished.Unix()), stage)
	m.reportGauge(UniverseMetricLastRunGathered, float64(v.Gathered), stage)
	m.reportGauge(UniverseMetricLastRunFiltered, float64(v.Filtered), stage)
	m.reportGauge(UniverseMetricLastRunScreenedPassed, float64(v.ScreenedPassed), stage)
	m.reportGauge(UniverseMetricLastRunScreenedFailed, float64(v.ScreenedFailed), stage)
	m.reportGauge(UniverseMetricLastRunRanked, float64(v.Ranked), stage)
	m.reportGauge(UniverseMetricLastRunQuotesRequested, float64(v.QuotesRequested), stage)
	m.reportGauge(UniverseMetricLastRunQuotesReturned, float64(v.QuotesReturned), stage)
	m.reportGauge(UniverseMetricLastRunTrustworthy, boolGauge(v.Trustworthy), stage)

	if prev, ok := m.lastOutcome[v.Stage]; ok && prev != "" && prev != v.Outcome {
		m.reportGauge(UniverseMetricLastRunOutcome, 0, map[string]string{
			UniverseStageLabel: v.Stage,
			"outcome":          prev,
		})
	}
	if v.Outcome != "" {
		if m.lastOutcome == nil {
			m.lastOutcome = map[string]string{}
		}
		m.lastOutcome[v.Stage] = v.Outcome
		m.reportGauge(UniverseMetricLastRunOutcome, 1, map[string]string{
			UniverseStageLabel: v.Stage,
			"outcome":          v.Outcome,
		})
	}
}

// ReportNextRun publishes the instant the scheduler is next expected to run the
// pipeline. A zero instant (unknown, e.g. an unreadable holiday calendar) is
// dropped: it is better to leave the heartbeat absent than to publish 1970.
func (m *UniverseMetrics) ReportNextRun(at time.Time) {
	if m == nil || m.runVerdictSink == nil || at.IsZero() {
		return
	}
	m.reportGauge(UniverseMetricNextRun, float64(at.Unix()), nil)
}

// warmUpRunVerdicts materializes the run-verdict gauges so /metrics shows the
// family before the first run of this process. finished is set to now (the
// process start instant, the best available "no run known since" marker) and
// valid to 0, so a consumer can never read a warmed gauge as a run verdict.
func (m *UniverseMetrics) warmUpRunVerdicts(now time.Time) {
	if m == nil || m.runVerdictSink == nil {
		return
	}
	for _, stage := range []string{UniverseStageDaily, UniverseStageWeekly} {
		labels := map[string]string{UniverseStageLabel: stage}
		m.reportGauge(UniverseMetricLastRunValid, 0, labels)
		m.reportGauge(UniverseMetricLastRunFinished, float64(now.Unix()), labels)
		m.reportGauge(UniverseMetricLastRunGathered, 0, labels)
		m.reportGauge(UniverseMetricLastRunFiltered, 0, labels)
		m.reportGauge(UniverseMetricLastRunScreenedPassed, 0, labels)
		m.reportGauge(UniverseMetricLastRunScreenedFailed, 0, labels)
		m.reportGauge(UniverseMetricLastRunRanked, 0, labels)
		m.reportGauge(UniverseMetricLastRunQuotesRequested, 0, labels)
		m.reportGauge(UniverseMetricLastRunQuotesReturned, 0, labels)
		m.reportGauge(UniverseMetricLastRunTrustworthy, 0, labels)
	}
}

func boolGauge(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
