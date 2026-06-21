# `risk.takeprofit_triggered` — 停利觸發事件

> **Wave**：8.x（規劃中）
> **穩定性**：draft
> **首次上線**：未上線
> **EventType 常數**：`eventbus.EventTakeprofitTriggered`（待定義）
> **字串值**：`"risk.takeprofit_triggered"`
> **Severity**：`info`

---

## 用途

當部位的停利條件被觸發並產生對應出場訂單時，發布本事件供審計軌跡與策略表現分析使用。

---

## 觸發點

`internal/risk/takeprofit.go` 的 `TriggerTakeprofit(positionID, price)` 函式（待實作）。

---

## Schema

```json
{
  "position_id": "<uuid>",
  "strategy_id": "<uuid>",
  "trigger_price": <float>,
  "target_price": <float>,
  "profit_pct": <float>,
  "executed_at": "<ISO8601>"
}
```

---

## 消費者

- SSE 即時串流
- JSONL 審計軌跡
- Prometheus 計數器 `atlas_takeprofit_triggered_total`

---

## 相關事件

- [`risk.stoploss_triggered`](./risk-stoploss-triggered.md) — 對稱事件
- [`risk.alert`](./risk-alert.md) — 高頻觸發時升級為 alert