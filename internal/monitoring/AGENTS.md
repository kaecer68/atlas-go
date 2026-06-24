# AGENTS.md — internal/monitoring

本目錄負責系統的監控、儀表板 API 與人工干預控制介面。

---

## 概覽 (OVERVIEW)

`DashboardAPI` 是專案中最大的單一服務組件，負責聚合 `ledger`、`narrative`、`orchestrator` 與 `sim` 的資料並提供 HTTP 介面。核心邏輯位於 `dashboard_api.go`。

## API 結構 (API STRUCTURE)

路由按職責分類註冊：
- **核心儀表板** (`RegisterRoutes`): `/api/dashboard/macro-radar`, `/api/dashboard/recommendation-pipeline` 等。
- **產業生態系** (`RegisterIndustryRoutes`): `/api/industry/cycles`（週期羅盤）、`/api/industry/seasonality`（季節性模式）、`/api/industry/seasonality/calendar`（季節性行事曆）、`/api/industry/linkage`（供應鏈連動）、`/api/industry/classification`（產業分類）、`/api/industry/detail`（產業詳細資訊）。
- **敘事分析** (`RegisterNarrativeRoutes`): `/api/narrative/events`, `/api/narrative/chains` 等。
- **人工干預** (`RegisterControlRoutes`): `/api/control/approve-recommendation`, `/api/control/reject-recommendation`, `/api/control/audit-log`。
- **實驗與回測**: `/api/experiment/*`, `/api/backtest/*`。

## JSON 慣例 (CONVENTIONS)

- **命名風格**: 外部 API 回傳結構一律使用 **snake_case**。
- **Domain 對齊**: `domain.*` 型別已內建 `json:"snake_case"` 標籤，定義新結構時必須沿用。
- **Unmarshal 陷阱**: 從 JSONL (`recommendation_outcomes.jsonl`) 讀取時，注意部分 legacy 欄位使用 **PascalCase** (如 `AgentID`, `Skill`)，Unmarshal 結構必須精確對應。

## Agent Observatory API (`GET /api/dashboard/agent-observatory`)

此端點回傳所有 agent 的 Scorecard 陣列，每個 Scorecard 包含以下 **OOS 驗證欄位**（位於 `Scorecard.ScorecardDetail`）：

| JSON 欄位 | 型別 | 意義 |
|-----------|------|------|
| `is_sharpe` | number | In-Sample Sharpe ratio |
| `oos_sharpe` | number | Out-of-Sample Sharpe ratio |
| `is_oos_ratio` | number | `oos_sharpe / is_sharpe`，< 1 表示衰減 |
| `overfit_warning` | string | `"none"` / `"mild"` / `"severe"` — 基於 IsOOSDivergent() |
| `rolling_sharpe_trend` | string | `"up"` / `"down"` / `"flat"` — rolling sharpe 線性迴歸斜率方向 |
| `oos_sample_warning` | string | `"none"` / `"low"` / `"very_low"` — OOS 樣本數充足性 |

> 前端 `dashboard.js` 的 observatory 表格使用 `rolling_sharpe_trend` 顯示 ▲▼ 趨勢箭頭，並根據 `overfit_warning` / `oos_sample_warning` 顯示色彩徽章。

**陷阱**：修改 `domain.Scorecard` 或 OOS 計算邏輯後，必須同步更新此端點的 response mapping 結構（`agentObservatoryScorecard`）。

## 高危反模式 (ANTI-PATTERNS)

| 陷阱 | 說明 |
|------|------|
| **大小寫失真** | 在 `handleRecommendationPipeline` 中解析 JSONL 時，若 anonymous struct 標籤錯誤會導致欄位為空值。 |
| **全域 Mutex 阻塞** | `backtestMu` 保護回測執行狀態，避免在 API handler 中進行長時間阻塞操作。 |
| **未處理的 Legacy 格式** | 讀取舊 session 時，`PassedGuards` 等新欄位可能缺失，需在 handler 進行 fallback 補齊（預設為 true）。 |
| **OOS 取樣範圍太小（單一 session）** | `LoadAgentObservatory()` 不傳 `sessionID` 時，若回退到 `LoadSessionOutcomes()` 只會讀取**最新單一 session**（88 筆），導致多數 agent OOS 樣本不足（`oos_sample_warning: insufficient`）。**必須改為直接呼叫 `LoadOutcomesFromSessions()`** 取得完整歷史資料（131k+ 筆，78 sessions）。|

