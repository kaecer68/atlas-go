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
	if l.Round != 0 {
		t.Errorf("Round = %d, want 0 (fresh loop)", l.Round)
	}
}

// TestAgentLoop_PlanReflectFinalSequence walks the full happy path and
// confirms that AdvanceToolCall / AdvanceReflect (which now return error)
// succeed in correct phase order. Issue #711 #5: callers MUST handle the
// returned error.
func TestAgentLoop_PlanReflectFinalSequence(t *testing.T) {
	l := NewAgentLoop(3)
	l.AdvancePlan([]PlanStep{{Kind: "tool", ToolName: "query_momentum"}})
	if l.Phase != PhasePlan {
		t.Errorf("after AdvancePlan: Phase = %q, want %q", l.Phase, PhasePlan)
	}
	if err := l.AdvanceToolCall(); err != nil {
		t.Fatalf("AdvanceToolCall from PhasePlan: unexpected error: %v", err)
	}
	if l.Phase != PhaseToolCall {
		t.Errorf("after AdvanceToolCall: Phase = %q, want %q", l.Phase, PhaseToolCall)
	}
	if err := l.AdvanceReflect(); err != nil {
		t.Fatalf("AdvanceReflect from PhaseToolCall: unexpected error: %v", err)
	}
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
	if l.FinalConviction != 100 {
		t.Errorf("FinalConviction = %d, want 100 (clamped)", l.FinalConviction)
	}
	l2 := NewAgentLoop(3)
	l2.AdvanceFinal(-5)
	if !l2.IsTerminal() {
		t.Error("IsTerminal should be true even with negative conviction")
	}
	if l2.FinalConviction != 0 {
		t.Errorf("FinalConviction = %d, want 0 (clamped)", l2.FinalConviction)
	}
}

func TestAgentLoop_AdvanceFinal_StoresConviction(t *testing.T) {
	l := NewAgentLoop(3)
	if l.FinalConviction != 0 {
		t.Errorf("fresh loop FinalConviction = %d, want 0", l.FinalConviction)
	}
	l.AdvanceFinal(75)
	if l.FinalConviction != 75 {
		t.Errorf("FinalConviction = %d, want 75", l.FinalConviction)
	}
}

// TestAgentLoop_ExhaustedAfterMaxIter verifies the happy-path Round-based
// exhaustion. After Round accumulates to MaxIter via AdvancePlan,
// Exhausted() returns true. Issue #711 #6 (C5 fix).
func TestAgentLoop_ExhaustedAfterMaxIter(t *testing.T) {
	l := NewAgentLoop(2)
	if l.Exhausted() {
		t.Error("fresh loop should not be exhausted (Round=0 < MaxIter=2)")
	}
	l.AdvancePlan([]PlanStep{{Kind: "tool"}})
	if l.Exhausted() {
		t.Error("loop should not be exhausted after Round=1 (MaxIter=2)")
	}
	l.AdvancePlan([]PlanStep{{Kind: "tool"}})
	if !l.Exhausted() {
		t.Errorf("loop should be exhausted after Round=2 reaches MaxIter=2 (got round=%d, maxIter=%d)", l.Round, l.MaxIter)
	}
}

// TestAgentLoop_Exhausted_BasedOnRoundsNotSteps is the plan-v2 test bar.
// Verifies the C5 fix core property: Exhausted() measures Round
// (cumulative plan-step count), not Steps or the number of AdvancePlan calls.
// A single AdvancePlan with 2 steps in one call must trigger Exhausted()
// when MaxIter=2.
func TestAgentLoop_Exhausted_BasedOnRoundsNotSteps(t *testing.T) {
	l := NewAgentLoop(2)
	l.AdvancePlan([]PlanStep{{Kind: "tool"}, {Kind: "tool"}})
	if !l.Exhausted() {
		t.Errorf("Exhausted() should be true after Round=2 reaches MaxIter=2; "+
			"got round=%d, steps=%d, maxIter=%d", l.Round, len(l.Steps), l.MaxIter)
	}
	if l.Round != 2 {
		t.Errorf("Round = %d, want 2 (incremented by len(steps) in single call, NOT +1)", l.Round)
	}
	if len(l.Steps) != 2 {
		t.Errorf("Steps = %d, want 2", len(l.Steps))
	}
}

// TestAgentLoop_AdvancePlan_IncrementsRoundByLenSteps verifies the C5 fix
// directly: each AdvancePlan call advances Round by len(steps), not +1.
func TestAgentLoop_AdvancePlan_IncrementsRoundByLenSteps(t *testing.T) {
	l := NewAgentLoop(10)
	l.AdvancePlan([]PlanStep{{Kind: "tool"}}) // +1
	if l.Round != 1 {
		t.Errorf("after 1-step plan: Round = %d, want 1", l.Round)
	}
	l.AdvancePlan([]PlanStep{{Kind: "tool"}, {Kind: "tool"}, {Kind: "tool"}}) // +3
	if l.Round != 4 {
		t.Errorf("after 1+3-step plans: Round = %d, want 4 (cumulative, not +1 per call)", l.Round)
	}
	l.AdvancePlan([]PlanStep{}) // +0
	if l.Round != 4 {
		t.Errorf("after empty plan: Round = %d, want 4 (unchanged by len=0)", l.Round)
	}
}

