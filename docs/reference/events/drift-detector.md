# DriftDetected 事件

> **事件常數**：`EventDriftDetected = "portfolio.drift.detected"`
> **對應 EventType 字串值**：`portfolio.drift.detected`
> **Severity**：info（PD-W9-1）
> **Schema Version**：2（v2 新增 target weights drift 偵測；v1 payload 欄位完整保留，向後相容）

## 觸發條件

DriftDetector 訂閱 `EventPositionUpdate` 與 `EventRegimeChangeConfirmed`（v2 新增），內部維護 `symbol → snapshot` map 與週期性 turnover 計算。

每 5 分鐘（`DriftCheckInterval`）呼叫 `checkPeriod`：
1. 計算 `total_value` = Σ symbol.MarketValue
2. 計算 `max_concentration` = max(symbol.MarketValue / total_value)
3. 計算 `turnover` = |total_value - prevTotal| / prevTotal
4. **v2 新增**：若 `TargetWeightsProvider` 非 nil 且 `GetTargetWeights(currentRegime)` 回傳非空 map，則對每個 symbol 計算 `drift = |actual_weight - target_weight|`，並取最大值 `max_drift`
5. 觸發條件（任一）：
   - `max_concentration > 0.25`（`DriftMaxConcentrationThreshold`）
   - `turnover > 0.15`（`DriftTurnoverThreshold`）
   - `max_drift > 0.10`（`DriftTargetWeightThreshold`，v2 新增）
6. 觸發時 emit `EventDriftDetected`，payload 包含 `reasons` 陣列標示哪個條件觸發

`ChangeType == "removed"` 的事件會從 snapshots 移除該 symbol。

**v2 regime tracking**：當 `EventRegimeChangeConfirmed` 到達時，更新內部 `currentRegime` 並重置 `prevTotal = 0`，避免 regime 切換時的偽 turnover 事件。

## Payload Schema

```json
{
  "max_concentration": 0.70,
  "max_symbol": "2330",
  "turnover": 0.50,
  "total_value": 1000000,
  "period_start": "2026-06-22T10:00:00Z",
  "reasons": ["concentration", "turnover", "target_drift"],
  "thresholds": {
    "concentration": 0.25,
    "turnover": 0.15,
    "target_drift": 0.10
  },
  "target_weights": {"2330": 0.30, "2454": 0.25, "2317": 0.25, "2881": 0.20},
  "actual_weights": {"2330": 0.70, "2454": 0.10, "2317": 0.10, "2881": 0.10},
  "max_drift": 0.40,
  "max_drift_symbol": "2330",
  "current_regime": "RISK_ON"
}
```

> **v2 欄位說明**：`target_weights` / `actual_weights` / `max_drift` / `max_drift_symbol` / `current_regime` 僅在 `TargetWeightsProvider` 非 nil 且其 `GetTargetWeights` 回傳非空 map 時才會出現。`thresholds.target_drift` 一律存在（常數）。

| 欄位 | 型別 | 說明 | v1 / v2 |
|------|------|------|---------|
| `max_concentration` | float64 | 最高單一持倉佔比（0~1） | v1 |
| `max_symbol` | string | 對應的 symbol | v1 |
| `turnover` | float64 | 週期內總值變化率 | v1 |
| `total_value` | float64 | 當前 portfolio 總市值 | v1 |
| `period_start` | time.Time (RFC3339) | 本期開始時間 | v1 |
| `reasons` | []string | 觸發原因（"concentration" / "turnover" / "target_drift"） | v1 + v2 |
| `thresholds` | map[string]float64 | 閾值快照 | v1（concentration/turnover）+ v2（target_drift） |
| `target_weights` | map[string]float64 | 當前 regime 的目標 symbol 權重 | v2 |
| `actual_weights` | map[string]float64 | 當前 portfolio 實際 symbol 權重 | v2 |
| `max_drift` | float64 | 最大 `|actual - target|` drift | v2 |
| `max_drift_symbol` | string | drift 最大的 symbol | v2 |
| `current_regime` | string | 當前 market regime（首次 regime change 前為空字串） | v2 |

## 對應 Source Event

- `EventPositionUpdate` payload（`PositionEventPayload`）：
  - `Symbol` / `ChangeType`（"added"/"updated"/"removed"）
  - `Position.MarketValue`（`internal/domain/types.go:12`）用於計算 weight
- `EventRegimeChangeConfirmed` payload（v2 新增，`regime_debouncer.go` 發布，type 為 `map[string]any`）：
  - `new_regime`（string）：新 regime 名稱，用於查詢 target weights
  - `old_regime`、`confirmed_at`、`stability_seconds`、`determined_by`、`confidence`（其他欄位，僅供記錄）

## 對應 Alert Rule

- 對應 Prometheus rule 檔案已於 2026-07-25 移除：原 `monitoring/rules/disabled/wave9_drift_detected.yml` 引用 `portfolio_max_concentration` 與 `portfolio_turnover_5m`，但這兩個 metrics 從未 emit；`EventDriftDetected` 仍透過 eventbus 發布，未來若實作 metric emit 應連同 rule 重新建立。

## 對應 Service

