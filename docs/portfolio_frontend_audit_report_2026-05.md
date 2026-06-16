# 前端頁面盤查報告：組合持倉、績效報告、AI 觀測台

**日期**: 2026-05-21  
**分支**: `feat/portfolio-frontend-audit-2026-05`  
**盤查範圍**: `web/static/` (前端), `internal/monitoring/` (後端 API), `internal/reporting/` (報告引擎), `internal/live/` (實盤狀態)

---

## 執行摘要

本次盤查針對三個前端頁面（組合持倉、績效報告、AI 觀測台）的前後端功能、計算邏輯與架構設計進行全面審計。整體而言，**資料流清晰、架構分層合理**，但發現了 **6 個需修復的問題**（2 高、2 中、2 低）以及 **3 個建議增強的項目**。

核心發現：**Sharpe ratio 計算方式正確**（符合金融工程標準），**但 PnL 歸因存在嚴重缺陷**——累積報酬率以現金為分母而非組合總值，導致槓桿/虧損情境下數字失真。

---

## 1. 架構總覽

### 1.1 前端頁面 → 後端 API 映射

| 前端頁面 | 前端檔案 | 後端 API | Handler |
|---------|---------|---------|---------|
| 組合持倉 | `web/static/js/pages/portfolio.js` | `/api/dashboard/portfolio-state` | `internal/monitoring/api/live/handlers.go:HandlePortfolioState` |
| | | `/api/dashboard/live-status` | `internal/monitoring/api/live/handlers.go:HandleLiveStatus` |
| | | `/api/dashboard/trade-history` | `internal/monitoring/api/live/handlers.go:HandleTradeHistory` |
| | | `/api/dashboard/tax-snapshot` | `internal/monitoring/api/tax/handlers.go` |
| 績效報告 | `web/static/js/components/performance-report.js` | `/api/dashboard/performance-report?period=` | `internal/monitoring/api/performance/handlers.go:HandleReport` |
| AI 觀測台 | `web/static/js/pages/dashboard.js` | `/api/dashboard/agent-observatory` | `internal/monitoring/api/pipeline/handlers.go:HandleAgentObservatory` |

### 1.2 核心資料流

```
Session Summaries (JSONL)
    ↓
LiveService.LoadPortfolioState()
    ├── livestore.LoadLastPortfolioState()  → 現金、部位市值
    ├── livestore.LoadLastPositions()       → 個股明細
    ├── sim.LoadPersistentState()           → 已實現損益
    └── buildEquityCurve()                  → 權益曲線
    ↓
/api/dashboard/portfolio-state  →  前端 portfolio.js 渲染 KPI + 持倉表

Session Summaries → RecommendationOutcomes (JSONL)
    ↓
reporting.GenerateReport()
    ├── calculateSharpeRatio()              → 年化夏普值
    ├── risk.CalculateMaxDrawdown()         → 最大回撤
    ├── calculateTradeMetrics()             → 勝率/平均盈虧
    ├── calculateTopAgents()               → Top-5 AI 貢獻
    ├── calculateRegimeBreakdown()          → 市場狀態績效
    └── calculateMonthlyReturns()           → 月度報酬
    ↓
/api/dashboard/performance-report  →  前端 performance-report.js 渲染

Outcomes → ledger.BuildScorecards()
    ↓
/api/dashboard/agent-observatory  →  前端 dashboard.js AI 觀測台卡片
```

---

## 2. 金融工程標準檢驗

### 2.1 Sharpe Ratio 計算 ✅ 正確

**檔案**: `internal/reporting/performance.go:353-375`

```go
func calculateSharpeRatio(dailyReturns []float64) float64 {
    mean := sum(returns) / N
    variance := sum((r - mean)^2) / N    // 母體變異數
    stdDev := sqrt(variance)
    return (mean / stdDev) * sqrt(252)   // 年化
}
```

**評估**: 使用母體變異數（除以 N 而非 N-1）在足夠樣本數下是可接受的簡化。年化因子 `sqrt(252)` 符合業界標準（台灣股市交易日約 250-252 天）。正確處理 `stdDev == 0` 的邊界情況。

**建議**: 考慮使用樣本標準差（除以 N-1）以獲得無偏估計，特別是在樣本數較少時。此為低優先級改善。

### 2.2 Max Drawdown 計算 ✅ 正確

**檔案**: `internal/risk/var_calculator.go` (由 `reporting.GenerateReport` 呼叫)

使用 `risk.CalculateMaxDrawdown(portfolioValues)` — 委派給獨立的風控模組，架構合理。

### 2.3 年化報酬率計算 ✅ 正確

