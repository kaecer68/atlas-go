package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

// Q2 batch 2, T2 — the window family (window summary + mutation brief) reader
// seam.
//
// Producers now persist the family through the backend-aware factory, so on
// ATLAS_STORE_BACKEND=postgres the windows live in PostgreSQL. These tests pin
// the three properties the seam must have:
//
//  1. injected ⇒ the store answers, and the JSONL files (which now hold only
//     pre-migration history) cannot decide the result;
//  2. nil ⇒ the legacy file behavior, byte for byte;
//  3. parity ⇒ the same fixture through both paths yields the same payload.

// fakeWindowStore is an in-memory ledger.WindowSummaryReader; err forces a
// backend failure so the "do not silently fall back to the old files" branch is
// reachable.
type fakeWindowStore struct {
	summaries []domain.BacktestWindowSummary
	err       error
	calls     int
}

func (f *fakeWindowStore) LoadWindowSummaries() ([]domain.BacktestWindowSummary, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.summaries, nil
}

func (f *fakeWindowStore) LoadWindowSummary(windowID string) (domain.BacktestWindowSummary, bool, error) {
	f.calls++
	if f.err != nil {
		return domain.BacktestWindowSummary{}, false, f.err
	}
	for _, summary := range f.summaries {
		if summary.WindowID == windowID {
			return summary, true, nil
		}
	}
	return domain.BacktestWindowSummary{}, false, nil
}

var _ ledger.WindowSummaryReader = (*fakeWindowStore)(nil)

func windowFixture(id string, generatedAt time.Time, sessions int) domain.BacktestWindowSummary {
	return domain.BacktestWindowSummary{
		WindowID:              id,
		StartDate:             generatedAt.AddDate(0, 0, -20),
		EndDate:               generatedAt,
		SessionCount:          sessions,
		OutcomeCount:          sessions * 3,
		GeneratedAt:           generatedAt,
		WorstAgentID:          "agent-" + id,
		WorstAgentSkill:       "skill-" + id,
		WorstAgentLayer:       "layer",
		WorstAgentWindowCount: 7,
		WorstAgentSharpeLike:  0.42,
	}
}

// seedWindowFiles writes the fixture to the JSONL window directory through the
// store itself, so the file layout is the producer's, not the test's.
func seedWindowFiles(t *testing.T, ledgerDir string, summaries ...domain.BacktestWindowSummary) {
	t.Helper()
	store := ledger.NewStore(ledgerDir).(*ledger.Store)
	for _, summary := range summaries {
		if err := store.RecordWindowSummary(summary); err != nil {
			t.Fatalf("seed window %s: %v", summary.WindowID, err)
		}
	}
}

var (
	windowOlder = windowFixture("window-20260101-20260121", time.Date(2026, 1, 21, 12, 0, 0, 0, time.UTC), 5)
	windowNewer = windowFixture("window-20260122-20260211", time.Date(2026, 2, 11, 12, 0, 0, 0, time.UTC), 9)
)

func TestReportService_WindowStoreSeam_InjectedStoreAnswers(t *testing.T) {
	dir := t.TempDir()
	// Pre-migration files on disk are NEWER and more numerous than the store
	// rows: only the injected store may decide the answer.
	seedWindowFiles(t, dir, windowOlder, windowNewer)
	store := &fakeWindowStore{summaries: []domain.BacktestWindowSummary{windowOlder}}

	svc := NewReportService(dir, dir, nil).WithWindowStore(store)
	summaries, err := svc.loadAllWindowSummaries()
	if err != nil {
		t.Fatalf("loadAllWindowSummaries: %v", err)
	}
	if len(summaries) != 1 || summaries[0].WindowID != windowOlder.WindowID {
		t.Fatalf("summaries = %+v, want exactly the store's window %s", summaries, windowOlder.WindowID)
	}
	if store.calls == 0 {
		t.Fatal("the injected store was never read")
	}

	latest, err := svc.loadLatestWindowSummary()
	if err != nil {
		t.Fatalf("loadLatestWindowSummary: %v", err)
	}
	if latest.WindowID != windowOlder.WindowID {
		t.Errorf("latest = %s, want the store's %s (the newer JSONL file must not win)", latest.WindowID, windowOlder.WindowID)
	}
}

