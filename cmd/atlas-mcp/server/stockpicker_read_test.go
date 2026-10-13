package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"

	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/stockpicker"
	"github.com/kaecer68/atlas-go/internal/testdb"
)

// This file covers the C6 read-path resolution (PostgreSQL vs. SQLite) and the
// two properties the PostgreSQL path must hold: the SQL it sends is valid for
// PostgreSQL ($N placeholders, strictly ascending, no duplicates — positional
// binding is what makes one SQL text work on both drivers) and every statement
// it sends is a read.
//
// The tests are hermetic (no PostgreSQL server): a stand-in SQLite ledger
// covers handle *selection*, a driver-recording stub covers the dialect, and
// testdb.UnconnectedPool covers the unreachable-PostgreSQL fallback. The real
// PostgreSQL behaviour (row-for-row parity, read-only session) is pinned by
// tools_stock_winrate_pg_integration_test.go.

const (
	testWindow         = "120d"
	testSourceBottom   = "stockpicker-price-volume-bottom-divergence"
	testSourceTop      = "stockpicker-price-volume-top-divergence"
	testCondBottom     = "price-volume-bottom-divergence"
	unreachableTestDSN = "postgres://atlas:atlas@127.0.0.1:1/atlas_c6_unreachable?sslmode=disable"
)

// --- resolveWinRateBackend ---------------------------------------------------

func TestResolveWinRateBackend(t *testing.T) {
	cases := []struct {
		declared string
		want     string
		wantErr  bool
	}{
		{declared: "", want: WinRateBackendSQLite},
		{declared: "jsonl", want: WinRateBackendSQLite},
		{declared: "sqlite", want: WinRateBackendSQLite},
		{declared: "postgres", want: WinRateBackendPostgres},
		{declared: "mysql", wantErr: true},
	}
	for _, tc := range cases {
		got, err := ResolveWinRateBackend(tc.declared)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("ResolveWinRateBackend(%q): want error, got %q", tc.declared, got)
			}
			if !strings.Contains(err.Error(), "mysql") {
				t.Fatalf("ResolveWinRateBackend(%q): error %q should name the rejected value", tc.declared, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ResolveWinRateBackend(%q): %v", tc.declared, err)
		}
		if got != tc.want {
			t.Fatalf("ResolveWinRateBackend(%q) = %q, want %q", tc.declared, got, tc.want)
		}
	}
}

// TestResolveWinRateBackendDelegatesToLedger pins the "one normalization rule"
// claim: every value ledger.ResolveStoreBackend accepts must reach the same
// win-rate backend through the wrapper.
func TestResolveWinRateBackendDelegatesToLedger(t *testing.T) {
	for _, declared := range []string{"", "jsonl", "sqlite", "postgres"} {
		want, err := ledger.ResolveStoreBackend(declared)
		if err != nil {
			t.Fatalf("ledger.ResolveStoreBackend(%q): %v", declared, err)
		}
		got, err := ResolveWinRateBackend(declared)
		if err != nil {
			t.Fatalf("ResolveWinRateBackend(%q): %v", declared, err)
		}
		if want == "postgres" && got != WinRateBackendPostgres {
			t.Fatalf("declared %q normalized to postgres but win-rate backend = %q", declared, got)
		}
		if want != "postgres" && got != WinRateBackendSQLite {
			t.Fatalf("declared %q normalized to %q but win-rate backend = %q", declared, want, got)
		}
	}
}

// --- read-only PostgreSQL handle --------------------------------------------

func TestReadOnlyWinRateConnConfigForcesReadOnlySession(t *testing.T) {
	const dsn = "postgres://atlas:pw@db.internal:5432/atlas?sslmode=disable"
	cc, err := readOnlyWinRateConnConfig(dsn)
	if err != nil {
		t.Fatalf("readOnlyWinRateConnConfig: %v", err)
	}
	if got := cc.RuntimeParams["default_transaction_read_only"]; got != "on" {
		t.Fatalf("default_transaction_read_only = %q, want \"on\" (the no-write contract must be structural, not a convention)", got)
	}
	// The rest of the DSN must survive: a read-only handle that lost the host
	// or the user would be useless.
	if cc.Host != "db.internal" || cc.Port != 5432 || cc.User != "atlas" || cc.Database != "atlas" {
		t.Fatalf("parsed conn config = host %q port %d user %q db %q, want db.internal:5432 atlas/atlas", cc.Host, cc.Port, cc.User, cc.Database)
	}
}

