// Package stockpicker — daily win-rate update runner (PR 2e).
//
// RunDailyUpdate is the shared core behind cmd/run-stockpicker-backtest and
// the internal/scheduler stockpicker_daily_update background task: replay the
// configured conditions over point-in-time bars + per-symbol T86 flows,
// record SignalOutcome rows (idempotent ON CONFLICT DO NOTHING), then
// aggregate per (symbol, source) into stock_win_rate and
// data/state/stock_win_rate.json.
//
// Outcomes always land in the job-local SQLite artifact
// (data/state/atlas.db), never in the postgres target; quotes are
// backend-aware (sqlite | postgres) via the shared ledger resolver (WP4).
// The postgres path requires ExpectDB (M12 target guard) and fails loudly
// otherwise (B1).
package stockpicker

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/db"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/taiwanholidays"
)

const (
	// DefaultForwardDays is the holding period (trading sessions) used by the
	// backtest and by the scheduled day-idempotency check. Kept in one place
	// so the two stay in lockstep.
	DefaultForwardDays = 5

	aggregationWindow         = "120d" // rolling window label
	confidenceLevel           = 0.95   // Wilson interval confidence
	defaultPostgresMigrations = "sql/migrations"
)

// Idempotency selects how a run decides "nothing new to compute".
type Idempotency int

const (
	// IdempotencyRange skips when outcomes already exist anywhere in
	// [Start, End] — the CLI's same-range rerun guard (same-day rerun skips
	// unless -force).
	IdempotencyRange Idempotency = iota
	// IdempotencyDay skips when outcomes already exist for the run's newest
	// trigger date (the trading day DefaultForwardDays sessions before AsOf)
	// — the scheduled daily-update guard. Each trading day's post-close run
	// adds exactly the triggers in the last DefaultForwardDays sessions, so
	// an outcome on that date proves today's increment is already recorded;
	// a failed run leaves no outcome there and the next tick retries.
	IdempotencyDay
	// IdempotencyNone always runs; RecordOutcomes' ON CONFLICT DO NOTHING
	// keeps rows unique (the -force path).
	IdempotencyNone
)

// RunDailyOptions configures one stockpicker daily win-rate update run.
type RunDailyOptions struct {
	WorkDir     string      // atlas repo root; data/state lives under it
	Backend     string      // quote backend: sqlite | postgres | "" (ATLAS_STORE_BACKEND, then job-local sqlite)
	ExpectDB    string      // postgres migration-target guard (M12); "" disables
	Idempotency Idempotency // skip policy for already-recorded outcomes
	DryRun      bool        // compute coverage without persisting
	Universe    string      // comma-separated symbols (default: all quote symbols)
	Conditions  string      // comma-separated condition IDs (default: parameters.json defaults)
	AsOf        time.Time   // point-in-time cutoff (zero → today UTC)
	Start       time.Time   // first trigger date (zero → AsOf-120d)
	End         time.Time   // last trigger date (zero → AsOf)
	Panel       PanelSource // nil → real panel built from WorkDir
}

// RunDailyResult reports what a run did (or why it skipped).
type RunDailyResult struct {
	Skipped    bool
	Backend    string
	AsOf       time.Time
	Existing   int            // outcome rows found by the idempotency check
	Conditions []string       // condition IDs evaluated
	Outcomes   int            // SignalOutcome rows produced (and, unless dry-run, recorded)
	Keys       int            // win-rate keys aggregated
	Eligible   int            // calibration-eligible keys
	StatePath  string         // written stock_win_rate.json path
	Coverage   CoverageReport // per-condition sample counts
}

