package stockpicker

// pg_primary_write_test.go — C7 (2026-10-13): on the postgres backend the
// daily update writes PostgreSQL as its PRIMARY destination, and the
// job-local SQLite artifact (data/state/atlas.db) becomes an opt-in mirror
// (ATLAS_STOCKPICKER_SQLITE_MIRROR, default off).
//
// These tests are hermetic: the PG handle is a recording driver (no server,
// no network) and the panel is synthetic, so a run can be driven end to end
// while every statement it sends to "PostgreSQL" stays inspectable. What they
// pin is:
//
//   - which destination receives which statement (writes AND the aggregation
//     reads, which follow the primary),
//   - that data/state/atlas.db is not opened nor created when the mirror is
//     off, and
//   - that the "artifact absent" assertions can actually fail (negative
//     controls below), so their green is not vacuous.
//
// Row-level parity against a real PostgreSQL is the integration test's job
// (pg_primary_integration_test.go).

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// --- the primary-destination decision ---------------------------------------

// TestPgPrimaryDestination_DecisionTable pins the rule that selects the
// primary write destination. Getting this wrong in either direction is a data
// incident: PG-primary is what stops atlas.db from being written, while the
// DualWrite=false branch is the documented rollback to SQLite-primary writes.
func TestPgPrimaryDestination_DecisionTable(t *testing.T) {
	injected := &sql.DB{}
	cases := []struct {
		name    string
		backend string
		opts    RunDailyOptions
		want    bool
	}{
		{"sqlite backend stays sqlite-primary", "sqlite", RunDailyOptions{DualWrite: true}, false},
		{"empty backend resolves sqlite", "", RunDailyOptions{DualWrite: true, PGOutcomesDB: injected}, false},
		{"postgres + dual-write on ⇒ PG primary", "postgres", RunDailyOptions{DualWrite: true}, true},
		{"postgres + kill switch off ⇒ sqlite-primary rollback", "postgres", RunDailyOptions{DualWrite: false}, false},
		{"postgres + injected handle ⇒ PG primary even with the kill switch off", "postgres", RunDailyOptions{PGOutcomesDB: injected}, true},
		{"dry run keeps the same plan", "postgres", RunDailyOptions{DualWrite: true, DryRun: true}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pgPrimaryDestination(tc.backend, tc.opts); got != tc.want {
				t.Fatalf("pgPrimaryDestination(%q, %+v) = %v, want %v", tc.backend, tc.opts, got, tc.want)
			}
		})
	}
}

// TestPostgresTargetGuardNeeded_DecisionTable pins when the M12 target guard
// still runs. The guard is the protection against migrating the wrong
// database; the only case it is skipped is a fully in-process run (injected
// PG handle + injected panel), where nothing can reach DATABASE_URL. A future
// edit that widens the skip would silently disarm it in production, so the
// table covers every combination.
func TestPostgresTargetGuardNeeded_DecisionTable(t *testing.T) {
	injected := &sql.DB{}
	cases := []struct {
		name string
		opts RunDailyOptions
		want bool
	}{
		{"production: nothing injected", RunDailyOptions{}, true},
		{"injected handle, real panel (panel opens DATABASE_URL)", RunDailyOptions{PGOutcomesDB: injected}, true},
		{"real handle, injected panel (dual-write pool opens DATABASE_URL)", RunDailyOptions{Panel: synthPanel(t)}, true},
		{"fully in-process run", RunDailyOptions{PGOutcomesDB: injected, Panel: synthPanel(t)}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := postgresTargetGuardNeeded(tc.opts); got != tc.want {
				t.Fatalf("postgresTargetGuardNeeded(%+v) = %v, want %v", tc.opts, got, tc.want)
			}
		})
	}
}

// --- the C7 contract as a reusable assertion --------------------------------

