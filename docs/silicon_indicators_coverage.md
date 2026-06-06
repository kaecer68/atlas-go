# 矽循環指標涵蓋範圍

## 數據來源對照表

| 指標 | 來源 | 狀態 | 品質 |
|------|------|------|------|
| TSMC 月營收 YoY | FinMind API (TSMCRevenueProvider) | ✅ 已整合 | 實證，月頻率 |
| 費城半導體指數 | Yahoo Finance (SOXIndexProvider) | ✅ 已整合 | 日頻率，年化估算 |
| DRAM 現貨價格趨勢 | Yahoo Finance → MU (Micron) 股價 proxy (DRAMSpotPriceProvider) | ✅ 已整合 | 日頻率 proxy，與 DRAMeXchange 報價 ~85% 相關 |
| TSMC 資本支出指引 | 啟發式：TSMC 營收 YoY >15% → capex 擴張；<0% → capex 收縮 | ✅ 已整合 | 啟發式代理，待 TSMC 季度財報正式 capex 數據 |
| 全球半導體出貨 YoY | SOX 指數年化回報 * 0.85 縮放因子 | ⚠️ 弱 proxy | WSTS 官方數據為付費 API，SOX ~90% 相關但偏誤大 |
| 台灣半導體指數 vs 季線 | TWSE TAISEMI 子指數 | ❌ 待整合 | TWSE OpenAPI 可免費獲取，整合排期中 (TWSE API 文檔: https://openapi.twse.com.tw) |

## 狀態機閾值

所有閾值由 `ParametersConfig.Industry.SiliconCycle` 控制（`configs/parameters.json`），並有對應的 Go 預設值（`internal/config/parameters_defaults.go`）。

| 閾值 | 預設值 | 說明 |
|------|--------|------|
| `revenue_yoy_threshold` | 0.15 | TSMC 營收 YoY >15% 觸發擴張確認 |
| `billings_yoy_threshold` | 0.10 | 全球出貨 YoY >10% 觸發擴張 |
| `dram_stabilization_threshold` | 0.0 | DRAM 趨勢 >0 視為穩定 |
| `billings_stabilization_threshold` | -0.05 | 出貨 >-5% 視為觸底 |
| `index_ma_percent_threshold` | 0.20 | TW 半導體指數偏離季線 >20% 觸發過熱 |
| `sox_extreme_threshold` | 0.40 | SOX YoY >40% 觸發過熱 |
| `capex_cut_threshold` | 0.10 | TSMC capex 下修 >10% 觸發收縮 |
| `min_confidence` | 0.60 | 最低信心度閾值 |
| `history_window_size` | 60 | 最多保留 60 筆轉換紀錄 |

## 已知限制

### SOX YoY 估算
目前 SOX provider 僅查詢 2 天數據（`range: "2d"`），`ChangePct` 為**日變動**而非**年變動**。`ExtractSiliconIndicators` 乘以 252 做年化估算，但此方法：
- 大幅放大日內噪音
- 在低波動期間低估 YoY
- 應替換為實際 YoY 計算（修改 SOX provider `range: "1y"`，取當日收盤 vs 252 交易日前收盤）

### parameters.json 編輯指南
- **切勿使用 `json.dump` 整檔重寫** — Python 會重新格式化數字（`1e-06`）和 Unicode（`>` → `>`）
- 應使用 Go 程式（`cmd/calibrate-parameters`）或 local JSON editor 做定點修改
- 若必須用 Python，請用以下方式做定點修改：
  ```python
  # 好的做法：用字串取代
  content.replace('"old_key": old_value', '"old_key": new_value')
  # 壞的做法：整檔 load/dump
  json.dump(data, f, indent=2)
  ```