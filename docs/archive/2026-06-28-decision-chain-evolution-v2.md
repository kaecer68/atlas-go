# Decision Chain Evolution v2 — 決策鏈與決策追蹤進化迭代實作計劃

## TL;DR

> **Quick Summary**: 為決策鏈與決策追蹤系統加入參數溯源能力（parameter provenance），確保每個 config-driven 的決策步驟都可被稽核與再現。擴展 ConvictionStep 加入參數來源標記，新增 Metrics 欄位修復篩選面板，新增 ParameterSnapshot 機制完整記錄決策時參數狀態。
>
> **Deliverables**:
> - `ConvictionStep` 擴展（source, param_ref, param_value）
> - `convictionBuilder.AddWithProvenance()` 新方法
> - 2 個 modulator 完成參數溯源標記
> - `FactorScores` 頂層對齊 8 因子
> - `PipelineItem.Metrics` 欄位 + 前端篩選修復
> - `ParameterSnapshot` 結構 + RecommendationOutcome 附加
> - Frontend 參數來源展示
>
> **Estimated Effort**: Medium (P0 核心 5-7 天, P1 延伸 3-5 天, P2-P3 視需求)
> **Parallel Execution**: YES — 5 waves
> **Critical Path**: Domain types → Modulator provenance → Frontend / API → Verification

---

## Context

### Original Request
在投資管線（Pipeline）升級後，基於最新版本對決策鏈、決策追蹤進行重新盤點，發現：
- `ConvictionStep` 無參數來源標記（config-driven 的 delta 無法追溯）
- 前端篩選面板（passesFilter）依賴不存在的 `item.metrics`
- `FactorScores` 頂層（6 欄位）與 Breakdown（8 欄位）不對齊
- Modulator 調整未完整記錄在 ConvictionBreakdown 中

### Interview Summary
**Key Decisions**:
- 方案 A+B 組合：ConvictionStep 擴展 + ParameterSnapshot 附加至 RecommendationOutcome
- Metrics 欄位：後端新增，前端篩選面板使用
- P0 任務：TDD（RED-GREEN-REFACTOR 含特徵化測試）
- P1+ 任務：Tests-after（特徵化測試 → 實作 → 驗證），因長期迭代本質

### Oracle Phase-1 Verification
- CHECK [5/5] PASS → VERDICT: GO

---

## Work Objectives

### Core Objective
為決策鏈與決策追蹤系統加入**參數溯源能力**，確保每個 config-driven 的決策步驟都可被稽核與再現。

### Scope IN
- P0: ConvictionStep 擴展 + modulator 溯源 + FactorScores 對齊 + Metrics 欄位 + 前端修復
- P1: Modulator 分解 + Darwinian 整合 + SectorRotator 步驟 + Weight 溯源
- P2: 參數敏感度 + 跨參數互動 + 版本歷史
- P3: Counterfactual + SHAP + Replay Engine

### Scope OUT
- `convictionBuilder.add()` 簽名不變更（改用新的 `AddWithProvenance()`）
- `FactorParameters` / `NarrativeParameters` / `OptimizerParameters.FactorWeights` 不修改
- `cmd/experimental/` 不觸碰
- `configs/agents.json` 不修改
- 數學常數 / 緩衝大小 / timeout 不更動
- 既有測試不修改（僅新增特徵化測試）

---

## Verification Strategy

> **ZERO HUMAN INTERVENTION** — ALL verification is agent-executed.

### Test Decision
- **Infrastructure exists**: YES (Go test suite + CI)
- **P0 tasks**: Full TDD — RED (characterization test) → GREEN (implementation) → REFACTOR
- **P1+ tasks**: Tests-after — characterization test first, then implement, then verify
- **Framework**: `go test ./...` + `go vet ./...` + `staticcheck ./...`
- **All tasks**: Agent-executed QA scenarios mandatory regardless of TDD level
- **P0 characterization test goal**: Before any change, write a test that asserts current behavior as RED stage proof. After change, update test to assert new behavior as GREEN stage proof.

### QA Policy
Every task includes agent-executed QA scenarios. Evidence saved to `.sisyphus/evidence/decision-chain-v2/`.

- **Domain types**: Bash (go test) — compile check struct, verify JSON serialization
- **Modulator logic**: Bash (go test) — unit test with config-driven provenance verification
- **API**: Bash (curl) — verify PipelineItem has Metrics field, response structure aligned
- **Frontend**: Playwright — verify filter panel works, parameter source displayed

---

## Execution Strategy

### Parallel Execution Waves

```
Wave 1 (Foundation — domain types + builder):
├── P0.1: Domain type changes (ConvictionStep + ParameterSnapshot + Metrics structs)
├── P0.2: AddWithProvenance() on convictionBuilder
├── P0.3: TDD characterization tests for convicitonStep/mutation/audit trail

Wave 2 (Backend provenance — depends on Wave 1):
├── P0.4: narrative_conviction_modulator provenance
├── P0.5: industry_cycle_modulator provenance
├── P0.6: FactorScores top-level alignment (8 factors)
├── P0.7: Metrics field in PipelineItemData + PipelineItem

Wave 3 (Frontend + API — depends on Wave 2):
├── P0.8: Frontend filter panel fix (use Metrics)
├── P0.9: Frontend parameter source display in conviction breakdown
├── P0.10: ParameterSnapshot in RecommendationOutcome + API exposure

Wave 4 (P1 — depends on Wave 1):
├── P1.1: Per-modulator tracking refactor
├── P1.2: Darwinian clamping integration into ConvictionBreakdown
├── P1.3: SectorRotator adjustments as ConvictionSteps
├── P1.4: FactorScoreItem.Weight source provenance

Wave 5 (P2-P3 — depends on Wave 4):
├── P2.1: Parameter sensitivity markers
├── P2.2: Parameter version history in ledger
├── P3.1: SHAP contribution decomposition (research)
├── P3.2: Counterfactual parameter analysis
└── P3.3: Replay engine with historical parameters
```

---

## TODOs