func TestOpenReadOnlyWinRatePGDBRejectsInvalidDSN(t *testing.T) {
	if _, err := OpenReadOnlyWinRatePGDB("not-a-dsn"); err == nil {
		t.Fatal("OpenReadOnlyWinRatePGDB accepted an invalid DSN")
	}
}

// TestOpenReadOnlyWinRatePGDBDoesNotDial pins the lazy-open contract the
// startup wiring depends on: an unreachable PostgreSQL must not turn into a
// "cannot start the MCP server" failure, because winRateHandle probes the
// handle per call and falls back to SQLite in the meantime.
func TestOpenReadOnlyWinRatePGDBDoesNotDial(t *testing.T) {
	db, err := OpenReadOnlyWinRatePGDB(unreachableTestDSN)
	if err != nil {
		t.Fatalf("OpenReadOnlyWinRatePGDB(unreachable): %v (want a lazy handle, not an error)", err)
	}
	defer func() { _ = db.Close() }()
	if err := db.PingContext(context.Background()); err == nil {
		t.Fatal("Ping succeeded against an unreachable DSN; the test DSN is not actually unreachable")
	}
}

// --- handle selection --------------------------------------------------------

// TestWinRateHandlePrefersPostgresHandle pins the resolution order: a usable
// Config.WinRatePGDB wins over a wired SQLite handle holding different data.
//
// The stand-in handle is another SQLite ledger because the win-rate SQL text
// is identical on both backends ($N placeholders); what this test pins is
// *which* handle the tools read, plus the caching rule. Real PostgreSQL
// behaviour is the integration test's job.
func TestWinRateHandlePrefersPostgresHandle(t *testing.T) {
	s := newWinRateReadServer(t)
	sqliteDB := openWinRateLedger(t)
	seedWinRateRow(t, sqliteDB, winRateSeed{Symbol: "2330", Source: testSourceBottom, Window: testWindow, Observations: 3, Hits: 2, WinRate: 2.0 / 3.0, WilsonLower: 0.15, WilsonUpper: 0.90})
	pgStandIn := openWinRateLedger(t)
	seedWinRateRow(t, pgStandIn, winRateSeed{Symbol: "2330", Source: testSourceBottom, Window: testWindow, Observations: 9, Hits: 7, WinRate: 7.0 / 9.0, WilsonLower: 0.44, WilsonUpper: 0.93})

	s.winRateSQLiteDB = sqliteDB
	s.cfg.WinRatePGDB = pgStandIn

	out, err := callStockWinRate(s, stockWinRateInput{Symbol: "2330"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !out.Found || len(out.Conditions) != 1 {
		t.Fatalf("found=%v conditions=%d, want the postgres row: %+v", out.Found, len(out.Conditions), out.Conditions)
	}
	got := out.Conditions[0]
	if got.Observations != 9 || got.Hits != 7 {
		t.Fatalf("observations/hits = %d/%d, want 9/7 (the postgres handle), not 3/2 (SQLite)", got.Observations, got.Hits)
	}
	if got.WinRate != 7.0/9.0 {
		t.Fatalf("win_rate = %v, want %v", got.WinRate, 7.0/9.0)
	}
	s.winRateMu.Lock()
	resolved := s.winRateResolved
	s.winRateMu.Unlock()
	if resolved == nil || resolved.backend != WinRateBackendPostgres {
		t.Fatalf("resolved handle = %+v, want the postgres handle cached", resolved)
	}
}

// TestWinRateHandleFallsBackToSQLiteWhenPGUnreachable pins the backward
// compatibility half of C6: an unusable Config.WinRatePGDB must not fail the
// tool — it reads the SQLite ledger, exactly as before this change.
func TestWinRateHandleFallsBackToSQLiteWhenPGUnreachable(t *testing.T) {
	s := newWinRateReadServer(t)
	sqliteDB := openWinRateLedger(t)
	seedWinRateRow(t, sqliteDB, winRateSeed{Symbol: "2330", Source: testSourceBottom, Window: testWindow, Observations: 3, Hits: 2, WinRate: 2.0 / 3.0, WilsonLower: 0.15, WilsonUpper: 0.90})
	s.winRateSQLiteDB = sqliteDB
	s.cfg.WinRatePGDB = stdlib.OpenDBFromPool(testdb.UnconnectedPool(t, unreachableTestDSN))

	out, err := callStockWinRate(s, stockWinRateInput{Symbol: "2330"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !out.Found || len(out.Conditions) != 1 {
		t.Fatalf("found=%v conditions=%d, want the SQLite fallback row: %+v", out.Found, len(out.Conditions), out.Conditions)
	}
	if got := out.Conditions[0]; got.Observations != 3 || got.Hits != 2 {
		t.Fatalf("observations/hits = %d/%d, want 3/2 (SQLite)", got.Observations, got.Hits)
	}
	// The fallback must NOT be cached as a decision: while PostgreSQL is
	// unreachable the next call probes it again, so a PostgreSQL that comes
	// back is picked up without restarting the MCP server.
	s.winRateMu.Lock()
	resolved := s.winRateResolved
	s.winRateMu.Unlock()
	if resolved != nil {
		t.Fatalf("handle resolution cached %+v while PostgreSQL was unreachable; want no cache", resolved)
	}
}

// TestWinRateHandleReportsBothBackendsWhenBothUnusable: with an unreachable
// PostgreSQL and no SQLite ledger configured, the message must not send the
// operator hunting for ATLAS_MCP_STOCKPICKER_DB only.
func TestWinRateHandleReportsBothBackendsWhenBothUnusable(t *testing.T) {
	s := newWinRateReadServer(t)
	s.cfg.WinRatePGDB = stdlib.OpenDBFromPool(testdb.UnconnectedPool(t, unreachableTestDSN))
	s.cfg.StockpickerDBPath = ""

	out, err := callStockWinRate(s, stockWinRateInput{Symbol: "2330"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Found {
		t.Fatal("found=true with no usable backend")
	}
	for _, want := range []string{"postgres win-rate ledger unusable", "ATLAS_MCP_STOCKPICKER_DB"} {
		if !strings.Contains(out.Message, want) {
			t.Fatalf("message = %q, want it to mention %q", out.Message, want)
		}
	}
}

// --- dialect + read-only contract on the PostgreSQL path ---------------------

func TestWinRatePostgresPathSendsPostgresSQLAndOnlyReads(t *testing.T) {
	s := newWinRateReadServer(t)
	rec, handle := newRecordingPGHandle(t)
	s.cfg.WinRatePGDB = handle

	// Every query builder in the win-rate read path, so a missing "$" or an
	// accidental "?" in any one of them fails this test:
	//   stock_get_win_rate           → winRateSources + stockpicker.LoadWinRate
	//   stock_picker_scan (default)  → scanWinRateRows
	//   stock_picker_scan (shrunk)   → scanWinRateRows + scanPooledPrior
	//   stock_picker_scan (avoid)    → the avoid-mode scanWinRateRows
	out, err := callStockWinRate(s, stockWinRateInput{Symbol: "2330"})
	if err != nil {
		t.Fatalf("stock_get_win_rate: %v", err)
	}
	if !out.Found {
		t.Fatalf("stock_get_win_rate found=false against the recording handle (message=%q)", out.Message)
	}
	for _, in := range []stockPickerScanInput{
		{ConditionID: testCondBottom, MinObservations: 3},
		{ConditionID: testCondBottom, MinObservations: 3, SortBy: "shrunk_rate"},
		{ConditionID: "price-volume-top-divergence", MinObservations: 3, Direction: "avoid"},
	} {
		if _, err := callStockPickerScan(s, in); err != nil {
			t.Fatalf("stock_picker_scan(%+v): %v", in, err)
		}
	}

	queries := rec.recorded()
	if len(queries) == 0 {
		t.Fatal("no SQL reached the recording handle; the test would pass vacuously")
	}
	joined := strings.Join(queries, "\n")
	for _, want := range []string{"SELECT DISTINCT source", "SUM(hits)", "FROM stock_signal_outcomes", "win_rate <= $3"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("no recorded statement contains %q; the test no longer exercises that builder:\n%s", want, joined)
		}
	}
	for _, q := range queries {
		if strings.Contains(q, "?") {
			t.Errorf("statement sent to PostgreSQL uses a '?' placeholder:\n%s", q)
		}
		if !strings.Contains(q, "$1") {
			t.Errorf("statement sent to PostgreSQL has no $1 placeholder:\n%s", q)
		}
		if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(q)), "SELECT") {
			t.Errorf("statement sent to PostgreSQL is not a read:\n%s", q)
		}
		assertAscendingPlaceholders(t, q)
	}
	if execs := rec.executed(); len(execs) > 0 {
		t.Fatalf("the win-rate read path executed %d statement(s) through Exec; it must be read-only: %v", len(execs), execs)
	}
}

// assertAscendingPlaceholders checks the invariant that makes one SQL text
// valid on both drivers: the $N placeholders must appear in ascending order
// starting at 1, each exactly once. Database/sql binds arguments positionally
// in order of appearance, so a gap (missing $2), a repeat (two $1) or a
// reordering silently binds the wrong value — or, on SQLite, changes the
// parameter count and fails the statement.
func assertAscendingPlaceholders(t *testing.T, query string) {
	t.Helper()
	matches := placeholderRe.FindAllStringSubmatch(query, -1)
	if len(matches) == 0 {
		t.Errorf("statement has no $N placeholders:\n%s", query)
		return
	}
	for i, m := range matches {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("placeholder %q is not a number in:\n%s", m[0], query)
		}
		if n != i+1 {
			t.Errorf("placeholder #%d of the statement is $%d, want $%d (positional binding would be wrong):\n%s", i+1, n, i+1, query)
		}
	}
}

var placeholderRe = regexp.MustCompile(`\$(\d+)`)

// --- helpers -----------------------------------------------------------------

// newWinRateReadServer builds a server for the read-path tests. The win-rate
// handlers only need the audit writer (the HTTP client is unused), so the
// shared harness is enough; no ledger is wired — each test wires its own.
func newWinRateReadServer(t *testing.T) *server {
	t.Helper()
	s, _, done := newTestHarness(t)
	t.Cleanup(done)
	return s
}

// openWinRateLedger opens an in-memory SQLite ledger with the full schema.
func openWinRateLedger(t *testing.T) *sql.DB {
	t.Helper()
	db, err := ledger.OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := ledger.InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	return db
}

// winRateSeed is one calibration-eligible win-rate summary used to seed a
// ledger; the fixed columns (confidence, cost rate, updated_at) stay constant
// so the PostgreSQL/SQLite parity comparison has nothing incidental to differ
// on.
type winRateSeed struct {
	Symbol            string
	Source            string
	Window            string
	Observations      int
	Hits              int
	WinRate           float64
	WilsonLower       float64
	WilsonUpper       float64
	AvgForwardReturn  float64
	BreakEvenCostRate float64
}

// seedWinRateRow writes one calibration-eligible win-rate summary.
func seedWinRateRow(t *testing.T, db *sql.DB, seed winRateSeed) {
	t.Helper()
	err := stockpicker.NewWinRateStore(db).SaveWinRate(context.Background(), stockpicker.StockWinRateSummary{
		Symbol:            seed.Symbol,
		Source:            seed.Source,
		Window:            seed.Window,
		Observations:      seed.Observations,
		Hits:              seed.Hits,
		WinRate:           seed.WinRate,
		WilsonLower:       seed.WilsonLower,
		WilsonUpper:       seed.WilsonUpper,
		Confidence:        0.95,
		CalibrationStatus: stockpicker.CalibrationEligible,
		NetCostRate:       0.00585,
		AvgForwardReturn:  seed.AvgForwardReturn,
		BreakEvenCostRate: seed.BreakEvenCostRate,
		UpdatedAt:         "2026-08-27T12:00:00Z",
	})
	if err != nil {
		t.Fatalf("save win rate %s/%s: %v", seed.Symbol, seed.Source, err)
	}
}

// --- recording driver --------------------------------------------------------

var (
	registerRecordingDriverOnce sync.Once
	// recordingDriverRecorder is process-global because database/sql driver
	// names are registered once per process; newRecordingPGHandle resets it so
	// each test sees only its own statements.
	recordingDriverRecorder = &winRateQueryRecorder{}
)

const recordingDriverName = "winrate_recording_stub"

// newRecordingPGHandle returns a *sql.DB whose driver records every statement
// and hands back canned rows, plus the recorder. It stands in for the
// PostgreSQL handle in hermetic tests: Ping succeeds, so the resolver selects
// it, and the SQL the read path sends becomes inspectable.
func newRecordingPGHandle(t *testing.T) (*winRateQueryRecorder, *sql.DB) {
	t.Helper()
	registerRecordingDriverOnce.Do(func() {
		sql.Register(recordingDriverName, recordingDriver{rec: recordingDriverRecorder})
	})
	recordingDriverRecorder.reset()
	db, err := sql.Open(recordingDriverName, "")
	if err != nil {
		t.Fatalf("open recording handle: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return recordingDriverRecorder, db
}

type winRateQueryRecorder struct {
	mu      sync.Mutex
	queries []string
	execs   []string
}

func (r *winRateQueryRecorder) add(queriesOrExecs *[]string, q string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	*queriesOrExecs = append(*queriesOrExecs, q)
}

func (r *winRateQueryRecorder) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queries = nil
	r.execs = nil
}

func (r *winRateQueryRecorder) recorded() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.queries...)
}

func (r *winRateQueryRecorder) executed() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.execs...)
}

