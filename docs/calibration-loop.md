# 自主校準閉環架構

從 Phase C6-D5 開始導入的自主演化架構，目標是讓系統自我校準、自我進化。

## 校準閉環流程

JANUS regime detection（每小時）→ regime change 偵測 → RiskGate.SelfCalibrate()：
1. 載入最近 30 個 session 的推薦與 forward return
2. 重播 pre-trade 決策（哪些被 block、哪些被 allow）
3. 對比實際結果（block 壞單 = TP, block 好單 = FP）
4. 計算 F1 score + precision/recall
5. Bayesian optimizer 搜尋最佳 threshold
6. 套用新參數 → 記錄 CalibrationReport

固定排程校準（每 24h）：`risk_gate_calibrate` task。

## 校準範圍

| 規則 | 參數 | 預設值 |
|------|------|--------|
| max_position_pct | risk_max_position_size | 0.15 |
| cash_buffer | risk_max_daily_loss_pct | 0.03 |

## 背景任務一覽

| Task | 間隔 | 觸發條件 | 行為 |
|------|------|----------|------|
| risk_gate_calibrate | 24h | 時間到 | 載入 30 session → 校準參數 |
| regime_calibrate | 1h | regime 變化 | 載入 20 session → 校準參數 |
| rule_engine_check | 30s | 時間到 | 檢查警報規則 |

所有閉環行為透過結構化 logging 輸出，CalibrationReport → `GET /api/dashboard/risk-calibration` 端點。
