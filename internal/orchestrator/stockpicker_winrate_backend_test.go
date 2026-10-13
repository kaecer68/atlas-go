// Tests for the C6c read-backend decision of StockpickerWinrateExecutor
// (winRateStore): a usable PostgreSQL is read first when the deployment declares
// it, the read-only SQLite ledger is the fallback, and an injected
// store/opener stays ahead of both.
//
// No PostgreSQL server is needed. The stand-in for the PostgreSQL handle is a
// second in-memory SQLite ledger holding deliberately different numbers; the
// win-rate SQL text is identical on both backends ($N placeholders), so what
// these tests pin is WHICH store the executor reads. The observable is the
// recommendation itself, which is why the two ledgers are seeded to disagree:
// one side holds a row that clears every eligibility gate, the other a
// sub-threshold row, so reading the wrong store cannot produce the right
// answer. A real server is covered by
// stockpicker_winrate_backend_integration_test.go (-tags=integration).
package orchestrator

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/stockpicker"
	"github.com/kaecer68/atlas-go/internal/stocktools"
	"github.com/kaecer68/atlas-go/internal/testdb"
)

const (
	// c6cUnreachableDSN points at a closed port: the handle opens (lazy dial)
	// but every probe fails — the "postgres declared, postgres down" branch.
	c6cUnreachableDSN = "postgres://atlas:atlas@127.0.0.1:1/atlas_c6c_unreachable?sslmode=disable"
	// c6cStandInDSN is only a decision input: the recorder substitutes the
	// handle, so nothing dials it. It stays DSN-shaped so the inputs the
	// executor sees look like production's.
	c6cStandInDSN = "postgres://atlas:atlas@127.0.0.1:1/atlas_c6c_standin?sslmode=disable"
)

// ── win-rate fixtures ─────────────────────────────────────────────────

// c6cWinRateSummary is the executor's own fixture summary with the numbers
// under test (2330 / testRetainedSource / 120d, calibration eligible).
func c6cWinRateSummary(observations, hits int, winRate, wilsonLower float64) stockpicker.StockWinRateSummary {
	s := eligibleWinRateSummary()
	s.Observations, s.Hits = observations, hits
	s.WinRate, s.WilsonLower = winRate, wilsonLower
	return s
}

// c6cPassingSummary clears every eligibility gate of the executor
// (observations >= 30, win_rate >= 0.55, wilson_lower >= 0.45).
func c6cPassingSummary() stockpicker.StockWinRateSummary {
	return c6cWinRateSummary(40, 26, 0.65, 0.50)
}

// c6cPostgresSummary also clears every gate, with DIFFERENT numbers: the
// recommendation's reason then names the store that was read (40 vs 60
// observations), so a test can tell the two ledgers apart without inspecting
// handles.
func c6cPostgresSummary() stockpicker.StockWinRateSummary {
	return c6cWinRateSummary(60, 42, 0.70, 0.58)
}

// c6cFailingSummary is below the win-rate gate: a store holding this row yields
// no recommendation, which is the falsifier for "the executor read the other
// store".
func c6cFailingSummary() stockpicker.StockWinRateSummary {
	return c6cWinRateSummary(40, 12, 0.30, 0.22)
}

// ── store fixtures ────────────────────────────────────────────────────

// c6cSQLiteLedger writes a file-backed SQLite ledger holding summary and
// returns its path (the shape DBPath takes). The handle is closed before
// returning, so the executor — not the test — owns the connection it opens.
func c6cSQLiteLedger(t *testing.T, summary stockpicker.StockWinRateSummary) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "atlas.db")
	db := c6cOpenLedger(t, path)
	if err := stockpicker.SaveWinRate(context.Background(), db, summary); err != nil {
		t.Fatalf("seed sqlite win rate: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close ledger: %v", err)
	}
	return path
}

