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
| I8 | 產業配置閉環 write-only | `SectorAllocationSnapshot.applied` 由 `FileClosureStore.Store()` 硬寫 true；`policy.Consume()` 與 `portfolio.SectorBudgetAllocator` 無生產呼叫者 ⇒ 對外宣稱已套用 | **狀態改由消費證據驅動**（明示未啟用）。`applied=true` 只在有 `ConsumptionReceipt` 時成立；否則 `applied=false` + `fallback_reason=allocator_unavailable`。`Store()` 不再寫入 `applied=true`（連呼叫端硬塞 true 也會被正規化為 false） | `internal/sectorallocation/policy.go`（`ApplicationStatusFor` / `DecorateApplicationStatus` / `RegisterPolicyConsumer`）、`internal/orchestrator/strategy_evolver.go`（`ApplySectorRotation` 回 `applied=false`）、`internal/monitoring/api/industry/handlers.go`；測試 `policy_application_status_test.go`、`strategy_evolver_applied_evidence_test.go`、`handlers_sector_allocation_applied_test.go` |
| I34 | `internal/sim` 的 `rotationFunc` 從未被賦值 | 只有宣告與 nil 守衛，無 setter、無指派 ⇒ 輪動邏輯不可能執行 | **移除**（非接線）。輪動已在推薦層實作（`orchestrator.PortfolioRotator` + `PositionEvaluator`），在 sim engine 內再接一條會產生第二套輪動路徑 | `internal/sim/engine.go`（移除 `RotationFunc` 型別／欄位／呼叫點，改註解指向真正的實作位置）、`internal/sim/testdata/sim_api.golden.json`（API 快照同步） |
| I35 | L1–L5 心法 plugin 是 no-op pass-through | plugin 在生產註冊（`cmd/atlas/main.go` → `WithStrategyTechniques`）且會收 narrative 事件，但 `ProcessRecommendations` 原樣回傳 ⇒ 心法層不影響任何決策 | **明示未啟用**：新增可機讀旗標 `orchestrator.TechniquesLayerActive=false`、attach 時記錄 `pass_through=true`，並在程式碼／本表寫明「看到 strategy_techniques 不等於心法生效」 | `internal/orchestrator/strategy_techniques_plugin.go`；測試 `strategy_techniques_plugin_test.go`（含「回傳同一個 slice」不變式） |
| I20 | 合成報酬取當日 intraday | `syntheticForwardReturn` 用 `(Last-Open)/Open × 0.8` 當「forward return」⇒ 非前瞻、與訊號同源（自我實現），且平盤日 `forwardReturn=0` ⇒ `Hit=false`（必然 miss） | **修正語意**：改為 regime 條件化的確定性 placeholder（`forward_return.risk_on_*`／`risk_off_*`，agent×symbol×交易日為種子），命名改為 `syntheticPlaceholderReturn`，並在註解寫明「不是 forward return，不得進任何命中率聚合」 | `internal/orchestrator/system.go`；測試 `synthetic_placeholder_return_test.go`（同日不同價格走勢必須得到同值、值域、非必然 miss） |
| I1 | `CycleCalibration` 校準權重從未被消費 | `resolveCardConfig()` 無條件回 default，但 `IndustryService.SetCycleCalibration` 的註解宣稱「wires it into the global card builder state」 | **接線**（含兩道護欄）：有 layer metrics 才重新分配權重，且**保留原 funded 權重和（0.85）**，避免空窗期把 composite coefficient 整體放大 1/0.85 | `internal/industry/cycle_status_card.go`（`resolveCardConfig` / `applyCycleCalibration`，global 改為 mutex 保護）；測試 `cycle_status_card_calibration_test.go` |

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
   - I24：`ledger.BuildScorecards` 不過濾 `IsSynthetic`，與 `darwinian_period_matrix` 的嚴格過濾不一致 ⇒ synthetic placeholder 仍可能污染 scorecard。
5. **其餘（Q6 之 I2、I15–I17、I21–I23、I25、I29–I32、I36）**：未在 Batch 1 觸碰，逐項狀態見 Q6 報告。
