# L2.3 Plan/Reflect — Sector Agent LLM Integration

> **Status**: Design landed; implementation in PR5a + PR5b. Tagged `v0.0.0.21`.
> **Plan**: [Issue #711 (Wave 10 L2.3 plan)](https://github.com/kaecer68/atlas-go/issues/711) (PR5a/PR5b of 7)
> **Issues**: [#711](https://github.com/kaecer68/atlas-go/issues/711) (11 design concerns from gstack /review of PR #703)

## Overview

L2.3 introduces an **LLM-driven sector agent** that drives a plan → tool_call → reflect loop to produce recommendations. This complements the existing **deterministic sector agents** (e.g. `SemiconductorExecutor`).

The two implementations coexist via a **gate mechanism**:

- Deterministic `SemiconductorExecutor` is the production default.
- LLM-driven `SemiconductorLLMAgent` is gated behind the `UseLLMSectorAgents` feature flag (default **off**).
- Both are always in the registry; `Supports()` resolves which one runs per spec.

## Architecture

```
                            ┌──────────────────────────┐
                            │  AgentRegistry           │
                            │  (builtinAgentExecutors)│
                            └────────────┬─────────────┘
                                         │
              ┌──────────────────────────┴──────────────────────────┐
              │                                                      │
   ┌──────────▼────────────┐                            ┌─────────────▼────────────┐
   │ SemiconductorExecutor │                            │ SemiconductorLLMAgent    │
   │ (deterministic)      │                            │ (LLM-driven, L2.3 PoC)    │
   │ Supports() always     │                            │ Supports() gated by flag  │
   │ true for "semicon_    │                            │   Supports → "semicon_    │
   │ ductor_desk"          │                            │   ductor_desk" AND        │
   │                       │                            │   UseLLMSectorAgents      │
   │ Builds conviction via │                            │                           │
   │ domain factors +       │                            │ Drives plan/reflect loop  │
   │ prompt heuristics     │                            │ via SectorAgentLLM        │
   └───────────────────────┘                            └────────────┬──────────────┘
                                                                     │
                                                              ┌──────▼──────┐
                                                              │ SectorAgent │
                                                              │    LLM      │
                                                              │             │
                                                              │ PlanStep    │
                                                              │ RunToolCall │
                                                              │ Reflect     │
                                                              └──────┬──────┘
                                                                     │
                                                              ┌──────────▼──────────┐
                                                              │ DriverAdapter       │
                                                              │ (internal/orchestor)│
                                                              │                     │
                                                              │ Prompt:             │
                                                              │   prompts.PlanPrompt │
                                                              │   prompts.Reflect... │
                                                              │                     │
                                                              │ Parse:              │
                                                              │   ParsePlanResponse │
                                                              │   ParseReflectResp  │
                                                              └──────────┬──────────┘
                                                                         │
                                                              ┌──────────▼──────────┐
                                                              │ llm.ProviderImpl    │
                                                              │ (DeepSeek/MiniMax/  │
                                                              │  Kimi K2.7)         │
                                                              └─────────────────────┘
```

## Components

| Component | File | Purpose |
|-----------|------|---------|
| `SectorAgentLLM` | `internal/orchestrator/sector_agent_llm.go` | State machine wrapper (plan → tool_call → reflect loop). Embeds `PlanDriver` + `ReflectDriver`. |
| `SemiconductorLLMAgent` | `internal/orchestrator/semiconductor_llm_agent.go` | LLM-driven sector agent (L2.3 PoC). `Supports()` gated by `UseLLMSectorAgents`. |
| `DriverAdapter` | `internal/orchestrator/llm_driver_adapter.go` | Bridges `SectorAgentLLM` to `llm.ProviderImpl`. Builds prompts via `prompts.PlanPrompt` / `prompts.ReflectPrompt`, parses responses. |
| `prompts.PlanPrompt` / `ReflectPrompt` | `internal/llm/prompts/{plan,reflect}.go` | Prompt templates with embedded JSON format specification. |
| `Request.Validate()` | `internal/llm/provider.go` | Validates `ToolChoice` before dispatch (Issue #711 #11). |
| `llm.Tool` + `SafeInvokeHandler` | `internal/llm/provider.go` | Tool handler interface with panic recovery. |
| `MockLLMDriver` | `internal/orchestrator/sector_agent_llm_test_helpers.go` | Test helper. Canned responses, no real LLM. |

## Feature flag: `UseLLMSectorAgents`

- **Default**: `false` (deterministic is the production path)
- **Toggle**: `configs/parameters.json` → `orchestrator.use_llm_sector_agents`
- **CLI**: `--use-llm-sector-agents` (when wired)
- **Source**: `experimental` (will be promoted to `empirical` after L2.4 observation)

```go
// internal/orchestrator/semiconductor_llm_agent.go
func (a SemiconductorLLMAgent) Supports(agent domain.AgentSpec) bool {
    if agent.Skill != SemiconductorLLMAgentSkill {
        return false
    }
    if a.UseLLMOverride != nil {
        return *a.UseLLMOverride  // test override
    }
    return config.GetUseLLMSectorAgents()  // flag check
}
```

## Known Limitations

1. **`RunToolCall` is a PR1 placeholder** — returns "tool dispatch not yet implemented" error. The actual tool dispatch (find tool by name → call `SafeInvokeHandler` → return result) will be wired in a follow-up PR after L2.3 PoC.
2. **Conviction from reflection only** — the LLM agent doesn't currently use domain `FactorScores` or other structured inputs. PR5b's `Recommend()` uses the `Reflection.FinalConviction` directly.
3. **No multi-iteration test coverage** — `TestSemiconductorLLMAgent_Recommend_ToolDispatchGap` tests a single iteration (the tool dispatch fails before reflection). Multi-iteration (continue=true → re-plan) is covered indirectly by `AgentLoop` tests (PR2).

## Observability (deferred to L2.4)

When `UseLLMSectorAgents=true`, the following should be logged:
- `agent_loop.round` (from `AgentLoop.Round`)
- `agent_loop.exhausted` (from `Exhausted()`)
- `llm.latency_ms` (p50, p95)
- `tool.calls_per_recommendation`
- `reflect.continue_rate` (fraction of continues)

See `docs/wave-11/L2_4_OBSERVATION.md` for the full metrics list.

## References

- Plan: [Issue #711 (Wave 10 L2.3 plan)](https://github.com/kaecer68/atlas-go/issues/711)
- Issue: [#711](https://github.com/kaecer68/atlas-go/issues/711)
- PRs: #724 (PR1), #725 (PR2/v0.0.0.20a), #726 (PR3), #729 (PR4), #732 (PR5a), #733 (PR5b/v0.0.0.21)
- Cross-references: `AGENT_LOOP_STATE_MACHINE.md`, `SEMICONDUCTOR_EXECUTOR.md`, `L2_4_OBSERVATION.md`
- Design authority: [`docs/llm-integration-strategy-framework.md`](../llm-integration-strategy-framework.md)
