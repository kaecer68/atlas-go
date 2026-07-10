# 前端 0 / 0.00 誤顯問題 — 稽核發現總表

> 稽核日期：2026-07-10  
> 分支：`fix/zero-metric-audit-20260710`  
> 方法：agent swarm 逐檔審核 + GitNexus / codebase-memory 追蹤後端對應位置

---

## 0. 稽核範圍

- `client_web/static/js/components/home-tier-sections.js`
- `shared_web/static/js/pages/*.js`（home, risk, performance-report, decision-chain, industry, pipeline, crossmarket, dashboard, narrative, metrics, datachannels）
- `shared_web/static/js/components/*.js`（sparkline, benchmark, risk-gate-panel, risk-panel, performance-report）
- `shared_web/static/js/shared/format-metric.js`、`shared_web/static/js/shared/utils.js`

---

## 1. 根因總結

| 根因 | 說明 | 影響檔案數 |
|------|------|-----------|
| **backend-sentinel** | Go 後端對可能缺失的數值使用 `float64`，zero value `0` 被序列化為有效數字；前端無法區分「真的 0」與「沒資料」。 | 8+ |
| **frontend-fallback** | 前端大量使用 `x \|\| 0`、`typeof x === 'number' ? x : 0`、truthy/falsy 判斷，把 `null/undefined` 與合法 `0` 混為一談。 | 16+ |
| **api-contract** | 前端讀取的欄位名稱或結構與後端回傳不一致，導致 `undefined` 被 fallback 成 0。 | 6+ |
| **architecture** | 缺乏統一的數值 formatter，各頁面各自發明 `toFixed` / `*100` / 正負號邏輯，格式不一致且容易出錯。 | 全部 |
| **source-bug** | 單位換算錯誤、正負號邏輯錯誤、陣列未過濾非數值等計算源頭問題。 | 4+ |

---

## 2. 高優先問題（會直接導致使用者看到錯誤數值）

### 2.1 home.js — 後端 sentinel 導致整頁 0/0.00%

- **前端**：`shared_web/static/js/pages/home.js:360-408`
- **後端**：`internal/monitoring/api/macro/handlers.go`、`internal/macro/*`
- **問題**：`/api/macro/snapshot/latest` 對未就緒指標回傳 zero-value `MacroDataPoint{"value":0,"change_pct":0,...}`，前端 `pointValue` / `pointChange` 視為有效 `0`。
- **根因**：backend-sentinel
- **修復方向**：後端改為 `*float64` + `omitempty`；前端 formatter 對 `null` 顯示 `—`。

### 2.2 home.js — 法人買賣超單位錯誤

- **前端**：`shared_web/static/js/pages/home.js:366,368,370`
- **問題**：把億元級絕對量除以 100 後用 `fmtSignedPct` 顯示，真實值 `45.2` 變 `+0.5%`，小值變 `0.0%`。
- **根因**：source-bug（單位換算錯誤）
- **修復方向**：直接使用絕對值顯示「45.2 億」，或後端提供真正的百分比欄位。

### 2.3 performance-report — cumulative 欄位缺失

- **前端**：`shared_web/static/js/components/performance-report.js:187`
- **後端**：`internal/reporting/performance.go:680-728`
- **問題**：`MonthlyReturn` 沒有 `cumulative` 欄位，前端永遠顯示 `0.00%`。
- **根因**：api-contract
- **修復方向**：後端新增 `cumulative` 欄位並計算累積報酬。

### 2.4 risk-gate-panel — API 結構錯配

- **前端**：`shared_web/static/js/components/risk-gate-panel.js:14-18`
- **後端**：`internal/monitoring/api/risk/handlers.go:44-134`
- **問題**：前端從 top-level 讀 `var_95` / `max_drawdown` / `position_count`，後端包在 `risk_snapshot` 下；`position_count` 根本不在此 endpoint。
- **根因**：api-contract
- **修復方向**：前端改讀 `data.risk_snapshot.*` 與 `data.gate_mode`；`position_count` 改接 `/api/dashboard/risk-exposure`。

### 2.5 benchmark.js — 後端 sentinel + 前端 fallback 疊加

