// Package metrics provides MCP-specific Prometheus metrics for the atlas-mcp server.
//
// It is distinct from internal/monitoring/prometheus.go, which serves
// atlas-go main system metrics. This package uses the official
// prometheus/client_golang library and exposes an /metrics endpoint
// (default 127.0.0.1:9091) separate from atlas-go's main port (8080).
//
// Maturity: experimental
package metrics
