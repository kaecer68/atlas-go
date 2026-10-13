//go:build integration

package stocktools

// PostgreSQL half of C6b: the four win-rate endpoints read PostgreSQL when the
// deployment declares it, and that switch must not change a single byte of the
// responses while the two stores agree.
//
// This file pins three things against a real PostgreSQL (DATABASE_URL; skipped
// locally when unset, fatal in CI — internal/testdb policy):
//
//  1. parity   — the same rows in both backends produce byte-identical endpoint
//                output (the HTTP body is the contract, so the comparison is
//                made on raw response bytes, not on selected fields);
//  2. negative — when the two backends are deliberately made to disagree, the
//                outputs follow PostgreSQL and the parity assertion goes red.
//                Without this the parity test could pass while the endpoints
//                still read SQLite;
//  3. read-only — the handle production wires rejects writes and still reads.
//
// The SQLite ledger is a mirror of the PostgreSQL tables taken after seeding, so
// the parity claim does not depend on the shared database being otherwise
// empty: rows other integration tests leave behind in the real sources are
// compared too, not silently ignored.
//
// Isolation: the summary fixture uses a rolling-window label no other producer
// writes (pgTestWindow), the outcome fixtures use two condition sources keyed to
// symbols that are not Taiwan tickers, and the industry fixture uses a condition
// id no producer writes (pgTestCondMapped). Cleanup deletes by those keys before
// and after the test, so the shared database is left as found.

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/stdlib"

	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/stockpicker"
	"github.com/kaecer68/atlas-go/internal/testdb"
)

const (
	pgTestWindow = "c6bpytest"
	// pgTestOutcomeWindow is the rolling-window label the outcome-based reads use
	// (condition / industry / family). Unlike the summary window label above it
	// must parse as "Nd": LoadOutcomes rejects anything else, and the default is
	// 120d. The fixture dates stay inside it.
	pgTestOutcomeWindow = "120d"
	pgTestSymbol        = "C6BT"
	pgTestSymbol2       = "C6B2"
	// pgTestCondMapped is a condition id no producer writes. The industry
	// endpoint keys on "stockpicker-<condition_id>", and the canonical L1
	// mapping only knows real tickers, so the industry fixture uses this source
	// with real symbols — which keeps both the mapping and the cleanup honest
	// (nothing else writes this source, so nothing else can be deleted).
	pgTestCondMapped   = "c6b-industry-fixture"
	pgTestCondBottom   = "price-volume-bottom-divergence"
	pgTestCondTop      = "price-volume-top-divergence"
	pgTestSource       = "stockpicker-" + pgTestCondBottom
	pgTestSourceTop    = "stockpicker-" + pgTestCondTop
	pgTestSourceMapped = "stockpicker-" + pgTestCondMapped
	// pgTestMappedSymbol[12] are real tickers with a canonical L1 industry
	// (SectorMachinery / SectorSemiconductor). They only ever carry the fixture
	// source above, so the fixture can exercise the industry ranking and the
	// industry_id filter without touching rows a producer wrote.
	pgTestMappedSymbol  = "4536"
	pgTestMappedSymbol2 = "2303"
	// pgTestGeneratedAt is the fixed clock of the family-expectancy read: the
	// response embeds generated_at, so comparing bodies byte-for-byte needs one
	// instant, not two calls to time.Now().
	pgTestGeneratedAt = "2026-10-13T07:00:00Z"
)

