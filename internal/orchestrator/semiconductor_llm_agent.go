package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/llm"
)

// SemiconductorLLMAgent is the LLM-driven PoC for the semiconductor
// sector. It is gated behind the UseLLMSectorAgents feature flag
// (default off) so the deterministic SemiconductorExecutor remains
// the production default until the L2.4 observation window validates
// the LLM-driven behavior.
//
// Gate mechanism (deviation from plan v2 C1's swap design):
//   - Supports() returns true only when both the spec.Skill matches
//     "semiconductor_desk" AND config.GetUseLLMSectorAgents() is true.
//   - When the flag is off, Supports() returns false and the
//     deterministic SemiconductorExecutor (always in the registry)
//     handles the spec.
//   - This avoids mutating the executor list at runtime and keeps
//     both implementations coexistable.
//
// File: internal/orchestrator/semiconductor_llm_agent.go
// Plan: PR5b of 7 in the Wave 10 L2.3 execution plan.
type SemiconductorLLMAgent struct {
	// PlanDriver is the LLM backend for the Plan phase of the
	// agent loop. May be nil — in that case Recommend returns false
	// (the agent is not actually wired).
	PlanDriver PlanDriver
	// ReflectDriver is the LLM backend for the Reflect phase. Same
	// nil semantics as PlanDriver.
	ReflectDriver ReflectDriver
	// Tools is the registry of tools the agent may invoke during the
	// plan/reflect loop. If empty and LLM is non-nil, RunToolCall
	// (in SectorAgentLLM) panics — see Issue #711 #4.
	Tools []llm.Tool
	// MaxIter is the AgentLoop's max iteration budget. Defaults to 3
	// when zero.
	MaxIter int
	// UseLLMOverride, if non-nil, overrides config.GetUseLLMSectorAgents()
	// for Supports(). Production code leaves it nil; tests set it
	// directly to avoid mutating global config state.
	UseLLMOverride *bool
	// Metrics is the logger used for L2.4 observation events. If nil,
	// slog.Default() is used. Tests inject a JSON-handler-backed logger
	// to assert emitted events without touching global state.
	Metrics *slog.Logger
}

// metricsLogger returns the agent's Metrics logger, falling back to slog.Default().
func (a SemiconductorLLMAgent) metricsLogger() *slog.Logger {
	if a.Metrics != nil {
		return a.Metrics
	}
	return slog.Default()
}

// SemiconductorLLMAgentSkill is the domain.Skill value for
// semiconductor agents. Must match SemiconductorExecutor.Supports.
const SemiconductorLLMAgentSkill = "semiconductor_desk"

// Supports satisfies AgentExecutor. Returns true when both:
//   - the spec.Skill matches the semiconductor desk skill, AND
//   - the UseLLMSectorAgents flag is enabled (config or override).
//
// When the flag is off, this returns false and the deterministic
// SemiconductorExecutor (also in the registry) handles the spec.
func (a SemiconductorLLMAgent) Supports(agent domain.AgentSpec) bool {
	if agent.Skill != SemiconductorLLMAgentSkill {
		return false
	}
	if a.UseLLMOverride != nil {
		return *a.UseLLMOverride
	}
	return config.GetUseLLMSectorAgents()
}

