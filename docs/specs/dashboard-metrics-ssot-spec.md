# 管理台指標單一事實源（SSOT）規格 — dashboard-metrics-ssot-spec

> 日期：2026-09-04（v1.0 正式化：2026-09-17）
> 狀態：**v1.0 正式規格**（2026-09-17 用戶裁決：由提案轉正式；本規格自此為管理台指標的 SSOT 依據）
> 索引：[`docs/documentation-map.md`](../documentation-map.md) §📐 Specs
> 上位決策：[`docs/decisions/2026-08-23-performance-report-ssot.md`](../decisions/2026-08-23-performance-report-ssot.md)（PG 為績效報告 SSoT）——本規格把該裁決**從績效報告一條路由推廣到全部管理台指標**
> 執行計畫：`.omo/plans/2026-09-04-risk-console-ssot-refactor.md`（workspace-private，不隨 repo 追蹤）
> 範圍：`/admin/live`、`/admin/portfolio`、`/admin/performance-report` 三頁消費的所有指標

---

## 1. 問題陳述（實測證據，2026-09-03 07:27 UTC production）

同一指標在三頁出現不同數值。根因不是前端顯示，是**後端讀取路徑分裂**：2026-08-23 的 SSOT 裁決只套用到 performance-report 一條路由，其餘 handler 仍各自直讀 JSONL 檔案或即時重算。

| 指標 | 數值 A | 數值 B | 數值 C | 根因 |
|------|--------|--------|--------|------|
| 交易數 | `trade-history` = **0 筆**（`ledger.NewStore` 硬編碼 JSONL，prod 為空殼） | `performance-report?period=all` total_trades = **24,014**（PG `recommendation_outcomes`，且是「推薦決策數」非成交數） | `period=30d` = 22,292 | 三來源 + 兩種語意混用 |
| 最大回撤 | 0.7220（JSONL 權益曲線全期，portfolio-state / risk / risk-exposure / capital-phase 四處各自讀檔重算） | 0.0561（perf-report 30d 窗口，PG） | 0.9229（Monte Carlo worst path，in-memory） | 三種口徑無標註；四 handler 重複實作 |
| VaR 95% | `risk-exposure`：`var_available=false` → 前端「觀察期中」 | `/api/dashboard/risk`：`insufficient_data=1` 但仍回 **-0.3214** 原值 → 前端照顯 -32.1% | — | 同一批 185 點資料，兩端點門檻邏輯分歧（30/252 判定分散兩處） |
| 累積稅負 | `tax-snapshot` total_tax_paid = **20,140**（對現持倉做「假設今日清倉」試算） | perf-30d total_tax_paid = **101,405**（PG 實繳累計） | — | 同名不同義：預估清倉稅 ≠ 已繳稅，兩頁都標「累積」 |
| 未實現損益 | live-status / portfolio-state / positions 全為 **0** | — | — | livestore 寫入端不 mark-to-market（`UnrealizedPnL` 自進場後未更新，`CurrentPrice` 有更新） |
| Sharpe | capital-phase `rolling_sharpe`=1.03（inline 自算、30 場次窗口、JSONL） | perf-report `sharpe_ratio`=1.66（全期、PG） | top_agents `sharpe_like`（第三定義） | 三處三實作三窗口 |

### 架構根因（一句話）

**PG 是已裁決的 SSOT，但只有 `RegisterPerformanceRoutes` 走 backend-aware factory（`NewReportOutcomeStore`）；其餘 6 處 handler 直接 `os.ReadDir(ledgerDir/sessions)` 或 `ledger.NewStore(ledgerDir)` 硬讀 JSONL** —— production（`ATLAS_STORE_BACKEND=postgres`）上 JSONL 是導入來源空殼，於是每頁各拿各的答案。

違規點清單（全部位於 `internal/monitoring/`）：

| 檔案 | 函式 | 違規讀取 |
|------|------|---------|
| `api/live/handlers.go` | `HandleRiskExposure` | `os.ReadDir(sessionsDir)` 自算 risk snapshot |
| `api/live/handlers.go` | `HandleTradeHistory` → `service/live.go LoadTradeHistory` | `ledger.NewStore(LedgerDir)` 硬編碼 JSONL |
| `service/live.go` | `buildEquityCurve` | 直讀 `sessions/*/summary.json` |
| `api/live/benchmark.go` | `HandleBenchmarkComparison` | `os.ReadDir(sessionsDir)` |
| `api/risk/handlers.go` | `HandleRiskMetrics` | `os.ReadDir(sessionsDir)` + 自己的 VaR 門檻邏輯 |
| `api/system/handlers.go` | `HandleCapitalPhase` | `os.ReadDir(sessionsDir)` + inline Sharpe/回撤自算 |

對照（正確示範）：`dashboard_api.go RegisterPerformanceRoutes` 用 `ledger.NewReportOutcomeStore(cfg)`（PG-first + JSONL fallback + degraded 標記）。

---

## 2. 資料分層（三層，先定義再談指標）