// TestWinRateEndpointsPostgresSQLiteParity asserts byte-for-byte parity of the
// four endpoints across the two backends.
func TestWinRateEndpointsPostgresSQLiteParity(t *testing.T) {
	pgReadOnly, pgWritable := pgWinRateFixture(t)
	seedWinRateFixture(t, pgWritable)
	sqlitePath, _ := mirrorWinRateStores(t, pgWritable)

	pgSource := newWinRatePGSrc(t, pgReadOnly)
	sqliteSource := NewWinRateSource(nil, sqlitePath)
	t.Cleanup(func() { _ = sqliteSource.Close() })

	fixed := pgTestFixedNow(t)
	pgMux := winRateEndpointMux(winRateAllFourDeps(pgSource), fixed)
	sqliteMux := winRateEndpointMux(winRateAllFourDeps(sqliteSource), fixed)

	for _, path := range parityEndpointCases(t) {
		t.Run(path, func(t *testing.T) {
			pgBody := serveWinRateEndpoint(t, pgMux, path)
			sqliteBody := serveWinRateEndpoint(t, sqliteMux, path)
			if pgBody != sqliteBody {
				t.Fatalf("postgres and sqlite response bodies differ for %s\npostgres: %s\nsqlite:   %s", path, pgBody, sqliteBody)
			}
			if path != pgNoDataPath && !strings.Contains(pgBody, `"found":true`) {
				t.Fatalf("%s answered no data; the parity comparison would be vacuous: %s", path, pgBody)
			}
		})
	}

	// The PostgreSQL server must have resolved the PostgreSQL handle; a silent
	// fallback to SQLite would make every case above pass while comparing SQLite
	// against SQLite. (Resolution happens on the first request, so this check
	// runs after the loop.)
	resolved := pgSource.resolvedHandle(t)
	if resolved == nil || resolved.backend != WinRateBackendPostgres {
		t.Fatalf("postgres source resolved %+v, want the postgres handle", resolved)
	}
}

// TestWinRateEndpointsFollowPostgresWhenBackendsDiverge is the negative proof
// for the parity test: the same fixture is made to disagree in PostgreSQL only.
// The parity assertion must now be false, the endpoints must report the
// PostgreSQL numbers, and the failure has to come from the data — not from an
// error, a status change, or a missing row.
func TestWinRateEndpointsFollowPostgresWhenBackendsDiverge(t *testing.T) {
	pgReadOnly, pgWritable := pgWinRateFixture(t)
	seedWinRateFixture(t, pgWritable)
	sqlitePath, _ := mirrorWinRateStores(t, pgWritable)

	pgSource := newWinRatePGSrc(t, pgReadOnly)
	sqliteSource := NewWinRateSource(nil, sqlitePath)
	t.Cleanup(func() { _ = sqliteSource.Close() })

	fixed := pgTestFixedNow(t)
	pgProvider := NewWinRateDBProvider(pgSource)
	sqliteProvider := NewWinRateDBProvider(sqliteSource)
	pgMux := winRateEndpointMux(Deps{
		WinRate:          pgProvider,
		ConditionWinRate: pgProvider,
		IndustryWinRate:  pgProvider,
	}, fixed)
	sqliteMux := winRateEndpointMux(Deps{
		WinRate:          sqliteProvider,
		ConditionWinRate: sqliteProvider,
		IndustryWinRate:  sqliteProvider,
	}, fixed)

	perSymbol := "/api/stock/win_rate?symbol=" + pgTestSymbol + "&rolling_window=" + pgTestWindow
	perCondition := "/api/stock/condition_winrate?condition_id=" + pgTestCondMapped + "&rolling_window=" + pgTestOutcomeWindow
	perIndustry := "/api/stock/industry_winrate?condition_id=" + pgTestCondMapped + "&rolling_window=" + pgTestOutcomeWindow

	// Control: the mirror is identical on both backends before the divergence.
	for _, path := range []string{perSymbol, perCondition, perIndustry} {
		pgBody := serveWinRateEndpoint(t, pgMux, path)
		if pgBody != serveWinRateEndpoint(t, sqliteMux, path) {
			t.Fatalf("%s: fixture is not identical before divergence\npostgres: %s\nsqlite:   %s", path, pgBody, serveWinRateEndpoint(t, sqliteMux, path))
		}
		if !strings.Contains(pgBody, `"found":true`) {
			t.Fatalf("%s: control request answered no data: %s", path, pgBody)
		}
	}

	// Diverge PostgreSQL only: (a) the stored summary behind the per-symbol read
	// and (b) one extra hit outcome behind the condition/industry aggregates.
	diverged := pgWinRateSummary(pgTestSymbol, pgTestSource)
	diverged.Observations, diverged.Hits = 9, 7
	diverged.WinRate = 7.0 / 9.0
	if err := stockpicker.NewWinRateStore(pgWritable).SaveWinRate(context.Background(), diverged); err != nil {
		t.Fatalf("diverge postgres summary: %v", err)
	}
	extra := stockpicker.SignalOutcome{
		Symbol:           pgTestMappedSymbol,
		TriggerDate:      pgTestDates()[len(pgTestDates())-1],
		Source:           pgTestSourceMapped,
		ForwardReturn:    0.05,
		NetForwardReturn: 0.04415,
		Hit:              true,
		CostRate:         0.00585,
		Regime:           "RISK_ON",
		CreatedAt:        "2026-10-13T00:00:00Z",
	}
	if err := stockpicker.NewSignalOutcomeStore(pgWritable).RecordOutcomes(context.Background(), []stockpicker.SignalOutcome{extra}); err != nil {
		t.Fatalf("diverge postgres outcomes: %v", err)
	}

	for _, path := range []string{perSymbol, perCondition, perIndustry} {
		pgBody := serveWinRateEndpoint(t, pgMux, path)
		sqliteBody := serveWinRateEndpoint(t, sqliteMux, path)
		if pgBody == sqliteBody {
			t.Fatalf("%s: outputs still match after PostgreSQL was diverged; the parity test above proves nothing about which backend is read", path)
		}
	}

	// Pin the direction: the diverged numbers are the PostgreSQL ones, and the
	// SQLite snapshot keeps the original ones.
	after := serveWinRateEndpoint(t, pgMux, perSymbol)
	afterSQLite := serveWinRateEndpoint(t, sqliteMux, perSymbol)
	for _, want := range []string{`"observations":9`, `"hits":7`} {
		if !strings.Contains(after, want) {
			t.Fatalf("postgres body does not carry the diverged row (%s): %s", want, after)
		}
	}
	for _, want := range []string{`"observations":40`, `"hits":30`} {
		if !strings.Contains(afterSQLite, want) {
			t.Fatalf("sqlite body does not carry the original row (%s): %s", want, afterSQLite)
		}
	}
}

