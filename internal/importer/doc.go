// Package importer provides CSV to JSONL data import for TWSE OpenAPI and
// FinMind historical datasets.
//
// importer is used by:
//   - scripts/import-replay/ for initial replay dataset setup
//   - cmd/run-replay-import for one-shot CSV→JSONL conversion
//
// Conversion semantics:
//   - One CSV row → one JSONL line (NDJSON format, NOT JSON array)
//   - Timestamp fields are normalized to RFC3339 UTC
//   - Missing fields are emitted as null (NOT omitted) to preserve
//     schema stability for downstream readers
//   - Numeric fields with units (e.g. "1,234.56") are stripped before parsing
//
// The output JSONL matches the format consumed by internal/ledger and
// internal/replay so importers can be chained (CSV → JSONL → ledger).
//
// Maturity: utility
package importer
