package service

import (
	"time"
)

// TrendPoint represents a single point in a metric trend
type TrendPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// MetricsSnapshot represents a point-in-time snapshot of all metrics
type MetricsSnapshot struct {
	ScreeningTotal     int64            `json:"screening_total"`
	ScreeningPassed    int64            `json:"screening_passed"`
	ScreeningRate      float64          `json:"screening_rate"`
	AlertsTriggered    int64            `json:"alerts_triggered"`
	AlertsAcknowledged int64            `json:"alerts_acknowledged"`
	AlertsByType       map[string]int64 `json:"alerts_by_type"`
	Timestamp          time.Time        `json:"timestamp"`
}

// CheckStatus represents the status of a quality check
type CheckStatus string

const (
	StatusOK       CheckStatus = "ok"
	StatusWarning  CheckStatus = "warning"
	StatusCritical CheckStatus = "critical"
	StatusSkipped  CheckStatus = "skipped"
)

// DataQualityCheck represents a single data quality check result
type DataQualityCheck struct {
	Name      string         `json:"name"`
	Status    CheckStatus    `json:"status"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	CheckedAt time.Time      `json:"checked_at"`
	Duration  time.Duration  `json:"duration_ms"`
}

// DataQualityReport is the overall data quality report
type DataQualityReport struct {
	Checks      []DataQualityCheck `json:"checks"`
	Overall     CheckStatus        `json:"overall"`
	Score       float64            `json:"score"`
	GeneratedAt time.Time          `json:"generated_at"`
}

// AlertThreshold mirrors monitoring.AlertThreshold to break the service<->monitoring import cycle.
type AlertThreshold struct {
	MinScreeningRate        float64 `json:"min_screening_rate"`
	MaxAlertTriggerRate     float64 `json:"max_alert_trigger_rate"`
	MaxUnacknowledgedAlerts int64   `json:"max_unacknowledged_alerts"`
}

// ThresholdViolation mirrors monitoring.ThresholdViolation for the same reason.
type ThresholdViolation struct {
	Metric    string  `json:"metric"`
	Current   float64 `json:"current"`
	Threshold float64 `json:"threshold"`
	Severity  string  `json:"severity"`
	Message   string  `json:"message"`
}

// ThresholdReport bundles violations with the threshold that produced them and the check time.
type ThresholdReport struct {
	Violations []ThresholdViolation `json:"violations"`
	Count      int                  `json:"count"`
	Threshold  AlertThreshold       `json:"threshold"`
	CheckedAt  time.Time            `json:"checked_at"`
}
