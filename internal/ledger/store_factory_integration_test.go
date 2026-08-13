package ledger

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestFactoryJSONLBackend(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{
		LedgerDir:    tmpDir,
		StoreBackend: "jsonl",
	}

	store, err := NewOutcomeStore(cfg)
	if err != nil {
		t.Fatalf("NewOutcomeStore(jsonl) failed: %v", err)
	}

	outcome := domain.RecommendationOutcome{
		AgentID:       "test-agent",
		Skill:         "sector-tech",
		Layer:         domain.LayerSector,
		Symbol:        "2330",
		Side:          domain.SideBuy,
		Conviction:    75,
		TargetPrice:   1100,
		StopLossPrice: 1000,
		Window:        "2026-01",
		ForwardReturn: 0.05,
		Hit:           true,
		Reason:        "test jsonl",
		Price:         1050,
		PassedGuards:  true,
		RecordedAt:    time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
	}

	if err := store.RecordOutcomes([]domain.RecommendationOutcome{outcome}); err != nil {
		t.Fatalf("RecordOutcome failed: %v", err)
	}

	outcomes, err := store.LoadOutcomes()
	if err != nil {
		t.Fatalf("LoadOutcomes failed: %v", err)
	}

	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
	if outcomes[0].Symbol != "2330" {
		t.Fatalf("expected symbol 2330, got %s", outcomes[0].Symbol)
	}
}

func TestFactorySQLiteBackend(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	cfg := config.Config{
		LedgerDir:    tmpDir,
		StoreBackend: "sqlite",
		SQLitePath:   dbPath,
	}

	store, err := NewOutcomeStore(cfg)
	if err != nil {
		t.Fatalf("NewOutcomeStore(sqlite) failed: %v", err)
	}

	outcome := domain.RecommendationOutcome{
		AgentID:       "test-agent-sqlite",
		Skill:         "sector-tech",
		Layer:         domain.LayerSector,
		Symbol:        "2454",
		Side:          domain.SideBuy,
		Conviction:    70,
		TargetPrice:   900,
		StopLossPrice: 800,
		Window:        "2026-02",
		ForwardReturn: 0.03,
		Hit:           true,
		Reason:        "test sqlite",
		Price:         850,
		PassedGuards:  true,
		RecordedAt:    time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
	}

	if err := store.RecordOutcomes([]domain.RecommendationOutcome{outcome}); err != nil {
		t.Fatalf("RecordOutcome failed: %v", err)
	}

	outcomes, err := store.LoadOutcomes()
	if err != nil {
		t.Fatalf("LoadOutcomes failed: %v", err)
	}

	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
	if outcomes[0].Symbol != "2454" {
		t.Fatalf("expected symbol 2454, got %s", outcomes[0].Symbol)
	}
}

func TestFactoryDefaultBackend(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{
		LedgerDir: tmpDir,
	}

	store, err := NewOutcomeStore(cfg)
	if err != nil {
		t.Fatalf("NewOutcomeStore(default) failed: %v", err)
	}

	outcome := domain.RecommendationOutcome{
		AgentID:       "test-agent-default",
		Skill:         "sector-tech",
		Layer:         domain.LayerSector,
		Symbol:        "2317",
		Side:          domain.SideBuy,
		Conviction:    65,
		TargetPrice:   200,
		StopLossPrice: 180,
		Window:        "2026-03",
		ForwardReturn: 0.02,
		Hit:           true,
		Reason:        "test default",
		Price:         190,
		PassedGuards:  true,
		RecordedAt:    time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
	}

	if err := store.RecordOutcomes([]domain.RecommendationOutcome{outcome}); err != nil {
		t.Fatalf("RecordOutcome failed: %v", err)
	}

	outcomes, err := store.LoadOutcomes()
	if err != nil {
		t.Fatalf("LoadOutcomes failed: %v", err)
	}

	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
	if outcomes[0].Symbol != "2317" {
		t.Fatalf("expected symbol 2317, got %s", outcomes[0].Symbol)
	}
}

func TestFactoryFullStoreJSONL(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{
		LedgerDir:    tmpDir,
		StoreBackend: "jsonl",
	}

	store, err := NewFullStore(cfg)
	if err != nil {
		t.Fatalf("NewFullStore(jsonl) failed: %v", err)
	}

	outcome := domain.RecommendationOutcome{
		AgentID:       "test-fullstore-jsonl",
		Skill:         "sector-tech",
		Layer:         domain.LayerSector,
		Symbol:        "2330",
		Side:          domain.SideBuy,
		Conviction:    75,
		TargetPrice:   1100,
		StopLossPrice: 1000,
		Window:        "2026-01",
		ForwardReturn: 0.05,
		Hit:           true,
		Reason:        "test fullstore jsonl",
		Price:         1050,
		PassedGuards:  true,
		RecordedAt:    time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
	}

	if err := store.RecordOutcomes([]domain.RecommendationOutcome{outcome}); err != nil {
		t.Fatalf("RecordOutcome failed: %v", err)
	}

	outcomes, err := store.LoadOutcomes()
	if err != nil {
		t.Fatalf("LoadOutcomes failed: %v", err)
	}

	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
	if outcomes[0].AgentID != "test-fullstore-jsonl" {
		t.Fatalf("expected agent test-fullstore-jsonl, got %s", outcomes[0].AgentID)
	}
}

