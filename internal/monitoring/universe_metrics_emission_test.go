package monitoring

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/monitoring/metrics"
)

// This file is the regression net for the 2026-09-25 production incident
// `universe-scoring-gap-20260925` in its *second* form:
//
//	the pipeline ran, produced a healthy snapshot, and two of the counters the
//	alert rules read (symbols_ranked_total, quotes_fetched_total) never
//	incremented — while their siblings (symbols_screened_total,
//	symbols_filtered_total) did.
//
// metrics_bridge_test.go pins the shape of the *sink* (per-event deltas, real
// label names). What was missing — and what let the defect reach production —
// is a test that runs the REAL pipeline (BuildUniverse, the same function the
// daily/weekly scheduled tasks call) through the REAL bridge into the REAL
// artifact the alerts read (/metrics). Every assertion below is made on that
// text, so a wiring change that silently drops a series fails here.
//
// Why the pipeline level matters: an emit point lives inside BuildUniverse
// (um.SymbolsRanked...Add / quotesFetchedCounter.Add). No sink-level test can
// see whether those lines were reached, whether the stage label was right, or
// whether a later refactor moved them behind an early return.

// scrapeUniverseMetrics returns the /metrics body produced by the production
// handler for the given collector.
func scrapeUniverseMetrics(t *testing.T, collector *MetricsCollector) string {
	t.Helper()
	rec := httptest.NewRecorder()
	PrometheusHandler(collector).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	return rec.Body.String()
}

// wantMetricLine reports whether the /metrics body contains the exact series
// line (labels sorted by name, as formatMetricLine writes them).
func wantMetricLine(t *testing.T, body, line string) {
	t.Helper()
	if !strings.Contains(body, line+"\n") {
		t.Errorf("/metrics is missing %q", line)
	}
}

// hasUnlabeledSampleLine reports whether the /metrics body contains a sample
// line for name that carries no labels at all (HELP/TYPE lines are ignored).
func hasUnlabeledSampleLine(body, name string) bool {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, name+" ") {
			return true
		}
	}
	return false
}

// wireUniverseSink reproduces the production wiring (cmd/atlas/main.go):
// counters through CollectorOnInc, warm-up after it.
func wireUniverseSink(collector *MetricsCollector) *metrics.UniverseMetrics {
	um := metrics.NewUniverseMetrics()
	um.SetOnInc(CollectorOnInc(collector))
	um.WarmUp()
	return um
}

// TestBuildUniverse_EmitsEveryCounterTheAlertsRead is the decisive check of
// requirement (1) of the 2026-09-27 task: does the FIXED wiring actually
// increment symbols_ranked_total and quotes_fetched_total?
//
// It runs BuildUniverse twice per stage (daily + weekly) — exactly as the two
// scheduled tasks do — and asserts the resulting /metrics text.
func TestBuildUniverse_EmitsEveryCounterTheAlertsRead(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name        string
		fullRebuild bool
		stage       string
	}{
		{name: "daily", fullRebuild: false, stage: metrics.UniverseStageDaily},
		{name: "weekly", fullRebuild: true, stage: metrics.UniverseStageWeekly},
	} {
		t.Run(tc.name, func(t *testing.T) {
			collector := NewMetricsCollector()
			um := wireUniverseSink(collector)

			deps := buildDepsFixture(t, tempDir(t))
			deps.UniverseMetrics = um

			result, ranked, err := BuildUniverse(ctx, deps, tc.fullRebuild)
			if err != nil {
				t.Fatalf("BuildUniverse: %v", err)
			}
			// Guards: a run that ranked nothing would make the assertions below
			// pass for the wrong reason (0 == 0 proves nothing).
			if result.SymbolsBuilt == 0 || len(ranked) == 0 {
				t.Fatalf("fixture produced built=%d ranked=%d; the assertions below need a non-empty run",
					result.SymbolsBuilt, len(ranked))
			}
			if result.QuotesReturned == 0 {
				t.Fatalf("fixture returned no quotes; can not assert quotes_fetched_total")
			}

			body := scrapeUniverseMetrics(t, collector)

			// The two series the 2026-09-25 incident was about.
			wantMetricLine(t, body,
				fmt.Sprintf("atlas_universe_symbols_ranked_total{stage=%q} %d.000000", tc.stage, len(ranked)))
			wantMetricLine(t, body,
				fmt.Sprintf("atlas_universe_quotes_fetched_total{stage=%q} %d.000000", tc.stage, result.QuotesReturned))

			// The stage's neighbours, so a wrong stage label can not hide.
			wantMetricLine(t, body,
				fmt.Sprintf("atlas_universe_symbols_gathered_total{stage=%q} %d.000000", tc.stage, result.SymbolsBuilt))
			wantMetricLine(t, body,
				fmt.Sprintf("atlas_universe_symbols_screened_total{result=%q,stage=%q} %d.000000",
					"passed", tc.stage, len(ranked)))

			// The other stage must stay at its warm-up zero: if the two stages
			// shared one series again (the pre-#1989 label-pairing defect), or if
			// a run leaked into the wrong stage, this fails.
			other := metrics.UniverseStageDaily
			if tc.stage == metrics.UniverseStageDaily {
				other = metrics.UniverseStageWeekly
			}
			wantMetricLine(t, body,
				fmt.Sprintf("atlas_universe_symbols_ranked_total{stage=%q} 0.000000", other))
			wantMetricLine(t, body,
				fmt.Sprintf("atlas_universe_quotes_fetched_total{stage=%q} 0.000000", other))

			// Negative control for the pre-#1989 label-pairing defect: the label
			// VALUES ("daily"/"weekly") must never appear as label NAMES, and the
			// single-label series must not be dropped into an unlabeled series.
			if strings.Contains(body, `{daily="`) || strings.Contains(body, `{weekly="`) {
				t.Error("/metrics exposes the invented label names {daily=...}/{weekly=...}")
			}
			for _, bare := range []string{
				"atlas_universe_symbols_ranked_total",
				"atlas_universe_quotes_fetched_total",
			} {
				if hasUnlabeledSampleLine(body, bare) {
					t.Errorf("%s is exposed without its stage label", bare)
				}
			}
		})
	}
}

