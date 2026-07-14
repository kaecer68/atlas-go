# `experiment.insufficient_data` — 資料不足事件

> **Wave**：8.x（規劃中）
> **穩定性**：draft
> **首次上線**：未上線
> **EventType 常數**：`eventbus.EventInsufficientData`（待定義）
> **字串值**：`"experiment.insufficient_data"`
> **Severity**：`warning`

---

## 用途

當實驗因資料不足（例如歷史價格序列太短、缺少必要特徵）而無法執行回測或訓練時，發布本事件供監控與告警使用。

---

## 觸發點

`internal/experiment/data_check.go` 的 `CheckDataSufficiency(strategy, symbols)` 函式（待實作）。

---

## Schema

```json
{
  "experiment_id": "<uuid>",
  "strategy_id": "<uuid>",
  "missing_symbols": ["<string>"],
  "min_required_days": <int>,
  "actual_available_days": <int>,
  "detected_at": "<ISO8601>"
}
```

---

## 消費者

- SSE 即時串流
- JSONL 審計軌跡
- Prometheus 計數器 `atlas_insufficient_data_total`

---

## 相關事件

- [`experiment.backtest_completed`](./backtest-completed.md) — 資料充足時的後續事件