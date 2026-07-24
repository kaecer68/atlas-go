# Hermes Agent 系統審計 — 根因診斷與修復

> **日期**：2026-07-24 | **審計觸發**：hermes agent 12 項異常回報
> **分支**：`feat/hermes-audit-e01-e12` | **ATLAS_ENV**：development

## Goal

修復 hermes agent 發現的 P0/P1 bugs，並驗證其餘問題的實際資料狀態。

## Invariant Table

| ID | 問題 | 根因 | 狀態 | 驗證結果 |
|---|---|---|---|---|
| E-01 | sector_allocation_plan 503 | `SnapshotReader` 未接線 | ✅ 已修復 | 需 rebuild 後驗證 |
| E-02 | stock_get_fundamentals Sector="" | JSON fundamentals 無 Sector，與 industry 常數表不同步 | ✅ 已修復 | 需 rebuild 後驗證 |
| E-09 | regime_history 6/30-7/20 gap | cold-start，macro-ingest 從 7/21 開始寫入 | 🟡 文件 | 現有 9 筆（3 live + 6 synthetic），live 資料正常累積中 |
| E-03 | capital flow calibrating | burn-in 預期（52d < 90d MinSamples） | 🟢 非 bug | 7 forces 全有資料，resonance=1，quality=neutral |
| E-04 | technical 僅 1 row | 設計如此（snapshot），sma20=0 當時資料不足 | 🟢 非 bug | sma20=2433, sma50=2360.6, rsi14=62.5 — 資料已足夠 |
| E-05 | correlation is_fallback | observations=8 < window=20 + zero_variance | 🟡 cold-start | 需要更多交易日累積樣本 |
| E-06 | industry sector 無 HTTP | 設計選擇（in-memory constants） | 🟢 非 bug | 無需修改 |
| E-07 | tool count mismatch | 需計數驗證 | 🟡 待測 | 需 rebuild 後跑 mcp list |
| E-08 | capital-flow 無 auth | 設計如此（公開路由） | 🟢 非 bug | 無需修改 |
| E-10 | stress synthetic data | staging demo 遺留，已標記 source=synthetic | 🟢 非 bug | 10 筆中有 7 筆 synthetic，3 筆 live |
| E-11 | chains 無 timestamp | schema 缺 per-chain detected_at | 🟡 schema gap | chains 有 event_id 但無時間戳 |
| E-12 | risk_exposure 空倉 | burn-in 空倉，portfolio_value=3M, position_count=0 | 🟢 非 bug | 預期行為 |

## 修復內容

### Commit 1: `fix(manifest): #E01 wire FileClosureStore as SnapshotReader`
- `internal/monitoring/dashboard_api.go`: `newWiredIndustryService` 新增 `workDir string` 參數
- 在 return 前注入 `svc.WithSnapshotReader(sectorallocation.NewFileClosureStore(filepath.Join(workDir, "data/state")))`
- 更新兩處 constructor call sites (`NewDashboardAPI` line 219, `NewDashboardAPIWithGateway` line 258)
- 更新 test call sites (`dashboard_api_test.go`)

### Commit 2: `fix(manifest): #E02 add industry.ClassifyBySymbol fallback for empty Sector`
- `internal/stocktools/handler.go`: `HandleFundamentals` 在 `data.Sector == ""` 時 fallback 到 `industry.ClassifyBySymbol(symbol)`
- 新增 `internal/industry` import

## Backlog

- E-05: 考慮放寬 `RollingCorrelation.Update()` 的 n<3 → insufficient_samples 閾值，或用更寬鬆的 fallback（目前用 0.5 sentinel）
- E-11: 在 chains response 加入 `generated_at` timestamp
- E-07: rebuild 後驗證 MCP tool count
