# `risk.gate_rejected` — 風險閘門拒絕事件

> **Wave**：8.2
> **穩定性**：stable
> **首次上線**：v0.0.0.x
> **EventType 常數**：`eventbus.EventRiskGateRejected`
> **字串值**：`"risk.gate_rejected"`
> **Severity**：`warning`

---

## 用途

當提議的訂單、部位調整或策略動作被風險閘門（RiskGate）拒絕時發布本事件。與 `risk.gate_overridden`（管理員覆寫）配對使用。

---

## 觸發點

`internal/risk/gate.go` 的 `Reject(orderID, reason)` 函式（Wave 8.2）。

---

## Schema

```json
{
  "order_id": "<uuid>",
  "strategy_id": "<uuid>",
  "rejection_reason": "<string>",
  "gate_layer": "<string>",
  "risk_score": <float>,
  "detected_at": "<ISO8601>"
}
```

---

## 消費者

- SSE 即時串流
- JSONL 審計軌跡
- Prometheus 計數器 `atlas_risk_gate_rejected_total`

---

## 相關事件

- [`risk.gate_overridden`](./risk-gate-overridden.md) — 管理員覆寫此拒絕事件
- [`risk.alert`](./risk-alert.md) — 高嚴重性時升級為 alert