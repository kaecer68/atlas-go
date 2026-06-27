// Package observability provides OpenTelemetry tracing initialization and
// helpers for atlas-go's runtime observability.
//
// observability is initialized once in cmd/atlas main() after config load
// but before HTTP server start. It wires:
//   - OTLP exporter (configurable endpoint, default localhost:4317)
//   - Resource attributes (service.name=atlas-go, service.version from VERSION)
//   - Trace batching with configurable flush interval
//
// Tracing helpers:
//   - StartSpan(ctx, name) — wrap operations in spans with consistent attributes
//   - SetError(span, err) — mark span error with structured error message
//   - RecordMetric(name, value, attrs) — counter/gauge recording
//
// Span attribute conventions follow semantic conventions:
//   - atlas.module = "internal/<module>"
//   - atlas.session_id = sessionID when applicable
//   - atlas.regime = current RegimeType (when in simulation context)
//
// Maturity: evolving
package observability
