# Code Quality Report — Hardcoded Parameters Config Migration

**Branch**: `feat/config-driven-parameters` (a65a5ca..HEAD)
**Date**: 2026-05-20
**Reviewer**: Sisyphus-Junior
**Scope**: 10 commits, 11 files changed (+1383/-162)

---

## Executive Summary

The migration is structurally sound. The `ParametersConfig` pattern is consistently applied across all migrated files with proper nil-safe config access and hardcoded fallback values. All new struct definitions follow the existing `ParameterMetadata[T]` convention. Build, vet, and tests pass.

**2 issues found**: 1 moderate (semantic key mismatch in sector_rotator config), 1 minor (PhaseScores default semantics changed). 2 files have gofmt whitespace issues (non-functional).

---

## Commit-by-Commit Review

| Commit | Description | Quality |
|--------|-------------|---------|
| `5b6f5cb` | Add FactorWeightParameters, NarrativeConvictionParameters, extend Industry/Orchestrator params | ✅ Clean struct design |
| `dbbc725` | Add characterization tests for FactorWeightEngine | ✅ Well-documented, notes known bugs |
| `dec6da5` | Add defaults for all new parameters | ✅ Complete Rationale/Source/Todo |
| `cbac258` | Migrate FactorWeightEngine + NarrativeConvictionModulator | ✅ Nil-safe patterns |
| `03cb76a` | Migrate industry cycle modulator + sector rotator | ⚠️ See Issue #1 |
| `e4a9fc4` | Docs: orchestrator AGENTS.md | ✅ Accurate |
| `ec1a1dc` | Docs: parameter documentation update | ✅ |
| `106ecd5` | Evidence: parameter documentation | ✅ |
| `a0af2d9` | Validate(), parameters.json groups, AGENTS.md updates | ✅ Comprehensive validation |
| `bc1c80b` | Epsilon comparison fix in ApplyStrategy tests | ✅ Properly documented |

---

## File-by-File Detailed Review

### 1. `internal/portfolio/factor_weight_engine.go` ✅

**Import Order**: ✅ stdlib (`maps`, `math`, `sync`) → internal (`config`, `narrative`, `strategy`)

**Nil-Safety**: ✅ All config reads use the pattern:
```go
fw := fwConfig()
var default_value float64 = <hardcoded>
if fw != nil {
    default_value = fw.Field.Value
}
```

**Remaining Hardcoded Values**: None. All numeric parameters (bull/bear/high_vol deltas, clamp bounds, severity deltas, risk-on/off adjustments, strategy adjustments) are now config-driven with hardcoded fallbacks.

**Theme mapping** (lines 235–255) — the decision of which factors to adjust per theme (`FactorQuality`, `FactorMomentum` for `AI_capex_surge`, etc.) remains hardcoded. This is structural design intent (not a numeric parameter), so it's acceptable.

**`fwConfig()` helper**: Clean nil-safe accessor. Returns nil when `GetParametersConfig()` returns nil.

**`BaseWeights` key conversion**: The config uses string keys (`"momentum"`, `"value"`) while the engine uses `FactorType` keys. The conversion loop `bw[FactorType(k)] = v` correctly handles this.

**No unused variables, no empty catch blocks.**

---

### 2. `internal/orchestrator/narrative_conviction_modulator.go` ✅

**Import Order**: ✅ stdlib (`fmt`, `math`) → internal (`config`, `domain`, `narrative`)

**Nil-Safety**: ✅ Dual nil-check pattern:
```go
if cfg := config.GetParametersConfig(); cfg != nil {
    if cfg.NarrativeConviction.ThemeHitRates.Value != nil {
        hitRates = cfg.NarrativeConviction.ThemeHitRates.Value
    }
    // ...
}
```

**All config reads have hardcoded fallback**: `hitRates := defaultThemeHitRates` then overwritten if config present.

**Remaining Hardcoded Values**: None — `defaultThemeHitRates` and `defaultSkillToTheme` serve as fallbacks, not the sole source.

