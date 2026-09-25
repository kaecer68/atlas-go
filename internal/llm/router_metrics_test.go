package llm

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
)

// countingMetrics is a test-only MetricsRecorder that accumulates counter and
// gauge values in memory. It mirrors internal/llm_annotator's countingMetrics
// (same key scheme: metric name + sorted label pairs) so both packages assert
// label sets the same way. All methods are goroutine-safe.
type countingMetrics struct {
	mu       sync.Mutex
	counters map[string]float64
	gauges   map[string]float64
	calls    map[string]int
}

func newCountingMetrics() *countingMetrics {
	return &countingMetrics{
		counters: make(map[string]float64),
		gauges:   make(map[string]float64),
		calls:    make(map[string]int),
	}
}

func (c *countingMetrics) RecordCounter(name string, value float64, labels map[string]string) {
	key := metricsTestKey(name, labels)
	c.mu.Lock()
	c.counters[key] += value
	c.calls[key]++
	c.mu.Unlock()
}

func (c *countingMetrics) RecordGauge(name string, value float64, labels map[string]string) {
	key := metricsTestKey(name, labels)
	c.mu.Lock()
	c.gauges[key] = value
	c.calls[key]++
	c.mu.Unlock()
}

// CounterValue returns the accumulated counter value for name + labels.
func (c *countingMetrics) CounterValue(name string, labels map[string]string) float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counters[metricsTestKey(name, labels)]
}

// CallCount returns how many times a counter/gauge was recorded for name+labels.
func (c *countingMetrics) CallCount(name string, labels map[string]string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls[metricsTestKey(name, labels)]
}

// Series returns the sorted keys of every recorded series whose metric name is
// exactly name. Keys embed the label set, so a test can assert that a label key
// is ABSENT (e.g. no from_provider label exists after a skipped primary).
func (c *countingMetrics) Series(name string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for _, m := range []map[string]float64{c.counters, c.gauges} {
		for k := range m {
			if k == name || strings.HasPrefix(k, name+"\x00") {
				out = append(out, k)
			}
		}
	}
	sort.Strings(out)
	return out
}

// TotalCalls counts every counter/gauge invocation across all metric names.
func (c *countingMetrics) TotalCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	total := 0
	for _, v := range c.calls {
		total += v
	}
	return total
}

// metricsTestKey builds a stable key from a metric name and label set; label
// keys are sorted so map iteration order cannot change the key.
func metricsTestKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(name)
	for _, k := range keys {
		b.WriteString("\x00" + k + "=" + labels[k])
	}
	return b.String()
}

