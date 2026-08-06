# L2.4 Unblocking Roadmap

> **Status**: Roadmap document. Captures dependency graph + unblock conditions for L2.4 follow-on work that is operationally blocked (not code-blocked).
> **對應**: `docs/archive/l2-4-followup.md` §1 + §3 + Issues #825 / #826

---

## Current state (post-session)

### ⛔ 2026-08-06 — 軌道收尾 (Issue #825 + #826 CLOSED)

> 依 [`docs/manifests/2026-08-06-l2-4-issue-alignment-audit.md`](../manifests/2026-08-06-l2-4-issue-alignment-audit.md) 盤查決策:
> - **#825 (auto-cron) 關閉**:`ShouldL24AutoCronFire` 0 production callers (dead code, 已加 deprecation 註記);自動化缺口由 C07 平行軌道 (c07-obs-collector / c07-day-evaluator / c07-preflight) 填補。
> - **#826 (promotion) 關閉**:Day 14 觀察 gate 未通過且短期無法滿足;3c (LLMDriver alias 移除) 已於本收尾完成。
> - L2.4 觀察期未啟動 (`use_llm_sector_agents.value=false` 28+ 天),無 baseline 可驗證 promotion。
> - 剩餘真實缺口 (其他 sector LLM 變體 + generic LLM framework) 另開 issue 追蹤。

### Shipped (7 PRs as of 2026-07-08)

| PR | Title | Followup.md item | Type |
|---|---|---|---|
| #1019 | Wave 11 observation log 範本 | §1 prereq item 4 (runbook §2 rewrite) | Doc |
| #1020 | install-hooks.sh 改進 | Operational infra | Tooling |
| #1021 | CLI flag `--use-llm-sector-agents` | §2 (CLI Flag Wiring) | Feature |
| #1023 | L2.4 fault tolerance design | §1 prereq item 3 | Doc |
| #1024 | LLMDriver usage audit | §3c prereq verification | Doc |
| #1025 | SemiconductorLLMAgent 嵌入重構 | §3c NEW prereq (not in original followup.md) | Refactor |
| #1022 | risk/AGENTS.md RiskGate omission | Drift fix (T16) | Doc |

### Pending → 已關閉 (2026-08-06)