// TestAgentLoop_AdvanceToolCall_PhaseMismatch_ReturnsError is the plan-v2
// test bar. Verifies the Issue #711 #5 fix: AdvanceToolCall returns an
// error if called from a non-PhasePlan state instead of silently no-op'ing.
// On error, Phase must remain unchanged so callers can recover.
func TestAgentLoop_AdvanceToolCall_PhaseMismatch_ReturnsError(t *testing.T) {
	// From PhaseInitial (fresh loop)
	l := NewAgentLoop(3)
	if err := l.AdvanceToolCall(); err == nil {
		t.Error("AdvanceToolCall from PhaseInitial should return error, got nil")
	}
	if l.Phase != PhaseInitial {
		t.Errorf("failed AdvanceToolCall should not mutate Phase, got %q (want %q)", l.Phase, PhaseInitial)
	}
	// From PhaseToolCall (already advanced)
	l2 := NewAgentLoop(3)
	l2.AdvancePlan([]PlanStep{{Kind: "tool"}})
	if err := l2.AdvanceToolCall(); err != nil {
		t.Fatalf("first AdvanceToolCall from PhasePlan: unexpected error: %v", err)
	}
	if err := l2.AdvanceToolCall(); err == nil {
		t.Error("AdvanceToolCall from PhaseToolCall should return error, got nil")
	}
	// From PhasePlan with empty Steps (edge case)
	l3 := NewAgentLoop(3)
	l3.Phase = PhasePlan
	if err := l3.AdvanceToolCall(); err == nil {
		t.Error("AdvanceToolCall from PhasePlan with no steps should return error, got nil")
	}
}

// TestAgentLoop_AdvanceReflect_PhaseMismatch_ReturnsError is the companion
// to the plan-v2 test bar. Same Issue #711 #5 fix applies to AdvanceReflect.
func TestAgentLoop_AdvanceReflect_PhaseMismatch_ReturnsError(t *testing.T) {
	// From PhaseInitial
	l := NewAgentLoop(3)
	if err := l.AdvanceReflect(); err == nil {
		t.Error("AdvanceReflect from PhaseInitial should return error, got nil")
	}
	if l.Phase != PhaseInitial {
		t.Errorf("failed AdvanceReflect should not mutate Phase, got %q (want %q)", l.Phase, PhaseInitial)
	}
	// From PhasePlan
	l2 := NewAgentLoop(3)
	l2.AdvancePlan([]PlanStep{{Kind: "tool"}})
	if err := l2.AdvanceReflect(); err == nil {
		t.Error("AdvanceReflect from PhasePlan should return error, got nil")
	}
	// From PhaseReflect (already advanced)
	l3 := NewAgentLoop(3)
	l3.AdvancePlan([]PlanStep{{Kind: "tool"}})
	if err := l3.AdvanceToolCall(); err != nil {
		t.Fatalf("setup: AdvanceToolCall: %v", err)
	}
	if err := l3.AdvanceReflect(); err != nil {
		t.Fatalf("setup: AdvanceReflect: %v", err)
	}
	if err := l3.AdvanceReflect(); err == nil {
		t.Error("AdvanceReflect from PhaseReflect should return error, got nil")
	}
}

// TestAgentLoop_NewAgentLoop_NonPositiveMaxIter_Warns verifies Issue #711
// #8: NewAgentLoop with maxIter <= 0 logs a warning and returns a loop
// with the default MaxIter=3. Positive values are unchanged.
func TestAgentLoop_NewAgentLoop_NonPositiveMaxIter_Warns(t *testing.T) {
	// maxIter=0 → default 3 (warning fires)
	l0 := NewAgentLoop(0)
	if l0.MaxIter != 3 {
		t.Errorf("NewAgentLoop(0).MaxIter = %d, want 3 (default)", l0.MaxIter)
	}
	// maxIter=-5 → default 3 (warning fires)
	lNeg := NewAgentLoop(-5)
	if lNeg.MaxIter != 3 {
		t.Errorf("NewAgentLoop(-5).MaxIter = %d, want 3 (default)", lNeg.MaxIter)
	}
	// Positive value: unchanged (no warning)
	lPos := NewAgentLoop(7)
	if lPos.MaxIter != 7 {
		t.Errorf("NewAgentLoop(7).MaxIter = %d, want 7 (positive values pass through)", lPos.MaxIter)
	}
	// All three should be in PhaseInitial regardless of maxIter
	for name, l := range map[string]*AgentLoop{"zero": l0, "neg": lNeg, "pos": lPos} {
		if l.Phase != PhaseInitial {
			t.Errorf("%s: Phase = %q, want %q", name, l.Phase, PhaseInitial)
		}
	}
}

// TestAgentLoop_AdvanceFinal_ClampsConviction_Warns verifies Issue #711
// #7: AdvanceFinal logs a warning when clamping conviction to [0,100].
// Behavior preservation: clamped value is stored and Phase is Final.
// (The warning is informational; this test does not assert on log output.)
func TestAgentLoop_AdvanceFinal_ClampsConviction_Warns(t *testing.T) {
	// conviction > 100 → clamp to 100 (warning fires)
	over := NewAgentLoop(3)
	over.AdvanceFinal(150)
	if over.FinalConviction != 100 {
		t.Errorf("AdvanceFinal(150).FinalConviction = %d, want 100 (clamped)", over.FinalConviction)
	}
	if !over.IsTerminal() {
		t.Error("AdvanceFinal(150): IsTerminal should be true")
	}
	// conviction < 0 → clamp to 0 (warning fires)
	under := NewAgentLoop(3)
	under.AdvanceFinal(-5)
	if under.FinalConviction != 0 {
		t.Errorf("AdvanceFinal(-5).FinalConviction = %d, want 0 (clamped)", under.FinalConviction)
	}
	if !under.IsTerminal() {
		t.Error("AdvanceFinal(-5): IsTerminal should be true")
	}
	// In-range conviction: unchanged (no warning)
	in := NewAgentLoop(3)
	in.AdvanceFinal(75)
	if in.FinalConviction != 75 {
		t.Errorf("AdvanceFinal(75).FinalConviction = %d, want 75 (no clamping)", in.FinalConviction)
	}
}
