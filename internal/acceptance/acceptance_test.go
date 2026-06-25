package acceptance

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestPipeline_AllPass(t *testing.T) {
	p := NewPipeline(
		FuncEvaluator{N: "always_pass", F: func(_ domain.PromptExperimentResult, _ EvalParams) Result {
			return Result{Passed: true}
		}},
	)
	input := domain.PromptExperimentResult{}
	ok, reason := p.Run(input, EvalParams{})
	if !ok {
		t.Errorf("expected pass, got fail: %s", reason)
	}
}

func TestPipeline_ShortCircuitsOnFirstFail(t *testing.T) {
	calls := 0
	p := NewPipeline(
		FuncEvaluator{N: "fail_first", F: func(_ domain.PromptExperimentResult, _ EvalParams) Result {
			calls++
			return Result{Passed: false, Reason: "first fails"}
		}},
		FuncEvaluator{N: "never_called", F: func(_ domain.PromptExperimentResult, _ EvalParams) Result {
			calls++
			return Result{Passed: true}
		}},
	)
	ok, reason := p.Run(domain.PromptExperimentResult{}, EvalParams{})
	if ok {
		t.Error("expected fail, got pass")
	}
	if calls != 1 {
		t.Errorf("expected 1 call (short-circuit), got %d", calls)
	}
	if reason != "fail_first: first fails" {
		t.Errorf("unexpected reason: %s", reason)
	}
}

func TestPipeline_Empty(t *testing.T) {
	p := NewPipeline()
	ok, _ := p.Run(domain.PromptExperimentResult{}, EvalParams{})
	if !ok {
		t.Error("empty pipeline should pass")
	}
}