// c6cInMemoryLedger returns a schema-initialised in-memory ledger: the handle
// an injected OpenDB gets, and the base of the PostgreSQL stand-in.
func c6cInMemoryLedger(t *testing.T) *sql.DB {
	t.Helper()
	db := c6cOpenLedger(t, ":memory:")
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func c6cOpenLedger(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := ledger.OpenSQLiteDB(path)
	if err != nil {
		t.Fatalf("open sqlite %s: %v", path, err)
	}
	if err := ledger.InitSchema(db); err != nil {
		_ = db.Close()
		t.Fatalf("init schema %s: %v", path, err)
	}
	return db
}

// c6cStandInPGHandle returns a handle that stands in for the read-only
// PostgreSQL handle of stocktools.WinRateSource: it answers the resolver's probe
// (Ping succeeds, so the source resolves it as the read backend) and serves the
// win-rate reads. A nil summary leaves the store reachable but EMPTY — the
// "empty is data, not an outage" branch.
func c6cStandInPGHandle(t *testing.T, summary *stockpicker.StockWinRateSummary) *sql.DB {
	t.Helper()
	db := c6cInMemoryLedger(t)
	if summary != nil {
		if err := stockpicker.SaveWinRate(context.Background(), db, *summary); err != nil {
			t.Fatalf("seed stand-in postgres: %v", err)
		}
	}
	return db
}

// c6cUnreachablePGHandle is a real *sql.DB whose pool never dials successfully:
// the resolver's probe fails, so the source falls back to SQLite — with no
// server, no network access and no timeout.
func c6cUnreachablePGHandle(t *testing.T) *sql.DB {
	t.Helper()
	db := stdlib.OpenDBFromPool(testdb.UnconnectedPool(t, c6cUnreachableDSN))
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// ── OpenSource seam ───────────────────────────────────────────────────

// c6cSourceCall is one invocation of the OpenSource seam: the decision inputs
// the executor produced.
type c6cSourceCall struct {
	pgURL    string
	declared string
	path     string
}

// c6cSourceRecorder is the OpenSource seam. It records the arguments the
// executor passes to the source constructor — the decision, asserted directly
// in every case — and answers with a source over the test's stand-in PostgreSQL
// handle, so the read itself can also be observed.
type c6cSourceRecorder struct {
	pg *sql.DB

	mu    sync.Mutex
	calls []c6cSourceCall
}

func (r *c6cSourceRecorder) open(pgURL, declaredBackend, sqlitePath string) *stocktools.WinRateSource {
	r.mu.Lock()
	r.calls = append(r.calls, c6cSourceCall{pgURL: pgURL, declared: declaredBackend, path: sqlitePath})
	r.mu.Unlock()
	// The same rule as stocktools.openWinRatePG: only a postgres declaration
	// with a DSN wires a PostgreSQL handle (declaredBackend arrives normalized
	// by the executor). Every test asserts the recorded arguments, so this
	// substitution cannot hide a wrong decision behind a passing read.
	if pgURL == "" || declaredBackend != stocktools.WinRateBackendPostgres {
		return stocktools.NewWinRateSource(nil, sqlitePath)
	}
	return stocktools.NewWinRateSource(r.pg, sqlitePath)
}

func (r *c6cSourceRecorder) recorded() []c6cSourceCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]c6cSourceCall(nil), r.calls...)
}

// c6cExecutor builds the executor under test with the flow gate satisfied
// (fresh, passing foreign net flow), so the win-rate read is the only thing that
// decides the outcome.
func c6cExecutor(open func(pgURL, declaredBackend, sqlitePath string) *stocktools.WinRateSource, storeBackend, pgURL, dbPath string) StockpickerWinrateExecutor {
	return StockpickerWinrateExecutor{
		Source:       testRetainedSource,
		StoreBackend: storeBackend,
		PGURL:        pgURL,
		DBPath:       dbPath,
		OpenSource:   open,
		FlowSource:   mockFlowSource{net: 50000, ok: true},
		Gateway:      testFlowGateway(),
	}
}

// c6cRecommend runs the executor for the default fixture quote (2330.TW) and
// returns the recommendation flag plus its reason (empty when there is no
// recommendation).
func c6cRecommend(t *testing.T, e StockpickerWinrateExecutor) (bool, string) {
	t.Helper()
	return c6cRecommendFor(t, e, stockpickerWinrateQuote())
}

// c6cRecommendFor runs the executor for an explicit quote, so a fixture keyed to
// another symbol (the isolation symbols the integration tests use) is actually
// the symbol the read asks for.
func c6cRecommendFor(t *testing.T, e StockpickerWinrateExecutor, quote domain.Quote) (bool, string) {
	t.Helper()
	rec, ok := e.Recommend(stockpickerWinrateAgent(), quote, "", domain.Regime(""), nil)
	return ok, rec.Reason
}

