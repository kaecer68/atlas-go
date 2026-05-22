# 全倉庫硬編碼參數遷移至 ParametersConfig

## TL;DR

> **Quick Summary**: 將 atlas-go 全倉庫中影響投資模型行為的硬編碼參數遷移至 ParametersConfig 統一管理架構，為每個參數附帶權威溯源（Rationale/Source/Todo），確保零行為回歸。
>
> **Deliverables**:
> - 新增 `FactorWeightParameters` 和 `NarrativeConvictionParameters` 結構
> - 擴展 `IndustryParameters`、`OrchestratorParameters` 等既有結構
> - 所有硬編碼值遷移至 `configs/parameters.json` 可配置
> - 跨參數驗證邏輯（權重總和 = 1.0 等）
>
> **Estimated Effort**: Large (50+ 參數點，跨 8+ 檔案)
> **Parallel Execution**: YES - 4 波並行執行
> **Critical Path**: 審計目錄 → 結構定義 → 逐檔遷移 → 整合驗證 → 審查

---

## Context

### Original Request
程式碼審查發現 PR #56 引入的 FactorWeightEngine 和 NarrativeConvictionModulator 中存在大量硬編碼值，未納入既有的 ParametersConfig 架構。使用者要求進行全倉庫硬編碼審計並遷移。

### Interview Summary
**Key Discussions**:
- **範圍**：從 PR #56 新程式碼擴大到「全倉庫硬編碼審計」（所有 internal/ 下影響投資模型行為的魔術數字）
- **結構設計**：新增 `FactorWeightParameters` 和 `NarrativeConvictionParameters` 獨立結構，不修改既有 `FactorParameters` 和 `NarrativeParameters`
- **測試策略**：TDD — 每個遷移任務先寫測試驗證原有行為 → 遷移 → 測試確認零回歸
- **分支**：`feat/hardcoded-params-config-migration`
- **約束**：omitempty 向後相容、所有新參數附帶溯源、禁止修改既有結構

**Research Findings**:
- 既有 ParametersConfig 含 17 個參數結構，每個參數都有 ParameterMetadata 包裝
- FactorWeightEngine 有 7 類別硬編碼（權重、政權調整、clamp、RISK_ON/OFF、嚴重程度 delta、事件映射、策略調整）
- NarrativeConvictionModulator 有 2 類別硬編碼（主題命中率、技能映射）
- IndustryCycleModulator 有 2 類別硬編碼（skillToIndustry 映射、phaseDelta 常數）
- sector_rotator.go 部分已配置化（基礎配置來自 SectorRotationConfig），但 macro/flow 調整方法中仍有硬編碼
- 既有 `OptimizerParameters.FactorWeights` 6 權重 vs `OrchestratorParameters.FactorWeight*` 4 欄位 vs `FactorWeightEngine.baseWeights` 8 權重 — 三套權重服務不同子系統

### Metis Review
**Identified Gaps** (addressed):
- **向後相容性**：新增 omitempty 結構，無 parameters.json 時使用預設值（行為不變）
- **遷移路徑**：每個參數遷移後立即執行 go test，若失敗則回滾
- **參數判斷標準**：「在回測中改變此值是否產生不同的投資決策？」→ 是則遷移
- **排除範圍明確化**：cmd/experimental/、測試檔案、web/static/、configs/agents.json
- **跨參數驗證**：權重總和 = 1.0 等約束加入 Validate()
- **零殘留驗證**：遷移完成後 grep 原始硬編碼值確認零殘留

---

## Work Objectives

### Core Objective
將 atlas-go 全倉庫中所有影響投資模型的硬編碼魔術數字遷移至 ParametersConfig 統一管理架構，為每個參數建立完整的權威溯源鏈。

### Concrete Deliverables
- `internal/config/parameters.go`: 新增 `FactorWeightParameters`、`NarrativeConvictionParameters` 結構及 Validate() 跨參數驗證
- `internal/config/parameters_defaults.go`: 新增對應的預設值函數（值與目前硬編碼完全一致）
- `configs/parameters.json`: 更新包含所有新參數群組的範例配置
- `internal/portfolio/factor_weight_engine.go`: 從 ParametersConfig 讀取配置，移除內部硬編碼
- `internal/orchestrator/narrative_conviction_modulator.go`: 從 ParametersConfig 讀取配置
- `internal/orchestrator/industry_cycle_modulator.go`: skillToIndustry 和 phaseDelta 遷移
- `internal/portfolio/sector_rotator.go`: applyMacroAdjustments 和 applyFlowAdjustments 硬編碼遷移
- `internal/portfolio/AGENTS.md`: 更新 FactorWeightEngine 文件，反映配置驅動設計

### Definition of Done
- [ ] `go test ./...` 遷移前後通過且結果一致
- [ ] `grep -rn` 確認已遷移檔案中無原始硬編碼值殘留
- [ ] 所有新參數有非空 Rationale/Source/Todo
- [ ] `configs/parameters.json` 含所有新參數群組
- [ ] 應用啟動成功，`/health` 返回 200

### Must Have
- 零行為回歸（遷移前後所有測試結果一致）
- 所有新 JSON 欄位使用 omitempty
- 每個參數附帶 Rationale/Source/Todo 溯源
- 跨參數驗證邏輯（權重總和 = 1.0 等）
- 不修改任何既有 Parameter 結構定義

### Must NOT Have (Guardrails)
- 不修改 FactorParameters、NarrativeParameters 等既有結構
- 不遷移數學/演算法常數（如 `math.Abs(total-1.0) < 0.001`）
- 不觸碰 `cmd/experimental/`、測試檔案、`web/static/`、`configs/agents.json`
- 不改變任何投資行為 — 僅將值從程式碼移到配置
- 不建立循環依賴（config 套件為葉節點，只使用 domain 類型）
- 不產生 new `go vet` 或 `staticcheck` 警告

---

## Verification Strategy

> **ZERO HUMAN INTERVENTION** - ALL verification is agent-executed. No exceptions.

### Test Decision
- **Infrastructure exists**: YES (go test + test files in same package)
- **Automated tests**: TDD
- **Framework**: go test (standard Go testing)
- **TDD Flow**: Write characterization test verifying CURRENT behavior → Extract parameter → Test still passes → Commit