**檔案**: `internal/reporting/performance.go:129-132`

```go
annualizedReturn = math.Pow(1 + totalReturn, 365.0/days) - 1
```

使用複利年化公式，正確處理 `totalReturn > -1` 的邊界。

### 2.4 月份報酬率計算 ⚠️ 有缺陷

**檔案**: `internal/reporting/performance.go:568-617`

```go
// 使用當月第一個和最後一個 portfolio value 計算報酬
startVal := values[0]
endVal := values[len(values)-1]
ret := (endVal - startVal) / startVal
```

**問題**: 僅使用當月第一個和最後一個觀測值計算報酬。如果月中發生大幅波動（先跌後漲），報酬率會正確反映淨變化，但不會捕捉到**最大回撤**或**波動率**。此外，如果當月只有 1 筆資料（`len(values) < 2`），該月份會被完全跳過。

**嚴重性**: 低。模擬系統的 session 頻率通常為每日一次，因此月份內通常有 20-22 筆資料。使用首尾值計算月報酬在實務上是常見做法。

### 2.5 勝率計算 ⚠️ 可能誤導

**檔案**: `internal/reporting/performance.go:377-412`

```go
if oc.ForwardReturn > 0 { wins++ }
```

**問題**: 勝率僅以 `ForwardReturn > 0` 判斷，未與**無風險利率**或**基準報酬**比較。即使 ForwardReturn = +0.001%（幾乎為零），也算作「贏」。

**嚴重性**: 低。對於模擬系統而言，ForwardReturn 代表推薦的實際 market impact，以此為基準是合理的。但在正式投資績效報告中，應標註「此勝率以零為基準」。

### 2.6 Sharpe-like（Agent）計算 ⚠️ 非標準 Sharpe

> **狀態更新（2026-06-16）**：`calculateSharpeLike` 已於 commit `a661daa8` 移除（PR #558）。
> 現行為：KPI（`PerformanceReport.SharpeRatio`）呼叫 `CalculateSharpeRatio(dailyReturns)` → `portfolio.ComputeSharpe(..., MinSamples: 2)`；
> Per-agent（`AgentContribution.SharpeLike`）呼叫 `portfolio.ComputeSharpe(..., MinSamples: 5)`，
> 樣本不足時回傳 `nil`（前端顯示 N/A）。
> 本節保留以說明歷史脈絡。

**檔案**: `internal/reporting/performance.go`（移除後的 `calculateSharpeLike` 對應呼叫）

```go
sharpeLike := portfolio.ComputeSharpe(returns, portfolio.SharpeConfig{
    Frequency:  portfolio.FrequencyPerOutcome,
    MinSamples: 5,
})
```

**問題（原）**: 舊版 `calculateSharpeLike` 計算 `mean / variance`（變異數倒數），而非標準 Sharpe ratio（`mean / stdDev * sqrt(N)`）。雖然前端標籤為「Sharpe-like」而非「Sharpe」，但計算方式與傳統 Sharpe 有本質差異——前者對波動率的懲罰是平方關係，後者是線性關係。

**比較**:
- 標準 Sharpe: `mean / stdDev` → 報酬每單位風險
- Sharpe-like: `mean / variance` → 對高波動策略懲罰更重

**嚴重性**: 低。前端已標註「Sharpe-like」，與真正的 Sharpe ratio 區分開來。

---

## 3. 發現的問題

### 🔴 HIGH #1: 累積報酬率分母使用現金而非組合總值

**檔案**: `internal/monitoring/service/live.go:196-198`

```go
if portfolio.Cash > 0 {
    resp.CumulativePnLPct = resp.CumulativePnL / portfolio.Cash
}
```

**問題**: 組合持倉頁面的「累積報酬率」以**現金**為分母，而非**組合總值（現金 + 持倉市值）**。這在以下情境會產生嚴重失真：

1. **高持股情境**：若組合 80% 持股、20% 現金，累積 PnL 是組合總盈虧，但只除以 20% 的現金 → 報酬率被放大 5 倍
2. **現金為零或極少**：分母接近零時報酬率趨近無限大
3. **虧損情境**：若組合虧損但仍有現金，報酬率會被低估（因為分母過大）

**金融工程標準**: 投資組合報酬率應以**期初組合總值**為分母（Modified Dietz 方法），或至少以**組合總市值**為參考基準。

**建議**: 
- `CumulativePnLPct` 應改為 `resp.CumulativePnL / (startingCash 或 initial_portfolio_value)`
- 或在 `PortfolioStateResponse` 中增加 `cumulative_return_pct` 欄位，以起始本金為分母

### 🔴 HIGH #2: 組合持倉頁面缺少關鍵風險指標

