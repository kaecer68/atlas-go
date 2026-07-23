# `performance.sharpe_degradation` — Sharpe 比率退化事件

> **Wave**：8.x（規劃中）
> **穩定性**：draft
> **首次上線**：未上線
> **EventType 常數**：已移除（`EventSharpeDegradation` 從未實作，2026-07-24 zombie cleanup — 保留本文件作為未來實作參考）
> **字串值**：`"performance.sharpe_degradation"`
> **Severity**：`warning`

---

## 用途

當運行中策略的 Sharpe 比率連續 N 天低於閾值（例如 < 0.5）時，發布本事件供 risk management 與策略退役流程使用。

---

## 觸發點

`internal/performance/monitor.go` 的 `CheckSharpeDegradation(strategyID, lookbackDays)` 函式（待實作）。

---

## Schema

```json
{
  "strategy_id": "<uuid>",
  "current_sharpe": <float>,
  "threshold": <float>,
  "lookback_days": <int>,
  "consecutive_days_below": <int>,
  "detected_at": "<ISO8601>"
}
```

---

## 消費者

- SSE 即時串流
- JSONL 審計軌跡
- Prometheus 計數器 `atlas_sharpe_degradation_total`

---

## 相關事件

- [`risk.drawdown_breach`](./drawdown-breach.md) — 另一個 performance 監控事件