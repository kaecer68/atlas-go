package live

import (
	"context"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	livestore "github.com/kaecer68/atlas-go/internal/live/store"
)

func TestAgentRunner_ApplyExecutionInput(t *testing.T) {
	s := livestore.NewStateStore(t.TempDir())
	runner := &AgentRunner{
		stateStore: s,
		marketData: nil,
		system:     nil,
	}

	input := ExecutionInput{
		Regime:               domain.RegimeRiskOn,
		RawRecommendations:   []domain.Recommendation{{Agent: "sector_semiconductor", Symbol: "2330.TW"}},
		FinalRecommendations: []domain.Recommendation{{Agent: "sector_semiconductor", Symbol: "2330.TW"}},
		DeterminedBy:         "orchestrator-pipeline-v1",
	}

	err := runner.ApplyExecutionInput(context.Background(), input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if got := store.GetCurrentRegime(); got != domain.RegimeRiskOn {
		t.Errorf("expected regime RiskOn, got %v", got)
	}

	pending := store.GetPendingRecommendations()
	if len(pending) != 1 || pending[0].Symbol != "2330.TW" {
		t.Errorf("expected 1 pending rec for 2330.TW, got %v", pending)
	}

	filtered := store.GetFilteredRecommendations()
	if len(filtered) != 1 || filtered[0].Symbol != "2330.TW" {
		t.Errorf("expected 1 filtered rec for 2330.TW, got %v", filtered)
	}
}

func TestAgentRunner_ApplyExecutionInput_EmptyRecommendations(t *testing.T) {
	s := livestore.NewStateStore(t.TempDir())
	runner := &AgentRunner{stateStore: store}

	input := ExecutionInput{
		Regime:               domain.RegimeNeutral,
		RawRecommendations:   nil,
		FinalRecommendations: nil,
		DeterminedBy:         "test",
	}

	err := runner.ApplyExecutionInput(context.Background(), input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if got := store.GetCurrentRegime(); got != domain.RegimeNeutral {
		t.Errorf("expected regime Neutral, got %v", got)
	}

	if pending := store.GetPendingRecommendations(); len(pending) != 0 {
		t.Errorf("expected no pending recs, got %v", pending)
	}
}