// RunDailyUpdate executes the stockpicker daily win-rate update pipeline.
// Both callers (CLI and scheduler) share this function so the backtest and
// aggregation logic exists exactly once (PR 2e: 禁止複製貼上邏輯).
func RunDailyUpdate(ctx context.Context, opts RunDailyOptions) (RunDailyResult, error) {
	var res RunDailyResult
	if opts.WorkDir == "" {
		return res, fmt.Errorf("stockpicker daily update: workdir is empty")
	}
	if opts.AsOf.IsZero() {
		opts.AsOf = DateOnlyUTC(time.Now())
	}
	if opts.Start.IsZero() {
		opts.Start = opts.AsOf.AddDate(0, 0, -120)
	}
	if opts.End.IsZero() {
		opts.End = opts.AsOf
	}
	if opts.End.Before(opts.Start) {
		return res, fmt.Errorf("end %s before start %s", opts.End.Format("2006-01-02"), opts.Start.Format("2006-01-02"))
	}

	backend, err := ResolveBackend(opts.Backend)
	if err != nil {
		return res, err
	}
	res.Backend = backend

	// M12 target guard: report the postgres connection target and abort on
	// ExpectDB mismatch before any migration can touch the wrong database.
	// B1: postgres without ExpectDB fails loudly before any connection — an
	// opt-in guard leaves the M12 failure mode reachable via a stray
	// DATABASE_URL.
	if opts.ExpectDB != "" && backend != "postgres" {
		return res, fmt.Errorf("-expect-db %q requires the postgres backend (resolved %s); outcomes are job-local sqlite and need no db guard", opts.ExpectDB, backend)
	}
	if backend == "postgres" && opts.ExpectDB == "" {
		return res, fmt.Errorf("postgres backend requires -expect-db <dbname> to assert the migration target (e.g. -expect-db atlas); re-run with -expect-db to proceed")
	}
	if backend == "postgres" {
		guardCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := guardDatabaseTarget(guardCtx, os.Getenv("DATABASE_URL"), opts.ExpectDB); err != nil {
			return res, err
		}
	}

	// Parameters from the worktree's parameters.json (P0-3/P0-6: cost and
	// min_samples come from config, never hard-coded).
	paramsPath := filepath.Join(opts.WorkDir, "configs", "parameters.json")
	if _, err := os.Stat(paramsPath); err != nil {
		return res, fmt.Errorf("parameters config %s: %w", paramsPath, err)
	}
	params, err := config.LoadParametersConfig(paramsPath)
	if err != nil {
		return res, fmt.Errorf("load parameters: %w", err)
	}
	costRate := params.Stockpicker.Costs.RoundTripPct.Value
	minSamples := params.Stockpicker.Calibration.MinSamples.Value

	registry := NewDefaultConditionRegistry(&params.Stockpicker.Conditions)
	conds, err := selectConditions(opts.Conditions, registry)
	if err != nil {
		return res, err
	}
	res.Conditions = conditionIDs(conds)

	outcomeDB, err := openOutcomeDB(opts.WorkDir)
	if err != nil {
		return res, err
	}
	defer func() { _ = outcomeDB.Close() }()

	outStore := NewSignalOutcomeStore(outcomeDB)
	winStore := NewWinRateStore(outcomeDB)

	// Idempotency: skip when this run's contribution is already recorded.
	if opts.Idempotency != IdempotencyNone && !opts.DryRun {
		var n int
		switch opts.Idempotency {
		case IdempotencyDay:
			newest := newestTriggerDate(opts.AsOf, DefaultForwardDays)
			n, err = countOutcomesForTriggerDate(ctx, outcomeDB, newest)
			if err != nil {
				return res, fmt.Errorf("idempotency check: %w", err)
			}
			if n > 0 {
				logging.Info("stockpicker_daily_update", "skip_day_done",
					"asof", opts.AsOf.Format("2006-01-02"),
					"newest_trigger", newest.Format("2006-01-02"),
					"existing", n)
				res.Existing = n
				res.Skipped = true
				return res, nil
			}
		default: // IdempotencyRange
			n, err = countExistingOutcomes(ctx, outcomeDB, opts.Start, opts.End)
			if err != nil {
				return res, fmt.Errorf("idempotency check: %w", err)
			}
			res.Existing = n
			if n > 0 {
				res.Skipped = true
				return res, nil
			}
		}
	}

	panel := opts.Panel
	if panel == nil {
		quoteStore, err := openQuoteStore(ctx, backend, opts.WorkDir)
		if err != nil {
			return res, err
		}
		panel, err = NewRealPanel(ctx, quoteStore, opts.WorkDir)
		if err != nil {
			return res, fmt.Errorf("build panel: %w", err)
		}
	}
	symbols := PanelSymbols(panel, opts.Universe)

	cfg := BacktestConfig{
		Universe:    symbols,
		Start:       opts.Start,
		End:         opts.End,
		AsOf:        opts.AsOf,
		ForwardDays: DefaultForwardDays,
		CostRate:    costRate,
		Source:      "stockpicker",
	}
	outcomes, err := RunBacktest(ctx, cfg, panel, conds...)
	if err != nil {
		return res, fmt.Errorf("run backtest: %w", err)
	}
	res.Outcomes = len(outcomes)
	res.Coverage = BuildCoverage(cfg, outcomes)

	if opts.DryRun {
		logging.Info("stockpicker_daily_update", "dry_run",
			"asof", opts.AsOf.Format("2006-01-02"),
			"outcomes", len(outcomes))
		return res, nil
	}

	if err := outStore.RecordOutcomes(ctx, outcomes); err != nil {
		return res, fmt.Errorf("record outcomes: %w", err)
	}

	summaries, err := AggregateFromStore(ctx, outStore, winStore, aggregationWindow, costRate, minSamples, confidenceLevel, opts.AsOf)
	if err != nil {
		return res, fmt.Errorf("aggregate: %w", err)
	}
	statePath := filepath.Join(opts.WorkDir, "data", "state", "stock_win_rate.json")
	if err := WriteStateJSON(statePath, summaries, opts.AsOf); err != nil {
		return res, fmt.Errorf("write state json: %w", err)
	}
	res.Keys = len(summaries)
	for _, s := range summaries {
		if s.CalibrationStatus == CalibrationEligible {
			res.Eligible++
		}
	}
	res.StatePath = statePath

	logging.Info("stockpicker_daily_update", "run_ok",
		"asof", opts.AsOf.Format("2006-01-02"),
		"backend", backend,
		"outcomes", len(outcomes),
		"keys", len(summaries),
		"eligible", res.Eligible,
		"state", statePath)
	return res, nil
}

