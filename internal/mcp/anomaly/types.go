package anomaly

import "time"

// AnomalyEvent is the structured output of the anomaly detector.
type AnomalyEvent struct {
	TenantID    string  `json:"tenant_id"`
	AnomalyType string  `json:"anomaly_type"`
	Score       float64 `json:"score"`
	TS          string  `json:"ts"`
	Tool        string  `json:"tool,omitempty"`
}

// Entry is the minimal audit event surface consumed by the detector.
// cmd/atlas-mcp/server.AuditEntryV2 implements this interface.
type Entry interface {
	Version() int
	ObservedAt() time.Time
	GetTool() string
	GetTenantID() string
	GetStatus() string
	GetError() string
}

// ScoreRecorder is the metrics hook used to expose anomaly scores.
type ScoreRecorder interface {
	SetAnomalyScore(tenantID, anomalyType string, score float64)
}
