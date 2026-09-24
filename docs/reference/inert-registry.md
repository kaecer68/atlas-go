# Inert（宣告生效但實際未生效）登記表

| 項目 | 內容 |
|---|---|
| 文件角色 | 「producer 有、consumer 無」「狀態宣稱生效但實際 inert」「死碼」的**單一登記處**，避免同一類缺陷（靜默失效）反覆被發現又重新遺忘 |
| 狀態 | v1（2026-09-24，issue [#1944](https://github.com/kaecer68/atlas-go/issues/1944) Batch 1） |
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

## Batch 2（未處置，等 #1943 併入後另開票）

> 依 #1944 指示，Batch 2 與 [#1943](https://github.com/kaecer68/atlas-go/issues/1943)（產業命名空間統一）動到同一批檔案／key 空間，故不在 Batch 1。權威清單仍以 issue #1944 與 Q6 報告為準。

1. **產業 key 空間相依（必須與 #1943 同批）**：I26 `SymbolL1Mapper` 只解出 18 支/5 產業、I27 三份 `buildSymbolSectorMap`（map range 非決定性）、I28 `sectorallocation/namespaces.go` 只有型別無 runtime consumer、I7 FinMind `BuildMapping` 無呼叫者、I12/I13 composition 路徑 macro/factor driver 硬寫 0 與 CycleTracker 只吃 config seed。
2. **`sector_data` 橋接路徑**：I14 `BridgeSectorDataToCycleTracker` 僅測試呼叫 + provider 讀不存在的 `data/state/sector_data/`（缺檔回零且 `err=nil`）。
3. **sectorallocation 其餘 inert**：I9 prior 晉升閘門（`StartObservation`／`IsPromotable` 僅測試）、I10 `IndustryCycleModulator`／`NarrativeConvictionModulator` 未註冊、I11/I33 `SetCycleCard` 無生產呼叫、I8 的**接線**（把 `SectorBudgetAllocator` 接成真正的 consumer，並呼叫 `RegisterPolicyConsumer`）、I4/I5/I6/I19 產業預測與 LLM sector agents 預設關閉。
4. **Batch 1 相鄰、但屬行為變更或需另票**：
   - I3：`industry.cycle_calibration` config 全 0（`min_samples=0`、clamp 0..0、`window_size=0`）⇒ 視窗每次被清空、clamp 視窗退化；Batch 1 只加了「無證據不動權重／全零權重回預設」的護欄，補預設值需另票。
   - I18：`internal/orchestrator/forward_return_fallback.go` 的 `GenerateForwardReturn` 無生產呼叫者，且其「有 quote 就用 intraday × 0.9」與 I20 同一語意問題 ⇒ 應與本表 I20 的 placeholder 合併成單一實作（含測試改寫）。
   - I20（剩餘部分）：產業桌 conviction 仍由 `Last > Open` 推導（與命中判定同源）；改動會影響決策，需另票。
   - I24：`ledger.BuildScorecards` 不過濾 `IsSynthetic`，與 `darwinian_period_matrix` 的嚴格過濾不一致 ⇒ synthetic placeholder 仍可能污染 scorecard。另：`syntheticPlaceholderReturn` 不看 `rec.Side`，因此 SELL（看空且正確）也會被 `Hit = forwardReturn > 0` 記成 miss——這與「synthetic 列不得進命中率聚合」一併處理。
   - SA11.A 觀測窗（2026-09-24 獨立複驗新增）：`ApplySectorRotation` 持久化成功就會 `RecordSession`，即使 `applied=false`（無消費證據）也照計 ⇒ promotion gate（`IsPromotable` = `SessionCount>=20` …）可能在 **0 個真正 applied 場次**下成立，與 spec §8.3 的精神衝突。目前無生產呼叫端（`internal/sectorallocation/closure_state.go:167-200`），故未在本批改動計數語意（屬 gate 行為變更，需另票）。
   - preflight 路徑不符（2026-09-24 獨立複驗新增）：`cmd/experimental/sector-allocation-closure-preflight/main.go:167-186`（與 `:63` 的操作訊息）檢查 `<work_dir>/data/state/sector_closure_policy.jsonl`，但實際 writer/reader 是 `<work_dir>/data/sector/allocation/`（`cmd/atlas/main.go:319`、`internal/monitoring/dashboard_api.go:564`）⇒ 依該工具/runbook 排查的 operator 會看一個系統永遠不寫的檔案。屬 pre-existing，本批未改（需抽共用路徑常數），列入 Batch 2。
5. **其餘（Q6 之 I2、I15–I17、I21–I23、I25、I29–I32、I36）**：未在 Batch 1 觸碰，逐項狀態見 Q6 報告。
