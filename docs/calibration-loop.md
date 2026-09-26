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

## 校準值的落點：SSOT ＋ overlay（FU-20260926-07）

校準值**不再寫回** `configs/parameters.json`。該檔是**受版控的 SSOT**（出廠值、人工審查），而且
`configs/` **不在** bind mount（只有 `data/`、`reports/`、`logs/` 在）⇒ 寫進去只會落在容器可寫層：
git 看不到、容器重建即失、且在容器活著時**無法分辨**「SSOT 值」與「校準後的值」。

| 角色 | 路徑 | 生命週期 |
|------|------|----------|
| SSOT（唯讀基準） | `configs/parameters.json`（image 內，受版控） | 每次重建回到 repo 值 |
| 校準 overlay | `data/state/parameters.calibrated.json`（`constants.StateParametersCalibrated`） | **binding mount**，跨容器重建存活 |

- **載入**：啟動時 `config.ApplyCalibratedOverlayLayer` 把 overlay 疊在 SSOT 之上
  （`config.GetParametersConfig` / `ReloadParametersConfig` / `cmd/atlas/main.go`；
  `config.LoadParametersConfig` 仍是**純 SSOT** 讀取，供稽核／工具使用）。
- **可見性**：每個套用項輸出結構化 log `overlay_entry_applied`（同時帶 `ssot` 與 `effective` 與 `ratio`）；
  `ratio` 落在單輪窗 `[0.3x, 3x]` 之外 ⇒ 額外 WARN。被下限拒絕的提案走 CalibrationReport 的 `rejected[]`。
- **失效（fail-closed）**：overlay 條目記錄它疊在哪個 SSOT 值上；SSOT 值被改（人工 charter 編輯）
  ⇒ 該條目**失效並移除** ＋ WARN。人工審查的 charter 永遠優先於過期的 runtime 適應。
- **運維**：`jq . risk/…` 對照兩檔即可看出「實際生效值 vs repo 值」；要放棄校準回到出廠值，
  刪除 overlay 檔（或其中一條 `entries.<param>`）後重啟即可。

## 校準下限（sanity floor，防多輪漂移）

相對窗 `[current*0.3, current*3.0]` 是**每輪速率限制**、不是守門：每輪縮 ≤3× 永遠在窗內，
累積即可無限下行（生產實測：`risk_max_daily_loss_pct` 0.03 → 0.0108、`risk_max_position_size`
0.15 → 0.054，每一步都被「允許」）。因此每個參數另有**絕對下限**（`calibrationSanityFloor`）：

| 參數 | 下限 | 依據 |
|------|------|------|
| `risk_max_position_size` | 0.12 | repo 內已文件化的最保守持倉比例（`engine.strategy_evolution.configs.value.cautious.max_position_size`）；SSOT 0.15 仍可收緊一次 |
| `risk_max_daily_loss_pct` | 0.03 | SSOT 值本身（"3% max daily loss"）。更緊 ⇒ 更早停牌，而本迴圈的分數無法為「誤停牌」定價 ⇒ 收緊必須是受審查的 charter 編輯 |

低於下限一律拒絕；已在**下限之下**的既有值（舊版寫入的部署）走 recovery：接受 `[floor, floor*3]`
讓它一輪爬回 sane 值，不凍結在漂移值。要放寬／收緊這些界限＝編輯 `configs/parameters.json`（受審查的 diff）。