// TestBuildUniverse_CounterIncrementsAreDeltasNotCumulative pins the second
// half of the 2026-09-25 defect (x·N vs x·N(N+1)/2) at the pipeline level:
// three consecutive runs must add 3x, not 6x.
func TestBuildUniverse_CounterIncrementsAreDeltasNotCumulative(t *testing.T) {
	ctx := context.Background()

	collector := NewMetricsCollector()
	um := wireUniverseSink(collector)
	deps := buildDepsFixture(t, tempDir(t))
	deps.UniverseMetrics = um

	var perRun int
	const runs = 3
	for i := 0; i < runs; i++ {
		_, ranked, err := BuildUniverse(ctx, deps, false)
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		if i == 0 {
			perRun = len(ranked)
		} else if len(ranked) != perRun {
			t.Fatalf("run %d ranked %d symbols, run 1 ranked %d; the arithmetic below assumes a stable run",
				i, len(ranked), perRun)
		}
	}
	if perRun == 0 {
		t.Fatal("fixture ranked nothing; the assertion below would be vacuous")
	}

	m, ok := collector.GetMetric("atlas_universe_symbols_ranked_total",
		map[string]string{"stage": metrics.UniverseStageDaily})
	if !ok {
		t.Fatal("ranked{daily} series missing from the collector")
	}
	if want := float64(runs * perRun); m.Value != want {
		t.Errorf("ranked{daily} = %v after %d runs of %d, want %v (%v means cumulative values are forwarded)",
			m.Value, runs, perRun, want, float64(perRun*runs*(runs+1)/2))
	}
}

// TestBuildUniverse_DroppedSeriesIsDetectable is the mutation half of the net:
// it feeds the same pipeline output through a sink that reproduces the
// 2026-09-25 production defect shape (single-label series dropped, label
// values used as label names — see CollectorOnInc's doc comment) and asserts
// that the checks of the test above DO see the difference. Without this, a
// green suite would not prove the assertions have teeth.
func TestBuildUniverse_DroppedSeriesIsDetectable(t *testing.T) {
	ctx := context.Background()

	collector := NewMetricsCollector()
	um := metrics.NewUniverseMetrics()
	// Defect shape (production, 2026-06-22..2026-09-25): drop every series with
	// fewer than two labels — that is what happened to
	// atlas_universe_symbols_ranked_total — and for the rest, turn the ordered
	// label VALUES into name=value pairs, which is how the production TSDB ended
	// up with {daily="failed"} instead of {stage="daily",result="failed"}.
	um.SetOnInc(func(counterName string, labels map[string]string, value float64) {
		switch len(labels) {
		case 1:
			return // single-label series dropped entirely
		case 2:
			collector.RecordCounter(counterName, value, map[string]string{
				labels["stage"]: labels["reason"] + labels["result"],
			})
		default:
			return
		}
	})
	um.WarmUp()

	deps := buildDepsFixture(t, tempDir(t))
	deps.UniverseMetrics = um
	if _, _, err := BuildUniverse(ctx, deps, false); err != nil {
		t.Fatalf("BuildUniverse: %v", err)
	}

	body := scrapeUniverseMetrics(t, collector)
	if strings.Contains(body, `atlas_universe_symbols_ranked_total{stage="daily"}`) {
		t.Fatal("the dropped-series defect is NOT detectable: ranked{daily} appeared anyway")
	}
	if !strings.Contains(body, `{daily="`) {
		t.Fatal("the label-pairing defect is NOT detectable: no {daily=...} series was produced")
	}
	t.Log("confirmed: with the pre-#1989 bridge shape the assertions of TestBuildUniverse_EmitsEveryCounterTheAlertsRead fail")
}

