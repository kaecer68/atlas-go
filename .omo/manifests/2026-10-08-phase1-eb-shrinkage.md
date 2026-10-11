# Manifest — Phase 1: EB 收縮 + Regime 條件 + Break-even（2026-10-08）

- Mode: Execute
- Branch: `feat/20261008-phase1-eb-shrinkage`
- Worktree: `/Users/kaecer/workspace/atlas/.worktrees/feat-20261008-phase1`
- ATLAS_ENV: development（production `atlas.goluck.uk` 不碰）
- In-scope: `internal/stockpicker/winrate.go`（新增純函數）、`internal/stockpicker/winrate_test.go`、
  `cmd/atlas-mcp/server/tools_stock_picker_scan.go`（scan 排序選項）、對應 test

## 背景

前一輪評估結論 Phase 1：Empirical Bayes shrinkage + regime-conditional 校準 + break-even 成本。
預研發現（本 session）：

1. 兩個預設家族（foreign-3d-net-buy、momentum-20d-positive）已於 2026-10-06 被降級
   （`internal/config/stockpicker_edge.go`，5 天淨成本期望 -0.517% / -0.987%），executor Stage 0b
   edge guard + scan 排名均已排除。新機制主要服務剩餘家族。
2. Regime 分層已存在（family_expectancy.go read-only + ConditionWinRate Regime 欄位 #1863），
   不可另造第三種算法；per-(symbol,source) 聚合與 gate 仍用 pooled 數字，此為缺口。
3. 無 shrinkage / break-even 實作（Step 0 新領域）。

## 本次範圍（additive only，LOW risk）

1. `ShrunkRate` 純函數（beta-binomial 收縮）+ 測試。
2. `BreakEvenCostRate` 純函數（forward return 中位數 = win_rate 50% 可承受之最大來回成本）+ 測試。
3. `RegimeFilteredWinRate` 純函數（同一切片先按 regime 過濾再走 SignalWinRate 口徑）+ 測試。
4. scan `sort_by=shrunk_rate`（Go 側 post-sort，不動 SQL whitelist）+ 測試；tool 描述同步。
5. executor 接線與 `stock_win_rate_by_regime` 遷移留待下輪（需雙言遷移 + 熱路徑量測），不納入本 PR。

## 不碰

- 現有 `SignalWinRate` / `ConditionWinRate` / `GroupAndSummarize` / `AggregateFromStore` 計算路徑。
- `stock_win_rate` schema（無遷移）。
- production、`.env`、live broker 旗標。
