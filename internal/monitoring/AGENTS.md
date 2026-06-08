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
