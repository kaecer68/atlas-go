# L2.4 T14-3c Prerequisite — LLMDriver Usage Audit

> **Status**: Audit document. No code change. Pre-implementation verification for `followup.md` §3c ("LLMDriver Deprecated Alias 移除").
> **對應**: `docs/operations/l2-4-followup.md` §3c + Issue #826 + `internal/orchestrator/sector_agent_llm.go`

---

## TL;DR

`LLMDriver` interface is **NOT just a deprecated alias** — it is **actively embedded as a struct field in production code** (`SemiconductorLLMAgent`).

`followup.md` §3c prerequisite says "確認所有呼叫端都改用 `SectorAgentLLMDriver`". **This audit proves that prerequisite is NOT met** for production code. T14-3c (remove LLMDriver interface) cannot proceed without first refactoring `SemiconductorLLMAgent` to embed `PlanDriver + ReflectDriver` separately.

---

## LLMDriver definition

`internal/orchestrator/sector_agent_llm.go:76-83`:

```go
// LLMDriver is the combined contract for an LLM backend that drives
// both the Plan and Reflect phases.
//
// Deprecated: use PlanDriver and ReflectDriver separately so an
// implementation can supply just the phase it supports. LLMDriver
// remains as a convenience alias (= PlanDriver + ReflectDriver) for
// backward compatibility with code written before the split.
type LLMDriver interface {
    PlanDriver
    ReflectDriver
}
```

The `// Deprecated:` GoDoc tag is correct, but **the deprecation is structural — removing the interface breaks compilation in places that embed it**.

---

## Production usage (non-test) — 3 actual code sites

### Site 1: `internal/orchestrator/semiconductor_llm_agent.go:32-36`

**CRITICAL — this is the gate for T14-3c.**

```go
// LLMDriver is the combined plan+reflect LLM backend (the
// deprecated LLMDriver alias from PR3, which is the intersection
// of PlanDriver and ReflectDriver interfaces).
LLMDriver  // <-- struct field embedding LLMDriver directly
```

