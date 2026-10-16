// Package stockpicker — daily win-rate update runner (PR 2e).
//
// RunDailyUpdate is the shared core behind cmd/run-stockpicker-backtest and
// the internal/scheduler stockpicker_daily_update background task: replay the
// configured conditions over point-in-time bars + per-symbol T86 flows,
// record SignalOutcome rows (idempotent ON CONFLICT DO NOTHING), then
// aggregate per (symbol, source) into stock_win_rate and
// data/state/stock_win_rate.json.
//
// Outcomes land in PostgreSQL — the primary destination on the postgres
// backend (SSOT→PG batch B, 2026-09-30; PG-primary write path C7,
// 2026-10-13) — and in the job-local SQLite artifact (data/state/atlas.db)
// on every other backend. On a PG-primary run the SQLite artifact is written
// only when the SQLiteMirror option is on (default off), so it is no longer
// a data source for those runs. Either destination failing fails the run,
// never a silent skip. Quotes are backend-aware (sqlite | postgres) via the
// shared ledger resolver (WP4). The postgres path requires ExpectDB (M12
// target guard) and fails loudly otherwise (B1).
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
	"github.com/jackc/pgx/v5/stdlib"

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
	// DualWrite selects PostgreSQL as the run's primary destination on the
	// postgres backend (SSOT→PG batch B, 2026-09-30: PG is the adjudicated
	// SSOT; C7, 2026-10-13: PG is also the primary writer). The handle is
	// derived from the shared pool opened for the quote store
	// (stdlib.OpenDBFromPool). Effective only when Backend resolves to
	// postgres, which keeps its original meaning: kill switch. Set false to
	// roll back to the pre-batch-B behavior — SQLite is primary again, the
	// SQLite→PG mirror is off, and no PG connection is attempted. Ignored
	// when PGOutcomesDB is injected (an explicit PG handle always wins).
	DualWrite bool
	// SQLiteMirror additionally mirrors the PostgreSQL-primary outcome +
	// win-rate writes into the job-local SQLite artifact (data/state/
	// atlas.db). Default false (ATLAS_STOCKPICKER_SQLITE_MIRROR): on a
	// PG-primary run the artifact is neither opened nor created, so it stops
	// being a data source (C7); set true only as the recoverable
	// rollback/observation path — PG stays authoritative either way. Has no
	// effect while SQLite is primary (sqlite/jsonl backend, or the DualWrite
	// kill switch off), where SQLite is written regardless.
	SQLiteMirror bool
	// PGOutcomesDB is an explicit dual-write destination — a *sql.DB over
	// PostgreSQL (e.g. stdlib.OpenDBFromPool(pool)). When non-nil it wins
	// over DualWrite/Backend resolution (explicit caller intent; used by
	// tests and out-of-process wiring).
	PGOutcomesDB *sql.DB
	// Regimes maps trigger date → market regime for outcome tagging
	// (issue #1863). nil → RunDailyUpdate falls back to the job-local
	// SQLite regime_history table (dev/CLI); production wires the
	// backend-aware historical store (regime_history lives in Postgres on
	// prod) via the scheduler's RegimeLoader.
	Regimes map[string]string
}

