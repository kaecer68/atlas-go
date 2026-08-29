package llm_annotator

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// MetricsRecorder is the minimal interface KimiClient uses to surface
// observability events to the host application. A nil MetricsRecorder is a
// valid no-op so callers that do not need metrics can leave Config.Metrics
// unset.
//
// The interface intentionally mirrors the in-memory MetricsCollector in
// internal/monitoring/metrics.go (Counter + Gauge only). Histograms are
// deliberately omitted because the existing RecordHistogram implementation
// in that package is dead code (no read path). Latency is exposed via
// KimiClient.Latency() instead.
//
// Implementations MUST be safe for concurrent use.
type MetricsRecorder interface {
	RecordCounter(name string, value float64, labels map[string]string)
	RecordGauge(name string, value float64, labels map[string]string)
}

// noopMetrics is a MetricsRecorder that drops every record. It is the
// default when Config.Metrics is nil and is also used by tests that do not
// care about metrics.
type noopMetrics struct{}

func (noopMetrics) RecordCounter(string, float64, map[string]string) {}
func (noopMetrics) RecordGauge(string, float64, map[string]string)   {}

// countingMetrics is a test-only MetricsRecorder that accumulates counter
// and gauge values into in-memory maps. Counters are summed per label set;
// gauges are overwritten (last-write-wins). All operations are goroutine-safe.
//
// Use this in tests to assert that the right metric calls happen with the
// right labels and values.
type countingMetrics struct {
	mu       sync.Mutex
	counters map[string]float64 // key: metricName + sorted labelKey
	gauges   map[string]float64 // key: metricName + sorted labelKey
	calls    map[string]int     // key: metricName + sorted labelKey
}

// newCountingMetrics returns a fresh, empty countingMetrics ready for use.
func newCountingMetrics() *countingMetrics {
	return &countingMetrics{
		counters: make(map[string]float64),
		gauges:   make(map[string]float64),
		calls:    make(map[string]int),
	}
}

func (c *countingMetrics) RecordCounter(name string, value float64, labels map[string]string) {
	key := metricKey(name, labels)
	c.mu.Lock()
	c.counters[key] += value
	c.calls[key]++
	c.mu.Unlock()
}

func (c *countingMetrics) RecordGauge(name string, value float64, labels map[string]string) {
	key := metricKey(name, labels)
	c.mu.Lock()
	c.gauges[key] = value
	c.calls[key]++
	c.mu.Unlock()
}

// CounterValue returns the cumulative counter value for the given metric
// name + labels, or 0 if no such call was recorded.
func (c *countingMetrics) CounterValue(name string, labels map[string]string) float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counters[metricKey(name, labels)]
}

// GaugeValue returns the last gauge value for the given metric name +
// labels, or 0 if no such call was recorded.
func (c *countingMetrics) GaugeValue(name string, labels map[string]string) float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gauges[metricKey(name, labels)]
}

// CallCount returns the number of times RecordCounter or RecordGauge was
// invoked for the given metric name + labels.
func (c *countingMetrics) CallCount(name string, labels map[string]string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls[metricKey(name, labels)]
}

// TotalCalls returns the sum of all counter+gauge invocations across every
// metric name. Useful for asserting "no metrics were recorded" without
// caring about specific names.
func (c *countingMetrics) TotalCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	total := 0
	for _, v := range c.calls {
		total += v
	}
	return total
}

// metricKey builds a stable map key from a metric name + labels. Label keys
// are sorted alphabetically so the same logical label set always produces
// the same key regardless of map iteration order.
func metricKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sortStrings(keys)
	var out strings.Builder
	out.WriteString(name)
	for _, k := range keys {
		out.WriteString("\x00" + k + "=" + labels[k])
	}
	return out.String()
}

// sortStrings sorts a slice of strings in ascending order in place.
// Extracted so this package does not need to import "sort" just for tests.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// FeatureCost is the cost breakdown for a single per-feature label.
// Requests is the number of Annotate calls that contributed to the
// token count; Cost is in USD.
type FeatureCost struct {
	Label    string  `json:"label"`
	Tokens   int64   `json:"tokens"`
	Requests int64   `json:"requests"`
	Cost     float64 `json:"cost"`
}

