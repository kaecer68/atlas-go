package stocktools

// Hermetic coverage of the C6b read-path resolution (PostgreSQL vs. SQLite) and
// of the properties the PostgreSQL path must hold: the SQL it sends is valid for
// PostgreSQL ($N placeholders, strictly ascending — positional binding is what
// makes one SQL text work on both drivers) and every statement it sends is a
// read.
//
// No PostgreSQL server is needed: a driver-recording stub stands in for the
// PostgreSQL handle (Ping succeeds, so the resolver selects it, and the SQL
// becomes inspectable), a real in-memory SQLite ledger covers handle selection,
// testdb.UnconnectedPool covers the unreachable-PostgreSQL fallback, and the
// file-backed SQLite ledger covers the fallback source. The real PostgreSQL
// behaviour (byte-for-byte endpoint parity, read-only session) is pinned by
// win_rate_pg_integration_test.go.

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"

	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/testdb"
)

const (
	// unreachableDSN points at a closed port: the handle opens (lazy dial) but
	// every probe fails, which is the "PostgreSQL declared, PostgreSQL down"
	// branch at request/startup-adjacent time.
	unreachableDSN = "postgres://atlas:atlas@127.0.0.1:1/atlas_c6b_unreachable?sslmode=disable"
)

// --- backend decision --------------------------------------------------------

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
// win-rate backend through the wrapper, so the HTTP read path can never
// disagree with the writers about what a declared backend means.
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
// "cannot start the HTTP server" failure, because WinRateSource.Handle probes
// the handle per call and falls back to SQLite in the meantime.
func TestOpenReadOnlyWinRatePGDBDoesNotDial(t *testing.T) {
	db, err := OpenReadOnlyWinRatePGDB(unreachableDSN)
	if err != nil {
		t.Fatalf("OpenReadOnlyWinRatePGDB(unreachable): %v (want a lazy handle, not an error)", err)
	}
	defer func() { _ = db.Close() }()
	if err := db.PingContext(context.Background()); err == nil {
		t.Fatal("Ping succeeded against an unreachable DSN; the test DSN is not actually unreachable")
	}
}

// --- wiring decision ---------------------------------------------------------

// TestOpenWinRateSourceWiringDecision pins the startup contract: the endpoints
// get a PostgreSQL handle only when DATABASE_URL is set AND the declared store
// backend normalizes to postgres. Every other combination leaves the handle nil,
// which is what makes the SQLite ledger the read path — and therefore must never
// happen silently: openWinRatePG returns the reason, and OpenWinRateSource logs
// it.
func TestOpenWinRateSourceWiringDecision(t *testing.T) {
	cases := []struct {
		name       string
		pgURL      string
		declared   string
		wantHandle bool
		wantReason string
	}{
		{name: "no DATABASE_URL", pgURL: "", declared: "postgres", wantReason: "DATABASE_URL not set"},
		{name: "declared postgres", pgURL: unreachableDSN, declared: "postgres", wantHandle: true},
		{name: "declared sqlite", pgURL: unreachableDSN, declared: "sqlite", wantReason: `ATLAS_STORE_BACKEND="sqlite" does not declare postgres`},
		{name: "declared jsonl", pgURL: unreachableDSN, declared: "jsonl", wantReason: `ATLAS_STORE_BACKEND="jsonl" does not declare postgres`},
		{name: "backend unset (legacy jsonl default)", pgURL: unreachableDSN, declared: "", wantReason: "ATLAS_STORE_BACKEND=\"(unset)\" does not declare postgres"},
		{name: "unknown backend", pgURL: unreachableDSN, declared: "mysql", wantReason: `ATLAS_STORE_BACKEND="mysql" is not a known backend`},
		{name: "unparsable DSN", pgURL: "not-a-dsn", declared: "postgres", wantReason: "postgres handle:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pg, reason := openWinRatePG(tc.pgURL, tc.declared)
			if tc.wantHandle {
				if pg == nil {
					t.Fatalf("openWinRatePG(%q, %q) wired no handle; want the read-only postgres handle", tc.pgURL, tc.declared)
				}
				t.Cleanup(func() { _ = pg.Close() })
				// Wiring must not dial: an unreachable PostgreSQL is handled by
				// the per-call probe + SQLite fallback, not by a startup crash.
				if err := pg.PingContext(context.Background()); err == nil {
					t.Fatal("wired handle answered a ping for an unreachable DSN; the test DSN is not actually unreachable")
				}
			} else {
				if pg != nil {
					t.Fatal("openWinRatePG wired a postgres handle; want the SQLite read path")
				}
				if !strings.Contains(reason, tc.wantReason) {
					t.Fatalf("reason = %q, want it to contain %q", reason, tc.wantReason)
				}
			}

			// The same decision through the production entry point, and the
			// startup log it emits: the operator must be able to tell which
			// store the endpoints will read.
			logs := captureWinRateLogs(t)
			src := OpenWinRateSource(tc.pgURL, tc.declared, "/tmp/atlas-test.db")
			t.Cleanup(func() { _ = src.Close() })
			if tc.wantHandle != (src.pg != nil) {
				t.Fatalf("OpenWinRateSource wired pg=%v, want %v", src.pg != nil, tc.wantHandle)
			}
			var joined string
			for _, line := range *logs {
				joined += line + "\n"
			}
			if !strings.Contains(joined, "stocktools: win-rate reads:") {
				t.Fatalf("OpenWinRateSource logged nothing about the read path; lines=%v", *logs)
			}
			if tc.wantHandle {
				if !strings.Contains(joined, "PostgreSQL first") || !strings.Contains(joined, "/tmp/atlas-test.db") {
					t.Fatalf("plan log does not name PostgreSQL and the SQLite fallback: %q", joined)
				}
			} else if !strings.Contains(joined, "SQLite ledger") || !strings.Contains(joined, reason) {
				t.Fatalf("plan log does not name the SQLite read path and the reason (%q): %q", reason, joined)
			}
		})
	}
}

