# `risk.drawdown_breach` — 最大回撤突破事件

> **Wave**：8.x（規劃中）
> **穩定性**：draft
> **首次上線**：未上線
> **EventType 常數**：`eventbus.EventDrawdownBreach`（待定義）
> **字串值**：`"risk.drawdown_breach"`
> **Severity**：`critical`

---

## 用途

當策略或投資組合的回撤突破設定的閾值（例如 -15%），發布本事件供緊急風險控管與自動減倉使用。

---

## 觸發點

`internal/risk/monitor.go` 的 `CheckDrawdownBreach(portfolioID, maxDrawdownPct)` 函式（待實作）。

---

## Schema

```json
{
  "portfolio_id": "<uuid>",
  "strategy_id": "<uuid>",
  "current_drawdown_pct": <float>,
  "threshold_pct": <float>,
  "breach_duration_hours": <int>,
  "detected_at": "<ISO8601>"
}
```

---

## 消費者

- SSE 即時串流
- JSONL 審計軌跡
- Prometheus 計數器 `atlas_drawdown_breach_total`
- 自動減倉 / 停損機制

---

## 相關事件

- [`performance.sharpe_degradation`](./sharpe-degradation.md) — performance 惡化早期指標
- [`risk.stoploss_triggered`](./risk-stoploss-triggered.md) — drawdown 後的執行結果