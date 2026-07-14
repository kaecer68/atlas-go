# `risk.stoploss_triggered` — 停損觸發事件

> **Wave**：8.x（規劃中）
> **穩定性**：draft
> **首次上線**：未上線
> **EventType 常數**：`eventbus.EventStoplossTriggered`（待定義）
> **字串值**：`"risk.stoploss_triggered"`
> **Severity**：`info`

---

## 用途

當部位的停損條件被觸發並產生對應出場訂單時，發布本事件供審計軌跡與策略表現分析使用。

---

## 觸發點

`internal/risk/stoploss.go` 的 `TriggerStoploss(positionID, price)` 函式（待實作）。

---

## Schema

```json
{
  "position_id": "<uuid>",
  "strategy_id": "<uuid>",
  "trigger_price": <float>,
  "stop_price": <float>,
  "loss_pct": <float>,
  "executed_at": "<ISO8601>"
}
```

---

## 消費者

- SSE 即時串流
- JSONL 審計軌跡
- Prometheus 計數器 `atlas_stoploss_triggered_total`

---

## 相關事件

- [`risk.takeprofit_triggered`](./risk-takeprofit-triggered.md) — 對稱事件
- [`risk.drawdown_breach`](./drawdown-breach.md) — drawdown 監控指標