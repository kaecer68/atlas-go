# AGENTS.md — internal/industry

本目錄負責產業生態系分析，包含三個核心板塊：供應鏈連動（Supply Chain Linkage）、季節性模式（Seasonal Patterns）、週期羅盤（Cycle Compass）。

---

## OVERVIEW

`internal/industry` 提供：供應鏈圖（含相關係數矩陣，支援敘事感知動態調整）、季節性引擎（各產業歷史月曆效應偵測與校準）、商業週期偵測（expansion/recovery/mature/recession 階段判定）、衝擊傳導、產業分類樹、半導體週期（`SiliconCycleTracker` 追蹤矽循環對台股產業影響）、ODM 通道（`ODMChannel` 監測出貨與傳導）、數據聚合器（`DataAggregator`）、週期狀態卡（`CycleStatusCardBuilder` 供前端 Pipeline Health 頁面）。

---

## 核心檔案

| 檔案 | 職責 |
|------|------|
| `linkage.go` | 供應鏈圖 / 相關矩陣 / 衝擊傳導 / 連結分析器（`LinkageAnalyzer`） |
| `seasonality.go` | 季節性引擎（`SeasonalEngine`）/ 模式偵測 / 調整係數 / 四層分解 |
| `seasonal_calibrator.go` | 模式校準（`CalibratePatterns`）/ 產業報酬聚合器 |
| `cycle.go` | 商業週期追蹤器 / 階段評分 / 信賴度計算 |
| `cycle_status_card.go` | 五層複合週期狀態卡（矽循環 + 商業週期 + 季節性 + 事件 + 供應鏈） |
| `silicon_cycle.go` | 矽循環追蹤器 / 相位偵測 / 指標快照 |
| `dynamic_env.go` | 動態環境調變器（將宏觀數據納入週期評分） |
| `risk.go` | 產業風險監控（客戶集中度、新聞延遲、不對稱風險） |
| `event_calendar.go` | 台股日曆事件偵測 / 情緒乘數 |
| `types.go` / `classification.go` | 產業分類樹結構 |

---

## 資料流

`Config (parameters.json) → LinkageAnalyzer / ShockPropagation / SeasonalEngine; Narrative Events (internal/narrative) → SeasonalBridge.CorrelationMultiplier() → ShockPropagation; Supply Chain Graph → LoadSupplyChainGraph(); Replay Data (cmd/calibrate-seasonal --replay) → CalibratePatterns() → updateParametersFile()`

---

## 關鍵 API（`monitoring/api/industry/handlers.go`）

| 端點 | 回傳 |
|------|------|
| `GET /api/dashboard/industry-classification` | 產業分類樹（`industries` + `count`） |
| `GET /api/dashboard/industry-overview` | 每產業 cycle + linkage + risk 總覽（含 `adjusted_weight`） |
| `GET /api/dashboard/industry-seasonality` | 季節性模式列表（含 `GetAdjustmentBreakdown`） |
| `GET /api/dashboard/industry-seasonality-calendar` | 年度季節性行事曆（12 個月） |
| `GET /api/dashboard/industry-graph` | 供應鏈圖（nodes + edges + 相關矩陣） |
| `GET /api/dashboard/cycle-status-card` | 五層複合週期狀態卡（含 `breakdown`、`silicon_phase`、`composite_coefficient`） |
| `GET /api/dashboard/industry-detail` | 單一產業完整資訊（連動 + 季節 + 週期 + 風險） |

> ⚠️ **歷史路徑**：本表為 `/api/dashboard/industry-*` 新路徑，舊 `/api/industry/*` 已棄用。前端 `web/static/js/pages/industry.js` 的 `loadIndustryData()` 在 `Promise.all` 中呼叫新路徑。

---

## Experimental Files

以下檔案**尚未進入穩定層**——目前無生產端點依賴、也無 `core/` 模組 import，僅作為隔離測試或 Stage 3 評估中保留：

| 檔案 | 職責 | 狀態 |
|------|------|------|
| `odm_channel.go` | `ODMChannel` 出貨通道監測、CoWoS 產能追蹤、`ODMTransmissionModel` | 待評估，僅 dashboard_api smoke test 引用 |
| `data_aggregator.go` | `DataAggregator` 從 FinMind 拉個股財務數據聚合 | 待評估，僅 `GetDataAggregatorSummary` 內部呼叫 |
| `seasonal_health.go` | 季節性校準健康狀態摘要 | 待評估，僅 `GetSeasonalHealth` 內部呼叫 |
| `correlation_loader.go` | 相關矩陣載入器元資料 | 待評估，僅 `GetCorrelationLoaderMetadata` 內部呼叫 |

> **處理原則**：Stage 3 之前不得修改這些檔案的邏輯。`handlers.go` 內已有對應的 `Handle*` 路由（覆蓋率 75–100%），但**目前未在 `RegisterRoutes` 註冊**——這是預期狀態而非 dead code。

---

## 陷阱與注意事項

| 陷阱 | 說明 |
|------|------|
| **financials 純上游資本供給** | `configs/supply_chain_graph.json` 中 financials 已連接 6 個 `upstream_of` 節點，`downstream_of` 為空；`CalculateLinkageScore` 只反映單向度數，系統重要性約 `(0 + 6) / max(MaxDegree, 10) = 0.6`，並非 0。 |
| **雙向邊不一致** | 部分節點（如 `foundry↔ai_supply_chain`、`server_assembly↔semiconductor`）upstream/downstream 邊不對稱（預期行爲，表達單向依賴），但 `CalculateLinkageScore` 和 `PropagateShock` 假設雙向索引 |
| **敘事感知需求** | `ShockPropagation` 若未呼叫 `SetNarrativeProvider()`，相關調整會回退到純相關矩陣查詢 |
| **衰減因子回退** | `PropagateShock` 在 `downstreamDecay`/`upstreamDecay` 為 0 時使用硬編碼 0.8/0.6；`NewLinkageAnalyzer()` 會從 config 設定，但 `NewShockPropagation()` 不會 |
| **校準 CLI** | `cmd/calibrate-seasonal` 的 `--replay` 與 `--update` 旗標可自動寫回 `configs/parameters.json` |
| **ETF 是金融工具而非經濟產業** | etf_rotation 在供應鏈圖譜中僅 `downstream_of: financials`。ETF 不生產實體商品，`PropagateShock` 對 etf_rotation 作為來源節點的衰減應為 0 |