## 跨市監控資料可見性 (Cross-Market Data Visibility)

`CrossMarketService` 與前端 `crossmarket.js` 採用 **4 層資料可見性** 模式,確保通道靜默失敗時資料缺失能被看見。

| 層級 | 檔案 | 職責 |
|------|------|------|
| L1 Gateway | `internal/apigateway/provider.go`, `gateway.go` | `FetchResult.Fallback` / `LastError` 標記 stale-cache fallback |
| L2 Adapter | `internal/monitoring/gateway_adapter.go` | `ChannelErrors()` 暴露 per-channel 錯誤 |
| L3 Service | `internal/monitoring/service/crossmarket.go` | `DataStatus` ("ok"/"degraded") + `FailedChannels []string` |
| L4 Frontend | `web/static/js/pages/crossmarket.js` | "資料獲取失敗" 紅色 badge + 降級 banner |

**觸發場景**: 任何 `MacroDataSnapshot` 欄位 Symbol 為空 → 表示對應 channel 失敗。

**Fail-safe 規則**:
- 8 個 US 指數/科技股欄位 (SPX/NDX/DJI/SOX/NVDA/AAPL/MSFT/TSM_ADR) 任一失敗 → `data_status="degraded"`
- `failed_channels` 列出失敗的 channelID (與 `internal/apigateway/gateway.go` 的 `channelIDs()` 對齊)
- 詳見 `.claude/skills/atlas-data-visibility/SKILL.md`

### 已修復：前端 PascalCase → snake_case 欄位錯誤（2026-05-07）

**問題描述**：前端 JavaScript 錯誤地使用 PascalCase 存取 API 回傳的 snake_case 欄位，導致畫面顯示 `undefined`。

**受影響欄位**（`GuardOutcome`）：
| Go 欄位名 | JSON tag | 前端錯誤引用 | 前端正確引用 |
|-----------|----------|-------------|-------------|
| `GuardID` | `guard_id` | `g.GuardID` | `g.guard_id` |
| `GuardSkill` | `guard_skill` | `g.GuardSkill` | `g.guard_skill` |
| `Passed` | `passed` | `g.Passed` | `g.passed` |
| `Reason` | `reason` | `g.Reason` | `g.reason` |
| `InputCount` | `input_count` | `g.InputCount` | `g.input_count` |
| `OutputCount` | `output_count` | `g.OutputCount` | `g.output_count` |

**影響範圍**：
- `renderMacroRadar()` — 總經雷達頁面顯示 `undefined 筆推薦 → 最終放行 undefined 筆`
- `renderDecisionChain()` — 決策鏈頁面控制層紀錄顯示異常
- `renderPipeline()` — 投資管線頁面控制層徽章顯示異常

**修復方式**：
1. 將所有前端 PascalCase 屬性存取改為 snake_case
2. 添加防禦性預設值（`g.input_count || 0`）防止未來欄位缺失
3. 新增 `validateApiResponse()` 函數，在開發階段自動檢測欄位命名不一致

**預防措施**：
- 後端 `domain.*` 型別的 JSON tag 一律為 snake_case
- 前端存取 API 回應時必須使用 snake_case
- 新增 `validateApiResponse(data, requiredFields, context)` 驗證工具，自動檢測 PascalCase 誤用

---

## DriftDetector v2 — Target Weights Drift

### Architecture

`internal/monitoring/service/drift_detector.go` + `drift_helpers.go` 提供 `EventDriftDetected` 的偵測與發布。

