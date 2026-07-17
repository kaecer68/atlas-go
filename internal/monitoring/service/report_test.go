package service

import (
	"encoding/json"
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
