package main

// store_test.go — hermetic tests for the calibrate-parameters ledger store
// wiring. No test here opens a real PostgreSQL connection: the pool is a
// never-dialing *pgxpool.Pool and the wiring dependencies are fakes.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain/recommendation"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

// uncalibratableTestDSN points at a closed local port, so an accidental dial
// fails fast instead of hanging. It carries no real credential.
const uncalibratableTestDSN = "postgres://atlas@127.0.0.1:1/atlas_test"

// newIdlePool returns a real *pgxpool.Pool that never dials: ParseConfig only
// parses and NewWithConfig connects lazily (minConns = 0), so the pool can be
// injected, injected-around and Closed without a PostgreSQL server.
func newIdlePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	poolCfg, err := pgxpool.ParseConfig(uncalibratableTestDSN)
	if err != nil {
		t.Fatalf("parse pool config: %v", err)
	}
	poolCfg.ConnConfig.ConnectTimeout = 2 * time.Second
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// forbidStoreWiring returns deps that fail the test if any wiring step runs —
// used to prove that a module which does not read the ledger stays inert.
func forbidStoreWiring(t *testing.T) storeDeps {
	t.Helper()
	return storeDeps{
		initPool: func(context.Context, string, string) (*pgxpool.Pool, error) {
			t.Error("initPool must not be called")
			return nil, errors.New("unexpected initPool call")
		},
		injectPool: func(*pgxpool.Pool) {
			t.Error("injectPool must not be called")
		},
		newStore: func(config.Config) (ledger.OutcomeStore, error) {
			t.Error("newStore must not be called")
			return nil, errors.New("unexpected newStore call")
		},
	}
}

func testOutcome() recommendation.RecommendationOutcome {
	return recommendation.RecommendationOutcome{
		AgentID:       "agent-a",
		Symbol:        "2330",
		Side:          "buy",
		Conviction:    5,
		Window:        "1d",
		Hit:           true,
		ForwardReturn: 0.01,
	}
}

// TestResolveCalibrationStore_JSONLBackendReadsTheCalibrationMirror pins that the
// store is anchored on <workDir>/data/state — the directory the darwinian
// calibrator actually reads — and not on some other ledger dir.
func TestResolveCalibrationStore_JSONLBackendReadsTheCalibrationMirror(t *testing.T) {
	workDir := t.TempDir()

	store, closer, err := resolveCalibrationStore(context.Background(), "darwinian", workDir,
		config.Config{StoreBackend: "jsonl"}, defaultStoreDeps())
	if err != nil {
		t.Fatalf("resolveCalibrationStore: %v", err)
	}
	defer closer()
	if store == nil {
		t.Fatal("jsonl backend must return a usable store")
	}

	if err := store.RecordOutcomes([]recommendation.RecommendationOutcome{testOutcome()}); err != nil {
		t.Fatalf("RecordOutcomes: %v", err)
	}
	mirror := filepath.Join(workDir, "data", "state", "recommendation_outcomes.jsonl")
	if _, err := os.Stat(mirror); err != nil {
		t.Fatalf("mirror %s was not written: %v", mirror, err)
	}

	loaded, err := store.LoadOutcomes()
	if err != nil {
		t.Fatalf("LoadOutcomes: %v", err)
	}
	if len(loaded) != 1 || loaded[0].AgentID != "agent-a" {
		t.Fatalf("round-trip = %+v, want the single recorded outcome", loaded)
	}
}

// TestResolveCalibrationStore_SQLiteBackend pins the sqlite branch: a store on the
// configured SQLitePath, with no pool and no DSN involved.
func TestResolveCalibrationStore_SQLiteBackend(t *testing.T) {
	workDir := t.TempDir()
	sqlitePath := filepath.Join(t.TempDir(), "atlas.db")

	store, closer, err := resolveCalibrationStore(context.Background(), "darwinian", workDir,
		config.Config{StoreBackend: "sqlite", SQLitePath: sqlitePath}, defaultStoreDeps())
	if err != nil {
		t.Fatalf("resolveCalibrationStore: %v", err)
	}
	if store == nil {
		t.Fatal("sqlite backend must return a usable store")
	}
	closer() // must be a no-op (the sqlite handle is shared/process-wide)

	if _, ok := store.(*ledger.SQLiteOutcomeStore); !ok {
		t.Fatalf("store type = %T, want *ledger.SQLiteOutcomeStore", store)
	}
	if err := store.RecordOutcomes([]recommendation.RecommendationOutcome{testOutcome()}); err != nil {
		t.Fatalf("RecordOutcomes: %v", err)
	}
	if _, err := os.Stat(sqlitePath); err != nil {
		t.Fatalf("sqlite file %s: %v", sqlitePath, err)
	}
}