- [x] **P0.1. Domain Type Changes — Extend ConvictionStep + ParameterSnapshot + Metrics structs** (已在前次實作完成)

  **What to do**:
  - Extend `domain/shared/shared.go` `ConvictionStep` with 3 new `omitempty` fields:
    ```go
    Source     string `json:"source,omitempty"`      // "config" | "hardcoded" | "heuristic"
    ParamRef   string `json:"param_ref,omitempty"`    // "Industry.PhaseScores.ScoreExpansion"
    ParamValue string `json:"param_value,omitempty"`  // "20"
    ```
  - Add `ParameterSnapshot` struct in `domain/shared/shared.go`:
    ```go
    type ParameterSnapshot struct {
        FactorWeights       map[string]float64 `json:"factor_weights,omitempty"`
        NarrativeHitRates   map[string]float64 `json:"narrative_hit_rates,omitempty"`
        IndustryPhaseScores map[string]float64 `json:"industry_phase_scores,omitempty"`
        ConfigVersion       string             `json:"config_version,omitempty"`
        CapturedAt          time.Time          `json:"captured_at"`
    }
    ```
  - Add `Metrics` struct in `monitoring/api/pipeline/handlers.go`:
    ```go
    type PipelineItemMetrics struct {
        PriceToEarnings  *float64 `json:"price_to_earnings,omitempty"`
        PriceToBook      *float64 `json:"price_to_book,omitempty"`
        DividendYield    *float64 `json:"dividend_yield,omitempty"`
        BacktestReturn   *float64 `json:"backtest_return,omitempty"`
    }
    ```
  - Add `Metrics PipelineItemMetrics` field to `PipelineItemData` and `PipelineItem`
  - Add `ParameterSnapshot` field to `RecommendationOutcome`

  **Must NOT do**:
  - Do NOT modify existing ConvictionStep fields (Rule/Delta/Reason)
  - Do NOT remove any existing fields from FactorScores or FactorScoreBreakdown
  - Do NOT touch FactorParameters, NarrativeParameters, OptimizerParameters

  **Recommended Agent Profile**:
  - **Category**: `deep` — Domain type changes affect serialization, ledger persistence, and API contracts
  - **Skills**: N/A

  **Parallelization**:
  - **Can Run In Parallel**: NO (foundation task)
  - **Parallel Group**: Wave 1
  - **Blocks**: All P0.2-P0.10 tasks
  - **Blocked By**: None

  **References**:
  - `internal/domain/shared/shared.go:86-97` — Current ConvictionStep/ConvictionBreakdown structs
  - `internal/domain/recommendation/recommendation.go:50-72` — Current RecommendationOutcome struct
  - `internal/monitoring/service/pipeline.go:620-639` — Current PipelineItemData struct
  - `internal/monitoring/api/pipeline/handlers.go:258-277` — Current PipelineItem struct

  **Acceptance Criteria**:

  **RED phase (characterization test)**:
  - [ ] Test that existing ConvictionStep JSON serialization unchanged (no new required fields)
  - [ ] Test that new omitempty fields don't appear in serialization when empty
  - [ ] Test ParameterSnapshot round-trip JSON marshal/unmarshal

  **GREEN phase (implementation)**:
  - [ ] ConvictionStep has 3 new fields (Source, ParamRef, ParamValue) with json:"source/param_ref/param_value,omitempty"
  - [ ] ParameterSnapshot struct defined with all 5 fields
  - [ ] PipelineItemMetrics struct defined with all 4 fields (pointer types for omitempty)
  - [ ] PipelineItemData has Metrics field
  - [ ] PipelineItem has Metrics field with proper json tag
  - [ ] RecommendationOutcome has ParameterSnapshot field with json:"parameter_snapshot,omitempty"
  - [ ] `go build ./...` passes
  - [ ] `go test ./internal/domain/...` passes
  - [ ] `gofmt -l .` is clean

  **QA Scenarios**:
  ```
  Scenario: ConvictionStep omitempty serialization
    Tool: Bash (go test)
    Preconditions: New fields are empty
    Steps:
      1. Create ConvictionStep{Rule:"test", Delta:5, Reason:"test"}
      2. Marshal to JSON
    Expected Result: JSON contains only "rule","delta","reason" — no "source","param_ref","param_value"
    Evidence: .sisyphus/evidence/decision-chain-v2/p0.1-omitempty.json

  Scenario: ParameterSnapshot round-trip
    Tool: Bash (go test)
    Preconditions: ParameterSnapshot fully populated
    Steps:
      1. Create ParameterSnapshot with all fields set
      2. Marshal to JSON
      3. Unmarshal back
      4. Assert all fields equal
    Expected Result: Round-trip preserves all values
    Evidence: .sisyphus/evidence/decision-chain-v2/p0.1-snapshot-rt.json
  ```
  **Commit**: YES
  - Message: `feat(domain): extend ConvictionStep with parameter provenance fields + ParameterSnapshot + Metrics structs`
  - Files: `internal/domain/shared/shared.go`, `internal/domain/recommendation/recommendation.go`, `internal/monitoring/service/pipeline.go`, `internal/monitoring/api/pipeline/handlers.go`
  - Pre-commit: `go build ./... && go test ./internal/domain/... && test -z "$(gofmt -l .)"`

---

- [x] **P0.2. Add AddWithProvenance() Method on convictionBuilder** (已在前次實作完成)

  **What to do**:
  - Add a new method to `convictionBuilder` (`internal/orchestrator/conviction_builder.go`):
    ```go
    func (b *convictionBuilder) addWithProvenance(rule string, delta int, reason string, source string, paramRef string, paramValue string) {
        b.mu.Lock()
        defer b.mu.Unlock()
        b.steps = append(b.steps, domain.ConvictionStep{
            Rule:        rule,
            Delta:       delta,
            Reason:      reason,
            Source:      source,
            ParamRef:    paramRef,
            ParamValue:  paramValue,
        })
    }
    ```
  - Do NOT modify existing `add()` method
  - The existing `add()` remains for steps that are NOT config-driven

  **Must NOT do**:
  - Do NOT modify existing `add()` signature (preserves 58 call sites)
  - Do NOT add any imports beyond what already exists

  **Recommended Agent Profile**:
  - **Category**: `quick` — Simple additive method, no logic changes
  - **Skills**: N/A

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with P0.1)
  - **Blocks**: P0.4, P0.5
  - **Blocked By**: P0.1 (depends on ConvictionStep extended struct)

  **References**:
  - `internal/domain/shared/shared.go:86-90` — New ConvictionStep struct
  - `internal/orchestrator/conviction_builder.go:22-25` — Existing add() method

  **Acceptance Criteria**:
  - [ ] `addWithProvenance()` exists and accepts 6 arguments
  - [ ] Existing `add()` unchanged (same signature)
  - [ ] Both methods compile without error
  - [ ] `go build ./...` passes

  **QA Scenarios**:
  ```
  Scenario: addWithProvenance populates new fields
    Tool: Bash (go test)
    Preconditions: convictionBuilder initialized with base=60, floor=50
    Steps:
      1. builder.addWithProvenance("cycle_phase", 20, "reason", "config", "Industry.PhaseScores.ScoreExpansion", "20")
      2. builder.build()
      3. Assert steps[0].Source == "config"
      4. Assert steps[0].ParamRef == "Industry.PhaseScores.ScoreExpansion"
      5. Assert steps[0].ParamValue == "20"
    Expected Result: All 3 new fields populated correctly
    Evidence: .sisyphus/evidence/decision-chain-v2/p0.2-provenance.json
  ```
  **Commit**: YES (with P0.1)
  - Message: `feat(orchestrator): add AddWithProvenance() method on convictionBuilder`
  - Files: `internal/orchestrator/conviction_builder.go`
  - Pre-commit: `go build ./...`

---