| 層 | 內容 | 權威儲存 | 生命週期 |
|----|------|---------|---------|
| **L-hot（盤中即時態）** | 現金、持倉、市值、熔斷狀態、當日損益 | livestore JSON（`data/state/live*`）——**本層 SSOT 維持檔案**，由 sim 引擎獨家寫入 | 盤中覆寫 |
| **L-cold（結算歷史）** | 場次淨值、成交、稅費、推薦 outcomes、風控評語 | **PG**（`session_summaries` / `trades` / `recommendation_outcomes`）；JSONL = 導入來源，非 SSOT | append-only |
| **L-sim（假設/模擬）** | Monte Carlo 回撤、預估清倉稅、壓力情境 | 各模擬器輸出（in-memory / 排程產物） | 每次模擬覆寫 |

**規則 R1**：任何指標先歸屬唯一一層；跨層同名指標視為**不同指標**，必須不同名（見 §3 命名規則）。
**規則 R2**：L-cold 的讀取一律經 `internal/ledger/store_factory.go` 的 backend-aware factory，禁止 handler 直讀 JSONL/SQLite 路徑（本規格把 AGENTS.md「CLI/job 須走 store_factory」禁令推廣到 **HTTP handler**）。
**規則 R3**：L-sim 指標的 API 回應必須帶 `"metric_kind": "simulated"` 與產生時間；前端必須以「模擬」字樣標示。
**規則 R4**：同名指標只能有一個計算函式。回撤一律 `risk.CalculateMaxDrawdown`；Sharpe 一律單一共用函式；禁止 handler 內 inline 重算（capital-phase 的 inline Sharpe/回撤須移除）。

---

## 3. 指標 SSOT 對照表（字典 v1）

> 每個指標指定**唯一權威來源 + 唯一對外端點**。其他頁面/端點一律消費該端點（可快取）。
> 「語意拆分」= 現在同名但不同義，必須改名分開。

### 3.1 資產與損益

| 指標（顯示名） | 層 | 權威來源 | 唯一對外端點.欄位 | 現有多源點（須消滅） |
|---|---|---|---|---|
| 現金 / 可用現金 | L-hot | livestore `LoadLastPortfolioState` | `live-status.portfolio.cash` / `.available_cash` | portfolio-state 另算一份（同源檔案，改為內部複用同一 loader，對外仍以 live-status 為準；portfolio-state 欄位保留但標 deprecated） |
| 持倉市值 / 總曝險 | L-hot | livestore positions ΣMarketValue | `live-status.portfolio.total_exposure` | risk-exposure、capital-phase 各自重算 ΣMV |
| 持倉數 | L-hot | livestore positions len | `live-status.portfolio.positions_count` | portfolio-state / risk-exposure / capital-phase 各自 len() |
| 未實現損益 | L-hot | livestore positions（**寫入端目前不更新 = bug**） | `portfolio-state.unrealized_pnl_total` + positions[].unrealized_pnl | 修復前全為 0；過渡期由讀取端以 QuoteStore 現價即時重算（見計畫 P1-6，不動 live 寫入路徑） |
| 已實現損益 | L-cold | PG `session_summaries` 累計（sim persistent state 為輔） | `portfolio-state.realized_pnl` | — |
| 淨值（稅前/稅後） | L-cold 歷史 + L-hot 當值 | PG `session_summaries.portfolio_value` / `total_tax_paid`；當值 = cash + ΣMV | `portfolio-state.portfolio_value`（當值）；`performance-report.after_tax_value`（期末） | tax-snapshot 的 before/after_tax_pnl 是清倉試算，**改名** `liquidation_estimate_*` |
| 權益曲線 | L-cold | PG `session_summaries`（經 factory） | `portfolio-state.equity_curve` | `buildEquityCurve` 直讀 JSONL；benchmark-comparison 自建第二條曲線 → 兩者改吃同一 loader |
| 累積已繳稅負 | L-cold | PG `session_summaries.total_tax_paid` Σ | `performance-report.total_tax_paid`（period 連動） | portfolio KPI 誤用 tax-snapshot 清倉試算值 → 改接本端點 |
| 預估清倉稅負（**語意拆分，新名**） | L-sim | `TaiwanTaxCalculator` 對現持倉試算 | `tax-snapshot.total_tax_paid`（標籤改「若今日清倉預估稅費」） | 與「已繳稅負」同名混淆 |

### 3.2 交易與決策計數

| 指標 | 層 | 權威來源 | 唯一對外端點.欄位 | 現有多源點 |
|---|---|---|---|---|
| 成交筆數 | L-cold | PG `trades` 表 | `trade-history`（陣列長度）+ `performance-report.real_trade_count` 改為**真正成交數** | `LoadTradeHistory` 硬讀 JSONL → prod 回 0 |
| 推薦決策數（**語意拆分，新名**） | L-cold | PG `recommendation_outcomes` | `performance-report.total_outcomes`（新增欄位；舊 `total_trades`=24,014 實為 outcome 數，標 deprecated） | 「24,014 筆交易」是把推薦當成交 |
| 交易明細 | L-cold | PG `trades` 表 | `trade-history` | 同上，走 factory |

### 3.3 風險指標