// TestLoggableBackendValue: the reason string embeds ATLAS_STORE_BACKEND, so the
// value is reduced to an enum-shaped token first — an environment variable must
// not be able to write arbitrary text into the log.
func TestLoggableBackendValue(t *testing.T) {
	cases := []struct{ in, want string }{
		{in: "", want: "(unset)"},
		{in: "postgres", want: "postgres"},
		{in: "sqlite", want: "sqlite"},
		{in: "some_backend-2.0", want: "some_backend-2.0"},
		{in: "postgres\nlevel=ERROR forged", want: "(contains characters outside [A-Za-z0-9_.-])"},
		{in: "postgres%0a", want: "(contains characters outside [A-Za-z0-9_.-])"},
		{in: strings.Repeat("a", 33), want: "(too long)"},
	}
	for _, tc := range cases {
		if got := loggableBackendValue(tc.in); got != tc.want {
			t.Errorf("loggableBackendValue(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// --- handle selection --------------------------------------------------------

// TestWinRateSourcePrefersPostgresHandle pins the resolution order: a usable
// PostgreSQL handle wins over a wired SQLite ledger holding different data.
//
// The stand-in handle is another SQLite ledger because the win-rate SQL text is
// identical on both backends ($N placeholders); what this test pins is *which*
// handle the endpoints read, plus the caching rule. Real PostgreSQL behaviour is
// the integration test's job.
func TestWinRateSourcePrefersPostgresHandle(t *testing.T) {
	sqlitePath := writeWinRateTestLedger(t, func(ctx context.Context, db *sql.DB) {
		seedWinRate(t, db, sampleWinRateSummary("2330", "stockpicker-foreign-3d-net-buy", "120d"))
	})
	pgStandIn := openWinRateTestDB(t)
	diverged := sampleWinRateSummary("2330", "stockpicker-foreign-3d-net-buy", "120d")
	diverged.Observations, diverged.Hits = 9, 7
	seedWinRate(t, pgStandIn, diverged)

	logs := &[]string{}
	src := NewWinRateSource(pgStandIn, sqlitePath)
	src.logf = recorderInto(logs)
	t.Cleanup(func() { _ = src.Close() })

	rec, out := getWinRate(t, Deps{WinRate: NewWinRateDBProvider(src)}, "/api/stock/win_rate?symbol=2330")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !out.Found || len(out.Conditions) != 1 {
		t.Fatalf("found=%v conditions=%d, want the postgres row: %+v", out.Found, len(out.Conditions), out.Conditions)
	}
	if got := out.Conditions[0]; got.Observations != 9 || got.Hits != 7 {
		t.Fatalf("observations/hits = %d/%d, want 9/7 (the postgres handle), not 40/24 (SQLite)", got.Observations, got.Hits)
	}

	resolved := src.resolvedHandle(t)
	if resolved == nil || resolved.backend != WinRateBackendPostgres {
		t.Fatalf("resolved handle = %+v, want the postgres handle cached", resolved)
	}
	assertLoggedBackend(t, *logs, "PostgreSQL")
}

// TestWinRateSourceFallsBackToSQLiteWhenPostgresUnreachable pins the backward
// compatibility half of C6b: an unusable PostgreSQL handle must not fail the
// endpoints — they read the SQLite ledger, exactly as before this change. And
// the fallback must NOT be cached as a decision, so a PostgreSQL that comes back
// is picked up without restarting the server.
func TestWinRateSourceFallsBackToSQLiteWhenPostgresUnreachable(t *testing.T) {
	sqlitePath := writeWinRateTestLedger(t, func(ctx context.Context, db *sql.DB) {
		seedWinRate(t, db, sampleWinRateSummary("2330", "stockpicker-foreign-3d-net-buy", "120d"))
	})

	logs := &[]string{}
	src := NewWinRateSource(unreachableWinRateHandle(t), sqlitePath)
	src.logf = recorderInto(logs)
	t.Cleanup(func() { _ = src.Close() })
	deps := Deps{WinRate: NewWinRateDBProvider(src)}

	rec, out := getWinRate(t, deps, "/api/stock/win_rate?symbol=2330")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !out.Found || len(out.Conditions) != 1 {
		t.Fatalf("found=%v conditions=%d, want the SQLite fallback row: %+v", out.Found, len(out.Conditions), out.Conditions)
	}
	if got := out.Conditions[0]; got.Observations != 40 || got.Hits != 24 {
		t.Fatalf("observations/hits = %d/%d, want the SQLite 40/24", got.Observations, got.Hits)
	}
	if resolved := src.resolvedHandle(t); resolved != nil {
		t.Fatalf("resolved = %+v while PostgreSQL was unreachable; want no cached decision", resolved)
	}
	assertLoggedBackend(t, *logs, "SQLite ledger")
	if joined := strings.Join(*logs, "\n"); !strings.Contains(joined, "postgres handle unusable") {
		t.Fatalf("fallback was not logged with its cause: %q", joined)
	}

	// PostgreSQL comes back on the same handle slot (a restart of the database,
	// not of the HTTP server): the next request must switch over.
	revived := openWinRateTestDB(t)
	diverged := sampleWinRateSummary("2330", "stockpicker-foreign-3d-net-buy", "120d")
	diverged.Observations, diverged.Hits = 9, 7
	seedWinRate(t, revived, diverged)
	src.mu.Lock()
	src.pg = revived
	src.mu.Unlock()

	_, out = getWinRate(t, deps, "/api/stock/win_rate?symbol=2330")
	if got := out.Conditions[0]; got.Observations != 9 || got.Hits != 7 {
		t.Fatalf("observations/hits = %d/%d after PostgreSQL came back, want 9/7 (no restart, no stale SQLite decision)", got.Observations, got.Hits)
	}
	if resolved := src.resolvedHandle(t); resolved == nil || resolved.backend != WinRateBackendPostgres {
		t.Fatalf("resolved = %+v, want the postgres decision once PostgreSQL answers", resolved)
	}
}

// TestWinRateSourceCachesOnlySuccessfulPostgres: once PostgreSQL has been
// resolved, later calls do not re-probe it, so a PostgreSQL that goes down
// mid-flight keeps serving from the cached (read-only) handle instead of
// silently switching store under the reader.
func TestWinRateSourceCachesOnlySuccessfulPostgres(t *testing.T) {
	pgStandIn := openWinRateTestDB(t)
	diverged := sampleWinRateSummary("2330", "stockpicker-foreign-3d-net-buy", "120d")
	diverged.Observations, diverged.Hits = 9, 7
	seedWinRate(t, pgStandIn, diverged)

	logs := &[]string{}
	// The SQLite path is deliberately absent: if the cached decision were
	// dropped, the request would fail instead of reading another store.
	src := NewWinRateSource(pgStandIn, filepath.Join(t.TempDir(), "absent.db"))
	src.logf = recorderInto(logs)
	t.Cleanup(func() { _ = src.Close() })
	deps := Deps{WinRate: NewWinRateDBProvider(src)}

	if _, out := getWinRate(t, deps, "/api/stock/win_rate?symbol=2330"); !out.Found {
		t.Fatal("first request did not read the postgres stand-in")
	}

	src.mu.Lock()
	src.pg = unreachableWinRateHandle(t)
	src.mu.Unlock()

	rec, out := getWinRate(t, deps, "/api/stock/win_rate?symbol=2330")
	if rec.Code != http.StatusOK || !out.Found {
		t.Fatalf("status=%d found=%v body=%s; want the cached postgres handle", rec.Code, out.Found, rec.Body.String())
	}
	if joined := strings.Join(*logs, "\n"); strings.Contains(joined, "postgres handle unusable") {
		t.Fatalf("a resolved postgres handle was re-probed (and logged as unusable): %q", joined)
	}
}

// TestWinRateSourceSQLiteOnlyCachesDecision: in a deployment that declares no
// postgres at all there is nothing to re-probe, so the SQLite decision is
// cached — the ledger is not reopened (nor re-logged) per request.
func TestWinRateSourceSQLiteOnlyCachesDecision(t *testing.T) {
	sqlitePath := writeWinRateTestLedger(t, func(ctx context.Context, db *sql.DB) {
		seedWinRate(t, db, sampleWinRateSummary("2330", "stockpicker-foreign-3d-net-buy", "120d"))
	})
	src := NewWinRateSource(nil, sqlitePath)
	t.Cleanup(func() { _ = src.Close() })
	deps := Deps{WinRate: NewWinRateDBProvider(src)}

	if _, out := getWinRate(t, deps, "/api/stock/win_rate?symbol=2330"); !out.Found {
		t.Fatal("first request did not read the SQLite ledger")
	}
	resolved := src.resolvedHandle(t)
	if resolved == nil || resolved.backend != WinRateBackendSQLite {
		t.Fatalf("resolved = %+v, want the sqlite decision cached", resolved)
	}

	// The handle is cached, not the path: removing the file afterwards still
	// serves the open (read-only) handle.
	if err := os.Remove(sqlitePath); err != nil {
		t.Fatalf("remove ledger: %v", err)
	}
	if _, out := getWinRate(t, deps, "/api/stock/win_rate?symbol=2330"); !out.Found {
		t.Fatal("second request reopened the ledger instead of using the cached handle")
	}
}

// TestWinRateEndpointsReportBothCausesWhenNeitherUsable: with an unreachable
// PostgreSQL and no SQLite ledger, every one of the four endpoints must answer
// 503 with a message that names BOTH causes. Reporting only the SQLite half
// would send an operator with a PostgreSQL outage hunting for a SQLite file.
func TestWinRateEndpointsReportBothCausesWhenNeitherUsable(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.db")
	src := NewWinRateSource(unreachableWinRateHandle(t), missing)
	t.Cleanup(func() { _ = src.Close() })
	provider := NewWinRateDBProvider(src)
	deps := Deps{
		WinRate:          provider,
		ConditionWinRate: provider,
		IndustryWinRate:  provider,
		FamilyExpectancy: provider,
	}

	paths := []string{
		"/api/stock/win_rate?symbol=2330",
		"/api/stock/condition_winrate?condition_id=price-volume-bottom-divergence",
		"/api/stock/industry_winrate?condition_id=price-volume-bottom-divergence",
		"/api/stock/family-expectancy",
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, deps)
	for _, path := range paths {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s: status = %d body=%s, want 503", path, rec.Code, rec.Body.String())
		}
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: unmarshal %s: %v", path, rec.Body.String(), err)
		}
		for _, want := range []string{"postgres win-rate ledger unusable", missing} {
			if !strings.Contains(body["error"], want) {
				t.Fatalf("%s: error = %q, want it to mention %q", path, body["error"], want)
			}
		}
	}
}

// TestWinRateEndpointsFallBackWhenSQLitePathIsUnset: no PostgreSQL and no
// configured SQLite path is the C6b analogue of the old "store not configured"
// 503 — descriptive, not a panic, and still on all four endpoints.
func TestWinRateEndpointsFallBackWhenSQLitePathIsUnset(t *testing.T) {
	src := NewWinRateSource(nil, "")
	t.Cleanup(func() { _ = src.Close() })
	provider := NewWinRateDBProvider(src)
	mux := http.NewServeMux()
	RegisterRoutes(mux, Deps{
		WinRate:          provider,
		ConditionWinRate: provider,
		IndustryWinRate:  provider,
		FamilyExpectancy: provider,
	})

	for _, path := range []string{
		"/api/stock/win_rate?symbol=2330",
		"/api/stock/condition_winrate?condition_id=price-volume-bottom-divergence",
		"/api/stock/industry_winrate?condition_id=price-volume-bottom-divergence",
		"/api/stock/family-expectancy",
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s: status = %d body=%s, want 503", path, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "ATLAS_MCP_STOCKPICKER_DB") {
			t.Fatalf("%s: body %s does not name the missing configuration", path, rec.Body.String())
		}
	}
}

// TestWinRateSourceHandleWithoutDatabase: a provider built with neither a
// source nor a handle is a wiring bug, and it must say so instead of panicking
// on a nil *sql.DB.
func TestWinRateSourceHandleWithoutDatabase(t *testing.T) {
	provider := &WinRateDBProvider{}
	if _, err := provider.Sources(context.Background(), "2330", "120d"); err == nil {
		t.Fatal("Sources succeeded with no database handle")
	}
	if _, _, err := provider.LoadConditionWinRate(context.Background(), "stockpicker-x", "120d", ""); err == nil {
		t.Fatal("LoadConditionWinRate succeeded with no database handle")
	}
	if _, _, err := provider.LoadFamilyExpectancy(context.Background(), 60, "2026-10-13T00:00:00Z"); err == nil {
		t.Fatal("LoadFamilyExpectancy succeeded with no database handle")
	}
	if _, _, err := provider.LoadWinRate(context.Background(), "2330", "stockpicker-x", "120d"); err == nil {
		t.Fatal("LoadWinRate succeeded with no database handle")
	}
	if _, _, ok := provider.OutcomeDateRange(context.Background(), "2330", "stockpicker-x"); ok {
		t.Fatal("OutcomeDateRange reported data with no database handle")
	}
}

// --- dialect + read-only contract on the PostgreSQL path ---------------------

// TestWinRatePostgresPathSendsPostgresSQLAndOnlyReads drives all four endpoints
// through a recording handle that stands in for PostgreSQL. The claim under test
// is the one that makes "the same SQL runs on both backends" checkable: every
// statement uses $N placeholders in ascending order, and none of them writes.
func TestWinRatePostgresPathSendsPostgresSQLAndOnlyReads(t *testing.T) {
	rec, handle := newRecordingWinRateHandle(t)
	src := NewWinRateSource(handle, "")
	t.Cleanup(func() { _ = src.Close() })
	provider := NewWinRateDBProvider(src)
	mux := http.NewServeMux()
	RegisterRoutes(mux, Deps{
		WinRate:          provider,
		ConditionWinRate: provider,
		IndustryWinRate:  provider,
		FamilyExpectancy: provider,
	})

	// Every query builder in the four endpoints, so a missing "$" or an
	// accidental "?" in any one of them fails this test:
	//   /api/stock/win_rate            → Sources + LoadWinRate + LoadOutcomes
	//   /api/stock/condition_winrate   → LoadOutcomes
	//   /api/stock/industry_winrate    → LoadOutcomes
	//   /api/stock/family-expectancy   → LoadOutcomes
	for _, path := range []string{
		"/api/stock/win_rate?symbol=2330",
		"/api/stock/condition_winrate?condition_id=price-volume-bottom-divergence",
		"/api/stock/industry_winrate?condition_id=price-volume-bottom-divergence",
		"/api/stock/family-expectancy",
	} {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s: status = %d body=%s", path, recorder.Code, recorder.Body.String())
		}
	}

	queries := rec.recorded()
	if len(queries) == 0 {
		t.Fatal("no SQL reached the recording handle; the test would pass vacuously")
	}
	joined := strings.Join(queries, "\n")
	for _, want := range []string{"SELECT DISTINCT source", "FROM stock_win_rate", "FROM stock_signal_outcomes"} {
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

	// The same SQL text must actually run on SQLite — the claim is "one text,
	// two backends", not "PostgreSQL-shaped text". Sources is the only builder
	// whose SQL lives in this package, so it is the one this test drives; the
	// stockpicker builders are shared with the MCP read path (C6).
	sqliteDB := openWinRateTestDB(t)
	seedWinRate(t, sqliteDB, sampleWinRateSummary("2330", "stockpicker-foreign-3d-net-buy", "120d"))
	sources, err := NewWinRateDBProviderForHandle(sqliteDB).Sources(context.Background(), "2330", "120d")
	if err != nil {
		t.Fatalf("Sources on the SQLite ledger (same $N text as PostgreSQL): %v", err)
	}
	if len(sources) != 1 || sources[0] != "stockpicker-foreign-3d-net-buy" {
		t.Fatalf("Sources on SQLite = %v, want the seeded source", sources)
	}
}

var placeholderRe = regexp.MustCompile(`\$(\d+)`)

// assertAscendingPlaceholders checks the invariant that makes one SQL text valid
// on both drivers: the $N placeholders must appear in ascending order starting
// at 1. database/sql binds arguments positionally in order of appearance, so a
// gap (missing $2) or a reordering silently binds the wrong value — and on the
// SQLite driver, which turns these into named parameters, a repeat would bind
// one value twice.
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

// --- helpers -----------------------------------------------------------------

// writeWinRateTestLedger creates a file-backed SQLite ledger (the shape
// WinRateSource opens read-only), runs seed against it, and returns the path.
// The handle is closed before returning, so the source — not the test — owns the
// only open connection.
func writeWinRateTestLedger(t *testing.T, seed func(ctx context.Context, db *sql.DB)) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "atlas.db")
	db, err := ledger.OpenSQLiteDB(path)
	if err != nil {
		t.Fatalf("open sqlite ledger %s: %v", path, err)
	}
	if err := ledger.InitSchema(db); err != nil {
		_ = db.Close()
		t.Fatalf("init schema: %v", err)
	}
	if seed != nil {
		seed(context.Background(), db)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close ledger: %v", err)
	}
	return path
}