// TestDefaultRouter_Metrics_RealFallbackCarriesFromAndToProvider pins the
// happy-path label contract of issue #1926: a chain member that actually failed
// (here: provider error) becomes from_provider, and the member that is about to
// be called becomes to_provider, with the capability always present.
func TestDefaultRouter_Metrics_RealFallbackCarriesFromAndToProvider(t *testing.T) {
	metrics := newCountingMetrics()
	primary := &mockProvider{
		name:       ProviderMiniMax,
		callErr:    errors.New("minimax 503"),
		healthResp: HealthStatus{Provider: ProviderMiniMax, Healthy: true},
	}
	backup := &mockProvider{
		name:       ProviderDeepSeek,
		callResp:   Response{Output: "deepseek answered", Provider: ProviderDeepSeek},
		healthResp: HealthStatus{Provider: ProviderDeepSeek, Healthy: true},
	}
	router := NewDefaultRouter(primary, backup).WithMetrics(metrics)

	if _, err := router.Call(context.Background(), Request{
		Capability: CapabilityFailureAttribution,
		DataClass:  DataClassUnmarked,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := map[string]string{
		metricLabelCapability:   string(CapabilityFailureAttribution),
		metricLabelFromProvider: string(ProviderMiniMax),
		metricLabelToProvider:   string(ProviderDeepSeek),
	}
	if got := metrics.CounterValue(MetricRouterFallbackTriggered, want); got != 1 {
		t.Errorf("CounterValue(%s, %v) = %v, want 1; recorded series: %v",
			MetricRouterFallbackTriggered, want, got, metrics.Series(MetricRouterFallbackTriggered))
	}
	if got := metrics.CallCount(MetricRouterFallbackTriggered, want); got != 1 {
		t.Errorf("CallCount = %d, want 1 (one fallback must produce exactly one record)", got)
	}
	if got := metrics.CounterValue(MetricRouterBackupChainExhausted, map[string]string{
		metricLabelCapability: string(CapabilityFailureAttribution),
	}); got != 0 {
		t.Errorf("backup_chain_exhausted = %v, want 0: the backup answered, the chain was not exhausted", got)
	}
}

// TestDefaultRouter_Metrics_BlankOutputFailureIsAFailureSource verifies the
// ADR-012 blank-output-failure path feeds the same from_provider label as a
// provider error: a silent empty success still names the provider that failed.
func TestDefaultRouter_Metrics_BlankOutputFailureIsAFailureSource(t *testing.T) {
	metrics := newCountingMetrics()
	primary := &mockProvider{
		name:       ProviderMiniMax,
		callResp:   Response{Output: "   \n", Provider: ProviderMiniMax},
		healthResp: HealthStatus{Provider: ProviderMiniMax, Healthy: true},
	}
	backup := &mockProvider{
		name:       ProviderDeepSeek,
		callResp:   Response{Output: "deepseek answered", Provider: ProviderDeepSeek},
		healthResp: HealthStatus{Provider: ProviderDeepSeek, Healthy: true},
	}
	router := NewDefaultRouter(primary, backup).WithMetrics(metrics)

	if _, err := router.Call(context.Background(), Request{
		Capability: CapabilityFailureAttribution,
		DataClass:  DataClassUnmarked,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := map[string]string{
		metricLabelCapability:   string(CapabilityFailureAttribution),
		metricLabelFromProvider: string(ProviderMiniMax),
		metricLabelToProvider:   string(ProviderDeepSeek),
	}
	if got := metrics.CounterValue(MetricRouterFallbackTriggered, want); got != 1 {
		t.Errorf("CounterValue(%s, %v) = %v, want 1; recorded series: %v",
			MetricRouterFallbackTriggered, want, got, metrics.Series(MetricRouterFallbackTriggered))
	}
}

// TestDefaultRouter_Metrics_SkippedPrimaryOmitsFromProvider pins the honest
// labeling decision: an unregistered primary never failed — it was skipped — so
// no from_provider label is emitted (rather than inventing a value). The
// fallback itself is still counted, and nothing else is recorded.
func TestDefaultRouter_Metrics_SkippedPrimaryOmitsFromProvider(t *testing.T) {
	metrics := newCountingMetrics()
	// Only the backup is registered; the chain's primary (kimi) is absent.
	backup := &mockProvider{
		name:       ProviderDeepSeek,
		callResp:   Response{Output: "deepseek answered", Provider: ProviderDeepSeek},
		healthResp: HealthStatus{Provider: ProviderDeepSeek, Healthy: true},
	}
	router := NewDefaultRouter(backup).WithMetrics(metrics)

	if _, err := router.Call(context.Background(), Request{
		Capability: CapabilityPromptLint,
		DataClass:  DataClassNonRegulated,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := map[string]string{
		metricLabelCapability: string(CapabilityPromptLint),
		metricLabelToProvider: string(ProviderDeepSeek),
	}
	if got := metrics.CounterValue(MetricRouterFallbackTriggered, want); got != 1 {
		t.Errorf("CounterValue(%s, %v) = %v, want 1; recorded series: %v",
			MetricRouterFallbackTriggered, want, got, metrics.Series(MetricRouterFallbackTriggered))
	}
	for _, series := range metrics.Series(MetricRouterFallbackTriggered) {
		if strings.Contains(series, metricLabelFromProvider+"=") {
			t.Errorf("series %q carries from_provider, but the skipped primary never failed", series)
		}
	}
	if got := metrics.TotalCalls(); got != 1 {
		t.Errorf("TotalCalls = %d, want 1: the skipped chain member must not be counted", got)
	}
}

// TestDefaultRouter_Metrics_UnsupportedProviderSkippedNotCounted covers the
// second skip reason (registered but does not Support the capability): the skip
// produces no metric record at all, and the real fallback is still labeled.
func TestDefaultRouter_Metrics_UnsupportedProviderSkippedNotCounted(t *testing.T) {
	metrics := newCountingMetrics()
	primary := &mockProvider{
		name:       ProviderKimi,
		supported:  map[Capability]bool{CapabilityCodeReviewAnnotation: true},
		callResp:   Response{Output: "kimi answered", Provider: ProviderKimi},
		healthResp: HealthStatus{Provider: ProviderKimi, Healthy: true},
	}
	backup := &mockProvider{
		name:       ProviderDeepSeek,
		callResp:   Response{Output: "deepseek answered", Provider: ProviderDeepSeek},
		healthResp: HealthStatus{Provider: ProviderDeepSeek, Healthy: true},
	}
	router := NewDefaultRouter(primary, backup).WithMetrics(metrics)

	// failure_attribution is not in the kimi provider's supported set.
	if _, err := router.Call(context.Background(), Request{
		Capability: CapabilityFailureAttribution,
		DataClass:  DataClassUnmarked,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := metrics.TotalCalls(); got != 1 {
		t.Errorf("TotalCalls = %d, want 1 (only the real fallback); series: %v",
			got, metrics.Series(MetricRouterFallbackTriggered))
	}
	want := map[string]string{
		metricLabelCapability: string(CapabilityFailureAttribution),
		metricLabelToProvider: string(ProviderDeepSeek),
	}
	if got := metrics.CounterValue(MetricRouterFallbackTriggered, want); got != 1 {
		t.Errorf("CounterValue(%s, %v) = %v, want 1", MetricRouterFallbackTriggered, want, got)
	}
}

// TestDefaultRouter_Metrics_ExhaustedChainRecordsCapability covers the second
// path required by issue #1926: every chain member fails, the last-resort
// handler answers, and the exhausted counter carries the capability label. The
// fallback counter still moves for the backup invocation, so operators can
// distinguish "fell back once" from "exhausted".
func TestDefaultRouter_Metrics_ExhaustedChainRecordsCapability(t *testing.T) {
	metrics := newCountingMetrics()
	primary := &mockProvider{
		name:       ProviderMiniMax,
		callErr:    errors.New("minimax down"),
		healthResp: HealthStatus{Provider: ProviderMiniMax, Healthy: true},
	}
	backup := &mockProvider{
		name:       ProviderDeepSeek,
		callErr:    errors.New("deepseek down"),
		healthResp: HealthStatus{Provider: ProviderDeepSeek, Healthy: true},
	}
	router := NewDefaultRouter(primary, backup).WithMetrics(metrics)

	resp, err := router.Call(context.Background(), Request{
		Capability: CapabilityFailureAttribution,
		DataClass:  DataClassUnmarked,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Provider != ProviderMock {
		t.Fatalf("expected last-resort ProviderMock, got %v", resp.Provider)
	}

	exhaustedLabels := map[string]string{metricLabelCapability: string(CapabilityFailureAttribution)}
	if got := metrics.CounterValue(MetricRouterBackupChainExhausted, exhaustedLabels); got != 1 {
		t.Errorf("CounterValue(%s, %v) = %v, want 1; series: %v",
			MetricRouterBackupChainExhausted, exhaustedLabels, got, metrics.Series(MetricRouterBackupChainExhausted))
	}
	fallbackLabels := map[string]string{
		metricLabelCapability:   string(CapabilityFailureAttribution),
		metricLabelFromProvider: string(ProviderMiniMax),
		metricLabelToProvider:   string(ProviderDeepSeek),
	}
	if got := metrics.CounterValue(MetricRouterFallbackTriggered, fallbackLabels); got != 1 {
		t.Errorf("CounterValue(%s, %v) = %v, want 1", MetricRouterFallbackTriggered, fallbackLabels, got)
	}
}

// TestDefaultRouter_Metrics_PrimarySuccessRecordsNothing guards against
// over-counting: a request served by the primary is not a fallback and must not
// touch the recorder.
func TestDefaultRouter_Metrics_PrimarySuccessRecordsNothing(t *testing.T) {
	metrics := newCountingMetrics()
	primary := &mockProvider{
		name:       ProviderMiniMax,
		callResp:   Response{Output: "primary answered", Provider: ProviderMiniMax},
		healthResp: HealthStatus{Provider: ProviderMiniMax, Healthy: true},
	}
	router := NewDefaultRouter(primary).WithMetrics(metrics)

	if _, err := router.Call(context.Background(), Request{
		Capability: CapabilityFailureAttribution,
		DataClass:  DataClassUnmarked,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := metrics.TotalCalls(); got != 0 {
		t.Errorf("TotalCalls = %d, want 0 for a primary-only success", got)
	}
}

// TestDefaultRouter_Metrics_NilRecorderIsSafe verifies the "not injected" and
// "explicitly nil" configurations are both no-ops and never panic — the
// behavior every caller that does not care about metrics relies on.
func TestDefaultRouter_Metrics_NilRecorderIsSafe(t *testing.T) {
	buildFailedChain := func() (*DefaultRouter, *mockProvider, *mockProvider) {
		primary := &mockProvider{
			name:       ProviderMiniMax,
			callErr:    errors.New("minimax down"),
			healthResp: HealthStatus{Provider: ProviderMiniMax, Healthy: true},
		}
		backup := &mockProvider{
			name:       ProviderDeepSeek,
			callErr:    errors.New("deepseek down"),
			healthResp: HealthStatus{Provider: ProviderDeepSeek, Healthy: true},
		}
		return NewDefaultRouter(primary, backup), primary, backup
	}

	t.Run("recorder never injected", func(t *testing.T) {
		router, _, _ := buildFailedChain()
		resp, err := router.Call(context.Background(), Request{Capability: CapabilityFailureAttribution})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Provider != ProviderMock {
			t.Errorf("expected last-resort ProviderMock, got %v", resp.Provider)
		}
	})

	t.Run("nil recorder injected", func(t *testing.T) {
		router, _, _ := buildFailedChain()
		router.WithMetrics(nil)
		if _, err := router.Call(context.Background(), Request{Capability: CapabilityFailureAttribution}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// TestNoOpMetrics_DiscardsEverything pins the no-op recorder used by callers
// that want a non-nil sink.
func TestNoOpMetrics_DiscardsEverything(t *testing.T) {
	var recorder MetricsRecorder = NoOpMetrics{}
	recorder.RecordCounter(MetricRouterFallbackTriggered, 1, map[string]string{"capability": "x"})
	recorder.RecordGauge(MetricRouterFallbackTriggered, 1, nil)
}
