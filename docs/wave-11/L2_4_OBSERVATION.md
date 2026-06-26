# L2.4 Observation Window

> ⚠️ **Status: PLANNED — not yet started.**
> `UseLLMSectorAgents` feature flag is default `false` (source=`experimental`) per `configs/parameters.json:5860`.
> Metrics implementation shipped (PR #743) but the **7-14 day observation window has not started**.
> Update this banner to "IN PROGRESS" once staging enablement begins.

> **Predecessor**: [`../specs/llm-sector-agent.md`](../specs/llm-sector-agent.md) (L2.3 PoC design — moved from `docs/wave-11/L2_3_PLAN_REFLECT.md`)
> **Linked log**: [`../archive/wave-10-observation-log.md`](../archive/wave-10-observation-log.md) (existing Wave 10 observations)
> **Runbook**: [`L2_4_RUNBOOK.md`](L2_4_RUNBOOK.md)

## Overview

After L2.3 PoC lands (v0.0.0.21), the `UseLLMSectorAgents` flag can be enabled in a non-production environment for a **7-14 day observation window**. The window validates LLM-driven sector agent behavior against the deterministic baseline and surfaces regressions before production rollout.

## Goals

1. **Validate LLM agent quality**: Are LLM-driven recommendations comparable to deterministic ones (or better)?
2. **Validate LLM agent reliability**: Uptime, error rate, latency distribution.
3. **Validate observability**: Are the logged metrics useful for debugging?
4. **Validate the flag's roll-back mechanism**: Can we disable the LLM agent and fall back to deterministic without disruption?

## Activation

```bash
# Enable the flag in configs/parameters.json
{
  "orchestrator": {
    "use_llm_sector_agents": {
      "value": true
    }
  }
}

# Or via CLI (when wired)
atlas run --use-llm-sector-agents
```

## Metrics

### Per-recommendation metrics

| Metric | Source | Notes |
|--------|--------|-------|
| `agent_loop.round` | `AgentLoop.Round` | Number of plan steps attempted. Capped at `MaxIter`. |
| `agent_loop.exhausted` | `Exhausted()` bool | True when the loop hit `MaxIter` before `Continue=false`. |
| `agent_loop.plan_count` | `SectorAgentLLM.PlanStep` calls | Number of LLM plan invocations. |
| `agent_loop.reflect_count` | `SectorAgentLLM.Reflect` calls | Number of LLM reflect invocations. |
| `agent_loop.tool_count` | `SectorAgentLLM.RunToolCall` calls | Number of tool calls. |
| `llm.latency_ms.plan` | Wrapper | Wall-clock for each `PlanComplete` call. |
| `llm.latency_ms.reflect` | Wrapper | Wall-clock for each `ReflectComplete` call. |
| `reflect.continue` | `Reflection.Continue` | True/false per reflect call. |
| `recommendation.conviction` | `Reflection.FinalConviction` | LLM's final conviction. |
| `recommendation.symbol` | `domain.Quote.Symbol` | Per-spec. |

### Aggregate metrics (rollup)

| Metric | Description | Target |
|--------|-------------|--------|
| `llm.latency_p50` | 50th percentile of `llm.latency_ms` (plan + reflect) | < 2s |
| `llm.latency_p95` | 95th percentile | < 8s |
| `tool.success_rate` | Fraction of `RunToolCall` calls that returned a result (vs error) | > 95% |
| `loop.exhausted_rate` | Fraction of recommendations where `Exhausted()=true` | < 5% |
| `reflect.continue_rate` | Fraction of reflect calls where `Continue=true` | < 50% (most should commit quickly) |
| `conviction.distribution` | Histogram of `Reflection.FinalConviction` | Skewed high (LLM should be selective) |

### Comparison vs deterministic baseline

For each sector/symbol, compare the LLM-driven and deterministic recommendations side-by-side:

| Sector | Symbol | LLM Conviction | Det Conviction | Match? | LLM Outcome | Det Outcome |
|--------|--------|----------------|----------------|--------|-------------|-------------|
| semiconductor | 2330 | 75 | 60 | No | +2.3% | +1.8% |
| ai_supply_chain | 2330 | 80 | 75 | Yes | +3.1% | +3.0% |

## Acceptance criteria

L2.4 → production promotion requires:

1. **Quality**: LLM Sharpe ratio >= deterministic baseline (per symbol, averaged over 7-14 days).
2. **Reliability**: `loop.exhausted_rate < 5%`, `tool.success_rate > 95%`, zero unhandled panics.
3. **Latency**: `llm.latency_p95 < 8s` (acceptable for non-HFT sector agents).
4. **Reasoning quality**: spot-check 20+ recommendations; reasoning must be coherent and reference tool outputs.
5. **Roll-back**: disabling the flag mid-window restores deterministic behavior within 1 recommendation cycle.

## Promotion path

After successful L2.4:

1. Mark `UseLLMSectorAgents` as `Source: SourceEmpirical` in `configs/parameters.json`.
2. Flip the default to `true` in a follow-up PR (with a separate `UseLLMSectorAgentsDeprecated` flag for safety).
3. Remove the `LLMDriver` deprecated alias (currently retained for backward compat).
4. Tag as `v0.0.22` or `v0.1.0` (depending on other accumulated changes).

## Risk

If L2.4 fails any acceptance criterion, the rollback is:
- Flip `use_llm_sector_agents` back to `false` in `configs/parameters.json`.
- The deterministic `SemiconductorExecutor` immediately resumes (gate mechanism ensures this).
- Investigate: was the issue the LLM model, the prompt, the tool dispatch, or the state machine?
- File a follow-up PR addressing the root cause before re-attempting L2.4.

## References

- Predecessor: [`../specs/llm-sector-agent.md`](../specs/llm-sector-agent.md) (L2.3 PoC design — moved from `docs/wave-11/L2_3_PLAN_REFLECT.md`)
- Existing log: [`docs/archive/wave-10-observation-log.md`](../archive/wave-10-observation-log.md)
- Plan: [Issue #711 (Wave 10 L2.3 plan)](https://github.com/kaecer68/atlas-go/issues/711) §L2.4
- Metrics: [Issue #740](https://github.com/kaecer68/atlas-go/issues/740) (slog metrics for L2.4 observability in `SemiconductorLLMAgent.Recommend`)