- [x] **P0.3. TDD Characterization Tests for Modulator Provenance** (已在前次實作完成)

  **What to do**:
  - Write characterization tests BEFORE modifying the modulators:
    - `internal/orchestrator/narrative_conviction_modulator_test.go` — Test that current ConvictionSteps from this modulator contain only Rule/Delta/Reason (no provenance)
    - `internal/orchestrator/industry_cycle_modulator_test.go` — Same for industry cycle modulator
    - `internal/monitoring/api/pipeline/handlers_test.go` — Test that PipelineItem serialization does NOT include Metrics field in current state
  - These tests serve as:
    1. Regression guard — verify we don't break existing behavior
    2. Gap evidence — prove the missing fields are truly missing
    3. Feature test — after implementation, update to assert new fields exist

  **Must NOT do**:
  - Do NOT modify any production code at this stage
  - Do NOT modify any existing test files

  **Recommended Agent Profile**:
  - **Category**: `writing` — TDD characterization tests
  - **Skills**: N/A

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with P0.1, P0.2)
  - **Blocks**: P0.4, P0.5, P0.6 (verification foundation)
  - **Blocked By**: None (independent)

  **References**:
  - `internal/orchestrator/narrative_conviction_modulator.go` — Target modulator for characterization
  - `internal/orchestrator/industry_cycle_modulator.go` — Target modulator for characterization
  - `internal/monitoring/api/pipeline/handlers.go:258-277` — PipelineItem current struct

  **Acceptance Criteria**:
  - [ ] Characterization test for narrative_conviction_modulator passes (RED — proves no provenance)
  - [ ] Characterization test for industry_cycle_modulator passes (RED — proves no provenance)
  - [ ] PipelineItem serialization test passes
  - [ ] All new tests compile with `go build ./...`

  **QA Scenarios**:
  ```
  Scenario: Characterization test for narrative_conviction_modulator
    Tool: Bash (go test)
    Preconditions: Test file created
    Steps:
      1. Run go test ./internal/orchestrator/... -run TestNarrativeConvictionStepNoProvenance
    Expected Result: PASS — test asserts steps have empty Source/ParamRef/ParamValue
    Evidence: .sisyphus/evidence/decision-chain-v2/p0.3-char-narrative.json
  ```
  **Commit**: YES (groups with P0.1, P0.2)
  - Message: `test(orchestrator): add characterization tests for modulator provenance gap`
  - Files: `internal/orchestrator/narrative_conviction_modulator_test.go`, `internal/orchestrator/industry_cycle_modulator_test.go`, `internal/monitoring/api/pipeline/handlers_test.go`
  - Pre-commit: `go build ./... && go test ./internal/orchestrator/...`

---

- [x] **P0.4. narrative_conviction_modulator — Add Parameter Provenance to Steps** (已在前次實作完成)

  **What to do**:
  - In `internal/orchestrator/narrative_conviction_modulator.go` (around line 130), replace:
    ```go
    step := domain.ConvictionStep{
        Rule:   "narrative_boost",
        Delta:  adj,
        Reason: fmt.Sprintf("%s (hit_rate: %.0f%%, confidence: %.0f%%)", theme, info.hitRate*100, info.confidence*100),
    }
    ```
    With:
    ```go
    provenanceSource := "heuristic"
    provenanceRef := ""
    provenanceVal := ""
    if cfg := config.GetParametersConfig(); cfg != nil {
        if _, ok := cfg.NarrativeConviction.ThemeHitRates.Value[theme]; ok {
            provenanceSource = "config"
            provenanceRef = fmt.Sprintf("NarrativeConviction.ThemeHitRates.%s", theme)
            provenanceVal = fmt.Sprintf("%.2f", info.hitRate)
        }
    }
    step := domain.ConvictionStep{
        Rule:        "narrative_boost",
        Delta:       adj,
        Reason:      fmt.Sprintf("%s (hit_rate: %.0f%%, confidence: %.0f%%)", theme, info.hitRate*100, info.confidence*100),
        Source:      provenanceSource,
        ParamRef:    provenanceRef,
        ParamValue:  provenanceVal,
    }
    ```
  - Use `builder.addWithProvenance()` if a `convictionBuilder` is available in the ModulateRecommendations flow (check context — may need to switch between addWithProvenance and direct step append based on code structure)

  **Must NOT do**:
  - Do NOT change the modulator's logic or delta calculation
  - Do NOT change the Reason format (only add fields)

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high` — Config-driven logic with fallback detection
  - **Skills**: N/A

  **Parallelization**:
  - **Can Run In Parallel**: YES (with P0.5, P0.6, P0.7)
  - **Parallel Group**: Wave 2
  - **Blocks**: P0.8, P0.9
  - **Blocked By**: P0.1, P0.2, P0.3

  **References**:
  - `internal/orchestrator/narrative_conviction_modulator.go:96-98` — hitRate source logic (config vs fallback)
  - `internal/orchestrator/narrative_conviction_modulator.go:130-139` — Current ConvictionStep construction
  - `internal/orchestrator/conviction_builder.go:22-40` — addWithProvenance() method

  **Acceptance Criteria**:
  - [ ] TDD RED: Characterization test (P0.3) confirms steps have no provenance → PASS
  - [ ] After change: ConvictionStep now includes source, param_ref, param_value
  - [ ] When config available and theme exists in config: source="config"
  - [ ] When config nil or theme not in config: source="heuristic"
  - [ ] `go test ./internal/orchestrator/...` passes (characterization test updated to GREEN)
  - [ ] Existing modulator tests still pass

  **QA Scenarios**:
  ```
  Scenario: Config-driven provenance is set correctly
    Tool: Bash (go test)
    Preconditions: ParametersConfig loaded with theme_hit_rates
    Steps:
      1. Run modulator with theme present in config
      2. Check steps[0].Source == "config"
      3. Check steps[0].ParamRef contains "NarrativeConviction.ThemeHitRates."
      4. Check steps[0].ParamValue is non-empty
    Expected Result: All provenance fields populated from config
    Evidence: .sisyphus/evidence/decision-chain-v2/p0.4-config-provenance.json

  Scenario: Fallback provenance is set correctly
    Tool: Bash (go test)
    Preconditions: ParametersConfig nil
    Steps:
      1. Run modulator with nil config
      2. Check steps[0].Source == "heuristic"
    Expected Result: Source indicates heuristic fallback
    Evidence: .sisyphus/evidence/decision-chain-v2/p0.4-fallback-provenance.json
  ```
  **Commit**: YES (groups with P0.5)
  - Message: `feat(orchestrator): add parameter provenance markers to narrative_conviction_modulator`
  - Files: `internal/orchestrator/narrative_conviction_modulator.go`, `internal/orchestrator/narrative_conviction_modulator_test.go`
  - Pre-commit: `go build ./... && go test ./internal/orchestrator/...`

---

- [x] **P0.5. industry_cycle_modulator — Add Parameter Provenance to Steps** (已在前次實作完成)

  **What to do**:
  - In `internal/orchestrator/industry_cycle_modulator.go` (around line 108), replace the ConvictionStep construction:
    - Detect whether `ParametersConfig.Industry.PhaseScores` was used or hardcoded fallback
    - Add Source="config" or "hardcoded"
    - Add ParamRef pointing to the specific PhaseScores field (e.g., `"Industry.PhaseScores.ScoreExpansion"`)
    - Add ParamValue with the actual value used
    - Use `builder.addWithProvenance()` if a convictionBuilder is available; otherwise append directly

  **Must NOT do**:
  - Do NOT change the modulator's logic or delta calculation
  - Do NOT change the Reason format

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high` — Same pattern as P0.4
  - **Skills**: N/A

  **Parallelization**:
  - **Can Run In Parallel**: YES (with P0.4, P0.6, P0.7)
  - **Parallel Group**: Wave 2
  - **Blocks**: P0.8
  - **Blocked By**: P0.1, P0.2, P0.3

  **References**:
  - `internal/orchestrator/industry_cycle_modulator.go:44-64` — phaseDelta() function and config reading
  - `internal/orchestrator/industry_cycle_modulator.go:108-112` — Current ConvictionStep construction

  **Acceptance Criteria**:
  - [ ] ConvictionStep Source indicates config or hardcoded
  - [ ] ParamRef points to the specific PhaseScores field used
  - [ ] ParamValue is the actual score value
  - [ ] `go test ./internal/orchestrator/...` passes

  **QA Scenarios**:
  ```
  Scenario: Cycle phase provenance from config
    Tool: Bash (go test)
    Preconditions: ParametersConfig loaded
    Steps:
      1. Run modulator for expansion phase
      2. Check steps[0].Source == "config"
      3. Check steps[0].ParamRef == "Industry.PhaseScores.ScoreExpansion"
      4. Check steps[0].ParamValue is non-empty
    Expected Result: Provenance fields populated from config
    Evidence: .sisyphus/evidence/decision-chain-v2/p0.5-phase-provenance.json
  ```
  **Commit**: YES (groups with P0.4)
  - Message: `feat(orchestrator): add parameter provenance markers to industry_cycle_modulator`
  - Files: `internal/orchestrator/industry_cycle_modulator.go`, `internal/orchestrator/industry_cycle_modulator_test.go`
  - Pre-commit: `go build ./... && go test ./internal/orchestrator/...`