// RunDailyResult reports what a run did (or why it skipped).
type RunDailyResult struct {
	Skipped bool
	Backend string
	// Primary is the write destination this run resolved: "postgres" or
	// "sqlite" (C7). SQLiteMirror reports whether the SQLite artifact was
	// written as a mirror of the PG-primary rows.
	Primary      string
	SQLiteMirror bool
	AsOf         time.Time
	Existing     int            // outcome rows found by the idempotency check
	Conditions   []string       // condition IDs evaluated
	Outcomes     int            // SignalOutcome rows produced (and, unless dry-run, recorded)
	Keys         int            // win-rate keys aggregated
	Eligible     int            // calibration-eligible keys
	StatePath    string         // written stock_win_rate.json path
	Coverage     CoverageReport // per-condition sample counts
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
		return res, fmt.Errorf("-expect-db %q requires the postgres backend (resolved %s); no db guard is needed for a sqlite-primary run", opts.ExpectDB, backend)
	}
	if backend == "postgres" && postgresTargetGuardNeeded(opts) {
		if opts.ExpectDB == "" {
			return res, fmt.Errorf("postgres backend requires -expect-db <dbname> to assert the migration target (e.g. -expect-db atlas); re-run with -expect-db to proceed")
		}
		guardCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := guardPostgresTarget(guardCtx, os.Getenv("DATABASE_URL"), opts.ExpectDB); err != nil {
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

	// This run owns every pool it opens: the defer below closes them and puts
	// the previous ledger injection back when the run ends, on every return
	// path (FU-20260930-12 — a pool per tick used to survive the tick).
	// Registered before the stdlib wrapper's Close so it runs last, i.e. the
	// *sql.DB handle is released before the pool underneath it.
	poolOwner := newPoolOwnership()
	defer poolOwner.release()

	// Resolve the PostgreSQL destination BEFORE opening anything else: the
	// idempotency gate below must reflect every enabled destination (P1 fix),
	// and the SQLite artifact must stay unopened on a PG-primary run (C7).
	pgOutcomesDB, pgPool, err := resolvePGDest(ctx, opts, backend, opts.Panel != nil)
	if err != nil {
		return res, err
	}
	// resolvePGDest returns non-nil only for a pool it opened itself (an
	// injected handle is caller-owned and must not be closed here).
	poolOwner.adopt(pgPool)
	if pgOutcomesDB != nil && opts.PGOutcomesDB == nil {
		// Derived handle (stdlib wrapper over the opened pool): close at the
		// end of the run. Injected handles are owned by the caller.
		defer func() { _ = pgOutcomesDB.Close() }()
	}

	// Write plan (C7). Where do this run's rows land?
	//
	//   - postgres backend with the DualWrite switch on (or an injected PG
	//     handle) ⇒ PostgreSQL is PRIMARY; the SQLite artifact is written
	//     only when SQLiteMirror is on, and is never even opened otherwise.
	//   - everything else (sqlite/jsonl backend, or the DualWrite kill
	//     switch off) ⇒ SQLite is primary, exactly as before C7; PG is only
	//     the legacy dual-write mirror (an injected handle).
	pgPrimary := pgPrimaryDestination(backend, opts)
	primaryLabel, mirrorLabel := " (postgres)", " (sqlite)"
	if !pgPrimary {
		primaryLabel, mirrorLabel = "", " (postgres)"
	}
	res.Primary = "sqlite"
	if pgPrimary {
		res.Primary = "postgres"
	}

	// SQLite is opened only when it has a role in this run: primary, or an
	// opted-in mirror of the PG-primary rows. A dry run never opens the
	// mirror (nothing is written on any destination).
	openSQLite := !pgPrimary || (opts.SQLiteMirror && !opts.DryRun)
	var sqliteDB *sql.DB
	if openSQLite {
		sqliteDB, err = openOutcomeDB(opts.WorkDir)
		if err != nil {
			return res, err
		}
		defer func() { _ = sqliteDB.Close() }()
	} else {
		// C7: on a PG-primary run with the mirror off the artifact must not
		// be opened nor created — an absent (or emptied) data/state/atlas.db
		// must never be recreated by the daily job.
		logging.Info("stockpicker_daily_update", "sqlite_artifact_not_opened",
			"path", filepath.Join(opts.WorkDir, "data", "state", "atlas.db"),
			"primary", "postgres",
			"sqlite_mirror", opts.SQLiteMirror,
			"dry_run", opts.DryRun)
	}

	// Primary and mirror handles. primaryDB is nil on a dry run only:
	// resolvePGDest deliberately opens no PG handle there, and the writes
	// below are never reached.
	var primaryDB, mirrorDB *sql.DB
	if pgPrimary {
		primaryDB = pgOutcomesDB
		if openSQLite {
			mirrorDB = sqliteDB
		}
	} else {
		primaryDB = sqliteDB
		mirrorDB = pgOutcomesDB
	}
	var outStore *SignalOutcomeStore
	var winStore *WinRateStore
	if primaryDB != nil {
		outStore = NewSignalOutcomeStore(primaryDB)
		winStore = NewWinRateStore(primaryDB)
	}
	res.SQLiteMirror = pgPrimary && mirrorDB != nil
	logging.Info("stockpicker_daily_update", "write_plan",
		"backend", backend,
		"primary", res.Primary,
		"sqlite_mirror", res.SQLiteMirror,
		"pg_mirror", !pgPrimary && mirrorDB != nil,
		"dual_write", opts.DualWrite,
		"dry_run", opts.DryRun)

	// Idempotency: skip when this run's contribution is already recorded in
	// EVERY enabled destination. The primary count alone would skip the
	// rerun that should heal a mirror-side gap, leaving that day permanently
	// missing from the mirror (independent-review P1, 2026-09-30 — the same
	// rule now covers the SQLite mirror of a PG-primary run). Semantics are
	// unchanged: the gate reads every destination the run writes to.
	if opts.Idempotency != IdempotencyNone && !opts.DryRun {
		var newest time.Time
		if opts.Idempotency == IdempotencyDay {
			newest = newestTriggerDate(opts.AsOf, DefaultForwardDays)
		}
		countOutcomes := func(db *sql.DB) (int, error) {
			if opts.Idempotency == IdempotencyDay {
				return countOutcomesForTriggerDate(ctx, db, newest)
			}
			return countExistingOutcomes(ctx, db, opts.Start, opts.End)
		}
		primaryN, err := countOutcomes(primaryDB)
		if err != nil {
			return res, fmt.Errorf("idempotency check%s: %w", primaryLabel, err)
		}
		recorded := primaryN > 0
		if recorded && mirrorDB != nil {
			// The gate counts the mirror side too: a day/range missing there
			// is "not yet recorded" and must rerun to heal it. A mirror COUNT
			// failure is a hard error naming that side — never silently read
			// as "0 rows" (that would misjudge a failed count as "not yet
			// run" and fire a doomed rerun).
			mirrorN, err := countOutcomes(mirrorDB)
			if err != nil {
				return res, fmt.Errorf("idempotency check%s: %w", mirrorLabel, err)
			}
			recorded = mirrorN > 0
		}
		if recorded {
			if opts.Idempotency == IdempotencyDay {
				logging.Info("stockpicker_daily_update", "skip_day_done",
					"asof", opts.AsOf.Format("2006-01-02"),
					"newest_trigger", newest.Format("2006-01-02"),
					"existing", primaryN)
			}
			res.Existing = primaryN
			res.Skipped = true
			return res, nil
		}
		if opts.Idempotency == IdempotencyRange {
			res.Existing = primaryN
		}
	}

	panel := opts.Panel
	if panel == nil {
		if backend == "postgres" {
			// Reuse the pool resolvePGDest opened for the derived dual-write
			// handle; open one only when dual-write did not (kill switch off,
			// or a dry-run that still needs quotes). Either way this run owns
			// whatever it opened here and closes it on return.
			pool := pgPool
			if pool == nil {
				opened, err := openPostgresPool(ctx)
				if err != nil {
					return res, err
				}
				pool = opened
				poolOwner.adopt(opened)
			}
			panel, err = NewRealPanel(ctx, ledger.NewPostgresQuoteStore(pool), opts.WorkDir)
			if err != nil {
				return res, fmt.Errorf("build panel: %w", err)
			}
		} else {
			quoteStore, err := openSQLiteQuoteStore(opts.WorkDir)
			if err != nil {
				return res, err
			}
			panel, err = NewRealPanel(ctx, quoteStore, opts.WorkDir)
			if err != nil {
				return res, fmt.Errorf("build panel: %w", err)
			}
		}
	}
	symbols := PanelSymbols(panel, opts.Universe)

	// Regime dimension (issue #1863): outcomes carry the market regime of
	// their trigger date. Injected map wins (production: backend-aware
	// historical store — regime_history lives in Postgres on prod);
	// fallback: the run's primary destination (C7: PG on a PG-primary run,
	// the job-local SQLite artifact otherwise — dev/CLI). regime_history
	// rows are recorded ON their date, so the map is PIT-safe. Missing data
	// → nil map → Regime stays "" (pre-#1863 behavior), never a hard
	// failure.
	regimes := opts.Regimes
	if regimes == nil {
		regimes = loadRegimeMap(ctx, primaryDB)
	}

	cfg := BacktestConfig{
		Universe:    symbols,
		Start:       opts.Start,
		End:         opts.End,
		AsOf:        opts.AsOf,
		ForwardDays: DefaultForwardDays,
		CostRate:    costRate,
		Source:      "stockpicker",
		Regimes:     regimes,
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
			"outcomes", len(outcomes),
			"primary", res.Primary,
			"sqlite_mirror", res.SQLiteMirror)
		return res, nil
	}

	if outStore == nil || winStore == nil {
		// Unreachable by construction: the only path that leaves the primary
		// handle nil is the dry-run return above. Guarded anyway — a nil
		// dereference in the scheduled job would be silent data loss.
		return res, fmt.Errorf("stockpicker daily update: no primary write destination resolved (backend=%s)", backend)
	}

	// Write the batch to the primary destination first, then to the mirror
	// (when one is configured). Either failure fails the whole run — the
	// scheduler records the failure, and the NEXT tick's idempotency gate
	// re-runs the day because it counts every enabled destination (a gap
	// there reads as "not yet recorded"); ON CONFLICT DO NOTHING / upsert
	// then make the healing rewrite idempotent. Already-committed rows are
	// never rolled back (compensating deletes on the hot path are riskier
	// than an idempotent rewrite).
	if err := outStore.RecordOutcomes(ctx, outcomes); err != nil {
		return res, fmt.Errorf("record outcomes%s: %w", primaryLabel, err)
	}
	if mirrorDB != nil {
		if err := RecordOutcomes(ctx, mirrorDB, outcomes); err != nil {
			return res, fmt.Errorf("record outcomes%s: %w", mirrorLabel, err)
		}
	}

	// Aggregation reads the PRIMARY destination, so a PG-primary run
	// aggregates the PG rows (the mirror is a copy, never an input).
	summaries, err := AggregateFromStore(ctx, outStore, winStore, aggregationWindow, costRate, minSamples, confidenceLevel, opts.AsOf)
	if err != nil {
		return res, fmt.Errorf("aggregate: %w", err)
	}
	if mirrorDB != nil {
		if err := mirrorWinRateRows(ctx, mirrorDB, summaries, mirrorLabel); err != nil {
			return res, err
		}
	}
	// Regime strata (Phase 1 regime-conditional gate): same outcomes,
	// grouped by (symbol, source, regime) into stock_win_rate_by_regime,
	// with the same mirror as the pooled summaries above.
	regimeSummaries, err := AggregateRegimeFromStore(ctx, outStore, winStore, aggregationWindow, costRate, minSamples, confidenceLevel, opts.AsOf)
	if err != nil {
		return res, fmt.Errorf("aggregate by regime: %w", err)
	}
	if mirrorDB != nil {
		if err := mirrorRegimeWinRateRows(ctx, mirrorDB, regimeSummaries, mirrorLabel); err != nil {
			return res, err
		}
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
		"regime_keys", len(regimeSummaries),
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
	for id := range strings.SplitSeq(list, ",") {
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
// and win-rate rows (data/state/atlas.db). It is called only when SQLite has
// a role in the run: the primary destination on sqlite/jsonl backends (and
// on the DualWrite=false rollback), or the opt-in mirror of a PG-primary run
// (RunDailyOptions.SQLiteMirror). A PG-primary run with the mirror off never
// calls it, so the artifact is neither read nor created (C7).
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

// guardPostgresTarget is the M12 target assertion (guardDatabaseTarget). It is a
// variable so a test can pin what happens AFTER the guard — the pool lifecycle
// (FU-20260930-12) — without a PostgreSQL server. The guard is a mandatory
// prerequisite for both pool-open sites on the postgres backend, so a test
// cannot simply choose options that skip it: that is why the seam exists.
var guardPostgresTarget = guardDatabaseTarget

// postgresTargetGuardNeeded reports whether the M12 migration-target guard
// (DATABASE_URL identity check) must run for this postgres-backend run.
//
// The guard protects the migration target: it prints current_database and
// aborts when it is not the expected one, BEFORE any migration can touch it.
// It is therefore required exactly when the run can open a DATABASE_URL
// connection:
//
//   - no injected PG handle ⇒ resolvePGDest opens the pool (migrations) on
//     the dual-write path, or the panel build opens it — guard required.
//   - injected PG handle but no injected panel ⇒ the panel build opens the
//     DATABASE_URL pool — guard required.
//   - injected PG handle AND injected panel ⇒ nothing in this run reaches
//     DATABASE_URL (no pool, no migration). This is the in-process test seam,
//     so the guard — which would otherwise need a live server — is not run.
//     Production never injects either handle.
func postgresTargetGuardNeeded(opts RunDailyOptions) bool {
	return opts.PGOutcomesDB == nil || opts.Panel == nil
}

// pgPrimaryDestination reports whether PostgreSQL is this run's PRIMARY
// write destination (C7). It is a pure function of the resolved backend and
// the options, so the decision can be pinned without a database.
//
// Rules:
//   - non-postgres backend ⇒ false (SQLite primary; no PG is ever touched).
//   - postgres backend with an injected PG handle ⇒ true (explicit caller
//     intent — resolvePGDest gives that handle precedence).
//   - postgres backend otherwise ⇒ the DualWrite value: true selects PG as
//     primary, false is the original kill switch, which rolls the whole run
//     back to SQLite-primary writes (pre-batch-B behavior, zero PG writes).
func pgPrimaryDestination(backend string, opts RunDailyOptions) bool {
	if backend != "postgres" {
		return false
	}
	if opts.PGOutcomesDB != nil {
		return true
	}
	return opts.DualWrite
}

// resolvePGDest picks the PostgreSQL destination for this run.
//
// Resolution order: an injected PGOutcomesDB wins (explicit caller intent —
// the test seam); otherwise the postgres backend derives a handle from the
// migration-target pool opened for the quote store, but ONLY when the
// DualWrite kill switch is ON and the run is not a dry-run. Every other
// combination yields nil = no PG destination, the pre-batch-B behavior (no
// PG connection is ever attempted on non-postgres backends).
//
// The handle plays two roles (C7): the PRIMARY destination when
// pgPrimaryDestination is true, else the legacy SQLite→PG dual-write mirror.
// A dry run never opens a PG handle (nothing is written anywhere), which is
// why the derived path returns nil there.
//
// The returned pool is non-nil only when this helper opened it (derived
// path); the panel build reuses it instead of opening a second connection.
// A derived pgDB is stdlib.OpenDBFromPool(pool) — the caller closes it
// (injected handles stay caller-owned).
func resolvePGDest(ctx context.Context, opts RunDailyOptions, backend string, panelInjected bool) (pgDB *sql.DB, pool *pgxpool.Pool, err error) {
	if opts.PGOutcomesDB != nil {
		return opts.PGOutcomesDB, nil, nil
	}
	if backend != "postgres" || !opts.DualWrite || opts.DryRun {
		return nil, nil, nil
	}
	if panelInjected {
		return nil, nil, fmt.Errorf("dual-write requested (backend=postgres) but panel was injected without PGOutcomesDB; pass PGOutcomesDB explicitly")
	}
	// Opened by this run, which must adopt and close it (it is returned so the
	// panel build can reuse it instead of dialing a second pool).
	pool, err = openPostgresPool(ctx)
	if err != nil {
		return nil, nil, err
	}
	return stdlib.OpenDBFromPool(pool), pool, nil
}

// poolOwnership owns every pgxpool a single RunDailyUpdate call opened
// (FU-20260930-12).
//
// Why a run has to own its pool: pgx v5 documents that closing the *sql.DB
// returned by stdlib.OpenDBFromPool does NOT close the pgxpool it wraps, so the
// `defer pgOutcomesDB.Close()` in RunDailyUpdate released the stdlib wrapper
// only. The daily task ticks hourly, and every tick past 18:00 Asia/Taipei
// reached at least one open site, so each tick left behind a pool together with
// its health-check goroutine, its idle connections, and the ledger-factory
// injection that pointed at it. Nothing observes that today, which is exactly
// why it accumulated.
//
// release() therefore does two things, and both are needed:
//   - close every adopted pool, so the connections go back to PostgreSQL;
//   - put the PREVIOUS ledger injection back (recorded at construction, before
//     the first open), because a closed pool left installed in the process-wide
//     factory turns a resource leak into a hard failure for every later store
//     that the factory builds.
type poolOwnership struct {
	pools   []*pgxpool.Pool
	restore *pgxpool.Pool
}

// newPoolOwnership records the ledger injection that is in place before this
// run opens any pool.
func newPoolOwnership() *poolOwnership {
	return &poolOwnership{restore: ledger.PostgresPool()}
}

// adopt takes ownership of a pool the run just opened. It is a method on a
// possibly nil receiver so the two call sites can stay one-liners.
func (o *poolOwnership) adopt(pool *pgxpool.Pool) {
	if o == nil || pool == nil {
		return
	}
	o.pools = append(o.pools, pool)
}

// release closes every adopted pool and restores the previous injection. It is
// safe to call with nothing adopted (a SQLite-primary run opens no pool): the
// restoration still runs, which is a no-op when the injection never changed.
func (o *poolOwnership) release() {
	if o == nil {
		return
	}
	for _, pool := range o.pools {
		pool.Close()
	}
	o.pools = nil
	ledger.SetPostgresPool(o.restore)
}

// openPostgresPool is the opener both call sites use. It is a variable so a test
// can substitute a pool that never dials (internal/testdb.UnconnectedPool) and
// pin the open/close lifecycle of a whole RunDailyUpdate call without a
// PostgreSQL server — the leak this fixes is a lifecycle bug, and a lifecycle
// bug that only an integration test can see is a bug that regrows.
var openPostgresPool = openPostgres

// openPostgres connects to the migration target, applies pending migrations,
// and injects the pool into the ledger factory (WP4 quote-store convention).
// Callers derive whatever handles they need from the returned pool (quote
// store, and — batch B — stdlib.OpenDBFromPool for outcome dual-write).
//
// The caller owns the returned pool: it must adopt it into the run's
// poolOwnership so it is closed when the run ends.
func openPostgres(ctx context.Context) (*pgxpool.Pool, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, fmt.Errorf("postgres backend requires DATABASE_URL")
	}
	pool, err := db.Init(ctx, dsn, defaultPostgresMigrations)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	ledger.SetPostgresPool(pool)
	return pool, nil
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
		  AND trigger_date >= $1 AND trigger_date <= $2`,
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
		  AND trigger_date = $1`,
		triggerDate.Format("2006-01-02"),
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count outcomes for trigger date: %w", err)
	}
	return n, nil
}

// loadRegimeMap builds the trigger-date → regime map from regime_history in
// the given primary ledger (SQLite: table created by ledger.InitSchema; PG:
// migration 000011 — both expose date/regime/captured_at, and the same $N
// text runs on either driver). When multiple rows exist per date, the latest
// captured_at wins (ORDER BY ... captured_at ASC + map overwrite). Returns
// nil on any error, on a nil handle (dry run), or when the table is absent —
// regime tagging is an optional enrichment, never a run blocker (issue
// #1863).
func loadRegimeMap(ctx context.Context, db *sql.DB) map[string]string {
	if db == nil {
		return nil
	}
	rows, err := db.QueryContext(ctx,
		`SELECT date, regime FROM regime_history ORDER BY date ASC, captured_at ASC`)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]string)
	for rows.Next() {
		var date, regime string
		if err := rows.Scan(&date, &regime); err != nil {
			return nil
		}
		out[date] = regime
	}
	if err := rows.Err(); err != nil || len(out) == 0 {
		return nil
	}
	return out
}

// mirrorWinRateRows writes the aggregated pooled summaries into the run's
// secondary destination (the SQLite artifact mirroring a PG-primary run, or
// the legacy PG dual-write mirror). label names that destination in errors
// (" (postgres)" / " (sqlite)"). The primary destination is written by
// AggregateFromStore itself; this only copies the same rows across.
func mirrorWinRateRows(ctx context.Context, db *sql.DB, summaries []StockWinRateSummary, label string) error {
	for _, summary := range summaries {
		if err := SaveWinRate(ctx, db, summary); err != nil {
			return fmt.Errorf("save win rate%s %s/%s: %w", label, summary.Symbol, summary.Source, err)
		}
	}
	return nil
}

// mirrorRegimeWinRateRows mirrors the per-regime strata, same contract as
// mirrorWinRateRows.
func mirrorRegimeWinRateRows(ctx context.Context, db *sql.DB, summaries []RegimeWinRateSummary, label string) error {
	for _, summary := range summaries {
		if err := SaveRegimeWinRate(ctx, db, summary); err != nil {
			return fmt.Errorf("save regime win rate%s %s/%s/%s: %w", label, summary.Symbol, summary.Source, summary.Regime, err)
		}
	}
	return nil
}
