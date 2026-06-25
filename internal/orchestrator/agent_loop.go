package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// AgentPhase tracks an agent's position in the plan → tool_call → reflect loop.
// A sector agent with an LLM backend progresses through these phases over
// one or more LLM round-trips; deterministic (non-LLM) agents skip the
// loop and emit their final recommendation directly.
type AgentPhase string

const (
	PhaseInitial  AgentPhase = "initial"
	PhasePlan     AgentPhase = "plan"
	PhaseToolCall AgentPhase = "tool_call"
	PhaseReflect  AgentPhase = "reflect"
	PhaseFinal    AgentPhase = "final"
)

// PlanStep is one item in the agent's plan: either a tool invocation or
// a reasoning note. The LLM produces the plan; the loop executes it.
type PlanStep struct {
	Kind     string // "tool" or "thought"
	ToolName string
	Args     map[string]any
	Note     string
}

// Reflection is the post-tool-call reasoning the agent produces to decide
// whether to issue another plan, emit the final recommendation, or abort.
type Reflection struct {
	Continue        bool
	FinalConviction int
	Reasoning       string
}

// AgentLoop is the stateful object that drives a single LLM-driven agent
// invocation through the plan → tool_call → reflect loop.
//
// The zero value is NOT usable; callers should construct via NewAgentLoop
// or use one of the concrete agent types (e.g. SectorAgentLLM) that
// embed an AgentLoop.
type AgentLoop struct {
	Phase           AgentPhase
	Steps           []PlanStep
	MaxIter         int
	FinalConviction int
	// Round is the cumulative count of plan steps recorded via AdvancePlan.
	// Incremented by len(steps) per call (NOT +1), so a single multi-step
	// plan counts as multiple rounds. Exhausted() returns true once
	// Round >= MaxIter. Issue #711 #6 (C5 fix).
	Round int
	// exhaustedWarningOnce fires at most once per loop instance if the
	// legacy len(Steps) >= MaxIter threshold is reached before the new
	// Round-based one. Catches callers that mutate Steps directly without
	// going through AdvancePlan (or pre-PR2 binaries that don't track Round).
	exhaustedWarningOnce sync.Once
}

// NewAgentLoop creates a fresh loop starting in PhaseInitial with the given
// max iteration budget. MaxIter bounds the total number of plan steps
// (counted via Round, not the number of planning rounds); once exhausted
// the loop is forced to PhaseFinal.
//
// Issue #711 #8: when maxIter <= 0, a slog.Warn is logged and the loop
// is created with the default MaxIter=3. Surfaces caller bugs that pass
// zero or negative iteration budgets instead of silently coercing.
func NewAgentLoop(maxIter int) *AgentLoop {
	if maxIter <= 0 {
		slog.Warn("agent_loop.NewAgentLoop: non-positive maxIter, using default",
			"requested", maxIter, "default", 3)
		maxIter = 3
	}
	return &AgentLoop{Phase: PhaseInitial, MaxIter: maxIter}
}

// AdvancePlan records that the agent has produced a plan and transitions
// to PhasePlan. Steps is the planned tool invocations.
//
// Issue #711 #6 (C5 fix): Round is incremented by len(steps), NOT +1.
// A single call with a 3-step plan advances Round by 3, so Exhausted()
// can trigger within one Plan→Reflect cycle if the LLM emits a large plan.
func (l *AgentLoop) AdvancePlan(steps []PlanStep) {
	l.Phase = PhasePlan
	l.Steps = append(l.Steps, steps...)
	l.Round += len(steps)
}

// AdvanceToolCall transitions to PhaseToolCall after the agent has emitted
// a single tool invocation (the first pending step).
//
// Issue #711 #5: returns an error if the loop is not in PhasePlan with
// pending steps. Callers MUST handle the error (no _ = suppression);
// silently skipping a phase transition masks LLM driver bugs that would
// otherwise corrupt the plan→reflect loop. On error, Phase is left
// unchanged so callers can recover.
func (l *AgentLoop) AdvanceToolCall() error {
	if l.Phase != PhasePlan {
		return fmt.Errorf("agent_loop.AdvanceToolCall: expected PhasePlan, got %q (Round=%d, Steps=%d)", l.Phase, l.Round, len(l.Steps))
	}
	if len(l.Steps) == 0 {
		return fmt.Errorf("agent_loop.AdvanceToolCall: no pending steps in PhasePlan (Round=%d)", l.Round)
	}
	l.Phase = PhaseToolCall
	return nil
}

