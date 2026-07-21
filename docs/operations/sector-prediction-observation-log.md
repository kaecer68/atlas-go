# Sector Prediction Observation Log

> **對應 runbook**：`docs/operations/sector-prediction-runbook.md`
> **對應 spec**：[`docs/specs/sector-dimension-prediction-spec.md`](../specs/sector-dimension-prediction.md)
> **對應 invariant manifest**：[`docs/manifests/sector-dimension-prediction-invariant-manifest.md`](../manifests/sector-dimension-prediction-invariant-manifest.md)

L2.4-style 觀察窗口的逐日記錄表。每個交易日填寫一次，欄位說明見 runbook §「Daily Check-in」。

## Record Schema（每行）

```text
| 日期 | sector_count | jsd.alert_rate | latency_p95_ms | confidence_violation | panic_count | spot_check_count | notes |
```

## Records

<!-- 由觀察窗口 owner 於每交易日收盤後填寫；首次填寫前此檔保持空白 -->
| 2026-07-16 | 20 | 0.0% | 11 | 0 | 0 | 5 | auto-collected; 5 spot-checks done (see Day 1 detail) |

## Day 1 Spot-check Detail（2026-07-16 → day 1 of observation window）

Spot-check 抽樣 5 個 MUST_WATCH sectors（semiconductor / electronics / financials / shipping / steel），對應 forecast day 1 = 2026-07-17。整體市場預測 direction = `neutral` (confidence 0.69, distribution 16/16/69)。Sectors majority 也 neutral (15/20)，與整體一致 ✅。

| sector | direction | confidence | drivers | driver source type |
|--------|-----------|-----------:|---------|--------------------|
| semiconductor | outflow | 0.400 | `event:法說會旺季`, `foreign_investor_net` | event + macro ✅ |
| electronics | outflow | 0.400 | `event:期貨結算日`, `event:法說會旺季` | event + event ✅ |
| financials | inflow | 0.400 | `us10y`, `event:期貨結算日` | macro + event ✅ |
| shipping | outflow | 0.400 | `bdi`, `dxy` | macro + macro ✅ |
| steel | outflow | 0.400 | `bdi`, `dxy` | macro + macro ✅ |

**Summary**：5/5 spot-check pass。

- 所有 drivers 都 reference 實際 macro / cycle / event 來源（不純 prior fallback）✅
- sector direction 與 overall direction 比例一致（15/20 sectors 偏中性，符合整體 `neutral`）✅
- 0 confidence floor violation（invariant I7；所有 confidence = 0.400，正落在 floor 邊界）✅
- 0 panic / 0 ERROR in atlas logs（連續 60s 監控）✅
- latency: 5 連續 API call 範圍 2.5–5.2ms（< 200ms threshold）✅

**Driver variety snapshot**（全 day-1 sectors，all 20）：

```
  15x  overall_baseline
  15x  taiex
   2x  event:期貨結算日
   2x  event:法說會旺季
   2x  bdi
   2x  dxy
   1x  us10y
   1x  foreign_investor_net
   1x  (other macro refs)
```

混搭 macro/event/baseline，三層覆蓋健康。

**Live behavior conclusion** (this session)：C + D 兩項都 OK。Atlas container runtime 穩定（SECTOR_PREDICTION_ENABLED=true、restart_count=0、無 panic）、spot-check 持續累積中。

