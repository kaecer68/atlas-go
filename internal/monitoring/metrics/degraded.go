package metrics

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// OnInc is called whenever a DegradedMetrics counter is incremented.
// counterName is either "degraded_activations" or "provider_errors".
// labels are supplied in the same order as the counter vector's label names.
type OnInc func(counterName string, labels []string, value float64)

// DegradedMetrics exposes degraded-mode counters for the SOX limiter.
// It mirrors prometheus.CounterVec semantics using an in-memory counter
// store so it can be used from any package without creating import cycles.
type DegradedMetrics struct {
	DegradedActivations *CounterVec
	ProviderErrors      *CounterVec
	onInc               OnInc
}

// SetOnInc installs a callback that is invoked on every counter increment.
// Calling SetOnInc multiple times replaces the previous callback.
func (d *DegradedMetrics) SetOnInc(fn OnInc) {
	d.onInc = fn
	if d.DegradedActivations != nil {
		d.DegradedActivations.OnInc = func(_ string, labels map[string]string, value float64) {
			if d.onInc != nil {
				d.onInc("degraded_activations", orderedLabelValues(labels, d.DegradedActivations.labelNames), value)
			}
		}
	}
	if d.ProviderErrors != nil {
		d.ProviderErrors.OnInc = func(_ string, labels map[string]string, value float64) {
			if d.onInc != nil {
				d.onInc("provider_errors", orderedLabelValues(labels, d.ProviderErrors.labelNames), value)
			}
		}
	}
}

// CounterVec is a vector of counters keyed by label values.
type CounterVec struct {
	name       string
	labelNames []string
	OnInc      func(name string, labels map[string]string, value float64)
	mu         sync.Mutex
	counters   map[string]*Counter
}

// Counter is a single labeled counter.
type Counter struct {
	name   string
	labels map[string]string
	value  atomic.Int64
	vec    *CounterVec
}

// Inc increments the counter by one.
func (c *Counter) Inc() {
	c.value.Add(1)
	if c.vec != nil && c.vec.OnInc != nil {
		c.vec.OnInc(c.name, c.labels, c.Value())
	}
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
		vec:    cv,
	}
	cv.counters[key] = counter
	return counter
}

func labelValuesKey(values []string) string {
	return strings.Join(values, "\x00")
}

// labelKey returns a deterministic string key for a label map by sorting the
// label names and joining each name/value pair.
func labelKey(labels map[string]string) string {
	names := make([]string, 0, len(labels))
	for n := range labels {
		names = append(names, n)
	}
	sort.Strings(names)

	parts := make([]string, 0, len(names)*2)
	for _, n := range names {
		parts = append(parts, n, labels[n])
	}
	return strings.Join(parts, "\x00")
}

func orderedLabelValues(labels map[string]string, names []string) []string {
	values := make([]string, len(names))
	for i, n := range names {
		values[i] = labels[n]
	}
	return values
}

func (cv *CounterVec) snapshotEntries() []map[string]any {
	cv.mu.Lock()
	defer cv.mu.Unlock()

	type entry struct {
		labels map[string]string
		value  float64
	}

	entries := make([]entry, 0, len(cv.counters))
	for _, c := range cv.counters {
		entries = append(entries, entry{labels: c.labels, value: c.Value()})
	}

	sort.Slice(entries, func(i, j int) bool {
		return labelKey(entries[i].labels) < labelKey(entries[j].labels)
	})

	result := make([]map[string]any, len(entries))
	for i, e := range entries {
		entry := make(map[string]any, len(e.labels)+1)
		for k, v := range e.labels {
			entry[k] = v
		}
		entry["value"] = e.value
		result[i] = entry
	}
	return result
}

// Snapshot returns the current values of all degraded counters.
func (m *DegradedMetrics) Snapshot() map[string][]map[string]any {
	return map[string][]map[string]any{
		"degraded_activations": m.DegradedActivations.snapshotEntries(),
		"provider_errors":      m.ProviderErrors.snapshotEntries(),
	}
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