func TestReportService_WindowStoreSeam_NilKeepsFileBehavior(t *testing.T) {
	dir := t.TempDir()
	seedWindowFiles(t, dir, windowOlder, windowNewer)

	svc := NewReportService(dir, dir, nil)
	summaries, err := svc.loadAllWindowSummaries()
	if err != nil {
		t.Fatalf("loadAllWindowSummaries: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("summaries = %d, want 2 from the files", len(summaries))
	}
	latest, err := svc.loadLatestWindowSummary()
	if err != nil {
		t.Fatalf("loadLatestWindowSummary: %v", err)
	}
	if latest.WindowID != windowNewer.WindowID {
		t.Errorf("latest = %s, want the newest file %s", latest.WindowID, windowNewer.WindowID)
	}
}

// TestReportService_WindowStoreSeam_Parity pins that the store path and the
// file path render the same report for the same fixture (the seam changes the
// backend, not the payload).
func TestReportService_WindowStoreSeam_Parity(t *testing.T) {
	dir := t.TempDir()
	seedWindowFiles(t, dir, windowOlder, windowNewer)

	// renderWindowReport reads its scorecards/session summaries from the
	// outcome store, so both sides need a (file-backed) ledger; only the window
	// read differs.
	fromFiles := NewReportService(dir, dir, ledger.NewStore(dir).(*ledger.Store))
	fromStore := NewReportService(dir, dir, ledger.NewStore(dir).(*ledger.Store)).WithWindowStore(&fakeWindowStore{
		summaries: []domain.BacktestWindowSummary{windowOlder, windowNewer},
	})

	listFiles, err := json.Marshal(mustReportList(t, fromFiles))
	if err != nil {
		t.Fatalf("marshal file report list: %v", err)
	}
	listStore, err := json.Marshal(mustReportList(t, fromStore))
	if err != nil {
		t.Fatalf("marshal store report list: %v", err)
	}
	if string(listFiles) != string(listStore) {
		t.Errorf("LoadReportList differs between the file and store paths:\n files: %s\n store: %s", listFiles, listStore)
	}

	contentFiles, nameFiles, err := fromFiles.LoadLatestReport()
	if err != nil {
		t.Fatalf("LoadLatestReport (files): %v", err)
	}
	contentStore, nameStore, err := fromStore.LoadLatestReport()
	if err != nil {
		t.Fatalf("LoadLatestReport (store): %v", err)
	}
	if nameFiles != nameStore {
		t.Errorf("report filename differs: %q vs %q", nameFiles, nameStore)
	}
	if string(contentFiles) != string(contentStore) {
		t.Errorf("rendered report differs between the file and store paths")
	}
}

func mustReportList(t *testing.T, svc *ReportService) []ReportEntry {
	t.Helper()
	reports, err := svc.LoadReportList()
	if err != nil {
		t.Fatalf("LoadReportList: %v", err)
	}
	return reports
}

// TestReportService_WindowStoreSeam_StoreErrorIsNotMasked pins that a broken
// backend surfaces instead of silently serving the migrated-away files.
func TestReportService_WindowStoreSeam_StoreErrorIsNotMasked(t *testing.T) {
	dir := t.TempDir()
	seedWindowFiles(t, dir, windowNewer)

	svc := NewReportService(dir, dir, nil).WithWindowStore(&fakeWindowStore{err: errors.New("postgres unavailable")})
	if _, err := svc.loadAllWindowSummaries(); err == nil {
		t.Fatal("loadAllWindowSummaries returned nil error while the injected store failed")
	}
	if _, err := svc.LoadReportList(); err == nil {
		t.Fatal("LoadReportList returned nil error while the injected store failed")
	}
	if _, _, err := svc.LoadLatestReport(); err == nil {
		t.Fatal("LoadLatestReport returned nil error while the injected store failed")
	}
}

func TestSystemService_WindowStoreSeam_InjectedStoreAnswers(t *testing.T) {
	dir := t.TempDir()
	// The file on disk is newer; the store must still decide.
	seedWindowFiles(t, dir, windowNewer)
	store := &fakeWindowStore{summaries: []domain.BacktestWindowSummary{windowOlder}}

	svc := NewSystemService(dir, dir, testBaselinePolicyPath(t), nil, nil, nil).WithWindowStore(store)
	health, err := svc.LoadSystemHealth()
	if err != nil {
		t.Fatalf("LoadSystemHealth: %v", err)
	}
	if health.LastWindowID != windowOlder.WindowID {
		t.Errorf("LastWindowID = %q, want the store's %q", health.LastWindowID, windowOlder.WindowID)
	}
	if !health.LastWindowGeneratedAt.Equal(windowOlder.GeneratedAt) {
		t.Errorf("LastWindowGeneratedAt = %v, want %v", health.LastWindowGeneratedAt, windowOlder.GeneratedAt)
	}
	for _, warning := range health.Warnings {
		if warning == "找不到回測窗口摘要" {
			t.Errorf("warnings = %v; the store answered, so the missing-window warning is a false positive", health.Warnings)
		}
	}
}

func TestSystemService_WindowStoreSeam_EmptyStoreWarns(t *testing.T) {
	dir := t.TempDir()
	seedWindowFiles(t, dir, windowNewer)
	store := &fakeWindowStore{}

	svc := NewSystemService(dir, dir, testBaselinePolicyPath(t), nil, nil, nil).WithWindowStore(store)
	health, err := svc.LoadSystemHealth()
	if err != nil {
		t.Fatalf("LoadSystemHealth: %v", err)
	}
	if health.LastWindowID != "" {
		t.Errorf("LastWindowID = %q, want empty (no rows in the declared backend)", health.LastWindowID)
	}
	if !contains(health.Warnings, "找不到回測窗口摘要") {
		t.Errorf("warnings = %v, want the missing-window warning once the declared backend is empty", health.Warnings)
	}
}

func TestSystemService_WindowStoreSeam_StoreErrorWarns(t *testing.T) {
	dir := t.TempDir()
	svc := NewSystemService(dir, dir, testBaselinePolicyPath(t), nil, nil, nil).
		WithWindowStore(&fakeWindowStore{err: errors.New("postgres unavailable")})
	health, err := svc.LoadSystemHealth()
	if err != nil {
		t.Fatalf("LoadSystemHealth: %v", err)
	}
	found := false
	for _, warning := range health.Warnings {
		if strings.Contains(warning, "回測窗口摘要讀取失敗") {
			found = true
		}
	}
	if !found {
		t.Errorf("warnings = %v, want the window-read failure warning", health.Warnings)
	}
	if health.LastWindowID != "" {
		t.Errorf("LastWindowID = %q, want empty after a failed store read", health.LastWindowID)
	}
}

// testBaselinePolicyPath points BaselinePath at a real policy so the
// "基線策略未載入" warning does not mask the window assertions.
func testBaselinePolicyPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline_policy.json")
	if err := os.WriteFile(path, []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatalf("write baseline policy: %v", err)
	}
	return path
}
