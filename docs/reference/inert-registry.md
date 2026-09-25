# Inert（宣告生效但實際未生效）登記表

| 項目 | 內容 |
|---|---|
| 文件角色 | 「producer 有、consumer 無」「狀態宣稱生效但實際 inert」「死碼」的**單一登記處**，避免同一類缺陷（靜默失效）反覆被發現又重新遺忘 |
| 狀態 | v2（2026-09-24，issue [#1944](https://github.com/kaecer68/atlas-go/issues/1944) Batch 1 + Batch 2） |
| Batch 2 權威盤點 | [`../specs/industry-allocation-inert-audit-20260924.md`](../specs/industry-allocation-inert-audit-20260924.md)（剩餘項、新發現 N-*、可重跑證據） |
| 判定法 | 對每個欄位／參數／旗標問三題：**有 producer 嗎？有 consumer 讀嗎？有測試嗎？** 三者缺一即列入 |
| 上游盤查 | `~/workspace/atlas-notes/03-system-health/2026-09-24-industry-hitrate-survey.md` §6（Q6，I1–I36） |
| 相關規格 | [`../specs/sector-allocation-simulation-closure-spec.md`](../specs/sector-allocation-simulation-closure-spec.md)（§8.3 application truthfulness）、[`../specs/industry-hitrate-metric-spec.md`](../specs/industry-hitrate-metric-spec.md)（命中率口徑） |

> **一句話**：本表只記錄**已判定處置**的 inert 項，並寫明依據（接線／明示未啟用／移除）與證據；尚未處理者一律列在 §Batch 2，不得只用註解口頭帶過。

## Batch 1（已處置，#1944）

| # | 項目 | 原症狀 | 處置 | 證據 |
|---|---|---|---|---|
| I8 | 產業配置閉環 write-only | `SectorAllocationSnapshot.applied` 由 `FileClosureStore.Store()` 硬寫 true；`policy.Consume()` 與 `portfolio.SectorBudgetAllocator` 無生產呼叫者 ⇒ 對外宣稱已套用 | **狀態改由消費證據驅動**（明示未啟用）。`applied=true` 只在有 `ConsumptionReceipt` 時成立；否則 `applied=false` + `fallback_reason=allocator_unavailable`。`Store()` 不再寫入 `applied=true`（連呼叫端硬塞 true 也會被正規化為 false），舊行在讀取時把 `fallback_reason` 的 target 註記搬進 `target_note`。**回滾語意**：`Delete()`（tombstone）的列一律從 `Latest()`／`LatestSnapshot()` 消失，即使曾被消費也不視為消費證據 | `internal/sectorallocation/policy.go`（`ApplicationStatusFor` / `DecorateApplicationStatus`（純函式，回傳複本）/ `RegisterPolicyConsumer` / `ResetPolicyConsumers`）、`internal/orchestrator/strategy_evolver.go`（`ApplySectorRotation` 回 `applied=false`）、`internal/monitoring/api/industry/handlers.go`；測試 `policy_application_status_test.go`、`policy_rollback_and_legacy_test.go`、`strategy_evolver_applied_evidence_test.go`、`handlers_sector_allocation_applied_test.go` |
| I34 | `internal/sim` 的 `rotationFunc` 從未被賦值 | 只有宣告與 nil 守衛，無 setter、無指派 ⇒ 輪動邏輯不可能執行 | **移除**（非接線）。輪動已在推薦層實作（`orchestrator.PortfolioRotator` + `PositionEvaluator`），在 sim engine 內再接一條會產生第二套輪動路徑 | `internal/sim/engine.go`（移除 `RotationFunc` 型別／欄位／呼叫點，改註解指向真正的實作位置）、`internal/sim/testdata/sim_api.golden.json`（API 快照同步） |
| I35 | L1–L5 心法 plugin 是 no-op pass-through | plugin 在生產註冊（`cmd/atlas/main.go` → `WithStrategyTechniques`）且會收 narrative 事件，但 `ProcessRecommendations` 原樣回傳 ⇒ 心法層不影響任何決策 | **明示未啟用**：新增可機讀旗標 `orchestrator.TechniquesLayerActive=false`、attach 時記錄 `pass_through=true`，並在程式碼／本表寫明「看到 strategy_techniques 不等於心法生效」 | `internal/orchestrator/strategy_techniques_plugin.go`；測試 `strategy_techniques_plugin_test.go`（含「回傳同一個 slice」不變式） |
| I20 | 合成報酬取當日 intraday | `syntheticForwardReturn` 用 `(Last-Open)/Open × 0.8` 當「forward return」⇒ 非前瞻、與訊號同源（自我實現），且平盤日 `forwardReturn=0` ⇒ `Hit=false`（必然 miss） | **修正語意**：改為 regime 條件化的確定性 placeholder（`forward_return.risk_on_*`／`risk_off_*`，agent×symbol×交易日為種子），命名改為 `syntheticPlaceholderReturn`，並在註解寫明「不是 forward return，不得進任何命中率聚合」 | `internal/orchestrator/system.go`；測試 `synthetic_placeholder_return_test.go`（同日不同價格走勢必須得到同值、值域、非必然 miss） |
| I1 | `CycleCalibration` 校準權重從未被消費 | `resolveCardConfig()` 無條件回 default，但 `IndustryService.SetCycleCalibration` 的註解宣稱「wires it into the global card builder state」 | **接線，但 config-blocked**：程式已消費 calibration（有 layer metrics 才重新分配權重，且**保留原 funded 權重和 0.85**、殘差補進最大層，避免空窗期把 composite coefficient 整體放大 1/0.85）。**但生產端目前不可能產生 metrics**：`configs/parameters.json` 的 `industry.cycle_calibration` 全 0，`window_size=0` 讓 `RecordOutcome` 每次都清空視窗（`outcomes[len-0:]` = 空），實測 20 次 `RecordOutcome` 後 `GetMetrics()` 仍為空 ⇒ 生產仍走預設權重。此依賴 **I3**（Batch 2 第 4 項） | `internal/industry/cycle_status_card.go`（`resolveCardConfig` / `applyCycleCalibration`，global 加 mutex）；測試 `cycle_status_card_calibration_test.go`（含 `TestCycleCalibration_ZeroWindowSizeConfigBlocksCalibration` 把 config-blocked 狀態釘住） |

### 對外狀態語意（Batch 1 定案）

`sector-allocation-plan` 回應欄位（`internal/sectorallocation`）：

| 欄位 | 語意 | 證據來源 |
|---|---|---|
| `applied` | 該快照是否**真的進入 allocation/order-sizing**（spec §8.3） | 只由 `consumption` 是否存在決定 |
| `consumption` | `ConsumptionReceipt`（誰、何時消費） | `FileClosureStore.Consume()` 寫入的記錄 |
| `fallback_reason` | 未生效的機讀原因（`allocator_unavailable`／`pending_consumption`／`no_simulation_session`／`snapshot_unavailable`） | `ApplicationStatusFor()`；`RegisterPolicyConsumer()` 是否已有 consumer |
| `target_note` | target **計算**退化註記（`no weight engine`、`projection failed: ...`） | 寫入時 provenance，非生效狀態 |

現況（2026-09-24）：沒有任何 producer 呼叫 `RegisterPolicyConsumer`，因此對外一律 `applied=false` + `fallback_reason=allocator_unavailable`；無 session 時維持 `fallback_reason=no_simulation_session`。

## Batch 2（已處置，#1944，2026-09-24）

> 完整證據、差異表與逐項 file:line 見 [`../specs/industry-allocation-inert-audit-20260924.md`](../specs/industry-allocation-inert-audit-20260924.md)（Batch 2 權威盤點）。本節只登記**處置結論**。

| # | 項目 | 原症狀 | 處置 | 證據 |
|---|---|---|---|---|
| I22 | 矽循環三處斷裂：config 被忽略（`_ = cfg`）、capex 訊號硬編碼 ±0.05 永遠跨不過 `< -0.10` 門檻、`window_size=0` 清空相位歷史 | 過熱相位與收縮轉移在生產不可達 | **接線 + 修正**：`getSiliconParams()` 讀 `industry.silicon_cycle`（僅非零覆寫）；capex 優先取 `MacroDataSnapshot.CapexGrowth`（原為有 producer 無 consumer 的欄位），否則用營收 YoY 等比例推估（`capexProxyScale=1.0`、clamp ±0.50）⇒ 衰退 ≥10% 即跨門檻；`HistoryWindowSize<=0` = 不修剪 | `internal/industry/silicon_cycle.go`；測試 `TestGetSiliconParams_ConsumesConfigFile`、`TestExtractSiliconIndicators_CapexReachesCutThreshold`、`TestExtractSiliconIndicators_PrefersSectorDataCapex`、`TestPhaseHistoryWindowZeroDoesNotWipe`、`TestSiliconIndicatorProvenance`。**未修（明示）**：SOX/billings 實為單日變動（`SiliconSOXIndicatorIsYoY=false`）、TW semi index 無 producer（`SiliconTWIndexProducerAvailable=false`） |
| I3 | `industry.cycle_calibration` config 全 0，`WindowSize=0` 使 `RecordOutcome` 每次清空視窗 ⇒ metrics 永遠空 | I1 的接線在生產無證據 | **merge 補預設 + 語意修正**：`mergeIndustryDefaults` 新增 all-zero → 預設（10/0.05/0.55/0.45/0.05/0.40/30）；`WindowSize<=0` = 不修剪；`WeightClampMax<=WeightClampMin` 視為未設定校準（回傳 base weights，不清空） | `internal/config/parameters_merge.go`、`internal/industry/cycle_calibration.go`；測試 `TestMergeIndustryDefaults_CycleCalibrationAllZero`、`TestShippedConfigCycleCalibrationIsUsable`、`TestCycleCalibration_ZeroWindowSizeKeepsOutcomes`。**I1 因此在本批後才真正閉環**（生產 producer = `auto_daily_simulation` → `RecordCycleCalibrationOutcome`） |
| I2 | `cycle_calibrate` 用第三份硬編碼權重呼叫 `CalibrateWeights` 後只 log `len()` 就丟棄 | 算了沒消費 | **明示未啟用（診斷）**：改回報實際生效的 `EffectiveCardConfig()`，log 明寫 `applied=false` / `fallback_reason=diagnostic_only_no_writeback`；刪除硬編碼副本 | `cmd/atlas/calibration_tasks.go`、`internal/industry/cycle_status_card.go`（`EffectiveCardConfig`） |
| I14 | 四個讀取點各讀不同路徑（`data/state/sector_data`、`<ledgerDir>`、`<workDir>/sector_data.json`），實際檔案在 `data/sector_data/`；bridge 僅測試呼叫；缺檔回零 `err=nil` | 通道靜默死亡 | **修正路徑 + 明示未啟用（bridge）**：新增唯一權威 `marketdata.SectorDataDirRel` / `ResolveSectorDataDir()`，四個讀取點改用；provider 記錄載入狀態（`SectorDataState`），apigateway `HealthCheck` 對缺檔／壞時間戳／超過 72h 回 `degraded`；`SectorDataBridgeWired=false`（理由：唯一輸入是無生產刷新者的人工檔，且會把 `EvidenceTier` 由 `estimated` 洗成 `empirical`） | `internal/marketdata/sector_data_provider.go`、`internal/apigateway/adapter_sector_data.go`、`channel_contract.go`、`internal/industry/sector_data_bridge.go`；測試 `TestResolveSectorDataDirMatchesShippedFile`、`TestSectorDataProvider_State*`、`TestSectorDataChannelAdapter_HealthCheck` |
| I10/I11/I33 | `IndustryCycleModulator`/`NarrativeConvictionModulator` 從未註冊（`With*` 只有測試呼叫）；`SetCycleCard` 無生產呼叫者 | 產業相位／主題命中率算完不影響決策 | **明示未啟用**：`orchestrator.ModulatorWiringActive=false` + 理由（wiring 卡在上游輸入：tracker 只有 config seed、narrative hit rate 是手寫常數） | `internal/orchestrator/plugin_registry.go`；測試 `TestProductionRegistryLeavesConvictionModulatorsUnwired` |
| 新 N-C1 | `industry.composite_card` config 已填滿但 `defaultCardConfig()` 回硬編碼副本 | 改 config 無效 | **接線**：`defaultCardConfig()` 疊加 config（空/零值保留預設）；shipped config 與硬編碼值相同 ⇒ 今日行為中性 | `internal/industry/cycle_status_card.go`（`applyCompositeCardConfig`） |
| 新 E1-E3 | 對外硬寫「已生效」：`period_weight_applied: true`（MCP narrative）、`appliedCount++` 不看 `SetParameter` 錯誤、`"calibrated": true` 無條件 | 對外宣稱生效 | **修正**：E1 改 `false` + 誠實 note；E2 先寫入後記錄（全失敗 verdict=`failed`）；E3 改由 `industry.CalibrationApplied()` 推導 | `cmd/atlas-mcp/server/tools_narrative.go`、`internal/config/calibrator.go`、`internal/monitoring/api/industry/handlers.go`；測試 `TestCalibrationApplied_DerivedFromEvidence` |

### Batch 2 剩餘（仍 inert，未修；逐項 file:line 與建議見 spec §4/§5）

- **I4/I12/I13**：產業預測 prior 恆 0、composition 路徑 macro/factor driver 硬寫 0、`CycleTracker` 只有 config seed（且 `SetCompositionRoot` 零呼叫者）。
- **I17/I23**：季節 pattern per-pattern 校準欄位無 producer、narrative 模板 HitRate 是手寫常數卻對外稱「歷史回測命中率」。
- **I25/N-U1..U7**：智慧母體 quote provider 寫死 nil ⇒ `symbols_ranked=0`（生產 artifact 實證）、Layer 2.5 流動性排除從未生效。
- **I5/I6/I19/I32/N-P1..P4**：產業預測整條預設關閉且無落地、LLM sector agent 因 executor 註冊順序不可達、`NewDriverAdapter` 死碼。
- **I24/I31/I16**：`BuildScorecards` 未過濾 synthetic（生產 56%）、`calibration-validate` 實跑 `OK=false` 但 CI 仍 success、心法 `volatile` 恆 0 命中。
- **I21/I36**：選股勝率 executor 在現行資料下 inert（生產者半邊已由 #1949 修）、Darwinian 15/21 agent 無訊號（部分回歸）。
- **I18/I27/I28/I29/I30/I7/I15**：死碼與監控/覆蓋率失效。
- **新 N-C1（`max_daily_weight_change` 假風控，高）**、N-A2/N-A3/A4（applied trace 位置、`RecordSession` 在 applied=false 也計數、preflight 路徑不符）。

> 未修項一律已登記（本表 + spec §4/§5），不留在註解口頭帶過。
