package orchestrator

import (
	"context"

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
	Phase   AgentPhase
	Steps   []PlanStep
	MaxIter int
}

// NewAgentLoop creates a fresh loop starting in PhaseInitial with the given
// max iteration budget. MaxIter bounds the total number of plan → reflect
// rounds; once exhausted the loop is forced to PhaseFinal.
func NewAgentLoop(maxIter int) *AgentLoop {
	if maxIter <= 0 {
		maxIter = 3
	}
	return &AgentLoop{Phase: PhaseInitial, MaxIter: maxIter}
}

// AdvancePlan records that the agent has produced a plan and transitions
// to PhasePlan. Steps is the planned tool invocations.
func (l *AgentLoop) AdvancePlan(steps []PlanStep) {
	l.Phase = PhasePlan
	l.Steps = append(l.Steps, steps...)
}

// AdvanceToolCall transitions to PhaseToolCall after the agent has emitted
// a single tool invocation (the first pending step).
func (l *AgentLoop) AdvanceToolCall() {
	if l.Phase == PhasePlan && len(l.Steps) > 0 {
		l.Phase = PhaseToolCall
	}
}

// AdvanceReflect transitions to PhaseReflect after the tool result is in.
func (l *AgentLoop) AdvanceReflect() {
	if l.Phase == PhaseToolCall {
		l.Phase = PhaseReflect
	}
}

// AdvanceFinal transitions to PhaseFinal, locking in the final conviction.
func (l *AgentLoop) AdvanceFinal(conviction int) {
	if conviction < 0 {
		conviction = 0
	}
	if conviction > 100 {
		conviction = 100
	}
	l.Phase = PhaseFinal
}

// Exhausted returns true if the loop has hit MaxIter and should force
// the agent to emit a final recommendation on the next reflect.
func (l *AgentLoop) Exhausted() bool {
	return len(l.Steps) >= l.MaxIter
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
	// continue planning or commit the final recommendation.
	Reflect(ctx context.Context, toolResult string, currentConviction int) (Reflection, error)
}
