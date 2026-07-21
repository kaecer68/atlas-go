# AGENTS.md — strategy cluster

> 合併 `strategy` / `strategy_ranker` / `strategy_validator` / `strategy_techniques` 的模組陷阱。完整架構與流程見 `docs/`。

---

## strategy_techniques（心法庫）

`strategy_techniques` 提供以 `StrategyFrame` 為核心的規則引擎，作為台股投資心法看板與系統決策依據。五層框架與 4 核心短線指標的完整對應表見 [`docs/specs/strategy-techniques-spec.md`](../../docs/specs/strategy-techniques-spec.md)。

### 關鍵陷阱

| 陷阱 | 說明 |
|------|------|
| **FactorType 變更協議** | 新增/刪除/改名 `FactorType` 必須執行 8 步同步協議，見 `.claude/skills/atlas-factor-change-protocol/SKILL.md`。 |
| **StrategyFrame 取代 EventRule** | `StrategyFrame` 為心法主結構；`Registry` 外部化至 `data/seeds/strategy_techniques.json`。 |
| **HitRate 來源** | 主題 HitRate 必須從 `DefaultTemplates` / `hitRateForTheme()` 取得，禁止手動計算。 |

---

## strategy（策略選擇與配置）

模組職責：策略註冊、選擇與比較，依據盤勢與績效動態切換投資策略。

| 陷阱 | 說明 |
|------|------|
| **策略切換有冷卻期** | `Selector.shouldSwitch()` 檢查 `MinSwitchInterval`，短時間內不會反覆切換。 |
| **無候選時 fallback** | `Selector.Select()` 無 regime 匹配策略時回傳 `all_weather`；若無 all_weather 則回傳 `fallback`。 |
| **ComparisonEngine 分數公式** | `GetScore()` = Sharpe×0.4 + DailyReturn×30×0.3 + WinRate×0.3，歷史不足 days 時回傳 0.5。 |
| **Allocator 權重夾制** | `StrategyAllocator` 預設 `maxWeight=0.50`、`minWeight=0.05`，以迭代方式重新正規化。 |

---

## strategy_ranker（回測排名）

模組職責：策略回測結果排名 + tier 標記（free/registered/premium）。

| 陷阱 | 說明 |
|------|------|
| **非即時推薦入口** | 此模組處理**回測結果排名**，不是 `/api/recommendations` 的修復點；即時建議應呼叫 `strategy.ComparisonEngine.GetScore()`。 |
| **Tier 判定在 validator** | 任何 tier 規則變更需修 `internal/strategy_validator/assign_tiers.go`，非本模組。 |
| **空輸入回傳空陣列** | `RankAndTier` 期望 `[]StrategyReport`；input 空時回傳空陣列，caller 需檢查。 |

---

## strategy_validator（回測驗證）

模組職責：策略歷史回測驗證、績效指標計算、排名與分層。

| 陷阱 | 說明 |
|------|------|
| **Sharpe 計算委託 shared** | 不自行實作年化 Sharpe，統一用 `internal/domain/shared.ComputeSharpe`，避免不同模組產出不同 Sharpe。 |
| **TAIEX 相關係數可能 NaN** | Pearson 相關係數在樣本不足或 TAIEX 持平時可能為 NaN，程式碼已防禦為 0。 |
| **排名邏輯在本包內** | `Rank()` 與 `AssignTiers()` 操作 `StrategyReport` 欄位，因此位於 validator 包內；外部透過 `strategy_ranker.Ranker` 呼叫。 |

---

## 測試

- `strategy`：Registry CRUD、Selector 切換邏輯、Allocator 風險平價、ComparisonEngine 分數。
- `strategy_ranker`：`ranker_test.go` 的 TestRankAndTier / TestFreePremiumFilters。
- `strategy_validator`：totalReturnPct、maxDrawdownPct、winRate、pearsonCorrelation、Validate 端到端、Rank/AssignTiers。
- `strategy_techniques`：`go test ./internal/strategy_techniques/...`。
