// Package liveness provides the task-liveness heartbeat store (Phase 1).
//
// It answers one cross-restart question: "did background task X actually
// succeed recently?" The BackgroundTaskManager only keeps runtime state in
// memory (internal/apigateway/background.go), so every task execution is
// additionally upserted into the `task_liveness` PostgreSQL table (one row
// per task, never appended — see sql/migrations/000016_task_liveness.up.sql).
//
// Components:
//   - Store:             PG upsert (Record) + list (List) + meta-heartbeat.
//   - StalenessMonitor:  periodic scan that alerts via monitor.Alert when a
//     task has not run for > interval x 3 (deduplicated per task).
//   - HandlePing:        internal POST endpoint used by cron containers to
//     report completion with an exit code (shared-secret guarded).
//
// The store is intentionally fire-and-forget from the caller's perspective:
// Record failures are logged, never propagated to the background task loop.
//
// Maturity: experimental
package liveness
