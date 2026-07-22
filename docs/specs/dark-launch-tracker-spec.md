# SA11 — Dark Launch 觀察期追蹤

> **開始日期**：2026-07-12（F06 策略競爭真實排名部署日）
> **目標**：累積 ≥20 個模擬交易 session，觀察真實預測命中率
> **達標後動作**：分析命中率 → 若 >55%，啟動 Predicted Trade Cycle 設計實作

## 進度

| 日期 | Session ID | Outcomes | Regime | 累積 |
|------|-----------|----------|--------|------|
| 2026-07-18 | session-20260718-daily | 27 | RISK_OFF | 7 |
| 2026-07-17 | session-20260717-daily | 27 | RISK_OFF | 6 |
| 2026-07-16 | session-20260716-daily | 27 | RISK_OFF | 5 |
| 2026-07-15 | session-20260715-daily | 26 | RISK_OFF | 4 |
| 2026-07-14 | session-20260714-daily | 27 | RISK_OFF | 3 |
| 2026-07-13 | session-20260713-daily | 26 | RISK_OFF | 2 |
| 2026-07-12 | session-20260712-daily | 26 | RISK_OFF | 1 |

**目前**：7 / 20（還需 13 個交易日，約 3 週）

## 追蹤方式

```bash
# 每日檢查最新 simulation session 數量
curl -s http://localhost:18080/api/dashboard/sessions | jq '[.sessions[] | select(.session_id >= "session-20260712")] | length'
```

## 達標後分析項目

1. 各策略命中率（F06 comparison engine 輸出）
2. 預測方向 vs 實際漲跌的一致性
3. 不同 Regime 下的策略表現差異
4. 判斷是否滿足 Predicted Trade Cycle 實作前提（命中率 >55%）
