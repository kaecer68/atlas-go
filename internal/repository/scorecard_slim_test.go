package repository

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// repoScorecardSlimStore is a repository.OutcomeStore that ALSO implements the
// optional LoadScorecardOutcomes slim projection.
type repoScorecardSlimStore struct {
	*mockOutcomeStore
	slimOutcomes []domain.RecommendationOutcome
	slimCalls    int
}

func (s *repoScorecardSlimStore) LoadScorecardOutcomes() ([]domain.RecommendationOutcome, error) {
	s.slimCalls++
	return s.slimOutcomes, nil
}

func scorecardSlimFixture() []domain.RecommendationOutcome {
	return []domain.RecommendationOutcome{
		{
			AgentID:       "repo-slim-agent",
			Skill:         "sector-tech",
			Window:        "2026-06-01",
			ForwardReturn: 0.025,
			Hit:           true,
			RecordedAt:    time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		},
	}
}

// TestDualWriteRepository_QueryScorecardOutcomes_UsesSlim proves the repo
// delegates to the optional slim loader when the JSONL outcome store
// implements it — with no fallback counting.
func TestDualWriteRepository_QueryScorecardOutcomes_UsesSlim(t *testing.T) {
	slimStore := &repoScorecardSlimStore{
		mockOutcomeStore: &mockOutcomeStore{},
		slimOutcomes:     scorecardSlimFixture(),
	}
	repo := NewDualWriteRepository(nil, &mockAlertStore{}, &mockMetricsStore{}, slimStore,
		&mockScreeningRejectStore{}, &mockSessionSummaryStore{}, &mockHumanInterventionStore{})

	before := ScorecardSlimRepoFallbackTotal()
	got, err := repo.QueryScorecardOutcomes(context.Background())
	if err != nil {
		t.Fatalf("QueryScorecardOutcomes: %v", err)
	}
	if len(got) != 1 || got[0].AgentID != "repo-slim-agent" {
		t.Fatalf("expected slim outcomes, got %+v", got)
	}
	if slimStore.slimCalls != 1 {
		t.Errorf("expected 1 slim call, got %d", slimStore.slimCalls)
	}
	if after := ScorecardSlimRepoFallbackTotal(); after != before {
		t.Errorf("repo fallback counter must stay flat on the slim path: before=%d after=%d", before, after)
	}
}

// TestDualWriteRepository_QueryScorecardOutcomes_FallsBack proves a JSONL
// outcome store without the optional loader keeps the pre-#1780 full-read
// behavior and increments the fallback counter (B1).
func TestDualWriteRepository_QueryScorecardOutcomes_FallsBack(t *testing.T) {
	plainStore := &mockOutcomeStore{outcomes: scorecardSlimFixture()}
	repo := NewDualWriteRepository(nil, &mockAlertStore{}, &mockMetricsStore{}, plainStore,
		&mockScreeningRejectStore{}, &mockSessionSummaryStore{}, &mockHumanInterventionStore{})

	before := ScorecardSlimRepoFallbackTotal()
	got, err := repo.QueryScorecardOutcomes(context.Background())
	if err != nil {
		t.Fatalf("QueryScorecardOutcomes: %v", err)
	}
	if len(got) != 1 || got[0].AgentID != "repo-slim-agent" {
		t.Fatalf("expected full-read fallback outcomes, got %+v", got)
	}
	if after := ScorecardSlimRepoFallbackTotal(); after != before+1 {
		t.Errorf("repo fallback counter delta = %d, want 1", after-before)
	}
}

// TestDualWriteRepository_QueryScorecardOutcomes_NilStore guards the JSON-only
// cmd paths that construct DualWriteRepository with nil outcome stores: the
// new method must fail loudly instead of panicking inside QueryAllOutcomes.
func TestDualWriteRepository_QueryScorecardOutcomes_NilStore(t *testing.T) {
	repo := NewDualWriteRepository(nil, nil, nil, nil, nil, nil, nil)
	_, err := repo.QueryScorecardOutcomes(context.Background())
	if err == nil {
		t.Fatal("expected error for nil outcome store, got nil")
	}
}
