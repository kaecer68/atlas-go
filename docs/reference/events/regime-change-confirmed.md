# RegimeChangeConfirmed 事件

> **事件常數**：`EventRegimeChangeConfirmed = "market.regime.confirmed"`
> **對應 EventType 字串值**：`market.regime.confirmed`
> **Severity**：info（PD-W9-1）
> **Schema Version**：1

## 觸發條件

RegimeDebouncer 訂閱 `EventRegimeChange`（eventbus event），內部 state machine 追蹤：
- `pending`（最新的 RegimeEventPayload）
- `pendingSince`（pending 收到的時間）
- `lastEmittedNew`（上次 emit 確認的新 regime，用於 dedup）

每 5 秒（`RegimeDebounceCheckInt`）檢查一次：
1. 若無 pending → 不 emit
2. 若 pending.newRegime == lastEmittedNew → 不 emit（已確認過此 regime）
3. 若 `now - pendingSince < 30s`（`RegimeStabilityWindow`）→ 不 emit（尚未穩定）
4. 否則 → emit `EventRegimeChangeConfirmed`

## Payload Schema

```json
{
  "old_regime": "bull",
  "new_regime": "bear",
  "confirmed_at": "2026-06-22T10:35:00Z",
  "stability_seconds": 31,
  "determined_by": "macro_narrative_agent",
  "confidence": 0.82
}
```

| 欄位 | 型別 | 說明 |
|------|------|------|
| `old_regime` | string | regime 變化前的 regime |
| `new_regime` | string | regime 變化後的 regime（已穩定 30 秒） |
| `confirmed_at` | time.Time (RFC3339) | confirmed 事件 emit 時間 |
| `stability_seconds` | int | new regime 已穩定秒數（≥ 30） |
| `determined_by` | string | regime 偵測來源（從 EventRegimeChange.DeterminedBy） |
| `confidence` | float64 | regime 變化信心度（從 EventRegimeChange.Confidence） |

## 對應 Source Event

- `EventRegimeChange`：當 `internal/orchestrator/system.go:431,714`（`PublishRegimeChange`）發出 regime 變化時觸發
- ⚠️ **風險**：`PublishRegimeChange` 路徑經 `internal/orchestrator/system.go`，是 #611 Wave 2 P1 refactor 目標。緩解：Wave 9.6 加 compile-time assertion 驗證 EventRegimeChange 仍可訂閱

## 對應 Alert Rule

- 對應 Prometheus rule 檔案已於 2026-07-25 移除：原 `monitoring/rules/disabled/wave9_regime_change_confirmed.yml` 引用 `regime_change_confirmed_total` 但該 metric 從未 emit；`EventRegimeChangeConfirmed` 仍透過 eventbus 發布，未來若實作 metric emit 應連同 rule 重新建立。

## 對應 Service

- `internal/monitoring/service/regime_debouncer.go`：`RegimeDebouncer` 介面 + 實作
- 依賴注入：`NewRegimeDebouncer(bus eventbus.EventBus)`（無 provider 依賴）
- 由 `internal/monitoring/dashboard_api.go` 或同等 wiring 程式呼叫 `Start(ctx)` / `Stop()`

## 測試

- `internal/monitoring/service/regime_debouncer_test.go`：
  - `TestRegimeDebouncer_NoEmitWithoutEvent`：無 pending 不 emit
  - `TestRegimeDebouncer_EmitsAfterStabilityWindow`：30 秒穩定後 emit
  - `TestRegimeDebouncer_ResetsWindowOnNewChange`：新變化重置 stability window
  - `TestRegimeDebouncer_DedupSameRegime`：同 regime 只 emit 一次
  - `TestRegimeDebouncer_StartStopLifecycle`：背景 poll 正常運作

## Forward-Compat 驗證

- ✅ 只訂閱 `EventRegimeChange` eventbus event（eventbus.go 不在 #611 list）
- ✅ 不讀 `RegimeAllocator.GetCurrentRegime()`（regime.go 在 #611 Wave 2 範圍）
- ✅ debouncer 完全在 monitoring/service 層，零 portfolio 內部依賴

## 啟用情境（建議）

- **保守交易訊號**：regime 穩定 30 秒後才視為可信號，避免假突破
- **Rebalance 觸發**：regime change confirmed 後觸發 portfolio rebalance（搭配 FactorWeightRegression）
- **波動性監控**：短時間多次 confirmed 表示市場反覆，暫停自動化操作
- **回測驗證**：regime 穩定窗口長短對回測結果影響的 A/B test 基準