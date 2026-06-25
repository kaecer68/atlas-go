package orchestrator

import "testing"

func TestAgentLoop_NewDefaultsToInitial(t *testing.T) {
	l := NewAgentLoop(0)
	if l.Phase != PhaseInitial {
		t.Errorf("Phase = %q, want %q", l.Phase, PhaseInitial)
	}
	if l.MaxIter != 3 {
		t.Errorf("MaxIter = %d, want 3 (default)", l.MaxIter)
	}
}

func TestAgentLoop_PlanReflectFinalSequence(t *testing.T) {
	l := NewAgentLoop(3)
	l.AdvancePlan([]PlanStep{{Kind: "tool", ToolName: "query_momentum"}})
	if l.Phase != PhasePlan {
		t.Errorf("after AdvancePlan: Phase = %q, want %q", l.Phase, PhasePlan)
	}
	l.AdvanceToolCall()
	if l.Phase != PhaseToolCall {
		t.Errorf("after AdvanceToolCall: Phase = %q, want %q", l.Phase, PhaseToolCall)
	}
	l.AdvanceReflect()
	if l.Phase != PhaseReflect {
		t.Errorf("after AdvanceReflect: Phase = %q, want %q", l.Phase, PhaseReflect)
	}
	l.AdvanceFinal(75)
	if l.Phase != PhaseFinal {
		t.Errorf("after AdvanceFinal: Phase = %q, want %q", l.Phase, PhaseFinal)
	}
	if !l.IsTerminal() {
		t.Error("IsTerminal should be true in PhaseFinal")
	}
}

func TestAgentLoop_AdvanceFinalClampsConviction(t *testing.T) {
	l := NewAgentLoop(3)
	l.AdvanceFinal(150)
	if !l.IsTerminal() {
		t.Error("IsTerminal should be true after AdvanceFinal")
	}
	l2 := NewAgentLoop(3)
	l2.AdvanceFinal(-5)
	if !l2.IsTerminal() {
		t.Error("IsTerminal should be true even with negative conviction")
	}
}

func TestAgentLoop_ExhaustedAfterMaxIter(t *testing.T) {
	l := NewAgentLoop(2)
	if l.Exhausted() {
		t.Error("fresh loop should not be exhausted")
	}
	l.AdvancePlan([]PlanStep{{Kind: "tool"}})
	l.AdvancePlan([]PlanStep{{Kind: "tool"}})
	if !l.Exhausted() {
		t.Error("loop should be exhausted after MaxIter steps")
	}
}

func TestAgentLoop_AdvanceToolCallOnlyFromPlan(t *testing.T) {
	l := NewAgentLoop(3)
	l.AdvanceToolCall()
	if l.Phase != PhaseInitial {
		t.Errorf("AdvanceToolCall from Initial should be no-op, got %q", l.Phase)
	}
}
