package monitoring

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSessionCalibrationProvider_LoadsSessions(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	os.MkdirAll(sessionsDir, 0o755)

	sessionDir := filepath.Join(sessionsDir, "session-20260501-daily")
	os.MkdirAll(sessionDir, 0o755)

	summary := map[string]any{
		"session_id":      "session-20260501-daily",
		"portfolio_value": 2_000_000.0,
		"ending_cash":     500_000.0,
		"recorded_at":     "2026-05-01T14:30:00+08:00",
	}
	summaryData, _ := json.Marshal(summary)
	os.WriteFile(filepath.Join(sessionDir, "summary.json"), summaryData, 0o644)

	var outcomesLines string
	for _, sym := range []string{"2330", "2303", "2317"} {
		o := map[string]any{
			"symbol":         sym,
			"conviction":     70,
			"forward_return": 0.03,
			"hit":            true,
			"side":           "buy",
			"price":          100.0,
		}
		data, _ := json.Marshal(o)
		outcomesLines += string(data) + "\n"
	}
	os.WriteFile(filepath.Join(sessionDir, "recommendation_outcomes.jsonl"), []byte(outcomesLines), 0o644)

	provider := NewSessionCalibrationProvider(tmpDir)
	sessions, err := provider.RecentSessions(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].SessionID != "session-20260501-daily" {
		t.Errorf("expected session-20260501-daily, got %s", sessions[0].SessionID)
	}
	if sessions[0].PortfolioValue != 2_000_000 {
		t.Errorf("expected 2M, got %.0f", sessions[0].PortfolioValue)
	}
	if len(sessions[0].Orders) != 3 {
		t.Errorf("expected 3 orders, got %d", len(sessions[0].Orders))
	}
}

func TestSessionCalibrationProvider_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	provider := NewSessionCalibrationProvider(tmpDir)
	sessions, err := provider.RecentSessions(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestSessionCalibrationProvider_Limit(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	os.MkdirAll(sessionsDir, 0o755)

	for i := 1; i <= 5; i++ {
		day := filepath.Join(sessionsDir, "session-2026050"+string(rune('0'+i))+"-daily")
		os.MkdirAll(day, 0o755)
		summary := map[string]any{
			"session_id":      "session-2026050" + string(rune('0'+i)) + "-daily",
			"portfolio_value": 1_000_000.0,
			"ending_cash":     200_000.0,
			"recorded_at":     "2026-05-0" + string(rune('0'+i)) + "T14:30:00+08:00",
		}
		data, _ := json.Marshal(summary)
		os.WriteFile(filepath.Join(day, "summary.json"), data, 0o644)
	}

	provider := NewSessionCalibrationProvider(tmpDir)
	sessions, err := provider.RecentSessions(context.Background(), 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) > 3 {
		t.Errorf("expected at most 3 sessions with limit=3, got %d", len(sessions))
	}
}

func TestSplitJSONLLines(t *testing.T) {
	lines := splitJSONLLines([]byte("line1\nline2\nline3\n"))
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}
}

func TestSessionCalibrationProvider_LoadSession_MissingFiles(t *testing.T) {
	tmpDir := t.TempDir()
	sessionDir := filepath.Join(tmpDir, "session-missing")
	os.MkdirAll(sessionDir, 0o755)

	provider := NewSessionCalibrationProvider(tmpDir)
	sessions, err := provider.RecentSessions(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions when summary missing, got %d", len(sessions))
	}
}

func TestSessionCalibrationProvider_LoadSession_InvalidSummary(t *testing.T) {
	tmpDir := t.TempDir()
	sessionDir := filepath.Join(tmpDir, "sessions", "session-bad")
	os.MkdirAll(sessionDir, 0o755)
	os.WriteFile(filepath.Join(sessionDir, "summary.json"), []byte("not json"), 0o644)

	provider := NewSessionCalibrationProvider(tmpDir)
	sessions, err := provider.RecentSessions(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session with invalid summary, got %d", len(sessions))
	}
	if sessions[0].SessionID != "" {
		t.Errorf("expected empty SessionID, got %s", sessions[0].SessionID)
	}
}

func TestSessionCalibrationProvider_LoadSession_NoOutcomes(t *testing.T) {
	tmpDir := t.TempDir()
	sessionDir := filepath.Join(tmpDir, "sessions", "session-no-outcomes")
	os.MkdirAll(sessionDir, 0o755)
	summary := map[string]any{
		"session_id":      "session-no-outcomes",
		"portfolio_value": 1_000_000.0,
		"ending_cash":     200_000.0,
		"recorded_at":     "2026-05-01T14:30:00+08:00",
	}
	data, _ := json.Marshal(summary)
	os.WriteFile(filepath.Join(sessionDir, "summary.json"), data, 0o644)

	provider := NewSessionCalibrationProvider(tmpDir)
	sessions, err := provider.RecentSessions(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if len(sessions[0].Orders) != 0 {
		t.Errorf("expected 0 orders, got %d", len(sessions[0].Orders))
	}
}

func TestSessionCalibrationProvider_LoadSession_EmptySideFallback(t *testing.T) {
	tmpDir := t.TempDir()
	sessionDir := filepath.Join(tmpDir, "sessions", "session-empty-side")
	os.MkdirAll(sessionDir, 0o755)
	summary := map[string]any{
		"session_id":      "session-empty-side",
		"portfolio_value": 1_000_000.0,
		"ending_cash":     200_000.0,
		"recorded_at":     "2026-05-01T14:30:00+08:00",
	}
	data, _ := json.Marshal(summary)
	os.WriteFile(filepath.Join(sessionDir, "summary.json"), data, 0o644)

	outcome := map[string]any{
		"symbol":         "2330",
		"conviction":     70,
		"forward_return": 0.03,
		"hit":            true,
		"side":           "",
		"price":          100.0,
	}
	line, _ := json.Marshal(outcome)
	os.WriteFile(filepath.Join(sessionDir, "recommendation_outcomes.jsonl"), append(line, '\n'), 0o644)

	provider := NewSessionCalibrationProvider(tmpDir)
	sessions, err := provider.RecentSessions(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 || len(sessions[0].Orders) != 1 {
		t.Fatalf("expected 1 order, got %+v", sessions)
	}
	if sessions[0].Orders[0].Side != "buy" {
		t.Errorf("expected side fallback buy, got %s", sessions[0].Orders[0].Side)
	}
}

func TestAvg(t *testing.T) {
	if got := avg([]float64{1, 2, 3, 4}); got != 2.5 {
		t.Errorf("avg = %v, want 2.5", got)
	}
	if got := avg(nil); got != 0 {
		t.Errorf("avg nil = %v, want 0", got)
	}
}

func TestStdev(t *testing.T) {
	if got := stdev([]float64{1, 2, 3, 4}, 2.5); got <= 0 {
		t.Errorf("stdev = %v, want > 0", got)
	}
	if got := stdev([]float64{1}, 1); got != 0 {
		t.Errorf("stdev single = %v, want 0", got)
	}
}

func TestMathSqrt(t *testing.T) {
	if got := mathSqrt(4); got <= 1.9 || got >= 2.1 {
		t.Errorf("sqrt(4) = %v, want ~2", got)
	}
	if got := mathSqrt(0); got != 0 {
		t.Errorf("sqrt(0) = %v, want 0", got)
	}
	if got := mathSqrt(-1); got != 0 {
		t.Errorf("sqrt(-1) = %v, want 0", got)
	}
}