- **前端**：`shared_web/static/js/components/benchmark.js`
- **後端**：`internal/monitoring/api/live/benchmark.go:20-147`
- **問題**：`sharpe_ratio`、`tracking_error`、`info_ratio` 樣本不足時回傳 0；前端 `|| 0` 疊加；`beta` 樣本不足回傳 1.0；雙正號 bug。
- **根因**：backend-sentinel + frontend-fallback + source-bug
- **修復方向**：後端改 `*float64` 回傳 `null`；前端統一使用 `shared/utils.js` formatter。

### 2.6 decision-chain — NaN% 與 core indicators 全 0

- **前端**：`shared_web/static/js/pages/decision-chain.js:121,128,214,244`
- **後端**：`internal/monitoring/api/decision/handlers.go:113-118,464-473`
- **問題**：confidence/hit_rate/pnl_pct 未防護直接 `*100`，可能顯示 `NaN%`；`snap==nil` 時 core indicators 全 0。
- **根因**：backend-sentinel + source-bug
- **修復方向**：後端 `*float64`；前端使用統一 formatter。

### 2.7 home-tier-sections.js — API 契約完全錯誤

- **前端**：`client_web/static/js/components/home-tier-sections.js:283-358`
- **後端**：`internal/capitalflow/*`、`internal/dailyreport/*`
- **問題**：`/api/capital-flow/summary` 無 `forces` 欄位，導致資金流向區塊完全隱藏；`/api/reports/latest` 無 `summary` 欄位，永遠顯示「尚無報告」；`p.confidence` 未防護導致 `NaN%`。
- **根因**：api-contract + frontend-fallback
- **修復方向**：對齊後端欄位名稱；前端對缺失值顯示 `—`。

### 2.8 dashboard.js — 大量 fallback 與函式未呼叫 bug

- **前端**：`shared_web/static/js/pages/dashboard.js`
- **問題**：18+ 處 `|| 0`；行 496 `${regimeLabel}` 未呼叫導致畫面印出函式原始碼。
- **根因**：frontend-fallback + source-bug
- **修復方向**：統一 formatter；修復 regimeLabel 呼叫。

### 2.9 narrative.js — 信心分數與季節性回報 NaN%

- **前端**：`shared_web/static/js/pages/narrative.js`
- **問題**：大量 `|| 0`、`e.confidence * 100` 無防護；季節性回報缺失時 `NaN%`。
- **根因**：frontend-fallback + source-bug
- **修復方向**：統一 formatter；後端對缺失季節性資料回傳 `null`。

### 2.10 metrics.js — 趨勢圖 NaN 傳播

- **前端**：`shared_web/static/js/pages/metrics.js:140-224`
- **問題**：趨勢資料點 `value` 缺失時 `values` 出現 `NaN`，導致 `Math.min/max`、Y 軸標籤、hover tooltip 全部無效。
- **根因**：api-contract + frontend-fallback
- **修復方向**：過濾或補 `null` 資料點；後端對缺失點回傳 `null`。

---

## 3. 中低優先問題（會影響正確性但較少出現）

| 檔案 | 問題 | 分類 |
|------|------|------|
| `industry.js` | 大量 `\|\| 0`、`\|\| 0.5`、`\|\| 1.0`；`edge.correlation` 真實 0 被當 0.5；`adjustment_factor` 真實 0 被當 1.0。 | frontend-fallback |
| `pipeline.js` | `forward_return` 缺失顯示 `NaN%`；價格欄位 truthy 判斷把 0 當缺失。 | frontend-fallback |
| `crossmarket.js` | `changePct` 缺失顯示下跌色；`generated_at` 欄位名稱錯誤。 | api-contract + frontend-fallback |
| `datachannels.js` | `ind.value` 後端 0 sentinel 顯示 `0.000`；`change_pct` 單位假設不明。 | backend-sentinel + api-contract |
| `sparkline.js` | `rolling_sharpe \|\| 0`、`signal_count \|\| 0`；陣列 undefined 導致 NaN 座標。 | frontend-fallback + source-bug |
| `risk-panel.js` | `current_drawdown`、`concentration_ratio`、`positions_count` 缺失顯示 0；leverage 計算可能 Infinity/NaN。 | frontend-fallback + source-bug |
| `format-metric.js` | 守衛僅擋 `null/undefined/NaN`，未驗證 `typeof === 'number'`。 | architecture |
| `utils.js` | 所有 formatter 把 0 視為有效；`fmtPct` 單位假設為 ratio；`pnlColor(0)` 顯示獲利色。 | architecture |