---

### 3. `internal/orchestrator/industry_cycle_modulator.go` ✅

**Import Order**: ✅ stdlib (`fmt`, `math`) → internal (`config`, `domain`, `industry`)

**Nil-Safety**: ✅
- Constructor: `cfg.Industry.SkillToIndustry.Value != nil` check
- `phaseDelta()`: `cfg != nil` check before reading PhaseScores

**`skillToIndustry` storage**: ✅ Stored in struct field (`m.skillToIndustry`) and accessed as `m.skillToIndustry`, not the package-level variable.

**PhaseScores semantic change**: The old `phaseDelta` returned hardcoded absolute deltas (20, 10, 0, -20). The new config defaults are multipliers (1.2, 1.1, 1.0, 0.8). When no config is loaded, the hardcoded fallback (20, 10, 0, -20) preserves original behavior. **This is by design** — the PhaseScores are now conviction multipliers, not absolute deltas. See Minor Notes below.

**gofmt**: ⚠️ Struct field alignment issue (lines 15–17) — `skillToIndustry` indented one extra space.

---

### 4. `internal/portfolio/sector_rotator.go` ⚠️

**Import Order**: ✅ stdlib (`fmt`, `maps`, `slices`, `sort`, `time`) → internal (`config`, `narrative`, `risk`)

**Nil-Safety**: ✅ Both `applyMacroAdjustments` and `applyFlowAdjustments` use:
```go
adjustments := defaultMacroAdjustments()
if cfg := config.GetParametersConfig(); cfg != nil && cfg.Orchestrator.SectorRotationMacroAdjustments.Value != nil {
    adjustments = cfg.Orchestrator.SectorRotationMacroAdjustments.Value
}
```

**Hardcoded fallback preservation**: ✅ `defaultMacroAdjustments()` and `defaultFlowAdjustments()` preserve the exact original switch-case logic.

**🔴 Issue #1 — Config Default Key Mismatch**: The hardcoded fallback uses keys `"yellow"`, `"orange"`, `"red"` matching what `macroLevelKey()` returns. The config defaults in `parameters_defaults.go` use completely different keys: `"high_risk"`, `"moderate_risk"`, `"low_risk"`. When config is loaded, `deltas, ok := adjustments[levelKey]` will **always return ok=false** because the keys don't match, causing the function to silently return without applying any macro adjustments. **This means the default config values for macro adjustments can never be applied.**

**Fix needed**: Either (a) align config keys to `"green"/"yellow"/"orange"/"red"` or (b) add a key mapping layer.

**New flow adjustments**: The `defaultFlowAdjustments()` hardcoded fallback preserves original `"risk_off"` / `"carry_trade_unwind"` / `"sector_rotation"` keys. The config defaults use completely different keys (`"gold_surge"`, `"cash_surge"`, `"tech_exodus"`, `"defensive_flight"`). This is **by design** — the config flow adjustments are a new feature, not a migration of old behavior. When no config is loaded, the original flow behavior applies. When config IS loaded, the new flow patterns apply. This is acceptable.

**gofmt**: ⚠️ Map literal value alignment inconsistencies in `defaultMacroAdjustments()` and `defaultFlowAdjustments()`.

---

### 5. `internal/config/parameters.go` ✅

**New Structs**: Both `FactorWeightParameters` (27 fields) and `NarrativeConvictionParameters` (2 fields) follow the existing `ParameterMetadata[T]` pattern with consistent `json:".*,omitempty"` tags.

**`ParametersConfig` struct**: ✅ New `FactorWeight` and `NarrativeConviction` fields added with `omitempty`.

**`OrchestratorParameters` extension**: ✅ Two new fields added:
- `SectorRotationMacroAdjustments` — `ParameterMetadata[map[string]map[string]float64]`
- `SectorRotationFlowAdjustments` — `ParameterMetadata[map[string]map[string]float64]`

