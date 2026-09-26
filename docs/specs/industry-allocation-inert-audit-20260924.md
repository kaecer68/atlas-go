# 產業／配置 inert 閉環再盤點（issue #1944 Batch 2，2026-09-24）

| 項目 | 內容 |
|---|---|
| 文件角色 | issue [#1944](https://github.com/kaecer68/atlas-go/issues/1944) Batch 2 的**權威盤點**：剩餘 inert 項、與 Batch 1 的差異、本批處置與證據、以及仍未處理項（誠實列表） |
| 基準 | `origin/main` @ `88683f06`（Batch 1 = PR #1950 已併入；#1951/#1953/#1956/#1958/#1960/#1962 已併入） |
| 上游舊清單 | `~/workspace/atlas-notes/03-system-health/2026-09-24-industry-hitrate-survey.md` §6（Q6, I1–I36）；**本檔為重做，不照抄** |
| 登記表 | [`../reference/inert-registry.md`](../reference/inert-registry.md)（處置結果登記處；本檔為完整證據） |
| 相關規格 | [`sector-allocation-simulation-closure-spec.md`](sector-allocation-simulation-closure-spec.md) §8.3、[`industry-hitrate-metric-spec.md`](industry-hitrate-metric-spec.md) |
| **Batch 3** | 本檔 §9（2026-09-25，`origin/main@f7fcd74d`）：高嚴重項逐項處置、Batch 3 新發現（N-A5、I31 的 CI 半邊等）、仍未處理清單 |
| **Batch 4** | 本檔 §10（2026-09-25，`origin/main@1bc4b534`）：對外誠實與 false claim 修正四項 — I23 前端文案、I24 scorecard synthetic 過濾、I31 CI 半邊＋freshness 政策裁決、`ATLAS_MARKET_DATA_PROVIDER=fubon` 誤導 |

## 0. 判定法（三問檢查）

對每個候選（欄位／參數／旗標／資料路徑）問：

1. **writer/producer 存在嗎？**（有東西會寫這個值，且在**生產**路徑上）
2. **consumer 讀嗎？**（有生產程式碼會讀它、且讀了會影響決策／輸出）
3. **config 旗標有 reader 嗎？狀態欄位有 producer/consumer 配對嗎？**

三者缺一即 inert。嚴重度：**高** = 對外宣稱生效但實際不影響任何決策/輸出且無機讀告警；**中** = 死碼、僅測試覆蓋或監控失效；**低** = 純文件/註解落差。

本批同時複驗兩個特殊類別：

- **硬寫「已生效」**：對外欄位（`applied` / `calibrated` / `*_applied`）是否由實際消費證據推導。
- **config 影子**：`configs/parameters.json` 有的區塊，程式是否真的有 reader。

## 1. 摘要

| 類別 | 數量 | 說明 |
|---|---|---|
| Batch 1 已清（I8/I34/I35/I20/I1） | 5 | 本批複驗仍成立（見 §2） |
| **本批（Batch 2）修復** | 7 | I22（config 接線 + capex 可達）、I3（merge 補預設 + 視窗語意）、I2（診斷不再丟棄計算）、I14（路徑單一權威）、composite_card（config 接 reader）、以及 3 處硬寫「已生效」（E1/E2/E3）→ §3 |
| **明示未啟用（不得靜默，加機讀旗標/理由）** | 3 | I10/I11/I33（modulator）、I14 的 bridge、I2（診斷任務宣告 `applied=false`） |
| **仍 inert（本批未修，誠實列出）** | 22 | I4/I5/I6/I7/I12/I13/I15/I16/I17/I18/I19/I21/I23/I24/I25/I27/I28/I29/I30/I31/I32/I36 → §4 |
| **本批新發現** | 18 | §5：N-C1..C3（config 影子／假風控）、N-U1..U7（母體管線）、N-P1..P4（預測/LLM）、N-A1..A4（applied trace／死欄位） |
| 已由其他 PR 修好 | 1 | I26（#1951，44/44 全解出）→ §2 |

生產影響的一句話總結：**產業/配置相關的「宣稱生效」對外欄位，現在全部由消費證據或明示未啟用驅動；上游真正缺的是資料源（母體 quote provider、sector_data 刷新、cycle tracker 真資料），不是接線程式碼。**

> **Batch 3（2026-09-25）見 §9**：Batch 2 §4/§5 列為高嚴重而未處理者（I25、N-C1、N-U7、I12、I13、I31、I16、I23、I4、I5、I6、I17）已逐項給出結論並附證據，另含本批新發現（N-A5：`ApplySectorRotation` 供給的 driver delta 全空 ⇒ 投影恆等 strategic prior）。下表 §4/§5 的列已就地標註 `→ **Batch 3：…（§9）**`。


## 2. 與 Batch 1 的差異

### 2.1 Batch 1 已清項目（本批複驗仍成立）

| ID | Batch 1 處置 | 本批複驗 |
|---|---|---|
| I8 | `applied` 改由 `ConsumptionReceipt` 驅動；`Store()` 強制 `applied=false` | **成立**。全鏈複驗：`policy.go:135`（Store 正規化）、`ApplicationStatusFor`/`DecorateApplicationStatus`（`policy.go:415-455`）、`strategy_evolver.go`（`applied=false`）、`monitoring/api/industry/handlers.go`（兩處硬寫皆為 `false`）。**無殘留硬寫 true** |
| I34 | 移除 `sim.Engine.rotationFunc` 死碼 | 成立（全 repo 無 `rotationFunc` 非註解引用） |
| I35 | `TechniquesLayerActive=false` + `pass_through=true` log | 成立 |
| I20 | 合成報酬改 `syntheticPlaceholderReturn` | 成立。**副作用**：`internal/orchestrator/forward_return_fallback.go` 的 `GenerateForwardReturn` 因此變成確定死碼（I18，§5） |
| I1 | `CycleCalibration` 權重由卡片建構期消費（有 metrics 才重分配） | 接線成立，但**當時無證據**：`WindowSize=0` 使視窗每次被清空。本批修 I3 後，生產確實有 producer（`cmd/atlas/main.go:1668` 的 `auto_daily_simulation` → `RecordCycleCalibrationOutcome`），因此 **I1 在本批後才真正閉環**（≥10 場次後 `CalibrationApplied()` 會變 true）；根因見 §3.2 |

### 2.2 舊清單說 inert、現在其實已修好

| ID | 舊判定 | 現況 | 修好的 PR |
|---|---|---|---|
| I26 | `SymbolL1Mapper` 只解出 18 支/5 產業，tree-only L1 全落空 | **FIXED**：兩層解析（rank0 結構祖先 / rank1 顯式映射）+ 未解出者顯式回報。實測 `go test ./internal/industry/ -run TestProductionClassificationTree -v` → `27 L1 representative symbols, 27 mapped to 9 canonical L1 (ratio 1.0000)`、`44 declared symbols overall, 44 mapped (ratio 1.0000)` | #1951 |

### 2.3 舊判定方向對、但性質改變者

| ID | 變化 |
|---|---|
| I18 | `GenerateForwardReturn` 由「語意可疑的預留」變成**確定死碼**（I20 已由 placeholder 取代） |
| I2 | 由「算了只 log」變成「與 I1 接線重複的冗餘計算」 |
| I31 | 由中升為**高**：`cmd/calibration-validate` 實跑 `OK=false`（7 個 segment 無代表股、exit 1），而 CI job 仍 success 且 `SLACK_WEBHOOK_URL` 為空 → 零告警 **Batch 3：判定機讀化；CI 吞失敗半邊明示未生效（§9.2 附精確修法）** |
| I19 | 舊描述不完整：`use_llm_sector_agents=true` 仍無效，真正原因是 executor first-match-wins（`SemiconductorExecutor` 先註冊且 `Supports` 恆真） |
| I21 | 舊證據已過期：#1949 已修生產者半邊（`main.go:1449` 排程、`conditions.go:162` 已註冊 `foreign-3d-net-buy`）；缺口轉為「生產未驗」 |


## 3. Batch 2 處置與證據

每個高嚴重項的結論（接線／明示未啟用／移除，**無一項靜默**）與可重跑證據如下。
測試名稱皆可直接 `go test ./internal/<pkg>/ -run <TestName> -v` 複跑。

### 3.1 I22 矽循環三處斷裂 → **接線 + 修正（可達性）**

| 子缺陷 | 舊況（現行 main 實證） | 本批處置 | 證據 |
|---|---|---|---|
| config 被忽略 | `getSiliconParams()` 內 `_ = cfg`，註解寫「pending integration」；但 `params.Industry.SiliconCycle`（`parameters.go:786`）存在且 `configs/parameters.json` 已填滿（`revenue_yoy_threshold 0.15` … `capex_cut_threshold 0.1`） | **接線**：逐欄讀 config，僅在非零時覆寫預設（全 0 舊 config 不會把門檻打成 0） | `internal/industry/silicon_cycle.go`（`getSiliconParams`）；`TestGetSiliconParams_ConsumesConfigFile`、`TestGetSiliconParams_AllZeroConfigKeepsDefaults` |
| capex 條件永不成立 | `ExtractSiliconIndicators` 用硬編碼 `±0.05` 當 capex 訊號，但兩條收縮轉移的條件是 `< -CapexCutThreshold`（預設 `0.10`）⇒ 1→3 與 2→3 **永不觸發** | **修正語意**：優先取 sector_data 的真實 `CapexGrowth.Value`（原本是「有 producer 無 consumer」的欄位），否則用營收 YoY 的**等比例**推估值（`capexProxyScale=1.0`、clamp ±0.50）⇒ 營收 YoY 衰退 ≥10% 即跨過預設門檻 | `silicon_cycle.go`（`capexProxyScale` / `capexProxyClamp` / `ExtractSiliconIndicators`）；`TestExtractSiliconIndicators_CapexReachesCutThreshold`（真實 snapshot 兩步 → `PhaseExpansionConfirmed` → `PhaseContraction`）、`TestExtractSiliconIndicators_PrefersSectorDataCapex`；既有測試期望值同步修正（`coverage_push_test.go` 0.05 → 0.25，該值原本編碼了 bug） |
| `window_size=0` 清空歷史 | `DetectPhase` 的 `e.history[len-0:]` = 空 ⇒ 零 config 每次清空相位歷史 | **修正語意**：`HistoryWindowSize <= 0` 代表「不修剪」 | `silicon_cycle.go`；`TestPhaseHistoryWindowZeroDoesNotWipe` |
| **未修（明示）** | `PhiladelphiaSOXIndexYoY` / `GlobalSemiconductorBillingsYoY` 實際是**單日**變動（Yahoo `^SOX` provider，`yahoo_session.go:32` `range=5d`），與名字宣稱的年增率不符，0.40/0.10 門檻語意失真；`TaiwanSemiconductorIndexMA` 無任何 producer（全 repo 只有 struct 與 merge 觸及 `MacroDataSnapshot.TaiwanSemiIndex`）⇒ 1→2 的 MA 分支不可達 | **明示未啟用（機讀常數）**：`SiliconTWIndexProducerAvailable=false`、`SiliconSOXIndicatorIsYoY=false` + doc block；不改門檻（改門檻等於用單日資料冒充年資料） | `silicon_cycle.go`（常數區塊）；`TestSiliconIndicatorProvenance` |

> 影響：`internal/monitoring/dashboard_api.go` 與 `IngestAndUpdateMacro` 皆在生產路徑餵 `ExtractSiliconIndicators`，故本項是**真實生產行為修正**（原本過熱相位只能靠 SOX 分支，而該分支門檻不可達）。

### 3.2 I3 CycleCalibration config 全 0 + 視窗被清空 → **接線（merge 補預設）+ 修正語意**

- 實證：`configs/parameters.json → industry.cycle_calibration.value` 全 0（`min_samples/learning_rate/hit_rate_*/weight_clamp_*/window_size`），`mergeIndustryDefaults`（`parameters_merge.go:462`）**無** CycleCalibration 分支 ⇒ Go 預設（10/0.05/0.55/0.45/0.05/0.40/30）永不生效。
- 處置：
  1. `parameters_merge.go` 新增 all-zero → 預設的 merge 規則（與 `SiliconCycle` 同慣例；部分設定者不覆寫）。
  2. `cycle_calibration.go`：`WindowSize <= 0` = 不修剪視窗（原 `outcomes[len-0:]` 清空）。
  3. `CalibrateWeights`：退化 clamp 視窗（`WeightClampMax <= WeightClampMin`）視為「未設定校準」，回傳 base weights（否則有 metrics 的層會被 clamp 成 0，反而清空權重）。
- 證據：`TestMergeIndustryDefaults_CycleCalibrationAllZero`、`TestMergeIndustryDefaults_CycleCalibrationPartialOverride`、`TestShippedConfigCycleCalibrationIsUsable`（**讀真實 `configs/parameters.json`**，斷言 `WindowSize>0`、`MinSamples>0`、clamp 非退化）、`TestCycleCalibration_ZeroWindowSizeKeepsOutcomes`（Batch 1 的 bug-pin 測試依其自身註解改寫為新語意）、`TestApplyCycleCalibration_AllZeroWeightsFallsBackToDefaults`。

### 3.3 I2 `cycle_calibrate` 算了只 log → **明示未啟用（診斷任務）**

- 舊況：`cmd/atlas/calibration_tasks.go:312` 用第三份硬編碼 layer weights 呼叫 `CalibrateWeights`，結果只取 `len()` 進 log 就丟棄；實際生效的權重由 `resolveCardConfig`/`applyCycleCalibration` 產生（Batch 1 I1）。
- 處置：改為回報**實際生效**的 `EffectiveCardConfig()`，並在 log 明寫 `applied=false`、`fallback_reason=diagnostic_only_no_writeback`；刪除硬編碼副本。
- 證據：`internal/industry/cycle_status_card.go`（新 exported `EffectiveCardConfig`、`CalibrationApplied`）、`cmd/atlas/calibration_tasks.go`。

### 3.4 對外硬寫「已生效」逐一清查 → **修正 3 處**

| # | 位置 | 舊況 | 處置 | 證據 |
|---|---|---|---|---|
| E1 | `cmd/atlas-mcp/server/tools_narrative.go:73` | `period_weight_applied: true` 無條件；且所指機制在生產不可達（`narrative/detector.go` 才套 PeriodWeight，`DetectEvents` 無 period 參數，`template_detector_scan.go` 從未設 `CurrentPeriod`） | 改 `false` + 誠實 note（週期僅情境標註） | `tools_narrative.go`；`go test ./cmd/atlas-mcp/server/` |
| E2 | `internal/config/calibrator.go:187` | `appliedCount++` 不看 `SetParameter` 回傳值 ⇒ 寫入失敗仍報 `applied N/M`、`Verdict=calibrated` | 先寫入後記錄；失敗 `continue` 並計 `failedCount`；全失敗時 verdict=`failed` | `calibrator.go`；`go test ./internal/config/` |
| E3 | `internal/monitoring/api/industry/handlers.go:412` | `"calibrated": true`（只要 metrics map 非 nil） | 改由 `industry.CalibrationApplied()`（實際是否重分配權重）推導，並另出 `calibration_metrics` | `handlers.go`、`cycle_status_card.go`；`TestCalibrationApplied_DerivedFromEvidence` |

**其他對外 `applied`/`fallback_reason` 全數無殘留硬寫**（複驗範圍：`internal/sectorallocation`、`internal/monitoring`、`internal/orchestrator`、`cmd/atlas-mcp`、前端 JS）：`projector.go` 的 `ProjectedTarget.FallbackReason` 為死欄位（無 producer/consumer，非狀態冒充）；前端無任何 JS 讀 `applied`/`fallback_reason`。

### 3.5 I14 `sector_data` 路徑不符 → **修正路徑 + 明示未啟用（bridge）**

- 實證（唯一權威常數 `marketdata.SectorDataDirRel` 的新註解即記錄此表）：
  - `dashboard_api.go:270`、`register_adapters.go:268` 讀 `<workDir>/data/state/sector_data/sector_data.json`
  - `system.go:424` 傳 `cfg.LedgerDir`（預設 `data/state`）⇒ 讀 `<workDir>/data/state/sector_data.json`
  - scheduler deps 讀 `<workDir>/sector_data.json`
  - **實際存在的檔案**是 `<workDir>/data/sector_data/sector_data.json`（`git ls-files` 證實，`updated_at 2026-05-12`）
- 處置：
  1. 新增 `marketdata.SectorDataDirRel` + `ResolveSectorDataDir(workDir)`，四個讀取點全部改用。
  2. provider 保留 graceful degradation（缺檔仍回零 + nil error）但**記錄載入狀態**（`SectorDataState{Path,Found,DataUpdatedAt,Reason}`），apigateway `HealthCheck` 據此回 `degraded`（缺檔／時間戳壞／超過 72h），不再對死通道回 `ok`；contract 補上 `FreshnessWindow=72h`。
  3. `BridgeSectorDataToCycleTracker` 維持**未接線**並明示：`industry.SectorDataBridgeWired=false`，理由寫在常數註解（唯一輸入是無生產刷新者的人工檔；且 `UpdatePosition` 會把 `EvidenceTier` 從 `estimated` 變成 `empirical`，等於把過期人工檔洗成「實證」）。
- 證據：`TestResolveSectorDataDirMatchesShippedFile`（權威路徑指向 repo 實際檔案，且三個舊路徑皆不存在）、`TestSectorDataProvider_StateReports{Freshness,Missing,BadInput}`、`TestSectorDataChannelAdapter_HealthCheck/{fresh_file_is_ok,stale_file_is_degraded,missing_file_is_degraded}`。

### 3.6 新發現：`industry.composite_card` 有 config 無 reader → **接線**

- 舊況：`configs/parameters.json → industry.composite_card` 已填滿（layer_weights/sentiment_thresholds/clamp 0.8-1.2），但 `defaultCardConfig()`（`cycle_status_card.go`）回傳硬編碼副本，註解甚至寫「future: add a CompositeCard ParameterMetadata」（該欄位**早已存在**）⇒ 改 config 完全無效。
- 處置：`defaultCardConfig()` 疊加 config（空/零值保留預設，避免全零 config 讓 clamp 退化成 [0,0]）；因 shipped config 與硬編碼值完全相同，**今日行為中性**。
- 證據：`internal/industry/cycle_status_card.go`（`applyCompositeCardConfig`）；既有卡片/校準測試全綠 + `go test ./internal/industry/`。

### 3.7 I10/I11/I33 modulator 未註冊 → **明示未啟用**

- 舊況：`IndustryCycleModulator` / `NarrativeConvictionModulator` 的消費端（`executor_collection.go:289-330`：產業相位 delta、CycleStatusCard 綜合情緒、narrative 主題加成）完整存在，但 `WithCycleModulator` / `WithNarrativeModulator` 的唯二呼叫者在測試 ⇒ 產業命中率/相位算完不影響任何決策。
- 處置（**不接線，理由寫在程式碼**）：`orchestrator.ModulatorWiringActive=false` + 註解說明 wiring 卡在**輸入**而非程式碼（composition 路徑的 `CycleTracker` 只有 config seed；`SetCycleCard` 無生產呼叫者；narrative 主題命中率是手寫常數 I23）。以測試釘住事實，翻轉常數時必須同步改測試與本文件。
- 證據：`internal/orchestrator/plugin_registry.go`、`plugin_registry_modulator_wiring_test.go`（`TestProductionRegistryLeavesConvictionModulatorsUnwired`、`TestSetCycleCardIsNilByDefaultInProductionShape`）。


## 4. 仍 inert 清單（本批未修，逐項證據）

> 這些項目**不再靜默**：全部登記於本節與 [`inert-registry.md`](../reference/inert-registry.md) §Batch 2 剩餘。

| ID | 位置（現行 file:line） | 嚴重度 | 現況證據 | 建議處置 |
|---|---|---|---|---|
| I4 | `internal/narrative/sector_predictor.go:49-61`；`internal/eventdriven/handler.go:341` | 中高 | 生產 `NewSectorPredictor(&snap, nil)`；`SetStrategicPrior`/`SetCycleProvider` 零生產呼叫者 ⇒ `overall_baseline` 與 `cycle_position` 貢獻恆 0 | 接線（注入 prior/cycle）或移除該欄位 **Batch 3：prior 已接線、cycle 明示未啟用（§9）** |
| I12 | `internal/orchestrator/composition/root.go:217-229` vs `internal/monitoring/dashboard_api.go:546-551` | 高 | composition 路徑的 macro/factor driver 硬寫回 0；且 `SetCompositionRoot`（`dashboard_api.go:213`）**零呼叫者** ⇒ 模擬決策實際吃 stub，dashboard 的真 SeasonalModulation 只有自己用 | 接線 `SetCompositionRoot`（或刪 stub 路徑），兩條路徑輸入需一致 **Batch 3：已接線（共享 dashboard 驅動器）（§9）** |
| I13 | `internal/orchestrator/composition/root.go:171` | 高 | `industry.NewCycleTracker()` 只有 config seed；真資料寫在 dashboard 的另一個實例（`dashboard_api.go:355`）⇒ 兩個 tracker 不同步 | 共用單一 tracker 實例（與 I12 同批） **Batch 3：已共用單一 tracker（§9）** |
| I17 | `internal/industry/seasonal_health.go:107-124,176-186` | 高 | 消費者讀 `calibration_observations/verdict/timestamp`，全 repo 無 producer；實測 health=critical、`total_observations=0`、`verdict={unknown:13}`；4 個 `adjustment_factor` 超出 [0.3,2.5]（含負值）而 `seasonality.go:235` 直接相乘 | 補 per-pattern 校準寫入，或讓 health 對「無觀測」明示（非 critical 假訊號） **Batch 3：health 明示未知＋消費端 clamp＋具名告警（§9）** |
| I25 | `cmd/atlas/bootstrap_helpers.go:209`（`Quotes: nil`）、`universe_builder.go:416-431` | 高 | 生產 `universe_snapshot.json`（`2026-09-24T06:00:23Z`）：`symbols_built 27 / symbols_ranked 0`、`ranked: []`；根因是排程路徑 quote provider 寫死 nil ⇒ 全數被量價過濾丟棄；下游 watchlist `symbols: null` | 接線真實 quote provider（與新發現 N-U1/N-U4 同批） **Batch 3：已接線真實 quote provider（§9）** |
| I27 | `monitoring/api/live/handlers.go:609`、`monitoring/service/live.go:489`、`monitoring/dashboard_dataloaders.go:84` | 中 | 三份 `buildSymbolSectorMap` 仍 `GetAllSegments()`（Go map range 無序）+ last-write-wins；config 有 6 支股票多重歸屬；未命中回 `"other"`（非 canonical） | 改排序或改讀 canonical 映射（#1943 已提供機制，只讀不改） |
| I28 | `internal/sectorallocation/namespaces.go:9-95` | 中 | `NamespaceKind`/`L1FinalTarget`/`ThemeExposure` 非測試引用 0（只有定義 + 測試） | 移除或標明 reserved |
| I29 | `internal/monitoring/universe_scheduler.go:761-783`（`:774` `total = mapped`） | 中 | 覆蓋率告警是恆等式（ratio 恆 1.0）；`main.go:1937-1951` 版分子分母同源 | 分母改用可交易母體全量 |
| I30 | `.github/workflows/nightly-refresh.yml`（無 commit/push step） | 中 | `backfill-industry-tree` 每夜在 runner 內寫 `configs/parameters.json` 後丟棄；後續 validate 讀未改動的 checkout | 移除或改為開 PR |
| I31 | `cmd/calibration-validate`、`internal/config/integrity.go:189-231` | 高 | 實跑 `OK=false`（7 個 segment 無代表股、`updated_at` 逾 48h、exit 1），而 CI job 仍 success，且 `SLACK_WEBHOOK_URL` 為空 ⇒ 零告警 | 補代表股 + 讓 CI 真的紅（或明示 advisory） **Batch 3：判定機讀化；CI 吞失敗半邊明示未生效（§9.2 附精確修法）** |
| I32 | `internal/ledger/event_flow_prediction_store.go:24-40` | 中 | 事件流預測紀錄仍無 `sector` 欄位（生產 35 筆 key 實證）；另有 #1948 的 `industry_winrate` 表面（不同口徑，勿混淆） | 落地時補 sector 欄位 |
| I21 | `internal/orchestrator/stockpicker_winrate_executor.go:73,129-139` | 中 | executor 零值建構 ⇒ source 固定 `stockpicker-foreign-3d-net-buy`；`stock_win_rate` 表中只有 momentum/price-volume 三源（`LoadWinRate found=false`）；`data/state/stock_flows/` 不存在 ⇒ flow gate fail-closed。生產者半邊已由 #1949 修（排程 + 條件註冊） | 生產跑一輪後複驗；必要時補 win-rate 校準產物 |
| I18 | `internal/industry/validate*.go`、`stockpicker/winrate.go:147,153`、`orchestrator/forward_return_fallback.go` | 中 | `ValidateAllPatterns`/`StockWinRate`/`StrategyWinRate` 連測試都沒有；`GenerateForwardReturn` 已確定死碼 | 移除死碼（本批未動以免擴大 diff） |
| I19 | `internal/orchestrator/plugin_registry.go:475-483`、`plugin_sector.go:14-16` | 中 | `use_llm_sector_agents=true` 仍無效：executor first-match-wins，`SemiconductorExecutor` 先註冊且 `Supports` 恆真 ⇒ LLM sector agent 不可達 | 修註冊順序/`Supports` 條件，或移除旗標 |
| I16 | `internal/strategy_techniques/evaluator.go:157-160` | 高 | `Evaluate` 只判 Up/Down ⇒ `Direction=volatile` 恆 0 命中；`strategies/handlers.go:163` 又會把 API hit_rate 覆寫成 0（seed 0.55 → 0） | 補 volatile 判定或明示該類樣本不計 **Batch 3：明示「方向不可量測」＋對外 `hit_rate_scope`/`hit_rate_source`（§9）** |
| I23 | `internal/narrative/knowledge_base.go:138,150,162`、`templates.go`、`structural_trend.go` | 高 | 模板/模型 HitRate 為手寫常數，卻以「歷史回測命中率」對外呈現（UI + 文件）；runtime EMA 只存記憶體 | 改標示為「先驗常數」或落地校準 **Batch 3：已加來源標記（`hit_rate_source`）（§9）** |
| I24 | `internal/ledger/ledger.go:450` vs `portfolio/darwinian_period_matrix.go:112` | 中高 | `BuildScorecards` 不過濾 `IsSynthetic`；實測 `recommendation_outcomes` 45668 列中 synthetic 25571（56.0%）；postgres slim query 選了欄位卻不 WHERE | 加過濾 + 對外揭露 `synthetic_share` |
| I5 | `internal/config/config.go:76,156`；`cmd/atlas/main.go:1022-1027` | 高 | `SECTOR_PREDICTION_ENABLED` 預設 false、無部署設定 ⇒ `predictor.go:200` 固定回空陣列（測試斷言 0 筆）；旗標關閉時 c07 collector 也不發告警 | 明示未啟用（env 預設 + 機讀 reason）或開旗標 **Batch 3：明示未啟用＋對外 status（§9）** |
| I6 | `internal/eventdriven/types.go:101` | 高 | `SectorDayPrediction` 只有 cmd/experimental/c07-* 消費者；ledger 落地型別無 sector 欄位 | 落地 + 對帳（與 I32 同批） **Batch 3：明示未落地（機讀）（§9）** |
| I7 | `internal/marketdata/symbol_industry_mapper.go:182` | 中 | `BuildMapping`/`NewSymbolIndustryMapper` 零呼叫者（生產走 `monitoring.NewTreeBasedMapper`） | 移除死碼 |
| I15 | `configs/agents.json`、`orchestrator/registry.go` | 中 | `PrimaryMetrics` 只寫不讀（0 reader）；`alpha_hit_rate` 全 repo 無計算 | 實作或移除宣告 **Batch 3：明示未啟用（`AgentPrimaryMetricsWired=false`）（§9）** |
| I36 | `docs/decisions/2026-08-21-performance-root-cause-audit.md §2.1` | 中 | 生產 `darwinian_weights.json`（`2026-09-24T05:56:41Z`）21 agents 中僅 6 有訊號、15 個 `total_signals=0` 且權重凍在 0.3 下限 ⇒ 部分回歸（文件稱 08-15 已修） | 另開票追上游 signal 供給 |

## 5. Batch 2 新發現（舊 Q6 清單漏列）

| ID | 位置 | 嚴重度 | 現況證據 | 建議處置 |
|---|---|---|---|---|
| N-C1 `max_daily_weight_change` | `internal/config/parameters.go:770`；`configs/parameters.json:5079` | 高 | config 宣告「Maximum 5% daily weight change to prevent excessive volatility」，但**全 repo 無 reader、無實作** ⇒ 假風控（operator 以為有保護） | 實作每日權重變動 clamp，或移除該參數（不得留在 config 假裝生效） **Batch 3：明示未啟用＋防再犯契約（§9）** |
| N-C2 `SetCompositionRoot` 零呼叫者 | `internal/monitoring/dashboard_api.go:213` | 中 | 宣告才成立的 composition 綁定從未被呼叫 ⇒ I12/I13 的 stub 輸入是**生產實際**行為 | 與 I12/I13 同批接線 **Batch 3：已解（SetCompositionRoot 已接線）（§9）** |
| N-C3 影子參數群 | `industry.event_sentiment_cap` / `event_calendar_rules` / `freshness_scores` | 低-中 | 皆有 config 值但硬編碼副本才是實際使用者 | 接線或標 `deprecated` |
| N-U1 `--build-universe run` 永遠錯誤 | `cmd/atlas/cmd_universe.go:87-94` | 中 | MockProvider + nil symbols ⇒ 該子命令恆失敗 | 修或移除子命令 **Batch 3：已修（§9）** |
| N-U2 未實作的子命令 | `cmd/atlas` help 的 `scrape` | 低 | help 列了但未實作 | 移除宣告 |
| N-U3 同一 snapshot 兩套 schema | 排程寫 `result/ranked` vs CLI 寫 `build_time/ranked_count` | 中 | 互讀為 0 且靜默 | 統一一套 schema **Batch 3：已統一為單一 schema（§9）** |
| N-U4 D6 watchlist consumer 不可達 | `cmd/atlas/main.go:2870` | 中 | 依賴恆空的 `ranked` ⇒ 需 `ConsecutiveFailures>=60` 永不成立 | 與 I25 同批 **Batch 3：已可達（§9）** |
| N-U5 stage3 中性哨兵不可達 | `universe` stage3 `evaluateModelConfidenceDegraded` | 中 | 用 0.5 當中性哨兵，但實際中性為 0 ⇒ 告警不可達 | 修門檻 |
| N-U6 `historical_hit_rate 12.1%` | 對外欄位 | 中 | 為 producer 寫入 neutral 的產物（33 對帳/4 命中） | 對外揭露口徑 |
| N-U7 Layer 2.5 流動性排除從未生效 | Layer 2.5 以 nil `QuoteProvider` 建構 | 高 | 流動性排除條件永不成立 | 接線 quote provider（與 I25 同批） **Batch 3：已接線＋skip 可見（§9）** |
| N-P1 LLM sector agent 路徑空轉 | `internal/orchestrator/factory.go:128`、`plugin_adapters.go:292-304` | 中 | 建構後即丟棄、`return recs` | 與 I19 同批 |
| N-P2 `sector_predictions` 無前端 consumer | `valid_fields.json:1667` 僅型別鏡射 | 中 | 無 UI 讀取 | 明確標示未使用 |
| N-P3 `NewDriverAdapter` 零非測試呼叫者 | `internal/llm/llm_driver_adapter.go:43` | 中 | plan/reflect 從未注入 | 移除死碼 **Batch 3：明示 reserved（§9）** |
| N-P4 `Theme=semiconductor_cycle_peak` 無模板 | `internal/narrative/narrative_detectors.go:1062-1104` | 中 | hitRate=0 且 `MatchChains` 不產鏈 ⇒ SOX/DRAM 訊號不影響任何產業 | 補模板或移除 detector **Batch 3：明示（機讀）（§9）** |
| N-A1 11 個 SAC emitter 無呼叫者 | `internal/orchestrator`（`EmitPolicyApplied` 等） | 低 | 全無呼叫者 | 移除 |
| N-A2 `macro_flow.applied` trace 位置 | `executor_pipeline.go:119` | 低 | 在 `ApplyControl` 之前發出，且 `RequireCROPass=false` 時並不真的套用（生產預設 true，故目前為真） | 移到套用後 |
| N-A3 `sa_closure_state.json` 欄位無 consumer | `internal/sectorallocation/closure_state.go:167-200` | 中 | `RecordSession` 在 `applied=false` 也照計 ⇒ promotion gate 可在 0 個真正 applied 場次成立（Batch 1 已記錄 SA11.A） | 另票改計數語意 |
| N-A4 preflight 路徑不符 | `cmd/experimental/sector-allocation-closure-preflight/main.go:167-186` | 中 | 檢查 `<work_dir>/data/state/sector_closure_policy.jsonl`，實際 writer/reader 在 `<work_dir>/data/sector/allocation/` | 抽共用路徑常數（與 §3.5 同手法） |


## 6. 驗收證據（可重跑）

```bash
# 本批新增／改寫的測試（全綠）
go test ./internal/industry/ -run 'Silicon|Calibration|PhaseHistory|Capex' -v
go test ./internal/config/  -run 'CycleCalibration' -v
go test ./internal/marketdata/ -run 'SectorData|ResolveSectorDataDir' -v
go test ./internal/apigateway/ -run 'SectorData' -v
go test ./internal/orchestrator/ -run 'Modulator' -v
go test ./cmd/atlas-mcp/... ./internal/config/ ./internal/industry/ ./internal/marketdata/ ./internal/apigateway/
```

| 驗收項 | 狀態 |
|---|---|
| 盤點表 + issue #1944 留言 | 本檔 §4/§5 + issue 摘要表 |
| 高嚴重項逐項處置與證據 | §3（I22/I3/I2/I10/I14/E1-E3/composite_card） |
| `gofmt -l` / `go vet` / `make ci-gate` / GitHub CI | 見 PR 描述 |
| 無新增參數 | 本批**未新增任何參數**；只把既有 config 接上 reader（I22 silicon_cycle、composite_card、cycle_calibration merge） |

## 7. 範圍與未做（誠實聲明）

- **未動** #1943 已定案的 canonical taxonomy / sectormap 映射（僅唯讀引用）。
- **未修** 需新資料源的項目：I25/N-U7（母體 quote provider）、I14 的 bridge 接線（需 sector_data 刷新者）、I13/I12（tracker/composition 共用實例）、I17/I23（校準落地）、I24（synthetic 過濾，屬 ledger 口徑票）。
- **未動** `docs/ATLAS_CONSTITUTION_AUDIT.md` 與 `auditSnapshot`（D1/E3 的 done 宣稱與 E1 的 MCP 旗標有關，但該檔有 `go generate` drift gate 且屬另一治理流程）；改動清單記於 §5 N-*，待另票處理。
- **未動** `docs/reference/traps.md`（已達 330 行 gate 上限）：本批結論全部寫在本檔與 `inert-registry.md`，traps.md 只在必要時留指標。
- 每項改動皆最小化：無重構、無刪測試、無放寬斷言；被改寫的兩個測試（`TestExtractSiliconIndicators_TSMC` 期望值、`TestCycleCalibration_ZeroWindowSizeConfigBlocksCalibration` 語意）原本**編碼了本批要修的 bug**，其自身註解即要求修好時同步更新。

## 8. 方法與分工

- 三問檢查逐項重跑於 worktree `~/workspace/atlas-inert-b2`（`origin/main@88683f06`）；生產 artifact 以**唯讀**讀取（`ssh kaecer@kmacmini cat ...`）交叉驗證，未執行任何寫入。
- 分四叢集平行盤點（config/參數、母體/命名空間、預測/LLM/narrative、對外 applied 欄位），再由本檔彙整；每項證據皆附可重跑的程式碼路徑或命令。

## 9. Batch 3（issue #1944，2026-09-25）— 高嚴重 inert 項逐項處置

| 項目 | 內容 |
|---|---|
| 範圍 | Batch 2 §4/§5 列為**高嚴重**且未處理者（I25、N-C1、N-U7、I12、I13、I31、I16、I23、I4、I5、I6、I17）+ 本批自行複核出的遺漏（N-U1/N-U3/N-U4、N-P3、N-C2、I15） |
| 基準 | `origin/main` @ `f7fcd74d`（含 #1976 secret-scan gate）；分支 `fix/20260925-inert-batch3` |
| 判定 | 沿用 §0 三問檢查；每項結論必為「**接線（附測試）／明示未啟用（機讀旗標＋理由＋釘樁測試）／移除**」之一，無一項靜默 |
| lane | root（N-C1 / I12 / I13 / I31）、quote lane（I25 / N-U7 / N-U1 / N-U3 / N-U4）、techniques lane（I16 / I17 / I15 / N-P3）、narrative lane（I23 / I4 / I5 / I6 / N-P4） |

### 9.1 逐項處置

| Item | 結論 | 主要檔案 | 證據（可重跑的測試／指令） | 對外狀態 |
|---|---|---|---|---|
| **N-C1** `max_daily_weight_change` 假風控 | **明示未啟用**（值 0.05 與行為皆不變） | `configs/parameters.json`、`configs/parameters/industry.json`、`internal/config/defaults_narrative.go` | `TestShippedConfigMaxDailyWeightChangeIsDeclaredUnenforced`、`TestMaxDailyWeightChangeHasNoConsumer`（掃 `internal/`、`cmd/` 非測試碼：reader 一出現就紅燈，強迫同 commit 更新宣告） | `GET /api/parameters/metadata` 的 `rationale`/`todo` 現在明寫 `NOT ENFORCED` + 語意未定的原因（原本宣稱「5% 日變動上限」） |
| **I12** composition 路徑 driver 硬寫 0 | **接線**（macro 取真值；factor 明示仍為 stub） | `internal/orchestrator/composition/root.go`、`internal/monitoring/dashboard_api.go`、`internal/industry/seasonality.go`（新增 `DynamicEnvModulator()` getter） | `TestSetCompositionRoot_SharesDashboardIndustryState`（macro tilt == dashboard modulator tilt == −0.05，非硬寫 0.0）、`TestBuildWeightEngine_UsesSharedSectorInputs` | 新增 `composition.SectorFactorDriverWired=false`：factor driver 仍是中立 0（無生產 provider），已明示不宣稱生效 |
| **I13** 兩個 `CycleTracker` 不同步 | **接線**（共用 dashboard 實例） | 同上 + `cmd/atlas/main.go`（`SetCompositionRoot` 首次真的被呼叫） | 同一測試：wiring 後更新 dashboard tracker，root engine 的 cycle multiplier 隨之變動；`go test -race` 乾淨 | `SetCompositionRoot` 不再注入 dashboard 的 legacy engine（無 Projector ⇒ `ComputeProjectedTarget` 會失敗），改為共享**輸入** |
| **I31** `calibration-validate` 失敗被吞 | **判定機讀化（本批）；CI 吞失敗半邊明示未生效**（需 workflow 一行，見 §9.2） | `internal/config/integrity.go`、`cmd/calibration-validate/main.go` | `TestValidateCalibration_FindingsAreClassified`（7 子案例）、`TestShippedConfigIntegrityFindingsAreClassified`（對出貨 config 做 exact-set 斷言）；實跑輸出見 §9.4 | `--format=json` 新增 `Findings[]{code,severity,segment,message}`（13 個穩定 code）；`Issues` 保留、exit code 仍為 1 |
| **I25** 母體 quote provider 寫死 nil | **接線（+ 生產尺度 chunked fetch）** | `cmd/atlas/bootstrap_helpers.go`、`internal/monitoring/universe_scheduler.go`（`QuoteFetchPolicy`／`fetchQuotesChunked`）、`cmd/atlas/cmd_universe.go` | `TestNewUniverseBuilderDeps_WiresRealQuoteProvider`、`TestBuildUniverseQuotesStatusOK`、`TestBuildUniverseNilQuoteProviderIsExplicit`、**`TestBuildUniverse_ProductionScaleChunkedFetchRanksSymbols`（1,599 檔 fixture → `input=1599 ranked=150 chunks=32`，見 §9.6）**、`TestFetchQuotesChunked_BoundsProviderCallSize`、`TestBuildUniverse_PartialQuoteFetchIsExplicit` | snapshot 新增 `quotes_status`/`quotes_returned`/`quotes_requested`/`quotes_chunks`/`quotes_chunks_failed`/`ranked_fallback_reason`/`ranked_trustworthy`；`quotes_status=partial` + `ranked_trustworthy=false`（部分 chunk 失敗時）⇒ 不再能被誤讀為「市場沒有合格股」 |
| **N-U7** Layer 2.5 流動性永不生效 | **接線 + 不靜默** | `internal/monitoring/risk_exclusion.go`、`cmd/atlas/bootstrap_helpers.go`、`cmd/atlas/cmd_universe.go` | `TestRiskExclusionLiquidityEvidence`（skip 留下 INFO `RuleDetail`；低於門檻 → `fail_reasons` 含 `liquidity`）、`TestNewUniverseBuilderDepsWithQuotes_SharesProviderWithRiskFilter` | 仍**非** fail-closed：缺資料只揭露、不擋股 |
| **N-U1** `-build-universe run` 恆失敗 | **接線**（改走真實 provider + 共用母體，mock 路徑移除） | `cmd/atlas/cmd_universe.go` | `TestBuildUniverseStatus_ReadsCanonicalSnapshot`、`TestBuildUniverseStatus_RejectsLegacySchema`；CLI 端無端到端測試（需真實網路）⇒ **弱證據**，已標明 | untrustworthy 時 CLI 回非零退出（明示） |
| **N-U3** 同一 snapshot 兩套 schema | **統一（單一權威）** | `internal/monitoring/universe_scheduler.go`（`UniverseSnapshotPath`/`SaveUniverseSnapshot`/`LoadUniverseSnapshot`） | `TestSnapshotSchemaIsSingleAndCanonical`、`TestBuildUniverseStatus_RejectsLegacySchema`（明確報 `incompatible schema`，不再靜默 0） | 舊 schema 檔在部署後第一次讀取會被判不可信（少一輪 D6，自我修復） |
| **N-U4** D6 watchlist consumer 不可達 | **接線（隨 I25 + N-U3 生效）** | `internal/monitoring/universe_scheduler.go`、`cmd/atlas/main.go`（coverage alert 亦改用 canonical reader） | `TestD6WatchlistChainReachable`（2317 累積至 `consecutive_failures=60`，main.go 的 `>=60` 述詞選得到） | coverage alert 不再因 schema 不符而靜默跳過 |
| **I16** 心法 `volatile` 恆 0 命中 | **明示「方向不可量測」+ 對外標籤** | `internal/strategy_techniques/evaluator.go`、`internal/monitoring/api/strategies/handlers.go` | `TestConditionEvaluator_VolatileNotCountedAsMiss`、`TestConditionEvaluator_UpDirectionStillScored`、`TestToSummary_VolatileFrameNotOverwrittenToZero`、`TestHandlers_ListStrategies_ExposesVolatileScope` | 新增 `hit_rate_scope`（`up_down`/`volatile_only`/`unmeasured`）、`hit_rate_source`（`snapshot_evaluator`/`feedback_store`/`seed`）、`volatile_tests`；seed 先驗不再被硬寫成 0 |
| **I17** 季節校準無 producer + 4 個超界 factor | **明示未知 + 消費端 clamp + 具名告警** | `internal/industry/seasonal_health.go`、`internal/industry/seasonality.go`、`cycle_status_card.go` | `TestSummarizeCalibrationHealth_NoObservationsIsUnknownNotCritical`、`TestGetPatternAdjustment_ClampsOutOfRangeFactors`、`TestClampAdjustmentFactor` | health 由 `critical` → `unknown`（`reason=no_observations`）；`adjustment_factor_status=out_of_range` + `out_of_range_patterns=[...]` 對外可見；4 個超界值消費時 clamp（−0.2634／−0.2883 → 1.0、2.5555／3.4414 → 2.5） |
| **I23** narrative `HitRate` 手寫常數對外稱「歷史回測」 | **接線（來源標記）** | `internal/narrative/hitrate_provenance.go`、`types.go`、`templates.go`、`knowledge_base*.go`、`narrative_detectors.go`、`ingestor.go`、`detector.go`、`internal/portfolio/factor_engine_aggregate.go` | `TestDefaultTemplatesCarryPriorHitRateSource`、`TestInvestmentModelsStartAsHandwrittenPrior`、`TestUpdateTemplateHitRatesRelabelsSource`、`TestAggregateHitRateSourceForEvents`、`TestHitRateProvenanceJSONContract` 等（22/22 PASS） | 5 個對外欄位新增 `hit_rate_source`（`handwritten_prior`/`replay_eval_in_memory`/`unavailable_no_samples`/`unavailable_no_template`/`not_populated`）；aggregate 的 `Formula` 標成 `narrative(theme=…, hit_rate=…, hit_rate_source=handwritten_prior)` |
| **I4** 預測 prior 恆 0 | **prior 接線；cycle 明示未啟用** | `cmd/atlas/main.go`、`internal/eventdriven/handler.go`、`sector_predictor.go` | `TestSectorPredictionStatusWiresStrategicPrior`、`TestStrategicPriorDrivesOverallBaselineDriver`、`TestSectorPredictionsAreNeverPersisted` | `StrategicPriorApplied()`/`CycleProviderWired()` 由實際輸入推導；cycle 卡上游 I13（本批已修 dashboard tracker 同步，但 predictor 端注入仍屬另一票）⇒ `eventdriven.SectorCycleProviderWired=false` |
| **I5** `SECTOR_PREDICTION_ENABLED` 預設關閉 | **明示未啟用（機讀）** | `internal/config/config.go`（**預設值未改**）、`cmd/atlas/main.go`、`internal/eventdriven/types.go`、`handler.go`、`cmd/experimental/c07-obs-collector/main.go` | `TestProductionSectorPredictionStatusFlagOff`（`reason=sector_prediction_disabled_by_flag`）、`TestSectorPredictionStatusJSONContract`、`TestSectorPredictionFlagDefaultsOff`；部署面掃描：`SECTOR_PREDICTION_ENABLED` 在 `*.yml/*.yaml/*.env/*.sh/Dockerfile*/*.toml` 出現 0 次 | `PredictionReport.SectorPredictionStatus`（enabled/applied/days/sector_rows/strategic_prior_applied/cycle_provider_wired/persisted/persistence_reason/reason）全部由實況推導；c07 collector 改讀此 status 的 reason（不再靜默不告警） |
| **I6** `SectorDayPrediction` 無落地 | **明示未落地（機讀）** | `internal/eventdriven/types.go`、`internal/ledger/event_flow_prediction_sector_gap_test.go` | 反射測試釘住 `ledger.EventFlowPredictionRecord`／`EventFlowPredictionStore` **無任何 sector 欄位或方法**（補上即紅燈）；`SectorPredictionPersisted=false` + `persistence_reason=ledger_event_flow_prediction_record_has_no_sector_column` | 未動 ledger storage schema（落地需 I32 另票） |
| **I15** `PrimaryMetrics` 只寫不讀 | **明示未啟用** | `internal/orchestrator/registry.go` | `TestAgentPrimaryMetrics_HasNoNonTestReader`（走訪全 repo 非測試 `.go`） | `orchestrator.AgentPrimaryMetricsWired=false` |
| **N-P3** `NewDriverAdapter` 零非測試呼叫者 | **明示 reserved**（不移除：它是 `PlanDriver`/`ReflectDriver` 唯一實作） | `internal/orchestrator/llm_driver_adapter.go` | `TestDriverAdapterReserved_HasNoNonTestCaller`（AST 掃描非測試 `CallExpr`） | `LLMSectorAgentDriverWired=false` |
| **N-P4** `semiconductor_cycle_peak` 無模板 | **明示（機讀）** | `internal/narrative/hitrate_provenance.go`、`narrative_detectors.go` | `TestThemesWithoutTemplateMatchesKB`、`TestSemiconductorCyclePeakEventCannotReachAnySector`（SOX 事件 `HitRate=0`、`hit_rate_source=unavailable_no_template`、`MatchChains=0`、`SectorBias=0`） | 事件本身標明無法到達任何產業（補模板／移除 detector 屬內容決策，未動） |
| **N-C2** `SetCompositionRoot` 零呼叫者 | **已解（由 I12/I13 接線一併處理）** | `cmd/atlas/main.go`、`internal/monitoring/dashboard_api.go` | 見 I12/I13 列 | — |

### 9.2 Batch 3 決策／新發現

| ID | 內容 | 嚴重度 | 處置 |
|---|---|---|---|
| **N-A5** | `ApplySectorRotation`（`internal/orchestrator/strategy_evolver.go:404`）是 `ComputeProjectedTarget` **唯一**生產呼叫者，但只帶 `CapitalFlowAction`；engine 的 `collect*Deltas` 只對「已存在的 key」套用 provider ⇒ **六個 driver adapter 在此路徑上永遠不會被呼叫**，投影等於 strategic prior | 高 | **明示未啟用**：`orchestrator.SectorDriverDeltasSupplied=false` + 釘樁測試 `TestApplySectorRotation_SuppliesNoDriverDeltas`（recording adapters 0 次呼叫 + `target == prior`）。要供給 delta 是行為變更，需同 commit 翻常數與更新本表 |
| **全市場 quote 抓取的 N+1 成本** | 1,599 檔母體下，fubon-proxy `/quotes` 逐檔呼叫上游（`services/fubon-proxy/main.py`），且 `HybridProvider` 只要一批中有一個不完整 quote 就整批 fallback 到 `FinMindProvider.GetQuotes`（逐檔一次 HTTP）⇒ 一檔停牌股可造成 ~1,599 次 FinMind 請求（≈14,400/日配額的 11%） | 高 | **已接線緩解**：`QuoteFetchPolicy`／`fetchQuotesChunked` 分批（預設 50 檔、100ms 間隔、60s/chunk），把 fallback 成本限制在單一 chunk；部分失敗明示 `quotes_status=partial`（§9.6）。**未修**：provider 內部的逐檔 fallback 行為本身（marketdata lane） |
| **I31 的 CI 半邊** | `nightly-refresh.yml` 以 `set +e` + 非最後一個指令的 `cat` 取值 ⇒ step 恆 success；Slack 步驟在 `SLACK_WEBHOOK_URL` 未設時 `exit 0` ⇒ 零告警。本批禁動 `.github/**`（lane 邊界） | 高 | **明示未生效**（本檔 §9.1 I31 + §9.3）。精確修法（待 CI lane 套用）：在 `cat validate-result.json` 之後加 `if [ "$(jq -r '.OK' validate-result.json 2>/dev/null)" != "true" ]; then echo "::warning::calibration validation failed"; exit 1; fi`。**前提**：需先裁定 freshness 政策——(a) 該檢查移到真正會被刷新的 production 主機執行，或 (b) 只驗結構、freshness 交給 production 監控（CI 檢出貨 checkout 的 `updated_at` 結構上不可能新鮮，見 I30） |
| **超界 `adjustment_factor` 的來源不明** | `cmd/calibrate-seasonal --update` 本身有超界守門（`validateCalibrationResult`），所以 production config 那 4 個超界值不是（或早於）該工具寫入 ⇒ 存在一條不受守門保護的參數寫入路徑 | 中高 | 本批只在**消費端** clamp + 具名 warn，未追污染源（超出 lane 範圍）；**建議另開票** |
| **config validator 允許負 `adjustment_factor`** | `internal/config/parameters_validate.go` 只檢查 `!= 0`，實證 production 有 2 個負值通過載入 | 中 | **刻意未改**：改成 `[0.3,2.5]` 會讓現行 production config 驗證失敗（等於擋啟動）。需與上一列一起處理 |
| **mock provider 會被標成可信** | `ATLAS_MARKET_DATA_PROVIDER=fugle` 無 key 時 `selectProvider` 回 `MockProvider`（回傳完整假 quote）；舊寫法會記 `ranked_trustworthy: true` | 中 | **已接線**：偵測 `IsMock()` → `quotes_status=mock`／`ranked_fallback_reason=quote_provider_mock`／`ranked_trustworthy=false`（`TestBuildUniverseMockProviderIsNotTrustworthy`）。只能偵測自曝的 mock；provider 內部 fallback 仍不可觀測 |
| **prior 在 cache miss 時被丟棄** | `eventdriven` handler 舊碼用 `NewSectorPredictor(&snap, nil)` 整個換掉 predictor ⇒ 即使有人呼叫 `SetStrategicPrior`，下一次重建就歸零（比 I4 原描述更嚴重） | 中高 | **已修**：prior/cycle 改存於 Handler 並在 rebuild 重新套用（`TestSectorPredictionStatusWiresStrategicPrior` 含 rebuild 後仍在的斷言） |
| **`ParameterSnapshot.NarrativeHitRates` 無來源標記** | `internal/domain/shared/shared.go` 的 theme hit rates 來自 config 常數集，對外仍無來源欄位 | 中 | **未處理**（超出本 lane）；僅登錄在此，Go 端已可從 `NarrativeEvent.HitRateSource` 追溯事件側 |
| **`factor_weight_engine.narrativeIntensity()`** | `internal/portfolio/factor_weight_engine.go` 以同一批 prior 平均當權重加成；不對外暴露 hit_rate，但仍是常數驅動 | 低 | **未處理**（已註記，可經 `NarrativeEvent.HitRateSource` 追溯） |
| **`internal/config/configs/parameters.json` 影子副本** | 與 `configs/parameters.json` 不同（md5 不同）且無任何 Go caller；`findRepoRoot("configs/parameters.json")` 會被它誤導到 `internal/config/`（本批測試已改用 go.mod 定位） | 低-中 | **未處理**（刪除 300KB 死檔需確認部署腳本未引用） |

### 9.3 Batch 3 仍未處理（誠實清單）

- **I31 的 CI 半邊**：如上，需 workflow 一行 + freshness 政策裁決（`.github/**` 不在本 lane）。→ **Batch 4 已處理（§10.2）**：CI 只驗結構、失敗真的失敗、freshness 移到 production 主機。
- **I24**（`BuildScorecards` 未過濾 `IsSynthetic`，生產 56%）、**I32**（ledger 事件流預測無 `sector` 欄位）、**I30**（nightly backfill 寫入被丟棄）**未動**：屬 ledger／workflow 口徑票。→ **I24 已由 Batch 4 處理（§10.1）**；**I32 仍未動**；**I30 仍未處理**（§10.2 只讓其後果可見）。
- **I17 的 producer 半邊**：per-pattern `calibration_observations/verdict/timestamp` 仍無寫入者；本批只讓 health 對「無觀測」誠實。補寫入需歷史回測寫回流程。
- **I4 的 cycle 半邊**：`SectorCycleProviderWired=false`；注入需先確定用哪個 tracker 作權威（I13 本批已讓 composition 與 dashboard 共用，predictor 端仍未接）。
- **I23 前端文案**：`shared_web/static/js/pages/narrative.js` 欄位標題仍寫「歷史命中率」——前端 lane 負責。→ **Batch 4 已處理（§10.4）**。
- **I16 的 (a) 選項**：volatile 若要真的量測，需產品先凍結「高波動」門檻定義（本批不猜門檻）。
- **仍未處理的 Batch 2 中/低項**：I7/I18/I19/I21/I27/I28/I29/I30/I36/N-C3/N-U2/N-U5/N-U6/N-A1..A4（見 §4/§5）。

### 9.4 驗收證據（可重跑）

```bash
# root lane
go test ./internal/config/ -run 'MaxDailyWeightChange|ValidateCalibration|ShippedConfigIntegrity' -v
go test ./internal/orchestrator/composition/ -run 'SharedSectorInputs|WithoutSharedInputs' -v
go test ./internal/monitoring/ -run SetCompositionRoot -v
go test ./internal/orchestrator/ -run SuppliesNoDriverDeltas -v
./atlas-validate --path=configs/parameters.json --max-age=48h   # OK=false、exit 1、[CODE] 分類
# quote lane
go test ./internal/monitoring/ -run 'BuildUniverse|SnapshotSchema|D6Watchlist|RiskExclusion' -v
go test ./cmd/atlas/ -run 'Universe' -v
# techniques lane
go test ./internal/strategy_techniques/ ./internal/monitoring/api/strategies/ -v
go test ./internal/industry/ -run 'Seasonal|Clamp|CalibrationHealth' -v
# narrative lane
go test ./internal/narrative/ ./internal/eventdriven/ -v
go test ./internal/portfolio/ -run Narrative -v

# 全量
gofmt -l . && go build ./... && go vet ./internal/... ./cmd/...
go test ./internal/... ./cmd/...
make ci-gate
```

| 驗收項 | 狀態 |
|---|---|
| 高嚴重項逐項處置與證據 | §9.1（20 項，含順手項） |
| 每項結論非接線即明示未啟用，無靜默 | 是；機讀旗標：`SectorFactorDriverWired`、`SectorDriverDeltasSupplied`、`SectorCycleProviderWired`、`SectorPredictionPersisted`、`AgentPrimaryMetricsWired`、`LLMSectorAgentDriverWired`、`ThemesWithoutTemplate`、`MaxDailyWeightChange`（metadata `todo`） |
| 對外 `applied`/`status` 欄位由消費證據驅動 | 是：`QuotesStatus`/`RankedTrustworthy`、`hit_rate_scope`/`hit_rate_source`、`hit_rate_source`（narrative）、`SectorPredictionStatus`、`seasonal health` 雙軸 |
| `gofmt -l` / `go vet` / `make ci-gate` / GitHub CI | 見 PR 描述（CI 全綠） |
| 未新增參數、未改 config 預設值 | 是（唯一 config 變更＝`max_daily_weight_change` 的 rationale/todo 文字，值 0.05 不變） |

### 9.5 方法與分工

- 三問檢查重跑於 worktree `~/workspace/atlas-inert-b3`（`origin/main@f7fcd74d`）；三個子 lane 各在獨立 worktree／branch 作業，由 root cherry-pick 併入（生成檔衝突以 `go generate ./...` 重生成解決）。
- 未執行任何寫入 production 的動作；未動 `.github/**`、`scripts/ci/**`、`internal/sectormap/**`、`internal/symbolindustry/**`。
- `docs/reference/traps.md` 維持 330 行（已達 gate 上限）：本批結論全部在本檔與 `inert-registry.md`，未新增 trap 列。


### 9.6 I25 生產尺度覆核（2026-09-25，業主提供生產實證後追加）

**生產實證（2026-09-25 06:00Z 每日母體重建，業主提供）**

```
06:00:30.523  symbols_gathered count=1599               ← per-stock 產業母體 gate 已生效（27 → 1599）
06:00:30.525  industry_filter_ok input=1599 output=1599
06:00:30.525  scoring_ok input=1599 ranked=0            ← 無 quote ⇒ 量價過濾把 1599 檔全數拒絕
```

`Quotes: nil` 使 `quoteMap` 為空，`ScoringScreener.applyVolumeAndPriceFilters` 對「沒有 quote 的 symbol」直接 `continue` ⇒ `symbols_ranked=0`。gate 讓母體由 27 成長到 1,599，但沒有 quote 就無法排名。

**本批追加的生產尺度防護（`QuoteFetchPolicy` / `fetchQuotesChunked`）**

單次「全市場」quote 呼叫在生產上不安全，兩個實證：

| 事實 | 來源 | 後果 |
|---|---|---|
| fubon-proxy `/quotes` 端點是**逐檔迴圈**（`for symbol in symbol_list: client.intraday.quote(symbol=symbol)`） | `services/fubon-proxy/main.py` | 一次「批次」HTTP 請求其實是 N 次上游呼叫；1,599 檔 = 1,599 次 |
| `HybridProvider` 只要批次內**任一** quote 不完整（`hasInvalidQuotes` → `QuoteComplete`）就丟棄整批並 fallback；下一個 provider 的 `FinMindProvider.GetQuotes` 是**逐檔一次 HTTP** | `internal/marketdata/hybrid_provider.go`、`internal/marketdata/finmind_client.go` | 一檔停牌股即可讓整批 fallback 成 ~1,599 次 FinMind 請求（≈ 每日 14,400 配額的 11%） |

處置（**不新增 config 參數**，常數即護欄）：

- `internal/monitoring/universe_scheduler.go` 新增 `QuoteFetchPolicy{ChunkSize, Pause, ChunkTimeout}`（零值 = 50 檔／100ms／60s）與 `fetchQuotesChunked()`：Step 3 改為**分批**呼叫 provider，並在 chunk 之間留間隔。
- 單一 chunk 逾時／失敗只讓該 chunk 退化（`quotes_chunk_error` warn），不拖垮整個母體；per-symbol fallback 成本由「整個母體」縮到「一個 chunk（≤50 檔）」。
- 部分失敗 ⇒ `quotes_status=partial` + `ranked_fallback_reason=quote_fetch_partial` + `ranked_trustworthy=false`（排名仍計算並落地供檢視，但**不得**當 D6 基準：失敗 chunk 的 symbol 缺席會被誤判為「不再合格」而累積 60 日失敗）。
- 全部 chunk 失敗 ⇒ `quotes_status=fetch_error` + 不信任（維持既有語意）。

**生產尺度證據（本批新增測試，真實輸出）**

```
# 1) pipeline 層（stub provider，CI 永遠可跑）
go test ./internal/monitoring/ -run TestBuildUniverse_ProductionScaleChunkedFetchRanksSymbols -v
  symbols_gathered count=1599 / industry_filter_ok input=1599 output=1599
  scoring_ok input=1599 ranked=150
  input=1599 ranked=150 chunks=32 chunks_failed=0 quotes_returned=1599 quotes_status=ok

# 2) 生產 wiring（cmd/atlas：newUniverseBuilderDepsWithQuotes + BuildUniverse）
#    母體清單 = cmd/atlas/testdata/universe_scale_symbols.txt（第一方 symbol_industry
#    通道實抓：total=1988 mapped=1599 unmapped=379，與生產 count=1599 一致）
go test ./cmd/atlas/ -run TestBuildUniverseProductionScale_RealSymbolList -v
  symbols_gathered count=1599 / industry_filter_ok input=1599 output=1599
  scoring_filters input=1599 no_quote=219 zero_volume=0 below_turnover_floor=58 below_price_floor=0 lots_converted=0 survivors=1322
  scoring_ok input=1599 ranked=150
  quotes: status=ok returned=1380 excluded=0 ranked_trustworthy=true
  fetch_widths=[50 ×31, 49, 150]   ← Step 3 分批；最後 150 是 Layer 2.5 的風險複核

# 3) 真實 provider 端到端（env-gated，預設 skip；本機唯讀打公開 TWSE 端點，未觸生產機）
ATLAS_TEST_UNIVERSE_LIVE=1 go test ./cmd/atlas -run ProductionScale_LiveQuoteProvider -v
  get_quotes_ok provider=hybrid-twse symbols=900
  scoring_ok input=1599 ranked=150   ← 真實 1,599 檔母體 + 真實第一方報價不再歸零
```

**成交量單位（第二個阻擋因素，本批已接線換算）**

| 事實 | 證據 |
|---|---|
| TWSE（第一方）的 `Volume` 是**股**（成交股數） | `internal/marketdata/twse_openapi.go` `convertToQuote`；live 實測 `max_volume=542,503,236`（股）且 `成交金額 ≈ volume × price` |
| Fugle / fubon-neo 的 `Volume` 是**張**（成交張數） | Fugle 官方 Candles 文件「整股：成交張數」；官方 Quote 範例 `tradeValue 31,019,803,000 ÷ tradeVolume 54,538 = avgPrice 568.77 × 1000`（若為股則與同 payload 的 310 億金額自相矛盾）；`services/fubon-proxy/main.py` 直接把 `total.tradeVolume` 當 volume 回傳 |
| 後果 | 量價過濾用 `Volume × Last` 對 NT$10M 門檻 ⇒ 以「張」計的報價把門檻實質變成 **NT$10bn**，中型股以下幾乎全滅，且 `quotes_status` 仍為 `ok`（靜默） |

處置：在**導出 TWD 量能的那一處**做 provider 來源別換算（`internal/monitoring/universe_builder.go` 的 `quoteVolumeLotSources` / `quoteVolumeInShares`：`fugle`／`fugle_candles`／`fubon` ×1000，其餘（`twse`／`finmind`／未知來源）視為股，行為與過去完全相同），換算筆數記入 `filterStats.LotsConverted` 與 `scoring_filters` 的 `lots_converted`，讓單位不符不再靜默。證據：`TestQuoteVolumeInShares_TranslatesLotsProviders`、`TestTurnoverFloor_IsUnitInvariant`（同一筆交易以「張」或「股」表述必須得到相同判定）、`TestTurnoverFloor_LotsQuoteBelowFloorStillDrops`（換算不會讓門檻失效）、`TestFilterStatsLotsConvertedIsRecorded`。

**追蹤（未修，需另票）**：`domain.Quote.Volume` 目前**一個欄位兩種單位**。正解是在 provider 邊界統一（Fugle×1000、Fubon proxy×1000），但那同時影響 ①`screener` 的 `VolumeIntraday` 條件語意、②`ledger` 已存的歷史 quote（會出現新舊混單位）、③dashboard/stocktools 顯示（台股慣例顯示「張」）⇒ 屬跨模組契約決策；本批只做上述單點換算並留下 tripwire 測試（`internal/marketdata/twse_stock_day_all_volume_test.go`）。

**其他覆蓋率事實（明示，未修）**：預設 `ATLAS_MARKET_DATA_PROVIDER=twse` 的 `STOCK_DAY_ALL` 只含上市，1,599 檔母體中僅 **900 檔**有報價（`no_quote=699`，44%）⇒ 上櫃（TPEx）半邊結構性無法進排名（缺 TPEx 日行情來源）；`hybrid` 路徑可涵蓋但成本較高。`ATLAS_MARKET_DATA_PROVIDER=fubon` 不是 `selectProvider` 的分支（只有 fugle/twse/hybrid/default），會靜默落到 hybrid，而 `configs/allowed_env_vars.md` 卻把它列為合法值。

**gate off 時的語意（業主第 3 點）**：quote provider **無條件**接線，因此 gate off（`substrate=nil`）時母體回到分類樹的 ~27 檔代表股，但同樣會抓 quote ⇒ `symbols_ranked` 由 0 變成 >0。這是刻意的（`symbols_ranked=0` 從來不是「市場沒有合格股」的合法表達，而是 I25 的 bug 症狀），已在 spec §9.1 與 PR body 明示；snapshot 的 `quotes_status`/`ranked_trustworthy` 讓兩種 gate 狀態都可稽核。

**排名是否還有其他阻擋因素（業主第 5 點，逐項複核）**

| 環節 | 是否阻擋 | 證據 |
|---|---|---|
| `applyVolumeAndPriceFilters`（`VolumeFloorTWD`／`PriceMin`） | **是（唯一真阻擋）** | 需要 `q.Volume × q.Last ≥ 10M TWD` 且 `q.Last ≥ 10`。provider 必須提供 Volume：TWSE `STOCK_DAY_ALL` 以「成交股數」填 `Volume`（`internal/marketdata/twse_openapi.go` `convertToQuote`）✓；Fubon proxy 用 SDK `tradeVolume`（單位需確認，見 §9.2 待辦） |
| `screener.Engine.ScreenUniverse` | 否 | `ScreeningCriteria` 全零 ⇒ `HasFilters()==false` ⇒ `ScreenDetailed` 直接 pass（`internal/screener/screener.go`） |
| `scoreAndRank`（分數為空即剔除） | 否 | `portfolio.FactorEngine.CalculateAllScores` 對任何 symbol 都回非空（momentum/value/quality/agent 四項無需外部資料）；但 adapter 傳 `bridgeInputs=nil` ⇒ `volume`/`foreign_flow` 分項恆 0（僅壓低分數，不剔除） |
| `ApplyConcentrationCap` / `TopN` | 否（只設上限） | 1,599 檔 ranking 後取 `MaxIndustryConcentration` 上限再取 `TopN`；實測 ranked=150（= TopN） |


---

## 10. Batch 4-A（issue #1944，2026-09-25）— 對外誠實與 false claim 修正（PR #1991）

| 項目 | 內容 |
|---|---|
| 範圍 | Batch 3 §9.3「仍未處理」中**直接影響對外可信度**的四項：**I23 前端文案**、**I24 scorecard 未過濾 synthetic**、**I31 CI 半邊**、**`ATLAS_MARKET_DATA_PROVIDER=fubon` 誤導** |
| 基準 | `origin/main` @ `1bc4b534`；分支 `fix/20260925-honesty-batch`（worktree `/tmp/atlas-honesty`） |
| lane 邊界 | 未動 `internal/marketdata/`、`internal/orchestrator/gateway_provider.go`（quote-reliability lane）、`docs/reference/inert-registry.md`（inert-batch4-registry lane）；`docs/reference/traps.md` 維持 330 行上限未新增列 |
| 判定 | 沿用 §0 三問檢查。I23 為「接線（來源標記→前端呈現）」；I24 為「接線（明確過濾 + 對外揭露）」；I31 為「讓失敗真的失敗 + freshness 政策裁決」；fubon 為「移除 + 明示」 |

### 10.1 I24 `BuildScorecards` 未過濾 synthetic → **明確過濾 + 對外揭露**

| 項目 | 內容 |
|---|---|
| 舊況（現行 main 實證） | `internal/ledger/ledger.go` 的 `BuildScorecards` 把**每一列** outcome 都聚合進 HitRate / SharpeLike / MaxDrawdown / IS-OOS；生產 `recommendation_outcomes` 45,668 列中 synthetic 25,571 列（**56.0%**）。synthetic 列的 `ForwardReturn` 是 `orchestrator.syntheticPlaceholderReturn` 的確定性佔位值（Batch 1 I20 語意），不是前向報酬 |
| 為何是 false claim | `syntheticPlaceholderReturn` 的 doc block 自己寫明「**MUST NOT** be aggregated into any 命中率 / hit-rate / win-rate or strategy-ranking number」，但唯一供給 scorecard 的函式沒有做這件事 ⇒ 對外 `hit_rate`/`sharpe` 是「量測 + 佔位分布」的混合 |
| 對照組（同 repo 既有正確做法） | `internal/portfolio/darwinian_period_matrix.go:112` 的 `BuildPeriodPerformanceMatrix` 第一行守衛就是 `o.IsSynthetic → continue`；`internal/strategy/shadow_evaluator.go:47` 同樣跳過。`BuildScorecards` 是漏掉的那一個 |
| 處置 | ① `BuildScorecards` 對 `outcome.IsSynthetic` 列改為 `continue`（只計數、不聚合），所有統計只由真實列計算；② `domain.Scorecard` 新增對外欄位 `synthetic_observations`（被排除列數）與 `synthetic_share`（`synthetic/(synthetic+real)`）作為**排除的稽核軌跡**；③ **全 synthetic 的 agent 不發 scorecard**（舊行為會發一張全 0 的卡 ⇒ 對 naive consumer 是新的 false claim「0% 準確率」；0 在此代表未知），改記 `WARN ledger.scorecard_agent_dropped_all_synthetic`（agents / total_agents / reason），保持「不靜默」 |
| 未改（明示） | `postgres_ledger.go` 的 slim query（`LoadScorecardOutcomes`）仍**不**在 SQL 端 `WHERE is_synthetic = false`：過濾放在 Go 的單一入口，避免同一語意在兩個查詢裡各寫一份；該欄位本來就有被 SELECT（`metadata->>'is_synthetic'`）所以 Go 端過濾在生產路徑上真的生效（slim 與 full 讀取都會帶到 `IsSynthetic`） |

**證據（可重跑）**

```bash
go test ./internal/ledger/ -run 'Synthetic' -v
```

| 測試名 | 釘住的行為 |
|---|---|
| `TestBuildScorecards_ExcludesSyntheticRows` | 2 真實 + 3 synthetic ⇒ `observations=2`、`synthetic_observations=3`、`synthetic_share=0.6`、`windows=2`、`hit_rate=0.5`（聚合全部會是 0.8）、`average_return=0.04` |
| `TestBuildScorecards_SyntheticRowsCannotChangeRealStatistics` | 對固定真實集合**追加** synthetic 列後，`HitRate/AverageReturn/SharpeLike/MaxDrawdown/TStat/HitRateTStat/IsSharpe/OosSharpe/IsOosRatio/RollingSharpeTrend` 必須**逐位元不變**（I24 的回歸守衛） |
| `TestBuildScorecards_AllSyntheticAgentIsOmitted` | 只有 synthetic 列的 agent 不得到 scorecard（未知 ≠ 0 準確率） |
| `TestBuildScorecards_RealOnlyAgentDisclosesZeroShare` | 全真實的 agent 對外揭露 `0/0.0`，不是缺欄位 |
| `TestScorecardSyntheticDisclosureJSONContract` | 對外 JSON key `synthetic_observations`/`synthetic_share` 存在，且既有 `observations` 未消失 |

對外影響：`internal/monitoring/service/agent_observatory.go`、`internal/monitoring/service/report.go`、`internal/backtest/window.go`、`internal/autobacktest/comparator.go`、`internal/autobacktest/signals.go`、`internal/orchestrator/phase3_controller.go`、`internal/orchestrator/system.go`（`NextExperimentCandidate`）、`internal/repository/postgres_audit.go` 全部同步改吃「真實列 only」的數字（它們都是同一個 `ledger.BuildScorecards`）。`internal/orchestrator/{prism,adversarial}_executor.go` 自行建構 outcome（`IsSynthetic` 預設 false），行為不變。

### 10.2 I31 的 CI 半邊 → **讓失敗真的失敗 + freshness 政策裁決**

| 項目 | 內容 |
|---|---|
| 舊況（現行 main 實證） | `.github/workflows/nightly-refresh.yml` 的 validate step 用 `set +e` + 非最終的 `cat`（`$?` 讀到的是 `cat`/`echo` 的退出碼）⇒ step 恆 success；Slack step 在 `SLACK_WEBHOOK_URL` 未設時 `exit 0` ⇒ **零告警**。同檔 backfill step 的 `\|\| echo` 讓 `steps.backfill.conclusion` 永遠不是 `failure`，Slack 條件因此永不成立 |
| 為何不能只加 `exit 1` | `cmd/calibration-validate` 對出貨的 `configs/parameters.json` 實跑 `OK=false`（exit 1），其中兩類是**結構上不可能消除**的：(a) `UPDATED_AT_STALE` — CI 驗的是 checkout 帶進來的檔案，`updated_at 2026-07-06`（I30：backfill 在 runner 內的寫入被丟棄，永遠不會被 commit）；(b) 7 個 by-design 的 `L1/L2_NO_REPRESENTATIVES`（`internal/config/integrity.go` 的 code 註解已記載）。直接讓它紅 ⇒ 每晚固定紅燈，跟永遠綠一樣沒有資訊 |
| **freshness 政策裁決（本批定案）** | **CI 只驗結構**；**freshness 由 production 主機執行**（檔案真的被刷新的地方）。CI 命令：`atlas-validate --path=configs/parameters.json --policy=configs/calibration-validation-policy.json --format=json`；production 命令：`atlas-validate --path=configs/parameters.json --max-age=48h --format=json`（scope=full，exit 1 必須接上生產監控） |
| 處置 | ① 新增 policy 檔 `configs/calibration-validation-policy.json`：宣告 `scope` 與 `accepted`（by-design findings，**每項必填 reason**，code 必須落在封閉集合、reason 不得是空/TODO，否則載入即失敗）；② `config.ValidateCalibrationWithOptions` + `CalibrationValidationPolicy`（fail-closed：無 policy 時行為與舊 `ValidateCalibration` 完全一致）；③ finding 新增 severity `observation`：scope=structure 時的 freshness findings、以及被 accepted 匹配的 findings 都是 observation，**仍列在 `Findings`**（可見、不靜默），其餘一律 error 並讓 `OK=false`；④ CLI 新增 `--policy`，輸出多出 `scope`/`freshness_enforced`/`error_count`/`observation_count` 與 `status`（`passed`/`passed_with_observations`/`failed`/`failed_structure`）；⑤ workflow：step 的 exit code **就是** `atlas-validate` 的 exit code（不再經管線讀 `$?`；錯誤級 findings 發 `::error::`、觀測級發 `::warning::` annotation ⇒ **零設定就有告警**，不依賴 Slack）；⑥ backfill step 移除 `\|\| echo`（`continue-on-error` 已足以保持 job 前進），改由後續 step 以 `steps.backfill.outcome != 'success'` 發 `::warning::`；⑦ Slack 仍為選配，未設定時發 `::warning::` 而非靜默 `exit 0`，且條件改看 `steps.*.outcome`（`conclusion` 在 `continue-on-error` 下永遠是 success） |

**證據（可重跑）**

```bash
# 出貨 config + 出貨 policy ⇒ 結構通過、8 筆全是 observation、exit 0
go build -o /tmp/atlas-validate ./cmd/calibration-validate
/tmp/atlas-validate --path=configs/parameters.json --policy=configs/calibration-validation-policy.json
# OK=true scope=structure freshness_enforced=false errors=0 observations=8 status=passed_with_observations

# fail-closed 預設（無 policy）⇒ 與 Batch 3 完全相同：8 errors、exit 1
/tmp/atlas-validate --path=configs/parameters.json
# OK=false scope=full freshness_enforced=true errors=8 observations=0 status=failed

# 新結構性缺陷仍會紅：清掉 semiconductor 的代表股
/tmp/atlas-validate --path=/tmp/mutated-params.json --policy=configs/calibration-validation-policy.json
# OK=false ... errors=1 observations=8 status=failed_structure  →  exit 1，且該 segment 標成 [error]

go test ./internal/config/ -run 'Policy|ValidateCalibration|Freshness|Finding' -v
```

| 測試名 | 釘住的行為 |
|---|---|
| `TestValidateCalibrationWithOptions_NoPolicyMatchesLegacy` | 無 policy ⇒ scope=full、0 observations、全部 severity=error、`Issues` 與 `Findings` 逐項 mirror、`OK` 與舊函式一致（防止「忘記傳 policy」變成放寬閘門） |
| `TestShippedConfigUnderShippedPolicyPassesStructureOnly` | 出貨 config + 出貨 policy ⇒ `OK=true`、`errors=0`、`observations=8`，且 freshness 仍以 observation 出現 |
| `TestShippedPolicyAcceptsOnlyByDesignFindings` | accepted 集合是**精確集合**（5 個 L1 + 2 個 L2），且 policy 不得硬接受任何 freshness code |
| `TestValidateCalibrationWithOptions_NewStructuralFindingFails` | 清掉 `semiconductor` 代表股 ⇒ `OK=false`、`errors=1`，該 finding severity=error（閘門有牙齒） |
| `TestValidateCalibrationWithOptions_FullScopePolicyEnforcesFreshness` | 同一個 policy 檔改成 `scope=full` ⇒ freshness 變 error、`OK=false`（production 側語意） |
| `TestCalibrationValidationPolicy_ValidateFailsClosed` | 9 個子案例：未知 scope / 未知 code / 空 code / 缺 reason / TODO reason / 空白 reason 全部拒絕 |
| `TestCalibrationFindingCodes_AreClosedAndSorted` | code 集合封閉為 13 個且排序（新增 code 必須同步更新 policy 契約） |
| `TestValidateCalibrationWithOptions_ZeroMaxAgeUsesDefault` | `MaxAge=0` 回退 48h，不會退化成「全部過期」 |
| `TestCalibrationValidationResult_JSONContract` | JSON key `OK`/`Issues`/`Findings`/`scope`/`freshness_enforced`/`error_count`/`observation_count` 與 finding 的 `severity` |

**仍未做（誠實）**：production 主機上的 freshness 檢查尚未接到既有監控（屬部署/監控 lane，不在本 PR）；I30（backfill 寫入被丟棄 ⇒ 本 job 無法刷新任何東西）未修，本 PR 只讓它的後果可見。

### 10.3 `ATLAS_MARKET_DATA_PROVIDER=fubon` 誤導 → **移除 + 明示**

| 項目 | 內容 |
|---|---|
| 舊況（現行 main 實證） | `selectProvider`（`internal/orchestrator/system_dispatcher.go`）只有 `fugle`/`twse`/`hybrid`/`""` 分支，`default` 直接回 `NewHybridProvider` 且**無任何訊號**；`configs/allowed_env_vars.md` 的 `ATLAS_MARKET_DATA_PROVIDER` 卻把 `fubon` 列為合法值 |
| 為何不做「實作該分支」 | fubon 通道只存在於 Python `services/fubon-proxy`（Go 側 `internal/fubonproxy` 只管理其生命週期，**沒有** `marketdata.Provider` 包裝它）。實作一個 provider 必須動 `internal/marketdata/`，那是另一條 lane（quote-reliability，#1986）的領域，本批禁動 |
| 處置（擇一並說明理由） | **從合法清單移除並明示**：① 新增機器可讀 SSOT `orchestrator.supportedMarketDataProviders`（`twse`/`fugle`/`hybrid`）與 `SupportedMarketDataProviders()`／`IsSupportedMarketDataProvider()`；② `default` 分支補 `WARN system.market_data_provider_unsupported`（帶 `configured`/`supported`/`fallback=hybrid`/`reason`），**只加這個 log，未重構 switch 其他部分**，hybrid 仍是安全 fallback；③ `configs/allowed_env_vars.md` 該列改寫為「有效值只有 `twse`/`fugle`/`hybrid`」，並寫明 fubon 沒有實作、會回退 hybrid 並 WARN |
| 未改（明示） | `ATLAS_MARKET_DATA_PROVIDER` 的值本身仍**不做啟動期驗證**（打錯字仍只是 WARN + hybrid，不是開機失敗）。理由：把它變成啟動錯誤會讓既有部署（可能含未知值）直接起不來，屬部署決策，需另票 |

**證據（可重跑）**

```bash
go test ./internal/orchestrator/ -run 'MarketDataProvider|SelectProvider|AllowedEnvVars' -v
```

| 測試名 | 釘住的行為 |
|---|---|
| `TestSupportedMarketDataProvidersMatchesSelectProvider` | SSOT 集合 == `{twse, fugle, hybrid}`、回傳複本（不可被呼叫端擴充）、`""` 視為支援、`fubon`/`yahoo`/`mock`/`FUGLE` 皆不支援 |
| `TestSelectProvider_FubonFallsBackToHybridNotMock` | `fubon` ⇒ hybrid provider（**不是** MockProvider，mock 會回傳完整假報價） |
| `TestAllowedEnvVarsDocDoesNotClaimFubon` | 直接讀 `configs/allowed_env_vars.md`：該列必須列出 `` `twse`/`fugle`/`hybrid` ``，且不得再把 `fubon` 寫成 backticked 值（文件漂回舊說法即紅燈） |

**與 §9.6 的關係**：§9.6 已記載「`ATLAS_MARKET_DATA_PROVIDER=fubon` 不是 `selectProvider` 的分支 … 會靜默落到 hybrid，而 `configs/allowed_env_vars.md` 卻把它列為合法值」。本節即該事實的處置。

### 10.4 I23 前端文案 → **接線（來源標記 → 前端呈現）+ 文案改正**

| 項目 | 內容 |
|---|---|
| 舊況（現行 main 實證） | `shared_web/static/js/pages/narrative.js` 的模型卡與模板表頭寫「歷史命中率」（`fmtSafePct(m.hit_rate)`，且固定用 `--color-success` 上綠色）；`capital-models.js` 同一個 `/api/narrative/models` 資料源也寫「歷史命中率」；`capital-causality.js` 的模板 badge 寫「命中率」；`client_web/static/index.html` 的因果頁說明寫 `hit_rate（歷史命中率）`。但 shipped 值 **100% 是手寫先驗常數**（`internal/narrative/templates.go` 的 24 個 `HistoricalHitRate` 字面值、`knowledge_base.go` 的 model `HitRate`），唯一的量測路徑（replay 評估）只在記憶體重算且不持久化 |
| 後端契約（#1979 / Batch 3 I23，本批只讀） | `hit_rate_source` 已貫穿 `InvestmentModel`（`/api/narrative/models` → `models[].hit_rate_source`）與 `CausalTemplate`（`/api/narrative/templates` → `templates[].hit_rate_source`）；權威定義 `internal/narrative/hitrate_provenance.go`（`handwritten_prior`/`replay_eval_in_memory`/`unavailable_no_samples`/`unavailable_no_template`/`not_populated`/`mixed`）。**本批未新增後端欄位** |
| 處置 | 新增單一權威前端 helper `shared_web/static/js/shared/narrative-hit-rate.js`（來源字串 → 對外文字／badge／tooltip／顏色；未知或欄位缺失回「來源不明」），三個頁面共用；主標籤改為 **`先驗命中率`**（刻意不含「歷史」「回測」「實測」「勝率」「已量測」），並在值旁呈現來源 badge；`unavailable_*`／`not_populated` 的 0 一律呈現「無法量測／未知」而**非 0%**（0 代表未知，不是零準確率）；先驗數值不再用綠／紅上色（避免暗示已量測績效） |
| 逐檔 before → after | `narrative.js` 模型卡：`歷史命中率 65.0%`（綠/紅）→ `先驗命中率 65.0%` ＋ badge `手寫先驗，未校準`（muted）；`narrative.js` 模板表頭／儲存格：`歷史命中率` / `55.0%`（固定綠）→ `先驗命中率` / `55.0% / 手寫先驗，未校準`；`capital-models.js`：`歷史命中率` → `先驗命中率` ＋來源 badge；`capital-causality.js`：badge `命中率 55.0%` → `先驗命中率 55.0%` ＋ badge `手寫先驗，未校準`；`client_web/static/index.html`：`hit_rate（歷史命中率）` → `hit_rate（先驗命中率 — 手寫先驗常數，尚未回測校準；來源另見 hit_rate_source）` |

**證據（可重跑）**

```bash
cd admin_web && npm run test        # 1..492 / # pass 492 / # fail 0
bash scripts/ci/build_all_frontends.sh
bash scripts/ci/check_field_contract.sh --strict
bash scripts/ci/check_frontend_imports.sh
npx playwright test tests/capital-pages.spec.js   # 2 passed（未改該測試）
```

| 測試名 | 釘住的行為 |
|---|---|
| `narrative-hit-rate-source.test.mjs`（21 個） | (a) 六個來源映射 ＋ 未知 fallback；(b) 先驗文案**不含**「歷史／回測／實測／勝率／已量測」且主標籤為 `先驗命中率`；(c) 0 值在 `unavailable_*`／`not_populated` **不得**出現 `0.0%`（結構式 `/>0\.0+%</` 斷言）；(d) 三頁必 import helper、不得再出現「歷史命中率」、不得未帶來源就格式化 hit_rate；(e) stub-DOM 直呼 `renderCapitalModels`／`renderCapitalCausality` 驗證實際輸出文字；(f) `client_web/static/index.html` 逐字守門 |

**弱證據（明示）**：`replay_eval_in_memory` / `not_populated` / `mixed` 三種來源在 shipped 資料不會出現（100% `handwritten_prior`），其文案只有單元測試覆蓋、**無真實 API 樣本驗證**；`capital-causality.js` 的驗證靠 stub DOM（非完整瀏覽器 SPA 路徑）。

## 11. Batch 4-B（issue #1944，2026-09-25）— inert 長尾清掃 + 登記表 v4（PR #1990）

| 項目 | 內容 |
|---|---|
| 範圍 | Batch 2 §4/§5 與 Batch 3 §9.3 的**剩餘長尾**（逐項「接線／明示未啟用／移除／仍 inert」） |
| 基準 | `origin/main` @ `1bc4b534`；分支 `fix/20260925-inert-batch4`（worktree `~/workspace/atlas-wt-inert4`） |
| 判定 | 沿用 §0 三問檢查；每項結論必為「接線（附測試）／明示未啟用（機讀旗標或 config 宣告＋理由）／移除／仍 inert（附理由與票）」之一 |
| lane | root（I17／I4／I19／I27／N-C3／N-A1／N-A3／N-U6／I30／I32／I36／I29 複核）、lane A（I18／I28／N-U2，commit `75942de7`）、lane B（N-U5／N-A2／N-A4／I29 唯讀，commit `5cd5f29b`） |
| 禁區（本批未動） | `internal/marketdata/**`（quote-reliability lane, #1986）、`internal/orchestrator/gateway_provider.go`、`internal/monitoring/universe_*`、`narrative.js`／`internal/stockpicker` 的 `BuildScorecards`、`.github/workflows/**`（honesty-batch lane） |

### 10.1 逐項處置

| Item | 結論 | 主要檔案 | 證據（可重跑的測試／指令） | 對外狀態 |
|---|---|---|---|---|
| **I17 producer 半邊** | **接線** | `cmd/calibrate-seasonal/main.go`、`internal/industry/seasonal_health.go` | `go test ./cmd/calibrate-seasonal/ -run UpdateParametersFileAt -v`（4 種 verdict 分支＋`SummarizeCalibrationHealth` 由 `none/unknown` → `measured`） | `cmd/calibrate-seasonal --update` 現在每個 pattern 寫 `calibration_observations`／`calibration_verdict`（`industry.SeasonalVerdict*` 共用詞彙）／`calibration_timestamp`；校準值仍受原本守門（0 觀測／低於門檻／超界）保護 |
| **I4 cycle 半邊** | **接線** | `internal/eventdriven/cycle_provider.go`、`internal/eventdriven/types.go`、`cmd/atlas/main.go` | `go test ./internal/eventdriven/ -run 'MeasuredCycleProvider|CycleProvider' -v` | `eventdriven.SectorCycleProviderWired` false → **true**；注入的是 industry service 的 `CycleTracker`（`auto_cycle_update` 每 6h 以 FinMind 營收/獲利 YoY 寫入，composition root 亦共用 ⇒ I13）。**只有 `EvidenceTier == empirical` 的產業會被回答**，seed-only 產業維持中性 0.0（不得把 config seed 當量測值） |
| **I7** | **仍 inert（lane 邊界）** | `internal/marketdata/symbol_industry_mapper.go:150,182` | `git grep -n 'NewSymbolIndustryMapper\|BuildMapping' internal/` → 只有定義（＋無測試）；`internal/marketdata/**` 由 quote-reliability lane（#1986）持有 | 未動；死碼仍在。**待處理**（另一 lane 移除） |
| **I18** | **移除（部分）** | `internal/orchestrator/forward_return_fallback.go`、`internal/stockpicker/winrate.go`、`internal/industry/seasonal_calibrator.go` | `git grep -nw 'GenerateForwardReturn\|generateFromDistribution\|StockWinRate\|StrategyWinRate\|ValidateAllPatterns'` → 非測試 0 命中（僅文件） | 移除死碼；**保留** `hashString`（`system.go` 仍用）、`ValidateCalibration`＋`ValidationResult`＋`makeYears`＋`evaluatePatternYears`（有測試與使用者）。其專屬測試一併移除，未放寬任何既有斷言 |
| **I19** | **接線（部分）＋明示殘留** | `internal/orchestrator/loader.go`、`plugin_registry.go`、`semiconductor_llm_agent.go` | `go test ./internal/orchestrator/ -run 'LLMSectorAgent|ResolveAgentExecutor|SemiconductorLLMAgent_Supports' -v` | 修正 first-match-wins 順序（LLM agent 先於 deterministic）＋`Supports` 要求兩個 driver 已注入 ⇒ flag-on 但未接線時不再靜默吃掉半導體 desk。**殘留**：production 沒有任何 driver 注入點（`LLMSectorAgentDriverWired=false`，N-P3），且 `llmSectorAgentsPlugin` 仍是 pass-through（N-P1）⇒ 旗標預設 false 不變、LLM 路徑仍未真正接線 |
| **I27** | **接線** | `internal/industry/symbol_sector_index.go`、`internal/monitoring/api/live/handlers.go`、`internal/monitoring/service/live.go`、`internal/monitoring/dashboard_dataloaders.go` | `go test ./internal/industry/ -run SymbolSectorIndex -v`（50 次重跑同值、最深 segment 勝出、`MultiAssigned` 可觀測、nil/空樹不 panic） | 三份重複實作收斂為單一決定性權威；`GetAllSegments()` 的 Go map 順序依賴消失。**殘留**：map 值仍是 segment ID（非 canonical L1），未命中仍回 `"other"`（非 canonical 標籤）⇒ 需另票與前端標籤契約一起處理 |
| **I28** | **移除（部分）** | `internal/sectorallocation/namespaces.go` | `git grep -nw 'NamespaceKind\|NamespaceEquityL1\|ThemeExposure\|ValidateThemeExposure'` → 0 命中 | 移除未被使用的一半；**保留** `L1FinalTarget`／`ValidateL1FinalTarget`（`legacy_gics_namespace_test.go` 與 `docs/specs/sector-namespace-canonical-spec.md` 的漂移守門指明它） |
| **I29** | **已修一半（#1983）＋明示殘留（本批未修）** | `internal/monitoring/universe_scheduler.go`（#1983）、`cmd/atlas/main.go:2045-2066` | 唯讀複核（lane B）：`CheckUniverseCoverage` 的分母＝第一方 `symbol_industry` upstream、分子＝resolved，`Resolved ≤ Upstream` 結構上不可能 >1；空母體走 `unavailable + alert` | 排程半邊的恆等式（`total = mapped`）已消失。**`cmd/atlas` 半邊仍假綠**：分母是分類樹代表股（實數 27 檔）、分子是 pipeline 母體（1,599 檔）⇒ `coveragePct ≈ 5922%`，`< 90` 永不成立；且 `snapshotSymbols == 0`（最該響的「快照缺失」）被 `snapshotSymbols > 0` 守衛排除。修法（另票）：改用 `monitoring.CheckUniverseCoverage(...)` 當唯一判準，並讓 `snapshotSymbols == 0` 走「不可量測」分支 |
| **I36** | **仍 inert（無生產存取）** | `darwinian_weights.json`（生產產物） | Batch 2 §4 證據：21 agents 中 15 個 `total_signals=0` | 本批未動（需生產資料與上游 signal 供給盤查）。**待處理** |
| **N-C3** | **接線 1 + 明示 2** | `internal/industry/event_calendar.go`、`configs/parameters.json`、`configs/parameters/industry.json`、`internal/config/defaults_narrative.go` | `go test ./internal/industry/ -run Sentiment -v`；`go test ./internal/config/ -run ShadowParameters -v` | `industry.event_sentiment_cap` **已接線**（`EventCalendar.sentimentCap()`；硬編 0.05 降為 no-config fallback，shipped 值相同 ⇒ 行為中性）；`freshness_scores`／`event_calendar_rules` 標 **NOT WIRED** 並寫明不可原樣接線的原因（前者 enum 語意與 `EvidenceTier` 不符；後者缺日期函式），`parameters_shadow_declarations_test.go` 在 consumer 出現時強迫更新宣告 |
| **N-U2** | **移除宣告** | `cmd/atlas/main.go`（`-build-universe` help） | help 字串改 `run\|map\|status`；`cmd/atlas/cmd_universe.go:32` 已有明確 unknown 模式錯誤 | 不再宣稱未實作的 `scrape` |
| **N-U5** | **接線** | `internal/monitoring/stage3_rules.go` | `go test ./internal/monitoring/ -run ModelConfidenceDegraded -v`（新增 padding／nil-count／0.5 三條） | 中性哨兵改為 ledger 的 `neutral = 0` 且要求 `RecentEventFlowPredictionsActualCount ≥ 5` ⇒「無資料 padding 0.5」不再製造假警報，真中性才告警 |
| **N-U6** | **接線（口徑揭露）** | `internal/eventdriven/types.go`、`handler.go` | `go test ./internal/eventdriven/ -run HistoricalHitRate -v` | `HistoricalHitRate` 新增 `neutral_samples`／`directional_samples`／`hit_rate_basis`（`t_plus_1_reconciled_direction_sign`）⇒ 12.1% 這種由 neutral 預測稀釋的比率可被正確解讀 |
| **N-A1** | **接線 7/11** | `internal/orchestrator/sector_allocation_closure_metrics.go`、`strategy_evolver.go`、`composition/root.go` | `go test ./internal/orchestrator/ -run 'SACLifecycle\|NilSACMetrics\|AppliedEmitsPolicyApplied' -v` | `SACMetrics` 由 `ApplySectorRotation`＋composition root 實際發射：`snapshot.start/current/target/fallback/end`、`policy.consumed/applied`。**仍未接線 4 個**（`snapshot.projection` 需 projector 回傳 clamped 統計、`legacy.read` 屬 legacy reader、`fallback.count` 無權威來源、`rollback.drill` 屬另一子系統）⇒ 留在 baseline 並寫明理由 |
| **N-A2** | **接線** | `internal/orchestrator/executor_pipeline.go`、`executor_control.go` | `go test ./internal/orchestrator/ -run 'MacroFlowTrace\|MacroAdjustmentAppliesTo' -v` | `macro_flow.applied` 移到 `ApplyControl` **之後**；未真的套用時改發 `macro_flow.skipped`＋`applied=false`＋原因（不再用 applied 字眼） |
| **N-A3** | **接線** | `internal/sectorallocation/closure_state.go`、`internal/orchestrator/strategy_evolver.go` | `go test ./internal/sectorallocation/ -run IsPromotable -v` | 新增 `applied_session_count`（只在有 `ConsumptionReceipt` 時遞增）；`IsPromotable()` 現在要求 ≥20 **applied** 場次（原本 20 筆「只存 snapshot、未消費」即可過關） |
| **N-A4** | **接線** | `internal/sectorallocation/policy.go`、`cmd/experimental/sector-allocation-closure-preflight/main.go`、`cmd/atlas/main.go`、`internal/monitoring/dashboard_api.go` | `go test ./cmd/experimental/sector-allocation-closure-preflight/ -v`（磁碟上唯一檔案路徑 == preflight 檢查路徑；AST 掃描所有 `NewFileClosureStore` 必須用共用 resolver） | 唯一權威 `sectorallocation.ResolveClosureStoreDir/Path`；preflight 不再探測不存在的 `data/state/...` |

### 10.2 ledger／nightly 口徑（B 組）與誠實聲明

| Item | 結論 | 證據 | 誠實聲明 |
|---|---|---|---|
| **I30**（nightly backfill 寫入被丟棄） | **明示未修（lane 邊界）** | `.github/workflows/nightly-refresh.yml` 仍無 commit/push step（Batch 2 §4 證據不變） | `.github/**` 由 honesty-batch lane 負責，本批不動；修法（移除或改開 PR）見 Batch 2 §4 |
| **I32**（ledger event-flow prediction 無 `sector` 欄位） | **明示未落地** | `internal/ledger/event_flow_prediction_store.go:24-40` 仍無 sector 欄位；I6 的反射釘樁測試仍在 | 落地需 I6＋ledger schema 變更（跨模組），本批不動 |
| **I21**（win-rate executor 恆 fail-closed） | **待觀察（生產驗證）** | 生產者半邊已由 #1949 修（排程＋條件註冊）；消費端 `stockpicker_winrate_executor.go:73` 的 source 仍固定 `stockpicker-foreign-3d-net-buy`、window `120d` | 本批**沒有**生產存取能力，**未**驗證 `stock_win_rate` 是否已有該 source/window 的列、`data/state/stock_flows/` 是否已生成。複驗方式：`stock_win_rate` 以 (symbol, source='stockpicker-foreign-3d-net-buy', window='120d') 查列數，並 `ls data/state/stock_flows/ \| wc -l`；若仍為 0，需先補校準產物（另票） |

### 10.3 驗收證據（可重跑）

```bash
# I17
go test ./cmd/calibrate-seasonal/ -run UpdateParametersFileAt -v
# I4 / N-U6
go test ./internal/eventdriven/ -run 'MeasuredCycleProvider|CycleProvider|HistoricalHitRate' -v
# I19
go test ./internal/orchestrator/ -run 'LLMSectorAgent|ResolveAgentExecutor|Supports' -v
# I27
go test ./internal/industry/ -run 'SymbolSectorIndex|Sentiment' -v
# N-C3
go test ./internal/config/ -run ShadowParameters -v
# N-U5
go test ./internal/monitoring/ -run ModelConfidenceDegraded -v
# N-A1 / N-A2 / N-A3
go test ./internal/orchestrator/ -run 'SACLifecycle|MacroFlowTrace|AppliedEmitsPolicyApplied' -v
go test ./internal/sectorallocation/ -run 'IsPromotable' -v
# N-A4
go test ./cmd/experimental/sector-allocation-closure-preflight/ -v
# 靜態閘門（#1944 建議 2）
bash scripts/ci/check_inert_closure.sh          # exit 0，baseline 187 → 179，stale 0
# 全量
gofmt -l . && go vet ./internal/... ./cmd/... && go test ./internal/... ./cmd/...
make ci-gate
```

| 驗收項 | 狀態 |
|---|---|
| 每項結論非接線即明示／移除，無靜默 | 是（§10.1／§10.2 共 18 項） |
| baseline 一致性 | 187 → **179** 筆（移除 1 個 config-inert＋7 個 writer-no-consumer），`stale` 0，新增違規 0 |
| registry v4 與 baseline 一致 | 見 `docs/reference/inert-registry.md` §Batch 4 |
| 未新增參數、未改 config 預設值 | 是（config 變更僅 3 處 rationale/todo 文字與 2 個新註解；`event_sentiment_cap` 值 0.05 不變） |
| 未動禁區 | 是（未動 `internal/marketdata/**`、`gateway_provider.go`、`universe_*`、`narrative.js`、`BuildScorecards`、`.github/**`） |

### 10.4 方法與分工

- 三問檢查重跑於 worktree `~/workspace/atlas-wt-inert4`（`origin/main@1bc4b534`）。
- root 負責：I17／I4／I19／I27／N-C3／N-A1／N-A3／N-U6 ＋ 登記表 v4 ＋ baseline。
- lane A（`inert4-deadcode`，commit `75942de7`）：I18／I28／N-U2 死碼移除；lane B（`inert4-alerts`，commit `5cd5f29b`）：N-U5／N-A2／N-A4 與 I29 唯讀複核。兩 lane 各自 worktree／branch，root cherry-pick 併入後重跑全部受影響套件。
- 本批**未**執行任何寫入 production 的動作；`docs/reference/traps.md` 維持原行數（未新增列，細節在本檔）。

## 12. Batch 4-A 的第二半（issue #1944，2026-09-26）— **I31 的 production 半邊：freshness 接上既有監控**

| 項目 | 內容 |
|---|---|
| 範圍 | §10.2「仍未做（誠實）」的第一項：**production 主機上的 freshness 檢查尚未接上既有監控** |
| 基準 | `origin/main` @ `df726b89`；分支 `fix/20260926-calibration-freshness-monitoring`（worktree `~/workspace/atlas-calib-freshness`） |
| 判定 | 沿用 §0 三問檢查。結論是**接線**（既有 metric 管線 + 既有規則樹），不是新造監控系統，也不是文件宣告 |
| 禁區（未動） | `internal/marketdata/**`（quote-reliability lane）、`.github/workflows/**`（CI 半邊已由 #1991 完成）、`docs/reference/traps.md`（維持 328 行，未新增列）、production |

### 12.1 掛載點的選擇（以及為什麼不用 channel health 契約）

| 候選 | 判定 | 理由 |
|---|---|---|
| **`cmd/atlas` 背景任務 + `internal/monitoring` 既有 metrics/告警規則** | **採用** | 與 `channel_health_metrics_export`、`atlas_universe_*` 完全同一條管線：`apigateway.BackgroundTaskManager` → `monitoring.MetricsCollector.RecordGauge` → `/metrics`（`PrometheusHandler`）→ `monitoring/prometheus.yml` 既有的 10s 抓取 → `monitoring/rules/*.yml`（權威樹）。Prometheus/Alertmanager 設定**一個字都不用改** |
| `channel health` 契約（`atlas_channel_staleness_overage_seconds` + `ChannelDataStale`） | **不採用** | 語意不合：`ChannelContracts()`（`internal/apigateway/channel_contract.go`）描述的是 **gateway 抓取的外部資料通道**，記錄由 adapter 寫入 `data/state/channel_health.json`；`configs/parameters.json` 是**應用自己寫的校準產物**，不是任何 adapter 抓回來的資料。硬塞一個合成 channel 記錄等於在通道健康報表上宣稱一個不存在的抓取通道（`DeriveChannelStatus`、後台通道清單、`cmd/check-channel-consistency` 都會把它當真）——正是本 session 反覆在修的「宣告與事實不符」 |
| 部署期 CLI + textfile collector | **不採用** | 要動 production（新增 cron／collector），且外部 cron 的失敗在 atlas 的監控裡看不到；背景任務順便把「檢查有沒有在跑」變成可觀測 |

判定面**重用** `config.ValidateCalibration`（`cmd/calibration-validate` 用的同一個函式）⇒ 不會出現第二套 staleness 語意。

### 12.2 一個順帶查實的事實：production image 裡沒有這條命令

政策上的 production 命令是 `atlas-validate --path=configs/parameters.json --max-age=48h`，
但**它沒有隨 image 出貨**：`cmd/calibration-validate` 是 CI 現場 build 的
（`.github/workflows/nightly-refresh.yml`：`go build -o atlas-validate ./cmd/calibration-validate`），
本 repo 的 Dockerfile 只把 `atlas-go`／`atlas-mcp`／`calibrate-seasonal`／`daily-replay-sync`
放進 `/app`。實查（2026-09-26，本機用同一份 Dockerfile 建出的 image）：

```bash
docker run --rm --entrypoint /bin/sh atlas-atlas:latest -c 'ls /app'
# atlas-go  atlas-mcp  calibrate-seasonal  daily-replay-sync  configs  data  logs  prompts  reports  scripts  sql
docker run --rm --entrypoint /bin/sh atlas-atlas:latest -c 'command -v python3 jq node'   # 三個都 MISSING
```

⇒ 在本次接線之前，生產上「`configs/parameters.json` 已不新鮮」是**零觀測**：
不是值班忘了跑，而是沒有東西會跑。（若它曾在生產被執行，那必然是 image 外的路徑；
本 PR 未能查證，明示為未驗證。）這也順帶修正了 §10.2 與 runbook 對「production 命令」的
隱含假設，並讓所有 triage 指令避免使用 image 沒有的 `python3`（改用實查存在的
`grep`／`stat`／`head`／`tail`／`curl`）。

### 12.3 指標族（`internal/monitoring/calibration_freshness.go`，label 只有 `artifact="parameters"`）

| 指標 | 語意 |
|---|---|
| `atlas_calibration_freshness_ok` | 1 = 新鮮（沒有任何 freshness finding）；**0 = 其他一切情況（含「無法評估」）** ⇒ fail-closed |
| `atlas_calibration_freshness_run_ok` | 1 = 檢查能評估產物；0 = 不能（stat/讀取/JSON 失敗）。區分「真的舊」與「不知道」 |
| `atlas_calibration_freshness_age_seconds` | **落後多少**（秒，`updated_at` 距今）；只在有記錄時間時輸出 |
| `atlas_calibration_last_calibrated_timestamp_seconds` | **最後一次成功校準時間**；`0` = 從未記錄（明確哨兵值） |
| `atlas_calibration_freshness_checked_timestamp_seconds` | 檢查最後一次執行時間（探針心跳） |

設計約束（每一條對應一個已發生過的缺陷）：

1. **不得等到第一次 increment 才存在**（#1995）：`_ok` / `_run_ok` / `_checked_timestamp_seconds`
   **每一輪無條件輸出**；任務第一次執行受 `startupStaggerDelay` 限制（interval=5m ⇒ 上限 30s），
   不是 1 天 ⇒ 沒有「重啟後缺席 71 小時」那種視窗。
2. **缺席要有規則負責**：另有一條 `absent()` 規則（以 `up{job="atlas-go"}` 為閘門）覆蓋
   「任務停止執行」與「指標被改名/移除」。
3. **fail-closed**：檢查本身失敗 ⇒ `run_ok=0` **且** `_ok=0`（未知不得當成新鮮），由專屬規則
   以 severity=error 發出。
4. **狀態必須持續覆寫**：`MetricsCollector` 的 gauge 是 last-write-wins 且不清序列
   ⇒ 條件消失就跳過輸出會讓舊樣本永久凍結（`channel_health_metrics_task.go` 的檔內註解記載過
   這個坑）。`_ok`／`_run_ok`／心跳每輪覆寫；`age`／`last_calibrated` 在無法判定時不寫入
   （不輸出假造量測值），**因此會凍結在最後一次可評估的值** —— 這是明示的行為，
   Go 測試 `TestObserveCalibrationFreshness_UnverifiableFreezesLastKnownSeries` 釘住它，
   runbook §2.1 規定判讀順序（先看 `_run_ok`）。
5. **缺席 ≠ 0**：本族**沒有任何**規則用 `or vector(0)`；無法評估時 `age` 不更新，該狀態由
   `run_ok` 規則單獨負責，不會被讀成「很舊」。
6. **`_ok` 與 CLI 同源**：`_ok` = 「沒有任何 freshness finding」＝CLI 的 freshness 判定
   （`updated_at` 與 mtime 都看）；`age` 只反映 `updated_at`。`cp -p` 還原舊檔時以 `_ok` 為準
   （`TestObserveCalibrationFreshness_MtimeStaleMatchesCLI` 釘住「gauge 必須與 CLI 同判」）。

### 12.4 契約（48h）與告警（`monitoring/rules/calibration_freshness_alerts.yml`）

**契約只有一個數字**：`monitoring.CalibrationFreshnessContract` **就是**
`config.DefaultCalibrationMaxAge`（本次把 `internal/config/integrity.go` 的常數改為 exported，
並讓 `cmd/calibration-validate` 的 `--max-age` flag 預設值也引用它）⇒ CLI 與監控不可能漂移。
規則檔刻意不重寫門檻；`TestCalibrationFreshnessContractMatchesCLIAndRunbook` 讀 CLI 原始碼與
runbook 把這件事釘住（也禁止再出現第二份 `48 * time.Hour` 字面值）。

實測基線（不是照抄別的規則）：

| 基線 | 值 | 來源 |
|---|---|---|
| 生產「健康」 | 容器啟動後約 **65 分鐘**就被改寫 | `Created=02:04:02Z` vs `updated_at=2026-09-26T03:08:44.9Z`（FU-20260926-07） |
| 生產「壞掉」 | 約 **82.8 天**（本 PR 對出貨檔實跑；`age_seconds` 隨時間增加，非固定值） | FU-20260926-07 + 本 PR 的真輸入測試 |
| 校準任務 cadence | 主要 24h（17 個 top-level；6h/1h 例外） | `cmd/atlas/calibration_tasks.go` |

48h 落在兩者之間（對健康值 ~44×、對壞值 ~41×），且 = 兩個 24h 週期（吸收一次失敗週期與週末）。

| 告警 | severity | 條件 | for |
|---|---|---|---|
| `CalibrationArtifactStale` | warning | `_ok == 0` **且** `_run_ok == 1` | 1h |
| `CalibrationFreshnessUnverifiable` | error | `_run_ok == 0` | 15m |
| `CalibrationFreshnessExporterDown` | warning | `absent(_checked_timestamp_seconds)` 或 `time() - _checked_timestamp_seconds > 1800`，`and on() up{job="atlas-go"} == 1` | 15m |

三條**互斥分工**（`_ok==0 且 run_ok==1` / `run_ok==0` / 探針缺席）：同一個根因不會有兩條規則
各自 paging；服務掛掉時三條都沉默（由 `AtlasGoTargetDown` 負責）。已知的 **≤15 分鐘交接窗**
（第 1 條 resolve → 第 2 條 firing）是刻意的取捨，已寫成 promtool 案例 K。

### 12.5 驗收證據（可重跑）

```bash
# Go 側（真輸入 + 正反案例 + fail-closed + 凍結樣本 + 契約漂移 + 規則↔指標名）
go test ./internal/monitoring/ -run CalibrationFreshness -v
go test ./cmd/atlas/ -run 'CalibrationFreshness|CalibrationParametersPath' -v

# 規則面：語法 + 「該 firing 的真的會 firing」
docker run --rm -v "$PWD:/work" -w /work --entrypoint /bin/promtool \
  prom/prometheus:v3.14.0 check rules monitoring/rules/calibration_freshness_alerts.yml
docker run --rm -v "$PWD:/work" -w /work --entrypoint /bin/promtool \
  prom/prometheus:v3.14.0 test rules monitoring/tests/calibration_freshness_test.yml
```

**真輸入的正反案例（本 PR 實跑，非合成 fixture）**
（`age_seconds` 是量測當下的值，隨時間增加 ⇒ 重跑會得到更大的數字；其餘欄位是穩定的）

```
# 反例：出貨的 configs/parameters.json（updated_at 2026-07-05T17:43:01Z）
shipped artifact: fresh=false freshness_code="UPDATED_AT_STALE" age_seconds=7150619 last_calibrated=2026-07-05T17:43:01Z findings=8
atlas_calibration_freshness_ok{artifact="parameters"} 0.000000
atlas_calibration_freshness_run_ok{artifact="parameters"} 1.000000
atlas_calibration_freshness_age_seconds{artifact="parameters"} 7150618.850603
atlas_calibration_last_calibrated_timestamp_seconds{artifact="parameters"} 1783273381.000000

# 正例：同一份內容、updated_at 改成 65 分鐘前（生產實測的寫入節奏）
fresh=true age_seconds=3900 last_calibrated=2026-09-26T10:55:00Z
atlas_calibration_freshness_ok{artifact="parameters"} 1.000000
atlas_calibration_freshness_run_ok{artifact="parameters"} 1.000000
atlas_calibration_freshness_age_seconds{artifact="parameters"} 3900.000000
```

**promtool 單元測試：11 個案例（6 個負向對照 + 1 個已知交接窗）** — `SUCCESS`

| 案例 | 世界 | 斷言 |
|---|---|---|
| A | 新鮮、可評估 | 三條全沉默 |
| B | 過期 72h（可評估） | 只有第 1 條 firing |
| C | `last_calibrated=0`、沒有 age | 第 1 條 firing（0 是哨兵，不是「沒資料」） |
| D | `run_ok=0`（讀不到） | 第 2 條 firing，**第 1 條必須沉默** |
| E | 心跳凍結（序列還在） | 第 3 條 firing（`time()-ts` 那個 arm） |
| F | 整族缺席 + 服務在跑 | 第 3 條 firing（`absent()` 那個 arm） |
| G | 整族缺席 + 服務沒在跑 | 三條全沉默（不與 `AtlasGoTargetDown` 重複 paging） |
| H | 心跳落後 25 分鐘（未達 1800s） | 第 3 條沉默（門檻負向對照） |
| I | 過期只持續 30 分鐘 | 第 1 條沉默（`for: 1h` 負向對照） |
| J | 無法評估只持續 10 分鐘 | 第 2 條沉默（`for: 15m` 負向對照） |
| K | `run_ok` 由 1 翻 0（第 1 條 firing 中） | 第 1 條立刻 resolve、第 2 條要 15m 才 firing ⇒ **交接窗**（刻意的取捨，寫成規格） |

**變異測試（證明測試網有牙齒；手跑，原始輸出見 PR 說明）**

| 變異 | 要求紅燈的案例 | 結果 |
|---|---|---|
| m1 第 1 條拿掉 `and run_ok == 1` | D、K | `FAILED` |
| m2 第 3 條拿掉 `up{job="atlas-go"} == 1` 閘門 | G | `FAILED` |
| m3 第 2 條 `== 0` 改成 `== 1` | A、B | `FAILED` |
| m4 第 1 條 `for: 1h` → `0m` | I | `FAILED` |
| m5 第 3 條 `1800` → `3600` | E | `FAILED` |
| m6 第 3 條拿掉 `absent()` arm | F | `FAILED` |
| m7 第 2 條 `for: 15m` → `30m` | K | `FAILED` |
| m8 規則側加回 `or vector(0)` | A、B | `FAILED` |

### 12.6 完成／未完成（誠實）

**完成**
- production 端的新鮮度檢查已接上既有監控：指標（5 個 gauge）+ 3 條規則 + 探針心跳，全部走既有管線與既有規則樹；契約收斂為單一常數（`config.DefaultCalibrationMaxAge`）；文件（本節＋runbook）與 FOLLOWUPS（FU-20260926-10）同步；`scripts/ci/check_critical_tasks.sh` 把匯出任務列為關鍵任務（被 DCE 移除 ⇒ 監控整體消失而 CI 全綠，這正是要防的形狀）。

**未完成（明示，不靜默）**
- **生產驗收未執行**：本 PR 只交付接線與本機/promtool 證據；`curl /metrics`、Prometheus 規則載入、告警實際狀態要在部署後才驗（runbook §4 有逐步命令）。
- **「校準任務有沒有在跑」沒有心跳指標**：校準寫入是**有變更才寫**（`internal/risk/self_calibrate.go`），所以產物年齡只是活動的**上界** ⇒ `CalibrationArtifactStale` 對「已收斂、連續 verdict=stable」的系統可能誤報。要接到 18 個校準任務才能給出直接訊號（FU-20260926-10）。
- **結構性 finding 仍未進生產監控**：`L1/L2_NO_REPRESENTATIVES` 之類由 CI 的 policy 負責；生產端的結構漂移目前沒有自動訊號。
- **`atlas-validate` 沒有隨 image 出貨**（§12.2 實查）：本 PR 讓「生產端沒有觀測」變成「有觀測」，但沒有處理「政策命令在生產不可直接執行」這件事；若業主希望 operator 能在主機上手動覆核，需另票（把 CLI 加進 image 或提供 wrapper）。
