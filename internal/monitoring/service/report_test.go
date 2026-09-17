package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestReportService_LoadAllWindowSummaries_EmptyDir(t *testing.T) {
	tmp := t.TempDir()
	svc := NewReportService(tmp, tmp, nil)
	summaries, err := svc.loadAllWindowSummaries()
	if err != nil {
		t.Fatalf("expected nil err for empty dir, got: %v", err)
	}
	if summaries != nil {
		t.Errorf("expected nil for empty dir, got %v", summaries)
	}
}

func TestReportService_LoadAllWindowSummaries_WindowsDirNotExist(t *testing.T) {
	tmp := t.TempDir()
	svc := NewReportService(tmp, "/nonexistent/path", nil)
	summaries, err := svc.loadAllWindowSummaries()
	if err != nil {
		t.Fatalf("expected nil err when dir not exist, got: %v", err)
	}
	if summaries != nil {
		t.Errorf("expected nil when dir not exist, got %v", summaries)
	}
}

func TestReportService_LoadAllWindowSummaries_SkipsMutationBrief(t *testing.T) {
	tmp := t.TempDir()
	windowsDir := filepath.Join(tmp, "windows")
	if err := os.MkdirAll(windowsDir, 0o755); err != nil {
		t.Fatalf("failed to create windows dir: %v", err)
	}
	summary := domain.BacktestWindowSummary{
		WindowID:     "test-window",
		GeneratedAt:  time.Now(),
		SessionCount: 5,
	}
	data, _ := json.Marshal(summary)
	if err := os.WriteFile(filepath.Join(windowsDir, "backtest_test-window.json"), data, 0o644); err != nil {
		t.Fatalf("failed to write summary: %v", err)
	}
	if err := os.WriteFile(filepath.Join(windowsDir, "backtest_test-window-mutation-brief.json"), data, 0o644); err != nil {
		t.Fatalf("failed to write mutation brief: %v", err)
	}

	svc := NewReportService(tmp, tmp, nil)
	summaries, err := svc.loadAllWindowSummaries()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(summaries) != 1 {
		t.Errorf("expected 1 summary (mutation-brief should be skipped), got %d", len(summaries))
	}
}

func TestReportService_LoadAllWindowSummaries_SkipsCorrupted(t *testing.T) {
	tmp := t.TempDir()
	windowsDir := filepath.Join(tmp, "windows")
	if err := os.MkdirAll(windowsDir, 0o755); err != nil {
		t.Fatalf("failed to create windows dir: %v", err)
	}
	summary := domain.BacktestWindowSummary{
		WindowID:     "good-window",
		GeneratedAt:  time.Now(),
		SessionCount: 5,
	}
	goodData, _ := json.Marshal(summary)
	if err := os.WriteFile(filepath.Join(windowsDir, "backtest_good-window.json"), goodData, 0o644); err != nil {
		t.Fatalf("failed to write summary: %v", err)
	}
	if err := os.WriteFile(filepath.Join(windowsDir, "backtest_bad-window.json"), []byte("not valid json"), 0o644); err != nil {
		t.Fatalf("failed to write bad json: %v", err)
	}

	svc := NewReportService(tmp, tmp, nil)
	summaries, err := svc.loadAllWindowSummaries()
	if err != nil {
		t.Fatalf("expected no error (corrupted skipped), got: %v", err)
	}
	if len(summaries) != 1 {
		t.Errorf("expected 1 summary (corrupted skipped), got %d", len(summaries))
	}
}

func TestReportService_LoadLatestWindowSummary_NoWindows(t *testing.T) {
	tmp := t.TempDir()
	svc := NewReportService(tmp, tmp, nil)
	_, err := svc.loadLatestWindowSummary()
	if err == nil {
		t.Error("expected error when no windows exist")
	}
}

