# DriftDetected 事件

> **事件常數**：`EventDriftDetected = "portfolio.drift.detected"`
> **對應 EventType 字串值**：`portfolio.drift.detected`
> **Severity**：info（PD-W9-1）
> **Schema Version**：1

## 觸發條件

DriftDetector 訂閱 `EventPositionUpdate`，內部維護 `symbol → snapshot` map 與週期性 turnover 計算。

每 5 分鐘（`DriftCheckInterval`）呼叫 `checkPeriod`：
1. 計算 `total_value` = Σ symbol.MarketValue
2. 計算 `max_concentration` = max(symbol.MarketValue / total_value)
3. 計算 `turnover` = |total_value - prevTotal| / prevTotal
4. 觸發條件（任一）：
   - `max_concentration ≥ 0.25`（`DriftMaxConcentrationThreshold`）
   - `turnover ≥ 0.15`（`DriftTurnoverThreshold`）
5. 觸發時 emit `EventDriftDetected`，payload 包含 `reasons` 陣列標示哪個條件觸發

`ChangeType == "removed"` 的事件會從 snapshots 移除該 symbol。

## Payload Schema

```json
{
  "max_concentration": 0.70,
  "max_symbol": "2330",
  "turnover": 0.50,
  "total_value": 1000000,
  "period_start": "2026-06-22T10:00:00Z",
  "reasons": ["concentration", "turnover"],
  "thresholds": {
    "concentration": 0.25,
    "turnover": 0.15
  }
}
```

| 欄位 | 型別 | 說明 |
|------|------|------|
| `max_concentration` | float64 | 最高單一持倉佔比（0~1） |
| `max_symbol` | string | 對應的 symbol |
| `turnover` | float64 | 週期內總值變化率 |
| `total_value` | float64 | 當前 portfolio 總市值 |
| `period_start` | time.Time (RFC3339) | 本期開始時間 |
| `reasons` | []string | 觸發原因（"concentration" / "turnover"） |
| `thresholds` | map[string]float64 | 閾值快照 |

## 對應 Source Event

- `EventPositionUpdate` payload（`PositionEventPayload`）：
  - `Symbol` / `ChangeType`（"added"/"updated"/"removed"）
  - `Position.MarketValue`（`internal/domain/types.go:12`）用於計算 weight

## 對應 Alert Rule

- `monitoring/rules/wave9_drift_detected.yml`（2 個規則）：
  - `PortfolioConcentrationDrift`：max_concentration > 0.25 持續 5 分鐘 → warning
  - `PortfolioTurnoverDrift`：5m turnover > 0.15 持續 5 分鐘 → info
- 預設 `enabled: false`（PD-W9-1）

## 對應 Service

- `internal/monitoring/service/drift_detector.go`：`DriftDetector` 介面 + 實作
- 依賴注入：`NewDriftDetector(bus eventbus.EventBus)`（無 portfolio 內部依賴）
- 純 event-driven：訂閱 EventPositionUpdate 聚合 MarketValue，5 分鐘週期檢查

## 測試

- `internal/monitoring/service/drift_detector_test.go`：
  - `TestDriftDetector_NoEmitOnLowConcentration`：4 個 symbol 各 25% 不 emit（threshold 25% 為 exclusive）
  - `TestDriftDetector_EmitsOnHighConcentration`：70% 集中 emit + 驗證 max_symbol + reasons
  - `TestDriftDetector_EmitsOnHighTurnover`：50% turnover emit + 驗證 reasons 包含 "turnover"
  - `TestDriftDetector_RemovedSymbolCleared`：ChangeType=removed 從 snapshots 清除
  - `TestDriftDetector_EmptyPortfolioNoEmit`：空 portfolio 不 emit（避免 divide by zero）
  - `TestAbsDiff`：3 個 sub-cases（a>b / a<b / a==b）

## Forward-Compat 驗證

- ✅ 只訂閱 EventPositionUpdate + 讀 PositionEventPayload（含 Position.MarketValue）
- ✅ 不讀任何 portfolio 內部 state（如 `*portfolio.Optimizer` 或 `factor_weight_engine.go`）
- ✅ 0 修改 #611 9 個檔案

## v1 vs v2 範圍

**v1（本 PR）**：concentration drift + simple turnover ratio
**v2（推遲至 #611 refactor 完成）**：target_weights drift（需要讀 portfolio.Optimizer 內部 target weight，會與 #611 conflict）

## 啟用情境（建議）

- **Risk management**：70% 集中於單一 symbol 警示，建議人工 rebalance
- **異常交易偵測**：5 分鐘內 turnover > 15% 表示高頻交易，可能為異常行為
- **回測驗證**：drift 偵測準確率對 portfolio sharpe 影響的 A/B test 基準