// Package liveness provides the task-liveness heartbeat store (Phase 1).
//
// It answers one cross-restart question: "did background task X actually
// succeed recently?" The BackgroundTaskManager only keeps runtime state in
// memory (internal/apigateway/background.go), so every task execution is
// additionally upserted into the `task_liveness` PostgreSQL table (one row
// per task, never appended — see sql/migrations/000016_task_liveness.up.sql).
//
// Components:
//   - Store:        PG upsert (Record) + list (List) + meta-heartbeat.
//   - StalenessMonitor: periodic scan that alerts via monitor.Alert when a
//     task has not run for > interval x 3 (deduplicated per task).
//   - HandlePing:   internal POST endpoint used by cron containers to report
//     completion with an exit code (shared-secret guarded).
//
// The store is intentionally fire-and-forget from the caller's perspective:
// Record failures are logged, never propagated to the background task loop.
package liveness

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ dbConn = (*pgxpool.Pool)(nil)

// RecordInput describes one completed task execution.
type RecordInput struct {
	// TaskName is the stable task identifier (BTM task name or cron task name).
	TaskName string
	// Err is nil for a successful run; any error marks the run as failed and
	// increments consecutive_failures.
	Err error
	// Duration is the wall-clock duration of the run (0 when unknown, e.g.
	// cron pings without a start timestamp).
	Duration time.Duration
}

// Row is one task_liveness row as stored.
type Row struct {
	TaskName            string
	LastRunAt           time.Time
	LastSuccessAt       time.Time // zero when the task has never succeeded
	LastError           string
	ConsecutiveFailures int
	LastDurationMs      int64
	UpdatedAt           time.Time
}

// execer is the minimal pgx surface used by Store for writes, so tests can
// substitute a fake without a live PostgreSQL. *pgxpool.Pool satisfies it.
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// querier is the minimal pgx surface used by Store for reads.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// Store upserts task-liveness heartbeats into PostgreSQL and tracks its own
// write activity for the meta-heartbeat (a writer that silently stops writing
// must be detectable: see StartMetaHeartbeat).
type Store struct {
	db          dbConn
	now         func() time.Time
	lastWriteMu sync.Mutex
	lastWriteAt time.Time
	lastWarnMu  sync.Mutex
	lastWarnAt  time.Time
}

// dbConn is the combined surface NewStore requires from a pgxpool.Pool.
type dbConn interface {
	execer
	querier
}