### QA Policy
Every task MUST include agent-executed QA scenarios.
Evidence saved to `.sisyphus/evidence/task-{N}-{scenario-slug}.txt`.

- **API/Backend**: Use Bash (go test) - Run focused tests, verify output
- **Build verification**: Use Bash (go build) - Must succeed with no new warnings
- **Grep verification**: Use Grep - Verify zero hardcoded values remain in migrated files

---

## Execution Strategy

### Parallel Execution Waves

> Target: 5-8 tasks per wave. Fewer than 3 per wave (except final) = under-splitting.

```
Wave 1 (Start Immediately - audit + struct definition):
├── Task 1: Catalog all hardcoded values with file:line references [quick]
├── Task 2: Define FactorWeightParameters struct [quick]
├── Task 3: Define NarrativeConvictionParameters struct [quick]
├── Task 4: Define industry/orchestrator extended parameter structs [quick]
├── Task 5: Write default value functions (matching current hardcoded values) [quick]
└── Task 6: Write characterization tests for FactorWeightEngine [quick]

Wave 2 (After Wave 1 - core migration, MAX PARALLEL):
├── Task 7: Migrate FactorWeightEngine base weights [quick]
├── Task 8: Migrate FactorWeightEngine regime adjustments [quick]
├── Task 9: Migrate FactorWeightEngine severity deltas + event theme mappings [quick]
├── Task 10: Migrate FactorWeightEngine strategy risk adjustments [quick]
├── Task 11: Migrate NarrativeConvictionModulator theme hit rates [quick]
└── Task 12: Migrate NarrativeConvictionModulator skill-to-theme mappings [quick]

Wave 3 (After Wave 2 - extended migration):
├── Task 13: Migrate IndustryCycleModulator skillToIndustry + phaseDelta [quick]
├── Task 14: Migrate sector_rotator applyMacroAdjustments hardcoded values [quick]
├── Task 15: Migrate sector_rotator applyFlowAdjustments hardcoded values [quick]
├── Task 16: Cross-parameter validation in Validate() [quick]
├── Task 17: Update configs/parameters.json with new parameter groups [quick]
└── Task 18: Update AGENTS.md documentation [quick]

Wave FINAL (After ALL tasks — 4 parallel reviews, then user okay):
├── Task F1: Plan compliance audit (oracle)
├── Task F2: Code quality review (unspecified-high)
├── Task F3: Real manual QA (unspecified-high)
└── Task F4: Scope fidelity check (deep)
-> Present results -> Get explicit user okay

Critical Path: Task 1 → Task 2 → Task 7 → Task 16 → Task 17 → F1-F4 → user okay
Parallel Speedup: ~60% faster than sequential
Max Concurrent: 6 (Waves 1 & 2)
```

### Dependency Matrix

- **1**: - - 7-18, 1
- **2**: 1 - 7, 3
- **3**: 1 - 12, 3
- **4**: 1 - 13, 15, 3
- **5**: 2,3,4 - 8-16, 3
- **6**: - - 7, 2
- **7**: 2,5,6 - 16, 2
- **8**: 2,5 - 16, 2
- **9**: 5 - 16, 2
- **10**: 5 - 16, 2
- **11**: 3,5 - 16, 2
- **12**: 3,5 - 16, 2
- **13**: 4,5 - 16, 2
- **14**: 4,5 - 16, 2
- **15**: 4,5 - 16, 2
- **16**: 7-15 - 17, 3
- **17**: 16 - 18, 3
- **18**: 17 - F1-F4, 3

### Agent Dispatch Summary

- **Wave 1**: **6** - T1 → `quick`, T2-T4 → `quick`, T5 → `unspecified-high`, T6 → `quick`
- **Wave 2**: **6** - T7-T12 → `quick`
- **Wave 3**: **6** - T13-T18 → `quick`
- **FINAL**: **4** - F1 → `oracle`, F2 → `unspecified-high`, F3 → `unspecified-high`, F4 → `deep`

---

## TODOs

- [ ] 1. 審計目錄：掃描全倉庫硬編碼值並建立 file:line 參考目錄

  **What to do**:
  - 使用 grep/ast_grep 掃描 `internal/` 下所有非測試 .go 檔案
  - 識別所有影響投資模型行為的硬編碼數值和字串常量
  - 對每個值應用 litmus test：「在回測中改變此值是否產生不同的投資決策？」
  - 分類為 CONFIG_CANDIDATE（應遷移）或 LEGITIMATE_CONSTANT（可保留）
  - 輸出結構化目錄：`file:line | value | context | classification`
  - 重點目錄：`internal/portfolio/`, `internal/orchestrator/`, `internal/industry/`, `internal/narrative/`, `internal/risk/`

  **Must NOT do**:
  - 不掃描測試檔案（`*_test.go`）
  - 不掃描 `cmd/experimental/`
  - 不標記數學常數（π, e, 緩衝大小, timeout 等）

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 掃描 + 分類任務，無需複雜邏輯
  - **Skills**: `[]`
  - **Skills Evaluated but Omitted**: 無

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 2-5)
  - **Blocks**: Tasks 2-18（所有遷移任務需要審計目錄）
  - **Blocked By**: None

  **References**:
  - `internal/portfolio/factor_weight_engine.go` - 主要的硬編碼集中地（權重、政權、delta、映射）
  - `internal/orchestrator/narrative_conviction_modulator.go` - 主題命中率 + 技能映射
  - `internal/orchestrator/industry_cycle_modulator.go` - skillToIndustry + phaseDelta
  - `internal/portfolio/sector_rotator.go` - applyMacroAdjustments + applyFlowAdjustments
  - `internal/config/parameters_defaults.go` - 既有參數作為參考範例

  **Acceptance Criteria**:
  - [ ] 目錄文件存於 `.sisyphus/evidence/task-1-audit-catalog.md`
  - [ ] 每個硬編碼值有 file:line 參考
  - [ ] 每個值有 CONFIG_CANDIDATE 或 LEGITIMATE_CONSTANT 分類
  - [ ] 至少發現 20+ 個 CONFIG_CANDIDATE 項目

  **QA Scenarios**:
  ```
  Scenario: 審計目錄完整性驗證
    Tool: bash
    Steps:
      1. cat .sisyphus/evidence/task-1-audit-catalog.md
      2. 驗證至少包含以下文件中的硬編碼：factor_weight_engine.go, narrative_conviction_modulator.go, industry_cycle_modulator.go, sector_rotator.go
      3. 驗證每個項目有 file:line 格式參考
    Expected Result: 目錄文件存在且包含預期文件的硬編碼條目
    Evidence: .sisyphus/evidence/task-1-audit-catalog.md

  Scenario: 排除測試檔案和實驗命令
    Tool: bash
    Steps:
      1. grep "_test.go" .sisyphus/evidence/task-1-audit-catalog.md
      2. grep "cmd/experimental" .sisyphus/evidence/task-1-audit-catalog.md
    Expected Result: 兩個 grep 都應返回空結果
    Evidence: .sisyphus/evidence/task-1-exclusion-verify.txt
  ```

  **Commit**: YES
  - Message: `docs(audit): catalog all hardcoded investment parameters`
  - Files: `.sisyphus/evidence/task-1-audit-catalog.md`

