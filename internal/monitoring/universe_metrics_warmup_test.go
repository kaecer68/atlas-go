package monitoring

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/monitoring/metrics"
)

// universeWarmupKey renders a series the way it appears in /metrics, so a
// failure names the exact series an operator would grep for.
func universeWarmupKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+labels[k])
	}
	return name + "{" + strings.Join(parts, ",") + "}"
}

// TestUniverseMetricsWarmUp_CoversEverySeriesThePipelineAdds keeps WarmUp's
// enumeration honest (issue #1995).
//
// WarmUp mirrors the Add sites in this package by hand, so a new bucket (or a
// new counter) added to BuildUniverse can silently fall back into the
// "invisible on /metrics until the first increment" state that #1995 is about.
// This test drives a real pipeline run through the same wiring production uses
// (UniverseMetrics + SetOnInc) and fails if the run materializes a series that
// WarmUp does not create.
func TestUniverseMetricsWarmUp_CoversEverySeriesThePipelineAdds(t *testing.T) {
	um := metrics.NewUniverseMetrics()

	warmed := map[string]bool{}
	um.SetOnInc(func(name string, labels map[string]string, _ float64) {
		warmed[universeWarmupKey(name, labels)] = true
	})
	um.WarmUp()
	if len(warmed) == 0 {
		t.Fatal("WarmUp materialized nothing")
	}

	produced := map[string]bool{}
	um.SetOnInc(func(name string, labels map[string]string, _ float64) {
		produced[universeWarmupKey(name, labels)] = true
	})

	deps := buildDepsFixture(t, tempDir(t))
	deps.UniverseMetrics = um
	if _, ranked, err := BuildUniverse(context.Background(), deps, false); err != nil {
		t.Fatalf("BuildUniverse: %v", err)
	} else if len(ranked) == 0 {
		t.Fatal("fixture produced no ranked symbols: the run did not exercise Step 4-7")
	}

	if len(produced) == 0 {
		t.Fatal("the pipeline run reported no counters: the sink is not wired")
	}
	for key := range produced {
		if !warmed[key] {
			t.Errorf("pipeline run materialized %s, which WarmUp does not create "+
				"(add it to warmUpSeries, otherwise the series vanishes from /metrics after every restart)", key)
		}
	}
}
