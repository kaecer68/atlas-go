package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// slimSessionStore adds the slim per-session projection to the pipeline test
// mock and counts which read path the service used. Counters are atomic because
// the per-session reads may run concurrently.
type slimSessionStore struct {
	*mockOutcomeStore
	slimCalls atomic.Int64
	fullCalls atomic.Int64
	outcomes  []domain.RecommendationOutcome
}

func (s *slimSessionStore) LoadSessionScorecardOutcomes(sessionID string) ([]domain.RecommendationOutcome, error) {
	s.slimCalls.Add(1)
	out := make([]domain.RecommendationOutcome, len(s.outcomes))
	copy(out, s.outcomes)
	for i := range out {
		out[i].Window = sessionID
	}
	return out, nil
}

func (s *slimSessionStore) LoadSessionOutcomes(sessionID string) ([]domain.RecommendationOutcome, error) {
	s.fullCalls.Add(1)
	return nil, nil
}

// TestLoadSessionsWithTopStrategies_PrefersSlimProjection locks the fix for the
// /api/dashboard/sessions OOM: enriching every session used the full metadata
// read (644 MB table-wide; ~1 GB heap per request in production), which
// OOM-killed the container. The slim projection must be used when available.
func TestLoadSessionsWithTopStrategies_PrefersSlimProjection(t *testing.T) {
	sessionIDs := []string{"session-20260915-daily", "session-20260916-daily"}
	ledgerDir := writeSessionSummaries(t, sessionIDs)
	store := &slimSessionStore{
		mockOutcomeStore: &mockOutcomeStore{},
		outcomes: []domain.RecommendationOutcome{
			{AgentID: "a-low", Symbol: "2330", Conviction: 10},
			{AgentID: "a-high", Symbol: "2317", Conviction: 90},
			{AgentID: "a-mid", Symbol: "2454", Conviction: 50},
		},
	}
	svc := NewPipelineService(t.TempDir(), ledgerDir, store)

	sessions, err := svc.LoadSessionsWithTopStrategies(2, 0)
	if err != nil {
		t.Fatalf("LoadSessionsWithTopStrategies: %v", err)
	}
	if len(sessions) != len(sessionIDs) {
		t.Fatalf("expected %d sessions, got %d", len(sessionIDs), len(sessions))
	}
	if got := store.slimCalls.Load(); got != int64(len(sessionIDs)) {
		t.Errorf("expected %d slim per-session reads, got %d", len(sessionIDs), got)
	}
	if got := store.fullCalls.Load(); got != 0 {
		t.Errorf("full metadata read must not be used when the slim projection exists, got %d", got)
	}
	for _, s := range sessions {
		if len(s.TopStrategies) != 2 {
			t.Fatalf("session %s: expected top 2 strategies, got %d", s.SessionID, len(s.TopStrategies))
		}
		if s.TopStrategies[0].AgentID != "a-high" || s.TopStrategies[1].AgentID != "a-mid" {
			t.Errorf("session %s: top strategies not ranked by conviction: %+v", s.SessionID, s.TopStrategies)
		}
	}
}

// TestLoadSessionsWithTopStrategies_FallsBackWithoutSlim ensures stores that
// lack the projection keep working through the full read.
func TestLoadSessionsWithTopStrategies_FallsBackWithoutSlim(t *testing.T) {
	ledgerDir := writeSessionSummaries(t, []string{"s1"})
	store := &mockOutcomeStore{}
	svc := NewPipelineService(t.TempDir(), ledgerDir, store)

	sessions, err := svc.LoadSessionsWithTopStrategies(3, 0)
	if err != nil {
		t.Fatalf("LoadSessionsWithTopStrategies: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].TopStrategies == nil {
		t.Error("expected an empty (non-nil) TopStrategies slice")
	}
}

// writeSessionSummaries lays out <tmp>/sessions/<id>/summary.json the way the
// ledger store does, so LoadSessions sees the sessions.
func writeSessionSummaries(t *testing.T, sessionIDs []string) string {
	t.Helper()
	ledgerDir := t.TempDir()
	for _, id := range sessionIDs {
		dir := filepath.Join(ledgerDir, "sessions", id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		payload, err := json.Marshal(domain.SessionSummary{
			SessionID:    id,
			RecordedAt:   time.Now(),
			OutcomeCount: 3,
		})
		if err != nil {
			t.Fatalf("marshal summary: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "summary.json"), payload, 0o644); err != nil {
			t.Fatalf("write summary: %v", err)
		}
	}
	return ledgerDir
}
