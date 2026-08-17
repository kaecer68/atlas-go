-- 000016_task_liveness.up.sql
-- Task Liveness heartbeat table (Phase 1, task-liveness-heartbeat).
--
-- One row per task (task_name PK), upsert-only: a task's last execution
-- outcome is *overwritten* on every run, never appended. The row survives
-- process restarts so operators can answer "did task X actually succeed
-- recently?" across restarts — the runtime /api/scheduler/status only holds
-- in-memory state.
--
-- Semantics of each column (written by internal/liveness.Store):
--   last_run_at          last execution attempt (success OR failure), UTC
--   last_success_at      last successful execution (NULL until first success)
--   last_error           error text of the latest failed run ('' when ok)
--   consecutive_failures running count of failures since the last success
--   last_duration_ms     wall-clock duration of the last run
--   updated_at           same as last_run_at for the row write
--
-- Staleness is NOT stored here: interval is a runtime property of the
-- BackgroundTaskManager, so "overdue (last_run > interval x 3)" is computed
-- at read time by the API (internal/monitoring/api/dashboard) using the
-- runtime interval. Cron-container pings (POST /api/internal/task-liveness)
-- write rows here too; they have no BTM interval and are reported with
-- source="cron".
CREATE TABLE IF NOT EXISTS task_liveness (
    task_name           TEXT PRIMARY KEY,
    last_run_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_success_at     TIMESTAMPTZ,
    last_error          TEXT NOT NULL DEFAULT '',
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    last_duration_ms    BIGINT NOT NULL DEFAULT 0,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_task_liveness_last_run_at ON task_liveness (last_run_at DESC);
