# Rate Limit Prevention Guide — Execution Patterns for Future Plans

> **Problem**: During `decision-chain-evolution-v2`, parallel verification agents exhausted API quota (`5 hour session limit`).
> **Purpose**: This guide documents the patterns that caused it and the replacements to use going forward.

---

## Rule 1: Verification Agents Run SEQUENTIALLY, Not In Parallel

**BAD — consumed ~4x quota simultaneously:**
```
Wave Final (parallel):
  ├── F1: Plan Compliance Audit → deep (background)
  ├── F2: Code Quality Review → unspecified-high (background)
  ├── F3: Real Manual QA → unspecified-high (background)
  └── F4: Scope Fidelity Check → deep (background)
```

**GOOD — quota consumed linearly:**
```
Wave Final (sequential):
  Step 1: F1 Plan Compliance Audit → oracle (sync, not background)
  Step 2: F2+F4 Build/Lint + Scope Check → quick (merged, sync)
  Step 3: F3 Manual QA → unspecified-high (sync, run last)
```

**Reason**: Each background agent creates an independent model session. 4 parallel agents = 4x per-minute consumption. Running sequentially means the 5-hour window is shared across all tasks, not exhausted by parallel overdraw.

---

## Rule 2: Match Category to Actual Complexity

Refer to the category descriptions — they have different model costs:

| Task Type | Use Category | Rationale |
|-----------|-------------|-----------|
| Checklist audit, doc review | `oracle` (subagent) or `unspecified-low` | Light read-only; no code gen |
| Build/lint/test execution | `quick` | Tool-running only, minimal model inference |
| Single file change with clear spec | `quick` | Known location, bounded change |
| 2–5 file change across domain boundary | `unspecified-high` | Needs understanding but not deep reasoning |
| Hard logic, algorithm design | `deep` or `ultrabrain` | Reserve for genuinely hard problems |
| Frontend/UI changes | `visual-engineering` | Dedicated model; also lighter than `deep` for UI work |

**Rule of thumb**: If the task can be expressed as a checklist → `unspecified-low`. If it's running existing tools → `quick`. Only use `deep` when the problem genuinely requires novel reasoning.

---

## Rule 3: Merge Overlapping Verification Tasks

During the F2–F4 wave, all three agents independently read:
- `internal/orchestrator/conviction_builder.go`
- `internal/monitoring/service/pipeline.go`
- `internal/monitoring/api/pipeline/handlers.go`
- `web/static/js/pages/pipeline.js`

That's 4 I/O passes x 3 agents = 12 redundant reads.

**Merge strategy:**
- F2 (quality check) + F4 (scope check) → one agent: "Run build/lint, then verify each task's implementation matches spec"
- F3 (manual QA) runs independently since it requires different tools (curl, Playwright)

---

## Rule 4: Limit Background Agent Prompt Scope

When firing background agents, add scope restrictions to prevent token waste:

```
BAD — open-ended:
  prompt: "Verify all P0 deliverables work correctly."

GOOD — scoped:
  prompt: "Verify P0 deliverables by: 1) Run go build ./..., 2) Check that PipelineItemMetrics struct exists in handlers.go, 3) Run go test ./internal/monitoring/... Do NOT read files outside internal/monitoring/ and internal/domain/."
```

---

## Rule 5: Set `run_in_background=false` for Critical Path Items

`run_in_background=true` means "I don't need this result to continue." For verification agents, this is almost never true — you DO need the result before shipping.

```typescript
// BAD: Can't act on result without waiting
task(category="deep", run_in_background=true, prompt="Audit complete plan...")

// GOOD: Blocking, deliberate, controlled
task(category="oracle", run_in_background=false, prompt="Check compliance...")
```

---

## Appendix: Token Budget Estimation (Rough)

| Category | Estimated tokens/task | Notes |
|----------|---------------------|-------|
| `quick` | ~20K–80K | Tool execution, small context |
| `oracle` (subagent) | ~50K–200K | Read-only reasoning, moderate context |
| `unspecified-low` | ~50K–150K | Light task execution |
| `unspecified-high` | ~100K–300K | Multi-file, moderate complexity |
| `deep` | ~200K–500K+ | Heavy reasoning, exploration, code gen |
| Parallel background (N agents) | Sum of all categories | Budget multiplier = N |

**Safe budget for 5-hour window**: ~500K–800K total tokens. Beyond this, expect rate limiting.