- `internal/monitoring/service/drift_detector.go`：`DriftDetector` 介面 + 實作
- `internal/monitoring/service/drift_helpers.go`：常數、型別（`DriftDetector` 介面、`TargetWeightsProvider` 介面、`driftSnapshot`、absDiff）
- **依賴注入**：
  - `NewDriftDetector(bus eventbus.EventBus)` — 舊版，無 target weights drift 功能（向後相容）
  - `NewDriftDetectorWithTargets(bus eventbus.EventBus, provider TargetWeightsProvider)` — v2 新增，provider 為 nil 時 graceful degradation
- **TargetWeightsProvider 介面**：`GetTargetWeights(regime string) map[string]float64`，回傳 symbol → 目標權重。回傳 nil 或空 map 表示無目標可比較。
- **Event subscriptions**：
  - `EventPositionUpdate`（v1 既有）— 累積 snapshots
  - `EventRegimeChangeConfirmed`（v2 新增）— 更新 `currentRegime` 並 re-baseline
- 純 event-driven：5 分鐘週期檢查

## 測試

- `internal/monitoring/service/drift_detector_test.go`（v1，6 個測試）
- `internal/monitoring/service/drift_detector_v2_test.go`（15 個測試：13 V2 + 2 V1）：
  - `TestDriftDetector_V2TargetDriftEmitted`：target 偏離 > 10% emit + 驗證 v2 payload 欄位
  - `TestDriftDetector_V2TargetDriftNoEmit`：target 對齊 + 平衡組合不 emit
  - `TestDriftDetector_V2NilProviderGraceful`：nil provider 保留 v1 行為
  - `TestDriftDetector_V2EmptyTargetWeights`：空 target map 跳過 target drift
  - `TestDriftDetector_V2RegimeChangeUpdatesCurrentRegime`：handler 更新 currentRegime
  - `TestDriftDetector_V2RegimeChangeRebaselinesPrevTotal`：regime change 重置 prevTotal
  - `TestDriftDetector_V2SymbolNotInTargetMap`：target=0 處理缺漏 symbol
  - `TestDriftDetector_V2SchemaVersionBumped`：SchemaVersion=2
  - `TestDriftDetector_V2ConcurrentProviderAccess`：concurrent 讀取無 race
  - `TestDriftDetector_V1ConstructorEmitsSchemaVersion1`：legacy constructor 維持 SchemaVersion=1
  - `TestDriftDetector_V2RegimeChangeTriggersNewProviderQuery`：regime 改變後使用新 regime 查詢 target weights
  - `TestDriftDetector_V2EmptyRegimeStringPassesToProvider`：首次檢查使用空 regime
  - `TestDriftDetector_V2StopCancelsBothSubscriptions`：v2 Stop 取消兩個訂閱
  - `TestDriftDetector_V1StartDoesNotSubscribeToRegime`：v1 不訂閱 regime confirmed
  - `TestDriftDetector_V2StartSubscribesToBoth`：v2 同時訂閱 position update 與 regime confirmed

**v0.0.0.18+ 整合測試**（PR #704；bus-level 真實 `ChannelEventBus` subscribe→handler→publish chain）：
- `internal/monitoring/service/wave9_integration_test.go`：
  - `TestWave9Integration_DriftDetectorV2Flow`：end-to-end 走 `NewDriftDetectorWithTargets` + 真實 `ChannelEventBus`，驗證 `SchemaVersion=2`、兩種 reasons（`concentration` + `target_drift`）、所有 v2-only payload 欄位（`current_regime` / `target_weights` / `actual_weights` / `max_drift` / `max_drift_symbol`），以及 v1 契約 `concentration` reason 仍正常 emit。
  - `TestWave9Integration_RegimeDebouncerDrivesDriftDetectorV2`：chain test，`RegimeDebouncer` emit `EventRegimeChangeConfirmed` 後 v2 detector 訂閱的 `onRegimeChangeConfirmed` handler 觸發 `prevTotal=0` re-baseline + `currentRegime` 更新，後續 `checkPeriod` emit target_drift。

## Forward-Compat 驗證

- ✅ 只訂閱 `EventPositionUpdate` + `EventRegimeChangeConfirmed` + 讀對應 payload
- ✅ v1 不讀任何 portfolio 內部 state（如 `*portfolio.Optimizer` 或 `factor_weight_engine.go`）
- ✅ 0 修改 #611 9 個檔案
- ✅ v2 透過 `TargetWeightsProvider` 介面注入 target weights（DI pattern），不直接讀 portfolio 內部
- ✅ `NewDriftDetector` 舊 constructor 保留（向後相容）
- ✅ v1 payload 欄位完整保留（append-only）

## v1 vs v2 範圍

**v1**：concentration drift + simple turnover ratio
**v2（PR feat/drift-detector-v2）**：target weights drift（透過 `TargetWeightsProvider` 注入 symbol-level target weights；對比 `WeightProvider` 是 factor-level，不可混用）+ regime tracking（`EventRegimeChangeConfirmed`）
**向後相容**：v1 6 個測試全部 PASS；v1 payload 欄位（max_concentration、max_symbol、turnover、total_value、period_start、reasons、thresholds）一字不刪；SchemaVersion 從 1 bump 到 2

## 啟用情境（建議）

- **Risk management**：70% 集中於單一 symbol 警示，建議人工 rebalance
- **異常交易偵測**：5 分鐘內 turnover > 15% 表示高頻交易，可能為異常行為
- **回測驗證**：drift 偵測準確率對 portfolio sharpe 影響的 A/B test 基準