// Recommend satisfies AgentExecutor. Drives the plan/reflect loop
// via the embedded LLM drivers and converts the final Reflection
// into a domain.Recommendation.
//
// Simplified flow for the L2.3 PoC:
//  1. Build a SectorAgentLLM with the agent's LLM + tools.
//  2. Loop: PlanStep -> RunToolCall -> Reflect, until Continue=false
//     or MaxIter is reached.
//  3. Return domain.Recommendation built from the final reflection.
//
// Returns (recommendation, true) on success; (zero, false) if the
// LLM is not wired (UseLLMOverride=false or flag off) or the loop
// fails.
//
// Issue #740 observation metrics: emits slog.Info events
// (start / plan / tool / reflect / end) with field names aligned
// to the acceptance spec in kaecer68/atlas-go#740.
func (a SemiconductorLLMAgent) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime, fq FactorQuery) (domain.Recommendation, bool) {
	if !a.Supports(agent) {
		return domain.Recommendation{}, false
	}
	if a.PlanDriver == nil || a.ReflectDriver == nil {
		return domain.Recommendation{}, false
	}

	maxIter := a.MaxIter
	if maxIter <= 0 {
		maxIter = 3
	}
	runner := &SectorAgentLLM{
		AgentLoop:     NewAgentLoop(maxIter),
		PlanDriver:    a.PlanDriver,
		ReflectDriver: a.ReflectDriver,
		Tools:         a.Tools,
		Skill:         agent.Skill,
	}

	// Build the base recommendation shell that we'll mutate as the
	// loop produces output. AgentID comes from agent.ID (the
	// domain.AgentSpec doesn't have a Symbol/Agent field — the
	// symbol comes from the quote).
	rec := domain.Recommendation{
		Agent:  agent.ID,
		Skill:  agent.Skill,
		Symbol: quote.Symbol,
		Side:   domain.SideBuy,
	}

	a.metricsLogger().Info(
		"agent_loop.start",
		"symbol", quote.Symbol,
		"skill", agent.Skill,
	)

	var loopExhausted bool

	// Issue #740: emit end via defer so it fires on every exit path.
	defer func() {
		a.metricsLogger().Info(
			"agent_loop.end",
			"symbol", quote.Symbol,
			"conviction", rec.Conviction,
			"exhausted", loopExhausted,
		)
	}()

	var lastReflection Reflection
	toolResult := ""
	for i := 0; i < maxIter; i++ {
		// Plan phase: use SectorAgentLLM.PlanStep (the LLM-backed
		// method that satisfies PlanReflectRunner.PlanStep)
		planStart := time.Now()
		steps, err := runner.PlanStep(context.Background(), quote.Symbol, rec)
		// Issue #740: emit plan metrics BEFORE the err guard so
		// aggregators see failed plans too.
		a.metricsLogger().Info(
			"agent_loop.plan",
			"size", len(steps),
			"latency_ms", time.Since(planStart).Milliseconds(),
			"err", err,
		)
		if err != nil {
			rec.Reason = fmt.Sprintf("LLM plan failed: %v", err)
			return rec, false
		}
		runner.AdvancePlan(steps)

		// Tool-call phase: concatenate all tool results into the
		// toolResult string fed to the next reflect prompt.
		toolResult = ""
		for j := range steps {
			if err := runner.AdvanceToolCall(); err != nil {
				rec.Reason = fmt.Sprintf("AdvanceToolCall[%d]: %v", j, err)
				return rec, false
			}
			toolStart := time.Now()
			result, err := runner.RunToolCall(context.Background(), steps[j])
			toolLatencyMs := time.Since(toolStart).Milliseconds()
			// Issue #740: emit tool metrics BEFORE the err guard so
			// aggregators see failed tool calls too.
			a.metricsLogger().Info(
				"agent_loop.tool",
				"name", steps[j].ToolName,
				"success", err == nil,
				"latency_ms", toolLatencyMs,
			)
			if err != nil {
				rec.Reason = fmt.Sprintf("RunToolCall[%d]: %v", j, err)
				return rec, false
			}
			toolResult += result + "\n"
		}

		// Reflect phase
		if err := runner.AdvanceReflect(); err != nil {
			rec.Reason = fmt.Sprintf("AdvanceReflect: %v", err)
			return rec, false
		}
		reflection, err := runner.Reflect(context.Background(), quote.Symbol, toolResult, 0)
		if err != nil {
			rec.Reason = fmt.Sprintf("Reflect: %v", err)
			return rec, false
		}
		lastReflection = reflection
		a.metricsLogger().Info(
			"agent_loop.reflect",
			"continue", reflection.Continue,
			"conviction", reflection.FinalConviction,
		)

		if !reflection.Continue {
			break
		}
		if i == maxIter-1 {
			loopExhausted = true
		}
	}

	// Map final reflection to recommendation
	rec.Conviction = lastReflection.FinalConviction
	rec.Reason = lastReflection.Reasoning
	return rec, true
}

// EvaluatePosition satisfies AgentExecutor. For the L2.3 PoC we
// return (zero, false) — position evaluation is not part of the
// scope. Future PRs can extend this to mirror the LLM logic.
func (a SemiconductorLLMAgent) EvaluatePosition(pos domain.Position, quote domain.Quote, agent domain.AgentSpec, prompt string, regime domain.Regime, fq FactorQuery) (domain.Recommendation, bool) {
	return domain.Recommendation{}, false
}

// StrategyMeta satisfies StrategyProvider. Mirrors the deterministic
// SemiconductorExecutor's metadata so observability/dashboards see
// the same shape regardless of which implementation runs.
func (a SemiconductorLLMAgent) StrategyMeta() StrategyMeta {
	fc := loadFactorConfig()
	return StrategyMeta{
		ID:          "semiconductor_llm",
		Skill:       SemiconductorLLMAgentSkill,
		Description: "LLM-driven semiconductor supply-chain leadership and capex cycle detector (L2.3 PoC)",
		Factors:     []string{"momentum", "liquidity"},
		Parameters:  append(momentumParams(fc), liquidityParams(fc)...),
	}
}
