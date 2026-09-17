package backtest

import (
	"sync/atomic"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

// slimScorecardStore embeds the ledger.OutcomeStore interface (nil) so only the
// two projection methods the runner uses need real implementations. It records
// which read path was taken.
type slimScorecardStore struct {
	ledger.OutcomeStore
	slimCalls atomic.Int64
	fullCalls atomic.Int64
	outcomes  []domain.RecommendationOutcome
}

func (s *slimScorecardStore) LoadScorecardOutcomes() ([]domain.RecommendationOutcome, error) {
	s.slimCalls.Add(1)
	return s.outcomes, nil
}

func (s *slimScorecardStore) LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	s.fullCalls.Add(1)
	return nil, nil, nil
}

// TestLoadAllSessionScorecards_PrefersSlimProjection locks the fix for the
// 2026-09-17 OOM loop: the in-container backtest scheduler called
// LoadAllSessionScorecards, which transferred the whole metadata JSONB (644 MB)
// and expanded to ~1.7 GB of heap every ~5 minutes.
func TestLoadAllSessionScorecards_PrefersSlimProjection(t *testing.T) {
	store := &slimScorecardStore{outcomes: []domain.RecommendationOutcome{
		{AgentID: "a1", Skill: "momentum", Layer: "L1", Window: "2026-09-16", ForwardReturn: 0.02, Hit: true},
		{AgentID: "a1", Skill: "momentum", Layer: "L1", Window: "2026-09-15", ForwardReturn: -0.01, Hit: false},
	}}
	runner := &Runner{store: store}

	scorecards, outcomes, err := runner.loadAllSessionScorecards()
	if err != nil {
		t.Fatalf("loadAllSessionScorecards: %v", err)
	}
	if got := store.slimCalls.Load(); got != 1 {
		t.Errorf("expected 1 slim scorecard read, got %d", got)
	}
	if got := store.fullCalls.Load(); got != 0 {
		t.Errorf("full LoadAllSessionScorecards must not be used when the projection exists, got %d", got)
	}
	if len(outcomes) != 2 {
		t.Errorf("expected 2 outcomes, got %d", len(outcomes))
	}
	if len(scorecards) != 1 || scorecards[0].AgentID != "a1" {
		t.Fatalf("expected one scorecard for a1, got %+v", scorecards)
	}
}
