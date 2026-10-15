package autobacktest

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

// Q2 batch 2, T2 — the auto-backtest runner (the daily producer of the window
// family) must build its store from the deployment's declaration and write the
// window summary through it.

// windowRecordingStore is a FullStore whose window-family writes are recorded
// instead of written to disk (the JSONL half keeps the reads working).
type windowRecordingStore struct {
	*ledger.Store
	summaries []domain.BacktestWindowSummary
	briefs    []string
}

func newWindowRecordingStore(dir string) *windowRecordingStore {
	return &windowRecordingStore{Store: ledger.NewStore(dir).(*ledger.Store)}
}

func (s *windowRecordingStore) RecordWindowSummary(summary domain.BacktestWindowSummary) error {
	s.summaries = append(s.summaries, summary)
	return nil
}

func (s *windowRecordingStore) RecordMutationBrief(windowID string, brief domain.MutationBrief) error {
	s.briefs = append(s.briefs, windowID)
	return nil
}

var _ ledger.FullStore = (*windowRecordingStore)(nil)

func autoBacktestCfg(dir string) config.Config {
	return config.Config{
		ReplayMode:        "daily",
		PrimaryMarket:     "TW",
		ReplayDataPath:    "../../samples/replay/twse_stock_day_all_sample.csv",
		LedgerDir:         dir,
		AgentRegistryPath: "../../configs/agents.json",
		ReplaySessionDate: "2026-03-26",
	}
}

func TestNewRunner_DeclaredBackendDecidesTheStore(t *testing.T) {
	// Declared postgres without an injected pool must fail loudly instead of
	// silently writing the window into the JSONL directory (the T1 lesson).
	ledger.SetPostgresPool(nil)
	t.Cleanup(func() { ledger.SetPostgresPool(nil) })
	postgresCfg := config.Config{LedgerDir: t.TempDir(), StoreBackend: "postgres"}
	if _, err := NewRunner(postgresCfg); err == nil {
		t.Error("NewRunner(postgres without pool) returned nil error; it must not fall back to JSONL")
	}
	if _, err := NewRunnerWithEventBus(postgresCfg, nil); err == nil {
		t.Error("NewRunnerWithEventBus(postgres without pool) returned nil error; it must not fall back to JSONL")
	}
	if _, err := NewRunner(config.Config{StoreBackend: "mysql"}); err == nil {
		t.Error("NewRunner(unknown backend) returned nil error")
	}

	// The jsonl declaration still works and is rooted at cfg.LedgerDir.
	dir := t.TempDir()
	cfg := autoBacktestCfg(dir)
	cfg.StoreBackend = "jsonl"
	runner, err := NewRunner(cfg)
	if err != nil {
		t.Fatalf("NewRunner(jsonl): %v", err)
	}
	if runner == nil || runner.btRunner == nil {
		t.Fatal("NewRunner(jsonl) returned an incomplete runner")
	}
}

func TestNewRunner_WindowWriteLandsInTheDeclaredDirectory(t *testing.T) {
	dir := t.TempDir()
	cfg := autoBacktestCfg(dir)
	cfg.StoreBackend = "jsonl"
	runner, err := NewRunner(cfg)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	start, _ := time.Parse("2006-01-02", "2026-03-26")
	end, _ := time.Parse("2006-01-02", "2026-03-27")
	summary, err := runner.btRunner.Run(start, end)
	if err != nil {
		t.Fatalf("run window: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "windows", summary.WindowID+".json")); statErr != nil {
		t.Fatalf("window summary file missing under the declared LedgerDir: %v", statErr)
	}
}

func TestNewRunnerWithStore_WindowWriteGoesThroughTheInjectedStore(t *testing.T) {
	dir := t.TempDir()
	store := newWindowRecordingStore(dir)
	// The declaration stays consistent with the process (orchestrator.NewSystem
	// also builds stores from it); the injected store is what the backtest
	// runner uses for the window family.
	cfg := autoBacktestCfg(dir)
	cfg.StoreBackend = "jsonl"

	runner := NewRunnerWithStore(cfg, store)
	start, _ := time.Parse("2006-01-02", "2026-03-26")
	end, _ := time.Parse("2006-01-02", "2026-03-27")
	summary, err := runner.btRunner.Run(start, end)
	if err != nil {
		t.Fatalf("run window: %v", err)
	}

	if len(store.summaries) != 1 || store.summaries[0].WindowID != summary.WindowID {
		t.Fatalf("recorded window summaries = %+v, want the injected store to receive %s", store.summaries, summary.WindowID)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "windows", summary.WindowID+".json")); !os.IsNotExist(statErr) {
		t.Fatalf("window summary also landed in the JSONL directory (stat err %v); the injected store must be the only write path", statErr)
	}
}
