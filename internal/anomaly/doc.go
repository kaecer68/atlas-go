// Package anomaly provides pluggable anomaly detection for MCP audit events.
// Detectors consume normalized audit entries and emit Anomaly values when a
// short-window statistic deviates from a longer baseline.
//
// Maturity: experimental
package anomaly