// TestResolveCalibrationStore_PostgresWithoutDSNFailsLoudly pins the no-silent-
// fallback rule, and that the run-level wrapper degrades to nil instead of
// failing the weekly job.
func TestResolveCalibrationStore_PostgresWithoutDSNFailsLoudly(t *testing.T) {
	workDir := t.TempDir()
	deps := forbidStoreWiring(t)

	_, closer, err := resolveCalibrationStore(context.Background(), "darwinian", workDir,
		config.Config{StoreBackend: "postgres"}, deps)
	if err == nil {
		t.Fatal("backend postgres without DATABASE_URL must be an explicit error, not a silent jsonl fallback")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("error = %v, want it to name DATABASE_URL", err)
	}
	if !strings.Contains(err.Error(), "postgres") {
		t.Fatalf("error = %v, want it to name the resolved backend", err)
	}
	if closer == nil {
		t.Fatal("closer must never be nil")
	}
	closer()

	store, runCloser := calibrationStoreForRun(context.Background(), "darwinian", workDir,
		config.Config{StoreBackend: "postgres"}, deps)
	if store != nil {
		t.Fatalf("store = %v, want nil (degrade to the JSONL mirror instead of failing)", store)
	}
	if runCloser == nil {
		t.Fatal("run closer must never be nil")
	}
	runCloser() // no-op must not panic
}

// TestResolveCalibrationStore_PostgresOpensInjectsThenBuilds pins the postgres
// order: open the pool → inject it into the ledger factory → build the store, and
// that the returned closer closes exactly that pool.
func TestResolveCalibrationStore_PostgresOpensInjectsThenBuilds(t *testing.T) {
	workDir := t.TempDir()
	pool := newIdlePool(t)

	var order []string
	var injected *pgxpool.Pool
	deps := storeDeps{
		initPool: func(_ context.Context, dsn, migrationsPath string) (*pgxpool.Pool, error) {
			order = append(order, "init")
			if dsn != uncalibratableTestDSN {
				t.Error("initPool received an unexpected DSN")
			}
			if want := filepath.Join(workDir, "sql", "migrations"); migrationsPath != want {
				t.Errorf("migrationsPath = %q, want %q", migrationsPath, want)
			}
			return pool, nil
		},
		injectPool: func(p *pgxpool.Pool) {
			order = append(order, "inject")
			injected = p
			ledger.SetPostgresPool(p)
		},
		newStore: func(cfg config.Config) (ledger.OutcomeStore, error) {
			order = append(order, "store")
			if cfg.StoreBackend != "postgres" {
				t.Errorf("store backend = %q, want postgres", cfg.StoreBackend)
			}
			if want := filepath.Join(workDir, "data", "state"); cfg.LedgerDir != want {
				t.Errorf("LedgerDir = %q, want %q", cfg.LedgerDir, want)
			}
			return ledger.NewOutcomeStore(cfg)
		},
	}
	t.Cleanup(func() { ledger.SetPostgresPool(nil) })

	store, closer, err := resolveCalibrationStore(context.Background(), "darwinian", workDir,
		config.Config{
			StoreBackend:   "postgres",
			DatabaseURL:    uncalibratableTestDSN,
			MigrationsPath: migrationsRelDir,
		}, deps)
	if err != nil {
		t.Fatalf("resolveCalibrationStore: %v", err)
	}
	if !reflect.DeepEqual(order, []string{"init", "inject", "store"}) {
		t.Fatalf("wiring order = %v, want [init inject store]", order)
	}
	if injected != pool {
		t.Error("injectPool did not receive the pool returned by initPool")
	}
	if _, ok := store.(*ledger.PostgresLedgerStore); !ok {
		t.Fatalf("store type = %T, want *ledger.PostgresLedgerStore", store)
	}
	if closer == nil {
		t.Fatal("postgres branch must return a closer that shuts the pool down")
	}

	// The closer must really close the pool: a closed pgxpool rejects Acquire
	// immediately with puddle.ErrClosedPool ("closed pool") instead of dialing.
	closer()
	acquireCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := pool.Acquire(acquireCtx); err == nil || !strings.Contains(err.Error(), "closed pool") {
		t.Fatalf("Acquire after closer() = %v, want the closed-pool error", err)
	}
}

