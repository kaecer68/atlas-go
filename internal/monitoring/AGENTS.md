# AGENTS.md — internal/monitoring

本目錄負責系統的監控、儀表板 API 與人工干預控制介面。

---

## 概覽

`DashboardAPI` 是專案中最大的單一服務組件，聚合 `ledger`、`narrative`、`orchestrator` 與 `sim` 的資料並提供 HTTP 介面。核心邏輯位於 `dashboard_api.go`。

> **Legacy API 變更**：`NewDashboardAPI`（legacy `CompositeMacroProvider` 組裝）已標記為 `Deprecated`；生產環境請使用 `NewDashboardAPIWithGateway` + `monitoring.DataFetcher`，讓 macro 資料統一流經 `apigateway.Gateway`。

### API 路由（按職責）

| 職責 | 端點 |
|------|------|
| 核心儀表板 | `/api/dashboard/macro-radar`, `/api/dashboard/recommendation-pipeline` |
| 產業生態系 | `/api/dashboard/industry-classification`, `/api/dashboard/industry-seasonality`, `/api/dashboard/industry-seasonality-calendar`, `/api/dashboard/industry-cycle`, `/api/dashboard/industry-linkage`, `/api/dashboard/industry-risk`, `/api/dashboard/industry-overview`, `/api/dashboard/industry-graph`, `/api/dashboard/industry-shock-simulation` |
| 敘事分析 | `/api/narrative/events`, `/api/narrative/chains` |
| 人工干預 | `/api/control/approve-recommendation`, `/api/control/reject-recommendation`, `/api/control/audit-log` |
| 實驗與回測 | `/api/experiment/*`, `/api/backtest/*` |

### JSON 慣例

讀 JSONL（`recommendation_outcomes.jsonl`）時，**部分 legacy 欄位使用 PascalCase**（如 `AgentID`, `Skill`），Unmarshal 結構必須精確對應。前端用 snake_case。詳見 `內部審計（.omo/audit/）`。

---

## Agent Observatory API（`GET /api/dashboard/agent-observatory`）

回傳所有 agent 的 Scorecard 陣列。**OOS 驗證欄位**（`Scorecard.ScorecardDetail`）：

| 欄位 | 意義 |
|------|------|
| `is_sharpe` / `oos_sharpe` | In/Out-of-Sample Sharpe |
| `is_oos_ratio` | `oos_sharpe / is_sharpe`，< 1 表示衰減 |
| `overfit_warning` | `none`/`mild`/`severe` — 基於 `IsOOSDivergent()` |
| `rolling_sharpe_trend` | `up`/`down`/`flat` — rolling sharpe 斜率方向 |
| `oos_sample_warning` | `none`/`low`/`very_low` — OOS 樣本數充足性 |

**陷阱**：修改 `domain.Scorecard` 後必須同步更新 `agentObservatoryScorecard` mapping。

---

## ANTI-PATTERNS

- **大小寫失真**：`handleRecommendationPipeline` 解析 JSONL 時 anonymous struct 標籤錯誤會導致欄位為空值
- **全域 Mutex 阻塞**：`backtestMu` 保護回測執行狀態，避免在 API handler 中長時間阻塞
- **未處理的 Legacy 格式**：讀舊 session 時 `PassedGuards` 等新欄位可能缺失，需在 handler fallback（預設 true）
- **OOS 取樣範圍太小**：`LoadAgentObservatory()` 不傳 `sessionID` 時若 fallback 到 `LoadSessionOutcomes()` 只會讀取**最新單一 session**，導致 OOS 樣本不足。**必須改用 `LoadOutcomesFromSessions()`** 取得完整歷史 session。

---

## 跨市監控資料可見性（4 層）

`CrossMarketService` 採用 4 層模式，確保通道靜默失敗時資料缺失能被看見：
- **L1 Gateway**：`internal/apigateway/provider.go` 標記 `FetchResult.Fallback` / `LastError`
- **L2 Adapter**：`internal/monitoring/gateway_adapter.go` 暴露 `ChannelErrors()`
- **L3 Service**：`internal/monitoring/service/crossmarket.go` 產出 `DataStatus` + `FailedChannels`，覆蓋以下 8 個 Yahoo 通道：
  `us_spx` / `us_ndx` / `us_dji` / `sox_index` / `us_nvda` / `us_aapl` / `us_msft` / `tsm_adr`
- **L4 Frontend**：`shared_web/static/js/pages/crossmarket.js` 顯示降級 badge/banner

完整設計與技能指引見 **`.claude/skills/atlas-data-visibility/SKILL.md`**。

---

## DriftDetector v2

`internal/monitoring/service/drift_detector.go` 提供 `EventDriftDetected` 偵測與發布。**v1 vs v2**：
- `NewDriftDetector(bus)`：v1，僅訂閱 `EventPositionUpdate`，無 target drift
- `NewDriftDetectorWithTargets(bus, provider)`：v2，多訂閱 `EventRegimeChangeConfirmed`，新增 `target_drift` 偵測

**閾值**（`thresholds` 常數）：`DriftMaxConcentrationThreshold=0.25`、`DriftTurnoverThreshold=0.15`、`DriftTargetWeightThreshold=0.10`

**v1/v2 payload 契約**：`max_concentration` / `max_symbol` / `turnover` / `total_value` / `period_start` / `reasons` / `thresholds` 為 v1 既有欄位（**append-only**），v2 僅在後方新增。詳見 `drift_detector.go` 與 `drift_helpers.go`。

---

## Live 偵測器協調器

Live 偵測器協調器（原始檔案仍保留 `wave9` 命名）在 live mode 統一啟動/協調/關閉 5 個偵測器（RegimeDebouncer、FactorWeightRegressionDetector、DriftDetector v2、ChannelHealthSynthesizer、IngestionLagMonitor）。歷史設計詳見 `內部交接（.omo/handoffs/）`。

---

## API 共享中介層認證白名單

`internal/monitoring/api/shared/` 提供 `AuthMiddleware`（API-key）與 `RequireUserJWT`（JWT）兩套 middleware，用途不同，勿混用。`AuthMiddleware` 在無 `ATLAS_API_KEY` 的 dev 環境會 pass through。

| 陷阱 | 說明 |
|------|------|
| **公開端點需同步加白名單** | 任何新增的 `/api/*` 公開端點必須**同步**加到 `cmd/atlas/main.go isPublicPath` + `internal/monitoring/api/shared/handler.go authFreeExactPaths/authFreePrefixPaths`，只改一處會 404/401。 |
