// Package reconcile provides one-shot repair tooling for session-summary
// dual-write drift (Phase B6): comparing the PostgreSQL session_summaries
// table against the JSONL ledger (sessions/<id>/summary.json) and backfilling
// one-sided gaps. cmd/reconcile-sessions is the CLI wrapper.
//
// Rules of thumb:
//   - Default is dry-run: nothing is written until Apply is set.
//   - Backfill only repairs session-ID gaps; content conflicts between the
//     two sides are reported and never auto-overwritten.
//   - Idempotent: re-running after convergence is a no-op.
//
// Do not import from hot runtime paths. This package is for operational
// reconciliation, not for production execution (the runtime read path lives
// in internal/repository DualWriteRepository).
//
// Maturity: utility
package reconcile