// sqliteArtifactProblems reports every way a PG-primary run breaks the C7
// contract: PostgreSQL must be the primary destination, the SQLite mirror must
// be reported off, and data/state/atlas.db must not exist. Returning the
// problems (instead of calling t.Fatal) is what makes the negative controls
// below possible: they run a configuration that is expected to violate the
// contract and assert this helper notices.
func sqliteArtifactProblems(res RunDailyResult, workdir string) []string {
	var problems []string
	if res.Primary != "postgres" {
		problems = append(problems, fmt.Sprintf("primary destination = %q, want %q", res.Primary, "postgres"))
	}
	if res.SQLiteMirror {
		problems = append(problems, "run reported the SQLite mirror as ON")
	}
	// The artifact is the file this card exists to retire; the state JSON
	// snapshot next to it is written either way (it is not a ledger).
	if _, err := os.Stat(filepath.Join(workdir, "data", "state", "atlas.db")); err == nil {
		problems = append(problems, "data/state/atlas.db was created")
	} else if !os.IsNotExist(err) {
		problems = append(problems, fmt.Sprintf("stat atlas.db: %v", err))
	}
	return problems
}

// pgPrimaryRunOptions is the hermetic PG-primary configuration shared by the
// tests below: the postgres backend with an injected recording handle (so no
// server is contacted) and the synthetic panel.
func pgPrimaryRunOptions(t *testing.T, rec *pgPrimaryRecorder) RunDailyOptions {
	t.Helper()
	return RunDailyOptions{
		Backend:      "postgres",
		PGOutcomesDB: rec.handle(t),
	}
}

// TestRunDailyUpdate_PGPrimary_WritesPostgresOnly is the core C7 case: with
// the mirror off, every write goes to PostgreSQL and the SQLite artifact is
// never created — not even opened.
func TestRunDailyUpdate_PGPrimary_WritesPostgresOnly(t *testing.T) {
	rec := newPGPrimaryRecorder(t)

	workdir, res, err := runUpdate(t, pgPrimaryRunOptions(t, rec))
	if err != nil {
		t.Fatalf("PG-primary run: %v", err)
	}
	if res.Skipped {
		t.Fatal("first run must not skip")
	}
	if res.Outcomes == 0 {
		t.Fatal("expected outcomes to be produced")
	}

	// (1) The contract: PG primary, mirror off, no atlas.db.
	if problems := sqliteArtifactProblems(res, workdir); len(problems) != 0 {
		t.Fatalf("PG-primary run violated the C7 contract:\n  - %s", strings.Join(problems, "\n  - "))
	}
	// Sanity: the run did persist something (the state snapshot is still
	// written), so "no atlas.db" cannot be explained by "the run did nothing".
	if _, err := os.Stat(filepath.Join(workdir, "data", "state", "stock_win_rate.json")); err != nil {
		t.Fatalf("state snapshot missing: %v", err)
	}

	// (2) The outcome batch reached PostgreSQL, one statement per outcome
	// (ON CONFLICT DO NOTHING keeps the batch rewrite idempotent).
	execs := rec.executed()
	outcomeInserts := countContaining(execs, "INSERT INTO stock_signal_outcomes")
	if outcomeInserts != res.Outcomes {
		t.Fatalf("recorded %d stock_signal_outcomes INSERTs, want one per outcome (%d); execs=%v",
			outcomeInserts, res.Outcomes, execs)
	}
	// (3) The aggregated rows reached PostgreSQL too, not SQLite.
	for _, want := range []string{"INSERT INTO stock_win_rate ", "INSERT INTO stock_win_rate_by_regime"} {
		if n := countContaining(execs, want); n == 0 {
			t.Fatalf("no %q statement reached PostgreSQL; execs=%v", want, execs)
		}
	}
	// (4) The aggregation READS follow the primary destination: the outcome
	// load and the regime fallback both ran against PostgreSQL, so a
	// PG-primary run never reads the mirror back as an input.
	queries := rec.readQueries()
	for _, want := range []string{"FROM stock_signal_outcomes", "FROM regime_history"} {
		if n := countContaining(queries, want); n == 0 {
			t.Fatalf("no %q read reached PostgreSQL; queries=%v", want, queries)
		}
	}
	// (5) Every statement is PostgreSQL-shaped: no '?' placeholder may leak
	// onto this path, every $N must be in ascending order (database/sql binds
	// positionally), and every write must be parameterized.
	for _, q := range rec.statements() {
		if strings.Contains(q, "?") {
			t.Errorf("statement sent to PostgreSQL uses a '?' placeholder:\n%s", q)
		}
		assertAscendingDollarPlaceholders(t, q)
	}
	for _, q := range execs {
		if !strings.Contains(q, "$1") {
			t.Errorf("write statement sent to PostgreSQL has no $1 placeholder:\n%s", q)
		}
	}
}

