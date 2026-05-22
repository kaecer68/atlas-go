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
	os.MkdirAll(sessionsDir, 0755)

	sessionDir := filepath.Join(sessionsDir, "session-20260501-daily")
	os.MkdirAll(sessionDir, 0755)

	summary := map[string]any{
		"session_id":      "session-20260501-daily",
		"portfolio_value": 2_000_000.0,
		"ending_cash":     500_000.0,
		"recorded_at":     "2026-05-01T14:30:00+08:00",
	}
	summaryData, _ := json.Marshal(summary)
	os.WriteFile(filepath.Join(sessionDir, "summary.json"), summaryData, 0644)

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
	os.WriteFile(filepath.Join(sessionDir, "recommendation_outcomes.jsonl"), []byte(outcomesLines), 0644)

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
	os.MkdirAll(sessionsDir, 0755)

	for i := 1; i <= 5; i++ {
		day := filepath.Join(sessionsDir, "session-2026050"+string(rune('0'+i))+"-daily")
		os.MkdirAll(day, 0755)
		summary := map[string]any{
			"session_id":      "session-2026050" + string(rune('0'+i)) + "-daily",
			"portfolio_value": 1_000_000.0,
			"ending_cash":     200_000.0,
			"recorded_at":     "2026-05-0" + string(rune('0'+i)) + "T14:30:00+08:00",
		}
		data, _ := json.Marshal(summary)
		os.WriteFile(filepath.Join(day, "summary.json"), data, 0644)
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
