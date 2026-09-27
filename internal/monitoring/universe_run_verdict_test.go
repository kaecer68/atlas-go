package monitoring

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/monitoring/metrics"
)

// TestRunOutcome_MapsEveryKnownReason pins the verdict vocabulary. A new
// RankedFallbackReason without a mapping lands in RunOutcomeOther, which is
// visible on /metrics but loses the specific triage step — this test is the
// reminder, the constant block in universe_run_verdict.go is the contract.
func TestRunOutcome_MapsEveryKnownReason(t *testing.T) {
	for _, tc := range []struct {
		reason string
		want   string
	}{
		{"", RunOutcomeOK},
		{RankedFallbackEmptyUniverse, RunOutcomeUniverseEmpty},
		{RankedFallbackEmptyFiltered, RunOutcomeFilteredEmpty},
		{RankedFallbackQuoteProviderUnavailable, RunOutcomeQuotesUnavailable},
		{RankedFallbackQuoteFetchError, RunOutcomeQuotesFetchError},
		{RankedFallbackQuoteFetchEmpty, RunOutcomeQuotesEmpty},
		{RankedFallbackQuoteFetchPartial, RunOutcomeQuotesPartial},
		{RankedFallbackQuoteProviderMock, RunOutcomeQuotesMock},
		{"something_new", RunOutcomeOther},
	} {
		if got := RunOutcome(tc.reason); got != tc.want {
			t.Errorf("RunOutcome(%q) = %q, want %q", tc.reason, got, tc.want)
		}
	}
}

// TestUniverseRunVerdictFor_ProjectsTheResult keeps the projection honest: the
// gauges must describe the run that actually happened, and the screened buckets
// must add up to the filtered population.
func TestUniverseRunVerdictFor_ProjectsTheResult(t *testing.T) {
	at := time.Date(2026, 9, 29, 6, 1, 0, 0, time.UTC)
	result := &UniverseBuildResult{
		SymbolsBuilt:         1599,
		SymbolsFiltered:      1599,
		SymbolsRanked:        150,
		QuotesRequested:      1599,
		QuotesReturned:       1581,
		RankedTrustworthy:    true,
		RankedFallbackReason: "",
	}
	v := UniverseRunVerdictFor(result, metrics.UniverseStageDaily, at)
	if v.Stage != metrics.UniverseStageDaily || v.Outcome != RunOutcomeOK || !v.Trustworthy {
		t.Fatalf("verdict = %+v, want stage=daily outcome=ok trustworthy=true", v)
	}
	if v.Gathered != 1599 || v.Filtered != 1599 || v.Ranked != 150 {
		t.Errorf("counts = gathered %d filtered %d ranked %d", v.Gathered, v.Filtered, v.Ranked)
	}
	if v.ScreenedPassed != 150 || v.ScreenedFailed != 1449 {
		t.Errorf("screened buckets = passed %d failed %d, want 150/1449", v.ScreenedPassed, v.ScreenedFailed)
	}
	if v.ScreenedPassed+v.ScreenedFailed != v.Filtered {
		t.Errorf("screened buckets do not cover the filtered population: %d+%d != %d",
			v.ScreenedPassed, v.ScreenedFailed, v.Filtered)
	}
	if v.FinishedAt != at {
		t.Errorf("FinishedAt = %v, want %v", v.FinishedAt, at)
	}

	// The empty_filtered shape: filtered = 0 and the failed bucket must not go
	// negative when ranked somehow exceeds filtered.
	empty := UniverseRunVerdictFor(&UniverseBuildResult{
		SymbolsBuilt:         100,
		SymbolsFiltered:      0,
		SymbolsRanked:        0,
		RankedFallbackReason: RankedFallbackEmptyFiltered,
	}, metrics.UniverseStageWeekly, at)
	if empty.Outcome != RunOutcomeFilteredEmpty {
		t.Errorf("outcome = %q, want %q", empty.Outcome, RunOutcomeFilteredEmpty)
	}
	if empty.ScreenedFailed != 0 {
		t.Errorf("failed bucket = %d, want 0", empty.ScreenedFailed)
	}

	// Nil result must not panic: the defer runs on every path, including paths
	// that never built a result.
	if got := UniverseRunVerdictFor(nil, metrics.UniverseStageDaily, at); got.Outcome != RunOutcomeOther {
		t.Errorf("nil result outcome = %q, want %q", got.Outcome, RunOutcomeOther)
	}
}

// TestNextUniverseRun_IsHolidayAware is the reason the heartbeat exists: it
// answers "when is the pipeline next expected to run?" from the Taiwan trading
// calendar, so a long holiday cannot be mistaken for a dead scheduler (and a
// dead scheduler is visible hours after the trigger, not six days later).
//
// 2026-09-28 is 教師節 (market closed, Monday), and the weekly rebuild skips
// non-trading Mondays while the daily refresh skips every Monday.
func TestNextUniverseRun_IsHolidayAware(t *testing.T) {
	for _, tc := range []struct {
		name string
		now  string
		want string
	}{
		{
			name: "friday after the run: the weekend plus the 09-28 holiday are skipped",
			now:  "2026-09-25T07:00:00Z",
			want: "2026-09-29T06:00:00Z",
		},
		{
			name: "on the holiday monday itself",
			now:  "2026-09-28T07:00:00Z",
			want: "2026-09-29T06:00:00Z",
		},
		{
			name: "before the trigger minute: today",
			now:  "2026-09-29T05:00:00Z",
			want: "2026-09-29T06:00:00Z",
		},
		{
			name: "after the trigger minute: tomorrow",
			now:  "2026-09-29T06:00:30Z",
			want: "2026-09-30T06:00:00Z",
		},
		{
			name: "ordinary monday is the next run (weekly rebuild)",
			now:  "2026-10-02T07:00:00Z",
			want: "2026-10-05T06:00:00Z",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := mustParseInstant(t, tc.now)
			want := mustParseInstant(t, tc.want)
			if got := NextUniverseRun(now); !got.Equal(want) {
				t.Fatalf("NextUniverseRun(%s) = %s, want %s", tc.now, got.UTC().Format(time.RFC3339), tc.want)
			}
		})
	}
}

func mustParseInstant(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return parsed
}
