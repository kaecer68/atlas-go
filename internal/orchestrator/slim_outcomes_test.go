package orchestrator

import (
	"sync/atomic"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

// projectionStore records which outcome read path candidate selection used.
type projectionStore struct {
	ledger.OutcomeStore
	slimCalls atomic.Int64
	fullCalls atomic.Int64
	slim      []domain.RecommendationOutcome
	full      []domain.RecommendationOutcome
}

func (s *projectionStore) LoadScorecardOutcomes() ([]domain.RecommendationOutcome, error) {
	s.slimCalls.Add(1)
	return s.slim, nil
}

func (s *projectionStore) LoadOutcomesFromSessions() ([]domain.RecommendationOutcome, error) {
	s.fullCalls.Add(1)
	return s.full, nil
}

// TestLoadScorecardOutcomesSlim_PrefersProjection locks the fix for the
// 2026-09-17 OOM loop: NextExperimentCandidate ran inside the in-container
// backtest scheduler every ~5 minutes and its full read of the outcomes table
// (metadata JSONB, 644 MB) expanded to ~1.7 GB of heap.
func TestLoadScorecardOutcomesSlim_PrefersProjection(t *testing.T) {
	store := &projectionStore{slim: []domain.RecommendationOutcome{{AgentID: "a1", Hit: true}}}
	outcomes, err := loadScorecardOutcomesSlim(store)
	if err != nil {
		t.Fatalf("loadScorecardOutcomesSlim: %v", err)
	}
	if len(outcomes) != 1 || outcomes[0].AgentID != "a1" {
		t.Fatalf("expected the slim projection result, got %+v", outcomes)
	}
	if got := store.fullCalls.Load(); got != 0 {
		t.Errorf("full read must not be used when the slim projection exists, got %d", got)
	}
}

// TestLoadScorecardOutcomesSlim_FallsBackWithoutProjection ensures stores
// without the projection keep the previous behavior.
func TestLoadScorecardOutcomesSlim_FallsBackWithoutProjection(t *testing.T) {
	store := &projectionStore{full: []domain.RecommendationOutcome{{AgentID: "full"}}}
	outcomes, err := loadScorecardOutcomesSlim(store)
	if err != nil {
		t.Fatalf("loadScorecardOutcomesSlim: %v", err)
	}
	if len(outcomes) != 1 || outcomes[0].AgentID != "full" {
		t.Fatalf("expected the full read result, got %+v", outcomes)
	}
}
