package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/reconcile"
	"github.com/kaecer68/atlas-go/internal/repository"
)

// registerSessionSummaryReconcile keeps the flat sessions/<id>/summary.json
// ledger in sync with the PostgreSQL SSoT (#1775).
//
// Root cause of risk_gate_calibrate's "no sessions available": under the
// PG-first store the daily sim writes session summaries to PG only
// (PGFirstOutcomeStore.RecordSessionSummary → PostgresLedgerStore), while
// sessionCalibrationProvider reads flat summary.json files — a B6 split-brain.
// PG held 185 rows; every flat file was missing. The B6 reconcile package
// (built for exactly this divergence) backfills the one-sided gaps.
//
// The JSONL→PG direction also runs (documented B6 trade-off): pre-migration
// JSONL-only sessions flow into PG once, then both sides converge.
func (d calibrationDeps) registerSessionSummaryReconcile() {
	_ = d.TaskMgr.Register(&apigateway.ScheduledTask{
		Name:     "session_summary_reconcile",
		Interval: 12 * time.Hour,
		Enabled:  true,
		Task: func(ctx context.Context) error {
			if d.Cfg.DatabaseURL == "" {
				return fmt.Errorf("session_summary_reconcile: DATABASE_URL not set (non-postgres deployments keep the flat ledger authoritative and do not need this task)")
			}
			runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()

			pool, err := pgxpool.New(runCtx, d.Cfg.DatabaseURL)
			if err != nil {
				return fmt.Errorf("pg pool: %w", err)
			}
			defer pool.Close()
			if err := pool.Ping(runCtx); err != nil {
				return fmt.Errorf("pg ping: %w", err)
			}

			jsonlStore, err := ledger.NewSessionStore(config.Config{LedgerDir: d.Cfg.LedgerDir})
			if err != nil {
				return fmt.Errorf("open ledger: %w", err)
			}
			pgRepo := repository.NewPostgresRepository(pool)

			res, err := reconcile.Run(runCtx, pgRepo, jsonlStore, reconcile.Options{Apply: true})
			if err != nil {
				return fmt.Errorf("reconcile: %w", err)
			}
			logging.Info("session_summary_reconcile",
				"reconcile_done",
				"pg", res.Diff.PGCount,
				"jsonl", res.Diff.JSONLCount,
				"backfilled_to_jsonl", res.BackfilledToJSONL,
				"backfilled_to_pg", res.BackfilledToPG,
				"conflicts", len(res.Diff.Conflicts),
				"errors", len(res.Errors))
			// Per-row backfill failures (e.g. a PG row rejected by the
			// SessionSummary.Validate SSoT guard — session-20260721-daily has
			// PortfolioValue=0) must not fail the task forever: the sync
			// itself converged for every other session and the skipped row is
			// a data-quality problem owned by #1775 follow-up, not a sync
			// error. Run() already logged each row error.
			if len(res.Errors) > 0 {
				logging.Warn("session_summary_reconcile",
					"reconcile_partial",
					"errors", len(res.Errors),
					"first", res.Errors[0])
			}
			return nil
		},
	})
	logging.Info("gateway", "registered_session_summary_reconcile", "interval", "12h")
}