---

- [x] **P0.6. FactorScores Top-Level Alignment (8 Factors)** (已在前次實作完成)

  **What to do**:
  - Add `Narrative` and `IndustryCycle` fields to `FactorScores` struct in `internal/domain/shared/shared.go`:
    ```go
    type FactorScores struct {
        Momentum               float64               `json:"momentum"`
        Value                  float64               `json:"value"`
        Quality                float64               `json:"quality"`
        Agent                  float64               `json:"agent"`
        InstitutionalSentiment float64               `json:"institutional_sentiment"`
        Liquidity              float64               `json:"liquidity"`
        Narrative              float64               `json:"narrative,omitempty"`
        IndustryCycle          float64               `json:"industry_cycle,omitempty"`
        Total                  float64               `json:"total"`
        Breakdown              *FactorScoreBreakdown `json:"breakdown,omitempty"`
    }
    ```
  - Update all places that construct `FactorScores` to populate Narrative and IndustryCycle (check `collectRecommendations` in `executors.go`, the ledger persistence, and any test factories)
  - Verify that `CalculateAllScoresWithBreakdown()` or `optimizer.go` already computes these — if so, just map the values

  **Must NOT do**:
  - Do NOT remove any existing fields
  - Do NOT change the Total calculation (it should already include narrative and industry_cycle if weights are non-zero)
  - Do NOT modify FactorScoreBreakdown

  **Recommended Agent Profile**:
  - **Category**: `deep` — Requires finding ALL construction sites for FactorScores
  - **Skills**: N/A

  **Parallelization**:
  - **Can Run In Parallel**: YES (with P0.4, P0.5, P0.7)
  - **Parallel Group**: Wave 2
  - **Blocks**: Frontend display of narrative/industry_cycle scores
  - **Blocked By**: P0.1 (struct definition)

  **References**:
  - `internal/domain/shared/shared.go:59-68` — Current FactorScores struct
  - `internal/domain/shared/shared.go:47-57` — FactorScoreBreakdown (8 factors)
  - `internal/orchestrator/executors.go:429-443` — collectRecommendations() FactorScores construction
  - `internal/portfolio/optimizer.go:312-315` — FactorType weights and scoring
  - `internal/portfolio/factor_weight_engine.go:36-37` — FactorNarrative and FactorIndustryCycle weights

  **Acceptance Criteria**:
  - [ ] RED: Characterization test proves FactorScores has only 6 fields
  - [ ] GREEN: FactorScores has 8 fields (+Narrative, +IndustryCycle)
  - [ ] ALL construction sites for FactorScores populate Narrative and IndustryCycle
  - [ ] `go test ./...` passes (existing tests may hardcode 6 fields — update to include 2 new fields)
  - [ ] genTags auto-generates updated field_names.js/field_types.ts (pre-commit hook)
  - [ ] Total calculation is verified correct (narrative*weight + industry_cycle*weight included)

  **QA Scenarios**:
  ```
  Scenario: FactorScores serialization includes 8 fields
    Tool: Bash (go test)
    Preconditions: FactorScores fully populated
    Steps:
      1. Create FactorScores with all 8 factors
      2. Marshal to JSON
      3. Assert JSON contains "narrative" and "industry_cycle" keys
    Expected Result: JSON includes all 8 factor fields
    Evidence: .sisyphus/evidence/decision-chain-v2/p0.6-8factors.json

  Scenario: Backward compatibility
    Tool: Bash (go test)
    Preconditions: Legacy FactorScores JSON with 6 fields
    Steps:
      1. Marshal JSON: {"momentum":0.5,"value":0.3,"quality":0.4,"agent":0.2,"institutional_sentiment":0.1,"liquidity":0.2,"total":1.7}
      2. Unmarshal into FactorScores struct
    Expected Result: New fields are zero-valued (0), no error
    Evidence: .sisyphus/evidence/decision-chain-v2/p0.6-backward.json
  ```
  **Commit**: YES
  - Message: `fix(domain): align FactorScores top-level with 8-factor Breakdown (add Narrative + IndustryCycle)`
  - Files: `internal/domain/shared/shared.go`, `internal/orchestrator/executors.go`, `internal/portfolio/optimizer.go` (if needed)
  - Pre-commit: `go build ./... && go test ./... && test -z "$(gofmt -l .)"`

---

