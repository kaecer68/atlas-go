// Package anomaly provides pluggable anomaly detection for MCP audit events.
// Detectors consume normalized audit entries and emit Anomaly values when a
// short-window statistic deviates from a longer baseline. The package is
// intentionally decoupled from cmd/atlas-mcp/server so detectors can be
// unit-tested and evolved without importing a main package.
//
// Maturity: experimental
package anomaly

import (
	"context"
	"time"
)

// AuditEntryV2 is the read model consumed by anomaly detectors. It mirrors
// cmd/atlas-mcp/server.AuditEntryV2 so the server package can translate audit
// records without anomaly depending on server.
type AuditEntryV2 struct {
	SchemaVersion int    `json:"schema_version,omitempty"`
	TS            string `json:"ts"`
	SessionID     string `json:"session_id,omitempty"`
	AgentID       string `json:"agent_id,omitempty"`
	Tool          string `json:"tool"`
	ArgsHash      string `json:"args_hash,omitempty"`
	Status        string `json:"status"`
	LatencyMS     int64  `json:"latency_ms"`
	DurationMS    int64  `json:"duration_ms,omitempty"`
	Transport     string `json:"transport,omitempty"`
	Error         string `json:"error,omitempty"`
}

// Detector is the anomaly detector abstraction. Each detector focuses on one
// baseline comparison (e.g., 5-minute vs 24-hour call volume).
type Detector interface {
	// Name returns the detector identifier, e.g. "baseline_5m_24h".
	Name() string

	// Detect computes anomalies from a batch of audit entries. An empty slice
	// means no anomaly was found. The detector must not mutate entries.
	Detect(ctx context.Context, entries []AuditEntryV2) ([]Anomaly, error)
}

// Anomaly is a single detected anomaly emitted by a detector.
type Anomaly struct {
	Type       string    `json:"type"`
	TenantID   string    `json:"tenant_id,omitempty"`
	Tool       string    `json:"tool,omitempty"`
	Score      float64   `json:"score"`
	DetectedAt time.Time `json:"detected_at"`
	Baseline   Baseline  `json:"baseline"`
	Current    Current   `json:"current"`
}

// Baseline holds long-window statistics used as the reference distribution.
type Baseline struct {
	WindowMin int     `json:"window_min"`
	Median    float64 `json:"median"`
	StdDev    float64 `json:"std_dev"`
	SampleN   int     `json:"sample_n"`
}

// Current holds short-window statistics compared against the baseline.
type Current struct {
	WindowMin int     `json:"window_min"`
	Median    float64 `json:"median"`
	SampleN   int     `json:"sample_n"`
}

// Config holds tunable parameters for anomaly detection.
type Config struct {
	DetectIntervalSec  int
	BaselineWindowMin  int
	CurrentWindowMin   int
	ZScoreThreshold    float64
	MinBaselineSamples int
}

// DefaultConfig returns production defaults for the anomaly detector.
func DefaultConfig() Config {
	return Config{
		DetectIntervalSec:  60,
		BaselineWindowMin:  1440,
		CurrentWindowMin:   5,
		ZScoreThreshold:    2.5,
		MinBaselineSamples: 30,
	}
}

// ZScoreFunc computes a z-score from baseline and current statistics.
// T1.2 intentionally stubs the real rolling-window math; T1.3 will swap the
// implementation without changing detector signatures.
type ZScoreFunc func(baseline Baseline, current Current) float64

// DefaultZScoreFunc is the T1.2 stub. It returns 0 when samples are
// insufficient or baseline standard deviation is zero; otherwise it returns
// the median difference normalized by the baseline standard deviation.
func DefaultZScoreFunc() ZScoreFunc {
	return func(baseline Baseline, current Current) float64 {
		if baseline.SampleN < 2 || current.SampleN < 1 {
			return 0
		}
		if baseline.StdDev == 0 {
			return 0
		}
		diff := current.Median - baseline.Median
		return diff / baseline.StdDev
	}
}

// FilterEntries returns entries whose timestamps fall within [since, until].
// Entries with missing or unparseable timestamps are excluded. A zero since
// means "from the beginning"; a zero until means "up to now".
func FilterEntries(entries []AuditEntryV2, since, until time.Time) []AuditEntryV2 {
	if until.IsZero() {
		until = time.Now()
	}
	out := make([]AuditEntryV2, 0, len(entries))
	for _, e := range entries {
		ts := parseTS(e.TS)
		if ts.IsZero() {
			continue
		}
		if !since.IsZero() && ts.Before(since) {
			continue
		}
		if ts.After(until) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func parseTS(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}
