package ledger

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestSessionWriter_WriteSession_AllArtifacts(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)
	writer := NewSessionWriter(store)

	session := domain.ReplaySession{ID: "session-20260422-daily"}
	req := SessionWriteRequest{
		Session: session,
		Outcomes: []domain.RecommendationOutcome{
			{AgentID: "a1", Symbol: "2330.TW", Side: domain.SideBuy, Window: "2026-04-22", Conviction: 80},
			{AgentID: "a2", Symbol: "2317.TW", Side: domain.SideSell, Window: "2026-04-22", Conviction: 60},
		},
		Rejects: []domain.ScreeningReject{
			{Symbol: "2498.TW", AgentID: "a3", Criterion: "volume"},
		},
		Summary: &domain.SessionSummary{
			SessionID:    "session-20260422-daily",
			Regime:       domain.RegimeRiskOn,
			OutcomeCount: 2,
			RecordedAt:   time.Now(),
		},
	}

	if err := writer.WriteSession(context.Background(), req); err != nil {
		t.Fatalf("WriteSession: %v", err)
	}

	sessionDir := filepath.Join(dir, "sessions", "session-20260422-daily")

	outcomes, err := store.LoadSessionOutcomes("session-20260422-daily")
	if err != nil {
		t.Fatalf("LoadSessionOutcomes: %v", err)
	}
	if len(outcomes) != 2 {
		t.Fatalf("expected 2 outcomes, got %d", len(outcomes))
	}
	if outcomes[0].AgentID != "a1" {
		t.Errorf("outcome[0].AgentID = %q, want a1", outcomes[0].AgentID)
	}

	outcomePath := filepath.Join(sessionDir, "recommendation_outcomes.jsonl")
	if _, err := os.Stat(outcomePath); err != nil {
		t.Errorf("expected outcomes file: %v", err)
	}

	rejectPath := filepath.Join(sessionDir, "screened_symbols.jsonl")
	if _, err := os.Stat(rejectPath); err != nil {
		t.Errorf("expected screened symbols file: %v", err)
	}

	summaryPath := filepath.Join(sessionDir, "summary.json")
	if _, err := os.Stat(summaryPath); err != nil {
		t.Errorf("expected summary file: %v", err)
	}
}

func TestSessionWriter_WriteSession_OutcomesOnly(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)
	writer := NewSessionWriter(store)

	req := SessionWriteRequest{
		Session: domain.ReplaySession{ID: "session-outcomes-only"},
		Outcomes: []domain.RecommendationOutcome{
			{AgentID: "a1", Symbol: "2330.TW", Side: domain.SideBuy, Window: "2026-04-22", Conviction: 80},
		},
	}

	if err := writer.WriteSession(context.Background(), req); err != nil {
		t.Fatalf("WriteSession: %v", err)
	}

	outcomes, err := store.LoadSessionOutcomes("session-outcomes-only")
	if err != nil {
		t.Fatalf("LoadSessionOutcomes: %v", err)
	}
	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}

	sessionDir := filepath.Join(dir, "sessions", "session-outcomes-only")
	if _, err := os.Stat(filepath.Join(sessionDir, "summary.json")); !os.IsNotExist(err) {
		t.Error("summary.json should not exist when no summary provided")
	}
	if _, err := os.Stat(filepath.Join(sessionDir, "screened_symbols.jsonl")); !os.IsNotExist(err) {
		t.Error("screened_symbols.jsonl should not exist when no rejects provided")
	}
}

func TestSessionWriter_WriteSession_ContextCancelled(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)
	writer := NewSessionWriter(store)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := SessionWriteRequest{
		Session: domain.ReplaySession{ID: "session-cancelled"},
		Outcomes: []domain.RecommendationOutcome{
			{AgentID: "a1", Symbol: "2330.TW", Side: domain.SideBuy, Window: "2026-04-22", Conviction: 80},
		},
	}

	err := writer.WriteSession(ctx, req)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	we, ok := err.(*WriteError)
	if !ok {
		t.Fatalf("expected *WriteError, got %T: %v", err, err)
	}
	if we.Op != "context" {
		t.Errorf("WriteError.Op = %q, want context", we.Op)
	}
}

