package llm_annotator

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestMetricsRecorder_NoopNeverPanics(t *testing.T) {
	// KimiClient stores a non-nil MetricsRecorder; the value type noopMetrics
	// is the default when Config.Metrics is nil.
	m := noopMetrics{}
	m.RecordCounter("any", 1, map[string]string{"k": "v"})
	m.RecordGauge("any", 1, nil)
}

func TestCountingMetrics_CounterAccumulates(t *testing.T) {
	m := newCountingMetrics()
	m.RecordCounter("requests_total", 1, map[string]string{"outcome": "success"})
	m.RecordCounter("requests_total", 1, map[string]string{"outcome": "success"})
	m.RecordCounter("requests_total", 1, map[string]string{"outcome": "error"})
	if got := m.CounterValue("requests_total", map[string]string{"outcome": "success"}); got != 2 {
		t.Errorf("success counter = %v, want 2", got)
	}
	if got := m.CounterValue("requests_total", map[string]string{"outcome": "error"}); got != 1 {
		t.Errorf("error counter = %v, want 1", got)
	}
	if got := m.CounterValue("requests_total", nil); got != 0 {
		t.Errorf("missing-label counter = %v, want 0", got)
	}
}

func TestCountingMetrics_GaugeLastWriteWins(t *testing.T) {
	m := newCountingMetrics()
	m.RecordGauge("latency_ms", 100, map[string]string{"op": "annotate"})
	m.RecordGauge("latency_ms", 250, map[string]string{"op": "annotate"})
	if got := m.GaugeValue("latency_ms", map[string]string{"op": "annotate"}); got != 250 {
		t.Errorf("gauge = %v, want 250 (last write)", got)
	}
}

func TestCountingMetrics_LabelOrderIndependent(t *testing.T) {
	m := newCountingMetrics()
	m.RecordCounter("requests_total", 1, map[string]string{"a": "1", "b": "2"})
	m.RecordCounter("requests_total", 1, map[string]string{"b": "2", "a": "1"})
	if got := m.CounterValue("requests_total", map[string]string{"a": "1", "b": "2"}); got != 2 {
		t.Errorf("counter across re-ordered labels = %v, want 2", got)
	}
}

func TestCountingMetrics_CallCountTracksEveryInvocation(t *testing.T) {
	m := newCountingMetrics()
	for range 7 {
		m.RecordCounter("x", 1, map[string]string{"k": "v"})
	}
	for range 3 {
		m.RecordGauge("y", 1, map[string]string{"k": "v"})
	}
	if got := m.CallCount("x", map[string]string{"k": "v"}); got != 7 {
		t.Errorf("x calls = %v, want 7", got)
	}
	if got := m.CallCount("y", map[string]string{"k": "v"}); got != 3 {
		t.Errorf("y calls = %v, want 3", got)
	}
	if got := m.TotalCalls(); got != 10 {
		t.Errorf("total calls = %v, want 10", got)
	}
}

func TestCountingMetrics_ConcurrentSafe(t *testing.T) {
	m := newCountingMetrics()
	const goroutines = 16
	const perG = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for j := range perG {
				m.RecordCounter("c", 1, map[string]string{"k": "v"})
				m.RecordGauge("g", float64(j), map[string]string{"k": "v"})
			}
		}()
	}
	wg.Wait()
	want := float64(goroutines * perG)
	if got := m.CounterValue("c", map[string]string{"k": "v"}); got != want {
		t.Errorf("concurrent counter = %v, want %v", got, want)
	}
	if got := m.CallCount("c", map[string]string{"k": "v"}); got != goroutines*perG {
		t.Errorf("concurrent call count = %v, want %v", got, goroutines*perG)
	}
}

func TestMetricKey_StableForSameLabels(t *testing.T) {
	a := metricKey("name", map[string]string{"a": "1", "b": "2"})
	b := metricKey("name", map[string]string{"b": "2", "a": "1"})
	if a != b {
		t.Errorf("metricKey not order-independent:\n a=%q\n b=%q", a, b)
	}
}

func TestMetricKey_NilLabelsEqualsEmptyLabels(t *testing.T) {
	if metricKey("name", nil) != metricKey("name", map[string]string{}) {
		t.Errorf("nil and empty labels must produce same key")
	}
}

