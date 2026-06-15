// Package backfill provides one-shot repair tools for ledger state that
// drifted from the canonical schema. Each subpackage corresponds to a
// specific drift pattern (orphan summaries, missing tax snapshots, etc.)
// and exposes a testable function used by a cmd/* CLI wrapper.
//
// Rules of thumb:
//   - Functions must be idempotent: re-running must not corrupt state.
//   - Existing files are never overwritten; backfill only fills gaps.
//   - Dry-run mode must be honored by every entry point.
//
// Do not import from hot runtime paths. This package is for operational
// recovery, not for production execution.
//
// Maturity: utility
package backfill