func TestReportService_LoadLatestWindowSummary_SelectsNewest(t *testing.T) {
	tmp := t.TempDir()
	windowsDir := filepath.Join(tmp, "windows")
	if err := os.MkdirAll(windowsDir, 0o755); err != nil {
		t.Fatalf("failed to create windows dir: %v", err)
	}
	older := domain.BacktestWindowSummary{
		WindowID:     "older",
		GeneratedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		SessionCount: 5,
	}
	newer := domain.BacktestWindowSummary{
		WindowID:     "newer",
		GeneratedAt:  time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		SessionCount: 10,
	}
	olderData, _ := json.Marshal(older)
	newerData, _ := json.Marshal(newer)
	os.WriteFile(filepath.Join(windowsDir, "backtest_older.json"), olderData, 0o644)
	os.WriteFile(filepath.Join(windowsDir, "backtest_newer.json"), newerData, 0o644)

	svc := NewReportService(tmp, tmp, nil)
	latest, err := svc.loadLatestWindowSummary()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if latest.WindowID != "newer" {
		t.Errorf("expected newest window 'newer', got %q", latest.WindowID)
	}
}

func TestReportService_LoadReportList_EmptyDir(t *testing.T) {
	tmp := t.TempDir()
	svc := NewReportService(tmp, tmp, nil)
	reports, err := svc.LoadReportList()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(reports) != 0 {
		t.Errorf("expected 0 reports, got %d", len(reports))
	}
}

func TestReportService_LoadDailySummary_NoDate(t *testing.T) {
	tmp := t.TempDir()
	svc := NewReportService(tmp, tmp, nil)
	report, err := svc.LoadDailySummary("")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if report == nil {
		t.Fatal("expected non-nil report")
	}
}

// ---------------------------------------------------------------------------
// /api/report/latest memory-safety regressions (2026-09-17 production incident:
// the endpoint re-rendered the full outcome table per request, allocating
// ~2.9 GB and OOM-killing the container every ~5.4 minutes).
// ---------------------------------------------------------------------------

// countingScorecardStore records which scorecard read path the report service
// takes. It embeds mockOutcomeStore for the rest of the OutcomeStore surface.
type countingScorecardStore struct {
	*mockOutcomeStore
	fullCalls    int
	slimCalls    int
	slimErr      error
	slimOutcomes []domain.RecommendationOutcome
	scorecards   []domain.Scorecard
}

func (c *countingScorecardStore) LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	c.fullCalls++
	return c.scorecards, nil, nil
}

func (c *countingScorecardStore) LoadScorecardOutcomes() ([]domain.RecommendationOutcome, error) {
	c.slimCalls++
	return c.slimOutcomes, c.slimErr
}

func writeWindowSummary(t *testing.T, ledgerDir, windowID string) {
	t.Helper()
	windowsDir := filepath.Join(ledgerDir, "windows")
	if err := os.MkdirAll(windowsDir, 0o755); err != nil {
		t.Fatalf("mkdir windows: %v", err)
	}
	summary := domain.BacktestWindowSummary{
		WindowID:     windowID,
		StartDate:    time.Now().AddDate(0, 0, -30),
		EndDate:      time.Now(),
		SessionCount: 3,
		OutcomeCount: 12,
		GeneratedAt:  time.Now(),
	}
	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	if err := os.WriteFile(filepath.Join(windowsDir, "backtest_"+windowID+".json"), data, 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}
}

// TestReportService_LoadLatestReport_PrefersSlimScorecardRead is the core
// regression: when the store offers the slim projection, the report must NOT
// take the full-metadata read.
func TestReportService_LoadLatestReport_PrefersSlimScorecardRead(t *testing.T) {
	dir := t.TempDir()
	writeWindowSummary(t, dir, "w1")
	store := &countingScorecardStore{
		mockOutcomeStore: &mockOutcomeStore{},
		slimOutcomes: []domain.RecommendationOutcome{
			{AgentID: "ai-desk-01", Skill: "ai_supply_chain", ForwardReturn: 0.02, Hit: true, Window: "2026-09-16"},
			{AgentID: "ai-desk-01", Skill: "ai_supply_chain", ForwardReturn: -0.01, Hit: false, Window: "2026-09-15"},
		},
	}
	svc := NewReportService(dir, dir, store)

	content, filename, err := svc.LoadLatestReport()
	if err != nil {
		t.Fatalf("LoadLatestReport: %v", err)
	}
	if len(content) == 0 || filename == "" {
		t.Fatalf("expected report content and filename, got len=%d name=%q", len(content), filename)
	}
	if store.slimCalls == 0 {
		t.Error("expected the slim scorecard read to be used")
	}
	if store.fullCalls != 0 {
		t.Errorf("full metadata read must not be used when slim is available (calls=%d)", store.fullCalls)
	}
}