---

- [ ] 2. 定義 FactorWeightParameters 結構

  **What to do**:
  - 在 `internal/config/parameters.go` 中新增 `FactorWeightParameters` struct
  - 包含以下 ParameterMetadata 欄位（全部使用 snake_case JSON tag + omitempty）：
    - `base_weights`: `ParameterMetadata[map[string]float64]` — 8 因子基礎權重
    - `regime_bull_momentum`: `ParameterMetadata[float64]` (+0.05)
    - `regime_bull_quality`: `ParameterMetadata[float64]` (-0.03)
    - `regime_bull_value`: `ParameterMetadata[float64]` (-0.02)
    - `regime_bear_quality`: `ParameterMetadata[float64]` (+0.05)
    - `regime_bear_value`: `ParameterMetadata[float64]` (+0.03)
    - `regime_bear_momentum`: `ParameterMetadata[float64]` (-0.05)
    - `regime_high_vol_liquidity`: `ParameterMetadata[float64]` (+0.05)
    - `regime_high_vol_momentum`: `ParameterMetadata[float64]` (-0.03)
    - `regime_high_vol_inst_sent`: `ParameterMetadata[float64]` (-0.02)
    - `severity_critical`: `ParameterMetadata[float64]` (0.10)
    - `severity_high`: `ParameterMetadata[float64]` (0.05)
    - `severity_medium`: `ParameterMetadata[float64]` (0.02)
    - `severity_low`: `ParameterMetadata[float64]` (0.01)
    - `clamp_min`: `ParameterMetadata[float64]` (0.02)
    - `clamp_max`: `ParameterMetadata[float64]` (0.50)
    - `risk_on_momentum`: `ParameterMetadata[float64]` (+0.05)
    - `risk_on_quality`: `ParameterMetadata[float64]` (-0.03)
    - `risk_off_momentum`: `ParameterMetadata[float64]` (-0.05)
    - `risk_off_quality`: `ParameterMetadata[float64]` (+0.05)
    - `risk_off_liquidity`: `ParameterMetadata[float64]` (+0.03)
    - `conservative_value`: `ParameterMetadata[float64]` (+0.05)
    - `conservative_quality`: `ParameterMetadata[float64]` (+0.05)
    - `conservative_momentum`: `ParameterMetadata[float64]` (-0.05)
    - `aggressive_momentum`: `ParameterMetadata[float64]` (+0.05)
    - `aggressive_inst_sent`: `ParameterMetadata[float64]` (+0.03)
    - `aggressive_value`: `ParameterMetadata[float64]` (-0.03)
    - `aggressive_quality`: `ParameterMetadata[float64]` (-0.03)
  - 在 `ParametersConfig` struct 中新增 `FactorWeight FactorWeightParameters` 欄位
  - 所有 JSON tag 使用 snake_case，Go 欄位名使用 PascalCase

  **Must NOT do**:
  - 不修改既有 `FactorParameters` struct
  - 不修改既有 `OptimizerParameters.FactorWeights`

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 結構定義任務，遵循既有模式
  - **Skills**: `[]`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 3-5)
  - **Blocks**: Tasks 5, 7-10
  - **Blocked By**: Task 1（需要審計目錄確認所有欄位）

  **References**:
  - `internal/config/parameters.go:70-88` - FactorParameters 結構作為範例
  - `internal/config/parameters.go:588-608` - ParametersConfig 頂層結構

  **Acceptance Criteria**:
  - [ ] `FactorWeightParameters` struct 定義在 `internal/config/parameters.go`
  - [ ] 所有欄位使用 `ParameterMetadata[T]` 包裝
  - [ ] JSON tag 使用 snake_case + omitempty
  - [ ] `go build ./...` 通過

  **QA Scenarios**:
  ```
  Scenario: 結構編譯驗證
    Tool: bash
    Steps:
      1. go build ./internal/config/...
    Expected Result: 編譯成功，無錯誤
    Evidence: .sisyphus/evidence/task-2-build-ok.txt

  Scenario: JSON tag 格式驗證
    Tool: bash
    Steps:
      1. go run -mod=readonly -json ./internal/config/ 2>&1 || true
      2. grep -c 'json:"' internal/config/parameters.go | head -5
    Expected Result: 新欄位使用 snake_case JSON tag
    Evidence: .sisyphus/evidence/task-2-json-tags.txt
  ```

  **Commit**: YES
  - Message: `feat(config): add FactorWeightParameters struct`
  - Files: `internal/config/parameters.go`

---

