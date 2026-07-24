// Package eventquality implements the event data quality gate for atlas-go.
//
// Maturity: evolving
//
// Stage 2 (~/workspace/atlas-notes/05-decisions/2026-07-13-stage-2-data-quality.md)
// mandates that any event ingested through a public API or background task must
// pass EventValidator.Validate() before being written to the event calendar.
// Rejected events are recorded in the QualityLog for downstream audit and
// debugging.
//
// The package provides three sub-components:
//   - EventValidator + 5 validation rules (required fields, source marking,
//     date range, confidence, dedup)
//   - QualityLog — JSONL file recording all rejected events
//   - CrossSourceStore — in-memory cross-source verification tracker
//   - SanitizeTitle — title quality checks (HTML, length, all-digits)
//
// Package independence: This package is NOT related to eventbus or eventdriven
// despite the shared \"event\" prefix. It is a data quality gate consumed solely
// by industry.EventCalendar. eventbus is pub/sub infrastructure; eventdriven
// is capital-flow prediction.
package eventquality