// c6cAssertCall pins the decision inputs of a single seam invocation.
func c6cAssertCall(t *testing.T, calls []c6cSourceCall, wantPGURL, wantDeclared, wantPath string) {
	t.Helper()
	if len(calls) != 1 {
		t.Fatalf("OpenSource called %d times, want exactly 1 (per read); calls=%+v", len(calls), calls)
	}
	got := calls[0]
	if got.pgURL != wantPGURL || got.declared != wantDeclared || got.path != wantPath {
		t.Fatalf("OpenSource(pgURL=%q, declaredBackend=%q, sqlitePath=%q), want (%q, %q, %q)",
			got.pgURL, got.declared, got.path, wantPGURL, wantDeclared, wantPath)
	}
}

// ── decision table ────────────────────────────────────────────────────

// TestStockpickerWinrateWinRateStoreBackendDecision pins the ordered decision:
//
//	postgres + DSN ⇒ PostgreSQL first; anything else (jsonl/sqlite/unset, or
//	postgres without a DSN) ⇒ the SQLite ledger; a declared backend no backend
//	implements ⇒ an error, never a silent fallback.
//
// Both stores are seeded so that the two possible answers are distinguishable:
// the SQLite ledger holds the sub-threshold row, the PostgreSQL stand-in the
// passing one. wantRec is therefore a statement about WHICH store answered.
func TestStockpickerWinrateWinRateStoreBackendDecision(t *testing.T) {
	sqlitePath := c6cSQLiteLedger(t, c6cFailingSummary())

	cases := []struct {
		name         string
		storeBackend string
		pgURL        string
		envBackend   string
		envPGURL     string

		wantSeam     bool
		wantPGURL    string
		wantDeclared string
		wantErr      string
		wantRec      bool
	}{
		{
			name:         "declared postgres with a DSN reads postgres",
			storeBackend: "postgres", pgURL: c6cStandInDSN,
			wantSeam: true, wantPGURL: c6cStandInDSN, wantDeclared: "postgres", wantRec: true,
		},
		{
			// The C6b/C6 symmetry rule: no DSN means the PostgreSQL handle is
			// never wired, so the read stays on the SQLite ledger (and says so
			// in the source's log line).
			name:         "declared postgres without a DSN falls back to sqlite",
			storeBackend: "postgres", pgURL: "",
			wantSeam: true, wantPGURL: "", wantDeclared: "postgres", wantRec: false,
		},
		{
			name:         "declared sqlite never reads postgres even with a DSN",
			storeBackend: "sqlite", pgURL: c6cStandInDSN,
			wantSeam: true, wantPGURL: c6cStandInDSN, wantDeclared: "sqlite", wantRec: false,
		},
		{
			name:         "declared jsonl never reads postgres even with a DSN",
			storeBackend: "jsonl", pgURL: c6cStandInDSN,
			wantSeam: true, wantPGURL: c6cStandInDSN, wantDeclared: "jsonl", wantRec: false,
		},
		{
			// An unset declaration is the repository's config default (jsonl),
			// which has no win-rate read implementation and maps to SQLite.
			name:         "unset declared backend resolves to jsonl (sqlite)",
			storeBackend: "", pgURL: c6cStandInDSN,
			wantSeam: true, wantPGURL: c6cStandInDSN, wantDeclared: "jsonl", wantRec: false,
		},
		{
			name:         "environment supplies both DATABASE_URL and the declaration",
			storeBackend: "", pgURL: "",
			envBackend: "postgres", envPGURL: c6cStandInDSN,
			wantSeam: true, wantPGURL: c6cStandInDSN, wantDeclared: "postgres", wantRec: true,
		},
		{
			name:         "unknown declared backend is an error",
			storeBackend: "mysql", pgURL: c6cStandInDSN,
			wantSeam: false, wantErr: "mysql", wantRec: false,
		},
		{
			name:         "unknown environment backend is an error",
			storeBackend: "", pgURL: "",
			envBackend: "mysql", envPGURL: c6cStandInDSN,
			wantSeam: false, wantErr: "mysql", wantRec: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Hermetic environment: the fields under test are the only source
			// of the decision unless the case supplies env values.
			t.Setenv("DATABASE_URL", tc.envPGURL)
			t.Setenv("ATLAS_STORE_BACKEND", tc.envBackend)

			rec := &c6cSourceRecorder{pg: c6cStandInPGHandle(t, ptrSummary(c6cPostgresSummary()))}
			e := c6cExecutor(rec.open, tc.storeBackend, tc.pgURL, sqlitePath)

			if tc.wantErr != "" {
				// A rejected declaration must be reported, not silently mapped
				// onto another store — and it must not reach the source builder.
				_, closer, err := e.winRateStore()
				closer()
				if err == nil {
					t.Fatalf("winRateStore accepted ATLAS_STORE_BACKEND=%q; want an error naming the value", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("winRateStore error = %q, want it to name %q", err, tc.wantErr)
				}
				if calls := rec.recorded(); len(calls) != 0 {
					t.Fatalf("OpenSource called %d times for a rejected declaration; calls=%+v", len(calls), calls)
				}
			}

			// The observable: the store that answered decides the outcome, and
			// the seam records the decision inputs the read used.
			ok, reason := c6cRecommend(t, e)
			if ok != tc.wantRec {
				t.Fatalf("Recommend = %v (reason %q), want %v — the wrong store answered", ok, reason, tc.wantRec)
			}
			if ok && !strings.Contains(reason, "observations=60") {
				t.Fatalf("reason %q does not carry the PostgreSQL row (observations=60)", reason)
			}
			if tc.wantSeam {
				c6cAssertCall(t, rec.recorded(), tc.wantPGURL, tc.wantDeclared, sqlitePath)
			}
		})
	}
}

