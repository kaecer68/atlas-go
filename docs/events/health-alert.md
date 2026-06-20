# `monitoring.health_alert` — 健康告警事件

> **Wave**：8.x（規劃中）
> **穩定性**：draft
> **首次上線**：未上線
> **EventType 常數**：`eventbus.EventHealthAlert`（待定義）
> **字串值**：`"monitoring.health_alert"`
> **Severity**：`warning`

---

## 用途

當系統健康指標（CPU、記憶體、磁碟、網路延遲、API 健康分數等）連續 N 次超過閾值時，發布本事件供監控與告警使用。

---

## 觸發點

`internal/monitoring/health.go` 的 `CheckHealthThresholds(metrics)` 函式（待實作）。

---

## Schema

```json
{
  "alert_id": "<uuid>",
  "metric_name": "<string>",
  "current_value": <float>,
  "threshold": <float>,
  "consecutive_breaches": <int>,
  "affected_component": "<string>",
  "detected_at": "<ISO8601>"
}
```

---

## 消費者

- SSE 即時串流
- JSONL 審計軌跡
- Prometheus 計數器 `atlas_health_alert_total`
- PagerDuty / Slack 整合（高 severity 時）

---

## 相關事件

- [`experiment.calibration_completed`](./calibration-completed.md) — 校準完成事件，可降低健康告警頻率