---

## 4. 後端需修改的關鍵位置

| 檔案 | 問題 | 建議修改 |
|------|------|---------|
| `internal/monitoring/api/live/benchmark.go:20-147` | `float64` zero value 當缺失 | `BenchmarkComparisonResponse` 與計算函式改 `*float64` |
| `internal/reporting/performance.go:680-728` | 無 `cumulative` 欄位 | `MonthlyReturn` 新增 `Cumulative *float64` |
| `internal/monitoring/api/decision/handlers.go:113-118,464-473` | `CoreIndicators` 全 `float64` | 改 `*float64`；`snap==nil` 時回傳 null 或全 null 欄位 |
| `internal/monitoring/api/risk/handlers.go:44-134` | `risk_snapshot` 包裝 | 確認前端合約；必要時提升欄位到 top-level |
| `internal/monitoring/api/macro/handlers.go:101-107` | `MacroDataPoint` zero value | 改 `*float64` + `omitempty` |
| `internal/marketdata/macro_provider.go:20-58` | `MacroDataSnapshot` 對缺失指標仍序列化 zero-value 物件 | 新增 `MarshalJSON`：當 `MacroDataPoint.Symbol == ""` 時省略該欄位 |
| `internal/monitoring/api/system/handlers.go:168-181` | retail sentiment 無 snapshot 時全 0 | ✅ 已改：`RetailSentimentResponse` 數值欄位改 `*float64`，缺失回傳 `nil` |
| `internal/monitoring/api/narrative/handlers.go:142-150` | `getPatternReturn` 失敗回傳 0 | 改回傳 `null`（frontend `seasonality-panel.js` 已先以 `—` 防護） |
| `internal/capitalflow/*` | summary vs daily 欄位不一致 | 確認 `/api/capital-flow/summary` 是否應回傳 `forces` |
| `internal/dailyreport/*` | `Report` 結構無頂層 `summary` | 前端改讀 `global.summary` 或後端新增 alias |

---

## 5. 前端需修改的關鍵位置

| 檔案 | 主要修改 |
|------|---------|
| `shared_web/static/js/shared/utils.js` | 強化 formatter：統一缺失值顯示 `—`、處理 `NaN/Infinity`、釐清 `fmtPct` ratio/percentage 合約 |
| `shared_web/static/js/shared/format-metric.js` | 加入 `typeof === 'number'` 與 `Number.isFinite` 守衛 |
| `shared_web/static/js/pages/home.js` | 移除 `\|\| 0`、修正法人買賣超單位、使用統一 formatter |
| `shared_web/static/js/pages/risk.js` | 移除 `\|\| 0`、金額改用 `!= null` 判斷 |
| `shared_web/static/js/pages/performance-report.js` + `components/performance-report.js` | 使用統一 formatter、移除 inline fallback |
| `shared_web/static/js/pages/decision-chain.js` | confidence/hit_rate/pnl_pct 使用統一 formatter |
| `shared_web/static/js/pages/industry.js` | 大量 `\|\| 0`/`\|\| 0.5`/`\|\| 1.0` 改明確 null 判斷 |
| `shared_web/static/js/pages/pipeline.js` | `forward_return` 缺失防護、價格權改用 `!= null` |
| `shared_web/static/js/pages/crossmarket.js` | 修正 `generated_at` → `computed_at`、加 `isNaN` 檢查 |
| `shared_web/static/js/pages/dashboard.js` | 統一 formatter、修復 regimeLabel 呼叫 |
| `shared_web/static/js/pages/narrative.js` | 統一 formatter、季節性回報缺失防護 |
| `shared_web/static/js/pages/metrics.js` | 趨勢圖過濾/補 null 資料點 |
| `shared_web/static/js/components/risk-gate-panel.js` | 對齊 `/api/dashboard/risk` 結構 |
| `shared_web/static/js/components/benchmark.js` | 使用 `shared/utils.js` formatter、修復雙正號 |
| `shared_web/static/js/components/sparkline.js` | 移除 `\|\| 0`、陣列過濾非數值 |
| `shared_web/static/js/components/risk-panel.js` | 移除 `\|\| 0`、leverage 計算防護 |
| `client_web/static/js/components/home-tier-sections.js` | 對齊 summary/daily report 結構、修正 `NaN%` |
| `shared_web/static/js/pages/evolution_panel.js` | Sharpe / 命中率 / 最大回撤 / 實驗 delta 改用統一 formatter，缺失顯示 `—` |
| `shared_web/static/js/pages/portfolio.js` | KPI、持倉、交易歷史改用 `isValidNumber` 判斷，缺失顯示 `—` |
| `shared_web/static/js/pages/strategies.js` | 平均命中率、核心指標改用 `isValidNumber`，缺失顯示 `—` |
| `shared_web/static/js/components/stock-quote-technical.js` | SMA / RSI 缺失顯示 `—`，訊號改為「無訊號」 |
| `shared_web/static/js/components/stock-quote-chips.js` | 三大法人買賣超缺失顯示 `—`，不渲染 0 長度 bar |
| `shared_web/static/js/components/attribution.js` | agent / sector / factor 平均報酬改用統一 formatter |
| `shared_web/static/js/shared/components/seasonality-panel.js` | 歷史準確度 / 典型報酬 / 調整因子缺失顯示 `—` |