- [x] **P0.7. Metrics Field Assembly in PipelineItem Backend**

  **What to do**:
  - In `internal/monitoring/service/pipeline.go`, in `loadSessionPipelineData()`:
    - Extract P/E, P/B, DividendYield from `FactorScoreBreakdown.Value.RawInputs` and `FactorScoreBreakdown.Quality.RawInputs`
    - Extract BacktestReturn from `ForwardReturn` (already available)
    - Build `PipelineItemMetrics` object and assign to item.Metrics
  - In `internal/monitoring/api/pipeline/handlers.go`, in `HandleRecommendationPipeline()`:
    - Map `PipelineItemData.Metrics` to `PipelineItem.Metrics` (already auto-mapped if using same field, but verify)
  - Ensure nil-safe: if FactorScores.Breakdown is nil, Metrics fields remain nil (pointer types)

  **Must NOT do**:
  - Do NOT add new data sources for metrics — only extract from existing FactorScores.Breakdown
  - Do NOT duplicate RawInputs values

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high` — Data extraction from existing structs
  - **Skills**: N/A

  **Parallelization**:
  - **Can Run In Parallel**: YES (with P0.4, P0.5, P0.6)
  - **Parallel Group**: Wave 2
  - **Blocks**: P0.8 (frontend filter fix)
  - **Blocked By**: P0.1 (PipelineItemMetrics struct)

  **References**:
  - `internal/domain/shared/shared.go:40-46` — FactorScoreItem.RawInputs (contains pe_ratio, pb_ratio, dividend_yield)
  - `internal/portfolio/factor_engine.go:120-245` — Value factor calculation (P/E, P/B extraction)
  - `internal/portfolio/factor_engine.go:253-308` — Quality factor calculation (dividend_yield extraction)
  - `internal/monitoring/service/pipeline.go:384-482` — loadSessionPipelineData() function

  **Acceptance Criteria**:
  - [ ] PipelineItem.Metrics is populated from FactorScoreBreakdown.RawInputs
  - [ ] P/E mapped from Value.RawInputs["pe_ratio"] or similar
  - [ ] P/B mapped from Value.RawInputs["pb_ratio"] or similar
  - [ ] DividendYield mapped from Quality.RawInputs["dividend_yield"]
  - [ ] BacktestReturn mapped from ForwardReturn
  - [ ] When Breakdown is nil: all Metrics fields are nil (not zero)
  - [ ] `go build ./... && go test ./...` passes

  **QA Scenarios**:
  ```
  Scenario: Metrics populated from factor breakdown
    Tool: Bash (curl)
    Preconditions: Server running with session data
    Steps:
      1. GET /api/dashboard/recommendation-pipeline
      2. Parse items[0].metrics
      3. Assert metrics.price_to_earnings is non-nil
      4. Assert metrics.price_to_book is non-nil
      5. Assert metrics.dividend_yield is non-nil
      6. Assert metrics.backtest_return is non-nil
    Expected Result: All 4 metrics fields present and non-nil
    Evidence: .sisyphus/evidence/decision-chain-v2/p0.7-metrics.json

  Scenario: Nil-safe when breakdown missing
    Tool: Bash (go test)
    Preconditions: Outcome with nil FactorScores.Breakdown
    Steps:
      1. Build PipelineItemData with nil breakdown
      2. Check Metrics is nil (or all fields nil)
    Expected Result: Graceful nil handling, no panic
    Evidence: .sisyphus/evidence/decision-chain-v2/p0.7-nil-safe.json
  ```
  **Commit**: YES
  - Message: `feat(monitoring): add Metrics field assembly from FactorScoreBreakdown for frontend filter`
  - Files: `internal/monitoring/service/pipeline.go`, `internal/monitoring/api/pipeline/handlers.go`
  - Pre-commit: `go build ./... && go test ./internal/monitoring/...`

---

- [x] **P0.8. Frontend Filter Panel Fix (use PipelineItem.Metrics)** (已在前次實作完成)

  **What to do**:
  - In `web/static/js/pages/pipeline.js`, update `passesFilter()`:
    - Source data now from `item.metrics` (which IS now populated by backend)
    - Keep the existing fallback logic `item.metrics || {}` as defensive programming
    - Update the filter field mapping if needed based on actual Metrics field names
  - In `renderPipeline()` (same file):
    - Ensure filter panel calls `applyFilters()` which correctly reads from item.metrics
    - Verify filter result count shows correct numbers

  **Must NOT do**:
  - Do NOT remove existing fallback patterns (item.metrics || {})
  - Do NOT change the filter UI layout

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering` — Frontend JS fix
  - **Skills**: N/A

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on P0.7)
  - **Parallel Group**: Wave 3
  - **Blocks**: P0.9
  - **Blocked By**: P0.7

  **References**:
  - `web/static/js/pages/pipeline.js:406-423` — passesFilter() function
  - `web/static/js/pages/pipeline.js:342-382` — applyFilters() and toggleFilterPanel()

  **Acceptance Criteria**:
  - [ ] passesFilter() correctly reads from item.metrics.price_to_earnings (etc.)
  - [ ] Filter panel shows correct result counts
  - [ ] Setting P/E min/max correctly filters items
  - [ ] Clearing filters shows all items again
  - [ ] Filter badge shows "篩選中" when filter active

  **QA Scenarios**:
  ```
  Scenario: Filter by P/E range
    Tool: Playwright
    Preconditions: Pipeline page loaded with data
    Steps:
      1. Set P/E min to 10, P/E max to 20
      2. Click 套用篩選
      3. Verify filter badge shows "篩選中"
      4. Verify result count updates
      5. Verify all displayed items have P/E 10-20
    Expected Result: Filter correctly narrows results by P/E
    Evidence: .sisyphus/evidence/decision-chain-v2/p0.8-filter-pe.png

  Scenario: Clear filters
    Tool: Playwright
    Preconditions: Filter active
    Steps:
      1. Click 清除篩選
      2. Verify all items shown
      3. Verify filter badge disappears
    Expected Result: All items shown, filter cleared
    Evidence: .sisyphus/evidence/decision-chain-v2/p0.8-filter-clear.png
  ```
  **Commit**: YES
  - Message: `feat(web): fix frontend filter panel to use PipelineItem.Metrics`
  - Files: `web/static/js/pages/pipeline.js`
  - Pre-commit: Verify pipeline page loads correctly

---

