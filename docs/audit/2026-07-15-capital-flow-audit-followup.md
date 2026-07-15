# Capital Flow Audit Follow-up — 2026-07-15

> **背景**：2026-07-15 健康度盤查發現 13 個缺口
> **完整規劃**：`.omo/plans/2026-07-15-capital-flow-audit-followup/`（plan-only, gitignored）

## 13 個缺口與 PR 對應

| Gap | 描述 | PR | 狀態 | Commit |
|-----|------|----|------|--------|
| G-01 | regime_history 測試資料污染 | PR-FIX-01 | ✅ | `1af1c0cf` |
| G-02 | stress_index_history 只 1 筆 | PR-FIX-01 | ⏸️ schema OK, loader 待跑 | `7879db8e` |
| G-03 | scheduler_get_status JSON unmarshal | PR-FIX-02 | ✅ | `d1715121` |
| G-04 | prediction_backtest 表為空 | PR-FIX-01 | ⏸️ schema OK, writer 待補 | — |
| G-05 | 19/24 template 沒 EventType | PR-FIX-03 | ⏸️ 加 2 個、9 個需 macro 來源 | `f58f6e50` |
| G-06 | narrative tilt legacy 5 themes | PR-FIX-04 | ✅ | `23adcd28` |
| G-07 | DetectorInput 空 | PR-FIX-05 | ⏸️ 函數加, main.go 待 wire | `8de1c3f3` |
| G-08 | template_detector_status 不在 schema | PR-FIX-08 | ✅ | `f754d014` |
| G-09 | HTML 過濾未實作 | PR-FIX-06 | ✅ audit 誤判（regex 早實作） | — |
| G-10 | Backfill 標記缺 | PR-FIX-06 | ⏸️ IsBackfill 加, predictor 待做 | `1625cb1b` |
| G-11 | geopolitical 永遠 = 0 | PR-FIX-07 | ⏸️ code correct, runtime RSS/GDELT | — |
| G-12 | capital_flow change_pct = 0 | PR-FIX-07 | ✅ | `e7f25dd7` |
| G-13 | narrative_tilt 沒 test | PR-FIX-09 | ✅ | `da4508cd` |

**Summary**：6 個完全修復、6 個 partial、1 個 audit 誤判。

## 後續追蹤（partial 的 6 個）

1. **G-02/G-04**：跑 `cmd/atlas-stage4-loader --drop-synthetic` 從 staging JSONL 回填
2. **G-05**：建立 `MacroEventType` enum（FOMC/BOJ/OPEC/CPI）→ 24 templates 全覆蓋
3. **G-07**：`dashboard.GetLatestMacroSnapshot()` + 構造 `MarketNarrativeData` 並 wire 到 `cmd/atlas/main.go:892`
4. **G-10**：`eventdriven.Predict` 內讀 `r.IsBackfill` 對 `BaseWeight` 套 0.7 折扣
5. **G-11**：staging 環境驗證 RSS / GDELT feed 通暢

## 10 PR 完成清單

| PR | Branch | Commit |
|----|--------|--------|
| PR-FIX-01 | fix/ledger-historical-cleanup | 6 commits |
| PR-FIX-02 | fix/scheduler-get-status-decode | `d1715121` |
| PR-FIX-03 | feat/event-type-theme-mapping-expansion | `f58f6e50` |
| PR-FIX-04 | feat/narrative-tilt-24-themes | `23adcd28` |
| PR-FIX-05 | feat/detector-input-injection | `8de1c3f3` |
| PR-FIX-06 | feat/event-sanitizer-html-backfill | `1625cb1b` |
| PR-FIX-07 | fix/data-anomalies-investigation | `e7f25dd7` |
| PR-FIX-08 | feat/template-detector-mcp-tool | `f754d014` |
| PR-FIX-09 | test/narrative-tilt-coverage | `da4508cd` |
| PR-FIX-10 | docs/audit-followup-cleanup | (this commit) |
