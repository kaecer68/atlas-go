//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestDualWriteOutcomes_RecordAndQueryBySession(t *testing.T) {
	repo := newTestDualWrite(t)
	ctx := context.Background()

	// A01: RecordOutcomes is the GLOBAL write path — session_id is '' (mirror
	// of SQLiteOutcomeStore.RecordOutcomes), Window is preserved in metadata.
	// Session-scoped rows are written via RecordSessionOutcomes (session.ID).
	outcomes := []domain.RecommendationOutcome{
		{Window: "session-out-1", Symbol: "2330.TW", AgentID: "agent_a", PassedGuards: true, Conviction: 80},
		{Window: "session-out-1", Symbol: "2317.TW", AgentID: "agent_a", PassedGuards: false, Conviction: 60},
	}
	if err := repo.RecordOutcomes(ctx, outcomes); err != nil {
		t.Fatalf("RecordOutcomes failed: %v", err)
	}

	// Session query: PG has no session_id="session-out-1" rows (global rows
	// carry ''), so the dual-write falls back to the JSONL store — Window is
	// the JSONL session key, so the outcomes are still found there.
	results, err := repo.QueryOutcomesBySession(ctx, "session-out-1")
	if err != nil {
		t.Fatalf("QueryOutcomesBySession failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 outcomes, got %d", len(results))
	}

	// Verify data round-trip
	found2330 := false
	found2317 := false
	for _, o := range results {
		if o.Window == "session-out-1" && o.Symbol == "2330.TW" {
			found2330 = true
			if o.AgentID != "agent_a" {
				t.Errorf("Expected agent_a, got %q", o.AgentID)
			}
		}
		if o.Window == "session-out-1" && o.Symbol == "2317.TW" {
			found2317 = true
		}
	}
	if !found2330 || !found2317 {
		t.Errorf("Missing outcomes: 2330=%v, 2317=%v", found2330, found2317)
	}

	// Global rows land in PG with session_id='' — QueryAllOutcomes must see them.
	all, err := repo.QueryAllOutcomes(ctx)
	if err != nil {
		t.Fatalf("QueryAllOutcomes failed: %v", err)
	}
	if len(all) == 0 {
		t.Fatal("Expected global outcomes to be queryable via QueryAllOutcomes")
	}
}

func TestDualWriteOutcomes_QueryBySymbol(t *testing.T) {
	repo := newTestDualWrite(t)
	ctx := context.Background()

	outcomes := []domain.RecommendationOutcome{
		{Window: "sess-1", Symbol: "2330.TW", AgentID: "a1"},
		{Window: "sess-1", Symbol: "2330.TW", AgentID: "a2"},
		{Window: "sess-1", Symbol: "2317.TW", AgentID: "a1"},
	}
	if err := repo.RecordOutcomes(ctx, outcomes); err != nil {
		t.Fatalf("RecordOutcomes failed: %v", err)
	}

	start := time.Now().Add(-time.Hour)
	end := time.Now().Add(time.Hour)

	results, err := repo.QueryOutcomesBySymbol(ctx, "2330.TW", start, end)
	if err != nil {
		t.Fatalf("QueryOutcomesBySymbol failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 outcomes for 2330.TW, got %d", len(results))
	}
}

func TestDualWriteOutcomes_QueryByAgent(t *testing.T) {
	repo := newTestDualWrite(t)
	ctx := context.Background()

	outcomes := []domain.RecommendationOutcome{
		{Window: "sess-2", Symbol: "2330.TW", AgentID: "target_agent"},
		{Window: "sess-2", Symbol: "2317.TW", AgentID: "target_agent"},
		{Window: "sess-2", Symbol: "2454.TW", AgentID: "other_agent"},
	}
	if err := repo.RecordOutcomes(ctx, outcomes); err != nil {
		t.Fatalf("RecordOutcomes failed: %v", err)
	}

	start := time.Now().Add(-time.Hour)
	end := time.Now().Add(time.Hour)

	results, err := repo.QueryOutcomesByAgent(ctx, "target_agent", start, end)
	if err != nil {
		t.Fatalf("QueryOutcomesByAgent failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 outcomes for target_agent, got %d", len(results))
	}
}