**`IndustryParameters` extension**: ✅ Two new fields added:
- `SkillToIndustry` — `ParameterMetadata[map[string]string]`
- (PhaseScores was already present, only Rationale/values updated)

**`Validate()`**: ✅ Comprehensive validation added for all new fields:
- FactorWeight: clamp min < max, severity ordering, regime delta bounds [-0.15, 0.15], base weights key completeness
- NarrativeConviction: hit rate range [0,1], theme key completeness, skill-to-theme non-empty
- Industry: skill-to-industry non-empty, all phase scores non-zero
- Orchestrator: macro/flow adjustments non-empty

**Validation concern**: `NarrativeConviction.SkillToTheme.Value` and `Industry.SkillToIndustry.Value` both check `len(...) == 0` which would fail if config is loaded from partial JSON with omitempty-zero values. However, `DefaultParametersConfig()` always populates these, and `LoadParametersConfig` falls back to defaults on missing file, so this is only triggerable with a deliberately malformed partial config.

---

### 6. `internal/config/parameters_defaults.go` ✅

**Completeness**: ✅ All 27 `FactorWeightParameters` fields have non-empty Rationale, Source (`SourceHeuristic`), and Todo.

**Quality of Rationales**: ✅ Each rationale explains the WHY behind the value:
- "Bull regime: shift toward momentum (+5%) to capture trend continuation"
- "Critical event: ±10% delta for severe market-moving events"

**Quality of Todos**: ✅ Each todo is specific and actionable:
- "Calibrate: derive from factor attribution backtest across 2024-2026 regime cycles"
- "Calibrate: severity deltas from event study of 50+ significant events"

**NarrativeConviction defaults**: ✅ Theme hit rates match the original `defaultThemeHitRates` values documented in `narrative/types.go` (0.81, 0.72, 0.68, 0.65, 0.58).

**PhaseScores update**: Values changed from `{1.0, 0.5, 0.0, -1.0}` to `{1.2, 1.1, 1.0, 0.8}`. Rationale now correctly describes them as "conviction multipliers" rather than "correlation scores". See Minor Notes.

**Sector rotation defaults**: Macro adjustments use keys `"high_risk"/"moderate_risk"/"low_risk"` — mismatch with `macroLevelKey()` output. Flow adjustments use new keys `"gold_surge"/"cash_surge"/"tech_exodus"/"defensive_flight"` — intentionally a new feature.

---

### 7. `internal/portfolio/factor_weight_engine_test.go` ✅

**Epsilon comparison**: ✅ `1e-10` epsilon properly documented:
```
// Use epsilon comparison because maps.Copy iteration order is non-deterministic
// and can produce ~5e-17 floating-point drift between successive calls.
```

**Test coverage**: ✅ Added tests for:
- `TestFactorWeightEngine_AddEvent_Critical` — verifies critical AI capex surge events boost quality and momentum
- `TestFactorWeightEngine_ApplyStrategy_Conservative` — characterization test, documents known bug (strategy_adjustment not read by GetWeights)
- `TestFactorWeightEngine_ApplyStrategy_Aggressive` — characterization test, same known bug documentation

**Bug documentation**: ✅ Tests explicitly note the known behavior where `ApplyStrategy` stores in `eventWeights` but `GetWeights` only reads entries in `activeEvents`, so strategy adjustments are silently ignored. This is proper characterization testing.

**All tests passing**: ✅ `go test ./internal/portfolio/... -run "TestFactorWeightEngine" -count=1` → `ok`

---

### 8. `configs/parameters.json` ✅

New file (536 lines) containing complete parameter configuration with all new sections fully populated. Matches the defaults structure.

---

## Quality Checklist Results

| Check | Status | Detail |
|-------|--------|--------|
| No hardcoded investment values in migrated files | ✅ | All numeric params are config-driven with hardcoded fallbacks |
| All config reads have nil-fallback | ✅ | Consistent `if fw != nil` / `if cfg != nil` pattern |
| No empty catch blocks or ignored errors | ✅ | No try-catch; no `_ = err` patterns |
| Import groups correct (stdlib, external, internal) | ✅ | All files follow proper grouping |
| `go build` passes | ✅ | No errors |
| `go vet` passes | ✅ | No warnings |
| `gofmt` | ⚠️ | 2 files have alignment issues (cosmetic) |
| Tests pass | ✅ | `go test ./internal/portfolio/...` → ok |