// TestReportService_LoadLatestReport_CachesAndRefreshesAfterTTL verifies the
// TTL cache: repeated requests inside the window reuse one render, and the
// report is recomputed once the TTL lapses.
func TestReportService_LoadLatestReport_CachesAndRefreshesAfterTTL(t *testing.T) {
	dir := t.TempDir()
	writeWindowSummary(t, dir, "w1")
	store := &countingScorecardStore{
		mockOutcomeStore: &mockOutcomeStore{},
		slimOutcomes:     []domain.RecommendationOutcome{{AgentID: "a", ForwardReturn: 0.01, Hit: true, Window: "d1"}},
	}
	now := time.Date(2026, 9, 17, 9, 0, 0, 0, time.UTC)
	svc := NewReportService(dir, dir, store).WithLatestReportTTL(60 * time.Second)
	svc.nowFn = func() time.Time { return now }

	first, name1, err := svc.LoadLatestReport()
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	for i := 0; i < 4; i++ {
		content, name, err := svc.LoadLatestReport()
		if err != nil {
			t.Fatalf("cached call %d: %v", i, err)
		}
		if string(content) != string(first) || name != name1 {
			t.Fatalf("cached call %d returned different content", i)
		}
	}
	if store.slimCalls != 1 {
		t.Fatalf("expected 1 render within TTL, got %d", store.slimCalls)
	}

	// Past the TTL: exactly one more render.
	now = now.Add(61 * time.Second)
	if _, _, err := svc.LoadLatestReport(); err != nil {
		t.Fatalf("post-TTL call: %v", err)
	}
	if store.slimCalls != 2 {
		t.Fatalf("expected a re-render after TTL, got %d renders", store.slimCalls)
	}
}

// TestReportService_LoadLatestReport_SlimFailureFallsBackToFullRead keeps the
// report available if the slim projection errors.
func TestReportService_LoadLatestReport_SlimFailureFallsBackToFullRead(t *testing.T) {
	dir := t.TempDir()
	writeWindowSummary(t, dir, "w1")
	store := &countingScorecardStore{
		mockOutcomeStore: &mockOutcomeStore{},
		slimErr:          errSlimUnavailable,
		scorecards:       []domain.Scorecard{{AgentID: "a"}},
	}
	svc := NewReportService(dir, dir, store)

	if _, _, err := svc.LoadLatestReport(); err != nil {
		t.Fatalf("LoadLatestReport should fall back, got: %v", err)
	}
	if store.slimCalls == 0 {
		t.Error("expected a slim attempt first")
	}
	if store.fullCalls == 0 {
		t.Error("expected fallback to the full scorecard read")
	}
}

// TestReportService_LoadLatestReport_StoreWithoutSlimUsesFullRead covers the
// jsonl/sqlite backends that do not implement the slim projection.
func TestReportService_LoadLatestReport_StoreWithoutSlimUsesFullRead(t *testing.T) {
	dir := t.TempDir()
	writeWindowSummary(t, dir, "w1")
	store := &fullOnlyStore{mockOutcomeStore: &mockOutcomeStore{}}
	svc := NewReportService(dir, dir, store)

	if _, _, err := svc.LoadLatestReport(); err != nil {
		t.Fatalf("LoadLatestReport: %v", err)
	}
	if store.fullCalls == 0 {
		t.Error("expected the full scorecard read when slim is not implemented")
	}
}

// fullOnlyStore deliberately does NOT implement LoadScorecardOutcomes, emulating
// the jsonl/sqlite backends.
type fullOnlyStore struct {
	*mockOutcomeStore
	fullCalls int
}

func (f *fullOnlyStore) LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	f.fullCalls++
	return nil, nil, nil
}

var errSlimUnavailable = errors.New("slim projection unavailable")
