package builtin

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/acceptance"
	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestImproveSharpeLike_Pass(t *testing.T) {
	e := ImproveSharpeLike()
	input := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{BaselineValue: 1.0, CandidateValue: 1.1},
	}
	r := e.Eval(input, acceptance.EvalParams{})
	if !r.Passed {
		t.Errorf("expected pass, got: %s", r.Reason)
	}
}

func TestImproveSharpeLike_Fail(t *testing.T) {
	e := ImproveSharpeLike()
	input := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{BaselineValue: 1.0, CandidateValue: 0.9},
	}
	r := e.Eval(input, acceptance.EvalParams{})
	if r.Passed {
		t.Error("expected fail")
	}
}

func TestPreserveDownsideProtection_Pass(t *testing.T) {
	e := PreserveDownsideProtection()
	input := domain.PromptExperimentResult{
		BaselineReturns:  []float64{0.1, -0.05, 0.08, 0.03},
		CandidateReturns: []float64{0.08, -0.04, 0.06, 0.02},
	}
	r := e.Eval(input, acceptance.EvalParams{DrawdownProtectionRatio: 0.8})
	if !r.Passed {
		t.Errorf("expected pass, got: %s", r.Reason)
	}
}

func TestPreserveDownsideProtection_Fail(t *testing.T) {
	e := PreserveDownsideProtection()
	input := domain.PromptExperimentResult{
		BaselineReturns:  []float64{0.1, -0.05, 0.08, 0.03},
		CandidateReturns: []float64{0.05, -0.30, 0.02, 0.01},
	}
	r := e.Eval(input, acceptance.EvalParams{DrawdownProtectionRatio: 0.8})
	if r.Passed {
		t.Error("expected fail")
	}
}

func TestNoDrawdownSpike_Pass(t *testing.T) {
	e := NoDrawdownSpike()
	input := domain.PromptExperimentResult{
		OOSResult: &domain.OOSResult{Passed: true, Reason: "ok"},
	}
	r := e.Eval(input, acceptance.EvalParams{})
	if !r.Passed {
		t.Errorf("expected pass, got: %s", r.Reason)
	}
}

func TestNoDrawdownSpike_Fail(t *testing.T) {
	e := NoDrawdownSpike()
	input := domain.PromptExperimentResult{
		OOSResult: &domain.OOSResult{Passed: false, Reason: "drawdown spike"},
	}
	r := e.Eval(input, acceptance.EvalParams{})
	if r.Passed {
		t.Error("expected fail")
	}
}

func TestFactorWeightStability_Pass(t *testing.T) {
	e := FactorWeightStability()
	input := domain.PromptExperimentResult{
		BaselineFactorCount: 10, BaselineFallbackCount: 2,
		CandidateFactorCount: 10, CandidateFallbackCount: 3,
	}
	r := e.Eval(input, acceptance.EvalParams{FactorWeightDriftThreshold: 0.15})
	if !r.Passed {
		t.Errorf("expected pass, got: %s", r.Reason)
	}
}

func TestFactorWeightStability_Fail(t *testing.T) {
	e := FactorWeightStability()
	input := domain.PromptExperimentResult{
		BaselineFactorCount: 10, BaselineFallbackCount: 1,
		CandidateFactorCount: 10, CandidateFallbackCount: 8,
	}
	r := e.Eval(input, acceptance.EvalParams{FactorWeightDriftThreshold: 0.15})
	if r.Passed {
		t.Error("expected fail")
	}
}

func TestRetailSentimentFilter_Pass(t *testing.T) {
	e := RetailSentimentFilter()
	input := domain.PromptExperimentResult{
		Brief: domain.MutationBrief{RSITwScore: 0.3},
	}
	r := e.Eval(input, acceptance.EvalParams{})
	if !r.Passed {
		t.Errorf("expected pass, got: %s", r.Reason)
	}
}

func TestRetailSentimentFilter_RejectExtreme(t *testing.T) {
	e := RetailSentimentFilter()
	input := domain.PromptExperimentResult{
		Brief: domain.MutationBrief{RSITwScore: 0.85},
	}
	r := e.Eval(input, acceptance.EvalParams{})
	if r.Passed {
		t.Error("expected fail for extreme RSI")
	}
}
