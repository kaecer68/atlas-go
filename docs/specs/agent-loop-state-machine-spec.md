# AgentLoop State Machine

> **Audience**: developers extending the `SectorAgentLLM` loop or debugging state transitions.
> **Implementation**: `internal/orchestrator/agent_loop.go`
> **Tests**: `internal/orchestrator/agent_loop_test.go` (9 tests)
> **Related**: [`llm-sector-agent.md`](llm-sector-agent-spec.md)

## Phase Diagram

```
              NewAgentLoop(maxIter)
                      │
                      ▼
                ┌───────────┐
                │  Initial  │ ◄────────────────────┐
                └─────┬─────┘                      │
                      │ AdvancePlan               │
                      ▼                            │
                ┌───────────┐                      │
                │   Plan    │                      │
                └─────┬─────┘                      │
                      │ AdvanceToolCall           │
                      ▼                            │
                ┌───────────┐                      │
                │ToolCall   │                      │
                └─────┬─────┘                      │
                      │ AdvanceReflect            │
                      ▼                            │
                ┌───────────┐                      │
                │  Reflect  │                      │
                └─────┬─────┘                      │
                      │                            │
              ┌───────┴───────┐                    │
              │               │                    │
       Continue=true    Continue=false             │
              │               │                    │
       (loop back to   AdvanceFinal                │
        AdvancePlan)        │                     │
              │             ▼                     │
              │       ┌───────────┐                │
              │       │  Final    │──── IsTerminal
              │       └───────────┘                │
              │             │                     │
              │             │ AdvanceFinal again? │
              │             └─────────────────────┘
              │                  (no-op: stays Final)
              │
              └──► Exhausted()?
                   yes → force Final on next Reflect
```

## States

| State | Constant | Description |
|-------|----------|-------------|
| `PhaseInitial` | `"initial"` | Fresh loop; no plan yet. |
| `PhasePlan` | `"plan"` | Plan recorded; awaiting tool invocation. |
| `PhaseToolCall` | `"tool_call"` | Tool dispatch in progress. |
| `PhaseReflect` | `"reflect"` | Tool result in; awaiting reflect decision. |
| `PhaseFinal` | `"final"` | Terminal state; conviction locked. |

## Transition Table