---

## 6. 驗證清單

| 項目 | 狀態 | 備註 |
|------|------|------|
| 後端修改的 API 回傳 `null` 而非 `0` 表示缺失。 | ✅ | `benchmark`、`decision`、`performance`、`MacroDataSnapshot`、`retail-sentiment` 已改為 nullable / 省略缺失欄位。 |
| 前端對 `null/undefined/NaN` 顯示 `—`，對真實 `0` 顯示 `0`/`0.00%`。 | ✅ | `utils.js`、`format-metric.js` 統一以 `Number.isFinite` 守衛；各頁面已移除 display path 的 `\|\| 0`。 |
| `/api/dashboard/performance-report` 的 `monthly_returns[].cumulative` 有值。 | ✅ | `internal/reporting/performance.go` 新增 `Cumulative *float64` 並計算。 |
| `/api/dashboard/risk` 與 risk-gate-panel 欄位對齊。 | ✅ | risk-gate-panel 改讀 `data.risk_snapshot.*` 與 `data.gate_mode`。 |
| `/api/capital-flow/summary` 與 home-tier-sections 欄位對齊。 | ✅ | `SummaryReport` 新增 `Forces []ForceScore`；daily report 新增頂層 `Summary`。 |
| `make ci` 與 `make ci-slow` 通過。 | ✅ | 兩者皆全綠。 |
| 前端 build 通過。 | ✅ | `client_web` 與 `admin_web` 的 `npm run build` 皆成功。 |
| 前端單元測試通過。 | ✅ | `client_web/npm test`：102/102 pass。 |
| Go 單元測試通過。 | ✅ | `go test ./internal/marketdata/... ./internal/monitoring/service/... ./internal/reporting/... ./internal/monitoring/api/live/... ./internal/monitoring/api/decision/... ./internal/capitalflow/... ./internal/dailyreport/...` 全過。 |
| smoke test（admin_web）通過。 | ✅ | 10/10 pass，console error 皆為已知 401 allowlist。 |
| smoke test（client_web）| ⚠️ | `home` page 因本機運行中的 server 仍是舊 binary（`#page-overview`），導致 `waitForSelector('#page-home.active')` timeout；其餘 10 頁皆通過且無 NaN/undefined/null。部署新 server 後應可恢復。 |

---

## 7. 已知限制與後續行動

1. **Server 必須重啟**：本機 Docker server 目前執行的是合併前 binary，未載入本次前端與後端修正；部署後需重新 build image 並重啟容器，client_web `home` smoke 才能通過。
2. **監控首頁指標**：建議部署後再次以瀏覽器確認 `home` 頁的「外資 / 加權 / 損益 / 最大回撤」等欄位在資料缺失時顯示 `—`，而非 `0.0%` / `0.00`。
3. **長期架構**：本 PR 以「後端 nullable + 前端統一 formatter」修復現有欄位；未來新增數值欄位時應沿用 `*float64` + `formatNumber`/`fmtSignedPct` 模式，避免再次出現 sentinel 0。
