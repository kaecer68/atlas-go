# AGENTS.md — internal/orchestrator

本目錄是 `atlas-go` 的大腦核心，負責協調領域專家、控制層過濾與系統擴充外掛。

---

## 核心職責

- **流程協調** (`SystemCore`)：管理模擬生命週期，連結市場資料、實驗狀態與模擬引擎。
- **分層路由** (`executor_*.go`)：依序執行 `Context` (Regime) → `Sector/Style/Superinvestor` (Agent) → `Control` (CRO/CIO) 三層架構。檔案已於 #611 sub-issue-8 拆分為 11 個按關注點分離的同 package 檔案（見下方「Executor 檔案佈局」）。
- **外掛託管** (`PluginHost`)：以統一介面處理 PRISM、Swarm、JANUS 等外部子系統的掛載與生命週期勾子。

## Executor 檔案佈局（sub-issue-8，PR #684）

| 檔案 | 行數 | 內容 |
|------|------|------|
| `executors.go` | 30 | Package doc + 檔案地圖 |
| `executor_types.go` | 93 | 公開型別（LayerRouter / ExecutionContext / ResearchResult / FilterAgentsByLayer）|
| `executor_strategies.go` | 102 | 6 strategy 介面 + 6 default impls |
| `executor_pipeline.go` | 148 | ExecuteWithContext + 5 ExecuteRegistry* wrappers |
| `executor_darwinian.go` | 47 | ExecuteRegistryResearchWithDarwinianWeights + private wrapper |
| `executor_policy.go` | 15 | DefaultExecutionPolicy |
| `executor_muted_filter.go` | 62 | filterMutedAgents + loadRecOverrides |
| `executor_regime.go` | 72 | inferRegime |
| `executor_collection.go` | 431 | collectRecommendations + avgConvictionScore |
| `executor_momentum_crash.go` | 46 | applyMomentumCrashProtection |
| `executor_control.go` | 249 | applyControlLayerWithOutcomes + applyCrowdingPenalty + applyAntiCorrelationLayer + severityForControlAgent + passRatio |
| `executor_symbols.go` | 189 | DefaultSymbols / loadSymbolsFromCSV / ExpandUniverse / RegistrySymbols / SymbolsForSkill / symbolIterator |

修改 executor 邏輯時，請優先定位到對應的關注點檔案，避免在錯誤檔案添加無關代碼。新增 phase helper 時參考既有關注點邊界（muted / regime / collection / momentum_crash / control / symbols）。

---

## 關鍵介面與模式

| 介面 | 職責 | 模式 |
|------|------|------|
| `AgentExecutor` | 負責產生個股推薦 | `Supports(spec) bool` + `Recommend(...)` |
| `RegimeExecutor` | 負責判定當前盤勢 | `Supports(spec) bool` + `Score(...)` |
| `Plugin` | 生命週期外掛 | `Attach(core)` + `ProcessRecommendations` + `PostSimulation` |

### 分層執行順序
1. **Regime 判定**：由 `LayerContext` 代理（如 Macro）計算總分決定 Risk On/Off。
2. **個股篩選**：由 `Screener` 依據 `agents.json` 的 `screening_criteria` 進行靜默過濾。
3. **推薦收集**：由 `Sector/Style` 代理產生原始推薦。
4. **控制過濾**：由 `LayerControl` 代理（CRO/CIO）執行強制阻擋或權重調整。

---

## 統一參數管理 (Parameter Management)

為了提高系統靈活性並確保投資模型的可調試性，所有核心硬編碼參數均已遷移至配置驅動架構：

- **配置來源**：`internal/config/parameters.go` (`ParametersConfig`) 與 `configs/parameters.json`。
- **配置範圍**：
    - `NarrativeConviction`：敘事主題命中率 (`ThemeHitRates`) 與技能映射 (`SkillToTheme`)。
    - `Industry`：行業週期分數 (`PhaseScores`) 與技能映射 (`SkillToIndustry`)。
- **權威準則**：開發者在調整模型參數時，應優先修改 `parameters.json` 而非原始碼。每個參數均包含 `Rationale` (設定理由) 與 `Source` (權威溯源)。

---

## 開發慣例

- **Executor 註冊**：新增 Executor 需在 `plugin_registry.go` 的 `NewPluginRegistry()` 中手動加入對應陣列。
- **因子分數**：所有推薦在進入控制層前，必須經由 `CalculateFactorScoresWithBreakdown` 補完因子細節。
- **原子性**：`System.RunDailySimulation` 應保持無副作用，直到結果寫入 `ledger`。

---

## 陷阱與反模式

- **禁止跨層呼叫**：Executor 不應直接讀取 `SystemCore` 的私有欄位，應透過傳入的 `registry` 或 `quotes`。
- **ID 混淆（已修復，2026-06-05）**：原 `buildFinalRecKey` 以 `Symbol+"|"+Agent` 為 key 進行 `PassedGuards` 查核，但 CIO aggregator 會以 bestAgent 覆寫 Agent 欄位，導致非最佳 agent 原始推薦查核失敗。
  現已改為 `buildPassedSymbolKey`（Symbol-only key），CIO 覆寫不再影響 `PassedGuards` 查核。
  仍應保留原始 Agent ID（`finalRecs[].Agent`），供後端 `recommendation_outcomes.jsonl` 與 `PassedGuards` audit trail 使用——僅僅不再依賴它做查核。
- **靜默過濾**：若標的不符合 `Screener` 門檻，將完全不會進入 `Recommend` 階段，開發時若發現「推薦消失」請優先檢查 `agents.json`。
- **Registry 變更**：修改 `AgentRegistry` 後，必須確認 `ExecuteRegistryResearch` 的路由邏輯能正確匹配新 Layer。
