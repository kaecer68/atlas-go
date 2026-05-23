## Phase 1: Context

**PR**: `feat(decision-chain): parameter provenance, sensitivity, and version history`
**Size**: 20 files, +727/−23 (large but coherent — single feature across domain → backend → frontend)
**CI**: gofmt ✅ build ✅ vet ✅ staticcheck ✅ (1 expected unused utility)
**Business need**: Config-driven decision traceability for audit and reproducibility

---

## Phase 2: High-Level Architecture

### Design Quality — Strong

- Clean layering: domain types → modulator provenance → API → frontend
- `ParameterSnapshot` captured once per session (shared pointer) — correct for read-only parameter state
- `CollectModulationSteps()` pattern cleanly separates step collection from application
- `ModulationStep` with `RecIndex` avoids in-place modification during collection
- `omitempty` everywhere — backward compatible with existing data/API consumers

---

## Phase 3: Findings

### [important] TS type mismatch: `parameter_snapshot` typed as `string | null`

**File**: `web/static/js/shared/field_types.d.ts:442` and `field_types.ts:445`

The generated TypeScript types declare:

```typescript
parameter_snapshot?: string | null;
```

But the Go struct produces a JSON object (the `ParameterSnapshot` struct), so it should be:

```typescript
parameter_snapshot?: ParameterSnapshot | null;
```

This is a code-generation artifact (`go generate .` → genTags). If frontend code ever accesses `item.parameter_snapshot.config_version`, it will silently fail at runtime (undefined) or TS would catch it as a type error. Recommend fixing the genTags mapping or post-processing the output — it matters once the frontend consumes this field.

### [important] Legacy `ModulateRecommendations()` and `CollectModulationSteps()` duplicate provenance logic

Both modulators now carry two methods that do the same thing with slightly different APIs:

| Method | In production? | Has prov. logic? |
|--------|--------------|-----------------|
| `CollectModulationSteps()` | Called from `executors.go` | Yes |
| `ModulateRecommendations()` | Only called from tests | Yes (duplicated) |

The duplication means if provenance logic changes, both paths need updating. Consider either: (a) have the old method delegate to the new one, or (b) document that the old path is legacy-only and wont be maintained.

### [nit] `addWithProvenance()` is unreachable dead code

`staticcheck` already flags this. The modulators use direct `ConvictionStep{}` construction instead. The plan documents this as an intentional utility method, but it adds a small confusion cost. Consider either removing it (since the pattern used is direct struct construction) or annotating it as `//nolint:unused` with a doc comment explaining the intent.

### [nit] Characterization tests pass both RED and GREEN phases

The three characterization tests use `t.Logf` (not `t.Error`/`t.Fatal`) for provenance field assertions. This means they never truly failed during the "RED" phase — they always pass. This is acceptable as a design choice but deviates from strict TDD. Consider using `t.Skip()` with a pre-condition check if the distinction matters.

### [praise] Excellent backward-compatibility discipline

- All new fields use `omitempty` → old JSON data unmarshals without error
- `add()` method signature unchanged
- `PipelineItem.Metrics` is a pointer with `omitempty` → old responses have no `metrics` key
- Frontend `passesFilter()` uses early return `if (!item.metrics) return true`
- `FactorScores` new fields are `omitempty` → old 6-field JSON still round-trips cleanly

### [praise] Clean ParameterSnapshot lifetime management

Captured once per session in `buildParameterSnapshot()` and shared via pointer across all outcomes — correct for read-only parameter state at the time of run.

---

## Phase 4: Decision

**Approve** — no blocking issues. Two items to address before shipping:

1. Fix the TypeScript `parameter_snapshot` type from `string | null` to `ParameterSnapshot | null` in the auto-generated type files
2. Consider deduplicating provenance logic or documenting that `ModulateRecommendations()` is legacy-only

**Pre-existing condition**: `TestCycleTracker_GetContinuousPhaseScore_LowConfidence` in `internal/industry` fails independently (config migration, not related to this PR).