// TestWinRatePostgresReadHandleRejectsWrites pins the no-write contract on the
// PostgreSQL side: production's handle is a read-only session, so a write fails
// in the server (SQLSTATE 25006) instead of depending on the queries staying
// SELECTs. The writable control on the same database keeps the assertion from
// passing just because the connection is broken.
func TestWinRatePostgresReadHandleRejectsWrites(t *testing.T) {
	pgReadOnly, pgWritable := pgWinRateFixture(t)
	ctx := context.Background()

	if _, err := pgWritable.ExecContext(ctx, `DELETE FROM stock_win_rate WHERE rolling_window = $1`, pgTestWindow); err != nil {
		t.Fatalf("writable control handle could not write: %v", err)
	}

	var mode string
	if err := pgReadOnly.QueryRowContext(ctx, `SELECT current_setting('default_transaction_read_only')`).Scan(&mode); err != nil {
		t.Fatalf("read-only handle cannot read: %v", err)
	}
	if mode != "on" {
		t.Fatalf("default_transaction_read_only = %q on the read handle, want \"on\"", mode)
	}

	if _, err := pgReadOnly.ExecContext(ctx, `DELETE FROM stock_win_rate WHERE rolling_window = $1`, pgTestWindow); err == nil {
		t.Fatal("DELETE succeeded on the win-rate read handle; the no-write contract is not structural")
	} else if !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("write failed with %v, want a read-only transaction error", err)
	}

	// The read-only handle the endpoints use must still serve the fixture.
	var count int
	if err := pgReadOnly.QueryRowContext(ctx, `SELECT count(*) FROM stock_win_rate WHERE rolling_window = $1`, pgTestWindow).Scan(&count); err != nil {
		t.Fatalf("read-only handle cannot read: %v", err)
	}
}

