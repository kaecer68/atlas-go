# AGENTS.md — internal/strategy_validator

**成熟度**: experimental
**模組職責**: 策略歷史回測驗證、績效指標計算、排名與分層。

---

## 核心型別

| 型別 | 功能 |
|------|------|
| `Validator` | 主驗證器，計算 Sharpe / 最大回撤 / 勝率 / TAIEX 相關係數 |
| `StrategyReport` | 單一策略驗證報告（JSON 可序列化） |
| `RankedReport` | 附帶排名與分層標籤的報告 |
| `BatchReport` | 一次驗證批次的所有策略報告集合 |

---

## 外部依賴

- `internal/domain/shared.ComputeSharpe` — 年化 Sharpe 計算（TWSE 頻率 sqrt(243)）
- `internal/strategy.ComparisonEngine` — 現有交易紀錄（可選，用於從既有數據建構）

---

## 陷阱

| 陷阱 | 說明 |
|------|------|
| **Sharpe 計算委託 shared** | 不自行實作年化 Sharpe，統一用 `shared.ComputeSharpe`，避免不同模組產出不同 Sharpe。 |
| **TAIEX 相關係數為 Pearson** | 若樣本不足或無變異（例如 TAIEX 連續多日持平），相關係數可能為 NaN — 程式碼已防禦為 0。 |
| **排名邏輯在 validator 內** | `Rank()` 和 `AssignTiers()` 在 validator 包內（非 strategy_ranker），因為它們操作 `StrategyReport` 型別欄位。外部可透過 `strategy_ranker.Ranker` 呼叫。 |
| **Tier 分層可覆寫** | `AssignTiers()` 是開放邏輯；外部可依商業需求自行分層，不強制依排名。 |

---

## 測試

- 涵蓋：totalReturnPct、maxDrawdownPct、winRate、pearsonCorrelation、Validate 端到端、Rank/AssignTiers
- 執行：`go test ./internal/strategy_validator/...`