// TestRunDailyUpdate_PGPrimary_IdempotencyDayUsesPostgres pins that the daily
// idempotency gate counts the PRIMARY destination: the scheduled job must
// still skip once the day's increment is recorded (in PostgreSQL), without
// writing anything anywhere.
func TestRunDailyUpdate_PGPrimary_IdempotencyDayUsesPostgres(t *testing.T) {
	rec := newPGPrimaryRecorder(t)
	rec.setCount(7) // "the newest trigger date already has rows"

	opts := pgPrimaryRunOptions(t, rec)
	opts.Idempotency = IdempotencyDay
	workdir, res, err := runUpdate(t, opts)
	if err != nil {
		t.Fatalf("PG-primary idempotency run: %v", err)
	}
	if !res.Skipped {
		t.Fatal("a recorded day must skip under IdempotencyDay")
	}
	if res.Existing != 7 {
		t.Fatalf("Existing = %d, want the PostgreSQL count (7)", res.Existing)
	}
	if got := len(rec.executed()); got != 0 {
		t.Fatalf("a skipped run wrote %d statement(s): %v", got, rec.executed())
	}
	if problems := sqliteArtifactProblems(res, workdir); len(problems) != 0 {
		t.Fatalf("skipped PG-primary run violated the C7 contract:\n  - %s", strings.Join(problems, "\n  - "))
	}
}

// TestRunDailyUpdate_PGPrimary_DryRunWritesNothing covers the dry-run rule:
// nothing is written on either destination, and the run neither creates the
// artifact nor (with the mirror explicitly on) opens it.
func TestRunDailyUpdate_PGPrimary_DryRunWritesNothing(t *testing.T) {
	for _, mirror := range []bool{false, true} {
		t.Run(fmt.Sprintf("sqlite_mirror=%v", mirror), func(t *testing.T) {
			rec := newPGPrimaryRecorder(t)
			opts := pgPrimaryRunOptions(t, rec)
			opts.DryRun = true
			opts.SQLiteMirror = mirror

			workdir, res, err := runUpdate(t, opts)
			if err != nil {
				t.Fatalf("dry run: %v", err)
			}
			if res.Outcomes == 0 {
				t.Fatal("dry-run should still compute outcomes")
			}
			// Reads are part of computing coverage; WRITES are what a dry
			// run must never do.
			if got := rec.executed(); len(got) != 0 {
				t.Fatalf("dry run wrote %d statement(s) to PostgreSQL: %v", len(got), got)
			}
			if _, err := os.Stat(filepath.Join(workdir, "data", "state", "stock_win_rate.json")); !os.IsNotExist(err) {
				t.Fatalf("dry run wrote the state snapshot (stat err=%v)", err)
			}
			if _, err := os.Stat(filepath.Join(workdir, "data", "state", "atlas.db")); !os.IsNotExist(err) {
				t.Fatalf("dry run created data/state/atlas.db (stat err=%v)", err)
			}
		})
	}
}

