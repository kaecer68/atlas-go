package monitoring

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/monitoring/metrics"
)

// TestCollectorOnInc_UniverseSeriesShape is the regression test for both
// 2026-09-25 monitoring defects, asserted on the artifact Prometheus and the
// alert rules actually consume (the /metrics text):
//
//	(1) values must accumulate as x·N, not x·N(N+1)/2 — RecordCounter accumulates,
//	    so the callback must forward per-event deltas;
//	(2) series must carry the vector's real label names (stage/result), so the
//	    daily and weekly stages are distinguishable and single-label series keep
//	    their label instead of being dropped.
func TestCollectorOnInc_UniverseSeriesShape(t *testing.T) {
	collector := NewMetricsCollector()
	um := metrics.NewUniverseMetrics()
	um.SetOnInc(CollectorOnInc(collector))

	// Production shape: two daily runs of 1599 screened symbols, none passing
	// (ranked = 0), plus one weekly run.
	for range 2 {
		um.SymbolsGathered.WithLabelValues("daily").Add(1599)
		um.SymbolsScreened.WithLabelValues("daily", "failed").Add(1599)
		um.SymbolsScreened.WithLabelValues("daily", "passed").Add(0)
		um.SymbolsRanked.WithLabelValues("daily").Add(0)
	}
	um.SymbolsScreened.WithLabelValues("weekly", "failed").Add(1200)

	t.Run("values_are_true_deltas", func(t *testing.T) {
		m, ok := collector.GetMetric("atlas_universe_symbols_screened_total",
			map[string]string{"stage": "daily", "result": "failed"})
		if !ok {
			t.Fatal("daily/failed series missing from the collector")
		}
		if m.Value != 3198 { // 2 × 1599; the bug produced 4797 = x·N(N+1)/2
			t.Fatalf("daily/failed = %v, want 3198 (4797 means cumulative values are being forwarded)", m.Value)
		}
		if m.Type != MetricTypeCounter {
			t.Fatalf("metric type = %q, want %q", m.Type, MetricTypeCounter)
		}
	})

	t.Run("stages_are_distinguishable", func(t *testing.T) {
		daily, ok := collector.GetMetric("atlas_universe_symbols_screened_total",
			map[string]string{"stage": "daily", "result": "failed"})
		if !ok {
			t.Fatal("daily series missing")
		}
		weekly, ok := collector.GetMetric("atlas_universe_symbols_screened_total",
			map[string]string{"stage": "weekly", "result": "failed"})
		if !ok {
			t.Fatal("weekly series missing: the two stages must not share one series")
		}
		if daily.Value != 3198 || weekly.Value != 1200 {
			t.Fatalf("daily = %v (want 3198), weekly = %v (want 1200)", daily.Value, weekly.Value)
		}
	})

	t.Run("single_label_series_keeps_its_label", func(t *testing.T) {
		m, ok := collector.GetMetric("atlas_universe_symbols_ranked_total",
			map[string]string{"stage": "daily"})
		if !ok {
			t.Fatal("ranked{daily} missing: the single-label series must keep stage")
		}
		if m.Value != 0 {
			t.Fatalf("ranked{daily} = %v, want 0 (Add(0) still materializes the series)", m.Value)
		}
	})

	t.Run("no_invented_label_names", func(t *testing.T) {
		// Negative control for (2): the old implementation paired label VALUES
		// into name=value, i.e. the value "daily" became a label name.
		if m, ok := collector.GetMetric("atlas_universe_symbols_screened_total",
			map[string]string{"daily": "failed"}); ok {
			t.Fatalf("invented-label series still recorded: %+v", m)
		}
		if _, ok := collector.GetMetric("atlas_universe_symbols_ranked_total", map[string]string{}); ok {
			t.Fatal("unlabeled ranked series still recorded: the stage label is being dropped")
		}
	})

	t.Run("prometheus_text_uses_real_label_names", func(t *testing.T) {
		rec := httptest.NewRecorder()
		PrometheusHandler(collector).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
		body := rec.Body.String()

		for _, want := range []string{
			`atlas_universe_symbols_screened_total{result="failed",stage="daily"} 3198.000000`,
			`atlas_universe_symbols_screened_total{result="failed",stage="weekly"} 1200.000000`,
			`atlas_universe_symbols_ranked_total{stage="daily"} 0.000000`,
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("/metrics is missing %q", want)
			}
		}
		if strings.Contains(body, `{daily=`) {
			t.Fatal(`/metrics still exposes the invented label name {daily=...}`)
		}
	})
}

