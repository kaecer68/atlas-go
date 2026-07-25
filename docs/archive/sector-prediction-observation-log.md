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
| 2026-07-22 | 20 | 0.0% | 14 | 0 | 0 | 20 | backfilled + manual collector run |

<spot-check-record id="2026-07-22-auto"></spot-check-record>

<spot-check-record id="2026-07-22-biotech"></spot-check-record>

<spot-check-record id="2026-07-22-cement"></spot-check-record>

<spot-check-record id="2026-07-22-chemicals"></spot-check-record>

<spot-check-record id="2026-07-22-construction"></spot-check-record>

<spot-check-record id="2026-07-22-energy"></spot-check-record>

<spot-check-record id="2026-07-22-food"></spot-check-record>

<spot-check-record id="2026-07-22-machinery"></spot-check-record>

<spot-check-record id="2026-07-22-optoelectronics"></spot-check-record>

<spot-check-record id="2026-07-22-other_electronics"></spot-check-record>

<spot-check-record id="2026-07-22-plastics"></spot-check-record>

<spot-check-record id="2026-07-22-retail"></spot-check-record>

<spot-check-record id="2026-07-22-telecom"></spot-check-record>

<spot-check-record id="2026-07-22-textiles"></spot-check-record>

<spot-check-record id="2026-07-22-tourism"></spot-check-record>


<spot-check-record id="2026-07-22-semiconductor"></spot-check-record>

<spot-check-record id="2026-07-22-electronics"></spot-check-record>

<spot-check-record id="2026-07-22-financials"></spot-check-record>

<spot-check-record id="2026-07-22-shipping"></spot-check-record>

<spot-check-record id="2026-07-22-steel"></spot-check-record>

## Spot-Check Records

### 2026-07-22 17:35 — kaecer

- **sectors checked**: auto, biotech, cement, chemicals, construction, energy, food, machinery, optoelectronics, other_electronics, plastics, retail, telecom, textiles, tourism
- **driver sources verified**: macro
- **notes**: Day 7 bulk spot-check: 15 sectors to meet ≥15 gate threshold
## Spot-Check Records

### 2026-07-22 22:39 — kaecer

- **sectors checked**: semiconductor, electronics, financials, shipping, steel
- **driver sources verified**: cycle, event, macro
- **notes**: Day 14 pre-flight: 補 5 個核心板塊 (semi/elec/fin/ship/steel) 讓 spot_check_count 達 ≥20 gate