// DateOnlyUTC truncates t to its calendar date at UTC midnight. Used to pass
// a stable point-in-time cutoff between the scheduler and the runner.
func DateOnlyUTC(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// newestTriggerDate returns the trading day exactly forwardDays trading
// sessions before asOf. A daily run with end=asof produces outcomes with
// newest trigger date equal to this value when the panel is current, so its
// presence in the outcome ledger proves the day's increment is recorded.
func newestTriggerDate(asOf time.Time, forwardDays int) time.Time {
	d := asOf.AddDate(0, 0, -1)
	for {
		if taiwanholidays.IsTradingDay(d) {
			forwardDays--
			if forwardDays == 0 {
				return d
			}
		}
		d = d.AddDate(0, 0, -1)
	}
}

// selectConditions resolves a comma-separated condition-ID list against the
// registry. An empty/blank list resolves to the full default registry set
// (the PR 1c demo conditions). Unknown IDs are rejected.
func selectConditions(list string, reg *ConditionRegistry) ([]Condition, error) {
	if strings.TrimSpace(list) == "" {
		return reg.All(), nil
	}
	var out []Condition
	for _, id := range strings.Split(list, ",") {
		id = strings.TrimSpace(id)
		c, ok := reg.Lookup(id)
		if !ok {
			return nil, fmt.Errorf("unknown condition %q (use -list-conditions)", id)
		}
		out = append(out, *c)
	}
	return out, nil
}

// conditionIDs extracts the ID of each condition in order.
func conditionIDs(conds []Condition) []string {
	ids := make([]string, len(conds))
	for i, c := range conds {
		ids[i] = c.ID
	}
	return ids
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
	logging.Info("stockpicker_daily_update", "outcomes_ledger",
		"path", dbPath)
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
		// jsonl resolves through the shared resolver but is not a backend the
		// daily update can serve — fail loudly instead of silently switching.
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
	logging.Info("stockpicker_daily_update", "db_target",
		"current_database", actual,
		"inet_server_port", port)
	if expectDB != "" && actual != expectDB {
		return fmt.Errorf("expected database %q but connected to %q; aborting before migrations (DATABASE_URL target mismatch)", expectDB, actual)
	}
	return nil
}

// ResolveBackend picks the quote backend. Explicit value wins, then the
// ATLAS_STORE_BACKEND env var, then the job-local sqlite default. It
// delegates normalization and fail-loud validation to the shared ledger
// resolver (WP4) instead of a local DATABASE_URL heuristic (M4②).
func ResolveBackend(flagValue string) (string, error) {
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
		// backend the daily update can serve — quotes read sqlite|postgres
		// and outcomes are a job-local SQLite artifact.
		return "", fmt.Errorf("store backend jsonl is not supported by the stockpicker daily update (quotes: sqlite | postgres; outcomes: job-local sqlite)")
	}
	return backend, nil
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

// countOutcomesForTriggerDate counts stockpicker outcome rows recorded for a
// single trigger date — the scheduled daily-update idempotency check.
func countOutcomesForTriggerDate(ctx context.Context, db *sql.DB, triggerDate time.Time) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM stock_signal_outcomes
		WHERE source LIKE 'stockpicker-%'
		  AND trigger_date = ?`,
		triggerDate.Format("2006-01-02"),
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count outcomes for trigger date: %w", err)
	}
	return n, nil
}

// NewestTriggerDateForCheck is a temporary exported wrapper for verification.
func NewestTriggerDateForCheck(asOf time.Time) time.Time {
	return newestTriggerDate(asOf, DefaultForwardDays)
}