// TestCollectorOnInc_WarmUpExposesFamilyBeforeAnyRun is the end-to-end
// regression test for issue #1995: on /metrics, the whole atlas_universe_*
// family used to be absent until the first pipeline run after a restart.
//
// Production evidence (2026-09-25): container restarted at 07:14Z, the family
// stayed absent for ~71h (until the next scheduled run), and because the alert
// rules read `sum(increase(...)) or vector(0)`, "no data" was scored as "0" —
// three alerts fired on a healthy pipeline whose counters simply had not been
// touched yet.
//
// The test drives the same artifact the alerts' operators inspect (the
// Prometheus text body) through the same wiring production uses.
func TestCollectorOnInc_WarmUpExposesFamilyBeforeAnyRun(t *testing.T) {
	collector := NewMetricsCollector()
	um := metrics.NewUniverseMetrics()
	um.SetOnInc(CollectorOnInc(collector))

	scrape := func() string {
		t.Helper()
		rec := httptest.NewRecorder()
		PrometheusHandler(collector).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
		return rec.Body.String()
	}
	countUniverseLines := func(body string) int {
		n := 0
		for _, line := range strings.Split(body, "\n") {
			if strings.HasPrefix(line, "atlas_universe_") {
				n++
			}
		}
		return n
	}

	if got := countUniverseLines(scrape()); got != 0 {
		t.Fatalf("before WarmUp the family already had %d lines: the test can no longer show the defect", got)
	}

	um.WarmUp()

	body := scrape()
	if got := countUniverseLines(body); got == 0 {
		t.Fatal("after WarmUp /metrics still exposes no atlas_universe_* series (issue #1995 is back)")
	}
	for _, want := range []string{
		`atlas_universe_symbols_gathered_total{stage="daily"} 0.000000`,
		`atlas_universe_symbols_filtered_total{reason="dropped",stage="daily"} 0.000000`,
		`atlas_universe_quotes_fetched_total{stage="daily"} 0.000000`,
		`atlas_universe_symbols_screened_total{result="failed",stage="daily"} 0.000000`,
		`atlas_universe_symbols_screened_total{result="passed",stage="daily"} 0.000000`,
		`atlas_universe_symbols_ranked_total{stage="daily"} 0.000000`,
		`atlas_universe_symbols_ranked_total{stage="weekly"} 0.000000`,
		`atlas_universe_coverage_mapped_total{industry="all",stage="coverage_check"} 0.000000`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics is missing %q", want)
		}
	}

	// A later run still accumulates on top of the warmed series (the warm-up
	// must not shadow or reset anything).
	um.SymbolsScreened.WithLabelValues("daily", "failed").Add(1449)
	um.SymbolsScreened.WithLabelValues("daily", "passed").Add(150)
	um.SymbolsRanked.WithLabelValues("daily").Add(150)

	body = scrape()
	for _, want := range []string{
		`atlas_universe_symbols_screened_total{result="failed",stage="daily"} 1449.000000`,
		`atlas_universe_symbols_screened_total{result="passed",stage="daily"} 150.000000`,
		`atlas_universe_symbols_ranked_total{stage="daily"} 150.000000`,
		`atlas_universe_symbols_ranked_total{stage="weekly"} 0.000000`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("after a run, /metrics is missing %q", want)
		}
	}
}

// TestCollectorOnInc_DegradedSeriesShape covers the second production wiring
// (DegradedMetrics → /metrics) through the same bridge.
func TestCollectorOnInc_DegradedSeriesShape(t *testing.T) {
	collector := NewMetricsCollector()
	dm := metrics.NewDegradedMetrics()
	dm.SetOnInc(CollectorOnInc(collector))

	c := dm.DegradedActivations.WithLabelValues("crossmarket", "snapshot_stale")
	c.Inc()
	c.Inc()

	m, ok := collector.GetMetric("degraded_activations",
		map[string]string{"service": "crossmarket", "reason": "snapshot_stale"})
	if !ok {
		t.Fatal("degraded_activations series missing from the collector")
	}
	if m.Value != 2 {
		t.Fatalf("degraded_activations = %v, want 2 (3 means cumulative values are being forwarded)", m.Value)
	}
}

// TestCollectorOnInc_NilCollector pins the nil-safe contract used by call sites
// that may not have a collector (e.g. CLI paths).
func TestCollectorOnInc_NilCollector(t *testing.T) {
	if fn := CollectorOnInc(nil); fn != nil {
		t.Fatal("CollectorOnInc(nil) must return a nil callback so SetOnInc records nothing")
	}
}