// AdvanceReflect transitions to PhaseReflect after the tool result is in.
//
// Issue #711 #5: returns an error if the loop is not in PhaseToolCall.
// Callers MUST handle the error (no _ = suppression).
func (l *AgentLoop) AdvanceReflect() error {
	if l.Phase != PhaseToolCall {
		return fmt.Errorf("agent_loop.AdvanceReflect: expected PhaseToolCall, got %q", l.Phase)
	}
	l.Phase = PhaseReflect
	return nil
}

// AdvanceFinal transitions to PhaseFinal, locking in the final conviction.
// The clamped value (0..100) is stored in l.FinalConviction so callers can
// read it back after the loop terminates.
//
// Issue #711 #7: when the requested conviction is outside [0,100], a
// slog.Warn is logged with the requested and clamped values. Surfaces
// LLM driver bugs that emit out-of-range convictions instead of silently
// coercing.
func (l *AgentLoop) AdvanceFinal(conviction int) {
	if conviction < 0 {
		slog.Warn("agent_loop.AdvanceFinal: clamping conviction to [0,100]",
			"requested", conviction, "clamped_to", 0)
		conviction = 0
	} else if conviction > 100 {
		slog.Warn("agent_loop.AdvanceFinal: clamping conviction to [0,100]",
			"requested", conviction, "clamped_to", 100)
		conviction = 100
	}
	l.Phase = PhaseFinal
	l.FinalConviction = conviction
}

// Exhausted returns true if the loop has hit MaxIter and should force
// the agent to emit a final recommendation on the next reflect.
//
// Issue #711 #6 (C5 fix): now checks Round >= MaxIter, where Round is
// the cumulative count of plan steps recorded via AdvancePlan. The
// previous implementation checked len(Steps) >= MaxIter, which measured
// the wrong thing if the agent emitted multi-step plans.
//
// If a legacy caller (or a direct mutation of Steps) still triggers
// len(Steps) >= MaxIter before Round >= MaxIter, a one-time slog.Warn
// fires to surface the divergence. Migrate such callers to use Round
// directly.
func (l *AgentLoop) Exhausted() bool {
	if len(l.Steps) >= l.MaxIter && l.Round < l.MaxIter {
		l.exhaustedWarningOnce.Do(func() {
			slog.Warn("agent_loop.Exhausted: legacy len(Steps)>=MaxIter divergence detected; "+
				"Exhausted() now checks Round>=MaxIter per Issue #711 #6",
				"round", l.Round, "steps", len(l.Steps), "max_iter", l.MaxIter)
		})
	}
	return l.Round >= l.MaxIter
}

// IsTerminal returns true if the loop is in PhaseFinal.
func (l *AgentLoop) IsTerminal() bool {
	return l.Phase == PhaseFinal
}

// PlanReflectRunner is the contract for the LLM-driven agent that owns
// an AgentLoop. It produces the plan, runs tool calls, and reflects
// before committing the final recommendation.
//
// This interface is intentionally minimal so concrete agents
// (e.g. SectorAgentLLM) can keep their own state and tool registry
// while exposing a uniform contract for the loop driver.
type PlanReflectRunner interface {
	// PlanStep is called first to ask the LLM what to do.
	PlanStep(ctx context.Context, symbol string, base domain.Recommendation) ([]PlanStep, error)
	// RunToolCall executes one planned tool call and returns its result.
	RunToolCall(ctx context.Context, step PlanStep) (string, error)
	// Reflect is called after each tool result to decide whether to
	// continue planning or commit the final recommendation. The symbol
	// argument carries the per-symbol context that the LLM needs to
	// produce a symbol-aware reflection.
	Reflect(ctx context.Context, symbol string, toolResult string, currentConviction int) (Reflection, error)
}
