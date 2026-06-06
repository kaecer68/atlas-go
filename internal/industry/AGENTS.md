# AGENTS.md — internal/industry

本目錄負責產業生態系分析，包含三個核心板塊：供應鏈連動（Supply Chain Linkage）、季節性模式（Seasonal Patterns）、週期羅盤（Cycle Compass）。

---

## OVERVIEW

`internal/industry` 提供：
- **供應鏈圖**：產業間的上下游關係與相關係數矩陣，支援敘事感知動態調整
- **季節性引擎**：各產業的歷史月曆效應偵測與校準
- **商業週期偵測**：產業生命週期階段（expansion/recovery/mature/recession）判定
- **衝擊傳導**：從來源產業出發的上下游衝擊影響計算
- **產業分類樹**：多層次產業分類結構

---

## 核心檔案

| 檔案 | 職責 |
|------|------|
| `linkage.go` | 供應鏈圖（`SupplyChainGraph`）、相關矩陣（`CorrelationMatrix`）、衝擊傳導（`ShockPropagation`）、產業連結分析器（`LinkageAnalyzer`）及 `DefaultSupplyChainGraph()` / `DefaultCorrelationMatrix()` / `LoadSupplyChainGraph()` |
| `seasonality.go` | 季節性引擎（`SeasonalEngine`）、模式偵測（`DetectCurrentPatterns`）、調整係數計算（`GetPatternAdjustment`）、四層分解（`GetAdjustmentBreakdown`） |
| `seasonal_calibrator.go` | 季節性模式校準（`CalibratePatterns`）、產業報酬聚合器（`IndustryReturnAggregator`） |
| `cycle.go` | 商業週期追蹤器（`CycleTracker`）、階段評分（`GetPhaseScore`）、信賴度計算 |
| `dynamic_env.go` | 動態環境調變器（`DynamicEnvModulator`）、將宏觀數據納入週期評分 |
| `risk.go` | 產業風險監控（客戶集中度、新聞延遲、不對稱風險） |
| `types.go` / `classification.go` | 產業分類樹結構 |

---

## 資料流

```
Config (parameters.json)
  ├─ linkage_params.correlation_matrix ──→ LinkageAnalyzer (via LoadCorrelationMatrixFromConfig)
  ├─ linkage_params.decay_factors ──────→ ShockPropagation (via SetDecayFactors)
  ├─ linkage_params.systemic_import_divisor ──→ CalculateLinkageScore (via config floor)
  └─ seasonal_patterns ────────────────→ SeasonalEngine

Narrative Events (internal/narrative)
  └─ SeasonalBridge.CorrelationMultiplier() ──→ ShockPropagation.getNarrativeAdjustedCorrelation()

Supply Chain Graph (configs/supply_chain_graph.json)
  └─ LoadSupplyChainGraph() ──→ SupplyChainGraph + CorrelationMatrix

Replay Data (cmd/calibrate-seasonal --replay)
  └─ CalibratePatterns() ──→ SeasonalCalibration results ──→ updateParametersFile()
```

---

## 關鍵 API

| 端點 | Handler 檔案 | 回傳內容 |
|------|-------------|---------|
| `GET /api/industry/linkage` | `monitoring/api/industry/handlers.go` | 供應鏈圖 + 相關矩陣 + 連動分數 |
| `GET /api/industry/seasonality` | 同上 | 季節性模式列表（含 `GetAdjustmentBreakdown`）|
| `GET /api/industry/seasonality/calendar` | 同上 | 年度季節性行事曆 |
| `GET /api/industry/cycles` | 同上 | 各產業週期位置與趨勢 |
| `GET /api/industry/classification` | 同上 | 產業分類樹 |
| `GET /api/industry/detail` | 同上 | 單一產業完整資訊（連動 + 季節 + 週期 + 風險） |

---

## 陷阱與注意事項

| 陷阱 | 說明 |
|------|------|
| **financials 純上游資本供給** | `configs/supply_chain_graph.json` 中 financials 目前已連接 6 個 `upstream_of` 節點（`semiconductor`、`ai_supply_chain`、`electronics`、`robotics`、`shipping`、`energy`），且 `downstream_of` 為空；`CalculateLinkageScore` 只反映單向度數，系統重要性約為 `(0 + 6) / max(MaxDegree, 10) = 0.6`，並非 0。若要提高分數，可擴大 `upstream_of` 涵蓋的資本密集產業，或調整 `systemic_importance_divisor` 預設值（10.0）。 |
| **雙向邊不一致** | 圖中部分節點（如 `foundry↔ai_supply_chain`、`server_assembly↔semiconductor`）的 upstream/downstream 邊不對稱，這是預期行爲（表達單向依賴），但 `CalculateLinkageScore` 和 `PropagateShock` 假設雙向索引 |
| **敘事感知需求** | `ShockPropagation` 若未呼叫 `SetNarrativeProvider()`，相關調整會回退到純相關矩陣查詢 |
| **衰減因子回退** | `PropagateShock` 在 `downstreamDecay`/`upstreamDecay` 為 0 時使用硬編碼的 0.8/0.6。`NewLinkageAnalyzer()` 會從 config 設定，但若直接使用 `NewShockPropagation()` 則不會 |
| **校準 CLI** | `cmd/calibrate-seasonal` 的 `--replay` 與 `--update` 旗標可自動寫回 `configs/parameters.json`，須確保該檔案正確 |
| **ETF 是金融工具而非經濟產業** | etf_rotation 在供應鏈圖譜中僅 downstream_of: financials。ETF 不生產實體商品，衝擊傳導僅反應資本流動。`PropagateShock` 對 etf_rotation 作為來源節點的衰減應為 0（不傳導至其他產業）。|