// ptrSummary returns a pointer to a copy, for the stand-in seeder.
func ptrSummary(s stockpicker.StockWinRateSummary) *stockpicker.StockWinRateSummary { return &s }

// ── injection priority ────────────────────────────────────────────────

// TestStockpickerWinrateInjectedReadersWinOverTheBackendDecision pins step 1
// and step 2 of the decision: an injected WinRateStore, and an injected OpenDB,
// both keep their existing semantics — they replace the read path even when the
// deployment declares postgres with a usable DSN and the PostgreSQL store would
// answer differently.
//
// The control case runs the same fixtures WITHOUT the injection, so the
// assertions below cannot pass just because the fixtures yield no
// recommendation.
func TestStockpickerWinrateInjectedReadersWinOverTheBackendDecision(t *testing.T) {
	sqlitePath := c6cSQLiteLedger(t, c6cFailingSummary()) // the injected OpenDB handle disagrees with postgres
	openDB := c6cInMemoryLedger(t)
	if err := stockpicker.SaveWinRate(context.Background(), openDB, c6cFailingSummary()); err != nil {
		t.Fatalf("seed injected handle: %v", err)
	}

	t.Run("control: no injection reads the passing postgres row", func(t *testing.T) {
		rec := &c6cSourceRecorder{pg: c6cStandInPGHandle(t, ptrSummary(c6cPostgresSummary()))}
		ok, reason := c6cRecommend(t, c6cExecutor(rec.open, "postgres", c6cStandInDSN, sqlitePath))
		if !ok {
			t.Fatal("control: declared postgres with a usable DSN produced no recommendation; the injection assertions below would be vacuous")
		}
		if !strings.Contains(reason, "observations=60") {
			t.Fatalf("control reason = %q, want the postgres row (observations=60)", reason)
		}
	})

	t.Run("injected WinRateStore replaces the read path", func(t *testing.T) {
		rec := &c6cSourceRecorder{pg: c6cStandInPGHandle(t, ptrSummary(c6cPostgresSummary()))}
		e := c6cExecutor(rec.open, "postgres", c6cStandInDSN, sqlitePath)
		e.WinRateStore = &mockWinRateStore{summary: c6cFailingSummary(), found: true}

		if ok, _ := c6cRecommend(t, e); ok {
			t.Fatal("Recommend = true with an injected sub-threshold store; the injection must win over the postgres read")
		}
		if calls := rec.recorded(); len(calls) != 0 {
			t.Fatalf("an injected WinRateStore still built a win-rate source: %+v", calls)
		}
		if _, closer, err := e.winRateStore(); err != nil {
			t.Fatalf("winRateStore with an injected store: %v", err)
		} else {
			closer() // must be a no-op closer: the executor does not own an injected reader
		}
	})

	t.Run("injected OpenDB wins over the postgres declaration", func(t *testing.T) {
		rec := &c6cSourceRecorder{pg: c6cStandInPGHandle(t, ptrSummary(c6cPostgresSummary()))}
		e := c6cExecutor(rec.open, "postgres", c6cStandInDSN, sqlitePath)
		e.OpenDB = func(path string) (*sql.DB, error) {
			if path != sqlitePath {
				t.Fatalf("injected OpenDB received path %q, want %q", path, sqlitePath)
			}
			return openDB, nil
		}

		if ok, _ := c6cRecommend(t, e); ok {
			t.Fatal("Recommend = true with an injected sub-threshold handle; OpenDB must win over the postgres read")
		}
		if calls := rec.recorded(); len(calls) != 0 {
			t.Fatalf("an injected OpenDB still built a win-rate source: %+v", calls)
		}
	})
}

