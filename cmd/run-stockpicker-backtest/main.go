// Command run-stockpicker-backtest runs the stockpicker panel backtest +
// win-rate aggregation job after market close.
//
// Usage (from the repo root):
//
//	run-stockpicker-backtest -workdir . -start 2026-01-01 -end 2026-08-26 -asof 2026-08-26
//
// Flags: -workdir, -start, -end, -asof, -force, -dry-run, -universe, -backend,
// -expect-db, -conditions, -list-conditions. The job:
//
//  1. replays the selected conditions (PR 2a configurable engine; default
//     foreign-3d-net-buy, momentum-20d-positive) over point-in-time price bars
//     (backend-aware quotes)
//     and per-symbol T86 flows (data/state/stock_flows/<symbol>.json), and
//  2. writes SignalOutcome rows into stock_signal_outcomes (idempotent:
//     ON CONFLICT DO NOTHING), then aggregates per (symbol, source) into
//     stock_win_rate and data/state/stock_win_rate.json.
//
// Outcomes and win-rate rows always land in a job-local SQLite artifact
// (data/state/atlas.db), never in the postgres target; quotes are
// backend-aware (sqlite | postgres). When the postgres backend is used the
// job prints current_database + inet_server_port at startup and aborts
// before running migrations unless -expect-db matches the actual database
// (M12 target guard).
//
// Same-day rerun skips unless -force is given. No runtime scheduler is wired
// in this PR; the job is CLI-only (scheduling lands in a later PR).
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/db"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

const (
	aggregationWindow         = "120d" // rolling window label
	confidenceLevel           = 0.95   // Wilson interval confidence
	defaultPostgresMigrations = "sql/migrations"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatalf("%v", err)
	}
}

// run parses CLI flags and delegates to runWithPanel. panel==nil builds the
// real point-in-time panel from the workdir (backend-aware quotes + flow files).
func run(args []string) error {
	return runWithPanel(args, nil)
}

