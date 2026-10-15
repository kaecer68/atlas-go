package backtest

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

// Q2 batch 2, T2 — the window family (window summary + mutation brief) must be
// WRITTEN through the store the caller injected, not into the JSONL directory
// the runner happens to know. Otherwise a postgres deployment records its
// windows where the (now backend-aware) report / system-health readers cannot
// see them.

// recordingWindowStore is a JSONL store that records the window-family writes
// instead of writing the files: the JSONL half keeps LoadAllSessionScorecards
// working, while the two Record* methods make the write path observable.
type recordingWindowStore struct {
	*ledger.Store
	summaries []domain.BacktestWindowSummary
	briefs    []string
}

func newRecordingWindowStore(dir string) *recordingWindowStore {
	return &recordingWindowStore{Store: ledger.NewStore(dir).(*ledger.Store)}
}

func (s *recordingWindowStore) RecordWindowSummary(summary domain.BacktestWindowSummary) error {
	s.summaries = append(s.summaries, summary)
	return nil
}

func (s *recordingWindowStore) RecordMutationBrief(windowID string, brief domain.MutationBrief) error {
	s.briefs = append(s.briefs, windowID)
	return nil
}

var (
	_ ledger.OutcomeStore  = (*recordingWindowStore)(nil)
	_ ledger.BacktestStore = (*recordingWindowStore)(nil)
)

func windowRunConfig(dir string) config.Config {
	return config.Config{
		ReplayMode:        "daily",
		PrimaryMarket:     "TW",
		ReplayDataPath:    "../../samples/replay/twse_stock_day_all_sample.csv",
		LedgerDir:         dir,
		AgentRegistryPath: "../../configs/agents.json",
		ReplaySessionDate: "2026-03-26",
	}
}

func TestRunWindow_WritesThroughTheInjectedStore(t *testing.T) {
	dir := t.TempDir()
	store := newRecordingWindowStore(dir)

	start, _ := time.Parse("2006-01-02", "2026-03-26")
	end, _ := time.Parse("2006-01-02", "2026-03-27")
	summary, err := NewRunner(windowRunConfig(dir), store).Run(start, end)
	if err != nil {
		t.Fatalf("run window: %v", err)
	}

	if len(store.summaries) != 1 {
		t.Fatalf("RecordWindowSummary calls = %d, want 1 (the window must be written through the injected store)", len(store.summaries))
	}
	if got := store.summaries[0]; got.WindowID != summary.WindowID || got.SessionCount != summary.SessionCount {
		t.Errorf("recorded summary = %+v, want the returned summary %+v", got, summary)
	}
	for _, windowID := range store.briefs {
		if windowID != summary.WindowID {
			t.Errorf("recorded mutation brief for %q, want %q", windowID, summary.WindowID)
		}
	}
	if len(store.briefs) > len(store.summaries) {
		t.Errorf("mutation briefs (%d) outnumber window summaries (%d)", len(store.briefs), len(store.summaries))
	}

	// The JSONL window directory must stay untouched: if it does not, the write
	// path is still the file default and a postgres deployment would diverge
	// from its readers.
	if _, statErr := os.Stat(filepath.Join(dir, "windows", summary.WindowID+".json")); !os.IsNotExist(statErr) {
		t.Fatalf("the window was also written to %s (stat err %v); the store must be the only write path",
			filepath.Join(dir, "windows", summary.WindowID+".json"), statErr)
	}
}

// TestRunWindow_ParityJSONLAndStorePath runs the same window twice with the
// same fixture — once against the JSONL file store, once against the recording
// store — and requires the same window summary (GeneratedAt is wall clock).
func TestRunWindow_ParityJSONLAndStorePath(t *testing.T) {
	fileDir := t.TempDir()
	start, _ := time.Parse("2006-01-02", "2026-03-26")
	end, _ := time.Parse("2006-01-02", "2026-03-27")

	fileSummary, err := NewRunner(windowRunConfig(fileDir), ledger.NewStore(fileDir).(*ledger.Store)).Run(start, end)
	if err != nil {
		t.Fatalf("run window (jsonl): %v", err)
	}

	storeDir := t.TempDir()
	store := newRecordingWindowStore(storeDir)
	storeSummary, err := NewRunner(windowRunConfig(storeDir), store).Run(start, end)
	if err != nil {
		t.Fatalf("run window (injected store): %v", err)
	}
	if len(store.summaries) != 1 {
		t.Fatalf("RecordWindowSummary calls = %d, want 1", len(store.summaries))
	}

	fromStore := store.summaries[0]
	if fromStore.WindowID != fileSummary.WindowID ||
		!fromStore.StartDate.Equal(fileSummary.StartDate) ||
		!fromStore.EndDate.Equal(fileSummary.EndDate) ||
		fromStore.SessionCount != fileSummary.SessionCount ||
		fromStore.OutcomeCount != fileSummary.OutcomeCount ||
		fromStore.WorstAgentID != fileSummary.WorstAgentID ||
		fromStore.WorstAgentSkill != fileSummary.WorstAgentSkill ||
		fromStore.WorstAgentLayer != fileSummary.WorstAgentLayer ||
		fromStore.WorstAgentWindowCount != fileSummary.WorstAgentWindowCount ||
		fromStore.WorstAgentSharpeLike != fileSummary.WorstAgentSharpeLike {
		t.Errorf("store-path summary %+v != jsonl-path summary %+v", fromStore, fileSummary)
	}
	if storeSummary.WindowID != fileSummary.WindowID {
		t.Errorf("returned window ids differ: %q vs %q", storeSummary.WindowID, fileSummary.WindowID)
	}
}