// TestWinRateEndpointsFallBackWhenPostgresDisappears exercises the fallback
// against the real server: a source whose PostgreSQL handle is dead reads the
// SQLite ledger and serves the same bytes as the (working) SQLite-only source.
func TestWinRateEndpointsFallBackWhenPostgresDisappears(t *testing.T) {
	_, pgWritable := pgWinRateFixture(t)
	seedWinRateFixture(t, pgWritable)
	sqlitePath, _ := mirrorWinRateStores(t, pgWritable)

	fallback := NewWinRateSource(unreachableWinRateHandle(t), sqlitePath)
	t.Cleanup(func() { _ = fallback.Close() })
	plain := NewWinRateSource(nil, sqlitePath)
	t.Cleanup(func() { _ = plain.Close() })

	fixed := pgTestFixedNow(t)
	fallbackMux := winRateEndpointMux(winRateAllFourDeps(fallback), fixed)
	plainMux := winRateEndpointMux(winRateAllFourDeps(plain), fixed)

	for _, path := range []string{
		"/api/stock/win_rate?symbol=" + pgTestSymbol + "&rolling_window=" + pgTestWindow,
		"/api/stock/condition_winrate?condition_id=" + pgTestCondBottom + "&rolling_window=" + pgTestOutcomeWindow,
		"/api/stock/family-expectancy",
	} {
		got := serveWinRateEndpoint(t, fallbackMux, path)
		if got != serveWinRateEndpoint(t, plainMux, path) {
			t.Fatalf("%s: fallback source and SQLite-only source disagree\nfallback: %s\nsqlite:   %s", path, got, serveWinRateEndpoint(t, plainMux, path))
		}
		if !strings.Contains(got, `"found":true`) {
			t.Fatalf("%s: fallback answered no data: %s", path, got)
		}
	}
	if resolved := fallback.resolvedHandle(t); resolved != nil {
		t.Fatalf("fallback cached %+v while postgres was unreachable; a postgres that comes back would not be picked up", resolved)
	}
}

// --- fixture -----------------------------------------------------------------

// pgNoDataPath is one parity case expected to answer found=false on both
// backends (a symbol the fixture never writes).
const pgNoDataPath = "/api/stock/win_rate?symbol=C6BNONE&rolling_window=" + pgTestWindow

// parityEndpointCases covers all four endpoints, the condition filter, the
// regime stratum, the industry ranking, the industry_id filter and the no-data
// path.
func parityEndpointCases(t *testing.T) []string {
	t.Helper()
	industryID, ok := ResolveCanonicalL1Industry(pgTestMappedSymbol)
	if !ok {
		t.Fatalf("fixture symbol %s has no canonical L1 industry; the industry cases would be vacuous", pgTestMappedSymbol)
	}
	return []string{
		pgNoDataPath,
		"/api/stock/win_rate?symbol=" + pgTestSymbol + "&rolling_window=" + pgTestWindow,
		"/api/stock/win_rate?symbol=" + pgTestSymbol + "&rolling_window=" + pgTestWindow + "&condition_id=" + pgTestCondBottom,
		"/api/stock/win_rate?symbol=" + pgTestSymbol2 + "&rolling_window=" + pgTestWindow,
		"/api/stock/condition_winrate?condition_id=" + pgTestCondBottom + "&rolling_window=" + pgTestOutcomeWindow,
		"/api/stock/condition_winrate?condition_id=" + pgTestCondBottom + "&rolling_window=" + pgTestOutcomeWindow + "&regime=RISK_ON",
		"/api/stock/condition_winrate?condition_id=" + pgTestCondTop + "&rolling_window=" + pgTestOutcomeWindow,
		"/api/stock/condition_winrate?condition_id=" + pgTestCondMapped + "&rolling_window=" + pgTestOutcomeWindow,
		"/api/stock/industry_winrate?condition_id=" + pgTestCondMapped + "&rolling_window=" + pgTestOutcomeWindow,
		"/api/stock/industry_winrate?condition_id=" + pgTestCondMapped + "&rolling_window=" + pgTestOutcomeWindow + "&industry_id=" + industryID,
		"/api/stock/industry_winrate?condition_id=" + pgTestCondBottom + "&rolling_window=" + pgTestOutcomeWindow,
		"/api/stock/family-expectancy",
		"/api/stock/family-expectancy?trailing_days=30",
	}
}