type recordingDriver struct{ rec *winRateQueryRecorder }

func (d recordingDriver) Open(string) (driver.Conn, error) { return recordingConn{rec: d.rec}, nil }

type recordingConn struct{ rec *winRateQueryRecorder }

func (c recordingConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("recording handle: Prepare is not supported; the read path must use QueryContext")
}
func (c recordingConn) Close() error { return nil }
func (c recordingConn) Begin() (driver.Tx, error) {
	return nil, errors.New("recording handle: Begin is not supported")
}
func (c recordingConn) Ping(context.Context) error {
	return nil
}

func (c recordingConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.rec.add(&c.rec.queries, query)
	cols, rows := cannedRows(query)
	return &recordingRows{cols: cols, rows: rows}, nil
}

func (c recordingConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	c.rec.add(&c.rec.execs, query)
	return driver.RowsAffected(0), nil
}

// cannedRows answers the win-rate read path's statements with the column
// count each caller scans, so every builder runs to completion. Argument
// values are ignored: this handle exists to inspect SQL, not to return data.
func cannedRows(query string) ([]string, [][]driver.Value) {
	switch {
	case strings.Contains(query, "SELECT DISTINCT source"):
		return []string{"source"}, [][]driver.Value{{testSourceBottom}}
	case strings.Contains(query, "SUM(hits)"):
		return []string{"source", "hits", "observations"}, [][]driver.Value{{testSourceBottom, int64(30), int64(40)}}
	case strings.Contains(query, "FROM stock_signal_outcomes"):
		return []string{"symbol", "trigger_date", "source", "forward_return", "net_forward_return", "hit", "cost_rate", "regime", "created_at"}, nil
	case strings.Contains(query, "FROM stock_win_rate"):
		return []string{
				"symbol", "source", "rolling_window", "observations", "hits", "win_rate",
				"wilson_lower", "wilson_upper", "confidence", "calibration_status",
				"net_cost_rate", "avg_forward_return", "break_even_cost_rate", "updated_at",
			},
			[][]driver.Value{{
				"2330", testSourceBottom, testWindow, int64(30), int64(20), 0.6666,
				0.55, 0.78, 0.95, "eligible", 0.00585, 0.01, 0.02, "2026-08-27T12:00:00Z",
			}}
	default:
		return nil, nil
	}
}

type recordingRows struct {
	cols []string
	rows [][]driver.Value
	next int
}

func (r *recordingRows) Columns() []string { return r.cols }
func (r *recordingRows) Close() error      { return nil }

func (r *recordingRows) Next(dest []driver.Value) error {
	if r.next >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.next])
	r.next++
	return nil
}

// jsonOf marshals an output for whole-struct comparisons (used by the
// PostgreSQL integration parity test).
func jsonOf(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}
