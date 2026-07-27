# Manifest: 擴充 Quote Store Warmup 涵蓋範圍

- **日期**: 2026-07-27
- **Issue**: 個股查詢出現「歷史股價資料尚未回填，無法計算技術指標」錯誤
- **根因**: `warmupQuotes()` 僅從 `DefaultRepresentativeStocks()` (96 檔) 收集 symbols，遺漏 ETF（14 檔）與非代表性個股（3 檔）
- **修復策略**: 讓 `warmupQuotes()` 同時從 `orchestrator.DefaultSymbols()` 收集 symbols（去 `.TW` 後綴）
- **變更範圍**: `internal/monitoring/dashboard_api.go` — 僅修改 `warmupQuotes()` 的 symbol collection 邏輯
- **風險**: LOW — 僅新增 symbols 到收集集合；Fugle API 不支援的 symbol 會被優雅跳過