- [ ] 3. 定義 NarrativeConvictionParameters 結構

  **What to do**:
  - 在 `internal/config/parameters.go` 中新增 `NarrativeConvictionParameters` struct
  - 包含以下 ParameterMetadata 欄位：
    - `theme_hit_rates`: `ParameterMetadata[map[string]float64]` — AI_capex_surge:0.81, US_rates_up:0.72, JPY_carry_unwind:0.68, geopolitical_risk_spike:0.65, oil_price_shock:0.58
    - `skill_to_theme`: `ParameterMetadata[map[string]string]` — 9 對 agent skill → narrative theme 映射
  - 在 `ParametersConfig` struct 中新增 `NarrativeConviction NarrativeConvictionParameters` 欄位
  - 所有 JSON tag 使用 snake_case + omitempty

  **Must NOT do**:
  - 不修改既有 `NarrativeParameters` struct

  **Recommended Agent Profile**:
  - **Category**: `quick`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1-2, 4-6)
  - **Blocks**: Tasks 5, 11-12
  - **Blocked By**: Task 1

  **References**:
  - `internal/config/parameters.go:232-303` - NarrativeParameters 結構
  - `internal/orchestrator/narrative_conviction_modulator.go:21-40` - 目前硬編碼值

  **Acceptance Criteria**:
  - [ ] `NarrativeConvictionParameters` struct 定義完成
  - [ ] `go build ./...` 通過

  **QA Scenarios**:
  ```
  Scenario: 結構編譯驗證
    Tool: bash
    Steps:
      1. go build ./internal/config/...
    Expected Result: 編譯成功
    Evidence: .sisyphus/evidence/task-3-build-ok.txt
  ```

  **Commit**: YES
  - Message: `feat(config): add NarrativeConvictionParameters struct`
  - Files: `internal/config/parameters.go`

---

- [ ] 4. 擴展 IndustryParameters 與 OrchestratorParameters

  **What to do**:
  - 在 `IndustryParameters` 中新增／擴展欄位（`phase_scores` 可能已存在，請核實後擴展而非重複定義）：
    - `phase_scores`: `ParameterMetadata[PhaseScoresConfig]` — expansion/recovery/mature/recession 分數
    - `skill_to_industry`: `ParameterMetadata[map[string]string]` — agent skill → industry 映射
  - 在 `OrchestratorParameters` 中新增欄位：
    - `sector_rotation_macro_adjustments`: `ParameterMetadata[map[string]map[string]float64]` — 各 macro risk level 的分配調整
    - `sector_rotation_flow_adjustments`: `ParameterMetadata[map[string]map[string]float64]` — 各 primary flow 的分配調整
  - 所有 JSON tag 使用 snake_case + omitempty

  **Must NOT do**:
  - 不修改既有欄位的 JSON tag 或型別

  **Recommended Agent Profile**:
  - **Category**: `quick`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1-3, 5-6)
  - **Blocks**: Tasks 5, 13-15
  - **Blocked By**: Task 1

  **References**:
  - `internal/config/parameters.go:367-409` - IndustryParameters 結構
  - `internal/config/parameters.go:341-365` - OrchestratorParameters 結構
  - `internal/orchestrator/industry_cycle_modulator.go:18-51` - 硬編碼值
  - `internal/portfolio/sector_rotator.go:100-158` - 硬編碼調整

  **Acceptance Criteria**:
  - [ ] 新增欄位編譯通過
  - [ ] `go build ./...` 通過

  **QA Scenarios**:
  ```
  Scenario: 編譯驗證
    Tool: bash
    Steps:
      1. go build ./internal/config/...
    Expected Result: 編譯成功
    Evidence: .sisyphus/evidence/task-4-build-ok.txt
  ```

  **Commit**: YES
  - Message: `feat(config): extend IndustryParameters and OrchestratorParameters`
  - Files: `internal/config/parameters.go`

---

- [ ] 5. 撰寫預設值函數

  **What to do**:
  - 在 `internal/config/parameters_defaults.go` 中新增函數：
    - `defaultFactorWeightParameters()` → 回傳與目前硬編碼值完全一致的參數
    - `defaultNarrativeConvictionParameters()` → 回傳與目前硬編碼值完全一致的參數
  - 更新 `defaultIndustryParameters()` → 包含新的 phase_scores 和 skill_to_industry
  - 更新 `defaultOrchestratorParameters()` → 包含新的 sector_rotation 參數
  - 每個參數必須包含非空的 `Rationale`、`Source`、`Todo` 欄位
  - 更新 `DefaultParametersConfig()` 以包含新參數群組
  - 難溯源的值使用 `SourceHeuristic`，Todo 設為 "Calibrate from backtest"

  **Must NOT do**:
  - 不改變任何預設值的數值 — 必須與目前硬編碼完全一致
  - 不修改既有函數的回傳結構（只新增欄位）

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: 需要對投資領域有足夠理解來撰寫有意義的 Rationale/Todo

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1-4, 6)
  - **Blocks**: Tasks 7-15
  - **Blocked By**: Tasks 2-4（需要結構定義）

  **References**:
  - `internal/config/parameters_defaults.go:156-247` - defaultFactorParameters() 範例
  - `internal/config/parameters_defaults.go:1400+` - defaultIndustryParameters() 範例
  - `internal/config/parameters_defaults.go:10-31` - DefaultParametersConfig() 組裝函數
  - `internal/config/parameters.go:12-20` - ParameterSource 常數

  **Acceptance Criteria**:
  - [ ] 所有新預設值函數編譯通過
  - [ ] 每個參數有非空 Rationale/Source/Todo
  - [ ] `DefaultParametersConfig()` 回傳完整結構
  - [ ] `go build ./...` 通過

  **QA Scenarios**:
  ```
  Scenario: 預設值正確性驗證
    Tool: bash
    Steps:
      1. go build ./internal/config/...
      2. go test ./internal/config/...
    Expected Result: 編譯和測試皆通過
    Evidence: .sisyphus/evidence/task-5-build-test.txt

  Scenario: Rationale 完整性驗證
    Tool: bash
    Steps:
      1. grep -c 'Rationale:' internal/config/parameters_defaults.go
      2. 確認新參數群組每個都有對應 Rationale
    Expected Result: 每個 ParameterMetadata 有 Rationale 欄位
    Evidence: .sisyphus/evidence/task-5-rationale-count.txt
  ```

  **Commit**: YES
  - Message: `feat(config): add default values for new parameter groups`
  - Files: `internal/config/parameters_defaults.go`

---