// winRateAllFourDeps wires the same source into the four endpoint slots (the
// production wiring does exactly this).
func winRateAllFourDeps(src *WinRateSource) Deps {
	provider := NewWinRateDBProvider(src)
	return Deps{
		WinRate:          provider,
		ConditionWinRate: provider,
		IndustryWinRate:  provider,
		FamilyExpectancy: provider,
	}
}

// winRateEndpointMux registers the four endpoints with a fixed clock (the
// family-expectancy response embeds generated_at; parity needs one instant) and
// the same shared.Get wrapper RegisterRoutes uses.
func winRateEndpointMux(deps Deps, now time.Time) *http.ServeMux {
	h := NewHandler(deps)
	h.nowFunc = func() time.Time { return now }
	mux := http.NewServeMux()
	mux.Handle("GET /api/stock/win_rate", shared.Get(h.HandleWinRate))
	mux.Handle("GET /api/stock/condition_winrate", shared.Get(h.HandleConditionWinRate))
	mux.Handle("GET /api/stock/industry_winrate", shared.Get(h.HandleIndustryWinRate))
	mux.Handle("GET /api/stock/family-expectancy", shared.Get(h.HandleFamilyExpectancy))
	return mux
}

// serveWinRateEndpoint returns the raw response body, failing on anything but
// 200: a comparison between two error bodies would be a false green.
func serveWinRateEndpoint(t *testing.T, mux *http.ServeMux, path string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d body = %s", path, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

// pgTestFixedNow is the clock both backends' handlers use.
func pgTestFixedNow(t *testing.T) time.Time {
	t.Helper()
	now, err := time.Parse(time.RFC3339, pgTestGeneratedAt)
	if err != nil {
		t.Fatalf("parse fixed clock: %v", err)
	}
	return now
}

// newWinRatePGSrc builds the production PostgreSQL source for a read-only handle
// with no SQLite fallback, so a broken PostgreSQL fails the test loudly instead
// of silently comparing SQLite against SQLite.
func newWinRatePGSrc(t *testing.T, readOnly *sql.DB) *WinRateSource {
	t.Helper()
	src := NewWinRateSource(readOnly, "")
	t.Cleanup(func() { _ = src.Close() })
	return src
}

// pgWinRateFixture returns the writable handle over the migrated database the
// fixture is seeded through. The fixture rows are removed before and after the
// test.
func pgWinRateFixture(t *testing.T) (pgReadOnly, pgWritable *sql.DB) {
	t.Helper()
	pgWritable = stdlib.OpenDBFromPool(testdb.Pool(t, "../../sql/migrations"))
	t.Cleanup(func() { _ = pgWritable.Close() })
	deleteWinRateFixture(t, pgWritable)
	t.Cleanup(func() { deleteWinRateFixture(t, pgWritable) })

	readOnly, err := OpenReadOnlyWinRatePGDB(testdb.URL(t))
	if err != nil {
		t.Fatalf("OpenReadOnlyWinRatePGDB: %v", err)
	}
	t.Cleanup(func() { _ = readOnly.Close() })
	return readOnly, pgWritable
}

// deleteWinRateFixture removes exactly the rows this file writes: the fixture
// window in stock_win_rate, the fixture sources, and — for the two real
// condition sources — only the fixture symbols.
func deleteWinRateFixture(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `DELETE FROM stock_win_rate WHERE rolling_window = $1`, pgTestWindow); err != nil {
		t.Fatalf("clean stock_win_rate: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`DELETE FROM stock_signal_outcomes WHERE source = $1 OR (source IN ($2, $3) AND symbol IN ($4, $5))`,
		pgTestSourceMapped, pgTestSource, pgTestSourceTop, pgTestSymbol, pgTestSymbol2); err != nil {
		t.Fatalf("clean stock_signal_outcomes: %v", err)
	}
}

// seedWinRateFixture writes the fixture into PostgreSQL: three eligible
// summaries over two symbols and two condition families, the outcome rows that
// produce data_start/data_end and the condition aggregates, and one industry
// fixture (a source no producer writes, over two real tickers with distinct
// canonical L1 industries) so the industry ranking and its industry_id filter
// carry data.
//
// Every row gets its own trigger_date: the outcome reads order by
// (trigger_date, source) only, so two rows sharing both keys would leave the
// row order — and therefore the float summation order — up to the backend, and
// the byte comparison would be comparing an accident.
func seedWinRateFixture(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	dates := pgTestDates()
	summaries := []struct {
		symbol       string
		source       string
		observations int
		hits         int
		winRate      float64
		avgForward   float64
		breakEven    float64
	}{
		{pgTestSymbol, pgTestSource, 40, 30, 0.75, 0.012, 0.020},
		{pgTestSymbol, pgTestSourceTop, 30, 9, 0.30, -0.008, 0.010},
		{pgTestSymbol2, pgTestSource, 25, 16, 0.64, 0.006, 0.015},
	}
	for _, s := range summaries {
		summary := pgWinRateSummary(s.symbol, s.source)
		summary.Observations, summary.Hits, summary.WinRate = s.observations, s.hits, s.winRate
		summary.AvgForwardReturn, summary.BreakEvenCostRate = s.avgForward, s.breakEven
		if err := stockpicker.NewWinRateStore(db).SaveWinRate(ctx, summary); err != nil {
			t.Fatalf("seed win rate %s/%s: %v", s.symbol, s.source, err)
		}
	}
	outcomes := []stockpicker.SignalOutcome{
		{Symbol: pgTestSymbol, TriggerDate: dates[0], Source: pgTestSource, ForwardReturn: 0.02, NetForwardReturn: 0.01415, Hit: true, CostRate: 0.00585, Regime: "RISK_ON", CreatedAt: "2026-10-13T00:00:00Z"},
		{Symbol: pgTestSymbol, TriggerDate: dates[1], Source: pgTestSource, ForwardReturn: -0.01, NetForwardReturn: -0.01585, Hit: false, CostRate: 0.00585, Regime: "RISK_ON", CreatedAt: "2026-10-13T00:00:00Z"},
		{Symbol: pgTestSymbol2, TriggerDate: dates[2], Source: pgTestSource, ForwardReturn: 0.03, NetForwardReturn: 0.02415, Hit: true, CostRate: 0.00585, Regime: "RISK_OFF", CreatedAt: "2026-10-13T00:00:00Z"},
		{Symbol: pgTestSymbol, TriggerDate: dates[3], Source: pgTestSourceTop, ForwardReturn: -0.02, NetForwardReturn: -0.02585, Hit: false, CostRate: 0.00585, Regime: "", CreatedAt: "2026-10-13T00:00:00Z"},
		{Symbol: pgTestMappedSymbol, TriggerDate: dates[4], Source: pgTestSourceMapped, ForwardReturn: 0.04, NetForwardReturn: 0.03415, Hit: true, CostRate: 0.00585, Regime: "RISK_ON", CreatedAt: "2026-10-13T00:00:00Z"},
		{Symbol: pgTestMappedSymbol, TriggerDate: dates[5], Source: pgTestSourceMapped, ForwardReturn: -0.03, NetForwardReturn: -0.03585, Hit: false, CostRate: 0.00585, Regime: "RISK_ON", CreatedAt: "2026-10-13T00:00:00Z"},
		{Symbol: pgTestMappedSymbol2, TriggerDate: dates[6], Source: pgTestSourceMapped, ForwardReturn: 0.01, NetForwardReturn: 0.00415, Hit: true, CostRate: 0.00585, Regime: "RISK_OFF", CreatedAt: "2026-10-13T00:00:00Z"},
	}
	if err := stockpicker.NewSignalOutcomeStore(db).RecordOutcomes(ctx, outcomes); err != nil {
		t.Fatalf("seed outcomes: %v", err)
	}
}

// mirrorWinRateStores copies the two win-rate tables from PostgreSQL into a
// fresh SQLite ledger and returns its path. The ledger is a snapshot taken after
// seeding, so the parity claim covers whatever else the shared database holds —
// and the negative test diverges PostgreSQL after the snapshot, which is what
// makes the comparison directional.
func mirrorWinRateStores(t *testing.T, pg *sql.DB) (path string, db *sql.DB) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "atlas.db")
	db, err := ledger.OpenSQLiteDB(path)
	if err != nil {
		t.Fatalf("open sqlite mirror %s: %v", path, err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := ledger.InitSchema(db); err != nil {
		t.Fatalf("init sqlite mirror schema: %v", err)
	}

	ctx := context.Background()
	for _, table := range []struct {
		name    string
		selectQ string
		insertQ string
		cols    int
	}{
		{
			name:    "stock_win_rate",
			selectQ: `SELECT symbol, source, rolling_window, observations, hits, win_rate, wilson_lower, wilson_upper, confidence, calibration_status, net_cost_rate, avg_forward_return, break_even_cost_rate, updated_at FROM stock_win_rate`,
			insertQ: `INSERT INTO stock_win_rate (symbol, source, rolling_window, observations, hits, win_rate, wilson_lower, wilson_upper, confidence, calibration_status, net_cost_rate, avg_forward_return, break_even_cost_rate, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) ON CONFLICT DO NOTHING`,
			cols:    14,
		},
		{
			name:    "stock_signal_outcomes",
			selectQ: `SELECT symbol, trigger_date, source, forward_return, net_forward_return, hit, cost_rate, regime, created_at FROM stock_signal_outcomes`,
			insertQ: `INSERT INTO stock_signal_outcomes (symbol, trigger_date, source, forward_return, net_forward_return, hit, cost_rate, regime, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT DO NOTHING`,
			cols:    9,
		},
	} {
		rows, err := pg.QueryContext(ctx, table.selectQ)
		if err != nil {
			t.Fatalf("read %s from postgres: %v", table.name, err)
		}
		n := 0
		for rows.Next() {
			args := make([]any, table.cols)
			dest := make([]any, table.cols)
			for i := range dest {
				dest[i] = &args[i]
			}
			if err := rows.Scan(dest...); err != nil {
				_ = rows.Close()
				t.Fatalf("scan %s row: %v", table.name, err)
			}
			if _, err := db.ExecContext(ctx, table.insertQ, args...); err != nil {
				_ = rows.Close()
				t.Fatalf("mirror %s row into sqlite: %v", table.name, err)
			}
			n++
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			t.Fatalf("iterate %s: %v", table.name, err)
		}
		_ = rows.Close()
		t.Logf("mirrored %d %s row(s) from postgres into the sqlite ledger", n, table.name)
	}
	return path, db
}

// pgWinRateSummary is one calibration-eligible summary with fixed columns, so
// the PostgreSQL/SQLite comparison has nothing incidental to differ on.
func pgWinRateSummary(symbol, source string) stockpicker.StockWinRateSummary {
	return stockpicker.StockWinRateSummary{
		Symbol:            symbol,
		Source:            source,
		Window:            pgTestWindow,
		Observations:      40,
		Hits:              30,
		WinRate:           0.75,
		WilsonLower:       0.60,
		WilsonUpper:       0.86,
		Confidence:        0.95,
		CalibrationStatus: stockpicker.CalibrationEligible,
		NetCostRate:       0.00585,
		AvgForwardReturn:  0.012,
		BreakEvenCostRate: 0.020,
		UpdatedAt:         "2026-08-27T12:00:00Z",
	}
}

// pgTestDates returns the fixture trigger dates, all inside the default rolling
// window and all distinct (see seedWinRateFixture). They are expressed relative
// to the run date: hard-coded dates silently age out of the window.
func pgTestDates() []string {
	now := time.Now()
	offsets := []int{70, 65, 60, 55, 50, 45, 40}
	out := make([]string, 0, len(offsets))
	for _, d := range offsets {
		out = append(out, now.AddDate(0, 0, -d).Format("2006-01-02"))
	}
	return out
}