// unreachableWinRateHandle is a real *sql.DB whose connection pool never dials
// successfully: the resolver's probe fails, which is the "PostgreSQL declared
// but down" branch, with no server and no network access.
func unreachableWinRateHandle(t *testing.T) *sql.DB {
	t.Helper()
	db := stdlib.OpenDBFromPool(testdb.UnconnectedPool(t, unreachableDSN))
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// recorderInto returns a logf that appends formatted lines to lines. It is the
// observability seam: "the resolved backend is logged" must be assertable.
func recorderInto(lines *[]string) func(format string, args ...any) {
	var mu sync.Mutex
	return func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		*lines = append(*lines, fmt.Sprintf(format, args...))
	}
}

// captureWinRateLogs swaps the package log sink for a recorder and restores it
// when the test ends. The read path must not depend on the process log to be
// observable, but the startup plan line is emitted before a caller can install a
// per-source sink, so this is the seam for that one line.
func captureWinRateLogs(t *testing.T) *[]string {
	t.Helper()
	lines := &[]string{}
	prev := winRateLogf
	winRateLogf = recorderInto(lines)
	t.Cleanup(func() { winRateLogf = prev })
	return lines
}

// assertLoggedBackend asserts that at least one log line names the backend that
// actually served the read.
func assertLoggedBackend(t *testing.T, lines []string, want string) {
	t.Helper()
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, want) {
		t.Fatalf("no log line names the backend %q; lines=%q", want, joined)
	}
}

