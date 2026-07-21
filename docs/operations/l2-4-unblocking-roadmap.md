# L2.4 Unblocking Roadmap

> **Status**: Roadmap document. Captures dependency graph + unblock conditions for L2.4 follow-on work that is operationally blocked (not code-blocked).
> **對應**: `docs/operations/l2-4-followup.md` §1 + §3 + Issues #825 / #826

---

## Current state (post-session)

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

### Pending (6 sub-items, all operationally blocked)

| ID | Item | Blocker(s) | Code-blocked? |
|---|---|---|---|
| T13 main | Auto-cron scheduler (Issue #825) | Manual observation success + AutoEnabled test in staging | ❌ No |
| T14-3a | Source upgrade `experimental` → `empirical` | Day 14 acceptance gate pass | ❌ No |
| T14-3b | Default flip to `true` + opt-out | T14-3a done + Day 14 success + deprecation flag review | ❌ No |
| T14-3c | Delete `LLMDriver` interface | Day 14 success + T14-3a + T14-3b (embedding refactor DONE) | ⚠️ 1 of 2 blockers removed |
| T14-3d | Version tag | T14-3a + 3b + 3c all merged | ❌ No |
| T15 main | L2.4 actual start in staging | USER DECISION (env + observer + start date) | ❌ No |

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
- First week: log to `docs/operations/l2-4-observation-log.md` per runbook §2

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

### Step 6: T13 main (auto-cron implementation)

After T14-3b done + production 7+ days:
- Use design from PR #1023 (fault tolerance)
- Use updated runbook §2 from PR #1019 (auto-triggered window review)
- Implement `internal/scheduler/l2_4.go` per design doc
- Wire registration in `cmd/atlas/main.go`
- Add observability (`l2_4_cron.fired/skipped/failed` events)
- Tests for all 8 recovery scenarios

**Time**: 1 PR, medium size.

### Step 7: T14-3c (Delete LLMDriver interface)

After T14-3b done + T13 main running:
- SemiconductorLLMAgent embedding refactor ✅ DONE (PR #1025)
- Remaining: just delete the interface definition at `internal/orchestrator/sector_agent_llm.go:76-83`
- Update `sector_agent_llm_test.go` to drop `stubLLMDriver` + `TestSectorAgentLLM_LLMDriver_DeprecatedAlias`
- Update `orchestrator/AGENTS.md` to remove "不可用 LLMDriver" warning (no longer applicable)

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

- `docs/operations/l2-4-followup.md` §1 (T13 main prereqs)
- `docs/operations/l2-4-followup.md` §3 (T14 promotion 4-step)
- `docs/operations/l2-4-runbook.md` §1, §2, §3 (manual flow)
- `docs/operations/l2-4-observation-log.md` (Week 0-4 template)
- `l2-4-fault-tolerance-design.md` (PR #1023, in docs/operations/)
- （內部審計，`.omo/audit/`）(PR #1024)
- `internal/orchestrator/sector_agent_llm.go:76-83` (LLMDriver definition, target of T14-3c)
- Issue #825 (auto-cron), Issue #826 (promotion), Issue #828 (CLI flag — DONE)