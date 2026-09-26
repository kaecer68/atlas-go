# Inert（宣告生效但實際未生效）登記表

| 項目 | 內容 |
|---|---|
| 文件角色 | 「producer 有、consumer 無」「狀態宣稱生效但實際 inert」「死碼」的**單一登記處**，避免同一類缺陷（靜默失效）反覆被發現又重新遺忘 |
| 狀態 | v4（2026-09-25，issue [#1944](https://github.com/kaecer68/atlas-go/issues/1944) Batch 1 + Batch 2 + Batch 3 + **Batch 4**） |
| Batch 2／3／4 權威盤點 | [`../specs/industry-allocation-inert-audit-20260924.md`](../specs/industry-allocation-inert-audit-20260924.md)（Batch 2 §4/§5；Batch 3 §9；**Batch 4 §10**：長尾逐項處置、I29 殘留診斷、ledger/nightly 誠實聲明、可重跑證據） |
| 靜態閘門一致性 | `scripts/ci/inert-baseline.json` = **179** 筆（Batch 4 前 187）。Batch 4 移除：`config-inert industry.event_sentiment_cap`（已接線）＋7 個 `writer-no-consumer` SAC emitter（`EmitSnapshotStart/Target/Current/Fallback/End`、`EmitPolicyConsumed/Applied`）；`check_inert_closure.sh` exit 0、stale 0 |
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

### Batch 2 剩餘（原清單；Batch 3 已逐項複核並就地標註，**Batch 4 §Batch 4 再複核剩餘項**；見 spec §4/§5/§10）

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
| **I25** | 母體 quote provider 寫死 nil ⇒ `symbols_ranked=0`（生產 1,599 檔實證） | 排程路徑無 quote ⇒ 全數被量價過濾丟棄 | **接線**：`newUniverseQuoteProvider` → `orchestrator.NewGatewayBackedProvider`；CLI `-build-universe run` 改走同一管線；**並改為 chunked fetch**（`QuoteFetchPolicy`／`fetchQuotesChunked`，50 檔／100ms／60s per chunk）以限制全市場抓取的 N+1 成本 | snapshot `quotes_status`（`ok`/`partial`/`empty`/`mock`/`fetch_error`/`provider_unavailable`）、`quotes_returned`、`quotes_requested`、`quotes_chunks`、`quotes_chunks_failed`、`ranked_fallback_reason`、`ranked_trustworthy` | `TestNewUniverseBuilderDeps_WiresRealQuoteProvider`、`TestBuildUniverseQuotesStatusOK`、`TestBuildUniverseNilQuoteProviderIsExplicit`、`TestBuildUniverse_ProductionScaleChunkedFetchRanksSymbols`、`TestBuildUniverseProductionScale_RealSymbolList`（生產 wiring + 真實 1,599 檔清單 → `ranked=150`）、`TestBuildUniverse_PartialQuoteFetchIsExplicit`（partial ⇒ 不信任） |
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
| 全市場 quote 抓取 N+1（fubon-proxy 逐檔 + Hybrid→FinMind 逐檔 fallback） | 一檔不完整 quote 可造成 ~1,599 次 FinMind 請求（≈11% 日配額） | **已緩解**（chunked fetch，fallback 成本限單一 chunk）；provider 內部逐檔行為未改（marketdata lane；spec §9.2/§9.6） |
| `domain.Quote.Volume` 一欄兩種單位（TWSE=股、Fugle/Fubon=張，差 1000×） | 以「張」計的報價把 NT$10M 量價門檻實質變成 NT$10bn ⇒ 中型股以下靜默全滅（`quotes_status` 仍 ok）；live 管線更讓 `shouldReducePosition` 等絕對門檻全滅 | **已由 #1987 根治**：provider 邊界統一為股（fugle/fubon/fugle-ws ×`domain.SharesPerLot`），消費端換算表 `quoteVolumeLotSources`／`quoteVolumeInShares`／`lots_converted` 全數刪除；邊界由 `provider_volume_contract_test.go` 逐 provider 釘住 |
| mock provider 被標成可信 | `selectProvider` 無 key 時回 `MockProvider`（假 quote 完整） | 中 | **已接線**：`IsMock()` → `quotes_status=mock`／`ranked_trustworthy=false` |
| `ParameterSnapshot.NarrativeHitRates` 無來源標記 | theme hit rates 來自 config 常數集 | 中 | **未處理**（登記於此） |
| `internal/config/configs/parameters.json` 影子副本 | 與 `configs/parameters.json` 不同、無 Go caller，還會誤導 `findRepoRoot` 探測 | 低-中 | **未處理**（刪除需確認部署腳本未引用） |

### Batch 3 仍未處理（誠實清單）

- **I31 CI 半邊**：需 `.github/workflows/` 一行（加上 freshness 政策裁決：production 主機執行，或 CI 只驗結構）。
- **I24 / I32 / I30**：ledger 口徑與 workflow 寫入票，Batch 4 仍未動（I30 屬 honesty-batch lane、I32 需 ledger schema 變更；見 §Batch 4）。
- **I17 producer 半邊** → **Batch 4 已接線**：`cmd/calibrate-seasonal --update` 寫 per-pattern `calibration_observations`／`calibration_verdict`／`calibration_timestamp`（見 §Batch 4）。
- **I4 cycle 半邊** → **Batch 4 已接線**：`SectorCycleProviderWired=true`＋`MeasuredCycleProvider`（seed-only 產業維持中性 0.0）（見 §Batch 4）。
- **I23 前端文案**：`shared_web/static/js/pages/narrative.js` 標題仍寫「歷史命中率」（前端 lane）。
- **I16 (a) 選項**：要真的量測 volatile 需產品先凍結「高波動」門檻定義。
- **Batch 2 中／低項** → **Batch 4 已逐項複核**：I18／I27／I28／N-C3／N-U2／N-U5／N-U6／N-A1（7/11）／N-A2／N-A3／N-A4 已接線或移除；I7／I19 殘留／I29 殘留／I36／I21 仍列在 §Batch 4 誠實清單。


## Batch 4（已處置，#1944，2026-09-25）— inert 長尾清掃

> 完整證據、逐項 file:line、可重跑命令與誠實聲明見 [`../specs/industry-allocation-inert-audit-20260924.md`](../specs/industry-allocation-inert-audit-20260924.md) §10。本節只登記**處置結論**與機讀旗標。
> 基準：`origin/main@1bc4b534`；分支 `fix/20260925-inert-batch4`。每項結論皆為「接線／明示未啟用／移除／仍 inert（附理由）」之一，無一項靜默。

| # | 項目 | 原症狀 | 處置 | 機讀旗標／對外欄位 | 證據 |
|---|---|---|---|---|---|
| **I17**（producer 半邊） | per-pattern 校準觀測無寫入者，health 只能報 `unknown` | **接線**（producer） | `cmd/calibrate-seasonal --update` 寫 `calibration_observations`／`calibration_verdict`（`industry.SeasonalVerdict*`）／`calibration_timestamp` | `CalibrationEvidence`（`none`→`measured`）、`VerdictCounts`、`ObservationStatus` | `TestUpdateParametersFileAt_*`、`TestUpdateParametersFileAt_ClosesTheEvidenceLoop` |
| **I4**（cycle 半邊） | `SectorCycleProviderWired=false`，`cycle_position` 永不貢獻 | **接線**（measured-only） | `eventdriven.MeasuredCycleProvider`＋`cmd/atlas` 注入 industry service 的 `CycleTracker`（6h FinMind 寫入、composition root 共用） | `eventdriven.SectorCycleProviderWired` → **true**；`SectorPredictionStatus.CycleProviderWired` 仍由 live predictor 推導 | `TestMeasuredCycleProvider_DropsSeedOnlyIndustries`、`TestSectorPredictionStatusWiresCycleProvider`、`..._CycleProviderSurvivesRebuild` |
| **I7** | `NewSymbolIndustryMapper`／`BuildMapping` 零呼叫者 | **仍 inert（lane 邊界）** | 未動（`internal/marketdata/**` 由 quote-reliability lane #1986 持有） | — | `git grep` 只有定義；另票移除 |
| **I18** | 死碼：`GenerateForwardReturn`、`StockWinRate`、`StrategyWinRate`、`ValidateAllPatterns` | **移除（部分）** | 移除上述四者與其專屬測試；保留仍有使用者的 `hashString`／`ValidateCalibration` 家族 | — | `git grep -nw` 非測試 0 命中 |
| **I19** | LLM sector agent 因 executor 註冊順序不可達 | **接線（部分）＋明示殘留** | LLM agent 註冊順序移到 deterministic 之前；`Supports` 要求兩個 driver 已注入（flag-on 但未接線時不得靜默吃掉 desk） | `LLMSectorAgentDriverWired=false`（殘留）、`resolveAgentExecutor`（first-match-wins 可測） | `TestBuiltinAgentExecutors_LLMSectorAgentPrecedesDeterministic`、`TestResolveAgentExecutor_LLMAgentClaimsDeskOnlyWhenWired`、`TestSemiconductorLLMAgent_Supports_FlagOnWithoutDrivers` |
| **I27** | 三份 `buildSymbolSectorMap` 依賴 Go map 順序（last-write-wins） | **接線** | `industry.BuildSymbolSectorIndex` 單一權威（最深 segment 勝出、同深度比 ID） | `SymbolSectorIndex.MultiAssigned`（模糊歸屬可觀測） | `TestBuildSymbolSectorIndex_DeterministicAcrossRuns`、`_MostSpecificSegmentWins`、`_ReportsMultiAssigned` |
| **I28** | `namespaces.go` 符號無非測試引用 | **移除（部分）** | 移除 `NamespaceKind`／`NamespaceEquityL1`／`NamespaceResearchThemeL2`／`NamespaceStrategyBucket`／`NamespaceAssetClass`／`IsValidNamespace`／`ThemeExposure`／`ValidateThemeExposure`；保留規格守門指名的 `L1FinalTarget`／`ValidateL1FinalTarget` | — | `git grep -nw` 移除項 0 命中 |
| **I29** | 覆蓋率告警是恆等式 | **已修一半（#1983）＋明示殘留** | `universe_scheduler` 半邊已以第一方母體當分母；`cmd/atlas` 半邊（分母＝分類樹代表股 27、分子＝母體 1,599 ⇒ 永不觸發；`snapshotSymbols==0` 被 `>0` 守衛排除）**本批未修**，診斷見 spec §10.1 | — | 唯讀複核（lane B）；修法：改用 `monitoring.CheckUniverseCoverage` 當唯一判準 |
| **I36** | 21 agents 中 15 個 `total_signals=0` | **仍 inert（無生產存取）** | 未動 | — | 需生產資料＋上游 signal 供給盤查（另票） |
| **N-C3** | 影子參數（config 有值、硬編碼才是實作） | **接線 1＋明示 2** | `industry.event_sentiment_cap` 接線（`EventCalendar.sentimentCap()`）；`freshness_scores`／`event_calendar_rules` 標 `NOT WIRED` 並寫明不可原樣接線的原因 | config `rationale` 的 `WIRED`／`NOT WIRED` 宣告 | `TestComputeSentimentAdjustment_ConsumesConfigCap`、`TestShadowParametersDeclarationMatchesConsumers` |
| **N-U2** | `-build-universe` help 列了未實作的 `scrape` | **移除宣告** | help 改 `run\|map\|status` | — | help 字串 diff；unknown 模式已有明確錯誤 |
| **N-U5** | stage3 中性哨兵 0.5 與 ledger 的 neutral=0 不符 | **接線** | 中性改 `0`，且要求 `RecentEventFlowPredictionsActualCount ≥ 5`（padding 不是證據） | 警報 metadata `neutral_sign`／`actual_count` | `TestStage3AlertEvaluator_ModelConfidenceDegraded{,_NoAlertOnPadding,_NilActualCountStaysSilent,_HalfIsNotNeutral}` |
| **N-U6** | `historical_hit_rate` 口徑未揭露（neutral 預測稀釋） | **接線（口徑揭露）** | 新增 `neutral_samples`／`directional_samples`／`hit_rate_basis` | `HitRateBasisDirectionSign`（`t_plus_1_reconciled_direction_sign`） | `TestComputeHistoricalHitRate_DisclosesNeutralBasis`、`_BasisIsSetOnEveryShape` |
| **N-A1** | 11 個 SAC emitter 全無呼叫者（暗啟動從未執行） | **接線 7/11** | `StrategyEvolver.WithSACMetrics`＋`ApplySectorRotation` 發射 snapshot 生命週期與 policy consumed/applied；composition root 注入 `NewSACMetrics(nil)` | `SACMetrics`＋`SACLifecycleObserver`（測試縫） | `TestApplySectorRotation_EmitsSACLifecycleEvents`、`_NilSACMetricsIsSafe`、`_AppliedEmitsPolicyApplied`；其餘 4 個仍列 baseline 並寫明理由 |
| **N-A2** | `macro_flow.applied` trace 在 `ApplyControl` 之前 | **接線** | trace 移到套用後；未套用改發 `macro_flow.skipped`＋`applied=false`＋原因 | trace `Action`／`Data.applied`／`Data.not_applied_reason` | `TestExecuteWithContext_MacroFlowTraceIsRecordedAfterApplyControl`、`..._HonestWhenControlLayerBypassed`、`TestMacroAdjustmentAppliesTo` |
| **N-A3** | `RecordSession` 連 `applied=false` 也計 ⇒ promotion gate 可在 0 applied 場次成立 | **接線** | 新增 `applied_session_count`（只在有 `ConsumptionReceipt` 時遞增）；`IsPromotable()` 要求 ≥20 **applied** 場次 | `sa_closure_state.json` 的 `applied_session_count` | `TestSACClosureStateManager_IsPromotable`（更新：20 筆未消費不得過關） |
| **N-A4** | preflight 檢查 `data/state/...`，實際 writer 在 `data/sector/allocation/` | **接線** | 唯一權威 `sectorallocation.ResolveClosureStoreDir/Path`；preflight／`cmd/atlas`／`dashboard_api` 共用 | — | `TestClosureStorePathMatchesFileClosureStoreWriteTarget`、`TestClosureStoreConstructionUsesSharedResolver`（AST 防漂移） |

### Batch 4 仍 inert（誠實清單）

- **I7**：死碼仍在 `internal/marketdata/symbol_industry_mapper.go`（lane 邊界，另票）。
- **I19 殘留**：production 無 driver 注入點（`LLMSectorAgentDriverWired=false`）＋`llmSectorAgentsPlugin` 仍 pass-through（N-P1）⇒ LLM sector agent 仍未真正接線（旗標預設 false）。
- **I29 殘留**：`cmd/atlas/main.go` 的 `universe_coverage_check` 仍永不觸發（分母/分子不同源、`snapshotSymbols==0` 被守衛排除）。
- **I30**：`.github/workflows/nightly-refresh.yml` 的 backfill 寫入仍被丟棄（`.github/**` 屬 honesty-batch lane）。
- **I32**：ledger event-flow prediction 仍無 `sector` 欄位（與 I6 同源，需 schema 變更）。
- **I36**：`darwinian_weights.json` 15/21 agents 無訊號，需生產資料盤查。
- **I21**：**待觀察**——生產者半邊已由 #1949 修，消費端 source/window 固定（`stockpicker-foreign-3d-net-buy`／`120d`）；本批無生產存取，未驗證 `stock_win_rate` 與 `data/state/stock_flows/` 是否已生成。
- **I27 殘留**：symbol→sector 的值仍是 segment ID（非 canonical L1），未命中仍回 `"other"`。
- **N-A1 殘留**：`snapshot.projection`（需 projector 回傳 clamped 統計）、`legacy.read`、`fallback.count`、`rollback.drill` 四個 emitter 仍無誠實呼叫點（baseline 已寫明理由）。
- **其他 Batch 2 中／低項**：`ParameterSnapshot.NarrativeHitRates` 無來源標記、`internal/config/configs/parameters.json` 影子副本、超界 `adjustment_factor` 污染源、config validator 允許負值、`domain.Quote.Volume` 單位未在 provider 邊界統一、`I23` 前端文案（前端 lane）、`I16(a)` volatile 門檻需產品定案、`N-P1`／`N-P2`／`N-U6` 之外的 ledger／前端項。

---
