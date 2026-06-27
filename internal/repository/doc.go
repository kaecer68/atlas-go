// Package repository provides PostgreSQL persistence with dual-write
// coordination for the atlas-go runtime.
//
// DualWriteRepository writes to both JSONL and PostgreSQL backends in
// parallel for data safety during the JSONL→PostgreSQL migration. Reads
// prefer PostgreSQL and fall back to JSONL if PG is unavailable.
//
// Backend modules:
//
//	JSONLRepository     — Per-table stores: alert, metrics, outcome,
//	                     screening_reject, session_summary, human_intervention
//	PostgresRepository  — pgxpool-backed canonical store (PostgreSQL 15+)
//
// Migration semantics:
//   - Write path: both backends receive the write; partial failures are
//     logged but don't fail the caller (eventual consistency via retry)
//   - Read path: PostgreSQL first, JSONL fallback when PG returns error
//   - Once migration is complete, DualWriteRepository will be replaced by
//     PostgresRepository directly (Phase 3 of the migration plan)
//
// Store interfaces follow the dual-write pattern: each domain record type
// (Alert, Metric, Outcome, etc.) has a paired interface in both backends.
//
// Maturity: stable
package repository
