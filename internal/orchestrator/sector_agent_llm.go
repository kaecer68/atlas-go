package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/llm"
)

// toolNames mirrors internal/llm.toolNames (which is package-private).
func toolNames(tools []llm.Tool) []string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return names
}

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
//
// Issue #711 #10: the LLM dependency is split into two embedded
// interfaces — PlanDriver and ReflectDriver — so an implementation
// can supply just the planning half, just the reflection half, or
// both (embed both to cover the combined case).
type SectorAgentLLM struct {
	*AgentLoop
	// Skill is the agent layer / skill this represents (e.g. "semiconductor").
	Skill string
	// PlanDriver is the LLM backend used for the Plan phase. Nil = stub.
	// Embedded interface: methods are promoted onto the struct.
	PlanDriver
	// ReflectDriver is the LLM backend used for the Reflect phase. Nil = stub.
	// Embedded interface: methods are promoted onto the struct.
	ReflectDriver
	// Tools is the registry of tools the agent may invoke during plan →
	// tool_call → reflect. When either driver is non-nil but Tools is empty,
	// RunToolCall panics to prevent silent stub data being fed back to the
	// LLM. See internal/llm.SafeInvokeHandler for the recommended call pattern.
	Tools []llm.Tool
}

// PlanDriver is the contract an LLM backend must satisfy to drive
// the Plan phase of a SectorAgentLLM.
//
// Issue #711 #10: split from the original LLMDriver interface so an
// implementation can supply just the planning half.
type PlanDriver interface {
	// PlanComplete sends a planning prompt to the LLM and returns the
	// parsed list of plan steps.
	PlanComplete(ctx context.Context, skill, symbol string) ([]PlanStep, error)
}

// ReflectDriver is the contract an LLM backend must satisfy to drive
// the Reflect phase of a SectorAgentLLM.
//
// Issue #711 #10: split from the original LLMDriver interface so an
// implementation can supply just the reflection half.
type ReflectDriver interface {
	// ReflectComplete sends a reflection prompt after a tool result.
	ReflectComplete(ctx context.Context, skill, symbol, toolResult string) (Reflection, error)
}

// ErrNotImplemented is returned by SectorAgentLLM runner methods when
// no LLMDriver is wired. This is expected during the L2.4 observation
// window — the state machine and integration with DarwinianWeight
// can be validated without a live LLM.
var ErrNotImplemented = fmt.Errorf("sector agent LLM not implemented")

// PlanStep satisfies PlanReflectRunner. It returns ErrNotImplemented
// when no PlanDriver is configured; production wiring must replace
// this with a real PlanComplete call. The symbol argument is forwarded
// to the LLM driver so per-symbol context is preserved.
//
// The method call uses the embedded-interface promoted form
// (a.PlanComplete) per staticcheck QF1008. The nil check above
// guarantees the embedded PlanDriver is non-nil before promotion
// resolves the call.
func (a *SectorAgentLLM) PlanStep(ctx context.Context, symbol string, _ domain.Recommendation) ([]PlanStep, error) {
	if a.PlanDriver == nil {
		return nil, ErrNotImplemented
	}
	return a.PlanComplete(ctx, a.Skill, symbol)
}

// RunToolCall satisfies PlanReflectRunner.
//
// PlanDriver == nil AND ReflectDriver == nil → returns ErrNotImplemented
//
//	(deterministic stub path, preserves test-time behavior
//	 of the unconfigured agent).
//
// (PlanDriver != nil OR ReflectDriver != nil) AND Tools empty → panics.
//
//	A wired LLM with no tool registry would silently feed synthetic
//	stub results back to the LLM, corrupting the plan→reflect loop.
//	Issue #711 #4.
//
// (PlanDriver != nil OR ReflectDriver != nil) AND Tools present →
//
//	dispatched to the matching tool via llm.SafeInvokeHandler, which
//	also recovers from panicking handlers (Issue #711 #3).
//	Lookup is linear over a.Tools (expected <10 entries per skill);
//	an unknown tool name produces a clear error that lists the
//	registered tools to help diagnose LLM hallucination.
func (a *SectorAgentLLM) RunToolCall(ctx context.Context, step PlanStep) (string, error) {
	if a.PlanDriver == nil && a.ReflectDriver == nil {
		return "", ErrNotImplemented
	}
	if len(a.Tools) == 0 {
		panic(fmt.Sprintf(
			"sector_agent_llm.RunToolCall: LLM driver(s) wired but Tools registry empty (skill=%q, step.ToolName=%q); "+
				"this would feed stub data back to the LLM. Wire Tools via SetTools() or set both drivers to nil to use the stub path",
			a.Skill, step.ToolName,
		))
	}

	// Linear search; slice is small (<10 per skill) and this avoids
	// exposing a mutable registry that callers could race on.
	var tool *llm.Tool
	for i := range a.Tools {
		if a.Tools[i].Name == step.ToolName {
			tool = &a.Tools[i]
			break
		}
	}
	if tool == nil {
		return "", fmt.Errorf(
			"sector_agent_llm.RunToolCall: unknown tool %q for skill=%q (registered: %v)",
			step.ToolName, a.Skill, toolNames(a.Tools),
		)
	}

	// Marshal Args to the json.RawMessage contract that SafeInvokeHandler expects.
	rawArgs, err := json.Marshal(step.Args)
	if err != nil {
		return "", fmt.Errorf(
			"sector_agent_llm.RunToolCall: marshal args for tool %q (skill=%q): %w",
			step.ToolName, a.Skill, err,
		)
	}

	result, err := llm.SafeInvokeHandler(ctx, tool, rawArgs)
	if err != nil {
		return "", fmt.Errorf(
			"sector_agent_llm.RunToolCall: tool %q (skill=%q): %w",
			step.ToolName, a.Skill, err,
		)
	}
	return string(result), nil
}

// Reflect satisfies PlanReflectRunner. Same caveat as PlanStep.
// The symbol argument is forwarded so the LLM driver receives the
// per-symbol context for symbol-aware reflection.
//
// Uses the embedded-interface promoted form (a.ReflectComplete) per
// staticcheck QF1008; nil check above guarantees ReflectDriver is
// non-nil before promotion resolves.
func (a *SectorAgentLLM) Reflect(ctx context.Context, symbol string, toolResult string, _ int) (Reflection, error) {
	if a.ReflectDriver == nil {
		return Reflection{}, ErrNotImplemented
	}
	return a.ReflectComplete(ctx, a.Skill, symbol, toolResult)
}
