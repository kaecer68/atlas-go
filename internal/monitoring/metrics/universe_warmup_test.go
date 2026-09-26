package metrics

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

// seriesKey renders a series the way an operator sees it in /metrics:
// atlas_universe_symbols_screened_total{result="failed",stage="daily"}.
func seriesKey(name string, labels map[string]string) string {
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

// warmUpSink records every OnInc event the family forwards.
type warmUpSink struct {
	deltas map[string]float64
}

func newWarmUpSink() *warmUpSink {
	return &warmUpSink{deltas: map[string]float64{}}
}

func (s *warmUpSink) onInc(name string, labels map[string]string, value float64) {
	s.deltas[seriesKey(name, labels)] += value
}

func (s *warmUpSink) keys() []string {
	out := make([]string, 0, len(s.deltas))
	for k := range s.deltas {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestUniverseMetricsWarmUp_MaterializesEverySeriesOnce is the regression test
// for issue #1995: the atlas_universe_* family must exist on /metrics from
// process start, not only after the first pipeline run. WarmUp must therefore
// create every series exactly once, all with a zero delta (no fabricated
// increments), and it must be idempotent.
func TestUniverseMetricsWarmUp_MaterializesEverySeriesOnce(t *testing.T) {
	um := NewUniverseMetrics()
	sink := newWarmUpSink()
	um.SetOnInc(sink.onInc)

	um.WarmUp()

	want := map[string]bool{}
	for _, spec := range um.warmUpSeries() {
		name := spec.counter.name
		labels := map[string]string{}
		for i, v := range spec.values {
			labels[spec.counter.labelNames[i]] = v
		}
		if want[seriesKey(name, labels)] {
			t.Fatalf("duplicate series in warmUpSeries(): %s", seriesKey(name, labels))
		}
		want[seriesKey(name, labels)] = true
	}

	got := sink.keys()
	if len(got) != len(want) {
		t.Fatalf("WarmUp materialized %d series, want %d\ngot  %v\nwant %v", len(got), len(want), got, sortedKeys(want))
	}
	for _, k := range got {
		if !want[k] {
			t.Errorf("WarmUp materialized %s, which no pipeline Add site produces", k)
		}
		if d := sink.deltas[k]; d != 0 {
			t.Errorf("WarmUp emitted delta %v for %s, want 0 (it must not fabricate increments)", d, k)
		}
	}

	// Idempotent: a second call must not add or change anything.
	um.WarmUp()
	if len(sink.deltas) != len(got) {
		t.Fatalf("second WarmUp changed the series count: %d -> %d", len(got), len(sink.deltas))
	}
	for k, d := range sink.deltas {
		if d != 0 {
			t.Errorf("after second WarmUp, %s has delta %v, want 0", k, d)
		}
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestUniverseMetricsWarmUp_CoversEveryCounterVec guards the enumeration in
// warmUpSeries() against the cheapest way to get it wrong: adding a counter to
// the struct (and to the pipeline) but forgetting to list it, which would put
// that series back into the "invisible until the first increment" state.
func TestUniverseMetricsWarmUp_CoversEveryCounterVec(t *testing.T) {
	um := NewUniverseMetrics()

	covered := map[string]bool{}
	for _, spec := range um.warmUpSeries() {
		covered[spec.counter.name] = true
	}

	typ := reflect.TypeOf(UniverseMetrics{})
	vectors := 0
	for i := range typ.NumField() {
		field := typ.Field(i)
		if field.Type != reflect.TypeOf(&CounterVec{}) {
			continue
		}
		vectors++
		vec := reflect.ValueOf(um).Elem().Field(i).Interface().(*CounterVec)
		if !covered[vec.name] {
			t.Errorf("%s (%s) has no warmed series: every counter of this family must be materialized at startup",
				field.Name, vec.name)
		}
	}
	if vectors == 0 {
		t.Fatal("reflection found no CounterVec fields: the guard is not looking at anything")
	}
}

// TestUniverseMetricsWarmUp_RequiresSetOnIncFirst pins the ordering contract
// documented on WarmUp. Before SetOnInc nothing is forwarded, so a WarmUp call
// placed above it is silently inert — the exact failure mode the wiring in
// cmd/atlas/main.go must not have.
func TestUniverseMetricsWarmUp_RequiresSetOnIncFirst(t *testing.T) {
	um := NewUniverseMetrics()
	sink := newWarmUpSink()

	um.WarmUp() // before SetOnInc: no sink yet
	um.SetOnInc(sink.onInc)
	if len(sink.deltas) != 0 {
		t.Fatalf("WarmUp before SetOnInc leaked %d events into the sink", len(sink.deltas))
	}

	um.WarmUp() // after SetOnInc: the family materializes
	if len(sink.deltas) == 0 {
		t.Fatal("WarmUp after SetOnInc materialized nothing")
	}
}