func TestSessionWriter_WriteSession_WriteError(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)
	writer := NewSessionWriter(store)

	req := SessionWriteRequest{
		Session: domain.ReplaySession{ID: "session-error"},
		Outcomes: []domain.RecommendationOutcome{
			{AgentID: "a1", Symbol: "2330.TW", Side: domain.SideBuy, Window: "2026-04-22", Conviction: 80},
		},
	}

	if err := writer.WriteSession(context.Background(), req); err != nil {
		t.Fatalf("first write: %v", err)
	}

	outcomes, err := store.LoadSessionOutcomes("session-error")
	if err != nil {
		t.Fatalf("LoadSessionOutcomes: %v", err)
	}
	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
}

func TestWriteOutcomesToFile_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "outcomes.jsonl")

	outcomes := []domain.RecommendationOutcome{
		{AgentID: "a1", Symbol: "2330.TW", Side: domain.SideBuy, Window: "2026-04-22", Conviction: 80, PassedGuards: true},
	}

	if err := writeOutcomesToFile(path, outcomes); err != nil {
		t.Fatalf("writeOutcomesToFile: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	var decoded domain.RecommendationOutcome
	if err := json.Unmarshal([]byte(lines[0]), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.AgentID != "a1" {
		t.Errorf("AgentID = %q", decoded.AgentID)
	}
	if !decoded.PassedGuards {
		t.Error("PassedGuards should be true")
	}
}

func TestWriteSummaryToFile_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "summary.json")

	summary := &domain.SessionSummary{
		SessionID:    "session-test",
		Regime:       domain.RegimeRiskOn,
		OutcomeCount: 5,
		RecordedAt:   time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC),
	}

	if err := writeSummaryToFile(path, summary); err != nil {
		t.Fatalf("writeSummaryToFile: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var decoded domain.SessionSummary
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.SessionID != "session-test" {
		t.Errorf("SessionID = %q", decoded.SessionID)
	}
	if decoded.OutcomeCount != 5 {
		t.Errorf("OutcomeCount = %d, want 5", decoded.OutcomeCount)
	}
}

func TestRecordSessionSummary_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)

	session := domain.ReplaySession{ID: "session-summary-test"}
	summary := domain.SessionSummary{
		SessionID:      "session-summary-test",
		Regime:         domain.RegimeRiskOn,
		EndingCash:     100_000,
		PortfolioValue: 1_000_000,
		OutcomeCount:   42,
		RecordedAt:     time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC),
	}

	if err := store.RecordSessionSummary(session, summary); err != nil {
		t.Fatalf("RecordSessionSummary: %v", err)
	}

	summaries, err := store.LoadSessionSummaries()
	if err != nil {
		t.Fatalf("LoadSessionSummaries: %v", err)
	}

	found := false
	for _, s := range summaries {
		if s.SessionID == "session-summary-test" {
			found = true
			if s.OutcomeCount != 42 {
				t.Errorf("OutcomeCount = %d, want 42", s.OutcomeCount)
			}
			break
		}
	}
	if !found {
		t.Error("session summary not found in loaded summaries")
	}
}

func TestLoadSessionOutcomes_MissingDir(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)

	outcomes, err := store.LoadSessionOutcomes("nonexistent-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcomes != nil {
		t.Errorf("expected nil outcomes for missing session, got %d", len(outcomes))
	}
}

func TestWriteError_Error(t *testing.T) {
	we := &WriteError{Op: "write_outcomes", Path: "/tmp/test", Err: os.ErrNotExist}
	msg := we.Error()
	if !strings.Contains(msg, "write_outcomes") {
		t.Errorf("error message missing op: %s", msg)
	}
	if !strings.Contains(msg, "/tmp/test") {
		t.Errorf("error message missing path: %s", msg)
	}
}

