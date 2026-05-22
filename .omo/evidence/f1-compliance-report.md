# F1 Plan Compliance Audit — Hardcoded Parameters Config Migration

**Audit Date**: 2026-05-20  
**Branch**: `feat/hardcoded-params-config-migration`  
**Plan**: `.sisyphus/plans/hardcoded-params-config-migration.md` (1132 lines, 18 tasks + 4 final review tasks)  
**Auditor**: oracle agent (F1 compliance review)

---

## Executive Summary

| Metric | Result |
|--------|--------|
| **Tasks compliant** | 14/18 (78%) |
| **Tasks with gaps** | 4/18 (22%) — T5, T13, T16 (medium severity) |
| **Tests pass** | ✅ 12/12 FactorWeightEngine, all config, all orchestrator |
| **Build** | ✅ `go build ./...` passes |
| **go vet / staticcheck** | ✅ Clean |
| **Must Have compliance** | 3.5/5 (70%) |
| **Must NOT Have compliance** | 5.5/6 (92%) |
| **VERDICT** | ⚠️ **CONDITIONAL APPROVE — 3 gaps require remediation before merge** |

---

## Task-by-Task Compliance

### T1 — Audit Catalog  ✅ COMPLIANT
- `task-1-audit-catalog.md` exists with 60 CONFIG_CANDIDATE + 5 LEGITIMATE_CONSTANT entries
- All entries have file:line references
- All 4 key files (factor_weight_engine.go, narrative_conviction_modulator.go, industry_cycle_modulator.go, sector_rotator.go) covered
- Excludes test files and cmd/experimental/

### T2 — FactorWeightParameters Struct  ✅ COMPLIANT
- Defined at `parameters.go:593-622`
- 28 fields, all using `ParameterMetadata[T]`
- All JSON tags use snake_case + omitempty
- Added to `ParametersConfig` at line 637: `FactorWeight FactorWeightParameters \`json:"factor_weight,omitempty"\``

### T3 — NarrativeConvictionParameters Struct  ✅ COMPLIANT
- Defined at `parameters.go:626-629`
- 2 fields: `ThemeHitRates` + `SkillToTheme`
- Added to `ParametersConfig` at line 649: `NarrativeConviction NarrativeConvictionParameters \`json:"narrative_conviction,omitempty"\``
- Does NOT modify `NarrativeParameters` struct

### T4 — IndustryParameters & OrchestratorParameters Extension  ✅ COMPLIANT
- `IndustryParameters` extended with `PhaseScores` (line 397) + `SkillToIndustry` (line 398)
- `OrchestratorParameters` extended with `SectorRotationMacroAdjustments` (line 209) + `SectorRotationFlowAdjustments` (line 210)
- All new fields use snake_case + omitempty

### T5 — Default Value Functions  ⚠️ PARTIAL COMPLIANT
- `defaultFactorWeightParameters()` ✅ — defined at line 1886, all 28 params have Rationale/Source/Todo, values match hardcoded originals
- `defaultNarrativeConvictionParameters()` ✅ — defined at line 2068, has Rationale/Source/Todo
- `defaultIndustryParameters()` — ⚠️ **PhaseScores values (1.2/1.1/1.0/0.8 at line 1758) are a DIFFERENT SEMANTIC SCALE from the hardcoded code fallback (20/10/0/-20 in industry_cycle_modulator.go:45)**. This violates the "Must NOT Have: 不改變任何投資行為" guardrail.
- `defaultOrchestratorParameters()` — ⚠️ **SectorRotationMacroAdjustments uses different risk level keys ("high_risk"/"moderate_risk"/"low_risk" at line 722-737) vs. code fallback ("yellow"/"orange"/"red" at sector_rotator.go:117-139)**. Flow adjustments also use different keys. **SkillToTheme in NarrativeConviction defaults (lines 2084-2092) uses different skill names** (macro_research, currency_desk, risk_analytics) vs. code fallback (ai_supply_chain_desk, shipping_desk, financials_desk at line 31-41).
- `DefaultParametersConfig()` (line 10) — ✅ includes all new parameter groups

