package orchestrator

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// TestBuiltinAgentExecutors_LLMSectorAgentPrecedesDeterministic pins the
// registration order fixed by #1944 Batch 4 item I19: PluginRegistry
// .Recommendation resolves first-match-wins, so the LLM-driven semiconductor
// agent must be registered before the deterministic SemiconductorExecutor or
// it can never be selected. This test fails if someone re-orders the list
// back into the (unreachable) L2.3 layout.
func TestBuiltinAgentExecutors_LLMSectorAgentPrecedesDeterministic(t *testing.T) {
	executors := builtinAgentExecutors()
	llmIdx, detIdx := -1, -1
	for i, exec := range executors {
		switch exec.(type) {
		case SemiconductorLLMAgent:
			if llmIdx == -1 {
				llmIdx = i
			}
		case SemiconductorExecutor:
			if detIdx == -1 {
				detIdx = i
			}
		}
	}
	if llmIdx == -1 {
		t.Fatal("builtinAgentExecutors is missing SemiconductorLLMAgent")
	}
	if detIdx == -1 {
		t.Fatal("builtinAgentExecutors is missing SemiconductorExecutor")
	}
	if llmIdx > detIdx {
		t.Fatalf("SemiconductorLLMAgent registered at %d, SemiconductorExecutor at %d: the LLM agent is unreachable (first-match-wins) — keep the LLM agent first (I19)", llmIdx, detIdx)
	}
}

// TestResolveAgentExecutor_FirstMatchWins pins the routing rule itself.
func TestResolveAgentExecutor_FirstMatchWins(t *testing.T) {
	first := agentExecutorStub{id: "first", supports: true, handled: true}
	second := agentExecutorStub{id: "second", supports: true, handled: true}
	got, ok := resolveAgentExecutor([]AgentExecutor{first, second}, makeSpec())
	if !ok {
		t.Fatal("resolveAgentExecutor: got ok=false, want true")
	}
	if got != first {
		t.Fatalf("resolveAgentExecutor picked %v, want the first supporting executor", got)
	}

	if _, ok := resolveAgentExecutor([]AgentExecutor{agentExecutorStub{id: "no", supports: false}}, makeSpec()); ok {
		t.Error("resolveAgentExecutor: got ok=true for a non-supporting executor, want false")
	}
}

// TestResolveAgentExecutor_LLMAgentClaimsDeskOnlyWhenWired is the end-to-end
// routing proof for I19: with the fixed order, a wired LLM agent serves the
// semiconductor desk, while an unwired one (the only registration production
// has today) steps aside so the deterministic executor keeps serving it.
func TestResolveAgentExecutor_LLMAgentClaimsDeskOnlyWhenWired(t *testing.T) {
	spec := makeSpec()

	mock := NewMockLLMDriver()
	wired := SemiconductorLLMAgent{
		PlanDriver:     mock,
		ReflectDriver:  mock,
		UseLLMOverride: truePtr(),
	}
	unwired := SemiconductorLLMAgent{UseLLMOverride: truePtr()}

	cases := []struct {
		name      string
		executors []AgentExecutor
		wantLLM   bool
	}{
		{"wired LLM agent + flag on", []AgentExecutor{wired, SemiconductorExecutor{}}, true},
		{"unwired LLM agent + flag on", []AgentExecutor{unwired, SemiconductorExecutor{}}, false},
		{"LLM agent flag off", []AgentExecutor{SemiconductorLLMAgent{PlanDriver: mock, ReflectDriver: mock, UseLLMOverride: falsePtr()}, SemiconductorExecutor{}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := resolveAgentExecutor(tc.executors, spec)
			if !ok {
				t.Fatal("no executor claimed the semiconductor desk")
			}
			_, isLLM := got.(SemiconductorLLMAgent)
			if isLLM != tc.wantLLM {
				t.Fatalf("routed to %T (isLLM=%v), want isLLM=%v", got, isLLM, tc.wantLLM)
			}
		})
	}
}

// agentExecutorStub is a minimal AgentExecutor for routing tests.
type agentExecutorStub struct {
	id       string
	supports bool
	handled  bool
}

func (s agentExecutorStub) Supports(domain.AgentSpec) bool { return s.supports }

func (s agentExecutorStub) Recommend(domain.AgentSpec, domain.Quote, string, domain.Regime, FactorQuery) (domain.Recommendation, bool) {
	return domain.Recommendation{Agent: s.id}, s.handled
}