**檔案**: `web/static/js/pages/portfolio.js` 與 `internal/monitoring/service/live.go`

**問題**: 組合持倉 KPI 區塊僅顯示：稅前淨值、稅後淨值、已實現損益、累積交易數、累積稅負。缺少以下關鍵風險指標：

| 缺少的指標 | 金融工程必要性 |
|-----------|-------------|
| 持倉集中度 (Concentration Ratio) | 識別過度集中的風險 |
| 產業曝險分布 (Sector Exposure) | 理解系統性風險來源 |
| 最大回撤 (Max Drawdown) | 最直觀的風險指標 |
| 未實現損益 (Unrealized PnL) | 目前僅在持倉表格中顯示，KPI 區塊缺乏彙總 |
| 當日損益 (Daily PnL) | 即時風險監控 |

**建議**: 
- 在 KPI 卡片區塊增加「未實現損益」、「最大回撤」、「持倉集中度」三個指標
- 產業曝險可從 `recommendation_outcomes.jsonl` 的 `FactorScores` 計算

### 🟡 MEDIUM #3: 績效報告缺少 Sortino/Calmar/Information Ratio

**檔案**: `internal/reporting/performance.go`

**問題**: `PerformanceReport` 僅包含 Sharpe ratio 和 Max Drawdown，缺少以下標準金融指標：

| 缺少的指標 | 計算方式 | 用途 |
|-----------|---------|------|
| Sortino Ratio | 下行標準差取代總標準差 | 更精確的風險調整報酬 |
| Calmar Ratio | 年化報酬 / 最大回撤 | 衡量回撤後的報酬效率 |
| Information Ratio | 超額報酬 / 追蹤誤差 | 衡量主動管理能力 |
| Profit Factor | 總獲利 / |總虧損| | 交易系統品質 |

**建議**: 
- 在 `internal/reporting/performance.go` 增加 `calculateSortinoRatio()`, `calculateCalmarRatio()` 函數
  - ✅ 已實作（May 2026）
  - 🔄 後續演化（Jun 2026, PR #452, commit `79499c92`）：`calculateSortinoRatio()` 改為 thin wrapper 呼叫 `internal/domain/shared/sortino.go:ComputeSortino`，因 wrapper 沉默丟棄 `targetReturn` 參數，最終在 code review 中連 wrapper 一併刪除。目前唯一生產呼叫端直接使用 `shared.ComputeSortino(dailyReturns, {Frequency: PerDay, MinSamples: 2})`。
- 在 `PerformanceReport` struct 增加對應欄位
  - ✅ 已完成（`report.SortinoRatio` 賦值於 `GenerateReport` 內）
- 前端 `performance-report.js` 新增對應 KPI 卡片
  - ✅ 已完成

### 🟡 MEDIUM #4: AI 觀測台僅顯示最弱 agent，缺少全面觀測視圖

**檔案**: `internal/monitoring/service/pipeline.go:117-160` 與 `web/static/js/pages/dashboard.js`

**問題**: `/api/dashboard/agent-observatory` 回傳的 `WeakestAgentScorecards` 僅載入「最弱」的 agent（依 Sharpe 排序取前 N），而非全部 agent 的觀測數據。前端儀表板僅顯示一個「待改進 AI 策略」卡片。

**建議**: 
- API 增加 `all_agents` 參數，回傳全部 agent 的 scorecard
- 前端增加 Agent Observatory 獨立區塊：顯示所有 agent 的 Sharpe、勝率、交易數、近期趨勢
- 考慮增加 agent 績效的時序圖（Sharpe 隨時間變化）

### 🔵 LOW #5: 前端硬編碼產業/agent 對照表

**檔案**: `internal/monitoring/api/live/handlers.go:284-307`

```go
sectorLabelMap := map[string]string{
    "semiconductor": "半導體",
    "ai_supply_chain": "AI供應鏈",
    ...
}
agentLayerMap := map[string]string{
    "taiwan-macro-01": "macro",
    "foreign-flow-01": "macro",
    ...
}
```

**問題**: 產業標籤對照表和 agent layer 對照表在 `handlers.go` 中硬編碼，與 `configs/agents.json` 和 `internal/industry/` 的定義重複。當新增產業（如低軌衛星）或 agent 時，此處需要手動同步更新，容易遺漏。

**建議**: 
- 產業標籤應從 `industry.ClassificationTree` 動態讀取
- Agent layer 應從 `configs/agents.json` 的 `layer` 欄位動態讀取

### 🔵 LOW #6: 組合持倉缺少產業板塊欄位

**檔案**: `web/static/js/pages/portfolio.js:80`

