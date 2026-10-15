package service

import (
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/testdb"
)

// Q2 batch 2, T2 — the store BacktestService injects must carry the window
// WRITE half, not just the summary read.
//
// Before this unit, dashboard_api.go injected
// ledger.NewReportOutcomeStore(cfg) into svc.WithStore, and that store did not
// implement ledger.BacktestStore. backtest.Runner.Run type-asserts the store
// after running the whole simulation and returns
// "store does not implement BacktestStore" — so /api/backtest/start ran the
// window and then threw it away on a postgres deployment. This test pins the
// contract without a database (the unconnected pool parses the DSN and never
// dials, so no query is issued).

const hermeticPostgresDSN = "postgres://atlas:atlas@127.0.0.1:1/atlas_q2b2w_hermetic?sslmode=disable"

func TestBacktestService_InjectedStoreCarriesTheWindowWriteHalf(t *testing.T) {
	ledger.SetPostgresPool(testdb.UnconnectedPool(t, hermeticPostgresDSN))
	t.Cleanup(func() { ledger.SetPostgresPool(nil) })

	dir := t.TempDir()
	store, err := ledger.NewReportOutcomeStore(config.Config{LedgerDir: dir, StoreBackend: "postgres"})
	if err != nil {
		t.Fatalf("NewReportOutcomeStore(postgres): %v", err)
	}
	if _, ok := store.(ledger.BacktestStore); !ok {
		t.Fatalf("the store injected into BacktestService (%T) does not implement ledger.BacktestStore; "+
			"backtest.Runner.Run would run the full simulation and then fail without persisting the window", store)
	}

	svc := NewBacktestService(config.Config{LedgerDir: dir, StoreBackend: "postgres"}).WithStore(store)
	if svc.summariesStore() != store {
		t.Error("BacktestService.summariesStore() did not return the injected store")
	}
}

func TestBacktestService_WindowStoreFactoryMatchesTheInjectedStore(t *testing.T) {
	ledger.SetPostgresPool(testdb.UnconnectedPool(t, hermeticPostgresDSN))
	t.Cleanup(func() { ledger.SetPostgresPool(nil) })

	dir := t.TempDir()
	// The dashboard builds one store per concern from the same declaration
	// (dashboard_api.go RegisterRoutes / RegisterBacktestRoutes); the window
	// factory must not silently resolve to a different backend.
	windowStore, err := ledger.NewWindowStore(config.Config{LedgerDir: dir, StoreBackend: "postgres"})
	if err != nil {
		t.Fatalf("NewWindowStore(postgres): %v", err)
	}
	summaryStore, err := ledger.NewReportOutcomeStore(config.Config{LedgerDir: dir, StoreBackend: "postgres"})
	if err != nil {
		t.Fatalf("NewReportOutcomeStore(postgres): %v", err)
	}
	runnerStore, ok := summaryStore.(ledger.BacktestStore)
	if !ok {
		t.Fatal("NewReportOutcomeStore(postgres) does not satisfy BacktestStore")
	}
	if _, ok := windowStore.(ledger.BacktestStore); !ok {
		t.Fatal("NewWindowStore(postgres) does not satisfy BacktestStore")
	}
	if _, ok := runnerStore.(ledger.WindowSummaryReader); !ok {
		t.Fatal("the runner store cannot read window summaries back")
	}
	if _, err := ledger.NewWindowStore(config.Config{LedgerDir: filepath.Join(dir, "state"), StoreBackend: "jsonl"}); err != nil {
		t.Fatalf("NewWindowStore(jsonl): %v", err)
	}
}