// runWithPanel is the testable core. A nil panel builds the real one.
func runWithPanel(args []string, panel stockpicker.PanelSource) error {
	fs := flag.NewFlagSet("run-stockpicker-backtest", flag.ContinueOnError)
	workDir := fs.String("workdir", ".", "atlas work directory (repo root)")
	startStr := fs.String("start", "", "backtest start date YYYY-MM-DD (default: as-of minus 120 days)")
	endStr := fs.String("end", "", "backtest end date YYYY-MM-DD (default: as-of)")
	asofStr := fs.String("asof", "", "point-in-time data cutoff YYYY-MM-DD (default: today UTC); any bar after this fails the run")
	force := fs.Bool("force", false, "rerun even when outcomes already exist for the date range")
	dryRun := fs.Bool("dry-run", false, "compute coverage and print it without persisting")
	universe := fs.String("universe", "", "comma-separated symbols to scan (default: all symbols present in quotes)")
	backendFlag := fs.String("backend", "", "quote backend: sqlite | postgres (default: ATLAS_STORE_BACKEND env, then job-local sqlite)")
	expectDB := fs.String("expect-db", "", "assert the postgres database name before migrations (e.g. atlas); aborts with the actual current_database on mismatch")
	conditionsFlag := fs.String("conditions", "", "comma-separated condition IDs to run (default: foreign-3d-net-buy,momentum-20d-positive)")
	listConditions := fs.Bool("list-conditions", false, "print the registered conditions and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx := context.Background()

	asOf, err := parseDate(*asofStr, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("parse asof: %w", err)
	}
	start, err := parseDate(*startStr, asOf.AddDate(0, 0, -120))
	if err != nil {
		return fmt.Errorf("parse start: %w", err)
	}
	end, err := parseDate(*endStr, asOf)
	if err != nil {
		return fmt.Errorf("parse end: %w", err)
	}
	if end.Before(start) {
		return fmt.Errorf("end %s before start %s", end.Format("2006-01-02"), start.Format("2006-01-02"))
	}

	backend, err := resolveBackend(*backendFlag)
	if err != nil {
		return err
	}

	// M12 target guard: report the postgres connection target and abort on
	// -expect-db mismatch before any migration can touch the wrong database
	// (yesterday's run migrated atlas_dev from a stray DATABASE_URL).
	if *expectDB != "" && backend != "postgres" {
		return fmt.Errorf("-expect-db %q requires the postgres backend (resolved %s); outcomes are job-local sqlite and need no db guard", *expectDB, backend)
	}
	if backend == "postgres" && *expectDB == "" {
		// Fail loudly before any connection: the guard is only meaningful if it
		// is mandatory. An opt-in guard leaves the M12 failure mode reachable
		// via ATLAS_STORE_BACKEND=postgres + a stray DATABASE_URL.
		return fmt.Errorf("postgres backend requires -expect-db <dbname> to assert the migration target (e.g. -expect-db atlas); re-run with -expect-db to proceed")
	}
	if backend == "postgres" {
		guardCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := guardDatabaseTarget(guardCtx, os.Getenv("DATABASE_URL"), *expectDB); err != nil {
			return err
		}
	}

	// Parameters from the worktree's parameters.json (P0-3/P0-6: cost and
	// min_samples come from config, never hard-coded).
	paramsPath := filepath.Join(*workDir, "configs", "parameters.json")
	if _, err := os.Stat(paramsPath); err != nil {
		return fmt.Errorf("parameters config %s: %w", paramsPath, err)
	}
	params, err := config.LoadParametersConfig(paramsPath)
	if err != nil {
		return fmt.Errorf("load parameters: %w", err)
	}
	costRate := params.Stockpicker.Costs.RoundTripPct.Value
	minSamples := params.Stockpicker.Calibration.MinSamples.Value

	registry := stockpicker.NewDefaultConditionRegistry(&params.Stockpicker.Conditions)
	if *listConditions {
		fmt.Print(listConditionsText(registry))
		return nil
	}
	conds, err := selectConditions(*conditionsFlag, registry)
	if err != nil {
		return err
	}

	outcomeDB, err := openOutcomeDB(*workDir)
	if err != nil {
		return err
	}
	defer func() { _ = outcomeDB.Close() }()

	outStore := stockpicker.NewSignalOutcomeStore(outcomeDB)
	winStore := stockpicker.NewWinRateStore(outcomeDB)

	// Idempotency: same-day rerun skips (autobacktest snapshot_exists_skip
	// pattern). -force bypasses the check.
	if !*force && !*dryRun {
		n, err := countExistingOutcomes(ctx, outcomeDB, start, end)
		if err != nil {
			return fmt.Errorf("idempotency check: %w", err)
		}
		if n > 0 {
			log.Printf("skip: %d stockpicker outcomes already exist for %s..%s (use -force to rerun)", n,
				start.Format("2006-01-02"), end.Format("2006-01-02"))
			return nil
		}
	}

	if panel == nil {
		quoteStore, err := openQuoteStore(ctx, backend, *workDir)
		if err != nil {
			return err
		}
		panel, err = newRealPanel(ctx, quoteStore, *workDir)
		if err != nil {
			return fmt.Errorf("build panel: %w", err)
		}
	}
	symbols := panelSymbols(panel, *universe)

	cfg := stockpicker.BacktestConfig{
		Universe:    symbols,
		Start:       start,
		End:         end,
		AsOf:        asOf,
		ForwardDays: 5,
		CostRate:    costRate,
		Source:      "stockpicker",
	}
	outcomes, err := stockpicker.RunBacktest(ctx, cfg, panel, conds...)
	if err != nil {
		return fmt.Errorf("run backtest: %w", err)
	}

	coverage := stockpicker.BuildCoverage(cfg, outcomes)
	printCoverage(coverage)
	fmt.Printf("conditions: %s\n", strings.Join(conditionIDs(conds), ", "))

	if *dryRun {
		log.Printf("dry-run: nothing persisted; a real run would write outcomes to job-local SQLite %s (non-prod artifact), not to the postgres target",
			filepath.Join(*workDir, "data", "state", "atlas.db"))
		return nil
	}

	if err := outStore.RecordOutcomes(ctx, outcomes); err != nil {
		return fmt.Errorf("record outcomes: %w", err)
	}
	fmt.Printf("recorded %d outcomes into stock_signal_outcomes\n", len(outcomes))

	summaries, err := stockpicker.AggregateFromStore(ctx, outStore, winStore, aggregationWindow, costRate, minSamples, confidenceLevel, asOf)
	if err != nil {
		return fmt.Errorf("aggregate: %w", err)
	}
	statePath := filepath.Join(*workDir, "data", "state", "stock_win_rate.json")
	if err := stockpicker.WriteStateJSON(statePath, summaries, asOf); err != nil {
		return fmt.Errorf("write state json: %w", err)
	}
	eligible := 0
	for _, s := range summaries {
		if s.CalibrationStatus == stockpicker.CalibrationEligible {
			eligible++
		}
	}
	fmt.Printf("aggregated %d keys into stock_win_rate (%d eligible) -> %s\n", len(summaries), eligible, statePath)
	return nil
}

// openOutcomeDB opens the job-local SQLite ledger holding backtest outcomes
// and win-rate rows. Outcomes are a job-local SQLite artifact
// (data/state/atlas.db), never written to the postgres target: quotes are
// backend-aware, outcomes stay local (M4③).
func openOutcomeDB(workDir string) (*sql.DB, error) {
	dbPath := filepath.Join(workDir, "data", "state", "atlas.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir state dir: %w", err)
	}
	log.Printf("outcomes ledger: %s (job-local SQLite artifact; postgres target untouched)", dbPath)
	db, err := ledger.OpenSQLiteDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open ledger db %s: %w", dbPath, err)
	}
	if err := ledger.InitSchema(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return db, nil
}