// resolvedHandle reads the cached resolution decision (nil = no decision
// cached).
func (s *WinRateSource) resolvedHandle(t *testing.T) *winRateReadHandle {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resolved
}

// --- recording driver --------------------------------------------------------

const winRateRecordingDriver = "stocktools_winrate_recording_stub"

var (
	registerWinRateRecordingDriver sync.Once
	// winRateRecorder is process-global because database/sql driver names are
	// registered once per process; newRecordingWinRateHandle resets it so each
	// test sees only its own statements.
	winRateRecorder = &winRateQueryRecorder{}
)

type winRateQueryRecorder struct {
	mu      sync.Mutex
	queries []string
	execs   []string
}

func (r *winRateQueryRecorder) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queries, r.execs = nil, nil
}

func (r *winRateQueryRecorder) addQuery(q string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queries = append(r.queries, q)
}

func (r *winRateQueryRecorder) addExec(q string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.execs = append(r.execs, q)
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

// newRecordingWinRateHandle returns a *sql.DB whose driver records every
// statement and answers with canned rows shaped like the win-rate schema. It
// stands in for the PostgreSQL handle in hermetic tests: Ping succeeds, so the
// resolver selects it, and the SQL the read path sends becomes inspectable.
func newRecordingWinRateHandle(t *testing.T) (*winRateQueryRecorder, *sql.DB) {
	t.Helper()
	registerWinRateRecordingDriver.Do(func() {
		sql.Register(winRateRecordingDriver, recordingDriver{rec: winRateRecorder})
	})
	winRateRecorder.reset()
	db, err := sql.Open(winRateRecordingDriver, "")
	if err != nil {
		t.Fatalf("open recording handle: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return winRateRecorder, db
}

type recordingDriver struct{ rec *winRateQueryRecorder }

func (d recordingDriver) Open(string) (driver.Conn, error) { return recordingConn(d), nil }

type recordingConn struct{ rec *winRateQueryRecorder }

func (c recordingConn) Prepare(query string) (driver.Stmt, error) {
	return recordingStmt{rec: c.rec, query: query}, nil
}

func (c recordingConn) Close() error { return nil }

func (c recordingConn) Begin() (driver.Tx, error) {
	return nil, errRecordingNoTx
}

var errRecordingNoTx = fmt.Errorf("recording driver: transactions are not supported (the win-rate read path must not open one)")

type recordingStmt struct {
	rec   *winRateQueryRecorder
	query string
}

func (s recordingStmt) Close() error  { return nil }
func (s recordingStmt) NumInput() int { return -1 }

func (s recordingStmt) Exec([]driver.Value) (driver.Result, error) {
	s.rec.addExec(s.query)
	return driver.RowsAffected(0), nil
}

func (s recordingStmt) Query([]driver.Value) (driver.Rows, error) {
	s.rec.addQuery(s.query)
	// Pointer receiver: the cursor must advance between Next calls, or every
	// reader loops forever on the first row.
	return &recordingRows{cols: cannedColumns(s.query), data: cannedData(s.query)}, nil
}

// cannedColumns returns the column names the read path scans for the statement
// it sent. Matching on the query text keeps the stub honest: a builder that
// changes the selected columns breaks the scan and fails the test instead of
// silently answering with the wrong shape.
func cannedColumns(query string) []string {
	switch {
	case strings.Contains(query, "SELECT DISTINCT source"):
		return []string{"source"}
	case strings.Contains(query, "FROM stock_win_rate"):
		return []string{
			"symbol", "source", "rolling_window", "observations", "hits", "win_rate",
			"wilson_lower", "wilson_upper", "confidence", "calibration_status",
			"net_cost_rate", "avg_forward_return", "break_even_cost_rate", "updated_at",
		}
	case strings.Contains(query, "FROM stock_signal_outcomes"):
		return []string{
			"symbol", "trigger_date", "source", "forward_return", "net_forward_return",
			"hit", "cost_rate", "regime", "created_at",
		}
	default:
		return nil
	}
}

func cannedData(query string) [][]driver.Value {
	switch {
	case strings.Contains(query, "SELECT DISTINCT source"):
		return [][]driver.Value{{"stockpicker-price-volume-bottom-divergence"}}
	case strings.Contains(query, "FROM stock_win_rate"):
		return [][]driver.Value{{
			"2330", "stockpicker-price-volume-bottom-divergence", "120d", int64(40), int64(30), 0.75,
			0.60, 0.86, 0.95, "eligible", 0.00585, 0.012, 0.020, "2026-08-27T12:00:00Z",
		}}
	case strings.Contains(query, "FROM stock_signal_outcomes"):
		return [][]driver.Value{{
			"2330", "2026-08-01", "stockpicker-price-volume-bottom-divergence", 0.02, 0.01415,
			int64(1), 0.00585, "", "2026-08-02T00:00:00Z",
		}}
	default:
		return nil
	}
}

type recordingRows struct {
	cols []string
	data [][]driver.Value
	pos  int
}

func (r *recordingRows) Columns() []string { return r.cols }
func (r *recordingRows) Close() error      { return nil }

func (r *recordingRows) Next(dest []driver.Value) error {
	if r.pos >= len(r.data) {
		return io.EOF
	}
	copy(dest, r.data[r.pos])
	r.pos++
	return nil
}
