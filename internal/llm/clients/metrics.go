package clients

// MetricsRecorder is the minimal observability interface consumed by
// BaseClient. It mirrors Counter + Gauge — the two metric types used by
// the existing KimiClient. Implementations must be safe for concurrent use.
type MetricsRecorder interface {
	RecordCounter(name string, value float64, labels map[string]string)
	RecordGauge(name string, value float64, labels map[string]string)
}

// NoOpMetrics is a MetricsRecorder that silently discards all metrics.
// Use it when metrics collection is not needed (e.g., tests).
type NoOpMetrics struct{}

func (NoOpMetrics) RecordCounter(string, float64, map[string]string) {}
func (NoOpMetrics) RecordGauge(string, float64, map[string]string)   {}
