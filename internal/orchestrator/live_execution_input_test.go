package orchestrator

import (
	"context"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestAdapterProducer_ImplementsInterface(t *testing.T) {
	var _ LiveExecutionInputProvider = (*AdapterProducer)(nil)
}

func TestAdapterProducer_ProducesDomainInput(t *testing.T) {
	p := &AdapterProducer{}
	_ = p
}

func TestErrSystemNotInitialized(t *testing.T) {
	if ErrSystemNotInitialized.Error() != "system not initialized" {
		t.Errorf("unexpected error message: %v", ErrSystemNotInitialized)
	}
}

func TestExecutionInputFields(t *testing.T) {
	input := domain.ExecutionInput{
		Regime:               domain.RegimeRiskOn,
		RawRecommendations:   []domain.Recommendation{{Agent: "test", Symbol: "2330.TW"}},
		FinalRecommendations: []domain.Recommendation{{Agent: "test", Symbol: "2330.TW"}},
		GuardOutcomes:        []domain.GuardOutcome{},
		DeterminedBy:         "test",
	}

	if input.Regime != domain.RegimeRiskOn {
		t.Errorf("expected RegimeRiskOn, got %v", input.Regime)
	}
	if len(input.RawRecommendations) != 1 {
		t.Errorf("expected 1 raw recommendation, got %d", len(input.RawRecommendations))
	}
	if input.DeterminedBy != "test" {
		t.Errorf("expected DeterminedBy=test, got %v", input.DeterminedBy)
	}
}

func TestAdapterProducer_Produce_SystemNotInitialized(t *testing.T) {
	p := &AdapterProducer{marketData: nil, system: nil}
	_, err := p.Produce(context.Background(), nil)
	if err != ErrSystemNotInitialized {
		t.Errorf("expected ErrSystemNotInitialized, got %v", err)
	}
}
