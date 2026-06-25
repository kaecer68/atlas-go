package orchestrator

import (
	"context"
	"fmt"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// SectorAgentLLM is the template for an LLM-driven sector agent that
// uses the plan → tool_call → reflect loop. It embeds AgentLoop and
// satisfies PlanReflectRunner so a loop driver can drive it.
//
// The concrete LLM call is OUT OF SCOPE for this skeleton. Wire to
// the llm.Router via dependency injection; the runner methods below
// return ErrNotImplemented until a real LLM backend is plugged in.
//
// Enable via the feature flag `UseLLMSectorAgents` in production;
// the default (deterministic) SectorAgent path remains active so
// backtests are reproducible during the observation window.
type SectorAgentLLM struct {
	*AgentLoop
	// Skill is the agent layer / skill this represents (e.g. "semiconductor").
	Skill string
	// LLM is the dependency that makes the actual LLM call. Nil = stub.
	LLM LLMDriver
	// ConvictionFloor is the minimum conviction to emit a final rec.
	ConvictionFloor int
}

// LLMDriver is the minimal contract an LLM backend must satisfy to
// drive a SectorAgentLLM through its loop.
type LLMDriver interface {
	// Complete sends a planning prompt to the LLM and returns the
	// parsed list of plan steps.
	PlanComplete(ctx context.Context, skill, symbol string) ([]PlanStep, error)
	// ReflectComplete sends a reflection prompt after a tool result.
	ReflectComplete(ctx context.Context, skill, symbol, toolResult string) (Reflection, error)
}

// ErrNotImplemented is returned by SectorAgentLLM runner methods when
// no LLMDriver is wired. This is expected during the L2.4 observation
// window — the state machine and integration with DarwinianWeight
// can be validated without a live LLM.
var ErrNotImplemented = fmt.Errorf("sector agent LLM not implemented")

// PlanStep satisfies PlanReflectRunner. It returns ErrNotImplemented
// when no LLM driver is configured; production wiring must replace
// this with a real PlanComplete call.
func (a *SectorAgentLLM) PlanStep(ctx context.Context, _ string, _ domain.Recommendation) ([]PlanStep, error) {
	if a.LLM == nil {
		return nil, ErrNotImplemented
	}
	steps, err := a.LLM.PlanComplete(ctx, a.Skill, "_")
	return steps, err
}

// RunToolCall is a no-op stub. Production wiring must dispatch to a
// tool registry (see internal/llm.Tool for the function-calling
// contract introduced in L2.3).
func (a *SectorAgentLLM) RunToolCall(_ context.Context, step PlanStep) (string, error) {
	return fmt.Sprintf("stub result for %s", step.ToolName), nil
}

// Reflect satisfies PlanReflectRunner. Same caveat as PlanStep.
func (a *SectorAgentLLM) Reflect(ctx context.Context, toolResult string, _ int) (Reflection, error) {
	if a.LLM == nil {
		return Reflection{}, ErrNotImplemented
	}
	return a.LLM.ReflectComplete(ctx, a.Skill, "_", toolResult)
}