// TestRunDailyUpdate_PGPrimary_SQLiteMirrorOnWritesBoth is the recoverable
// path: with ATLAS_STOCKPICKER_SQLITE_MIRROR on, the PG-primary rows are also
// written into the artifact (the pre-C7 "both sides" behavior), while
// PostgreSQL stays the destination the aggregation reads.
func TestRunDailyUpdate_PGPrimary_SQLiteMirrorOnWritesBoth(t *testing.T) {
	rec := newPGPrimaryRecorder(t)
	opts := pgPrimaryRunOptions(t, rec)
	opts.SQLiteMirror = true

	workdir, res, err := runUpdate(t, opts)
	if err != nil {
		t.Fatalf("PG-primary run with the SQLite mirror on: %v", err)
	}
	if !res.SQLiteMirror {
		t.Fatal("result must report the SQLite mirror as on")
	}
	if res.Primary != "postgres" {
		t.Fatalf("primary = %q, want postgres (the mirror must not take over)", res.Primary)
	}
	if problems := sqliteArtifactProblems(res, workdir); len(problems) == 0 {
		t.Fatal("the artifact-absence assertions must fail when the mirror is on (they are not the ones under test here)")
	}

	db := openTestOutcomeDB(t, workdir)
	if n := outcomeCount(t, db); n != res.Outcomes {
		t.Fatalf("SQLite mirror holds %d outcomes, want %d", n, res.Outcomes)
	}
	// The win-rate mirror is written from the aggregated rows, which came
	// from the PG read path (canned rows) — so both sides were written.
	if n := countRows(t, db, `SELECT COUNT(*) FROM stock_win_rate`); n == 0 {
		t.Fatal("SQLite mirror received no stock_win_rate rows")
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM stock_win_rate_by_regime`); n == 0 {
		t.Fatal("SQLite mirror received no stock_win_rate_by_regime rows")
	}
	if n := countContaining(rec.executed(), "INSERT INTO stock_signal_outcomes"); n != res.Outcomes {
		t.Fatalf("PostgreSQL received %d outcome INSERTs, want %d", n, res.Outcomes)
	}
}

// TestRunDailyUpdate_NegativeControl_SQLitePrimaryStillWritesArtifact is the
// disproof of the assertion above: run the SAME assertion helper against the
// pre-C7 shape (SQLite primary, PostgreSQL as the mirror), and it must report
// the violations — including that the artifact was created. Without this, a
// green "atlas.db absent" test could be green for the wrong reason.
func TestRunDailyUpdate_NegativeControl_SQLitePrimaryStillWritesArtifact(t *testing.T) {
	rec := newPGPrimaryRecorder(t)

	// sqlite backend + injected PG handle = the write shape C7 replaces (and
	// the shape the DualWrite kill switch still selects on the postgres
	// backend): SQLite is primary, PG is the mirror.
	opts := RunDailyOptions{PGOutcomesDB: rec.handle(t)}

	workdir, res, err := runUpdate(t, opts)
	if err != nil {
		t.Fatalf("legacy sqlite-primary run: %v", err)
	}
	if res.Primary != "sqlite" {
		t.Fatalf("primary = %q, want sqlite for the legacy shape", res.Primary)
	}
	// The PG mirror got the rows (so the run really did write), and the
	// artifact assertions below are not passing for lack of activity.
	if n := countContaining(rec.executed(), "INSERT INTO stock_signal_outcomes"); n != res.Outcomes {
		t.Fatalf("legacy run sent %d outcome INSERTs to the PG mirror, want %d", n, res.Outcomes)
	}
	problems := sqliteArtifactProblems(res, workdir)
	if len(problems) == 0 {
		t.Fatal("the C7 assertions accepted a run that writes SQLite as its primary destination")
	}
	if !strings.Contains(strings.Join(problems, "\n"), "atlas.db was created") {
		t.Fatalf("the artifact must be reported as created for the legacy shape; problems=%v", problems)
	}
}

// TestRunDailyUpdate_NegativeControl_MirrorOnCreatesArtifact is the second
// disproof, aimed at the new flag: flipping only SQLiteMirror on a PG-primary
// run must flip the artifact assertion to red.
func TestRunDailyUpdate_NegativeControl_MirrorOnCreatesArtifact(t *testing.T) {
	rec := newPGPrimaryRecorder(t)
	opts := pgPrimaryRunOptions(t, rec)
	opts.SQLiteMirror = true

	workdir, res, err := runUpdate(t, opts)
	if err != nil {
		t.Fatalf("PG-primary run with the mirror on: %v", err)
	}
	problems := sqliteArtifactProblems(res, workdir)
	if len(problems) == 0 {
		t.Fatal("the artifact assertion stayed green while the mirror was on")
	}
	for _, want := range []string{"SQLite mirror as ON", "atlas.db was created"} {
		if !strings.Contains(strings.Join(problems, "\n"), want) {
			t.Fatalf("problems must mention %q; problems=%v", want, problems)
		}
	}
}

// --- helpers ----------------------------------------------------------------

// countRows runs a scalar COUNT query against a SQLite test handle.
func countRows(t *testing.T, db *sql.DB, query string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query).Scan(&n); err != nil {
		t.Fatalf("count rows (%s): %v", query, err)
	}
	return n
}

// countContaining counts the recorded statements containing substr.
func countContaining(stmts []string, substr string) int {
	n := 0
	for _, s := range stmts {
		if strings.Contains(s, substr) {
			n++
		}
	}
	return n
}

// assertAscendingDollarPlaceholders mirrors the C6b check: one SQL text valid
// on both drivers must use $1..$N in ascending order, because database/sql
// binds arguments positionally (a gap or a repeat silently binds the wrong
// value).
func assertAscendingDollarPlaceholders(t *testing.T, query string) {
	t.Helper()
	seen := 0
	for i := 0; i < len(query); i++ {
		if query[i] != '$' {
			continue
		}
		j := i + 1
		for j < len(query) && query[j] >= '0' && query[j] <= '9' {
			j++
		}
		if j == i+1 {
			t.Errorf("bare '$' in statement:\n%s", query)
			continue
		}
		seen++
		if want := fmt.Sprintf("$%d", seen); query[i:j] != want {
			t.Errorf("placeholder #%d is %q, want %q (positional binding would be wrong):\n%s", seen, query[i:j], want, query)
		}
		i = j - 1
	}
	// Zero placeholders is legitimate for a filter-free read (e.g. the
	// regime_history scan); callers that need parameterization assert it
	// separately.
}

// --- recording driver -------------------------------------------------------

// pgPrimaryRecordingDriverName is registered once per process; the recorder
// itself is reset per test so each test sees only its own statements.
const pgPrimaryRecordingDriverName = "stockpicker_pg_primary_recording_stub"

var (
	registerPGPrimaryDriver sync.Once
	pgPrimarySharedRecorder = &pgPrimaryRecorder{}
)

// pgPrimaryRecorder records every statement the run sends to the (fake)
// PostgreSQL handle and answers SELECTs from a fixed canned table. It is
// deliberately NOT a PostgreSQL emulator: inserts are recorded, never stored,
// and reads answer canned rows, so tests assert on WHICH destination received
// WHICH statement — row-value parity belongs to the real-PG integration test.
type pgPrimaryRecorder struct {
	mu       sync.Mutex
	execs    []string
	queries  []string
	count    int64
	handleDB *sql.DB
}

func newPGPrimaryRecorder(t *testing.T) *pgPrimaryRecorder {
	t.Helper()
	registerPGPrimaryDriver.Do(func() {
		sql.Register(pgPrimaryRecordingDriverName, pgPrimaryDriver{rec: pgPrimarySharedRecorder})
	})
	rec := pgPrimarySharedRecorder
	rec.mu.Lock()
	rec.execs, rec.queries, rec.count, rec.handleDB = nil, nil, 0, nil
	rec.mu.Unlock()
	t.Cleanup(func() {
		rec.mu.Lock()
		if rec.handleDB != nil {
			_ = rec.handleDB.Close()
			rec.handleDB = nil
		}
		rec.mu.Unlock()
	})
	return rec
}

// handle opens (once per test) a *sql.DB over the recording driver.
func (r *pgPrimaryRecorder) handle(t *testing.T) *sql.DB {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.handleDB == nil {
		db, err := sql.Open(pgPrimaryRecordingDriverName, "")
		if err != nil {
			t.Fatalf("open recording handle: %v", err)
		}
		r.handleDB = db
	}
	return r.handleDB
}

func (r *pgPrimaryRecorder) setCount(n int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.count = n
}

func (r *pgPrimaryRecorder) addExec(q string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.execs = append(r.execs, q)
}

func (r *pgPrimaryRecorder) addQuery(q string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queries = append(r.queries, q)
}

func (r *pgPrimaryRecorder) executed() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.execs...)
}

func (r *pgPrimaryRecorder) readQueries() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.queries...)
}

func (r *pgPrimaryRecorder) statements() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.execs)+len(r.queries))
	out = append(out, r.execs...)
	return append(out, r.queries...)
}

func (r *pgPrimaryRecorder) countValue() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count
}

type pgPrimaryDriver struct{ rec *pgPrimaryRecorder }

func (d pgPrimaryDriver) Open(string) (driver.Conn, error) { return pgPrimaryConn(d), nil }

type pgPrimaryConn struct{ rec *pgPrimaryRecorder }

func (c pgPrimaryConn) Prepare(query string) (driver.Stmt, error) {
	return pgPrimaryStmt{rec: c.rec, query: query}, nil
}

func (c pgPrimaryConn) Close() error { return nil }

// Begin/BeginTx: RecordOutcomes wraps its batch in a transaction, so the
// recording connection must support one (the write path legitimately uses it).
func (c pgPrimaryConn) Begin() (driver.Tx, error) { return pgPrimaryTx{}, nil }

func (c pgPrimaryConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return pgPrimaryTx{}, nil
}

type pgPrimaryTx struct{}

func (pgPrimaryTx) Commit() error   { return nil }
func (pgPrimaryTx) Rollback() error { return nil }

type pgPrimaryStmt struct {
	rec   *pgPrimaryRecorder
	query string
}

func (s pgPrimaryStmt) Close() error  { return nil }
func (s pgPrimaryStmt) NumInput() int { return -1 }

func (s pgPrimaryStmt) Exec([]driver.Value) (driver.Result, error) {
	s.rec.addExec(normalizeSQL(s.query))
	return driver.RowsAffected(0), nil
}

func (s pgPrimaryStmt) Query([]driver.Value) (driver.Rows, error) {
	q := normalizeSQL(s.query)
	s.rec.addQuery(q)
	cols, data, ok := pgPrimaryCanned(s.rec, q)
	if !ok {
		// Fail loudly: an unexpected statement means the run found another
		// PostgreSQL destination, and silently answering it would hide that.
		return nil, fmt.Errorf("pg-primary recording driver: unexpected query: %s", q)
	}
	return &pgPrimaryRows{cols: cols, data: data}, nil
}

// pgPrimaryCanned answers the three shapes the daily update sends to its
// primary destination. Matching on the query text keeps the stub honest: a
// builder that changes its SELECT list or table breaks the test instead of
// being silently answered with the wrong shape.
func pgPrimaryCanned(rec *pgPrimaryRecorder, query string) ([]string, [][]driver.Value, bool) {
	switch {
	case strings.Contains(query, "SELECT COUNT(*) FROM stock_signal_outcomes"):
		return []string{"count"}, [][]driver.Value{{rec.countValue()}}, true
	case strings.Contains(query, "FROM stock_signal_outcomes"):
		return []string{
				"symbol", "trigger_date", "source", "forward_return", "net_forward_return",
				"hit", "cost_rate", "regime", "created_at",
			}, [][]driver.Value{
				{"2330", "2026-03-16", "stockpicker-foreign-3d-net-buy", 0.02, 0.0196, int64(1), 0.00585, "RISK_ON", "2026-03-16T00:00:00Z"},
				{"2330", "2026-03-13", "stockpicker-foreign-3d-net-buy", 0.01, 0.0096, int64(1), 0.00585, "RISK_ON", "2026-03-13T00:00:00Z"},
			}, true
	case strings.Contains(query, "FROM regime_history"):
		return []string{"date", "regime"},
			[][]driver.Value{{"2026-03-16", "RISK_ON"}, {"2026-03-13", "RISK_ON"}}, true
	default:
		return nil, nil, false
	}
}

// pgPrimaryRows must use a POINTER receiver: the cursor has to advance between
// Next calls (a value receiver resets it and the reader loops forever on the
// first row).
type pgPrimaryRows struct {
	cols []string
	data [][]driver.Value
	pos  int
}

func (r *pgPrimaryRows) Columns() []string { return r.cols }
func (r *pgPrimaryRows) Close() error      { return nil }

func (r *pgPrimaryRows) Next(dest []driver.Value) error {
	if r.pos >= len(r.data) {
		return io.EOF
	}
	copy(dest, r.data[r.pos])
	r.pos++
	return nil
}

// normalizeSQL collapses the whitespace of a statement so the canned-query
// matching is insensitive to the formatting used in the source.
func normalizeSQL(q string) string {
	return strings.Join(strings.Fields(q), " ")
}