// TestBuildUniverse_PublishesRunVerdictAndHeartbeat is the end-to-end proof for
// the second half of the 2026-09-27 change: the run verdict and the schedule
// heartbeat must appear on /metrics with the values the run actually produced —
// including on the degraded paths — because the alert rules read them instead
// of counter increments.
func TestBuildUniverse_PublishesRunVerdictAndHeartbeat(t *testing.T) {
	ctx := context.Background()

	t.Run("healthy daily run", func(t *testing.T) {
		collector := NewMetricsCollector()
		um := metrics.NewUniverseMetrics()
		um.SetOnInc(CollectorOnInc(collector))
		um.SetRunVerdictSink(GaugeSink(collector))
		um.WarmUp()

		deps := buildDepsFixture(t, tempDir(t))
		deps.UniverseMetrics = um

		result, ranked, err := BuildUniverse(ctx, deps, false)
		if err != nil {
			t.Fatalf("BuildUniverse: %v", err)
		}
		if len(ranked) == 0 || result.QuotesReturned == 0 {
			t.Fatalf("fixture produced ranked=%d quotes_returned=%d; the assertions need a non-empty run",
				len(ranked), result.QuotesReturned)
		}

		body := scrapeUniverseMetrics(t, collector)
		for _, line := range []string{
			`atlas_universe_last_run_valid{stage="daily"} 1.000000`,
			fmt.Sprintf("atlas_universe_last_run_symbols_ranked{stage=%q} %d.000000", "daily", len(ranked)),
			fmt.Sprintf("atlas_universe_last_run_quotes_returned{stage=%q} %d.000000", "daily", result.QuotesReturned),
			`atlas_universe_last_run_ranked_trustworthy{stage="daily"} 1.000000`,
			`atlas_universe_last_run_outcome{outcome="ok",stage="daily"} 1.000000`,
			// The weekly stage has not run in this process: the warm-up must leave
			// valid at 0 so no alert can read a fresh-but-empty verdict.
			`atlas_universe_last_run_valid{stage="weekly"} 0.000000`,
		} {
			wantMetricLine(t, body, line)
		}

		// The heartbeat is published by the same run without any stage label.
		if !strings.Contains(body, "atlas_universe_next_run_timestamp_seconds ") {
			t.Error("/metrics is missing the schedule heartbeat atlas_universe_next_run_timestamp_seconds")
		}
	})

	t.Run("degraded run flips the verdict and un-sets the previous outcome", func(t *testing.T) {
		collector := NewMetricsCollector()
		um := metrics.NewUniverseMetrics()
		um.SetOnInc(CollectorOnInc(collector))
		um.SetRunVerdictSink(GaugeSink(collector))
		um.WarmUp()

		deps := buildDepsFixture(t, tempDir(t))
		deps.UniverseMetrics = um
		if _, _, err := BuildUniverse(ctx, deps, false); err != nil {
			t.Fatalf("healthy run: %v", err)
		}

		// The historical production regression: the quote provider is not wired.
		degraded := buildDepsFixture(t, tempDir(t))
		degraded.UniverseMetrics = um
		degraded.Quotes = nil
		result, ranked, err := BuildUniverse(ctx, degraded, false)
		if err != nil {
			t.Fatalf("degraded run: %v", err)
		}
		if len(ranked) != 0 || result.RankedTrustworthy {
			t.Fatalf("fixture did not degrade as expected: ranked=%d trustworthy=%v", len(ranked), result.RankedTrustworthy)
		}

		body := scrapeUniverseMetrics(t, collector)
		for _, line := range []string{
			`atlas_universe_last_run_valid{stage="daily"} 1.000000`,
			`atlas_universe_last_run_symbols_gathered{stage="daily"} 2.000000`,
			`atlas_universe_last_run_symbols_ranked{stage="daily"} 0.000000`,
			`atlas_universe_last_run_ranked_trustworthy{stage="daily"} 0.000000`,
			`atlas_universe_last_run_outcome{outcome="quotes_unavailable",stage="daily"} 1.000000`,
			// One-hot: the previous outcome of that stage is zeroed, so the
			// current outcome is unambiguous.
			`atlas_universe_last_run_outcome{outcome="ok",stage="daily"} 0.000000`,
		} {
			wantMetricLine(t, body, line)
		}
	})
}