// ── negative proof ────────────────────────────────────────────────────

// TestStockpickerWinrateForcingSQLiteFlipsTheOutcome is the negative proof for
// "PostgreSQL first": the fixtures and the executor are identical except for the
// declaration, which is forced to sqlite — the mutation "always read SQLite".
// The observable must flip, and the seam arguments must show the other decision.
//
// Without this pair, a test asserting that a postgres declaration reads
// PostgreSQL could pass while the executor always read SQLite.
func TestStockpickerWinrateForcingSQLiteFlipsTheOutcome(t *testing.T) {
	sqlitePath := c6cSQLiteLedger(t, c6cFailingSummary())
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ATLAS_STORE_BACKEND", "")

	pgRecorder := &c6cSourceRecorder{pg: c6cStandInPGHandle(t, ptrSummary(c6cPostgresSummary()))}
	pgOK, pgReason := c6cRecommend(t, c6cExecutor(pgRecorder.open, "postgres", c6cStandInDSN, sqlitePath))
	c6cAssertCall(t, pgRecorder.recorded(), c6cStandInDSN, "postgres", sqlitePath)
	if !pgOK || !strings.Contains(pgReason, "observations=60") {
		t.Fatalf("declared postgres: Recommend = %v (reason %q), want the passing postgres row (observations=60)", pgOK, pgReason)
	}

	sqliteRecorder := &c6cSourceRecorder{pg: c6cStandInPGHandle(t, ptrSummary(c6cPostgresSummary()))}
	sqliteOK, _ := c6cRecommend(t, c6cExecutor(sqliteRecorder.open, "sqlite", c6cStandInDSN, sqlitePath))
	c6cAssertCall(t, sqliteRecorder.recorded(), c6cStandInDSN, "sqlite", sqlitePath)
	if sqliteOK {
		t.Fatal("declared sqlite: Recommend = true; forcing the decision to SQLite must flip the observable, " +
			"otherwise the postgres case above proves nothing about which store is read")
	}
}

// ── reachable but empty ───────────────────────────────────────────────

// TestStockpickerWinrateEmptyPostgresIsNotAnOutage pins the "empty is data, not
// an outage" rule inherited from C6b: a reachable PostgreSQL that holds no row
// for the key answers not-found, and the executor must NOT fall back to SQLite —
// an empty result is a measurement, and reading a second store would answer a
// different question.
func TestStockpickerWinrateEmptyPostgresIsNotAnOutage(t *testing.T) {
	passingPath := c6cSQLiteLedger(t, c6cPassingSummary())
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ATLAS_STORE_BACKEND", "")

	// The SQLite row really would pass the gates: without this, "no
	// recommendation" could be a property of the fixture instead of the
	// no-fallback rule.
	assertSQLiteRowPasses(t, passingPath)

	rec := &c6cSourceRecorder{pg: c6cStandInPGHandle(t, nil)} // reachable, no rows
	e := c6cExecutor(rec.open, "postgres", c6cStandInDSN, passingPath)

	ok, reason := c6cRecommend(t, e)
	if ok {
		t.Fatalf("Recommend = true (reason %q) with an empty but reachable postgres: the executor fell back to SQLite", reason)
	}
	c6cAssertCall(t, rec.recorded(), c6cStandInDSN, "postgres", passingPath)
}

