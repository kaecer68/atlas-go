package liveness

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// DefaultMetaHeartbeatWarnAfter is how long the writer may stay silent before
// the meta-heartbeat logs a WARN ("liveness writer may be dead").
const DefaultMetaHeartbeatWarnAfter = 15 * time.Minute

// metaHeartbeatCheckEvery is how often the meta-heartbeat goroutine inspects
// lastWriteAt. Deliberately much smaller than the warn window so a dead
// writer is detected within a minute of the window elapsing.
const metaHeartbeatCheckEvery = 1 * time.Minute

const (
	upsertSQL = `
INSERT INTO task_liveness
    (task_name, last_run_at, last_success_at, last_error, consecutive_failures, last_duration_ms, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $2)
ON CONFLICT (task_name) DO UPDATE SET
    last_run_at = EXCLUDED.last_run_at,
    last_success_at = COALESCE(EXCLUDED.last_success_at, task_liveness.last_success_at),
    last_error = EXCLUDED.last_error,
    consecutive_failures = CASE
        WHEN EXCLUDED.last_success_at IS NOT NULL THEN 0
        ELSE task_liveness.consecutive_failures + 1
    END,
    last_duration_ms = EXCLUDED.last_duration_ms,
    updated_at = EXCLUDED.updated_at
`

	listSQL = `
SELECT task_name, last_run_at, last_success_at, last_error, consecutive_failures, last_duration_ms, updated_at
FROM task_liveness
ORDER BY task_name
`
)

// NewStore creates a liveness Store backed by the given pool. now is a
// clock injection point for tests; pass nil to use time.Now.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{db: pool, now: time.Now}
}

// setNow overrides the clock (tests).
func (s *Store) setNow(f func() time.Time) {
	s.now = f
}

// newStoreWithExec builds a Store over an arbitrary dbConn (test fakes; the
// read half may be a stub).
func newStoreWithExec(conn dbConn) *Store {
	return &Store{db: conn, now: time.Now}
}

// Record upserts one task execution outcome. It is fire-and-forget from the
// caller's perspective: the returned error is only for logging — the BTM
// completion hook must never let a liveness write failure affect the main
// task loop.
func (s *Store) Record(ctx context.Context, in RecordInput) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("liveness store: not configured")
	}
	now := s.now().UTC()
	failed := in.Err != nil
	var successAt any
	if !failed {
		successAt = now
	}
	errMsg := ""
	if failed {
		errMsg = in.Err.Error()
	}
	durMs := int64(0)
	if in.Duration > 0 {
		durMs = in.Duration.Milliseconds()
	}
	// Seed consecutive_failures for the INSERT branch (a fresh row starting
	// with a failure must read 1, not 0). On conflict the DO UPDATE branch
	// overrides this with old+1 (failure) or 0 (success).
	seedFailures := 0
	if failed {
		seedFailures = 1
	}
	_, err := s.db.Exec(ctx, upsertSQL,
		in.TaskName, now, successAt, errMsg, seedFailures, durMs,
	)
	if err != nil {
		return fmt.Errorf("liveness upsert %s: %w", in.TaskName, err)
	}
	s.markWrite(now)
	return nil
}

// markWrite records a successful DB write for the meta-heartbeat.
func (s *Store) markWrite(at time.Time) {
	s.lastWriteMu.Lock()
	defer s.lastWriteMu.Unlock()
	s.lastWriteAt = at
}

// lastWrite returns the timestamp of the last successful DB write.
func (s *Store) lastWrite() time.Time {
	s.lastWriteMu.Lock()
	defer s.lastWriteMu.Unlock()
	return s.lastWriteAt
}

// List returns all task_liveness rows ordered by task name.
func (s *Store) List(ctx context.Context) ([]Row, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("liveness store: not configured")
	}
	rows, err := s.db.Query(ctx, listSQL)
	if err != nil {
		return nil, fmt.Errorf("liveness list: %w", err)
	}
	defer rows.Close()

	out := make([]Row, 0, 64)
	for rows.Next() {
		var r Row
		var successAt *time.Time
		if err := rows.Scan(&r.TaskName, &r.LastRunAt, &successAt, &r.LastError,
			&r.ConsecutiveFailures, &r.LastDurationMs, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("liveness scan: %w", err)
		}
		if successAt != nil {
			r.LastSuccessAt = *successAt
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("liveness rows: %w", err)
	}
	return out, nil
}

// StartMetaHeartbeat runs the meta-heartbeat watchdog loop (BLOCKING; the
// caller launches it as a goroutine — cmd/atlas/main.go, so the
// constitution background-task check sees a main-owned goroutine): if the
// store goes N minutes without a successful write (warnAfter; default
// DefaultMetaHeartbeatWarnAfter when <= 0), it logs a WARN — the signal
// that the liveness writer itself may have died silently. At most one WARN
// per warnAfter window. Returns when ctx is cancelled.
func (s *Store) StartMetaHeartbeat(ctx context.Context, warnAfter time.Duration) {
	if s == nil || s.db == nil {
		return
	}
	if warnAfter <= 0 {
		warnAfter = DefaultMetaHeartbeatWarnAfter
	}
	ticker := time.NewTicker(metaHeartbeatCheckEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkMetaHeartbeat(warnAfter)
		}
	}
}

func (s *Store) checkMetaHeartbeat(warnAfter time.Duration) {
	now := s.now()
	silentFor := now.Sub(s.lastWrite())
	if silentFor < warnAfter {
		return
	}
	s.lastWarnMu.Lock()
	defer s.lastWarnMu.Unlock()
	if !s.lastWarnAt.IsZero() && now.Sub(s.lastWarnAt) < warnAfter {
		return // already warned within the current window
	}
	s.lastWarnAt = now
	logging.Warn("liveness", "meta_heartbeat_silent",
		"silent_for", silentFor.Round(time.Second).String(),
		"warn_after", warnAfter.String(),
		"hint", "liveness writer has not written task_liveness; check PG connectivity and BTM completion hook wiring",
	)
}
