package orchestrator

import (
	"context"
	"fmt"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/llm"
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
	// Tools is the registry of tools the agent may invoke during plan →
	// tool_call → reflect. When LLM != nil but Tools is empty, RunToolCall
	// panics to prevent silent stub data being fed back to the LLM.
	// See internal/llm.SafeInvokeHandler for the recommended call pattern.
	Tools []llm.Tool
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
// this with a real PlanComplete call. The symbol argument is forwarded
// to the LLM driver so per-symbol context is preserved.
func (a *SectorAgentLLM) PlanStep(ctx context.Context, symbol string, _ domain.Recommendation) ([]PlanStep, error) {
	if a.LLM == nil {
		return nil, ErrNotImplemented
	}
	steps, err := a.LLM.PlanComplete(ctx, a.Skill, symbol)
	return steps, err
}

// RunToolCall satisfies PlanReflectRunner.
//
// LLM == nil  → returns ErrNotImplemented (deterministic stub path,
//
//	preserves test-time behavior of the unconfigured agent).
//
// LLM != nil AND Tools empty → panics. A wired LLM with no tool registry
//
//	would silently feed synthetic stub results back to the LLM,
//	corrupting the plan→reflect loop. Issue #711 #4.
//
// LLM != nil AND Tools present → dispatched to the matching tool via
//
//	SafeInvokeHandler. Full tool-dispatch implementation lives
//	in the L2.3 adapter PR (PR5a); for now this branch returns
//	an explicit "not yet wired" error so callers see a clear
//	signal instead of silent stubs.
func (a *SectorAgentLLM) RunToolCall(_ context.Context, step PlanStep) (string, error) {
	if a.LLM == nil {
		return "", ErrNotImplemented
	}
	if len(a.Tools) == 0 {
		panic(fmt.Sprintf(
			"sector_agent_llm.RunToolCall: LLM wired but Tools registry empty (skill=%q, step.ToolName=%q); "+
				"this would feed stub data back to the LLM. Wire Tools via SetTools() or set a.LLM = nil to use the stub path",
			a.Skill, step.ToolName,
		))
	}
	return "", fmt.Errorf("sector_agent_llm.RunToolCall: tool dispatch not yet implemented for skill=%q tool=%q (PR5a)", a.Skill, step.ToolName)
}

// Reflect satisfies PlanReflectRunner. Same caveat as PlanStep.
// The symbol argument is forwarded so the LLM driver receives the
// per-symbol context for symbol-aware reflection.
func (a *SectorAgentLLM) Reflect(ctx context.Context, symbol string, toolResult string, _ int) (Reflection, error) {
	if a.LLM == nil {
		return Reflection{}, ErrNotImplemented
	}
	return a.LLM.ReflectComplete(ctx, a.Skill, symbol, toolResult)
}