| 元件 | 位置 | 職責 |
|------|------|------|
| `DriftDetector` 介面 | `drift_helpers.go` | 公開 `Start(ctx) / Stop()` 契約 |
| `TargetWeightsProvider` 介面 | `drift_helpers.go` | 注入 symbol-level 目標權重（v2） |
| `driftDetector` 實作 | `drift_detector.go` | 訂閱 position update + regime change，週期性檢查 |
| `NewDriftDetector` | `drift_detector.go` | 舊版 constructor（無 target drift，向後相容） |
| `NewDriftDetectorWithTargets` | `drift_detector.go` | v2 constructor，接受 provider（可 nil） |

### Event Subscriptions

- `EventPositionUpdate`（v1 既有）：累積 `symbol → MarketValue` snapshot map
- `EventRegimeChangeConfirmed`（v2 新增）：更新 `currentRegime` 並重置 `prevTotal = 0`（re-baseline）

### 閾值 (Thresholds)

| 名稱 | 預設值 | 說明 |
|------|--------|------|
| `DriftMaxConcentrationThreshold` | 0.25 | 單一持倉最大佔比 |
| `DriftTurnoverThreshold` | 0.15 | 週期內總值變化率 |
| `DriftTargetWeightThreshold` | 0.10 | v2 — `|actual - target|` 最大 drift |

### 觸發原因 (`reasons`)

- `"concentration"`：max_concentration > 0.25
- `"turnover"`：turnover > 0.15
- `"target_drift"`：v2 — max_drift > 0.10（僅當 `TargetWeightsProvider` 非 nil 且回傳非空 map）

### 陷阱 (Traps)

| 陷阱 | 說明 |
|------|------|
| `WeightProvider` vs `TargetWeightsProvider` | `WeightProvider`（既有，factor-level）用於 `FactorWeightRegressionDetector`；`TargetWeightsProvider`（v2 新增，symbol-level）用於 DriftDetector。**不可混用**。 |
| `EventRegimeChangeConfirmed` payload 型別 | `regime_debouncer.go` 發布的是 `map[string]any`，**不是** `RegimeEventPayload` struct。Handler 必須用 `payload.(map[string]any)` 然後 `payload["new_regime"].(string)` 取值。 |
| `new_regime` 為 string | `regime_debouncer` 透過 `string()` 將 `domain.Regime` 轉為 string。**不可** type-assert 為 `domain.Regime`。 |
| Nil provider graceful | `NewDriftDetectorWithTargets(bus, nil)` 必須保留 v1 行為：concentration/turnover 偵測照常，target_drift 不 emit，v2 欄位不出現於 payload。`thresholds.target_drift` 一律存在（常數）。 |
| Regime change re-baseline | `onRegimeChangeConfirmed` 將 `prevTotal = 0` 而非 current total。**這是預期行為**：下一次 `checkPeriod` 會視為首次檢查並建立新 baseline，避免 regime 切換時的偽 turnover 事件。 |
| `Stop()` 必須取消兩個訂閱 | `d.regimeSub` 與 `d.sub` 都必須取消（nil-safe）。漏掉其中一個會造成 goroutine 洩漏。 |
| Symbol 不在 target map | 視為 `target = 0`，drift = `actual_weight`。`TestDriftDetector_V2SymbolNotInTargetMap` 驗證此行為。 |
| v1 payload 欄位不可變更 | `max_concentration` / `max_symbol` / `turnover` / `total_value` / `period_start` / `reasons` / `thresholds` 為既有契約，**append-only**。v2 僅在後方新增欄位。 |
| `max_drift_symbol` 確定性 | Go map 迭代順序不確定，`max_drift_symbol` 在 drift 平手時可能不固定。已用 `sort.Strings` 排序 symbol keys 確保確定性。 |

### 向後相容

- `NewDriftDetector(bus)` 保留（無 target drift 能力）
- v1 6 個測試一字不改
- v1 payload 欄位全部保留
- `DriftEventSchemaVer` 從 1 bump 到 2（消費者可透過此欄位區分）
- `thresholds.target_drift` 一律存在（即使 nil provider），讓前端可顯示閾值
