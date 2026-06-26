---
name: atlas-factor-change-protocol
description: "Mandatory 8-step protocol for adding, removing, or renaming FactorType values in atlas-go. Ensures optimizer constants, weight engine maps, domain structs, pipeline score structs, and event adjustments stay synchronized."
version: "1.0"
category: feature
auto_load: false
load_policy: manual_only
created: "2026-06-26"
updated: "2026-06-26"
target_audience: developer
---

# Atlas Factor Change Protocol

本技能定義在 `atlas-go` 新增、刪除或重新命名 `FactorType` 時必須執行的 8 步協議，避免 optimizer、factor engine、domain model 與事件調整權重之間的型別漂移。

## 何時觸發

- 使用者要求新增、刪除或重新命名 `FactorType`
- 修改 `internal/portfolio/optimizer.go`、`factor_weight_engine.go`、`factor_engine_aggregate.go` 的 factor 相關常數或欄位
- 修改 `internal/domain/shared/shared.go` 的 `FactorScoreBreakdown` 或 `FactorScores`
- 修改 `internal/portfolio/optimizer_pipeline.go` 的 `symbolScore` 或 `calculateMultiFactorScores`

## 核心概念

### FactorType

- 定義：描述投資組合因子（Momentum、Value、Quality、Agent、InstitutionalSentiment、Liquidity 等）的型別或常數。
- 實作位置：`internal/portfolio/optimizer.go`
- 關鍵：新增/刪除/改名會影響權重計算、分數聚合與事件調整，必須全端同步。

### FactorWeightEngine

- 定義：根據市場事件與 Regime 動態調整 12 因子權重的引擎。
- 實作位置：`internal/portfolio/factor_weight_engine.go`
- 關鍵：基礎權重與事件 delta 皆來自 `ParametersConfig.FactorWeight`，是 factor 權重的唯一權威來源。

## 實作位置

| 概念 | 檔案路徑 | 關鍵函數 / 結構 |
|------|---------|----------------|
| FactorType 常數 | `internal/portfolio/optimizer.go` | `FactorType` 常數宣告 |
| 預設權重 | `internal/portfolio/factor_weight_engine.go` | `defaultBaseWeights` |
| Domain score breakdown | `internal/domain/shared/shared.go` | `FactorScoreBreakdown` |
| Domain scores | `internal/domain/shared/shared.go` | `FactorScores` |
| Pipeline score | `internal/portfolio/optimizer_pipeline.go` | `symbolScore` |
| Pipeline total score | `internal/portfolio/optimizer_pipeline.go` | `calculateMultiFactorScores` |
| Aggregate breakdown | `internal/portfolio/factor_engine_aggregate.go` | `CalculateAllScoresWithBreakdown` |
| Event adjustment | `internal/portfolio/factor_weight_engine.go` | `applyEventAdjustment` / `strategyDeltas` / `GetWeights` |

## 8 步協議

新增、刪除或改名任何 `FactorType` 時，**必須順序更新以下 8 個位置**：

| Step | 位置 |
|------|------|
| 1 | `optimizer.go` FactorType 常數宣告 |
| 2 | `factor_weight_engine.go` `defaultBaseWeights` map |
| 3 | `internal/domain/shared/shared.go` `FactorScoreBreakdown` struct |
| 4 | `internal/domain/shared/shared.go` `FactorScores` struct |
| 5 | `optimizer_pipeline.go` `symbolScore` struct |
| 6 | `optimizer_pipeline.go` `calculateMultiFactorScores` totalScore 計算 |
| 7 | `factor_engine_aggregate.go` `CalculateAllScoresWithBreakdown` breakdown 建構 |
| 8 | `factor_weight_engine.go` `applyEventAdjustment` / `strategyDeltas` / `GetWeights` |

## 驗證規則

- [ ] 8 個位置全部更新
- [ ] 執行 `go generate .`
- [ ] 執行 `bash scripts/ci/verify_factor_integrity.sh`
- [ ] 執行 `go build ./... && go test ./internal/portfolio/...`
- [ ] 確認 CI `quality.yml` 的 `factor-integrity` job 會通過

## 相關技能

| 技能 | 關聯 |
|------|------|
| `atlas-pre-change-protocol` | 修改 `internal/portfolio/` 前必須執行 7 步檢查 |
| `atlas-risk-management` | 因子權重與風險控管互動時參考 |

## 版本歷史

| 版本 | 日期 | 變更 |
|------|------|------|
| 1.0 | 2026-06-26 | 從 `internal/portfolio/AGENTS.md` §12 抽出成獨立 skill |