// CostReport is a point-in-time view of a KimiClient's LLM cost. It
// aggregates the global Usage plus the per-label breakdown. CostPer1kTokens
// is the USD price per 1,000 tokens (e.g. 0.001 = $0.001/1k). The caller
// supplies the rate so the report does not embed a fixed pricing model.
//
// Cost = tokens * CostPer1kTokens / 1000.
//
// This is a reporting-only helper; it does not feed back into runtime
// decisions. LatencyMillis reflects the most recent Annotate call's
// wall-clock duration (same as Snapshot().Latency).
type CostReport struct {
	Provider        string                 `json:"provider"`
	CostPer1kTokens float64                `json:"cost_per_1k_tokens"`
	TotalTokens     int64                  `json:"total_tokens"`
	TotalRequests   int64                  `json:"total_requests"`
	TotalCost       float64                `json:"total_cost"`
	ByFeature       map[string]FeatureCost `json:"by_feature"`
	LatencyMillis   int64                  `json:"latency_ms"`
	GeneratedAt     time.Time              `json:"generated_at"`
}

// CostReport returns a CostReport computed from the current state of the
// client. Safe to call from any goroutine. The costPer1kTokens parameter
// is the USD price per 1,000 tokens; pass 0 to compute token counts only
// without a USD total.
func (k *KimiClient) CostReport(costPer1kTokens float64) CostReport {
	k.usageMu.RLock()
	byFeature := make(map[string]FeatureCost, len(k.usageByLabel))
	for label, u := range k.usageByLabel {
		byFeature[label] = FeatureCost{
			Label:    label,
			Tokens:   u.TotalTokens,
			Requests: u.Requests,
			Cost:     float64(u.TotalTokens) * costPer1kTokens / 1000.0,
		}
	}
	totalUsage := k.usage
	k.usageMu.RUnlock()

	return CostReport{
		Provider:        k.Name(),
		CostPer1kTokens: costPer1kTokens,
		TotalTokens:     totalUsage.TotalTokens,
		TotalRequests:   totalUsage.Requests,
		TotalCost:       float64(totalUsage.TotalTokens) * costPer1kTokens / 1000.0,
		ByFeature:       byFeature,
		LatencyMillis:   k.Latency().Milliseconds(),
		GeneratedAt:     time.Now(),
	}
}

// AnnotationRecord is the per-call trace for cost-per-annotation reporting.
// ID is a unique call identifier (timestamp + counter). Label is the
// FailureContext.Label if set; empty for unlabeled calls. Tokens is the
// Usage.TotalTokens for the call (0 for failures / cache hits). Outcome
// is the final outcome label from recordOutcome (success, cache_hit,
// rate_limited, etc.). LatencyMs is the call's wall-clock duration.
//
// Records are kept in an in-memory ring buffer (cap: 1000) and surfaced via
// RecentAnnotations so callers can join per-call cost with downstream
// signals without paying for a persistent store.
type AnnotationRecord struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Label     string    `json:"label"`
	Tokens    int64     `json:"tokens"`
	Outcome   string    `json:"outcome"`
	LatencyMs int64     `json:"latency_ms"`
}

// RecentAnnotations returns the most recent n records (oldest first within
// the returned slice). Pass n <= 0 to get all retained records. Safe to
// call from any goroutine. The returned slice is a defensive copy.
func (k *KimiClient) RecentAnnotations(n int) []AnnotationRecord {
	k.annotationMu.Lock()
	defer k.annotationMu.Unlock()
	size := len(k.recentAnnotations)
	if n <= 0 || n > size {
		n = size
	}
	out := make([]AnnotationRecord, n)
	copy(out, k.recentAnnotations[size-n:])
	return out
}

const annotationBufferCap = 1000

func (k *KimiClient) appendAnnotation(rec AnnotationRecord) {
	k.annotationMu.Lock()
	k.recentAnnotations = append(k.recentAnnotations, rec)
	if len(k.recentAnnotations) > annotationBufferCap {
		k.recentAnnotations = k.recentAnnotations[len(k.recentAnnotations)-annotationBufferCap:]
	}
	store := k.annotationStore
	k.annotationMu.Unlock()

	if store != nil {
		_ = store.Write(rec)
	}
}

// SetAnnotationStore wires an AnnotationStore for durable persistence.
// A nil store disables persistence. Persistence failures are swallowed
// (the in-memory ring buffer remains the source of truth for runtime
// decisions); callers can monitor disk health via the store's Close error.
func (k *KimiClient) SetAnnotationStore(s AnnotationStore) {
	k.annotationMu.Lock()
	k.annotationStore = s
	k.annotationMu.Unlock()
}

func (k *KimiClient) nextAnnotationID() string {
	k.annotationMu.Lock()
	k.annotationCounter++
	id := fmt.Sprintf("ann-%d-%d", time.Now().UnixNano(), k.annotationCounter)
	k.annotationMu.Unlock()
	return id
}