func TestDualWriteOutcomes_QueryPassRate(t *testing.T) {
	repo := newTestDualWrite(t)
	ctx := context.Background()

	outcomes := []domain.RecommendationOutcome{
		{Window: "sess-pr", Symbol: "A", AgentID: "pr_agent", PassedGuards: true},
		{Window: "sess-pr", Symbol: "B", AgentID: "pr_agent", PassedGuards: true},
		{Window: "sess-pr", Symbol: "C", AgentID: "pr_agent", PassedGuards: false},
	}
	if err := repo.RecordOutcomes(ctx, outcomes); err != nil {
		t.Fatalf("RecordOutcomes failed: %v", err)
	}

	rate, err := repo.QueryPassRate(ctx, "pr_agent", time.Hour)
	if err != nil {
		t.Fatalf("QueryPassRate failed: %v", err)
	}
	if rate != 2.0/3.0 {
		t.Errorf("Expected pass rate ~0.667, got %f", rate)
	}
}

func TestDualWriteOutcomes_QueryTopSymbols(t *testing.T) {
	repo := newTestDualWrite(t)
	ctx := context.Background()

	outcomes := []domain.RecommendationOutcome{
		{Window: "sess-ts", Symbol: "2330.TW", AgentID: "a1"},
		{Window: "sess-ts", Symbol: "2330.TW", AgentID: "a2"},
		{Window: "sess-ts", Symbol: "2330.TW", AgentID: "a3"},
		{Window: "sess-ts", Symbol: "2317.TW", AgentID: "a1"},
		{Window: "sess-ts", Symbol: "2317.TW", AgentID: "a2"},
	}
	if err := repo.RecordOutcomes(ctx, outcomes); err != nil {
		t.Fatalf("RecordOutcomes failed: %v", err)
	}

	start := time.Now().Add(-time.Hour)
	end := time.Now().Add(time.Hour)

	top, err := repo.QueryTopSymbols(ctx, 5, start, end)
	if err != nil {
		t.Fatalf("QueryTopSymbols failed: %v", err)
	}
	if len(top) < 2 {
		t.Fatalf("Expected at least 2 symbols, got %d", len(top))
	}
	// 2330.TW should be first (3 occurrences)
	if top[0].Symbol != "2330.TW" {
		t.Errorf("Expected top symbol 2330.TW, got %q", top[0].Symbol)
	}
	if top[0].Count != 3 {
		t.Errorf("Expected count 3 for 2330.TW, got %d", top[0].Count)
	}
}

func TestDualWriteOutcomes_QueryAllOutcomes(t *testing.T) {
	repo := newTestDualWrite(t)
	ctx := context.Background()

	outcomes := []domain.RecommendationOutcome{
		{Window: "sess-qa", Symbol: "2330.TW", AgentID: "a1"},
	}
	if err := repo.RecordOutcomes(ctx, outcomes); err != nil {
		t.Fatalf("RecordOutcomes failed: %v", err)
	}

	all, err := repo.QueryAllOutcomes(ctx)
	if err != nil {
		t.Fatalf("QueryAllOutcomes failed: %v", err)
	}
	if len(all) == 0 {
		t.Fatal("Expected at least 1 outcome from QueryAllOutcomes")
	}
}

func TestDualWriteOutcomes_SessionLifecycle(t *testing.T) {
	repo := newTestDualWrite(t)
	ctx := context.Background()

	// RecordSessionOutcomes
	session := domain.ReplaySession{ID: "replay-sess-1"}
	outcomes := []domain.RecommendationOutcome{
		{Window: "replay-sess-1", Symbol: "2330.TW", AgentID: "a1"},
	}
	if err := repo.RecordSessionOutcomes(ctx, session, outcomes); err != nil {
		t.Fatalf("RecordSessionOutcomes failed: %v", err)
	}

	// RecordExperiment (JSONL-only — no PG side effect)
	experiment := domain.ExperimentRecord{
		ID:            "exp-001",
		TargetAgentID: "a1",
	}
	if err := repo.RecordExperiment(ctx, experiment); err != nil {
		t.Fatalf("RecordExperiment failed: %v", err)
	}

	// RecordSessionExperiment (JSONL-only)
	if err := repo.RecordSessionExperiment(ctx, session, experiment); err != nil {
		t.Fatalf("RecordSessionExperiment failed: %v", err)
	}
}
