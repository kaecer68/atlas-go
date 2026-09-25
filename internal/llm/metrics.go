package llm

// Router metrics contract (issue #1926).
//
// Background: DefaultRouter has always counted fallbacks in the package-level
// int64 counters FallbackTriggeredTotal / BackupChainExhaustedTotal (router.go),
// but those counters have no metric name, no labels and no reader outside
// router_test.go — so a fallback storm was invisible on /metrics.
//
// This file defines the producer side of an injected emitter. The router does
// NOT import a Prometheus client and does NOT import internal/monitoring:
// internal/llm stays free of the monitoring dependency and the host application
// decides where the values go.

// MetricsRecorder is the minimal observability interface consumed by
// DefaultRouter. The shape deliberately mirrors the two existing implementations
// of the same idea in this tree so a single host object can satisfy all three:
//
//   - internal/monitoring.MetricsCollector (the collector behind /metrics)
//   - internal/llm/clients.MetricsRecorder
//   - internal/llm_annotator.MetricsRecorder
//
// Counter + Gauge only: the router emits counters today, and keeping Gauge
// avoids a second interface when provider-health observation lands
// (see docs/specs/llm-routing-spec.md §6.5). Implementations MUST be safe for
// concurrent use: Call may run from many goroutines.
//
// A nil MetricsRecorder is a valid "no metrics" configuration; the router
// treats it as a no-op (see DefaultRouter.recordCounter).
type MetricsRecorder interface {
	RecordCounter(name string, value float64, labels map[string]string)
	RecordGauge(name string, value float64, labels map[string]string)
}

// NoOpMetrics is a MetricsRecorder that discards every record. It is the
// explicit equivalent of injecting nothing, and is useful for callers that
// want a non-nil placeholder.
type NoOpMetrics struct{}

// RecordCounter implements MetricsRecorder.
func (NoOpMetrics) RecordCounter(string, float64, map[string]string) {}

// RecordGauge implements MetricsRecorder.
func (NoOpMetrics) RecordGauge(string, float64, map[string]string) {}

// Prometheus metric names emitted by DefaultRouter.
//
// Naming follows the LLM-domain metrics already emitted through an injected
// recorder: `llm_annotator_requests_total` (internal/llm_annotator) and
// `llm_client_requests_total` (internal/llm/clients). The `atlas_` prefix used
// by internal/monitoring/startup_metrics.go is for metrics owned by the
// monitoring package itself, so it is intentionally not applied here.
// `_total` marks a counter; both metrics below are monotonically increasing.
const (
	// MetricRouterFallbackTriggered counts every invocation of a NON-primary
	// routing-chain member, i.e. a fallback that really happened.
	//
	// Labels:
	//   capability    (required) — the capability whose chain was walked
	//   to_provider   (required) — the chain member about to be called
	//   from_provider (optional) — the chain member that just failed; the key
	//                              is ABSENT when no member was invoked before
	//                              this point (primary skipped because it is
	//                              unregistered or does not Support the
	//                              capability). See fallbackLabels.
	MetricRouterFallbackTriggered = "llm_router_fallback_triggered_total"

	// MetricRouterBackupChainExhausted counts requests where every available
	// chain member failed and the last-resort handler answered instead.
	//
	// Labels:
	//   capability (required) — the exhausted chain's capability
	//
	// Deliberately no provider label: exhaustion means "no provider served this
	// request", so a provider dimension would be empty or misleading. The
	// per-provider breakdown is already available on
	// MetricRouterFallbackTriggered{to_provider=...}.
	MetricRouterBackupChainExhausted = "llm_router_backup_chain_exhausted_total"
)

// Label keys used by the router metrics above. Values are bounded sets
// (12 capabilities, 6 provider constants) so cardinality stays small.
const (
	metricLabelCapability   = "capability"
	metricLabelToProvider   = "to_provider"
	metricLabelFromProvider = "from_provider"
)

// fallbackLabels builds the label set for MetricRouterFallbackTriggered.
//
// capability and toProvider are always present. fromProvider is the provider
// that just failed — the last entry of attempted — and is recorded only when
// at least one chain member was actually invoked and failed.
//
// When the router reaches a backup because the primary was *skipped* (not
// registered, or it does not Support the capability), no provider failed and
// attempted is empty. In that case the from_provider key is omitted rather than
// filled with a placeholder: `from_provider="unknown"` would claim a failure
// happened whose subject is merely unknown, which is a different and wrong
// story. Absence of the key is the accurate signal "no provider failed".
// Consequence for queries: use sum by (capability) or
// sum without (from_provider), since the two label shapes are distinct series.
func fallbackLabels(capability Capability, toProvider Provider, attempted []Provider) map[string]string {
	labels := map[string]string{
		metricLabelCapability: string(capability),
		metricLabelToProvider: string(toProvider),
	}
	if n := len(attempted); n > 0 {
		labels[metricLabelFromProvider] = string(attempted[n-1])
	}
	return labels
}
