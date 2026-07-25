# L2.4 Observation Window — Spec

> **Status**: Metrics implementation shipped (PR #821, merge commit `f69b3551`, 2026-06-29).
> **Predecessor**: [`./llm-sector-agent-spec.md`](./llm-sector-agent-spec.md) (L2.3 PoC design)
> **Operational guide**: [`../operations/l2-4-runbook.md`](../operations/l2-4-runbook.md)
> **Linked log**: `docs/operations/l2-4-observation-log.md`
> **Future work**: [`../operations/l2-4-followup.md`](../operations/l2-4-followup.md) §3
> **Issues**: [#740](https://github.com/kaecer68/atlas-go/issues/740) (slog metrics for L2.4 observability in `SemiconductorLLMAgent.Recommend`)

## Overview

After L2.3 PoC lands, the `UseLLMSectorAgents` flag can be enabled in a non-production environment for a **7-14 day observation window**. The window validates LLM-driven sector agent behavior against the deterministic baseline and surfaces regressions before production rollout.

This spec defines the **metrics schema** — the slog event contract that any downstream log consumer (dashboard, SLO, alert) depends on. For operational procedures (pre-flight, daily check-in, acceptance criteria, rollback), see the [L2.4 Runbook](../operations/l2-4-runbook.md).

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

# Plus env var (required for factory to register the plugin)
export LLM_SECTOR_AGENTS_ENABLED=true

# Or via CLI (when wired — see ../operations/l2-4-followup.md §2)
atlas run --use-llm-sector-agents
```

## Metrics

> 對齊 `SemiconductorLLMAgent.Recommend()` 實際 emit 的 slog events(Issue #740 設計 — 見 `internal/orchestrator/semiconductor_llm_agent.go`)。不發獨立 `agent_loop.exhausted` event,改用 `agent_loop.end.exhausted` 欄位;reflect 不量測 latency。PR #743 舊命名(`plan_complete` / `tool_call`)已被 commit `eff0db79` (#746) 取代。

### Per-recommendation metrics

| Metric | Source slog event | Field | Notes |
|--------|-------------------|-------|-------|
| `recommendation.symbol` | `agent_loop.start` / `agent_loop.end` | `symbol` | 觀察對象的 ticker |
| `recommendation.skill` | `agent_loop.start` | `skill` | `SemiconductorLLMAgentSkill = "semiconductor_desk"` |
| `agent_loop.plan.size` | `agent_loop.plan` | `size` | LLM 規劃的 tool step 數量(可能為 0 = plan error) |
| `agent_loop.plan.latency_ms` | `agent_loop.plan` | `latency_ms` | Plan 階段 wall-clock |
| `agent_loop.plan.err` | `agent_loop.plan` | `err` | 非 nil = plan 失敗,該 recommendation 中止 |
| `agent_loop.tool.name` | `agent_loop.tool` | `name` | 對應 `llm.Tool.Name` |
| `agent_loop.tool.success` | `agent_loop.tool` | `success` | bool,`err==nil` 時為 true |
| `agent_loop.tool.latency_ms` | `agent_loop.tool` | `latency_ms` | 單次 tool 執行 wall-clock |
| `reflect.continue` | `agent_loop.reflect` | `continue` | LLM 是否要繼續 iterate |
| `recommendation.conviction` | `agent_loop.reflect` / `agent_loop.end` | `conviction` | LLM 最終 conviction(0-100) |
| `loop.exhausted` | `agent_loop.end` | `exhausted` | bool;`true` = for-loop 跑完所有 `MaxIter` 仍未 `Continue=false` |

### Aggregate metrics (rollup)

| Metric | Source | Target |
|--------|--------|--------|
| `llm.latency_p50` | 50th percentile of `agent_loop.plan.latency_ms` | < 2s |
| `llm.latency_p95` | 95th percentile of `agent_loop.plan.latency_ms` | < 8s |
| `tool.success_rate` | `agent_loop.tool.success=true` / 總 `agent_loop.tool` 數 | > 95% |
| `loop.exhausted_rate` | `agent_loop.end.exhausted=true` / 總 recommendation 數 | < 5% |
| `reflect.continue_rate` | `agent_loop.reflect.continue=true` / 總 reflect 數 | < 50% |
| `conviction.distribution` | Histogram of `recommendation.conviction` | Skewed high |

> `llm.latency_p50` / `p95` 僅量測 plan 階段;reflect 延遲 Issue #740 不量測。若需 reflect 延遲,需在後續 PR 補上(需同步修改 `semiconductor_llm_agent_metrics_test.go` 的 reflect latency 斷言)。

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

Operational details (Day 7 / Day 14 checkpoints, rollback procedure): see [L2.4 Runbook §3](../operations/l2-4-runbook.md#3-acceptance-criteria) and [§4 Rollback](../operations/l2-4-runbook.md#4-rollback-procedure).

## Future work (Promotion)

The full promotion procedure (4 steps: Source upgrade → Default flip → LLMDriver removal → Version tag) is documented in [`../operations/l2-4-followup.md`](../operations/l2-4-followup.md) §3. The spec intentionally stops at the metric schema boundary — operational concerns (roll-back, promotion, risk) live in the runbook to avoid duplication.

## References

- Predecessor: [`./llm-sector-agent-spec.md`](./llm-sector-agent-spec.md) (L2.3 PoC design)
- Operational guide: [`../operations/l2-4-runbook.md`](../operations/l2-4-runbook.md)
- Future work: [`../operations/l2-4-followup.md`](../operations/l2-4-followup.md) (auto-cron / CLI flag / promotion)
- Existing log: `docs/operations/l2-4-observation-log.md`
- Plan: [Issue #711 (Wave 10 L2.3 plan)](https://github.com/kaecer68/atlas-go/issues/711) §L2.4
- Metrics: [Issue #740](https://github.com/kaecer68/atlas-go/issues/740)
- Implementation: PR #821 — `internal/orchestrator/semiconductor_llm_agent.go` (orchestrator), `internal/monitoring/api/pipeline/l2_4_*.go` (state + handlers), `cmd/atlas/main.go` (route registration + `SetConfig` seed), `shared_web/static/js/pages/synergy.js` (admin UI)