func TestFactoryFullStoreSQLite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_fullstore.db")
	cfg := config.Config{
		LedgerDir:    tmpDir,
		StoreBackend: "sqlite",
		SQLitePath:   dbPath,
	}

	store, err := NewFullStore(cfg)
	if err != nil {
		t.Fatalf("NewFullStore(sqlite) failed: %v", err)
	}

	outcome := domain.RecommendationOutcome{
		AgentID:       "test-fullstore-sqlite",
		Skill:         "sector-tech",
		Layer:         domain.LayerSector,
		Symbol:        "2454",
		Side:          domain.SideBuy,
		Conviction:    70,
		TargetPrice:   900,
		StopLossPrice: 800,
		Window:        "2026-02",
		ForwardReturn: 0.03,
		Hit:           true,
		Reason:        "test fullstore sqlite",
		Price:         850,
		PassedGuards:  true,
		RecordedAt:    time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
	}

	if err := store.RecordOutcomes([]domain.RecommendationOutcome{outcome}); err != nil {
		t.Fatalf("RecordOutcome failed: %v", err)
	}

	outcomes, err := store.LoadOutcomes()
	if err != nil {
		t.Fatalf("LoadOutcomes failed: %v", err)
	}

	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
	if outcomes[0].AgentID != "test-fullstore-sqlite" {
		t.Fatalf("expected agent test-fullstore-sqlite, got %s", outcomes[0].AgentID)
	}
}

func TestFactoryInvalidSQLitePath(t *testing.T) {
	tmpDir := t.TempDir()
	invalidPath := filepath.Join(tmpDir, "nonexistent", "subdir", "test.db")
	cfg := config.Config{
		LedgerDir:    tmpDir,
		StoreBackend: "sqlite",
		SQLitePath:   invalidPath,
	}

	_, err := NewOutcomeStore(cfg)
	if err == nil {
		t.Fatalf("expected error for invalid SQLite path, got nil")
	}
}

func TestFactoryUnknownBackendFallsBackToJSONL(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{
		LedgerDir:    tmpDir,
		StoreBackend: "unknown_backend",
	}

	store, err := NewOutcomeStore(cfg)
	if err != nil {
		t.Fatalf("NewOutcomeStore(unknown) should not fail, got: %v", err)
	}

	outcome := domain.RecommendationOutcome{
		AgentID:       "test-fallback",
		Skill:         "sector-tech",
		Layer:         domain.LayerSector,
		Symbol:        "2330",
		Side:          domain.SideBuy,
		Conviction:    75,
		TargetPrice:   1100,
		StopLossPrice: 1000,
		Window:        "2026-01",
		ForwardReturn: 0.05,
		Hit:           true,
		Reason:        "fallback test",
		Price:         1050,
		PassedGuards:  true,
		RecordedAt:    time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
	}

	if err := store.RecordOutcomes([]domain.RecommendationOutcome{outcome}); err != nil {
		t.Fatalf("RecordOutcome failed: %v", err)
	}

	outcomes, err := store.LoadOutcomes()
	if err != nil {
		t.Fatalf("LoadOutcomes failed: %v", err)
	}

	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
}

func TestFactoryQuoteStorePostgresNoPoolError(t *testing.T) {
	// postgresPool is a package-global; save/restore so the nil-pool error
	// case is deterministic and does not depend on other tests.
	prev := postgresPool
	t.Cleanup(func() { postgresPool = prev })
	postgresPool = nil

	cfg := config.Config{StoreBackend: "postgres"}
	_, err := NewQuoteStore(cfg)
	if err == nil {
		t.Fatalf("expected nil-pool error for postgres backend, got nil")
	}
	if !strings.Contains(err.Error(), "SetPostgresPool") {
		t.Fatalf("error should mention SetPostgresPool, got: %v", err)
	}
}

func TestFactoryQuoteStorePostgresWithPool(t *testing.T) {
	pool := connectTestPG(t)
	prev := postgresPool
	t.Cleanup(func() { postgresPool = prev })
	postgresPool = pool

	cfg := config.Config{StoreBackend: "postgres"}
	store, err := NewQuoteStore(cfg)
	if err != nil {
		t.Fatalf("NewQuoteStore(postgres, pool) failed: %v", err)
	}
	if _, ok := store.(*PostgresQuoteStore); !ok {
		t.Fatalf("expected *PostgresQuoteStore, got %T", store)
	}
}