- [ ] 6. 撰寫 FactorWeightEngine 特徵化測試

  **What to do**:
  - 為 FactorWeightEngine 的所有公開方法撰寫特徵化測試
  - 測試目標：捕捉目前行為作為基準線
  - 測試方法：
    - `GetWeights("")` → 驗證回傳 8 個權重且總和 ≈ 1.0
    - `GetWeights("bull")` → 驗證 momentum > quality（政權調整生效）
    - `GetWeights("bear")` → 驗證 quality > momentum
    - `AddEvent(critical AI_capex_surge)` → 驗證權重變化
    - `OnRegimeChange("RISK_ON")` → 驗證 momentum 提升
    - `ApplyStrategy(conservative)` → 驗證 value/quality 提升
  - 這些測試在遷移後仍通過（證明零行為回歸）

  **Must NOT do**:
  - 不依賴 ParametersConfig（測試直接驗證行為）

  **Recommended Agent Profile**:
  - **Category**: `quick`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1-5)
  - **Blocks**: None（獨立於遷移任務，純粹提供基準線）
  - **Blocked By**: None

  **References**:
  - `internal/portfolio/factor_weight_engine_test.go` - 既有測試
  - `internal/portfolio/factor_weight_engine.go:38-88` - GetWeights 邏輯

  **Acceptance Criteria**:
  - [ ] `go test ./internal/portfolio/... -run TestFactorWeightEngine` → PASS
  - [ ] 至少新增 3 個特徵化測試案例（覆蓋 bull/bear/event）

  **QA Scenarios**:
  ```
  Scenario: 基準線測試執行
    Tool: bash
    Steps:
      1. go test -v ./internal/portfolio/... -run TestFactorWeightEngine
    Expected Result: 所有測試 PASS
    Evidence: .sisyphus/evidence/task-6-baseline-test.txt
  ```

  **Commit**: YES
  - Message: `test(portfolio): add characterization tests for FactorWeightEngine`
  - Files: `internal/portfolio/factor_weight_engine_test.go`

---

- [ ] 7. 遷移 FactorWeightEngine 基礎權重

  **What to do**:
  - 修改 `NewFactorWeightEngine()` 從 `config.GetParametersConfig().FactorWeight` 讀取 `BaseWeights`
  - 若 config 不可用（nil），回退至硬編碼預設值（向後相容）
  - 修改 `SetBaseWeights()` 以接受外部注入
  - 確保現有 `factor_weight_engine_test.go` 中所有測試仍通過

  **Must NOT do**:
  - 不改變權重計算邏輯
  - 不改變 normalizeWeights 行為

  **Recommended Agent Profile**:
  - **Category**: `quick`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 8-12)
  - **Blocks**: Task 16（跨參數驗證）
  - **Blocked By**: Tasks 2, 5, 6

  **References**:
  - `internal/portfolio/factor_weight_engine.go:20-36` - NewFactorWeightEngine 目前實作
  - `internal/config/parameters.go:22-31` - ParameterMetadata[T] 存取範例
  - `internal/config/config.go` - GetParametersConfig() 函數

  **Acceptance Criteria**:
  - [ ] `go test ./internal/portfolio/... -run TestFactorWeightEngine` → PASS
  - [ ] NewFactorWeightEngine 從 config 讀取基礎權重
  - [ ] config == nil 時使用預設值（不 panic）

  **QA Scenarios**:
  ```
  Scenario: 基礎權重一致
    Tool: bash
    Steps:
      1. go test -v ./internal/portfolio/... -run TestFactorWeightEngine_GetWeights
    Expected Result: 所有測試 PASS，權重值與遷移前一致
    Evidence: .sisyphus/evidence/task-7-weights-consistent.txt

  Scenario: Config 不可用時回退
    Tool: bash
    Steps:
      1. go test -v ./internal/portfolio/... -run TestFactorWeightEngine
    Expected Result: 測試 PASS（使用預設值不 panic）
    Evidence: .sisyphus/evidence/task-7-fallback-works.txt
  ```

  **Commit**: YES
  - Message: `refactor(portfolio): migrate FactorWeightEngine base weights to ParametersConfig`
  - Files: `internal/portfolio/factor_weight_engine.go`

---

- [ ] 8. 遷移 FactorWeightEngine 政權調整值

  **What to do**:
  - 修改 `GetWeights()` 中的政權調整邏輯，從 config 讀取 bull/bear/high_vol 的 delta 值
  - 修改 `OnRegimeChange()` 中的 RISK_ON/RISK_OFF 調整值
  - 確保特徵化測試中 bull/bear 行為斷言仍通過

  **Must NOT do**:
  - 不改變政權字串比對邏輯（"bull"/"bear"/"high_vol" 字面值保留）

  **Recommended Agent Profile**:
  - **Category**: `quick`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 7, 9-12)
  - **Blocks**: Task 16
  - **Blocked By**: Tasks 2, 5, 6

  **References**:
  - `internal/portfolio/factor_weight_engine.go:44-57` - GetWeights 政權 switch
  - `internal/portfolio/factor_weight_engine.go:113-131` - OnRegimeChange

  **Acceptance Criteria**:
  - [ ] `go test ./internal/portfolio/... -run TestFactorWeightEngine_GetWeights_Bull` → PASS
  - [ ] `go test ./internal/portfolio/... -run TestFactorWeightEngine_OnRegimeChange` → PASS
  - [ ] bull 政權下 momentum 權重 > quality 權重

  **QA Scenarios**:
  ```
  Scenario: 政權調整正確
    Tool: bash
    Steps:
      1. go test -v ./internal/portfolio/... -run "Bull|Bear"
    Expected Result: bull 測試 PASS（momentum > quality）, bear 測試 PASS（quality > momentum）
    Evidence: .sisyphus/evidence/task-8-regime-test.txt
  ```

  **Commit**: YES
  - Message: `refactor(portfolio): migrate regime adjustments to ParametersConfig`
  - Files: `internal/portfolio/factor_weight_engine.go`

---