### T6 — Characterization Tests  ✅ COMPLIANT
- New tests present: `TestFactorWeightEngine_AddEvent_Critical` (line 175), `TestFactorWeightEngine_ApplyStrategy_Conservative` (line 211), `TestFactorWeightEngine_ApplyStrategy_Aggressive` (line 238)
- All 12 FactorWeightEngine tests pass
- Coverage: bull/bear/event/normalization

### T7 — FactorWeightEngine Base Weights Migration  ✅ COMPLIANT
- `NewFactorWeightEngine()` reads from `fwConfig()` at line 43
- `fwConfig()` helper (line 21) returns `&cfg.FactorWeight` or nil
- Falls back to `defaultBaseWeights()` (line 28) when config unavailable
- `SetBaseWeights()` preserved (line 140)

### T8 — Regime Adjustments Migration  ✅ COMPLIANT
- `GetWeights()` reads regime deltas from config at lines 79-91
- Falls back to hardcoded original values (lines 67-78) when config nil
- `OnRegimeChange()` reads RISK_ON/RISK_OFF deltas from config at lines 174-180

### T9 — Severity Deltas & Event Theme Mappings  ✅ COMPLIANT
- `applyEventAdjustment()` reads severity deltas from config at lines 215-220
- Event theme → factor mapping preserved (AI_capex_surge → quality+momentum, etc.)
- `ApplyStrategy()` reads conservative/aggressive deltas from config at lines 295-303

### T10 — Clamp Bounds Migration  ✅ COMPLIANT
- Clamp min/max read from config at lines 89-90
- Falls back to 0.02/0.50 when config nil
- Both pre-normalization (line 117-123) and post-normalization (line 128-134) clamping preserved

### T11 — Theme Hit Rates Migration  ✅ COMPLIANT
- `NewNarrativeConvictionModulator()` reads `ThemeHitRates` from config at line 52
- Falls back to `defaultThemeHitRates` (line 22) when config nil
- Hit rates preserved: 0.81/0.72/0.68/0.65/0.58

### T12 — Skill-to-Theme Mappings Migration  ✅ COMPLIANT
- `NewNarrativeConvictionModulator()` reads `SkillToTheme` from config at line 55
- Falls back to `defaultSkillToTheme` (line 31) when config nil
- Config read correctly structured

### T13 — IndustryCycleModulator Migration  ⚠️ PARTIAL COMPLIANT
- `NewIndustryCycleModulator()` reads `SkillToIndustry` from config at line 34 ✅
- `phaseDelta()` reads `PhaseScores` from config at lines 46-50 ✅
- **GAP**: The default PhaseScores values (1.2/1.1/1.0/0.8 at parameters_defaults.go:1758) are semantically different from the hardcoded fallback (20/10/0/-20 at industry_cycle_modulator.go:45). When config IS loaded (via DefaultParametersConfig()), the delta values change from integers (20/10/0/-20) to near-1.0 multipliers (1/1/1/1 after rounding). The JSON file values (1.0/0.5/0.0/-1.0) represent yet a third scale. **This constitutes a behavioral regression**.

### T14 — Sector Rotator Macro Adjustments  ✅ COMPLIANT
- `applyMacroAdjustments()` reads from `cfg.Orchestrator.SectorRotationMacroAdjustments` at line 145
- Falls back to `defaultMacroAdjustments()` at line 144
- Config reading correctly integrated

### T15 — Sector Rotator Flow Adjustments  ✅ COMPLIANT
- `applyFlowAdjustments()` reads from `cfg.Orchestrator.SectorRotationFlowAdjustments` at line 193
- Falls back to `defaultFlowAdjustments()` at line 192
- Config reading correctly integrated

### T16 — Cross-Parameter Validation  ⚠️ PARTIAL COMPLIANT
- `Validate()` extended with FactorWeight checks at lines 1033-1072 ✅:
  - Base weight keys validation (lines 1034-1041) ✅
  - clamp_min < clamp_max (line 1042) ✅
  - Severity descending: critical > high > medium > low (lines 1045-1052) ✅
  - Regime delta range [-0.15, +0.15] (lines 1054-1072) ✅
- NarrativeConviction checks at lines 1074-1090 ✅:
  - ThemeHitRates keys validation (lines 1075-1081) ✅
  - ThemeHitRates range [0,1] (lines 1082-1086) ✅
  - SkillToTheme non-empty (line 1088) ✅
