# Sector Prediction Observation Log

> **對應 runbook**：`docs/operations/sector-prediction-runbook.md`
> **對應 spec**：[`docs/specs/sector-dimension-prediction.md`](../specs/sector-dimension-prediction.md)
> **對應 invariant manifest**：[`docs/manifests/sector-dimension-prediction-invariant-manifest.md`](../manifests/sector-dimension-prediction-invariant-manifest.md)

L2.4-style 觀察窗口的逐日記錄表。每個交易日填寫一次，欄位說明見 runbook §「Daily Check-in」。

## Record Schema（每行）

```text
| 日期 | sector_count | jsd.alert_rate | latency_p95_ms | confidence_violation | panic_count | spot_check_count | notes |
```

## Records

<!-- 由觀察窗口 owner 於每交易日收盤後填寫；首次填寫前此檔保持空白 -->

<!--
範例列（Day 1 placeholder；實際填寫時請刪除）:
| 2026-07-21 | 20 | 0.0% | 145 | 0 | 0 | 0 | 啟用 `SECTOR_PREDICTION_ENABLED=true`；baseline established |
-->