| ID | Item | 處置 |
|---|---|---|
| T13 main | Auto-cron scheduler (Issue #825) | ❌ **CLOSED** — 0 callers dead code, C07 已覆蓋 |
| T14-3a | Source upgrade `experimental` → `empirical` | ❌ **CLOSED** — Day 14 gate 未通過 |
| T14-3b | Default flip to `true` + opt-out | ❌ **CLOSED** — 依賴 3a |
| T14-3c | Delete `LLMDriver` interface | ✅ **DONE (2026-08-06)** — alias 移除,0 production usages |
| T14-3d | Version tag | ❌ **CLOSED** — 依賴 3a/3b |
| T15 main | L2.4 actual start in staging | ❌ **CLOSED** — 28+ 天無 USER DECISION,軌道收尾 |

---

## Dependency graph

```
                            T15 main (USER DECISION: staging env)
                                  │
                                  ▼
                       Manual observation runs 14 days
                                  │
                                  ▼
                       Day 7 / Day 14 acceptance gate
                                  │
            ┌─────────────────────┼─────────────────────┐
            ▼                     ▼                     ▼
   T14-3a (Source upgrade)   T13 main (auto-cron)   (T14-3c now
   10 min change             §1 "否" + 4 prereqs     partially unblocked)
   followup.md §3a          followup.md §1          embedding refactor
                                                      ✅ DONE (PR #1025)
            │                     │
            ▼                     ▼
       T14-3b (Default flip)   (auto-cron implemented
       needs 3a + Day 14        in T13 follow-up PR)
       followup.md §3b
            │
            ▼
       T14-3c (LLMDriver delete)
       needs 3b + Day 14
       followup.md §3c
       ⚠️ 1 of 2 blockers removed by this session (T20)
            │
            ▼
       T14-3d (Version tag)
       needs 3a + 3b + 3c
```

---

## What THIS session removed from the blockers (vs followup.md baseline)

| Blocker | Original spec | Now | Source |
|---|---|---|---|
| Auto-cron prereq #3 (fault tolerance) | Need to design | ✅ DESIGNED | PR #1023 |
| Auto-cron prereq #4 (runbook §2 rewrite) | Need to rewrite | ✅ REWRITTEN | PR #1019 (commits to runbook §2) |
| T14-3c prereq (grep LLMDriver clean) | Need to verify | ✅ VERIFIED (found production usage instead) | PR #1024 |
| T14-3c prereq (SemiconductorLLMAgent embedding) | **UNSPECIFIED in followup.md** | ✅ REFACTORED | PR #1025 (newly identified gap) |

**Net effect**: When staging env is ready + 14 days pass, T13 main has 2 of 4 prereqs done; T14-3c has 1 of 2 blockers removed.

---

## Unblock action sequence

> **⛔ 2026-08-06 — 本序列已失效**:#825 / #826 已關閉,Step 1-5 (T15 / 觀察 / Day 14 / 3a / 3b) 與 Step 8 (3d) 不再執行;Step 6 (auto-cron) 關閉;Step 7 (3c) 已完成。保留本序列供未來若重啟 L2.4 觀察期的流程參考。

### Step 1: User picks staging env (T15)

Decision required:
- Which environment? (Docker compose staging? Dedicated L2.4 harness? Existing dev box with env flag set?)
- Who observes first week? (Owner? Delegate?)
- Start date? (drives 14-day schedule)

**Time**: user decision, no code.

### Step 2: T15 main — start manual observation

After Step 1:
- `UseLLMSectorAgents.value: false` → `true` (parameters.json)
- `LLM_SECTOR_AGENTS_ENABLED=true` env var
- `docker compose restart atlas`
- Verify `/admin/#page-synergy` L2.4 schedule panel
- First week: log to `docs/archive/l2-4-observation-log.md` per runbook §2

**Time**: 14 days (operator attention daily per runbook §2 daily review)

### Step 3: Day 14 acceptance gate

Per runbook §3:
- 6 indicators all pass
- ≥ 30 spot-checks
- 1× rollback verification
- LLM-driven Sharpe ≥ deterministic baseline

If pass: proceed to Step 4. If fail: rollback + iterate (see runbook §4).

### Step 4: T14-3a (Source upgrade)

```bash
# configs/parameters.json
{
  "orchestrator.use_llm_sector_agents": {
    "source": "empirical",  # was "experimental"
    "value": false           # value unchanged
  }
}
```

**Time**: 10 min, 1 PR.

### Step 5: T14-3b (Default flip + opt-out flag)

Per followup.md §3b:
- `value: false` → `true`
- Add `orchestrator.use_llm_sector_agents_deprecated` (default `true`) for emergency opt-out
- Update `internal/orchestrator/orchestrator.go` `Supports()` gate logic
- Synergy page deprecation warning + opt-out toggle
- CSS update for deprecation warning

**Time**: 1-2 days, 1 PR.

### Step 6: T13 main (auto-cron implementation) — ⛔ CLOSED (2026-08-06)

> #825 已關閉。`ShouldL24AutoCronFire` 0 production callers,gate 未 wire-up;C07 平行軌道已填補自動化缺口。原始指令保留供參考:
- Use design from PR #1023 (fault tolerance)
- Implement `internal/scheduler/l2_4.go` per design doc
- Wire registration in `cmd/atlas/main.go`
- Add observability (`l2_4_cron.fired/skipped/failed` events)
- Tests for all 8 recovery scenarios

**Time**: 1 PR, medium size.

### Step 7: T14-3c (Delete LLMDriver interface) — ✅ DONE (2026-08-06)

> 已於 2026-08-06 完成 (Issue #826 關閉時清理):
- SemiconductorLLMAgent embedding refactor ✅ DONE (PR #1025)
- ✅ `internal/orchestrator/sector_agent_llm.go` 的 `LLMDriver` 介面已刪除 (原 :76-83)
- ✅ `sector_agent_llm_test.go` 已 drop `TestSectorAgentLLM_LLMDriver_DeprecatedAlias`
- ✅ `orchestrator/AGENTS.md` 已移除「不可用 LLMDriver」warning

**Time**: 0.5 day, 1 PR.

### Step 8: T14-3d (Version tag)

After Steps 4-7:
- `git tag v0.0.0.22` (or appropriate)
- Update `CHANGELOG.md` with full L2.4 promotion notes
- 1 PR

**Time**: 30 min.

---

## What this roadmap does NOT cover

- **CLI flag `--use-llm-sector-agents` (Issue #828 / PR #1021)**: already shipped. Operators can use it for staging start/stop without waiting for auto-cron.
- **L2.4 observation log 範本 (PR #1019)**: already shipped. Use it to record daily observations.
- **Auto-cron fault tolerance design (PR #1023)**: already shipped. Use it as design spec when implementing T13 main.

---

## Why this doc exists

Scattered PRs make it hard to see the path forward. This doc captures:

1. **Current state** (what's shippable, what's pending)
2. **Dependency graph** (which items unlock which)
3. **Specific unblock conditions** (what each item waits for)
4. **What THIS session did** (so future readers understand PR #1025 is not orphan — it's the missing prerequisite from followup.md §3c)
5. **Order of operations** (the 8-step sequence)

When picking up this work next session, **start at Step 1 (T15 user decision)**. Everything else flows from that.

---

## References

- `docs/archive/l2-4-followup.md` §1 (T13 main prereqs)
- `docs/archive/l2-4-followup.md` §3 (T14 promotion 4-step)
- `docs/operations/l2-4-runbook.md` §1, §2, §3 (manual flow)
- `docs/archive/l2-4-observation-log.md` (Week 0-4 template)
- `l2-4-fault-tolerance-design.md` (PR #1023, in docs/operations/)
- （內部審計，`.omo/audit/`）(PR #1024)
- `internal/orchestrator/sector_agent_llm.go:76-83` (LLMDriver definition, target of T14-3c)
- Issue #825 (auto-cron), Issue #826 (promotion), Issue #828 (CLI flag — DONE)