// openQuoteStore builds a QuoteStore for the requested backend.
func openQuoteStore(ctx context.Context, backend, workDir string) (ledger.QuoteStore, error) {
	switch backend {
	case "postgres":
		return openPostgresQuoteStore(ctx)
	case "sqlite":
		return openSQLiteQuoteStore(workDir)
	default:
		// jsonl resolves through the shared resolver but is not a backend this
		// command can serve — fail loudly instead of silently switching.
		return nil, fmt.Errorf("unknown backend %q (quotes support sqlite | postgres)", backend)
	}
}

func openSQLiteQuoteStore(workDir string) (ledger.QuoteStore, error) {
	dbPath := filepath.Join(workDir, "data", "state", "atlas.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir state dir: %w", err)
	}
	db, err := ledger.OpenSQLiteDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db %s: %w", dbPath, err)
	}
	if err := ledger.InitSchema(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return ledger.NewSQLiteQuoteStore(db), nil
}

func openPostgresQuoteStore(ctx context.Context) (ledger.QuoteStore, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, fmt.Errorf("postgres backend requires DATABASE_URL")
	}
	pool, err := db.Init(ctx, dsn, defaultPostgresMigrations)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	ledger.SetPostgresPool(pool)
	return ledger.NewPostgresQuoteStore(pool), nil
}

// guardDatabaseTarget reports the postgres connection target (M12) and, when
// expectDB is set, aborts on mismatch. It runs before any migration so a
// stray DATABASE_URL (e.g. source .env pointing at atlas_dev) can never
// migrate the wrong database. A read-only connection is used: no migrations,
// no schema writes.
func guardDatabaseTarget(ctx context.Context, dsn, expectDB string) error {
	if dsn == "" {
		return fmt.Errorf("postgres backend requires DATABASE_URL (or use -backend sqlite)")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect for db target check: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping for db target check: %w", err)
	}
	var actual string
	var port int
	if err := pool.QueryRow(ctx, "SELECT current_database(), inet_server_port()").Scan(&actual, &port); err != nil {
		return fmt.Errorf("query db target: %w", err)
	}
	log.Printf("db target: current_database=%s inet_server_port=%d", actual, port)
	if expectDB != "" && actual != expectDB {
		return fmt.Errorf("expected database %q but connected to %q; aborting before migrations (DATABASE_URL target mismatch)", expectDB, actual)
	}
	return nil
}

// resolveBackend picks the quote backend. Explicit -backend wins, then the
// ATLAS_STORE_BACKEND env var, then the job-local sqlite default. It
// delegates normalization and fail-loud validation to the shared ledger
// resolver (WP4) instead of a local DATABASE_URL heuristic (M4②).
func resolveBackend(flagValue string) (string, error) {
	value := flagValue
	if value == "" {
		value = os.Getenv("ATLAS_STORE_BACKEND")
	}
	if value == "" {
		value = "sqlite" // job-local default: outcomes are a local SQLite artifact
	}
	backend, err := ledger.ResolveStoreBackend(value)
	if err != nil {
		return "", err
	}
	if backend == "jsonl" {
		// jsonl resolves through the shared resolver (WP4) but is not a
		// backend this command can serve — quotes read sqlite|postgres and
		// outcomes are a job-local SQLite artifact.
		return "", fmt.Errorf("store backend jsonl is not supported by run-stockpicker-backtest (quotes: sqlite | postgres; outcomes: job-local sqlite)")
	}
	return backend, nil
}

// parseDate parses YYYY-MM-DD; empty falls back to def (UTC, date only).
func parseDate(s string, def time.Time) (time.Time, error) {
	if s == "" {
		return time.Date(def.Year(), def.Month(), def.Day(), 0, 0, 0, 0, time.UTC), nil
	}
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q (want YYYY-MM-DD)", s)
	}
	return d, nil
}

// countExistingOutcomes counts stockpicker rows already recorded in the range.
func countExistingOutcomes(ctx context.Context, db *sql.DB, start, end time.Time) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM stock_signal_outcomes
		WHERE source LIKE 'stockpicker-%'
		  AND trigger_date >= ? AND trigger_date <= ?`,
		start.Format("2006-01-02"), end.Format("2006-01-02"),
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count outcomes: %w", err)
	}
	return n, nil
}
