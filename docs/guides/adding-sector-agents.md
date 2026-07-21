# Semiconductor Executor — Concrete Sector Agent Example

> **Audience**: developers adding new sector agents to the orchestrator.
> **Related**: [`../specs/agent-loop-state-machine.md`](../specs/agent-loop-state-machine-spec.md), [`../specs/llm-sector-agent.md`](../specs/llm-sector-agent-spec.md)

## Two Implementations

The `semiconductor` sector has **two coexisting agent implementations**:

| Implementation | File | When Active |
|----------------|------|-------------|
| `SemiconductorExecutor` (deterministic) | `internal/orchestrator/plugin_sector.go` | Always (production default) |
| `SemiconductorLLMAgent` (LLM-driven) | `internal/orchestrator/semiconductor_llm_agent.go` | When `UseLLMSectorAgents=true` (L2.3 PoC) |

Both are registered in `builtinAgentExecutors()` (see `internal/orchestrator/loader.go`).

## Skill Constant

```go
// internal/orchestrator/semiconductor_llm_agent.go
const SemiconductorLLMAgentSkill = "semiconductor_desk"
```

This is the `domain.AgentSpec.Skill` value that both executors match. Adding a new sector agent? Define a similar constant.

## Adding a New Sector Agent (Deterministic)

Use `SemiconductorExecutor` as a template. Pattern:

1. **Create the struct + `Supports` + `Recommend` + `EvaluatePosition`** in `plugin_sector.go`:

   ```go
   type MySectorExecutor struct{}

   func (MySectorExecutor) Supports(agent domain.AgentSpec) bool {
       return agent.Skill == "my_sector_desk"
   }

   func (MySectorExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime, fq FactorQuery) (domain.Recommendation, bool) {
       // build conviction via factors + prompt heuristics
       b := newConvictionBuilder(...)
       // ... add adjustments ...
       return b.toRecommendation(agent, quote), true
   }

   func (MySectorExecutor) EvaluatePosition(...) (domain.Recommendation, bool) {
       // existing-position evaluation
   }
   ```

2. **Add to `builtinAgentExecutors()`** in `loader.go`.

3. **Implement `StrategyMeta()`** so observability/dashboards see consistent metadata.

4. **Test**: add to `internal/orchestrator/plugin_sector_test.go`.

## Adding a New Sector Agent (LLM-Driven)

Use `SemiconductorLLMAgent` as a template. Pattern:

1. **Create the struct** in a new file (e.g. `my_sector_llm_agent.go`):

   ```go
   type MySectorLLMAgent struct {
       LLMDriver
       Tools []llm.Tool
       MaxIter int
       UseLLMOverride *bool
   }

   const MySectorLLMAgentSkill = "my_sector_desk"
   ```

2. **Implement `Supports()`** with the gate pattern:

   ```go
   func (a MySectorLLMAgent) Supports(agent domain.AgentSpec) bool {
       if agent.Skill != MySectorLLMAgentSkill {
           return false
       }
       if a.UseLLMOverride != nil {
           return *a.UseLLMOverride
       }
       return config.GetUseMySectorLLMAgents()  // separate flag
   }
   ```

3. **Implement `Recommend()`** by driving a `SectorAgentLLM` loop (copy from `SemiconductorLLMAgent`).

4. **Implement `EvaluatePosition()`** (currently `(zero, false)` — out of scope for L2.3 PoC).

5. **Add to `builtinAgentExecutors()`** alongside the deterministic version.

6. **Add parameter metadata** (`UseMySectorLLMAgents` to `OrchestratorParameters` + `GetUseMySectorLLMAgents()`).

7. **Test** with `MockLLMDriver` + `TestTools()`.

## Tool Registry (Shared)

The 3 L2.3 PoC test tools are in `internal/llm/test_tools.go`:

- `get_factor_weight` → factor weights (mock data)
- `get_regime` → market regime classification (mock data)
- `get_liquidity` → liquidity metrics (mock data)

Each takes `{"symbol": "<ticker>"}` and returns hardcoded mock JSON. Production tools would replace these with real implementations (e.g., factor weights from `FactorEngine`, regime from `RegimeExecutor`, liquidity from market data providers).

## MockLLMDriver (Test Helper)

`internal/orchestrator/sector_agent_llm_test_helpers.go` (file suffix `_test_helpers.go` → test-only compilation):

```go
mock := NewMockLLMDriver().
    WithPlanResponse([]PlanStep{
        {Kind: "tool", ToolName: "get_factor_weight", Args: map[string]any{"symbol": "2330"}},
    }).
    WithReflectResponse(Reflection{
        Continue:        false,
        FinalConviction: 75,
        Reasoning:       "Factors look strong; momentum confirmed.",
    })

agent := &SemiconductorLLMAgent{
    LLMDriver:     mock,
    Tools:         llm.TestTools(),
    MaxIter:       3,
    UseLLMOverride: ptr(true),  // bypass flag for tests
}
```

`MockLLMDriver` records call history:
- `PlanCallCount()`, `LastPlanCall()` — for assertions
- `ReflectCallCount()`, `LastReflectCall()` — for assertions
- `WithPlanError(err)`, `WithReflectError(err)` — for error-path tests

## Known Limitations (L2.3 PoC Scope)

1. `EvaluatePosition` is out of scope (returns `(zero, false)`).
2. No multi-iteration test coverage — covered indirectly by `AgentLoop` tests.

## References

- Deterministic: `SemiconductorExecutor` in `internal/orchestrator/plugin_sector.go`
- LLM-driven: `SemiconductorLLMAgent` in `internal/orchestrator/semiconductor_llm_agent.go`
- Test helper: `MockLLMDriver` in `internal/orchestrator/sector_agent_llm_test_helpers.go`
- State machine: [`../specs/agent-loop-state-machine.md`](../specs/agent-loop-state-machine-spec.md)
- L2.3 design: [`../specs/llm-sector-agent.md`](../specs/llm-sector-agent-spec.md)
- RunToolCall wiring (L3): [PR #739](https://github.com/kaecer68/atlas-go/pull/739) (`SectorAgentLLM.RunToolCall` → `llm.SafeInvokeHandler`)
