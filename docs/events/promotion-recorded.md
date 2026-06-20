# `experiment.promotion_recorded` — 策略晉升記錄事件

> **Wave**：8.x（規劃中）
> **穩定性**：draft
> **首次上線**：未上線
> **EventType 常數**：`eventbus.EventPromotionRecorded`（待定義）
> **字串值**：`"experiment.promotion_recorded"`
> **Severity**：`info`

---

## 用途

當策略經過評估後被晉升至正式 production pool（例如從 candidate pool 提升至 live trading pool）時發布本事件，供審計軌跡與 Prometheus 監控使用。

---

## 觸發點

`internal/experiment/promotion.go` 的 `RecordPromotion(strategyID, fromPool, toPool)` 函式（待實作）。

---

## Schema

```json
{
  "strategy_id": "<uuid>",
  "from_pool": "<string>",
  "to_pool": "<string>",
  "promoted_at": "<ISO8601>",
  "promoter": "<string>",
  "metadata": { ... }
}
```

---

## 消費者

- SSE 即時串流
- JSONL 審計軌跡
- Prometheus 計數器 `atlas_promotion_total`

---

## 相關事件

- [`experiment.backtest_completed`](./backtest-completed.md) — 晉升前的回測依據