```html
<td>—</td>  <!-- 產業板塊欄位為空 -->
```

**問題**: 前端組合持倉表格的「產業板塊」欄位永遠顯示 `—`。此欄位在 HTML 模板中存在但從未被填充，可能是因為後端 `PositionDTO` 缺少產業分類資訊。

**建議**: 
- 在 `PositionDTO` 增加 `Sector` 欄位
- `LoadPortfolioState()` 透過 `symbol → sector` 映射（可從 `industry.ClassificationTree` 取得）填充產業資訊

---

## 4. 建議增強項目

### 4.1 增加時間加權報酬 (TWR) 計算

目前系統使用**簡單報酬**（`(end - start) / start`）計算累積報酬。對於有多筆現金流入/流出的組合，建議增加 **時間加權報酬 (Time-Weighted Return, TWR)** 計算：

```go
// TWR = ∏(1 + sub_period_return) - 1
// 每個子期間以現金流發生時點為邊界
```

這在實盤交易（有出入金）情境下尤為重要。

### 4.2 增加基準比較 (Benchmark Comparison)

目前績效報告僅顯示絕對報酬，缺少與基準指數（如加權指數、台灣 50）的比較。建議：

- 在 `PerformanceReport` 增加 `BenchmarkReturn`, `Alpha`, `TrackingError` 欄位
- 從 `marketdata` provider 取得基準指數歷史報酬
- 前端增加基準比較圖表

### 4.3 增加部位層級的風險分解 (Risk Decomposition)

目前組合持倉頁面僅顯示個股的未實現損益，缺少風險分解。建議：

- 增加 **VaR 貢獻** (Component VaR)：每檔持股對整體 VaR 的邊際貢獻
- 增加 **因子曝險分解**：每檔持股在各因子（動能、價值、品質）上的曝險
- 前端以熱力圖或長條圖呈現

---

## 5. 架構評估

### 5.1 優點 ✅

- **清晰的職責分離**：`internal/reporting/` 負責純計算（無 I/O），`internal/monitoring/service/` 負責資料聚合，`internal/monitoring/api/` 負責 HTTP 處理
- **統一的 JSON 命名慣例**：所有 API 回應使用 snake_case，與 `domain.*` 型別的 JSON tag 一致
- **正確使用 ledger 作為單一事實來源**：所有持倉和績效數據從 ledger 讀取，避免重複計算
- **模組化前端**：JS 檔案按頁面/組件合理拆分，支援 lazy loading

### 5.2 可改進處 ⚠️

- **LiveService 過於龐大**：`internal/monitoring/service/live.go` 同時處理 portfolio、trade、PnL、equity curve 等多種職責，建議拆分為 `PortfolioService`、`TradeService`、`EquityService`
- **重複的產業對照邏輯**：`handlers.go` 中的 `sectorLabelMap` 和 `agentLayerMap` 應該來自於設定檔而非硬編碼
- **缺少 API 回應的結構定義**：部分 handler 使用 anonymous struct 作為回應型別（如 `PnLAttributionResponse`），不利於文件化和型別安全

---

## 6. 行動建議優先級

| 優先級 | 項目 | 預估工時 |
|--------|------|---------|
| 🔴 HIGH | 修復累積報酬率分母（現金 → 起始本金） | 1h |
| 🔴 HIGH | 組合持倉 KPI 增加風險指標（未實現損益、最大回撤、集中度） | 2h |
| 🟡 MEDIUM | 績效報告增加 Sortino/Calmar/Information Ratio | 3h |
| 🟡 MEDIUM | AI 觀測台擴展為全面 agent dashboard | 3h |
| 🔵 LOW | 產業/agent 對照表改為動態讀取 | 1h |
| 🔵 LOW | 組合持倉產業板塊欄位填充 | 1h |
| ⚪ 建議 | 增加 TWR 計算、基準比較、風險分解 | 5h+ |

---

## 7. 結論

atlas-go 的組合持倉、績效報告、AI 觀測台三個前端頁面及其對應後端系統，**架構設計合理、資料流清晰、計算邏輯大部分正確**。Sharpe ratio、Max Drawdown、年化報酬等核心金融指標的計算方式符合標準。

最關鍵的修復是 **累積報酬率分母問題**（HIGH #1）——這直接影響投資決策的正確性。其次，**組合持倉缺少風險指標**（HIGH #2）降低了前端的實用性。

建議的增強項目（TWR、基準比較、風險分解）屬於「進階金融工程功能」，可在核心修復完成後逐步迭代。

---

*報告由 Sisyphus（AI Orchestrator）基於程式碼盤查與金融工程標準檢驗自動產生。*
