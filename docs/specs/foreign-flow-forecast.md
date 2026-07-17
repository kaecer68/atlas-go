# 外資方向推估 v1（Foreign Flow Forecast）

> **文件角色**：manifest #E03 的設計文件，定義 v1 計分卡（scorecard）、資料流、校準哲學與啟用門檻。
> **對齊**：[`docs/reference/product-positioning.md`](../reference/product-positioning.md) §7、§8。

---

## 1. 目標

利用「**領先訊號**」（TAIFEX 外資期貨未平倉）+「**現貨確認**」（每日外資現貨買賣超）+「**外部環境**」（美股、匯率、VIX）對**下一個交易日**外資現貨買賣超的方向做機率性預測，給散戶一個有歷史命中率支撐的方向提示，**而非保證**。

## 2. 預測目標定義

- **target**：下一個交易日（T+1）外資現貨買賣超的方向。
  - bullish：T+1 外資現貨淨買超 > 0
  - bearish：T+1 外資現貨淨賣超 < 0
  - neutral：abs(淨額) < 顯著性門檻（見 §6 顯著性）
- **預測單位**：單一方向 + 機率（0..1），不做點估計、不做區間。

## 3. 輸入特徵（v1 scorecard）

| 特徵 | 來源 | 方向假設 | 權重 |
|------|------|----------|------|
| `foreign_futures_oi_net_z` | TAIFEX `#E01` ForeignFuturesOINet 60 日 Z | 領先訊號，正 Z → bullish | **0.30** |
| `foreign_spot_5d_slope` | snap.ForeignInvestorNet 過去 5 個交易日斜率（億/日） | 正斜率 → bullish | 0.20 |
| `tsm_adr_change_pct` | snap.TSMADR.ChangePct | 收紅 → bullish | 0.15 |
| `spx_change_pct` | snap.SPXIndex.ChangePct | 收紅 → bullish | 0.15 |
| `ndx_change_pct` | snap.NDXIndex.ChangePct | 收紅 → bullish（半導體連動） | 0.10 |
| `usd_twd_change_pct` | snap.USD_TWD.ChangePct | **反**向：台幣升值 → bullish（資金流入） | 0.10 |
| `vix` | snap.VIX.Value | 修飾項：>25 時整體 -0.10×(vix-25)/10 | ±0.10 |

權重總和 = 1.00（加上 VIX 修飾實際為 0.90-1.00 範圍，後續以 logistic squash 收斂）。

## 4. 計分

```
score = sum(weight_i * tanh(feature_i / scale_i))
prob  = 0.5 + 0.5 * score   // squash 到 [0, 1]
direction =
  prob >= 0.60 → bullish
  prob <= 0.40 → bearish
  else        → neutral
```

`tanh` 在 v1 取代 sigmoid——對稱、無需選定零點；當特徵值為 0 時貢獻為 0。`scale` 為該特徵的「典型顯著波動」（如 spx_change_pct 的 scale 為 1.5 表示 ±1.5% 視為滿分）。

## 5. 顯著性門檻

預測 T+1 是 bullish/bearish 而非 neutral，要求 T+1 實際 |淨買賣超| > **30 億台幣**（外資典型單日規模）；小於此門檻視為中性（即使方向猜對也不算「命中」，預測落為 neutral）。

## 6. 校準與啟用門檻（§8 對齊）

預測 v1 **不上線即開用**——必須滿足：

1. **樣本量** ≥ 90 個交易日的「預測 vs 實際」紀錄（自 E03 上線起算；每日由 background 排程寫入 ledger）。
2. **90 日滾動命中率** ≥ 55%（以顯著性門檻計算）。
3. **近期表現**：連續 5 個交易日內正確率 ≥ 60% 作為輔助條件（避免「歷史好但近期失效」）。

任一未達 → API 回傳 `"calibrated": false, "reason": "校準中（樣本 N/90、命中率 X%）"` + 仍附上當下預測（僅做為方向參考，不參與自動下單邏輯——本平台亦不做自動下單）。

## 7. 資料流

```
每日 E01 (taifex_institutional) + 大盤 snapshot
  → internal/forecast/foreign_forecast.go Score()
  → 寫 ledger：data/state/foreign_forecast/YYYYMMDD.json
  → T+1 收盤後：background 補 actual_outcome + correct 欄位
  → 90 日滾動命中率（in-memory，process-local——同 BK-15 限制）
```

Ledger schema：
```json
{
  "date": "20260716",
  "predicted_direction": "bearish",
  "probability": 0.62,
  "score": -0.24,
  "actual_outcome": "neutral",   // 補入於 T+1
  "actual_net": 15000000000,     // 補入於 T+1（小於顯著性門檻則落 neutral）
  "correct": null               // null = 待補；true/false = 已判定
}
```

## 8. v1 的局限（誠實聲明）

- **過程式啟發式**：v1 用加權特徵和，**不是機器學習模型**。所有權重與特徵必須可逐條向散戶解釋（§8）。
- **同向共線性**：futures OI Z 與 spot 淨額高度共線，導致 v1 等同於「跟著期貨走」；長期需引入正交化特徵（散戶融資、選擇權 PCR 等）。
- **30 億顯著性門檻**為經驗值，未做歷史敏感度分析；後續 §8 校準流程會重新估計。
- **90 日滾動命中率是 process-local**——重啟即清空；首次啟用後至少需連續運行 90 個交易日才能驗證。

## 9. 後續工作

- E03 v1 上線後，§8 校準哲學流程應對各權重 / scale 做敏感度分析、回測 Sharpe-like、寫回 `parameters.json`。
- 累積 ≥ 90 個交易日後，啟用對外展示（API、`/api/capital-flow/daily` 的 `forecast` 區塊）。
- 新特徵候選：選擇權 PCR、融資增減、晶圓代工同業 ADR（聯電、世界先進）、亞股連動（韓股 KOSPI、日股 TOPIX 隔夜）。