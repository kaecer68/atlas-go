# AGENTS.md — internal/monitoring

本目錄負責系統的監控、儀表板 API 與人工干預控制介面。

---

## 概覽 (OVERVIEW)

`DashboardAPI` 是專案中最大的單一服務組件，負責聚合 `ledger`、`narrative`、`orchestrator` 與 `sim` 的資料並提供 HTTP 介面。核心邏輯位於 `dashboard_api.go`。

## API 結構 (API STRUCTURE)

路由按職責分類註冊：
- **核心儀表板** (`RegisterRoutes`): `/api/dashboard/macro-radar`, `/api/dashboard/recommendation-pipeline` 等。
- **敘事分析** (`RegisterNarrativeRoutes`): `/api/narrative/events`, `/api/narrative/chains` 等。
- **人工干預** (`RegisterControlRoutes`): `/api/control/approve-recommendation`, `/api/control/reject-recommendation`, `/api/control/audit-log`。
- **實驗與回測**: `/api/experiment/*`, `/api/backtest/*`。

## JSON 慣例 (CONVENTIONS)

- **命名風格**: 外部 API 回傳結構一律使用 **snake_case**。
- **Domain 對齊**: `domain.*` 型別已內建 `json:"snake_case"` 標籤，定義新結構時必須沿用。
- **Unmarshal 陷阱**: 從 JSONL (`recommendation_outcomes.jsonl`) 讀取時，注意部分 legacy 欄位使用 **PascalCase** (如 `AgentID`, `Skill`)，Unmarshal 結構必須精確對應。

## 高危反模式 (ANTI-PATTERNS)

| 陷阱 | 說明 |
|------|------|
| **大小寫失真** | 在 `handleRecommendationPipeline` 中解析 JSONL 時，若 anonymous struct 標籤錯誤會導致欄位為空值。 |
| **全域 Mutex 阻塞** | `backtestMu` 保護回測執行狀態，避免在 API handler 中進行長時間阻塞操作。 |
| **未處理的 Legacy 格式** | 讀取舊 session 時，`PassedGuards` 等新欄位可能缺失，需在 handler 進行 fallback 補齊（預設為 true）。 |
