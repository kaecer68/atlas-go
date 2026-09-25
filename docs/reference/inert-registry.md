# Inert（宣告生效但實際未生效）登記表

| 項目 | 內容 |
|---|---|
| 文件角色 | 「producer 有、consumer 無」「狀態宣稱生效但實際 inert」「死碼」的**單一登記處**，避免同一類缺陷（靜默失效）反覆被發現又重新遺忘 |
| 狀態 | v3（2026-09-25，issue [#1944](https://github.com/kaecer68/atlas-go/issues/1944) Batch 1 + Batch 2 + Batch 3） |
| Batch 2／Batch 3 權威盤點 | [`../specs/industry-allocation-inert-audit-20260924.md`](../specs/industry-allocation-inert-audit-20260924.md)（Batch 2 §4/§5；**Batch 3 §9**：高嚴重項逐項處置、新發現 N-A5、仍未處理清單、可重跑證據） |
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

### Batch 2 剩餘（原清單；**Batch 3 已逐項複核並就地標註**，見下方 Batch 3 表與 spec §4/§5）

- **I4/I12/I13** → **Batch 3 已處理**：I12/I13 接線（共享 dashboard 驅動器與 tracker）；I4 prior 接線、cycle 明示未啟用。
- **I17/I23** → **Batch 3 已處理**：health 明示未知＋消費端 clamp；narrative HitRate 全面加 `hit_rate_source` 來源標記。
- **I25/N-U1/N-U3/N-U4/N-U7** → **Batch 3 已處理**：quote provider 接線、snapshot schema 統一、D6 可達、liquidity skip 可見。**N-U2/N-U5/N-U6 仍未處理**。
- **I5/I6/N-P3/N-P4** → **Batch 3 已處理**（明示未啟用／明示未落地／明示 reserved／明示無模板）。**I19/I32/N-P1/N-P2 仍未處理**。
- **I16/I31** → **Batch 3 已處理**（I16 明示方向不可量測＋對外 scope；I31 判定機讀化，**CI 吞失敗半邊明示未生效**）。**I24 仍未處理**。
- **I15** → **Batch 3 已處理**（明示未啟用）。**I7/I18 仍未處理**。
- **I21/I36/I18/I27/I28/I29/I30**：仍未處理（見 spec §4）。
- **N-C1** → **Batch 3 已處理**（明示未啟用＋防再犯契約）。**N-C3/N-A1..A4 仍未處理**。

> 未修項一律已登記（本表 + spec §4/§5），不留在註解口頭帶過。

## Batch 3（已處置，#1944，2026-09-25）

> 完整證據、逐項 file:line、可重跑命令與「仍未處理」清單見 [`../specs/industry-allocation-inert-audit-20260924.md`](../specs/industry-allocation-inert-audit-20260924.md) §9。本節只登記**處置結論**與機讀旗標。
> 基準：`origin/main@f7fcd74d`；分支 `fix/20260925-inert-batch3`。每項結論皆為「接線／明示未啟用／移除」之一，無一項靜默。

| # | 項目 | 原症狀 | 處置 | 機讀旗標／對外欄位 | 證據 |
|---|---|---|---|---|---|
| **N-C1** | `industry.max_daily_weight_change` 假風控 | config 宣告 5% 日權重變動上限，全 repo 無 reader ⇒ operator 以為有保護 | **明示未啟用**（值 0.05 與行為不變）：`rationale`/`todo` 明寫 `NOT ENFORCED` + 語意未定的原因 | `GET /api/parameters/metadata` 的 `rationale`/`todo` | `TestShippedConfigMaxDailyWeightChangeIsDeclaredUnenforced`、`TestMaxDailyWeightChangeHasNoConsumer`（掃非測試碼 0 reader，reader 出現即紅燈） |
| **I12** | composition 路徑 macro/factor driver 硬寫 0 | 模擬路徑吃 stub，dashboard 的真 SeasonalModulation 只有自己用 | **接線**：新增 `composition.SharedSectorInputs` + `Root.WithSharedSectorInputs`；macro tilt 改由 dashboard `DynamicEnvModulator` 推導 | `SectorFactorDriverWired=false`（factor 仍中立 0，明示） | `TestSetCompositionRoot_SharesDashboardIndustryState`（tilt == modulator tilt == −0.05）、`TestBuildWeightEngine_UsesSharedSectorInputs` |
| **I13** | 兩個 `CycleTracker` 不同步 | root 自建 config-seeded tracker，真資料在 dashboard 另一個實例 | **接線**：`DashboardAPI.SetCompositionRoot`（原本零呼叫者）改為共享 dashboard 的 cycle/seasonal/linkage/macro 輸入；`cmd/atlas/main.go` 首次呼叫 | — | 同上（wiring 後改 dashboard tracker，root engine multiplier 隨之變動）；`-race` 乾淨 |
| **N-C2** | `SetCompositionRoot` 零呼叫者 | 宣告才成立的綁定從未被呼叫 | **已解**（由 I12/I13 一併處理） | — | 見 I12/I13 |
| **I31** | `calibration-validate` 實跑 `OK=false` 但 CI step 仍 success、Slack 未設 | 失敗被吞、零告警 | **判定機讀化**（本批）＋**CI 半邊明示未生效**： nightly job 的 `set +e` + 非最終 `cat` 使 step 恆 success；修法需 `.github/**` 一行（本批 lane 邊界），精確 patch 與前提見 spec §9.2 | `--format=json` 新增 `Findings[]{code,severity,segment,message}`（13 code）；`Issues` 保留、exit 仍 1 | `TestValidateCalibration_FindingsAreClassified`、`TestShippedConfigIntegrityFindingsAreClassified`（出貨 config exact-set） |
| **I25** | 母體 quote provider 寫死 nil ⇒ `symbols_ranked=0` | 排程路徑無 quote ⇒ 全數被量價過濾丟棄 | **接線**：`newUniverseQuoteProvider` → `orchestrator.NewGatewayBackedProvider`；CLI `-build-universe run` 改走同一管線 | snapshot `quotes_status`/`quotes_returned`/`ranked_fallback_reason`/`ranked_trustworthy` | `TestNewUniverseBuilderDeps_WiresRealQuoteProvider`、`TestBuildUniverseQuotesStatusOK`、`TestBuildUniverseNilQuoteProviderIsExplicit` |
| **N-U7** | Layer 2.5 流動性排除從未生效 | nil quote provider ⇒ 檢查永遠 skip 且無痕跡 | **接線**（與 I25 共用 provider）＋**skip 不再靜默**（INFO `RuleDetail`） | `RuleDetail`（`liquidity`） | `TestRiskExclusionLiquidityEvidence`、`TestNewUniverseBuilderDepsWithQuotes_SharesProviderWithRiskFilter` |
| **N-U1** | `--build-universe run` 永遠錯誤 | MockProvider + nil symbols ⇒ 恆失敗 | **接線**（移除 mock 路徑，委派 `monitoring.BuildUniverse`） | untrustworthy 時 CLI 非零退出 | `TestBuildUniverseStatus_ReadsCanonicalSnapshot`／`_RejectsLegacySchema`（CLI 端無端到端測試，弱證據） |
| **N-U3** | 同一 snapshot 兩套 schema（互讀為 0） | 排程與 CLI 各寫一套，讀方靜默 0 | **統一**：`UniverseSnapshotPath`／`SaveUniverseSnapshot`／`LoadUniverseSnapshot` 單一權威 | 舊 schema 讀取回明確 `incompatible schema` | `TestSnapshotSchemaIsSingleAndCanonical`、`TestBuildUniverseStatus_RejectsLegacySchema` |
| **N-U4** | D6 watchlist consumer 不可達 | 依賴恆空 `ranked` ⇒ 門檻永不成立 | **接線**（隨 I25+N-U3 生效）；coverage alert 亦改用 canonical reader | — | `TestD6WatchlistChainReachable`（`consecutive_failures=60` 真的累積） |
| **I16** | 心法 `volatile` 恆 0 命中 | `Evaluate` 只判 up/down；handler 又把 API hit_rate 硬寫 0 | **明示「方向不可量測」**＋對外標籤（volatile 樣本不入命中率分母、不計 miss） | `hit_rate_scope`（`up_down`/`volatile_only`/`unmeasured`）、`hit_rate_source`（`seed`/`feedback_store`/`snapshot_evaluator`）、`volatile_tests` | `TestConditionEvaluator_VolatileNotCountedAsMiss`、`TestToSummary_VolatileFrameNotOverwrittenToZero`、`TestHandlers_ListStrategies_ExposesVolatileScope` |
| **I17** | 季節 per-pattern 校準無 producer；health 對「無觀測」報 critical；4 個超界 `adjustment_factor` 直接相乘 | 假 critical 訊號 + 超界值（含負值）污染調整倍率 | **明示未知**（`unknown`/`reason=no_observations`）＋**消費端 clamp**（負值視為中性 1.0、上限 2.5）＋具名 warn | `adjustment_factor_status`、`out_of_range_patterns`、`calibration_evidence`、`observation_status` | `TestSummarizeCalibrationHealth_NoObservationsIsUnknownNotCritical`、`TestGetPatternAdjustment_ClampsOutOfRangeFactors`、`TestClampAdjustmentFactor` |
| **I23** | narrative 模板/模型 `HitRate` 是手寫常數，卻對外稱「歷史回測」 | 先驗常數冒充量測值 | **接線（來源標記）**：新增 `hit_rate_source` 貫穿 templates/models/events/aggregate | `hit_rate_source`（`handwritten_prior`/`replay_eval_in_memory`/`unavailable_no_samples`/`unavailable_no_template`/`not_populated`）；aggregate `Formula` 帶來源 | `TestDefaultTemplatesCarryPriorHitRateSource`、`TestInvestmentModelsStartAsHandwrittenPrior`、`TestUpdateTemplateHitRatesRelabelsSource`、`TestAggregateHitRateSourceForEvents` |
| **I4** | 預測 prior 恆 0（且 cache miss 會把 prior 洗掉） | `SetStrategicPrior` 零生產呼叫者；rebuild 用 `NewSectorPredictor(&snap, nil)` 覆蓋 | **prior 接線**（rebuild 重新套用）＋**cycle 明示未啟用** | `StrategicPriorApplied()`／`CycleProviderWired()`；`eventdriven.SectorCycleProviderWired=false` | `TestSectorPredictionStatusWiresStrategicPrior`、`TestStrategicPriorDrivesOverallBaselineDriver` |
| **I5** | `SECTOR_PREDICTION_ENABLED` 預設 false、無部署設定、旗標關閉也不告警 | 整條產業預測靜默關閉 | **明示未啟用（機讀）**（預設值未改） | `PredictionReport.SectorPredictionStatus`（`enabled`/`applied`/`days`/`sector_rows`/`strategic_prior_applied`/`cycle_provider_wired`/`reason`）；c07 collector 讀此 reason | `TestProductionSectorPredictionStatusFlagOff`、`TestSectorPredictionStatusJSONContract`；部署檔掃描 0 命中 |
| **I6** | `SectorDayPrediction` 無落地 | 只有 experimental 消費者，ledger 型別無 sector 欄位 | **明示未落地（機讀）**（未動 storage schema） | `SectorPredictionPersisted=false` + `persistence_reason` | `TestSectorPredictionsAreNeverPersisted`、`internal/ledger/event_flow_prediction_sector_gap_test.go`（反射釘住無 sector 欄位） |
| **I15** | `PrimaryMetrics` 只寫不讀 | 宣告了 metric 卻無計算 | **明示未啟用** | `orchestrator.AgentPrimaryMetricsWired=false` | `TestAgentPrimaryMetrics_HasNoNonTestReader` |
| **N-P3** | `NewDriverAdapter` 零非測試呼叫者 | plan/reflect 從未注入 | **明示 reserved**（不移除：唯一實作且有既有測試） | `LLMSectorAgentDriverWired=false` | `TestDriverAdapterReserved_HasNoNonTestCaller`（AST 掃描） |
| **N-P4** | `Theme=semiconductor_cycle_peak` 無模板 | SOX/DRAM 訊號不影響任何產業 | **明示（機讀）** | `ThemesWithoutTemplate`；事件 `hit_rate_source=unavailable_no_template` | `TestThemesWithoutTemplateMatchesKB`、`TestSemiconductorCyclePeakEventCannotReachAnySector` |

### Batch 3 新發現／決策（須另票或另一 lane）

| ID | 內容 | 嚴重度 | 處置 |
|---|---|---|---|
| **N-A5** | `ApplySectorRotation` 是 `ComputeProjectedTarget` 唯一生產呼叫者，卻只帶 `CapitalFlowAction` ⇒ 六個 driver delta map 全空、adapter 永不被呼叫、投影恆等 strategic prior | 高 | **明示未啟用**：`orchestrator.SectorDriverDeltasSupplied=false` + 釘樁測試 `TestApplySectorRotation_SuppliesNoDriverDeltas` |
| I31 CI 半邊 | nightly workflow 吞掉 validate 失敗、Slack 未設即跳過 | 高 | **明示未生效**（`scripts/ci/**`、`.github/**` 屬另一 lane）；精確 patch 見 spec §9.2 |
| 超界 `adjustment_factor` 來源不明 | `cmd/calibrate-seasonal --update` 有守門，故 production 的 4 個超界值來自（或早於）不受守門保護的寫入路徑 | 中高 | 本批只做消費端 clamp；**建議另票追污染源** |
| config validator 允許負 `adjustment_factor` | `parameters_validate.go` 只檢查 `!= 0`（實證有 2 個負值載入成功） | 中 | **刻意未改**（會擋掉現行 production 啟動）；與上一列一起處理 |
| mock provider 被標成可信 | `selectProvider` 無 key 時回 `MockProvider`（假 quote 完整） | 中 | **已接線**：`IsMock()` → `quotes_status=mock`／`ranked_trustworthy=false` |
| `ParameterSnapshot.NarrativeHitRates` 無來源標記 | theme hit rates 來自 config 常數集 | 中 | **未處理**（登記於此） |
| `internal/config/configs/parameters.json` 影子副本 | 與 `configs/parameters.json` 不同、無 Go caller，還會誤導 `findRepoRoot` 探測 | 低-中 | **未處理**（刪除需確認部署腳本未引用） |

### Batch 3 仍未處理（誠實清單）

- **I31 CI 半邊**：需 `.github/workflows/` 一行（加上 freshness 政策裁決：production 主機執行，或 CI 只驗結構）。
- **I24 / I32 / I30**：ledger 口徑與 workflow 寫入票，未動。
- **I17 producer 半邊**：per-pattern 校準仍無寫入者（health 只做到誠實未知）。
- **I4 cycle 半邊**：`SectorCycleProviderWired=false`（predictor 端仍未接權威 tracker）。
- **I23 前端文案**：`shared_web/static/js/pages/narrative.js` 標題仍寫「歷史命中率」（前端 lane）。
- **I16 (a) 選項**：要真的量測 volatile 需產品先凍結「高波動」門檻定義。
- **Batch 2 中／低項**：I7 / I18 / I19 / I21 / I27 / I28 / I29 / I36 / N-C3 / N-U2 / N-U5 / N-U6 / N-A1..A4 仍見 spec §4/§5。

---