`SemiconductorLLMAgent` struct **embeds** the LLMDriver interface as a field. This is the dominant production user of LLMDriver — `SemiconductorLLMAgent` is the L2.4 agent (replaced `SectorAgentLLM` in PR #733).

### Site 2: `internal/orchestrator/semiconductor_llm_agent.go:103`

```go
if a.LLMDriver == nil {
    // no LLMDriver wired — L2.4 observation window state machine
    // ...
}
```

**Production runtime path** — guard for missing driver during L2.4 observation.

### Site 3: `internal/orchestrator/semiconductor_llm_agent.go:111-115`

```go
return &SemiconductorExecutionContext{
    PlanDriver:    a.LLMDriver,   // ← production code uses a.LLMDriver
    ReflectDriver: a.LLMDriver,   // ← production code uses a.LLMDriver
    ...
}
```

**PRODUCTION CODE** — `BuildExecutionContext` method (or equivalent) copies `a.LLMDriver` into the new context as both PlanDriver and ReflectDriver. If `LLMDriver` interface is removed, this assignment fails to compile.

### Site 4: `internal/orchestrator/system_plugins.go:114`

```go
func (s *System) WithLLMSectorAgents(driver *SectorAgentLLMDriver) *System {
```

Uses `SectorAgentLLMDriver` (the NEW struct) — **clean, no LLMDriver reference**. ✅

### Site 5: `internal/orchestrator/factory.go:89` — comment only

```go
// with a real SectorAgentLLMDriver implementation.
```

No code change needed.

---

## Test usage — 4 explicit test sites

### `internal/orchestrator/sector_agent_llm_test.go:14,19,21`

```go
// Regression guard against the Issue #711 #10 LLMDriver split
// stubLLMDriver is a minimal implementation that satisfies both the
// PlanDriver and ReflectDriver interfaces (and therefore the
// LLMDriver alias). Tests that wire SectorAgentLLM should assign
```

### `sector_agent_llm_test.go:23-30` — `stubLLMDriver` struct

Used to satisfy BOTH PlanDriver + ReflectDriver with one type (mirrors LLMDriver alias behavior).

### `sector_agent_llm_test.go:106-107` — `TestSectorAgentLLM_LLMDriver_DeprecatedAlias`

```go
// TestSectorAgentLLM_LLMDriver_DeprecatedAlias verifies the Issue #711
// #10 backward-compat guarantee: the deprecated LLMDriver interface
```

**Explicit test for the deprecated alias contract** — must be deleted as part of T14-3c.

### `llm_sector_agent_plugin_test.go:30,53,87-93`

Uses `&SectorAgentLLMDriver{}` directly (NEW struct). Will need refactor when LLMDriver embedding in `SemiconductorLLMAgent` changes.

---

## Doc / comment usage

| Location | Type | Action for T14-3c |
|---|---|---|
| `sector_agent_llm.go:36,58,69,76,80` | GoDoc comments on PlanDriver/ReflectDriver/LLMDriver | Doc-only; will go away with the type itself |
| `sector_agent_llm.go:89` | `ErrNotImplemented` comment mentions "no LLMDriver is wired" | Re-write to "no LLM backend is wired" |
| `semiconductor_llm_agent.go:32-33` | Struct field comment | Re-write to mention PlanDriver + ReflectDriver separately |

---

## T14-3c Feasibility Analysis

### Prerequisite verification (per followup.md §3c)

> **前置條件**:
> - 3b 完成 + production 跑過 7+ 天
> - 確認 `grep -r LLMDriver internal/` 沒有非測試用法
> - `sector_agent_llm_test.go` 改用新介面

**Result of `grep -r LLMDriver internal/`** (this audit):

| Location | Type | Status |
|---|---|---|
| `sector_agent_llm.go:76-83` | Interface definition | Remove |
| `semiconductor_llm_agent.go:32-36` | **PRODUCTION struct field** | ⚠️ Refactor needed |
| `semiconductor_llm_agent.go:103` | **PRODUCTION guard** | ⚠️ Refactor needed |
| `semiconductor_llm_agent.go:113-114` | **PRODUCTION struct literal** | ⚠️ Refactor needed |
| `sector_agent_llm_test.go:14,19,21,23-30,106-107` | Test-only | Refactor or delete |
| `llm_sector_agent_plugin_test.go:30,53,87-93` | Test-only | Refactor |
| `sector_agent_llm.go:36,58,69,80,89` | Comments only | Doc-only |
| `factory.go:89` | Comment only | Doc-only |

### Verdict

**T14-3c as currently specified CANNOT proceed safely.** The prerequisite "確認 `grep -r LLMDriver internal/` 沒有非測試用法" is **NOT met** — `semiconductor_llm_agent.go` has 3 production usages.

### Required prerequisite work (NEW sub-task)

Before T14-3c can run, this refactor must happen first:

**Refactor: `SemiconductorLLMAgent` embedding split**

1. Replace `LLMDriver` field embedding (L32-36) with separate `PlanDriver` + `ReflectDriver` fields
2. Update L103 nil check to check both fields (or keep as single semantic if they're always set together)
3. Update L111-115 to use new field names directly
4. Update `system_plugins.go:114` `WithLLMSectorAgents` signature to accept both (or pass struct that has both)
5. Update `llm_sector_agent_plugin_test.go` test wiring
6. Add `TestSemiconductorLLMAgent_PlanAndReflectSeparately` to verify split behavior

**After refactor**: `LLMDriver` interface has only test usages + the deprecated alias definition. T14-3c becomes safe to execute (per followup.md §3c original prerequisites).

---

## Recommended PR Sequence

1. **This PR (audit)**: ships this doc. No code change. Documents the gap.
2. **Future PR A**: refactor `SemiconductorLLMAgent` embedding (listed above)
3. **Future PR B** (T14-3c): delete LLMDriver interface + update sector_agent_llm_test.go

**Each PR independently merge-able, but must be done in this order.**

---

## What this audit does NOT cover

- Does NOT verify `WithLLMSectorAgents` callers (other modules that wire `SectorAgentLLMDriver`)
- Does NOT check if any external consumer (cmd/atlas, configs/) hardcodes LLMDriver
- Does NOT verify backward-compat: removing LLMDriver may break third-party agents that use the alias

These should be checked before T14-3c merge.

---

## References

- `docs/operations/l2-4-followup.md` §3c (Issue #826, T14-3c)
- `internal/orchestrator/sector_agent_llm.go:76-83` (LLMDriver definition)
- `internal/orchestrator/semiconductor_llm_agent.go:32-36, 103, 113-114` (production usage)
- `internal/orchestrator/sector_agent_llm_test.go` (test-only usage)
- `internal/orchestrator/system_plugins.go:114` (uses new struct, no LLMDriver)