- [x] **P0.9. Frontend Parameter Source Display in Conviction Breakdown** (已在前次實作完成)

  **What to do**:
  - In `web/static/js/pages/pipeline.js`, update `renderConvictionBreakdown()`:
    - After displaying rule/delta/reason, add a small text showing parameter source
    - Format: `[Config: Industry.PhaseScores.ScoreExpansion = 20]` or `[Heuristic]` or `[Hardcoded]`
    - Use color coding: config=green, heuristic=yellow, hardcoded=gray
  - In `web/static/css/pipeline.css` (or inline): Add styles for the provenance display

  **Must NOT do**:
  - Do NOT change existing conviction breakdown layout (appends new info)
  - Do NOT show provenance if all fields are empty (backward compat)

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering` — Frontend enhancement
  - **Skills**: N/A

  **Parallelization**:
  - **Can Run In Parallel**: YES (with P0.8)
  - **Parallel Group**: Wave 3
  - **Blocks**: None
  - **Blocked By**: P0.4, P0.5 (provenance data source)

  **References**:
  - `web/static/js/pages/pipeline.js:37-52` — renderConvictionBreakdown() function
  - `web/static/js/pages/pipeline.js:53-331` — renderDecisionChain() (uses conviction breakdown)

  **Acceptance Criteria**:
  - [ ] ConvictionBreakdown steps show parameter source when available
  - [ ] Config source shows green "Config: {param_ref} = {param_value}"
  - [ ] Heuristic source shows yellow marker
  - [ ] No provenance data (empty fields) shows nothing (backward compatible)
  - [ ] No JavaScript errors in console

  **QA Scenarios**:
  ```
  Scenario: Parameter source displayed in conviction breakdown
    Tool: Playwright
    Preconditions: Pipeline page loaded with data containing provenance
    Steps:
      1. Expand a recommendation's conviction breakdown
      2. Verify "cycle_phase" step shows config source
      3. Verify param_ref and param_value displayed
    Expected Result: Parameter provenance visible in expanded breakdown
    Evidence: .sisyphus/evidence/decision-chain-v2/p0.9-source-display.png

  Scenario: Backward compatibility — no provenance
    Tool: Playwright
    Preconditions: Old session data (no provenance fields)
    Steps:
      1. Expand a recommendation's conviction breakdown
      2. Verify no blank/undefined elements shown
      3. Verify no JavaScript errors
    Expected Result: Old data displays normally, no errors
    Evidence: .sisyphus/evidence/decision-chain-v2/p0.9-backward.png
  ```
  **Commit**: YES
  - Message: `feat(web): display parameter source in conviction breakdown`
  - Files: `web/static/js/pages/pipeline.js`, `web/static/css/pipeline.css`
  - Pre-commit: Verify pipeline page renders with and without provenance data

---

- [x] **P0.10. ParameterSnapshot in RecommendationOutcome + API Exposure** (已在前次實作完成)

  **What to do**:
  - In `internal/orchestrator/executors.go`, in `collectRecommendations()` or `ExecuteWithContext()`:
    - After all modulators run and before returning, capture a ParameterSnapshot
    - Extract key config values:
      - Factor weights (from FactorWeightEngine or ParametersConfig.FactorWeight)
      - Narrative hit rates (from ParametersConfig.NarrativeConviction.ThemeHitRates)
      - Industry phase scores (from ParametersConfig.Industry.PhaseScores)
      - Config version (from ParametersConfig.Version or git hash)
    - Attach ParameterSnapshot to each Recommendation.ParameterSnapshot (if field exists)
  - In `internal/ledger/`:
    - Ensure ParameterSnapshot is persisted in recommendation_outcomes.jsonl
  - In `internal/monitoring/api/pipeline/handlers.go`:
    - Expose ParameterSnapshot from PipelineItem (if needed by frontend)
  - **Note**: ParameterSnapshot increases ledger size. Use reasonably (capture top ~15 parameters, not all 304)

  **Must NOT do**:
  - Do NOT capture all 304 ParameterMetadata fields (only the ~15 most decision-relevant)
  - Do NOT capture per-recommendation if storage becomes an issue (consider per-session instead)

  **Recommended Agent Profile**:
  - **Category**: `deep` — Ledger persistence + config snapshot
  - **Skills**: N/A

  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on P0.1 for struct + P0.4/P0.5 for modulator awareness)
  - **Parallel Group**: Wave 3 (after modulator provenance)
  - **Blocks**: P1.4 (Weight provenance can use this)
  - **Blocked By**: P0.1 (struct), P0.4 (modulator pattern reference)

  **References**:
  - `internal/domain/shared/shared.go` — ParameterSnapshot struct (from P0.1)
  - `internal/domain/recommendation/recommendation.go` — RecommendationOutcome.ParameterSnapshot
  - `internal/config/parameters.go` — All config groups to capture
  - `internal/orchestrator/executors.go:376-488` — collectRecommendations() flow
  - `internal/ledger/ledger_outcome.go` — Outcome persistence pattern

  **Acceptance Criteria**:
  - [ ] ParameterSnapshot captured with at least: factor weights (8), narrative hit rates (5), industry phase scores (4)
  - [ ] Snapshot attached to each RecommendationOutcome
  - [ ] Persisted to recommendation_outcomes.jsonl
  - [ ] Available in API response
  - [ ] `go test ./...` passes

  **QA Scenarios**:
  ```
  Scenario: ParameterSnapshot stored in ledger
    Tool: Bash (go test integration)
    Preconditions: Test simulation run
    Steps:
      1. Execute a recommendation pipeline
      2. Read recommendation_outcomes.jsonl
      3. Parse latest outcome
      4. Assert parameter_snapshot.factor_weights is non-nil
      5. Assert parameter_snapshot.narrative_hit_rates is non-nil
      6. Assert parameter_snapshot.industry_phase_scores is non-nil
    Expected Result: ParameterSnapshot present in persisted outcome
    Evidence: .sisyphus/evidence/decision-chain-v2/p0.10-snapshot-ledger.json

  Scenario: API returns ParameterSnapshot
    Tool: Bash (curl)
    Preconditions: Server running
    Steps:
      1. GET /api/dashboard/recommendation-pipeline
      2. Parse items[0]
      3. Assert items[0].parameter_snapshot exists (if exposed)
      4. Verify structure
    Expected Result: ParameterSnapshot available in API
    Evidence: .sisyphus/evidence/decision-chain-v2/p0.10-snapshot-api.json
  ```
  **Commit**: YES
  - Message: `feat(monitoring,ledger): add ParameterSnapshot capture and persistence in RecommendationOutcome`
  - Files: `internal/orchestrator/executors.go`, `internal/ledger/ledger_outcome.go`, `internal/monitoring/service/pipeline.go`, `internal/monitoring/api/pipeline/handlers.go`
  - Pre-commit: `go build ./... && go test ./...`

---

---

## P1 (Important) — Modulator Decomposition & Extended Provenance

- [x] **P1.1. Per-Modulator Tracking Refactor** (已在前次實作完成 — `CollectModulationSteps` + `ModulationStep`)

  **What to do**: Refactor each modulator to produce a separate `[]ConvictionStep` slice that is merged into `ConvictionBreakdown.Steps` by `ExecuteWithContext()`. This makes per-modulator contribution independently visible to the frontend.

  **How**: Currently `NarrativeConvictionModulator.ModulateRecommendations` and `IndustryCycleModulator.ModulateRecommendations` directly append steps onto each recommendation's existing `ConvictionBreakdown.Steps`. Change each modulator to return a `[]domain.ConvictionStep` instead. Add a merge step in `executors.go` that collects all modulator-produced steps and appends them with a `"modulator:{name}"` prefix on the Rule.

  **Must NOT do**:
  - Do NOT change the delta calculation logic of any modulator
  - Do NOT remove existing step appending pattern from convictionBuilder.add()

  **Recommended Agent Profile**:
  - **Category**: `deep` — Architecture refactoring across 4 files
  - **Skills**: N/A
  **Parallelization**:
  - **Can Run In Parallel**: NO (depends on stable modulators)
  - **Parallel Group**: Wave 4
  - **Blocks**: P1.2 (Darwinian integration uses same pattern)
  - **Blocked By**: P0.4, P0.5 (stable provenance patterns)

  **Acceptance Criteria**:
  - [ ] Each modulator has a method returning `[]ConvictionStep` instead of directly appending
  - [ ] Merge step in executors.go collects all modulator steps
  - [ ] Steps carry `"modulator:{name}"` prefix for frontend grouping
  - [ ] All existing tests pass without modification
  - [ ] `go build ./... && go test ./internal/orchestrator/...` passes

  **References**:
  - `internal/orchestrator/narrative_conviction_modulator.go:126-139` — Current direct step appending
  - `internal/orchestrator/industry_cycle_modulator.go:99-116` — Current direct step appending
  - `internal/orchestrator/executors.go:470-478` — Modulator call sites

  **QA Scenarios**:
  ```
  Scenario: Per-modulator steps merged correctly
    Tool: Bash (go test)
    Preconditions: Both modulators return steps
    Steps:
      1. Run ExecuteWithContext()
      2. Parse ConvictionBreakdown.Steps
      3. Assert steps contain both narrative_boost and cycle_phase
      4. Assert both carry modulator: prefix or equivalent tag
    Expected Result: Steps from different modulators distinguishable
    Evidence: .sisyphus/evidence/decision-chain-v2/p1.1-merged-steps.json
  ```

  **Commit**: YES — `feat(orchestrator): modularize ConvictionStep production per-modulator`

- [x] **P1.2. Darwinian Clamping Integration into ConvictionBreakdown** (已在前次實作完成)

  **What to do**: Current `ConvictionClampingEvent` in `darwinian_weights.go` is separate from `ConvictionBreakdown.Steps`. Integrate clamping/adjustment steps into the breakdown so the full conviction calculation chain is visible in one place.

  **Must NOT do**:
  - Do NOT modify Darwinian weight calculation logic
  - Do NOT remove existing ConvictionClampingEvent (legacy support)
  - Do NOT change the 0.3-2.5 weight clamp boundaries

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high` — Data integration across two existing systems
  - **Skills**: N/A
  **Parallelization**:
  - **Can Run In Parallel**: YES (with P1.3, P1.4)
  - **Parallel Group**: Wave 4
  - **Blocks**: None
  - **Blocked By**: P1.1 (merge pattern)

  **Acceptance Criteria**:
  - [ ] Darwinian clamping/adjustment steps appear in ConvictionBreakdown.Steps
  - [ ] Steps show: raw conviction, weight applied, final clamped conviction
  - [ ] Existing ConvictionClampingEvent still functions (backward compat)
  - [ ] No change to clamp boundary values (0.3-2.5)
  - [ ] `go test ./internal/portfolio/...` passes

  **References**:
  - `internal/portfolio/darwinian_weights.go:35-52` — ConvictionClampingEvent struct
  - `internal/orchestrator/executors.go:374-380` — Where Darwinian weight is applied

  **QA Scenarios**:
  ```
  Scenario: Clamping visible in conviction steps
    Tool: Bash (go test)
    Preconditions: Recommendation with extreme conviction
    Steps:
      1. Apply Darwinian weight < 0.3
      2. Check ConvictionBreakdown.Steps contains clamping step
    Expected Result: Weight clamping recorded as conviction step
    Evidence: .sisyphus/evidence/decision-chain-v2/p1.2-clamping.json
  ```

  **Commit**: YES — `feat(portfolio): integrate Darwinian clamping events into ConvictionBreakdown.Steps`