// TestResolveCalibrationStore_PostgresKeepsAbsoluteMigrationsPath pins that an
// absolute MigrationsPath is used as-is.
func TestResolveCalibrationStore_PostgresKeepsAbsoluteMigrationsPath(t *testing.T) {
	workDir := t.TempDir()
	pool := newIdlePool(t)
	absMigrations := filepath.Join(t.TempDir(), "migrations")

	var gotMigrations string
	deps := storeDeps{
		initPool: func(_ context.Context, _ string, migrationsPath string) (*pgxpool.Pool, error) {
			gotMigrations = migrationsPath
			return pool, nil
		},
		injectPool: func(p *pgxpool.Pool) { ledger.SetPostgresPool(p) },
		newStore:   ledger.NewOutcomeStore,
	}
	t.Cleanup(func() { ledger.SetPostgresPool(nil) })

	_, closer, err := resolveCalibrationStore(context.Background(), "darwinian", workDir,
		config.Config{StoreBackend: "postgres", DatabaseURL: uncalibratableTestDSN, MigrationsPath: absMigrations}, deps)
	if err != nil {
		t.Fatalf("resolveCalibrationStore: %v", err)
	}
	defer closer()
	if gotMigrations != absMigrations {
		t.Fatalf("migrationsPath = %q, want the absolute %q", gotMigrations, absMigrations)
	}
}

// TestResolveCalibrationStore_UnknownBackendIsAnError pins that a misconfigured
// backend is named, never rounded down to jsonl.
func TestResolveCalibrationStore_UnknownBackendIsAnError(t *testing.T) {
	workDir := t.TempDir()
	deps := forbidStoreWiring(t)

	_, closer, err := resolveCalibrationStore(context.Background(), "darwinian", workDir,
		config.Config{StoreBackend: "mysql"}, deps)
	if err == nil || !strings.Contains(err.Error(), "mysql") {
		t.Fatalf("error = %v, want it to name the unknown backend", err)
	}
	if closer == nil {
		t.Fatal("closer must never be nil")
	}
	closer()

	store, runCloser := calibrationStoreForRun(context.Background(), "darwinian", workDir,
		config.Config{StoreBackend: "mysql"}, deps)
	if store != nil {
		t.Fatalf("store = %v, want nil on an unknown backend", store)
	}
	runCloser()
}

// TestResolveCalibrationStore_OtherModulesStayInert pins that garch/var/factor do
// not resolve a backend, read a DSN or open a pool.
func TestResolveCalibrationStore_OtherModulesStayInert(t *testing.T) {
	deps := forbidStoreWiring(t)
	for _, module := range []string{"garch", "var", "factor"} {
		store, closer, err := resolveCalibrationStore(context.Background(), module, t.TempDir(),
			config.Config{StoreBackend: "postgres"}, deps)
		if err != nil {
			t.Fatalf("%s: resolveCalibrationStore = %v, want no error and no store", module, err)
		}
		if store != nil {
			t.Fatalf("%s: store = %v, want nil", module, store)
		}
		if closer == nil {
			t.Fatalf("%s: closer must never be nil", module)
		}
		closer()
	}

	// "all" includes darwinian, so it must resolve a store (and therefore fail
	// loudly here for the same reason as darwinian).
	_, _, err := resolveCalibrationStore(context.Background(), "all", t.TempDir(),
		config.Config{StoreBackend: "postgres"}, deps)
	if err == nil {
		t.Fatal("module all includes darwinian: postgres without DATABASE_URL must error")
	}
}