- [ ] 9. 遷移 FactorWeightEngine 嚴重程度 delta 與事件主題映射

  **What to do**:
  - 修改 `applyEventAdjustment()` 從 config 讀取 severity deltas（critical/high/medium/low）
  - 修改事件主題 → 因子映射（AI_capex_surge/US_rates_up/oil_price_shock）從 config 讀取
  - 修改 `ApplyStrategy()` 從 config 讀取 conservative/aggressive 調整值

  **Must NOT do**:
  - 不改變 `applyEventAdjustment` 的邏輯結構

  **Recommended Agent Profile**:
  - **Category**: `quick`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 7-8, 10-12)
  - **Blocks**: Task 16
  - **Blocked By**: Tasks 2, 5

  **References**:
  - `internal/portfolio/factor_weight_engine.go:141-176` - applyEventAdjustment
  - `internal/portfolio/factor_weight_engine.go:201-223` - ApplyStrategy

  **Acceptance Criteria**:
  - [ ] `go test ./internal/portfolio/... -run TestFactorWeightEngine_GetWeights_Event` → PASS
  - [ ] `go test ./internal/portfolio/... -run TestFactorWeightEngine_ApplyStrategy` → PASS

  **QA Scenarios**:
  ```
  Scenario: 事件調整正確
    Tool: bash
    Steps:
      1. go test -v ./internal/portfolio/... -run "Event"
    Expected Result: 所有事件測試 PASS
    Evidence: .sisyphus/evidence/task-9-event-test.txt
  ```

  **Commit**: YES
  - Message: `refactor(portfolio): migrate severity deltas and event mappings to config`
  - Files: `internal/portfolio/factor_weight_engine.go`

---

- [ ] 10. 遷移 FactorWeightEngine clamp 邊界

  **What to do**:
  - 修改 `GetWeights()` 中的 clamp 邊界從 config 讀取（目前 0.02 / 0.50）
  - 確保特徵化測試中 clamp 行為斷言仍通過

  **Recommended Agent Profile**:
  - **Category**: `quick`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 7-9, 11-12)
  - **Blocks**: Task 16
  - **Blocked By**: Tasks 2, 5

  **References**:
  - `internal/portfolio/factor_weight_engine.go:67-87` - GetWeights clamp 邏輯

  **Acceptance Criteria**:
  - [ ] `go test ./internal/portfolio/... -run TestFactorWeightEngine_GetWeights_EventClamped` → PASS
  - [ ] 權重值恆在 [clamp_min, clamp_max] 範圍內

  **QA Scenarios**:
  ```
  Scenario: Clamp 邊界驗證
    Tool: bash
    Steps:
      1. go test -v ./internal/portfolio/... -run "Clamped"
    Expected Result: 測試 PASS，所有權重在 clamp 範圍內
    Evidence: .sisyphus/evidence/task-10-clamp-test.txt
  ```

  **Commit**: YES (groups with Task 9)
  - Files: `internal/portfolio/factor_weight_engine.go`

---

- [ ] 11. 遷移 NarrativeConvictionModulator 主題命中率

  **What to do**:
  - 修改 `NewNarrativeConvictionModulator()` 從 config 讀取 `ThemeHitRates`
  - 若 config 不可用，回退至 `defaultThemeHitRates`（向後相容）
  - 確保 `ModulateRecommendations()` 中命中率查找行為不變
  - 撰寫特徵化測試驗證 ai_capex_surge 命中率 0.81 的行為

  **Must NOT do**:
  - 不改變 `ModulateRecommendations` 的邏輯結構

  **Recommended Agent Profile**:
  - **Category**: `quick`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 7-10, 12)
  - **Blocks**: Task 16
  - **Blocked By**: Tasks 3, 5

  **References**:
  - `internal/orchestrator/narrative_conviction_modulator.go:21-27` - defaultThemeHitRates
  - `internal/orchestrator/narrative_conviction_modulator.go:42-49` - NewNarrativeConvictionModulator
  - `internal/orchestrator/narrative_conviction_modulator.go:59-127` - ModulateRecommendations

  **Acceptance Criteria**:
  - [ ] `go test ./internal/orchestrator/...` → PASS
  - [ ] 命中率從 config 讀取，預設值與目前一致
  - [ ] config == nil 時使用 defaultThemeHitRates 不 panic

  **QA Scenarios**:
  ```
  Scenario: 命中率查找正確
    Tool: bash
    Steps:
      1. go test -v ./internal/orchestrator/... -run "Narrative"
    Expected Result: 所有敘事相關測試 PASS
    Evidence: .sisyphus/evidence/task-11-hitrate-test.txt
  ```

  **Commit**: YES
  - Message: `refactor(orchestrator): migrate theme hit rates to ParametersConfig`
  - Files: `internal/orchestrator/narrative_conviction_modulator.go`

---

- [ ] 12. 遷移 NarrativeConvictionModulator 技能到主題映射

  **What to do**:
  - 修改 `NewNarrativeConvictionModulator()` 從 config 讀取 `SkillToTheme`
  - 若 config 不可用，回退至 `defaultSkillToTheme`
  - 撰寫特徵化測試驗證 semiconductor_desk → AI_capex_surge 映射

  **Recommended Agent Profile**:
  - **Category**: `quick`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 7-11)
  - **Blocks**: Task 16
  - **Blocked By**: Tasks 3, 5

  **References**:
  - `internal/orchestrator/narrative_conviction_modulator.go:30-40` - defaultSkillToTheme

  **Acceptance Criteria**:
  - [ ] `go test ./internal/orchestrator/...` → PASS
  - [ ] 映射從 config 讀取，預設值與目前一致

  **QA Scenarios**:
  ```
  Scenario: 技能映射正確
    Tool: bash
    Steps:
      1. go test -v ./internal/orchestrator/... -run "Narrative"
    Expected Result: 測試 PASS，映射結果正確
    Evidence: .sisyphus/evidence/task-12-skill-mapping-test.txt
  ```

  **Commit**: YES
  - Message: `refactor(orchestrator): migrate skill-to-theme mappings to ParametersConfig`
  - Files: `internal/orchestrator/narrative_conviction_modulator.go`

---

- [ ] 13. 遷移 IndustryCycleModulator 硬編碼

  **What to do**:
  - 修改 `IndustryCycleModulator` 從 config 讀取 `skillToIndustry` 映射
  - 從 config 讀取 phase delta 值（expansion=+20, recovery=+10, mature=0, recession=-20）
  - 若 config 不可用，回退至目前硬編碼值
  - 撰寫特徵化測試

  **Recommended Agent Profile**:
  - **Category**: `quick`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3 (with Tasks 14-18)
  - **Blocks**: Task 16
  - **Blocked By**: Tasks 4, 5

  **References**:
  - `internal/orchestrator/industry_cycle_modulator.go:18-28` - skillToIndustry
  - `internal/orchestrator/industry_cycle_modulator.go:38-51` - phaseDelta

  **Acceptance Criteria**:
  - [ ] `go test ./internal/orchestrator/...` → PASS
  - [ ] 映射和 delta 從 config 讀取

  **QA Scenarios**:
  ```
  Scenario: 行業映射驗證
    Tool: bash
    Steps:
      1. go test -v ./internal/orchestrator/... -run "IndustryCycle"
    Expected Result: 測試 PASS
    Evidence: .sisyphus/evidence/task-13-industry-test.txt
  ```

  **Commit**: YES
  - Message: `refactor(orchestrator): migrate IndustryCycleModulator to config`
  - Files: `internal/orchestrator/industry_cycle_modulator.go`

