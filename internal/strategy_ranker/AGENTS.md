# AGENTS.md — internal/strategy_ranker

**成熟度**: experimental (X-tier, Wave 11)
**模組職責**: 策略排名 + tier 標記（free/registered/premium gating）

---

## 核心型別

| 型別 | 檔案 | 功能 |
|------|------|------|
| `Ranker` | `ranker.go` | 對外服務：`RankAndTier(reports) → Ranked` |
| `RankAndTier` | `ranker.go:30-34` | 兩步：先 `Rank` 再 `AssignTiers`（從 `strategy_validator` 借） |
| `RankedReport` | (delegated) | `strategy_validator.RankedReport` — 包含排名 + tier 標記 |

## 資料流

```
Input: []strategy_validator.StrategyReport (回測結果)
       ↓
strategy_ranker.Ranker.RankAndTier(reports)
       ↓
       strategy_validator.Rank(reports)
       ↓
       strategy_validator.AssignTiers(ranked)
       ↓
Output: []strategy_validator.RankedReport
       ↓
GET /api/strategy-ranker
       ↓
handleRank (HTTP layer)
```

## 與 P0-1 的關係

**重要**：此模組**不是** `/api/recommendations` 的修復點。它處理的是**回測結果排名**（歷史），不是即時市場建議（即時）。

P0-1 重寫 `internal/recommender/handler.go::HandleRecommendations` 所需的策略信號**不透過這裡**，應直接呼叫 `strategy.ComparisonEngine.GetScore()` 等即時 API（見 `internal/recommender/AGENTS.md` 整合點）。

## 已知陷阱

| 陷阱 | 說明 |
|------|------|
| **輸入依賴** | `RankAndTier` 期望 `StrategyReport` 列表（回測引擎產出）；若 input 空會回傳空陣列，caller 需檢查。 |
| **Tier 判定規則集中於 strategy_validator** | 任何 tier 規則變更需修 `internal/strategy_validator/assign_tiers.go`，不是這檔。 |
| **無 free/registered/premium filter** | 由 `StrategyReport` 已內附 `Tier` 欄位判定；`AssignTiers` 為每個 report 標記 tier。 |

## 與其他模組整合

- `internal/strategy/selector.go` — `Strategy` 介面（本模組不依賴）
- `internal/strategy_validator/` — `Rank` / `AssignTiers` 委派對象
- `cmd/atlas-mcp/server/tools.go` — `strategy_ranker` MCP tool 包裝
- `cmd/atlas/main.go:1175` — `strategyRanker.RegisterRoutes(mux, stRegistry)`

## 測試

- `ranker_test.go` 既有 TestRankAndTier / TestFreePremiumFilters
- Mock input 用 `[]*StrategyReport` 結構