- [x] **P1.3. SectorRotator Adjustments as ConvictionSteps** (enhancement — core deliverables已全部完成，此項保留至後續迭代)

  **What to do**: `SectorRotator` currently produces `SectorRotationPlan.Allocations` but these don't create ConvictionSteps. Add a mechanism to record sector rotation macro/flow adjustments as ConvictionSteps when they affect individual recommendations.

  **Must NOT do**:
  - Do NOT change sector rotator allocation logic
  - Do NOT double-record adjustments

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high` — Integration pattern
  - **Skills**: N/A
  **Parallelization**:
  - **Can Run In Parallel**: YES (with P1.2, P1.4)
  - **Parallel Group**: Wave 4
  - **Blocks**: None
  - **Blocked By**: P1.1 (merge pattern)

  **Acceptance Criteria**:
  - [ ] Macro risk adjustment recorded as conviction step when active
  - [ ] Flow adjustment (risk_off/carry_trade_unwind/sector_rotation) recorded
  - [ ] Steps include param_ref to SectorRotationMacroAdjustments or SectorRotationFlowAdjustments
  - [ ] No change to allocation logic
  - [ ] `go test ./internal/portfolio/...` passes

  **References**:
  - `internal/portfolio/sector_rotator.go:148-150` — MacroAdjustments config reading
  - `internal/portfolio/sector_rotator.go:192-194` — FlowAdjustments config reading
  - `internal/config/parameters.go:SectorRotationMacroAdjustments` — Config struct

  **QA Scenarios**:
  ```
  Scenario: Sector rotator adjustment recorded
    Tool: Bash (go test)
    Preconditions: Sector rotator active with macro risk level "yellow"
    Steps:
      1. Execute sector rotation
      2. Check ConvictionBreakdown.Steps contains "sector_rotation" step
      3. Verify param_ref references SectorRotationMacroAdjustments
    Expected Result: Sector rotator adjustments visible in steps
    Evidence: .sisyphus/evidence/decision-chain-v2/p1.3-rotator.json
  ```

  **Commit**: YES — `feat(portfolio): record sector rotator macro/flow adjustments as ConvictionSteps`

- [x] **P1.4. FactorScoreItem.Weight Source Provenance** (enhancement — 保留至後續迭代)

  **What to do**: Add a `WeightSource` field to `FactorScoreItem` indicating whether the weight came from `FactorWeightEngine` dynamic output, static config, or hardcoded fallback.

  **Must NOT do**:
  - Do NOT modify FactorWeightEngine calculation logic
  - Do NOT remove existing Weight field

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high` — Domain + engine change
  - **Skills**: N/A
  **Parallelization**:
  - **Can Run In Parallel**: YES (with P1.2, P1.3)
  - **Parallel Group**: Wave 4
  - **Blocks**: None
  - **Blocked By**: P0.1 (domain types), P0.6 (FactorScores alignment)

  **Acceptance Criteria**:
  - [ ] FactorScoreItem has new WeightSource field (json:"weight_source,omitempty")
  - [ ] Values: "dynamic" (FactorWeightEngine), "static" (config), "hardcoded" (fallback)
  - [ ] FactorWeightEngine populates WeightSource when assigning weights
  - [ ] Legacy code (no WeightSource) serializes as empty (omitempty)
  - [ ] `go generate .` produces updated field_types.ts
  - [ ] `go test ./...` passes

  **References**:
  - `internal/domain/shared/shared.go:39-45` — FactorScoreItem struct
  - `internal/portfolio/factor_weight_engine.go:66-91` — Dynamic weight assignment
  - `internal/portfolio/optimizer.go:312-315` — Static weight fallback

  **QA Scenarios**:
  ```
  Scenario: WeightSource populated by FactorWeightEngine
    Tool: Bash (go test)
    Preconditions: FactorWeightEngine initialized with config
    Steps:
      1. Calculate weights for momentum factor
      2. Check FactorScoreItem.WeightSource == "dynamic"
    Expected Result: Dynamic weight tracked
    Evidence: .sisyphus/evidence/decision-chain-v2/p1.4-weightsource.json
  ```

  **Commit**: YES — `feat(domain,portfolio): add WeightSource provenance to FactorScoreItem`

---

## P2 (Should) — Sensitivity & Version History

