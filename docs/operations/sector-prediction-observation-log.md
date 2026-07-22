# Sector Prediction Observation Log

> **對應 runbook**：`docs/operations/sector-prediction-runbook.md`
> **對應 spec**：[`docs/specs/sector-dimension-prediction-spec.md`](../specs/sector-dimension-prediction-spec.md)
> **對應 invariant manifest**：[`docs/manifests/README.md`](../manifests/README.md)（governance templates 入口,individual manifests 走 `.omo/manifests/`）

L2.4-style 觀察窗口的逐日記錄表。每個交易日填寫一次,欄位說明見 runbook §「Daily Check-in」。

## Record Schema（每行）

```text
| 日期 | sector_count | jsd.alert_rate | latency_p95_ms | confidence_violation | panic_count | spot_check_count | notes |
```

## Records

| 2026-07-16 | 20 | 0.0% | 9 | 0 | 0 | 5 | backfilled from PR #1206 spot-check |
| 2026-07-17 | 20 | 0.0% | 5075 | 0 | 0 | 0 | backfilled — first prod API call, cold-cache p95 |
| 2026-07-21 | 20 | 0.0% | 11 | 0 | 0 | 0 | backfilled |
| 2026-07-22 | 20 | 0.0% | 14 | 0 | 0 | 0 | backfilled + manual collector run |