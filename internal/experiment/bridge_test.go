package experiment

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/acceptance"
	"github.com/kaecer68/atlas-go/internal/acceptance/builtin"
	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestBridge_LegacyAndPipelineAgree(t *testing.T) {
	input := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			BaselineValue:  1.0,
			CandidateValue: 1.1,
			AcceptanceGates: []string{
				"improve_sharpe_like",
				"no_drawdown_spike",
				"retail_sentiment_filter",
			},
		},
		Brief: domain.MutationBrief{
			RSITwScore:    0.3,
			MaturityLevel: "level_3_regime_aware",
		},
		BaselineReturns:       []float64{0.01, 0.02, 0.015, 0.018, 0.02, 0.017, 0.019, 0.02, 0.018, 0.021, 0.019, 0.02},
		CandidateReturns:      []float64{0.015, 0.025, 0.02, 0.022, 0.024, 0.02, 0.023, 0.025, 0.022, 0.026, 0.023, 0.025},
		BaselineObservations:  12,
		CandidateObservations: 12,
		OOSResult:             &domain.OOSResult{Passed: true, Reason: "ok"},
	}

	registry := acceptance.NewRegistry()
	registry.Register(builtin.ImproveSharpeLike())
	registry.Register(builtin.NoDrawdownSpike())
	registry.Register(builtin.RetailSentimentFilter())

	params := acceptance.EvalParams{}

	legacyResult := legacyCheckGates(input, params)
	pipelineResult := true
	for _, gate := range input.Experiment.AcceptanceGates {
		e, ok := registry.Get(gate)
		if !ok {
			t.Fatalf("pipeline missing evaluator for %q", gate)
		}
		if !e.Eval(input, params).Passed {
			pipelineResult = false
			break
		}
	}

	if legacyResult != pipelineResult {
		t.Errorf("legacy and pipeline disagree: legacy=%v pipeline=%v", legacyResult, pipelineResult)
	}
}

func legacyCheckGates(input domain.PromptExperimentResult, _ acceptance.EvalParams) bool {
	for _, gate := range input.Experiment.AcceptanceGates {
		switch gate {
		case "improve_sharpe_like":
			if input.Experiment.CandidateValue <= input.Experiment.BaselineValue {
				return false
			}
		case "no_drawdown_spike":
			if input.OOSResult != nil && !input.OOSResult.Passed {
				return false
			}
		case "retail_sentiment_filter":
			if abs(input.Brief.RSITwScore) >= 0.7 {
				return false
			}
		}
	}
	return true
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