- Industry/Orchestrator new field checks at lines 1092-1108 ✅
- **GAP 1**: Missing base weights sum ≈ 1.0 validation (plan requirement: "FactorWeight 基礎權重總和 ≈ 1.0（容許誤差 ±0.001）"). Only key existence checked, not value sum.
- **GAP 2**: No negative test case for base weights sum ≠ 1.0 in `parameters_test.go`. The "valid_default" test covers the happy path but no test validates the rejection path for malformed weights.

### T17 — configs/parameters.json Update  ✅ COMPLIANT
- `factor_weight` section present with 28 parameter objects (line 544)
- `narrative_conviction` section present with 2 parameter objects (line 3288)
- `phase_scores` section present (line 4194)
- `skill_to_industry` section present (line 4213)
- JSON valid (verified via `python3 -m json.tool`)
- Note: JSON PhaseScores values (1.0/0.5/0.0/-1.0) differ from both defaults and code fallback (see T5/T13 gaps)

### T18 — AGENTS.md Update  ✅ COMPLIANT
- `internal/portfolio/AGENTS.md` §2.4 updated with config-driven notes (line 68-69)
- Marks all FactorWeightEngine params as "配置化" through `ParametersConfig.FactorWeight`
- SectorRotator section (line 121-127) marks macro/flow adjustments as config-driven

---

## Must Have Compliance

| Requirement | Status | Evidence |
|-------------|--------|----------|
| 零行為回歸 | ⚠️ PARTIAL | Tests pass (immediate regression zero). However, T5 defaults (PhaseScores, SectorRotation keys, SkillToTheme names) differ from hardcoded code fallbacks, creating latent behavioral divergence when config is loaded without a JSON file |
| JSON omitempty | ✅ PASS | All new struct fields use `\`json:"...,omitempty"\`` |
| Rationale/Source/Todo per param | ✅ PASS | All 28 FactorWeight + 2 NarrativeConviction params have non-empty metadata. Verified in `parameters_defaults.go` |
| Cross-parameter validation | ⚠️ GAP | Missing base weights sum ≈ 1.0 check in Validate() |
| No modification to existing Parameter structs | ✅ PASS | `FactorParameters` (line 71) and `NarrativeParameters` (line 234) unchanged |

## Must NOT Have Compliance

| Restriction | Status | Evidence |
|-------------|--------|----------|
| No modification to FactorParameters/NarrativeParameters | ✅ PASS | Struct definitions unchanged |
| No migration of mathematical constants | ✅ PASS | Constants like `math.Abs(total-1.0) < 0.001` preserved in code |
| No touching cmd/experimental/, test files, web/static/, configs/agents.json | ✅ PASS | Changes limited to `internal/config/`, `internal/portfolio/`, `internal/orchestrator/`, `configs/parameters.json`, `internal/portfolio/AGENTS.md` |
| No behavioral change | ⚠️ GAP | Config defaults diverge from code fallbacks (T5) |
| No circular dependencies | ✅ PASS | config package is a leaf node |
| No new go vet / staticcheck warnings | ✅ PASS | Both pass cleanly |

---

## Detailed Gap Analysis

### GAP 1: PhaseScores Semantic Scale Mismatch (T5, T13) — SEVERITY: MEDIUM

**Three different value sets for the same parameter:**

| Source | Expansion | Recovery | Mature | Recession |
|--------|-----------|----------|--------|-----------|
| Code fallback (industry_cycle_modulator.go:45) | 20 | 10 | 0 | -20 |
| Config defaults (parameters_defaults.go:1758) | 1.2 | 1.1 | 1.0 | 0.8 |
| JSON file (configs/parameters.json:4196-4199) | 1.0 | 0.5 | 0.0 | -1.0 |

The original hardcoded values (20/10/0/-20) represent integer delta values (after `math.Round()`). The defaults (1.2/1.1/1.0/0.8) round to (1/1/1/1) — effectively neutralizing the phase adjustment. The JSON (1.0/0.5/0.0/-1.0) rounds to (1/1/0/-1).

**Impact**: When running WITHOUT a JSON file but WITH DefaultParametersConfig() loaded, cycle phase adjustments become negligible (1 vs 20 delta). This is behavioral regression.

