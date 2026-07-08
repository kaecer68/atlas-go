package orchestrator

import (
	"errors"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/llm"
)

// truePtr / falsePtr are small helpers for building *bool values
// to set SemiconductorLLMAgent.UseLLMOverride in tests.
func truePtr() *bool  { b := true; return &b }
func falsePtr() *bool { b := false; return &b }

// makeSpec returns a domain.AgentSpec for the semiconductor desk.
func makeSpec() domain.AgentSpec {
	return domain.AgentSpec{
		ID:    "test_semiconductor",
		Name:  "Test Semiconductor",
		Skill: SemiconductorLLMAgentSkill,
	}
}

// TestSemiconductorLLMAgent_Supports_FlagOff verifies that
// Supports() returns false when the feature flag is off — the
// deterministic SemiconductorExecutor handles the spec.
func TestSemiconductorLLMAgent_Supports_FlagOff(t *testing.T) {
	agent := SemiconductorLLMAgent{UseLLMOverride: falsePtr()}
	if agent.Supports(makeSpec()) {
		t.Error("Supports with UseLLMOverride=false: got true, want false")
	}
}

// TestSemiconductorLLMAgent_Supports_FlagOn verifies that
// Supports() returns true when the feature flag is on.
func TestSemiconductorLLMAgent_Supports_FlagOn(t *testing.T) {
	agent := SemiconductorLLMAgent{UseLLMOverride: truePtr()}
	if !agent.Supports(makeSpec()) {
		t.Error("Supports with UseLLMOverride=true: got false, want true")
	}
}

// TestSemiconductorLLMAgent_Supports_WrongSkill verifies that
// Supports() returns false for any skill other than the
// semiconductor desk.
func TestSemiconductorLLMAgent_Supports_WrongSkill(t *testing.T) {
	agent := SemiconductorLLMAgent{UseLLMOverride: truePtr()}
	wrong := domain.AgentSpec{ID: "x", Skill: "wrong_skill"}
	if agent.Supports(wrong) {
		t.Error("Supports with wrong skill + flag on: got true, want false")
	}
}

// TestSemiconductorLLMAgent_StrategyMeta verifies the
// StrategyProvider interface returns sensible metadata.
func TestSemiconductorLLMAgent_StrategyMeta(t *testing.T) {
	agent := SemiconductorLLMAgent{}
	meta := agent.StrategyMeta()
	if meta.Skill != SemiconductorLLMAgentSkill {
		t.Errorf("meta.Skill = %q, want %q", meta.Skill, SemiconductorLLMAgentSkill)
	}
	if meta.ID != "semiconductor_llm" {
		t.Errorf("meta.ID = %q, want %q", meta.ID, "semiconductor_llm")
	}
}

// TestSemiconductorLLMAgent_EvaluatePosition_NotImplemented
// verifies that EvaluatePosition returns (zero, false) — position
// evaluation is out of scope for the L2.3 PoC.
func TestSemiconductorLLMAgent_EvaluatePosition_NotImplemented(t *testing.T) {
	agent := SemiconductorLLMAgent{UseLLMOverride: truePtr()}
	rec, ok := agent.EvaluatePosition(domain.Position{}, domain.Quote{}, makeSpec(), "", domain.Regime(""), nil)
	if ok {
		t.Error("EvaluatePosition: got ok=true, want false (out of scope for L2.3 PoC)")
	}
	_ = rec // should be zero-value Recommendation
}

// TestSemiconductorLLMAgent_Recommend_NoLLM verifies that Recommend
// returns (zero, false) when the LLM driver is nil — prevents
// silent stub data.
func TestSemiconductorLLMAgent_Recommend_NoLLM(t *testing.T) {
	agent := SemiconductorLLMAgent{UseLLMOverride: truePtr()}
	rec, ok := agent.Recommend(makeSpec(), domain.Quote{Symbol: "2330"}, "", domain.Regime(""), nil)
	if ok {
		t.Error("Recommend with nil PlanDriver/ReflectDriver: got ok=true, want false")
	}
	if rec.Agent != "" || rec.Symbol != "" {
		t.Errorf("Recommend with nil PlanDriver/ReflectDriver: got non-zero rec %+v, want zero", rec)
	}
}

// TestSemiconductorLLMAgent_Recommend_FlagOff verifies that Recommend
// short-circuits when the flag is off.
func TestSemiconductorLLMAgent_Recommend_FlagOff(t *testing.T) {
	mock := NewMockLLMDriver()
	agent := SemiconductorLLMAgent{
		PlanDriver:     mock,
		ReflectDriver:  mock,
		Tools:          llm.TestTools(),
		UseLLMOverride: falsePtr(),
	}
	rec, ok := agent.Recommend(makeSpec(), domain.Quote{Symbol: "2330"}, "", domain.Regime(""), nil)
	if ok {
		t.Error("Recommend with flag off: got ok=true, want false")
	}
	if mock.PlanCallCount() != 0 {
		t.Errorf("PlanCallCount = %d, want 0 (flag off should short-circuit)", mock.PlanCallCount())
	}
	_ = rec
}

