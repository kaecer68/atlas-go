---
name: atlas-experimental-feature-launch-gate
description: Use when adding a new experimental feature that needs a launch gate, designing rollout safety, or promoting L2.x observation patterns. Reference for the canonical "5-condition hard gate" pattern established by L2.4 preflight + auto-cron.
location: ".claude/skills/atlas-experimental-feature-launch-gate/SKILL.md"
scope: project
---

# Skill: Experimental Feature Launch Gate

## When to Invoke

Invoke this skill when:

- Adding a new **experimental feature** that will have a feature flag default off + observation window + planned promotion
- Designing rollout safety for a new L2.x / Sprint N feature
- Asked to write a "preflight" or "launch gate" or "readiness check" for any atlas-go subsystem
- Reviewing a PR that adds a `cmd/experimental/*-preflight/` or `internal/scheduler/*_auto_cron.go` file
- Question arises about whether a feature needs a launch gate

**Do NOT invoke** for:
- Normal feature work (non-experimental)
- Bug fixes, refactors, docs (no rollout risk)
- Internal refactors that don't touch user-facing flag

## What This Skill Teaches

The canonical pattern for experimental feature launch gates in atlas-go. Established by L2.4 sector agents observation window (PR #1027 + #1029), where preflight + auto-cron together form a **5-condition hard gate** that prevents "partial production rollout" silent failures.

## Reference Implementation

Read these in order before designing any new launch gate:

1. **`docs/specs/experimental-feature-launch-gate-spec.md`** — Canonical pattern spec (this skill's source of truth)
2. **`cmd/experimental/l2-4-preflight/main.go`** — Manual path (operator runs before flipping flag)
3. **`internal/scheduler/l2_4_auto_cron.go`** — Automated path (scheduler gates cron trigger)
4. **`docs/operations/l2-4-runbook.md`** — Operator-facing procedure
5. **`docs/archive/l2-4-followup.md`** — Open follow-up issues + promotion plan

## Pattern Summary

```
                ┌────────────────────────┐
                │ Experimental Feature    │
                │ (flag=off, has window,  │
                │  promotion plan)        │
                └────────┬───────────────┘
                         │
            ┌────────────┴─────────────┐
            ▼                          ▼
   ┌─────────────────┐         ┌─────────────────┐
   │  Preflight      │         │  Auto-cron gate │
   │  (manual path)  │         │  (auto path)    │
   │  cmd/exp/*/     │         │  int/sched/*/   │
   │  -preflight/    │         │  *_auto_cron.go │
   └────────┬────────┘         └────────┬────────┘
            │ 5 automatable + 3 manual   │ 5-condition hard
            │ checks                    │ gate
            ▼                          ▼
   ┌──────────────────────────────────────────────┐
   │          Shared decision logic               │
   │   (any condition false → no-op / exit 1)     │
   └──────────────────────────────────────────────┘
```

## How to Apply

### Step 1: Verify 3 Conditions for "Experimental Feature"

Before designing a launch gate, confirm **all 3** are true:

- [ ] Feature flag default is **off** (e.g., `UseLLMSectorAgents = false`)
- [ ] There is a planned **observation window** (e.g., 30 days)
- [ ] There is a **promotion plan** (e.g., 4-step procedure: Source upgrade → Default flip → Removal → Tag)

If any 1 is false, this is NOT an experimental feature — proceed with normal feature delivery (no launch gate needed).

### Step 2: Clone Preflight Pattern

Create `cmd/experimental/<feature>-preflight/main.go`:

```go
type checkResult struct {
    Name    string
    OK      bool
    Manual  bool   // true = operator confirms externally
    Message string
}

func main() {
    baseURL := validateLocalhostURL(os.Args[1])  // SSRF guard, mandatory

    checks := []checkResult{
        checkConfigFlag(),         // automatable
        checkProviderHealth(),     // automatable
        checkDataReady(),          // automatable
        checkCircuitBreaker(),     // automatable
        checkOperatorConfirm(),    // manual
    }
    // ... render + exit code logic per L2.4 reference
}
```

Constraints:
- **SSRF guard** mandatory (`validateLocalhostURL`)
- **5-7 checks** total (not fewer, not more)
- **Manual checks** must be marked explicitly, not lumped with automatable

### Step 3: Clone Auto-cron Pattern (if applicable)

If the feature has a scheduler trigger, create `internal/scheduler/<feature>_auto_cron.go`:

```go
func ShouldL24AutoCronFire() (bool, string) {
    if os.Getenv(L24AutoCronEnvVar) != "true" { return false, "env not opt-in" }
    if params == nil || !params.GetAutoEnabled() { return false, "flag not enabled" }
    if !hasDataReady() { return false, "data not ready" }
    if !inTimeWindow() { return false, "out of window" }
    // ... 5 conditions, all must pass
    return true, ""
}
```

Constraints:
- **Default off** — env var must be explicitly set
- **No partial pass** — any condition false → return false
- **No exceptions** — even for "we know what we're doing"

### Step 4: Update CI Allow-list

Add the new preflight/auto-cron to `scripts/ci/check_no_duplicate_preflight.sh` allow list. Without this, the CI will warn that there are now 2 instances.

### Step 5: Update Spec

Add a row to the "Reference Implementations" table in `docs/specs/experimental-feature-launch-gate-spec.md`. This keeps the pattern's audit trail complete.

### Step 6: Cross-link Docs

Update related `docs/operations/<feature>-*` files to link to the new launch gate.

## Anti-patterns (Hard No)

| Don't | Why | Do Instead |
|-------|-----|-----------|
| Write ad-hoc preflight in your module | Standards fragmentation | Follow this spec, clone L2.4 pattern |
| Generalize L2.4 preflight into shared library | L2.4-specific checks resist generalization; risk over-engineering | Keep instance-level, share pattern only |
| Bypass gate with "we know what we're doing" | Loses audit trail + silent failure risk | Always run preflight, fix failures |
| Add soft-check "this condition is optional" | Partial production = no production | Hard include or remove condition |
| Run preflight against production URL | SSRF risk | `validateLocalhostURL` mandatory |
| Use different logic for manual vs auto path | "Human OK, scheduler skipped" inconsistency | Share decision logic |

## Verification Checklist

Before merging a new launch gate, verify:

- [ ] All 3 "experimental feature" conditions met (flag off + window + promotion)
- [ ] Preflight has SSRF guard
- [ ] Preflight has 5-7 checks, manual ones marked
- [ ] Auto-cron (if any) has 5-condition hard gate, default off
- [ ] Allow-list updated in `scripts/ci/check_no_duplicate_preflight.sh`
- [ ] Spec table updated with new row
- [ ] Doc cross-links in place
- [ ] Local test: preflight exits 0 when all pass, non-zero when fail
- [ ] CI passes (including `check_no_duplicate_preflight.sh` warning check)

## Reference: Existing Instance

The only existing instance is L2.4:

- **Preflight**: `cmd/experimental/l2-4-preflight/main.go` (PR #1027)
- **Auto-cron**: `internal/scheduler/l2_4_auto_cron.go` (PR #1029)
- **Spec**: `docs/specs/l2-4-observation-spec.md` (metric schema only — operational concerns in runbook)
- **Runbook**: `docs/operations/l2-4-runbook.md`
- **Followup**: `docs/archive/l2-4-followup.md`