| From | Method | To | Notes |
|------|--------|----|----|
| `PhaseInitial` | `AdvancePlan(steps)` | `PhasePlan` | Increments `Round` by `len(steps)`. |
| `PhasePlan` | `AdvanceToolCall()` | `PhaseToolCall` | **Returns error** if `Phase != PhasePlan` OR `len(Steps) == 0` (Issue #711 #5). |
| `PhaseToolCall` | `AdvanceReflect()` | `PhaseReflect` | **Returns error** if `Phase != PhaseToolCall` (Issue #711 #5). |
| `PhaseReflect` | (LLM decides) | `PhasePlan` or `PhaseFinal` | If `Reflection.Continue=true` → re-plan. If `false` → `AdvanceFinal(conviction)`. |
| Any | `AdvanceFinal(c)` | `PhaseFinal` | Clamps `c` to `[0,100]` (warns on clamp, Issue #711 #7). |

## Key Invariants

1. **Round counts plan steps, not calls** (Issue #711 #6 / C5 fix):
   ```go
   func (l *AgentLoop) AdvancePlan(steps []PlanStep) {
       l.Phase = PhasePlan
       l.Steps = append(l.Steps, steps...)
       l.Round += len(steps)  // NOT +1
   }
   ```
   A single `AdvancePlan` call with 3 steps advances `Round` by 3. This ensures `Exhausted()` measures total plan work, not planning-round count.

2. **Exhausted = `Round >= MaxIter`** (Issue #711 #6):
   ```go
   func (l *AgentLoop) Exhausted() bool {
       if len(l.Steps) >= l.MaxIter && l.Round < l.MaxIter {
           l.exhaustedWarningOnce.Do(func() { /* warn */ })
       }
       return l.Round >= l.MaxIter
   }
   ```
   The legacy `len(Steps) >= MaxIter` check is preserved as a divergence detector (one-time `slog.Warn` via `sync.Once`) for callers that mutate `Steps` directly.

3. **Error returns on illegal transitions** (Issue #711 #5):
   ```go
   func (l *AgentLoop) AdvanceToolCall() error {
       if l.Phase != PhasePlan { return fmt.Errorf(...) }
       if len(l.Steps) == 0  { return fmt.Errorf(...) }
       l.Phase = PhaseToolCall
       return nil
   }
   ```
   Callers MUST handle the error (no `_ =` suppression). The Phase is unchanged on error so callers can recover.

4. **Clamping warns, never silently coerces** (Issue #711 #7):
   ```go
   func (l *AgentLoop) AdvanceFinal(conviction int) {
       if conviction < 0 { slog.Warn("...clamping..."); conviction = 0 }
       else if conviction > 100 { slog.Warn("...clamping..."); conviction = 100 }
       l.Phase = PhaseFinal
       l.FinalConviction = conviction
   }
   ```

5. **NewAgentLoop non-positive defaults to 3** (Issue #711 #8):
   ```go
   func NewAgentLoop(maxIter int) *AgentLoop {
       if maxIter <= 0 { slog.Warn("...non-positive..."); maxIter = 3 }
       return &AgentLoop{Phase: PhaseInitial, MaxIter: maxIter}
   }
   ```

## Embedded Interface Pattern (PR3)

`SectorAgentLLM` embeds the `*AgentLoop` plus two interfaces:
```go
type SectorAgentLLM struct {
    *AgentLoop
    PlanDriver    // embedded interface
    ReflectDriver // embedded interface
    Tools []llm.Tool
}
```

**Important**: accessing `a.PlanDriver.PlanComplete(...)` is **discouraged by staticcheck QF1008** — use the promoted form `a.PlanComplete(...)` instead. The nil check (`if a.PlanDriver == nil`) must come BEFORE the promoted call to avoid nil-pointer dereference.

## Error Semantics

- `AdvancePlan(steps)` — **no error return** (PR2; the method is permissive by design)
- `AdvanceToolCall()` — **returns error** on `Phase != PhasePlan` or empty `Steps`
- `AdvanceReflect()` — **returns error** on `Phase != PhaseToolCall`
- `AdvanceFinal(c)` — **no error return** (clamps with warning)

## IsTerminal vs Exhausted

| Check | Returns | When True |
|-------|---------|-----------|
| `IsTerminal()` | bool | `Phase == PhaseFinal` |
| `Exhausted()` | bool | `Round >= MaxIter` |

A loop can be:
- **Not terminal + not exhausted**: continue planning
- **Not terminal + exhausted**: force `AdvanceFinal` on next `Reflect` (caller's responsibility)
- **Terminal**: done

## Concurrent Safety

**AgentLoop is NOT safe for concurrent use.** Documented by `TestAgentLoop_ConcurrentUnsafe` (skipped by default; uncommenting + `-race` demonstrates a data race on `l.Steps` / `l.Round` / `l.Phase` / `l.exhaustedWarningOnce`).

If you need concurrent access, wrap in an external mutex at the caller level.

## Tests (9 in `agent_loop_test.go`)

| Test | Purpose |
|------|---------|
| `TestAgentLoop_NewDefaultsToInitial` | `NewAgentLoop(0)` → `MaxIter=3`, `Phase=Initial` |
| `TestAgentLoop_PlanReflectFinalSequence` | Happy path: Plan → ToolCall → Reflect → Final |
| `TestAgentLoop_AdvanceFinalClampsConviction` | `AdvanceFinal(150)` → `100`; `AdvanceFinal(-5)` → `0` |
| `TestAgentLoop_AdvanceFinal_StoresConviction` | `AdvanceFinal(75)` → `FinalConviction=75` |
| `TestAgentLoop_ExhaustedAfterMaxIter` | `Exhausted()=true` after `Round=MaxIter` |
| `TestAgentLoop_Exhausted_BasedOnRoundsNotSteps` | C5 fix: `Round=MaxIter` via multi-step plan |
| `TestAgentLoop_AdvancePlan_IncrementsRoundByLenSteps` | `Round += len(steps)`, not +1 |
| `TestAgentLoop_AdvanceToolCall_PhaseMismatch_ReturnsError` | Error on illegal transition |
| `TestAgentLoop_AdvanceReflect_PhaseMismatch_ReturnsError` | Error on illegal transition |
| `TestAgentLoop_NewAgentLoop_NonPositiveMaxIter_Warns` | `NewAgentLoop(<=0)` warns + uses 3 |
| `TestAgentLoop_AdvanceFinal_ClampsConviction_Warns` | `AdvanceFinal(150/-5)` warns on clamp |
| `TestAgentLoop_ConcurrentUnsafe` | Documents non-safety (skipped by default) |

## References

- Implementation: `internal/orchestrator/agent_loop.go`
- Tests: `internal/orchestrator/agent_loop_test.go`
- Issue: [#711](https://github.com/kaecer68/atlas-go/issues/711) #5, #6, #7, #8
- Plan: [Issue #711 (Wave 10 L2.3 plan)](https://github.com/kaecer68/atlas-go/issues/711) (PR2)
- PR: [#725](https://github.com/kaecer68/atlas-go/pull/725) (v0.0.0.20a)