| 指標 | 層 | 權威來源 | 唯一對外端點.欄位 | 現有多源點 |
|---|---|---|---|---|
| 最大回撤（全期） | L-cold | `risk.CalculateMaxDrawdown(PG 權益曲線全期)` | `risk-exposure.max_drawdown_pct` | portfolio-state / risk / capital-phase 各自讀檔重算（四份實作 → 收斂為一） |
| 最大回撤（期間窗口） | L-cold | 同函式 + period 窗口 | `performance-report.max_drawdown_pct`，標籤必帶「近 N 日」 | 與全期值同名混淆（72.2% vs 5.5%） |
| 壓力模擬回撤（**語意拆分**） | L-sim | Monte Carlo 排程產物 | `drawdown.max_drawdown`，標籤「Monte Carlo 最壞路徑」 | 92.3% 與歷史回撤並排無標註 |
| VaR 95/99、CVaR | L-cold | `risk.ComputeRiskSnapshot` + **單一 252 門檻判定** | `risk-exposure.var_95/99, cvar_95, var_available` | `/api/dashboard/risk` 的 `risk_snapshot` 重複定義且門檻邏輯分歧 → 該端點 snapshot 欄位標 deprecated，前端 risk-gate-panel 改吃 risk-exposure；未滿 252 一律只回 `var_available=false`，不回原值 |
| Rolling Sharpe（近 30 場次） | L-cold | 單一共用 Sharpe 函式 + 固定 30 場次窗口 | `capital-phase.rolling_sharpe` | capital-phase inline 自算 → 移除 inline |
| 全期 Sharpe | L-cold | 同函式 + 全期 | `performance-report.sharpe_ratio` | top_agents.sharpe_like 改名 `per_agent_sharpe_like` 並標註「非組合級 Sharpe」 |
| 集中度 HHI | L-hot | portfolio positions Σw² | `portfolio-state.concentration_ratio` | portfolio 風險分析卡重複 |
| 前 5 大權重 | L-hot | positions 排序取 5 | `risk-exposure.concentration` | 與 HHI 同名「集中度」→ 標籤改「前 5 大持倉權重」 |
| 板塊曝險 | L-hot | risk-exposure computeSectorFactorExposure | `risk-exposure.sector_exposure` | — |
| 熔斷狀態 | L-hot | livestore circuit breaker 檔 | `live-status.circuit_breaker` | `state=unknown` 前端誤顯示「正常」（v1 計畫 B7） |

### 3.4 歸因與敘事

| 指標 | 層 | 權威來源 | 唯一對外端點.欄位 | 現有多源點 |
|---|---|---|---|---|
| AI 貢獻（全期/期間） | L-cold | PG outcomes 聚合 | `performance-report.top_agents` | pnl-attribution 只讀「最新場次」JSONL → 改走 PG 或明確改名「最近場次歸因」並標註窗口 |
| 風控長評語（最新） | L-hot 決策 + L-cold 歷史 | RiskGate.LastDecision；**PG `session_summaries.risk_commentary` 已有欄位且已持久化** | `risk/commentary`（in-memory 為主，**fallback 讀 PG 最新 summary.risk_commentary**，純讀取端改動） | 服務重啟後評語憑空消失 |
| 市場狀態績效 | L-cold | PG outcomes × regime | `performance-report.regime_breakdown` | — |
| 基準比較 | L-cold | PG 權益曲線 vs TAIEX | `benchmark-comparison` | 內建第二條權益曲線 → 改用 §3.1 同一 loader |

### 3.5 後端落實方式（對照表如何變成程式）

1. 新共用 loader：`internal/monitoring/service` 新增（或擴充 `LiveService`）`SessionHistoryProvider`，內部走 `ledger.NewReportOutcomeStore(cfg)`（PG-first + degraded），對外提供 `EquityCurve()`、`SessionSummaries(period)`、`Trades()`、`Outcomes()`。
2. 上表 §6 違規點的 6 個 handler 全部改注入該 provider；DI 入口在 `dashboard_api.go` 的 `Register*Routes`（`RegisterPerformanceRoutes` 已有同款前例，可直接複製 pattern）。
3. 每個端點回應加 `"source": "postgres"|"jsonl"` + `"degraded": bool`（沿用 PGFirstOutcomeStore 既有介面），前端 degraded 時顯示來源警示。
4. 聚合端點（供頁面合併後一次取數）：新增 `GET /api/dashboard/overview` = live-status + portfolio-state 精華 + risk-exposure 摘要，60s TTL 快取（agent-observatory 快取前例 PR #1813）。

---

## 4. 相容性紅線

- 不刪既有欄位；語意改變的欄位用「新增正名欄位 + 舊欄位 deprecated 註解」過渡一個版本。
- 不動 live trading 寫入路徑（sim 引擎、RiskGate 決策流程）；unrealized mark-to-market 與 commentary fallback 皆為**讀取端**改動。livestore 寫入端的 mark-to-market 修正另立風險評估，不在本規格範圍。
- 本規格落地後，任何新增管理台指標必須先在本表登記（層、來源、端點），否則不予合併。