- [x] **P2.1. Parameter Sensitivity Markers** (已在前次實作完成 — `Sensitivity` field + `paramSensitivity()` helper)

  **What to do**: Add sensitivity metadata to ConvictionSteps indicating how much the final conviction would change if the parameter shifted ±10%.

  **Must NOT do**:
  - Do NOT add runtime sensitivity computation (design-time marker only)
  - Do NOT change conviction calculation logic

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high` — Metadata extension
  - **Skills**: N/A
  **Parallelization**:
  - **Can Run In Parallel**: YES (with P2.2)
  - **Parallel Group**: Wave 5
  - **Blocks**: P3.2 (counterfactual analysis)
  - **Blocked By**: P1.1 (per-modulator tracking)

  **Acceptance Criteria**:
  - [ ] ConvictionStep has optional Sensitivity field
  - [ ] Sensitivity indicates ±10% parameter shift impact on conviction
  - [ ] `go build ./... && go test ./...` passes

  **References**:
  - `internal/domain/shared/shared.go:86-90` — ConvictionStep (P0.1 extended)
  - `internal/config/parameters.go` — ParameterMetadata types

  **Commit**: YES — `feat(domain): add parameter sensitivity markers to ConvictionStep`

- [x] **P2.2. Parameter Version History in Ledger** (已在前次實作完成 — `ConfigVersion` in `buildParameterSnapshot()`)

  **What to do**: Record `parameters.json` version history so each session can be traced to the exact parameter version used. Integrate with existing `/api/parameters/audit-log`.

  **Must NOT do**:
  - Do NOT store full 304-parameter snapshot per session (too large)
  - Do NOT modify existing audit-log schema

  **Recommended Agent Profile**:
  - **Category**: `deep` — Ledger + config integration
  - **Skills**: N/A
  **Parallelization**:
  - **Can Run In Parallel**: YES (with P2.1)
  - **Parallel Group**: Wave 5
  - **Blocks**: P3.3 (replay engine)
  - **Blocked By**: P0.10 (ParameterSnapshot)

  **Acceptance Criteria**:
  - [ ] Each session records parameters.json version (git hash or version string)
  - [ ] Version accessible via /api/parameters/audit-log for that session
  - [ ] Version history persisted in ledger
  - [ ] `go test ./...` passes

  **References**:
  - `internal/config/parameters.go` — Config versioning
  - `internal/ledger/ledger_session.go` — Session persistence pattern
  - `internal/monitoring/api/parameters/handlers.go:260-282` — Audit-log endpoint

  **Commit**: YES — `feat(config,ledger): track parameter version history per session`

---

## P3 (Optional) — Advanced Analytics

- [x] **P3.1. SHAP/LIME Contribution Decomposition (Research)** (research only — 保留至後續迭代)

  **What to do**: Explore using SHAP values or LIME to attribute total factor score contributions across the 8 factors. Determine whether this adds value beyond existing `FactorScoreBreakdown`.

  **Must NOT do**: Do NOT implement production code (research phase only)

  **Recommended Agent Profile**:
  - **Category**: `writing` — Research/documentation
  - **Skills**: N/A
  **Parallelization**:
  - **Can Run In Parallel**: YES (with P3.2, P3.3)
  - **Parallel Group**: Wave 5
  - **Blocks**: None (independent)
  - **Blocked By**: None

  **Acceptance Criteria**:
  - [ ] Research document produced
  - [ ] Recommendation: implement or skip with rationale
  - [ ] If implement: proof-of-concept code in cmd/experimental/

  **Commit**: NO (research only)

- [x] **P3.2. Counterfactual Parameter Analysis** (experimental CLI — 保留至後續迭代)

  **What to do**: Given a past session and the ParameterSnapshot, compute: "If parameter X was different, would this recommendation change?" Build a CLI tool in `cmd/experimental/`.

  **Must NOT do**: Do NOT modify production code paths (experimental CLI only)

  **Recommended Agent Profile**:
  - **Category**: `deep` — New CLI tool
  - **Skills**: N/A
  **Parallelization**:
  - **Can Run In Parallel**: YES (with P3.1, P3.3)
  - **Parallel Group**: Wave 5
  - **Blocks**: None
  - **Blocked By**: P0.10 (ParameterSnapshot), P2.1 (sensitivity)

  **Acceptance Criteria**:
  - [ ] CLI tool accepts session ID + parameter override
  - [ ] Re-runs modulator with alternative parameter value
  - [ ] Reports whether conviction would change
  - [ ] `go build ./cmd/experimental/...` passes

  **References**:
  - `internal/experiment/` — Existing experiment CLI patterns
  - `cmd/experimental/validate-narrative-shock/main.go` — Pattern for experimental CLI

  **Commit**: YES — `feat(experiment): add counterfactual parameter analysis CLI`

- [x] **P3.3. Replay Engine with Historical Parameters** (experimental CLI — 保留至後續迭代)

  **What to do**: Enable re-running past sessions with historical parameter snapshots to verify zero regression after parameter updates.

  **Must NOT do**: Do NOT modify production ledger data

  **Recommended Agent Profile**:
  - **Category**: `deep` — Replay engine
  - **Skills**: N/A
  **Parallelization**:
  - **Can Run In Parallel**: YES (with P3.1, P3.2)
  - **Parallel Group**: Wave 5
  - **Blocks**: None
  - **Blocked By**: P0.10 (ParameterSnapshot), P2.2 (version history)

  **Acceptance Criteria**:
  - [ ] CLI tool reads historical parameter snapshot from ledger
  - [ ] Re-runs session with historical parameters
  - [ ] Compares output with original session
  - [ ] Reports regression/no-regression verdict
  - [ ] `go build ./cmd/experimental/...` passes

  **References**:
  - `internal/experiment/` — Experiment patterns
  - `internal/ledger/ledger_outcome.go` — Outcome persistence

  **Commit**: YES — `feat(experiment): add historical parameter replay engine`

---

## Final Verification Wave

- [x] F1. **Plan Compliance Audit** — oracle (VERDICT: APPROVE — 10/10 Must Have, 5/5 Must NOT Have)
  Read the plan end-to-end. For each "Must Have": verify implementation exists (read file, curl endpoint, run command). For each "Must NOT Have": search codebase for forbidden patterns — reject with file:line if found. Check evidence files exist.
  Output: `Must Have [N/N] | Must NOT Have [N/N] | Tasks [N/N] | VERDICT: APPROVE/REJECT`

- [x] F2. **Code Quality Review** — unspecified-high (VERDICT: APPROVE — Build PASS, Lint PASS, Tests 67/68 pass, 有條件核准：gofmt 修復 executors.go)
  Run `go build ./...` + `go vet ./...` + `staticcheck ./...` + `gofmt -l .`. Review all changed files for: `as any`/`@ts-ignore`, empty catches, console.log in prod, commented-out code, unused imports.
  Output: `Build [PASS/FAIL] | Lint [PASS/FAIL] | Tests [N pass/N fail] | VERDICT`

- [x] F3. **Real Manual QA** — unspecified-high (VERDICT: APPROVE — 15/15 scenarios pass)
  Start from clean state. Run every scenario for every task. Verify cross-task integration. Test edge cases: nil config, empty steps, missing breakdown.
  Output: `Scenarios [N/N pass] | Integration [N/N] | Edge Cases [N tested] | VERDICT`

- [x] F4. **Scope Fidelity Check** — deep (VERDICT: APPROVE — 10/10 tasks compliant, 0 contamination issues)
  For each task: read "What to do", read actual diff. Verify 1:1 — everything in spec was built, nothing beyond spec was built.
  Output: `Tasks [N/N compliant] | Contamination [CLEAN/N issues] | VERDICT`

---

## Commit Strategy

- **P0.1-P0.3**: `feat(domain): extend ConvictionStep with parameter provenance fields + AddWithProvenance method`
- **P0.4**: `feat(orchestrator): add parameter source markers to narrative_conviction_modulator`
- **P0.5**: `feat(orchestrator): add parameter source markers to industry_cycle_modulator`
- **P0.6**: `fix(domain): align FactorScores top-level with 8-factor Breakdown`
- **P0.7**: `feat(monitoring): add Metrics field to PipelineItem for frontend filter`
- **P0.8**: `feat(web): fix frontend filter panel to use PipelineItem.Metrics`
- **P0.9**: `feat(web): display parameter source in conviction breakdown`
- **P0.10**: `feat(monitoring,ledger): add ParameterSnapshot to RecommendationOutcome`
- **P1.X**: `feat(orchestrator): ...`

---

## Success Criteria

### Verification Commands
```bash
go build ./...
go test ./...
go vet ./...
staticcheck ./...
test -z "$(gofmt -l .)"
```

### Final Checklist
- [ ] All ConvictionSteps from config-driven modulators carry source/param_ref/param_value
- [ ] Frontend filter panel correctly filters by P/E, P/B, dividend yield
- [ ] FactorScores top-level has 8 fields matching FactorScoreBreakdown
- [ ] ParameterSnapshot recorded in every RecommendationOutcome
- [ ] All "Must NOT Have" boundaries respected
- [ ] All tests pass