// TestSemiconductorLLMAgent_Recommend_HappyPath verifies the
// end-to-end LLM-driven recommendation flow: plan → dispatch tool
// via registered TestTools → reflect → return final conviction
// and reasoning.
func TestSemiconductorLLMAgent_Recommend_HappyPath(t *testing.T) {
	mock := NewMockLLMDriver().
		WithPlanResponse([]PlanStep{
			{Kind: "tool", ToolName: "get_factor_weight", Args: map[string]any{"symbol": "2330"}},
		}).
		WithReflectResponse(Reflection{
			Continue:        false,
			FinalConviction: 75,
			Reasoning:       "Factors look strong; momentum confirmed.",
		})

	agent := SemiconductorLLMAgent{
		PlanDriver:     mock,
		ReflectDriver:  mock,
		Tools:          llm.TestTools(),
		MaxIter:        3,
		UseLLMOverride: truePtr(),
	}

	quote := domain.Quote{Symbol: "2330"}
	rec, ok := agent.Recommend(makeSpec(), quote, "", domain.Regime(""), nil)
	if !ok {
		t.Fatalf("Recommend happy path: got ok=false, want true; rec.Reason=%q", rec.Reason)
	}
	if rec.Conviction != 75 {
		t.Errorf("rec.Conviction = %d, want 75", rec.Conviction)
	}
	if rec.Reason == "" {
		t.Error("rec.Reason = empty, want non-empty reflection reasoning")
	}
	if strings.Contains(rec.Reason, "not yet implemented") {
		t.Errorf("rec.Reason = %q, should not contain 'not yet implemented'", rec.Reason)
	}
	if mock.PlanCallCount() != 1 {
		t.Errorf("PlanCallCount = %d, want 1", mock.PlanCallCount())
	}
	if mock.ReflectCallCount() != 1 {
		t.Errorf("ReflectCallCount = %d, want 1", mock.ReflectCallCount())
	}
}

// TestSemiconductorLLMAgent_Recommend_PlanError verifies that a
// plan error propagates as ok=false with a Reason describing the
// failure.
func TestSemiconductorLLMAgent_Recommend_PlanError(t *testing.T) {
	mock := NewMockLLMDriver().WithPlanError(errors.New("LLM unavailable"))
	agent := SemiconductorLLMAgent{
		PlanDriver:     mock,
		ReflectDriver:  mock,
		Tools:          llm.TestTools(),
		UseLLMOverride: truePtr(),
	}
	rec, ok := agent.Recommend(makeSpec(), domain.Quote{Symbol: "2330"}, "", domain.Regime(""), nil)
	if ok {
		t.Error("Recommend with plan error: got ok=true, want false")
	}
	if rec.Reason == "" {
		t.Error("rec.Reason should describe the plan error, got empty")
	}
}

// TestSemiconductorLLMAgent_PlanAndReflectSeparately verifies that
// after the LLMDriver → (PlanDriver + ReflectDriver) split, the two
// fields are wired independently:
//   - Each field gets called with its own call count (no shared state)
//   - Distinct mock implementations on each field receive separate calls
//   - Setting only one field (other nil) causes Recommend to return
//     ok=false per the L116 nil guard, and the non-nil field is NOT
//     called (early-return before any LLM invocation)
func TestSemiconductorLLMAgent_PlanAndReflectSeparately(t *testing.T) {
	t.Run("distinct drivers wired independently", func(t *testing.T) {
		planMock := NewMockLLMDriver().
			WithPlanResponse([]PlanStep{{Kind: "thought", Note: "noop"}})
		reflectMock := NewMockLLMDriver().
			WithReflectResponse(Reflection{Continue: false, Reasoning: "skip"})

		agent := SemiconductorLLMAgent{
			PlanDriver:     planMock,
			ReflectDriver:  reflectMock,
			Tools:          llm.TestTools(),
			UseLLMOverride: truePtr(),
		}
		_, _ = agent.Recommend(makeSpec(), domain.Quote{Symbol: "2330"}, "", domain.Regime(""), nil)

		if planMock.PlanCallCount() == 0 {
			t.Error("PlanDriver was never called; expected at least 1 PlanComplete call")
		}
	})

	t.Run("nil PlanDriver returns ok=false without calling ReflectDriver", func(t *testing.T) {
		reflectMock := NewMockLLMDriver().
			WithReflectResponse(Reflection{Continue: false, Reasoning: "skip"})

		agent := SemiconductorLLMAgent{
			PlanDriver:     nil,
			ReflectDriver:  reflectMock,
			UseLLMOverride: truePtr(),
		}
		_, ok := agent.Recommend(makeSpec(), domain.Quote{Symbol: "2330"}, "", domain.Regime(""), nil)
		if ok {
			t.Error("Recommend with nil PlanDriver: got ok=true, want false (per L116 nil guard)")
		}
		if reflectMock.ReflectCallCount() != 0 {
			t.Errorf("ReflectDriver should NOT be called when PlanDriver is nil, got %d calls",
				reflectMock.ReflectCallCount())
		}
	})

	t.Run("nil ReflectDriver returns ok=false without calling PlanDriver", func(t *testing.T) {
		planMock := NewMockLLMDriver().
			WithPlanResponse([]PlanStep{{Kind: "thought", Note: "noop"}})

		agent := SemiconductorLLMAgent{
			PlanDriver:     planMock,
			ReflectDriver:  nil,
			UseLLMOverride: truePtr(),
		}
		_, ok := agent.Recommend(makeSpec(), domain.Quote{Symbol: "2330"}, "", domain.Regime(""), nil)
		if ok {
			t.Error("Recommend with nil ReflectDriver: got ok=true, want false (per L116 nil guard)")
		}
		if planMock.PlanCallCount() != 0 {
			t.Errorf("PlanDriver should NOT be called when ReflectDriver is nil, got %d calls",
				planMock.PlanCallCount())
		}
	})
}
