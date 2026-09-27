package monitoring

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/metrics"
)

// This file turns one BuildUniverse run into the atlas_universe_last_run_*
// verdict family (see internal/monitoring/metrics/universe_run.go) and computes
// the schedule heartbeat. Both exist so the alert rules can reason from the
// pipeline's OUTPUT instead of from counter increments — the distinction that
// the 2026-09-25 incident (healthy pipeline, two counters never emitted) made
// necessary.

// RunOutcome* values are the vocabulary of the atlas_universe_last_run_outcome
// gauge. They partition the verdict space so that each alert owns exactly one
// bucket and one root cause can never page twice:
//
//	ok               the ranked list is a market verdict (trustworthy)
//	universe_empty   Step 1 gathered nothing
//	filtered_empty   Step 2 (industry filter) removed every gathered symbol
//	quotes_*         the quote input was unusable, one value per reason
//	other            a fallback reason this mapping does not know
//
// ⚠️ Adding a RankedFallbackReason constant REQUIRES adding a mapping here and
// a matching mentions entry in monitoring/rules/atlas_universe_scoring_alerts.yml
// (the triage text lists the vocabulary). An unmapped reason lands in `other`,
// which is visible on /metrics and still pages through AtlasUniverseQuotesMissing
// (its evidence is "not trustworthy", not this label), but it loses the
// specific triage step.
const (
	RunOutcomeOK                = "ok"
	RunOutcomeUniverseEmpty     = "universe_empty"
	RunOutcomeFilteredEmpty     = "filtered_empty"
	RunOutcomeQuotesUnavailable = "quotes_unavailable"
	RunOutcomeQuotesFetchError  = "quotes_fetch_error"
	RunOutcomeQuotesEmpty       = "quotes_empty"
	RunOutcomeQuotesPartial     = "quotes_partial"
	RunOutcomeQuotesMock        = "quotes_mock"
	RunOutcomeOther             = "other"
)

// RunOutcome maps a run's RankedFallbackReason to the outcome vocabulary above.
// The empty reason means the run produced a trustworthy ranking.
func RunOutcome(reason string) string {
	switch reason {
	case "":
		return RunOutcomeOK
	case RankedFallbackEmptyUniverse:
		return RunOutcomeUniverseEmpty
	case RankedFallbackEmptyFiltered:
		return RunOutcomeFilteredEmpty
	case RankedFallbackQuoteProviderUnavailable:
		return RunOutcomeQuotesUnavailable
	case RankedFallbackQuoteFetchError:
		return RunOutcomeQuotesFetchError
	case RankedFallbackQuoteFetchEmpty:
		return RunOutcomeQuotesEmpty
	case RankedFallbackQuoteFetchPartial:
		return RunOutcomeQuotesPartial
	case RankedFallbackQuoteProviderMock:
		return RunOutcomeQuotesMock
	default:
		return RunOutcomeOther
	}
}

// UniverseRunVerdictFor projects a finished run onto the metrics verdict.
//
// It is a pure function of the result so it can be unit-tested without running
// the pipeline, and so every exit path of BuildUniverse publishes the same
// shape (the pipeline publishes it from a single defer).
func UniverseRunVerdictFor(result *UniverseBuildResult, stage string, finishedAt time.Time) metrics.UniverseRunVerdict {
	if result == nil {
		return metrics.UniverseRunVerdict{Stage: stage, Outcome: RunOutcomeOther, FinishedAt: finishedAt}
	}
	passed := result.SymbolsRanked
	failed := result.SymbolsFiltered - result.SymbolsRanked
	if failed < 0 {
		failed = 0
	}
	return metrics.UniverseRunVerdict{
		Stage:           stage,
		Outcome:         RunOutcome(result.RankedFallbackReason),
		Gathered:        result.SymbolsBuilt,
		Filtered:        result.SymbolsFiltered,
		ScreenedPassed:  passed,
		ScreenedFailed:  failed,
		Ranked:          result.SymbolsRanked,
		QuotesRequested: result.QuotesRequested,
		QuotesReturned:  result.QuotesReturned,
		Trustworthy:     result.RankedTrustworthy,
		FinishedAt:      finishedAt,
	}
}

// ReportUniverseHeartbeat publishes the instant the pipeline is next expected to
// run. It is called at process start (so a scheduler that never fires is
// visible immediately) and after every run (see BuildUniverse).
//
// A run is expected on every Taiwan trading day at 14:00 Asia/Taipei (06:00
// UTC): Monday is the weekly full rebuild, Tuesday–Friday the daily refresh,
// and both skip non-trading days (internal/taiwanholidays via marketdata).
func ReportUniverseHeartbeat(um *metrics.UniverseMetrics, now time.Time) {
	if um == nil {
		return
	}
	um.ReportNextRun(NextUniverseRun(now))
}

// universeNextRunScanDays bounds the search for the next trading day. Two weeks
// is far beyond the longest Taiwan market closure (Lunar New Year, ~9 days);
// hitting the bound means the calendar is unreadable, and then no heartbeat is
// published rather than a wrong one (the alert stays silent instead of firing
// forever — AtlasUniverseRunOverdue documents that trade-off).
const universeNextRunScanDays = 14

// NextUniverseRun returns the next instant at which the universe pipeline is
// scheduled to run, or the zero time when no trading day is found within
// universeNextRunScanDays.
//
// It answers the question the alert rules need — "has the scheduler missed its
// own next trigger?" — from the Taiwan trading calendar, so a holiday week can
// never produce a false "the pipeline stopped" page (the failure mode of the
// window arithmetic the counter-based rules used until 2026-09-27).
func NextUniverseRun(now time.Time) time.Time {
	loc := universeLocation()
	local := now.In(loc)
	target := time.Date(local.Year(), local.Month(), local.Day(), universeTriggerHourTW, 0, 0, 0, loc)
	for day := 0; day <= universeNextRunScanDays; day++ {
		candidate := target.AddDate(0, 0, day)
		if candidate.Before(now) {
			continue
		}
		if marketdata.IsTaiwanTradingDay(candidate) {
			return candidate
		}
	}
	return time.Time{}
}

// PreviousUniverseRun returns the last instant at or before now at which the
// universe pipeline was scheduled to run, or the zero time when no trading day is
// found within universeNextRunScanDays.
//
// It is the exact mirror of NextUniverseRun, and it exists because the freshness
// of an artifact can only be judged against a calendar fact, never against a
// fixed window: "the snapshot is older than the last session that should have
// written it" is answerable across a holiday closure (2026-09-25 中秋 + 09-28
// 教師節 leave a five-day gap in which no run is expected at all), while "the
// snapshot is older than 24h" fires on every closure. The in-process coverage
// check (see universe_coverage_check.go) is the caller.
//
// The zero return means "unknown": a calendar that cannot be read must not be
// turned into an alert, the same rule ReportNextRun follows when it drops a zero
// heartbeat instead of publishing 1970.
func PreviousUniverseRun(now time.Time) time.Time {
	loc := universeLocation()
	local := now.In(loc)
	target := time.Date(local.Year(), local.Month(), local.Day(), universeTriggerHourTW, 0, 0, 0, loc)
	for day := 0; day <= universeNextRunScanDays; day++ {
		candidate := target.AddDate(0, 0, -day)
		if candidate.After(now) {
			continue
		}
		if marketdata.IsTaiwanTradingDay(candidate) {
			return candidate
		}
	}
	return time.Time{}
}