func TestAppendAnnotation_StoresAndReturns(t *testing.T) {
	k := &KimiClient{}
	rec := AnnotationRecord{ID: "ann-1", Label: "alerts", Tokens: 42, Outcome: "success", LatencyMs: 100}
	k.appendAnnotation(rec)

	got := k.RecentAnnotations(0)
	if len(got) != 1 {
		t.Fatalf("RecentAnnotations(0) len = %d, want 1", len(got))
	}
	if got[0] != rec {
		t.Errorf("RecentAnnotations[0] = %+v, want %+v", got[0], rec)
	}
}

func TestRecentAnnotations_ReturnsLastN(t *testing.T) {
	k := &KimiClient{}
	for i := range 5 {
		k.appendAnnotation(AnnotationRecord{ID: fmt.Sprintf("ann-%d", i), Tokens: int64(i)})
	}

	got := k.RecentAnnotations(3)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	for i, want := range []string{"ann-2", "ann-3", "ann-4"} {
		if got[i].ID != want {
			t.Errorf("RecentAnnotations[%d].ID = %q, want %q", i, got[i].ID, want)
		}
	}
}

func TestRecentAnnotations_NLargerThanSizeReturnsAll(t *testing.T) {
	k := &KimiClient{}
	for i := range 3 {
		k.appendAnnotation(AnnotationRecord{ID: fmt.Sprintf("ann-%d", i)})
	}

	got := k.RecentAnnotations(10)
	if len(got) != 3 {
		t.Errorf("RecentAnnotations(10) len = %d, want 3", len(got))
	}
}

func TestRecentAnnotations_RingBufferDropsOldest(t *testing.T) {
	k := &KimiClient{}
	total := annotationBufferCap + 50
	for i := range total {
		k.appendAnnotation(AnnotationRecord{ID: fmt.Sprintf("ann-%d", i)})
	}

	got := k.RecentAnnotations(0)
	if len(got) != annotationBufferCap {
		t.Errorf("ring buffer len = %d, want %d (cap)", len(got), annotationBufferCap)
	}
	if got[0].ID != fmt.Sprintf("ann-%d", total-annotationBufferCap) {
		t.Errorf("oldest retained = %q, want ann-%d", got[0].ID, total-annotationBufferCap)
	}
	if got[len(got)-1].ID != fmt.Sprintf("ann-%d", total-1) {
		t.Errorf("newest = %q, want ann-%d", got[len(got)-1].ID, total-1)
	}
}

func TestAppendAnnotation_ConcurrentSafe(t *testing.T) {
	k := &KimiClient{}
	const goroutines = 8
	const perG = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := range goroutines {
		go func(gid int) {
			defer wg.Done()
			for j := range perG {
				k.appendAnnotation(AnnotationRecord{ID: fmt.Sprintf("g%d-%d", gid, j)})
			}
		}(g)
	}
	wg.Wait()

	got := k.RecentAnnotations(0)
	if len(got) != goroutines*perG {
		t.Errorf("concurrent append total = %d, want %d", len(got), goroutines*perG)
	}
}

func TestNextAnnotationID_Unique(t *testing.T) {
	k := &KimiClient{}
	seen := make(map[string]bool, 100)
	for i := range 100 {
		id := k.nextAnnotationID()
		if seen[id] {
			t.Fatalf("duplicate id %q at iteration %d", id, i)
		}
		seen[id] = true
	}
}

func TestNextAnnotationID_Format(t *testing.T) {
	k := &KimiClient{}
	id := k.nextAnnotationID()
	if !strings.HasPrefix(id, "ann-") {
		t.Errorf("id %q missing ann- prefix", id)
	}
	before, after, ok := strings.Cut(id, "-")
	_ = before
	if !ok || after == "" {
		t.Errorf("id %q missing counter suffix", id)
	}
}

func TestRecentAnnotations_DefensiveCopy(t *testing.T) {
	k := &KimiClient{}
	k.appendAnnotation(AnnotationRecord{ID: "ann-1", Tokens: 10})

	got := k.RecentAnnotations(0)
	got[0].Tokens = 999

	again := k.RecentAnnotations(0)
	if again[0].Tokens != 10 {
		t.Errorf("mutating returned slice leaked back to client: tokens = %d", again[0].Tokens)
	}
}
