package metrics

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

// DegradedMetrics exposes degraded-mode counters for the SOX limiter.
// It mirrors prometheus.CounterVec semantics using an in-memory counter
// store so it can be used from any package without creating import cycles.
type DegradedMetrics struct {
	DegradedActivations *CounterVec
	ProviderErrors      *CounterVec
}

// CounterVec is a vector of counters keyed by label values.
type CounterVec struct {
	name       string
	labelNames []string
	mu         sync.Mutex
	counters   map[string]*Counter
}

// Counter is a single labeled counter.
type Counter struct {
	name   string
	labels map[string]string
	value  atomic.Int64
}

// Inc increments the counter by one.
func (c *Counter) Inc() {
	c.value.Add(1)
}

// Value returns the current counter value, or 0 if the counter has not
// been recorded yet.
func (c *Counter) Value() float64 {
	return float64(c.value.Load())
}

// WithLabelValues returns the counter for the supplied label values.
// It panics if the number of values does not match the vector's label names.
func (cv *CounterVec) WithLabelValues(values ...string) *Counter {
	if len(values) != len(cv.labelNames) {
		panic(fmt.Sprintf("%s: expected %d label values, got %d", cv.name, len(cv.labelNames), len(values)))
	}
	key := labelValuesKey(values)

	cv.mu.Lock()
	defer cv.mu.Unlock()

	if cv.counters == nil {
		cv.counters = make(map[string]*Counter)
	}
	if counter, ok := cv.counters[key]; ok {
		return counter
	}

	labels := make(map[string]string, len(values))
	for i, v := range values {
		labels[cv.labelNames[i]] = v
	}
	counter := &Counter{
		name:   cv.name,
		labels: labels,
	}
	cv.counters[key] = counter
	return counter
}

func labelValuesKey(values []string) string {
	return strings.Join(values, "\x00")
}

// NewDegradedMetrics creates a new DegradedMetrics instance backed by an
// in-memory counter store.
func NewDegradedMetrics() *DegradedMetrics {
	return &DegradedMetrics{
		DegradedActivations: &CounterVec{
			name:       "atlas_sox_limiter_degraded_activations_total",
			labelNames: []string{"service", "reason"},
		},
		ProviderErrors: &CounterVec{
			name:       "atlas_sox_limiter_provider_errors_total",
			labelNames: []string{"service", "error_type"},
		},
	}
}
