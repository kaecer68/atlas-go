package backfill

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestBackfillSummaries_OrphanSessionGetsMinimalSummary(t *testing.T) {
	baseDir := t.TempDir()
	sessionsDir := filepath.Join(baseDir, "sessions")
	sessionID := "session-20260614-daily"
	sessionDir := filepath.Join(sessionsDir, sessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outcomes := []domain.RecommendationOutcome{
		{AgentID: "agent-1", Symbol: "2330.TW", Side: domain.SideBuy, RecordedAt: time.Date(2026, 6, 14, 4, 0, 0, 0, time.UTC)},
		{AgentID: "agent-2", Symbol: "2454.TW", Side: domain.SideBuy, RecordedAt: time.Date(2026, 6, 14, 4, 0, 0, 0, time.UTC)},
		{AgentID: "agent-3", Symbol: "3008.TW", Side: domain.SideBuy, RecordedAt: time.Date(2026, 6, 14, 4, 0, 0, 0, time.UTC)},
	}
	var buf []byte
	for _, o := range outcomes {
		b, _ := json.Marshal(o)
		buf = append(buf, b...)
		buf = append(buf, '\n')
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "recommendation_outcomes.jsonl"), buf, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := BackfillSummaries(sessionsDir, false)
	if err != nil {
		t.Fatalf("BackfillSummaries: %v", err)
	}
	if result.Backfilled != 1 {
		t.Errorf("Backfilled = %d, want 1", result.Backfilled)
	}
	if result.Scanned != 1 || result.SkippedExists != 0 || result.SkippedEmpty != 0 {
		t.Errorf("counts = %+v, want Scanned=1 SkippedExists=0 SkippedEmpty=0", result)
	}

	data, err := os.ReadFile(filepath.Join(sessionDir, "summary.json"))
	if err != nil {
		t.Fatalf("summary.json not created: %v", err)
	}
	var summary domain.SessionSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	if summary.SessionID != sessionID {
		t.Errorf("SessionID = %q, want %q", summary.SessionID, sessionID)
	}
	if summary.OutcomeCount != 3 {
		t.Errorf("OutcomeCount = %d, want 3", summary.OutcomeCount)
	}
	wantDate := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	if !summary.RecordedAt.Equal(wantDate) {
		t.Errorf("RecordedAt = %v, want %v", summary.RecordedAt, wantDate)
	}
}

func TestBackfillSummaries_ExistingSummaryNotOverwritten(t *testing.T) {
	baseDir := t.TempDir()
	sessionsDir := filepath.Join(baseDir, "sessions")
	sessionID := "session-20260615-daily"
	sessionDir := filepath.Join(sessionsDir, sessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := domain.SessionSummary{SessionID: sessionID, Regime: domain.RegimeRiskOn, OutcomeCount: 99}
	data, _ := json.Marshal(existing)
	if err := os.WriteFile(filepath.Join(sessionDir, "summary.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "recommendation_outcomes.jsonl"), []byte("{}\n{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := BackfillSummaries(sessionsDir, false)
	if err != nil {
		t.Fatalf("BackfillSummaries: %v", err)
	}
	if result.Backfilled != 0 {
		t.Errorf("Backfilled = %d, want 0 (existing summary must not be overwritten)", result.Backfilled)
	}
	if result.SkippedExists != 1 {
		t.Errorf("SkippedExists = %d, want 1", result.SkippedExists)
	}
}

func TestBackfillSummaries_EmptySessionSkipped(t *testing.T) {
	baseDir := t.TempDir()
	sessionsDir := filepath.Join(baseDir, "sessions")
	if err := os.MkdirAll(filepath.Join(sessionsDir, "session-empty-daily"), 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := BackfillSummaries(sessionsDir, false)
	if err != nil {
		t.Fatalf("BackfillSummaries: %v", err)
	}
	if result.Backfilled != 0 {
		t.Errorf("Backfilled = %d, want 0 (no jsonl, no summary)", result.Backfilled)
	}
	if result.SkippedEmpty != 1 {
		t.Errorf("SkippedEmpty = %d, want 1", result.SkippedEmpty)
	}
	if _, err := os.Stat(filepath.Join(sessionsDir, "session-empty-daily", "summary.json")); !os.IsNotExist(err) {
		t.Error("summary.json should not be created for empty session")
	}
}

func TestBackfillSummaries_DryRunDoesNotWrite(t *testing.T) {
	baseDir := t.TempDir()
	sessionsDir := filepath.Join(baseDir, "sessions")
	sessionID := "session-20260616-daily"
	sessionDir := filepath.Join(sessionsDir, sessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "recommendation_outcomes.jsonl"), []byte("{}\n{}\n{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := BackfillSummaries(sessionsDir, true)
	if err != nil {
		t.Fatalf("BackfillSummaries: %v", err)
	}
	if result.Backfilled != 1 {
		t.Errorf("Backfilled = %d, want 1 (dry-run counts as planned)", result.Backfilled)
	}
	if _, err := os.Stat(filepath.Join(sessionDir, "summary.json")); !os.IsNotExist(err) {
		t.Error("dry-run must not create summary.json")
	}
}

func TestBackfillSummaries_MissingDirIsNoop(t *testing.T) {
	result, err := BackfillSummaries("/nonexistent/path/to/sessions", false)
	if err != nil {
		t.Errorf("missing dir should be noop, got err: %v", err)
	}
	if result.Scanned != 0 {
		t.Errorf("Scanned = %d, want 0", result.Scanned)
	}
}

func TestBackfillSummaries_MixedBatch(t *testing.T) {
	baseDir := t.TempDir()
	sessionsDir := filepath.Join(baseDir, "sessions")

	orphan := filepath.Join(sessionsDir, "session-orphan-daily")
	if err := os.MkdirAll(orphan, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(orphan, "recommendation_outcomes.jsonl"), []byte("{}\n{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	healthy := filepath.Join(sessionsDir, "session-healthy-daily")
	if err := os.MkdirAll(healthy, 0o755); err != nil {
		t.Fatal(err)
	}
	healthySummary := domain.SessionSummary{SessionID: "session-healthy-daily", Regime: domain.RegimeRiskOn, OutcomeCount: 7}
	d, _ := json.Marshal(healthySummary)
	if err := os.WriteFile(filepath.Join(healthy, "summary.json"), d, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(healthy, "recommendation_outcomes.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	empty := filepath.Join(sessionsDir, "session-empty-daily")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := BackfillSummaries(sessionsDir, false)
	if err != nil {
		t.Fatalf("BackfillSummaries: %v", err)
	}
	if result.Scanned != 3 {
		t.Errorf("Scanned = %d, want 3", result.Scanned)
	}
	if result.Backfilled != 1 {
		t.Errorf("Backfilled = %d, want 1 (only orphan)", result.Backfilled)
	}
	if result.SkippedExists != 1 {
		t.Errorf("SkippedExists = %d, want 1", result.SkippedExists)
	}
	if result.SkippedEmpty != 1 {
		t.Errorf("SkippedEmpty = %d, want 1", result.SkippedEmpty)
	}
}