---

- [ ] 14. 遷移 sector_rotator applyMacroAdjustments 硬編碼

  **What to do**:
  - 修改 `applyMacroAdjustments()` 從 config 讀取各 macro risk level 的分配調整
  - Yellow: defensive+0.05, cash+0.03, ai_supply_chain-0.03, semiconductor-0.03
  - Orange: defensive+0.10, cash+0.08, gold+0.05, ai_supply_chain-0.08, semiconductor-0.08, financials-0.05
  - Red: cash+0.25, defensive+0.15, gold+0.10, financials-0.10, shipping-0.05
  - 使用 OrchestratorParameters 中的新欄位
  - 撰寫特徵化測試

  **Recommended Agent Profile**:
  - **Category**: `quick`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3 (with Tasks 13, 15-18)
  - **Blocks**: Task 16
  - **Blocked By**: Tasks 4, 5

  **References**:
  - `internal/portfolio/sector_rotator.go:100-131` - applyMacroAdjustments

  **Acceptance Criteria**:
  - [ ] `go test ./internal/portfolio/... -run TestSectorRotator` → PASS
  - [ ] 各 risk level 的分配調整從 config 讀取

  **QA Scenarios**:
  ```
  Scenario: Macro 調整驗證
    Tool: bash
    Steps:
      1. go test -v ./internal/portfolio/... -run "SectorRotator"
    Expected Result: 測試 PASS
    Evidence: .sisyphus/evidence/task-14-macro-test.txt
  ```

  **Commit**: YES
  - Message: `refactor(portfolio): migrate sector_rotator macro adjustments to config`
  - Files: `internal/portfolio/sector_rotator.go`

---

- [ ] 15. 遷移 sector_rotator applyFlowAdjustments 硬編碼

  **What to do**:
  - 修改 `applyFlowAdjustments()` 從 config 讀取各 primary flow 的分配調整
  - risk_off: gold+0.10, high_dividend+0.07, ai_supply_chain-0.10, semiconductor-0.10
  - carry_trade_unwind: jpy+0.05, ai_supply_chain+0.02, semiconductor+0.03, financials-0.10
  - sector_rotation: alternative_energy+0.05, shipping+0.05, high_valuation_tech+0.02
  - 使用 OrchestratorParameters 中的新欄位

  **Recommended Agent Profile**:
  - **Category**: `quick`

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3 (with Tasks 13-14, 16-18)
  - **Blocks**: Task 16
  - **Blocked By**: Tasks 4, 5

  **References**:
  - `internal/portfolio/sector_rotator.go:133-158` - applyFlowAdjustments

  **Acceptance Criteria**:
  - [ ] `go test ./internal/portfolio/... -run TestSectorRotator` → PASS
  - [ ] 各 flow 的分配調整從 config 讀取

  **QA Scenarios**:
  ```
  Scenario: Flow 調整驗證
    Tool: bash
    Steps:
      1. go test -v ./internal/portfolio/... -run "SectorRotator"
    Expected Result: 測試 PASS
    Evidence: .sisyphus/evidence/task-15-flow-test.txt
  ```

  **Commit**: YES (groups with Task 14)
  - Files: `internal/portfolio/sector_rotator.go`

---

- [ ] 16. 加入跨參數驗證到 Validate()

  **What to do**:
  - 在 `ParametersConfig.Validate()` 中新增驗證邏輯：
    - FactorWeight 基礎權重總和 ≈ 1.0（容許誤差 ±0.001）
    - clamp_min < clamp_max
    - 所有政權調整值在合理範圍內（-0.1 ~ +0.1）
    - 嚴重程度 delta 遞減（critical > high > medium > low）
    - NarrativeConviction 的 theme_hit_rates 值在 [0, 1] 範圍內
  - 每項驗證失敗時回傳有意義的錯誤訊息

  **Must NOT do**:
  - 不改變既有 Validate() 邏輯

  **Recommended Agent Profile**:
  - **Category**: `quick`

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential
  - **Blocks**: Tasks 17-18
  - **Blocked By**: Tasks 7-15

  **References**:
  - `internal/config/parameters.go:610-630` - Validate() 既有邏輯

  **Acceptance Criteria**:
  - [ ] `go test ./internal/config/...` → PASS
  - [ ] 建立測試案例：權重總和 ≠ 1.0 → Validate 回傳錯誤
  - [ ] 建立測試案例：合理配置 → Validate 回傳 nil

  **QA Scenarios**:
  ```
  Scenario: 驗證邏輯正確
    Tool: bash
    Steps:
      1. go test -v ./internal/config/... -run TestValidate
    Expected Result: 所有驗證測試 PASS
    Evidence: .sisyphus/evidence/task-16-validate-test.txt
  ```

  **Commit**: YES
  - Message: `feat(config): add cross-parameter validation for new groups`
  - Files: `internal/config/parameters.go`, `internal/config/parameters_test.go`

---

- [ ] 17. 更新 configs/parameters.json

  **What to do**:
  - 生成或更新 `configs/parameters.json`，包含所有新參數群組
  - 值與目前硬編碼完全一致
  - 格式遵循既有 JSON 結構

  **Must NOT do**:
  - 不改變既有欄位的值

  **Recommended Agent Profile**:
  - **Category**: `quick`

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential
  - **Blocks**: Task 18
  - **Blocked By**: Task 16

  **References**:
  - `configs/parameters.json` - 既有格式
  - `internal/config/parameters_defaults.go` - 預設值來源

  **Acceptance Criteria**:
  - [ ] JSON 格式有效
  - [ ] 包含所有新參數群組

  **QA Scenarios**:
  ```
  Scenario: JSON 格式驗證
    Tool: bash
    Steps:
      1. python3 -m json.tool configs/parameters.json > /dev/null
    Expected Result: JSON 格式有效
    Evidence: .sisyphus/evidence/task-17-json-valid.txt
  ```

  **Commit**: YES
  - Message: `feat(config): update parameters.json with new groups`
  - Files: `configs/parameters.json`

