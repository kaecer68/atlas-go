package llm_annotator

import (
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
	for i := 0; i < 7; i++ {
		m.RecordCounter("x", 1, map[string]string{"k": "v"})
	}
	for i := 0; i < 3; i++ {
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
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perG; j++ {
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