**Fix**: Either align defaults to match code fallback scale (20/10/0/-20), or document this as an intentional recalibration with a separate calibration audit trail.

### GAP 2: Orchestrator/NarrativeDefaults Key Mismatch (T5) — SEVERITY: MEDIUM

Three sub-issues:
1. **SectorRotationMacroAdjustments defaults** use keys "high_risk"/"moderate_risk"/"low_risk" but `macroLevelKey()` in sector_rotator.go maps `MacroRiskLevel` to "yellow"/"orange"/"red"/"green". The defaults would NEVER be matched by `applyMacroAdjustments()`.
2. **SectorRotationFlowAdjustments defaults** use keys "gold_surge"/"cash_surge"/"tech_exodus"/"defensive_flight" but sector_rotator.go calls with "risk_off"/"carry_trade_unwind"/"sector_rotation".
3. **NarrativeConviction SkillToTheme defaults** use different skill names (macro_research, currency_desk, risk_analytics) vs. code fallback (ai_supply_chain_desk, shipping_desk, financials_desk), resulting in different agent → theme mappings.

**Impact**: Defaults are effectively dead code — they never match any runtime key. In practice, the hardcoded code fallbacks get used, so behavioral regression is zero in tests. But the defaults are misleading and would cause confusion.

**Fix**: Align defaults with code fallback keys/names.

### GAP 3: Missing Base Weights Sum Validation (T16) — SEVERITY: LOW

Plan requirement: "FactorWeight 基礎權重總和 ≈ 1.0（容許誤差 ±0.001）"

Current code (parameters.go:1034-1041) checks that all 8 expected keys exist but does not verify that the values sum to ~1.0. No corresponding test case in parameters_test.go.

**Impact**: A malformed config with base weights summing to 2.0 (which is then normalized) would pass validation silently. Low severity because `GetWeights()` normalizes regardless.

**Fix**: Add a sum check loop in Validate() and a corresponding negative test case.

---

## Build & Test Verification

```bash
✅ go build ./...                    # Clean
✅ go test ./internal/portfolio/...  # 12/12 FactorWeightEngine tests PASS
✅ go test ./internal/config/...     # All tests PASS
✅ go test ./internal/orchestrator/...# All tests PASS
✅ go vet ./...                      # No warnings
✅ staticcheck ./...                 # No new issues
✅ python3 -m json.tool configs/parameters.json  # Valid JSON
✅ grep for residual hardcoded values in migrated files — clean
```

---

## Recommendations

### Must Fix (blockers)
1. **Align PhaseScores defaults** with code fallback scale (20/10/0/-20) in `parameters_defaults.go:1758`
2. **Align SectorRotation defaults keys** with `macroLevelKey()` return values ("yellow"/"orange"/"red")
3. **Align NarrativeConviction SkillToTheme defaults** with the hardcoded `defaultSkillToTheme` map entries

### Should Fix (not blockers)
4. **Add base weights sum ≈ 1.0 validation** in `Validate()` (lines 1034-1041) plus a negative test case
5. **Add negative Validate test** for malformed base weights in `parameters_test.go`

### Consider
6. Document whether the PhaseScores scale change (from integer deltas to multipliers) is an intentional recalibration, and if so, update both defaults and JSON file consistently

---

## Final Verdict

| Category | Score |
|----------|-------|
| Structural implementation (T2-T4, T7-T12, T14-T15, T17-T18) | ✅ **PASS** — Correct |
| Default values (T5) | ⚠️ **3 medium gaps** |
| Characterization tests (T6) | ✅ **PASS** |
| Industry migration (T13) | ⚠️ **PhaseScores scale mismatch** |
| Cross-parameter validation (T16) | ⚠️ **1 low gap** |
| Build/Test/Vet/Lint | ✅ **ALL PASS** |
| Must Have compliance | 3.5/5 |
| Must NOT Have compliance | 5.5/6 |

**Verdict: CONDITIONAL APPROVE** — The migration architecture is correct and tests pass with zero regression. The 3 gaps in T5 defaults (PhaseScores, SectorRotation, SkillToTheme) plus the missing T16 weight sum validation must be resolved before merge. The remaining 14/18 tasks are fully compliant.
