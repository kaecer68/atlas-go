package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecommendations_FilterBySkill(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "data", "state", "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}
	sessionDir := filepath.Join(sessionsDir, "session-20260101-daily")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session: %v", err)
	}

	jsonlContent := `{"symbol":"A","forward_return":0.05,"factor_scores":{"momentum":0.5},"agent_id":"exec1","skill":"momentum"}
{"symbol":"B","forward_return":0.03,"factor_scores":{"value":0.3},"agent_id":"exec2","skill":"value"}
{"symbol":"C","forward_return":-0.02,"factor_scores":{"momentum":-0.1},"agent_id":"exec1","skill":"momentum"}
`
	if err := os.WriteFile(filepath.Join(sessionDir, "recommendation_outcomes.jsonl"), []byte(jsonlContent), 0o644); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}

	provider := NewConvictionCalibrationProvider(tmpDir)

	// Filter by specific skill
	recs, err := provider.Recommendations("momentum")
	if err != nil {
		t.Fatalf("recommendations(momentum): %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 momentum recs, got %d", len(recs))
	}
	if recs[0].Symbol != "A" || recs[1].Symbol != "C" {
		t.Errorf("unexpected filtered symbols: %s, %s", recs[0].Symbol, recs[1].Symbol)
	}

	// "all" returns everything
	all, err := provider.Recommendations("all")
	if err != nil {
		t.Fatalf("recommendations(all): %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 total recs for 'all', got %d", len(all))
	}

	// Empty string returns everything
	empty, err := provider.Recommendations("")
	if err != nil {
		t.Fatalf("recommendations(''): %v", err)
	}
	if len(empty) != 3 {
		t.Fatalf("expected 3 recs for empty filter, got %d", len(empty))
	}

	// No-match skill returns empty
	none, err := provider.Recommendations("nonexistent")
	if err != nil {
		t.Fatalf("recommendations(nonexistent): %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("expected 0 recs for nonexistent skill, got %d", len(none))
	}
}

func TestRecommendations_EmptySessionDir(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "data", "state", "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}
	provider := NewConvictionCalibrationProvider(tmpDir)

	recs, err := provider.Recommendations("momentum")
	if err != nil {
		t.Fatalf("recommendations on empty session dir: %v", err)
	}
	if len(recs) != 0 {
		t.Errorf("expected 0 recs from empty sessions dir, got %d", len(recs))
	}
}