func TestWriteError_Unwrap(t *testing.T) {
	we := &WriteError{Op: "mkdir", Err: os.ErrPermission}
	if we.Unwrap() != os.ErrPermission {
		t.Error("Unwrap should return the wrapped error")
	}
}

func TestWriteRejectsToFile_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rejects.jsonl")

	rejects := []domain.ScreeningReject{
		{Symbol: "2498.TW", AgentID: "a1", Criterion: "volume", CriterionLabel: "成交量", Threshold: "1000000", ActualValue: "500000"},
	}

	if err := writeRejectsToFile(path, rejects); err != nil {
		t.Fatalf("writeRejectsToFile: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	var decoded domain.ScreeningReject
	if err := json.Unmarshal([]byte(lines[0]), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Criterion != "volume" {
		t.Errorf("Criterion = %q", decoded.Criterion)
	}
}

func TestSessionWriter_WriteSession_SummaryOnly(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)
	writer := NewSessionWriter(store)

	req := SessionWriteRequest{
		Session: domain.ReplaySession{ID: "session-summary-only"},
		Summary: &domain.SessionSummary{
			SessionID:  "session-summary-only",
			Regime:     domain.RegimeRiskOff,
			RecordedAt: time.Now(),
		},
	}

	if err := writer.WriteSession(context.Background(), req); err != nil {
		t.Fatalf("WriteSession summary-only: %v", err)
	}

	sessionDir := filepath.Join(dir, "sessions", "session-summary-only")
	if _, err := os.Stat(filepath.Join(sessionDir, "summary.json")); os.IsNotExist(err) {
		t.Error("summary.json should exist")
	}
	if _, err := os.Stat(filepath.Join(sessionDir, "recommendation_outcomes.jsonl")); !os.IsNotExist(err) {
		t.Error("outcomes file should not exist")
	}
}

func TestSessionWriter_WriteSession_RejectsOnly(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)
	writer := NewSessionWriter(store)

	req := SessionWriteRequest{
		Session: domain.ReplaySession{ID: "session-rejects-only"},
		Rejects: []domain.ScreeningReject{
			{Symbol: "2498.TW", AgentID: "a1", Criterion: "volume"},
		},
	}

	if err := writer.WriteSession(context.Background(), req); err != nil {
		t.Fatalf("WriteSession rejects-only: %v", err)
	}

	sessionDir := filepath.Join(dir, "sessions", "session-rejects-only")
	if _, err := os.Stat(filepath.Join(sessionDir, "screened_symbols.jsonl")); os.IsNotExist(err) {
		t.Error("screened_symbols.jsonl should exist")
	}
}

func TestSessionWriter_WriteSession_RereadAfterWrite(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)
	writer := NewSessionWriter(store)
	sessionID := "session-twice"

	req := SessionWriteRequest{
		Session: domain.ReplaySession{ID: sessionID},
		Outcomes: []domain.RecommendationOutcome{
			{AgentID: "a1", Symbol: "2330.TW", Side: domain.SideBuy, Window: "2026-04-22", Conviction: 80},
		},
		Summary: &domain.SessionSummary{
			SessionID: sessionID, Regime: domain.RegimeNeutral, RecordedAt: time.Now(),
		},
	}
	if err := writer.WriteSession(context.Background(), req); err != nil {
		t.Fatalf("WriteSession: %v", err)
	}

	outcomes, err := store.LoadSessionOutcomes(sessionID)
	if err != nil {
		t.Fatalf("LoadSessionOutcomes: %v", err)
	}
	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
	if outcomes[0].AgentID != "a1" {
		t.Errorf("AgentID = %q, want a1", outcomes[0].AgentID)
	}

	summaries, err := store.LoadSessionSummaries()
	if err != nil {
		t.Fatalf("LoadSessionSummaries: %v", err)
	}
	found := false
	for _, s := range summaries {
		if s.SessionID == sessionID {
			found = true
			break
		}
	}
	if !found {
		t.Error("session summary not found")
	}
}