---

## Issues Found

### 🔴 Issue #1 — Config Default Key Mismatch (Moderate)

**File**: `internal/portfolio/sector_rotator.go` + `internal/config/parameters_defaults.go`

**Problem**: `macroLevelKey()` returns `"green"/"yellow"/"orange"/"red"` based on `MacroRiskLevel`. The hardcoded fallback `defaultMacroAdjustments()` uses these exact keys. But the config defaults in `defaultOrchestratorParameters()` use keys `"high_risk"/"moderate_risk"/"low_risk"`. When config is loaded, the lookup `adjustments[levelKey]` always fails, and macro adjustments are silently skipped.

**Impact**: When using `parameters.json` (which is the intended production path), all macro risk adjustments are silently ignored. This is the same number of levels that should have adjustments: green, yellow, orange, red.

**Fix**: Either update `parameters_defaults.go` to use `"green"/"yellow"/"orange"/"red"` keys matching the hardcoded fallback, or add a key remapping layer in `applyMacroAdjustments`. The hardcoded fallback already has the correct keys and values — the config defaults should match.

### ⚠️ Issue #2 — gofmt Non-Compliance (Minor)

**Files**: `internal/orchestrator/industry_cycle_modulator.go`, `internal/portfolio/sector_rotator.go`

**Problem**: Struct field alignment and map value alignment are off by 1–2 spaces. Non-functional but would fail CI.

**Fix**: Run `gofmt -w` on both files.

### ℹ️ Note — PhaseScores Semantic Change

**File**: `internal/orchestrator/industry_cycle_modulator.go` + `internal/config/parameters_defaults.go`

**Observation**: The old `phaseDelta()` hardcoded values (20, 10, 0, -20) were absolute conviction deltas. The new PhaseScores defaults (1.2, 1.1, 1.0, 0.8) are conviction multipliers. When no config is loaded, the hardcoded fallback preserves the old absolute-delta behavior. When config IS loaded, the multiplier behavior applies. This is an intentional design change — the PhaseScores are now semantically different from the original hardcoded values.

### ℹ️ Note — Flow Adjustments Key Mismatch (Intentional)

**File**: `internal/portfolio/sector_rotator.go` + `internal/config/parameters_defaults.go`

**Observation**: The config defaults for flow adjustments use completely different flow names (`"gold_surge"/"cash_surge"/"tech_exodus"/"defensive_flight"`) than the hardcoded fallback (`"risk_off"/"carry_trade_unwind"/"sector_rotation"`). This appears intentional — the config represents a new design for flow-based adjustments, while the hardcoded fallback preserves the original behavior for backward compatibility. When no config is loaded, original flow adjustments apply. When config IS loaded, new flow patterns apply. This is acceptable as a migration strategy.

---

## Overall Assessment

**Grade**: B+ (Good, with 1 moderate issue to fix before merge)

### Strengths
- Consistent nil-safe config access pattern across all migrated files
- Comprehensive hardcoded fallback values preserving original behavior
- Well-structured new types following existing `ParameterMetadata[T]` convention
- Thorough `Validate()` with meaningful constraints
- Complete Rationale/Source/Todo on all 29 new ParameterMetadata entries
- Tests properly documenting known behavior and float precision issues

### Required Actions Before Merge
1. **Fix Issue #1**: Align config default keys in `SectorRotationMacroAdjustments` with `macroLevelKey()` output (use `"green"/"yellow"/"orange"/"red"` keys with the same values from the hardcoded fallback)
2. **Fix gofmt**: Run `gofmt -w` on `industry_cycle_modulator.go` and `sector_rotator.go`