// assertSQLiteRowPasses proves the SQLite fixture clears the executor's gates by
// reading it directly.
func assertSQLiteRowPasses(t *testing.T, path string) {
	t.Helper()
	db, err := stocktools.OpenWinRateDB(path)
	if err != nil {
		t.Fatalf("open ledger %s: %v", path, err)
	}
	defer func() { _ = db.Close() }()
	summary, found, err := stockpicker.NewWinRateStore(db).LoadWinRate(context.Background(), "2330", testRetainedSource, "120d")
	if err != nil || !found {
		t.Fatalf("SQLite fixture row: found=%v err=%v, want a stored row", found, err)
	}
	if summary.CalibrationStatus != stockpicker.CalibrationEligible ||
		summary.Observations < stockpickerWinrateMinObservations ||
		summary.WinRate < stockpickerWinrateMinWinRate ||
		summary.WilsonLower < stockpickerWinrateMinWilsonLower {
		t.Fatalf("SQLite fixture row %+v does not clear the executor's gates; the empty-postgres assertion would be vacuous", summary)
	}
}

// ── unavailable postgres ──────────────────────────────────────────────

// TestStockpickerWinrateFallsBackToSQLiteWhenPostgresUnusable pins the fallback:
// a PostgreSQL that cannot answer the probe must not remove the recommendation
// path — the executor reads the SQLite ledger instead, exactly as it did before
// C6c, and the reason carries that ledger's row (40 observations, not the
// stand-in's 60).
func TestStockpickerWinrateFallsBackToSQLiteWhenPostgresUnusable(t *testing.T) {
	sqlitePath := c6cSQLiteLedger(t, c6cPassingSummary())
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ATLAS_STORE_BACKEND", "")

	rec := &c6cSourceRecorder{pg: c6cUnreachablePGHandle(t)}
	e := c6cExecutor(rec.open, "postgres", c6cStandInDSN, sqlitePath)

	ok, reason := c6cRecommend(t, e)
	if !ok {
		t.Fatal("Recommend = false with an unreachable postgres and a passing SQLite ledger; the fallback must keep the read path alive")
	}
	c6cAssertCall(t, rec.recorded(), c6cStandInDSN, "postgres", sqlitePath)
	if !strings.Contains(reason, "observations=40") || strings.Contains(reason, "observations=60") {
		t.Fatalf("reason = %q, want the SQLite row (observations=40), not the postgres stand-in's", reason)
	}
}

// ── the production constructor ────────────────────────────────────────

// TestStockpickerWinrateDefaultSourceWiring pins the wiring the executor uses
// when nothing is injected: the OpenSource seam is stocktools.OpenWinRateSource
// (C6b), so a declared postgres that is unreachable, unparsable or DSN-less
// degrades to the SQLite ledger instead of failing the read, and wiring never
// dials (an unreachable database must not break the run).
func TestStockpickerWinrateDefaultSourceWiring(t *testing.T) {
	sqlitePath := c6cSQLiteLedger(t, c6cPassingSummary())

	cases := []struct {
		name     string
		pgURL    string
		envPGURL string
		wantRec  bool
	}{
		{name: "unreachable DSN falls back to sqlite", pgURL: c6cUnreachableDSN, wantRec: true},
		{name: "unparsable DSN falls back to sqlite", pgURL: "not-a-dsn", wantRec: true},
		{name: "missing DSN falls back to sqlite", pgURL: "", envPGURL: "", wantRec: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", tc.envPGURL)
			t.Setenv("ATLAS_STORE_BACKEND", "")

			// OpenSource nil: the production constructor, not a stand-in.
			e := c6cExecutor(nil, "postgres", tc.pgURL, sqlitePath)
			ok, reason := c6cRecommend(t, e)
			if ok != tc.wantRec {
				t.Fatalf("Recommend = %v (reason %q), want %v", ok, reason, tc.wantRec)
			}
			if ok && !strings.Contains(reason, "observations=40") {
				t.Fatalf("reason = %q, want the SQLite row (the postgres path must have been unusable)", reason)
			}
		})
	}
}
