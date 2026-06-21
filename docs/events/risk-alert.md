# `risk.alert` — 風險告警事件

> **Wave**：8.x（規劃中）
> **穩定性**：draft
> **首次上線**：未上線
> **EventType 常數**：`eventbus.EventRiskAlert`（待定義）
> **字串值**：`"risk.alert"`
> **Severity**：`critical`

---

## 用途

當多個風險事件同時觸發、或單一風險事件嚴重性升級時，發布本事件作為統一告警入口，與其他 risk.* 事件配對使用。

---

## 觸發點

`internal/risk/aggregator.go` 的 `AggregateRiskAlert(alerts)` 函式（待實作）。

---

## Schema

```json
{
  "alert_id": "<uuid>",
  "severity": "<critical|high|medium>",
  "triggered_events": ["<event_type>", ...],
  "affected_strategies": ["<uuid>", ...],
  "recommended_action": "<string>",
  "detected_at": "<ISO8601>"
}
```

---

## 消費者

- SSE 即時串流
- JSONL 審計軌跡
- Prometheus 計數器 `atlas_risk_alert_total`
- PagerDuty / Slack 整合

---

## 相關事件

- [`risk.gate_rejected`](./risk-gate-rejected.md) — 觸發來源
- [`risk.gate_overridden`](./risk-gate-allowed.md) — 觸發來源
- [`risk.drawdown_breach`](./drawdown-breach.md) — 觸發來源
- [`risk.stoploss_triggered`](./risk-stoploss-triggered.md) — 觸發來源
- [`risk.takeprofit_triggered`](./risk-takeprofit-triggered.md) — 觸發來源