---

- [ ] 18. 更新文件與最終驗證

  **What to do**:
  - 更新 `internal/portfolio/AGENTS.md` §2.4：標記所有權重為「配置驅動」
  - 執行零殘留驗證：grep 原始硬編碼值確認已移除
  - 執行完整 CI：`gofmt`, `go build`, `go test`, `go vet`, `staticcheck`

  **Must NOT do**:
  - 不修改 AGENTS.md 中與本次無關的章節

  **Recommended Agent Profile**:
  - **Category**: `quick`

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential
  - **Blocks**: F1-F4
  - **Blocked By**: Task 17

  **References**:
  - `internal/portfolio/AGENTS.md:64-96` - FactorWeightEngine 文件

  **Acceptance Criteria**:
  - [ ] AGENTS.md 更新完成
  - [ ] `test -z "$(gofmt -l .)"` → 通過
  - [ ] `go build ./...` → 通過
  - [ ] `go test ./...` → 通過（所有測試結果與遷移前一致）
  - [ ] `go vet ./...` → 無新警告
  - [ ] `staticcheck ./...` → 無新問題

  **QA Scenarios**:
  ```
  Scenario: 完整 CI 驗證
    Tool: bash
    Steps:
      1. test -z "$(gofmt -l .)"
      2. go build ./...
      3. go test ./internal/portfolio/... ./internal/orchestrator/... ./internal/config/...
      4. go vet ./internal/portfolio/... ./internal/orchestrator/... ./internal/config/...
      5. staticcheck ./internal/portfolio/... ./internal/orchestrator/... ./internal/config/...
    Expected Result: 全部通過
    Evidence: .sisyphus/evidence/task-18-ci-verify.txt
  ```

  **Commit**: YES
  - Message: `docs(portfolio): update AGENTS.md for config-driven FactorWeightEngine`
  - Files: `internal/portfolio/AGENTS.md`

---

## Final Verification Wave (MANDATORY — after ALL implementation tasks)

> 4 review agents run in PARALLEL. ALL must APPROVE. Present consolidated results to user and get explicit "okay" before completing.

- [ ] F1. **Plan Compliance Audit** — `oracle`
  Read the plan end-to-end. For each "Must Have": verify implementation exists. For each "Must NOT Have": search codebase for forbidden patterns — reject with file:line if found. Check evidence files exist in .sisyphus/evidence/. Verify: no hardcoded values remain in migrated files (grep for original values), all new params have Rationale/Source/Todo, all JSON tags use snake_case.
  Output: `Must Have [N/N] | Must NOT Have [N/N] | Tasks [18/18] | VERDICT: APPROVE/REJECT`

- [ ] F2. **Code Quality Review** — `unspecified-high`
  Run `gofmt`, `go vet`, `staticcheck`, `go build ./...`, `go test ./...`. Review all changed files for: import order violations, missing error wrapping, inconsistent naming, unexported types that should be exported. Check coverage remains ≥40%.
  Output: `Build [PASS/FAIL] | Test [N pass/N fail] | Lint [PASS/FAIL] | Coverage [N%] | VERDICT`

- [ ] F3. **Real Manual QA** — `unspecified-high`
  Start from clean state. Execute EVERY QA scenario from EVERY task. Test cross-task integration: ParameterDefaultsConfig returns complete config, FactorWeightEngine reads from config, NarrativeConvictionModulator reads from config. Test edge cases: config missing → fallback values work, partial config → remaining defaults work. Save to `.sisyphus/evidence/final-qa/`.
  Output: `Scenarios [N/N pass] | Integration [N/N] | Edge Cases [N tested] | VERDICT`

- [ ] F4. **Scope Fidelity Check** — `deep`
  For each task: read "What to do", read actual diff (git log/diff). Verify 1:1 — everything in spec was built, nothing beyond spec was built. Check "Must NOT do" compliance. Detect cross-task contamination: Task N touching Task M's files. Flag unaccounted changes.
  Output: `Tasks [N/N compliant] | Contamination [CLEAN/N issues] | Unaccounted [CLEAN/N files] | VERDICT`

---

## Commit Strategy

| Wave | Tasks | Strategy |
|------|-------|----------|
| 1 | 1-6 | Atomic per task — each audit/struct/test is independent |
| 2 | 7-12 | Atomic per task — each migration is independent, verified by characterization tests |
| 3 | 13-18 | Tasks 13-15 parallel, 16→17→18 sequential — integration chain |
| FINAL | F1-F4 | Parallel reviews, all must APPROVE before user sign-off |

- **Commit message format**: `type(scope): description` (e.g., `feat(config): add FactorWeightParameters struct`)
- **Pre-commit verification**: `go test ./...` for the affected package(s)

---

## Success Criteria

### Verification Commands
```bash
# Zero regression
go test ./...                                 # Expected: all pass, same results as pre-migration

# Build integrity
go build ./...                                # Expected: succeeds, no warnings

# Code quality
test -z "$(gofmt -l .)"                       # Expected: empty (no format issues)
go vet ./...                                  # Expected: no warnings
staticcheck ./...                             # Expected: no new issues

# Zero residual hardcoded values (key checks)
grep -rn "0.25.*0.20.*0.20.*0.15" internal/portfolio/factor_weight_engine.go  # Expected: no match
grep -rn "AI_capex_surge.*0.81" internal/orchestrator/narrative_conviction_modulator.go  # Expected: no match
```

### Final Checklist
- [ ] All "Must Have" present (zero regression, omitempty, Rationale/Source/Todo, cross-param validation)
- [ ] All "Must NOT Have" absent (no existing struct modification, no constant migration, no experimental/cmd touched)
- [ ] All tests pass with identical results pre/post migration
- [ ] `configs/parameters.json` contains all new parameter groups
- [ ] Application starts successfully (`go run ./cmd/atlas`, `/health` returns 200)
- [ ] No hardcoded investment parameter values remain in migrated source files
