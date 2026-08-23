# 績效報告單一真相源 (SSoT) 決策 — 2026-08-23

> **裁決**: **PG `session_summaries` + `recommendation_outcomes` 為績效報告 SSoT (主)**。
> JSONL 為導入來源 (寫入 PG 後完成使命) — 同 capital_flow SSoT 模式
> (docs/decisions/2026-08-18-capital-flow-ssot.md)。

## 背景 (為何需要 SSoT)

績效報告在 2026-07-30 → 08-21 被修 **8 次**，每次都是「新發現的獨立資料問題」：

| 修復 | 問題 |
|---|---|
| #1431 | PascalCase corrupted summary (JSON tag 錯 → 0 值污染報表) |
| BL-01 | SQLite 5 欄投影缺 PortfolioValue → equity 全 0 |
| B02b | outcome.Window 是 ISO 日期，被當 session ID 解析 → regime 錯配 |
| R01 | regime 歸屬誤用 window→summary 查找，而非 outcome 自身 regime |
| BL-06b | SQLite NULL 欄位 → scan 失敗 |
| A7 / 1634 | synthetic 交易混入 headline 統計、純 synthetic 樣本 |

**共同根因**：輸入資料來自多後端 (JSONL / SQLite / PG)，schema 與語意不統一。
每接一個源就爆一次。資本 flow 在 2026-08-18 拍板 PG 為 SSoT 後不再反覆；
績效報告沒有 SSoT，所以一直修。

## 現況 (2026-08-23)

| 面向 | 狀態 |
|---|---|
| PG `session_summaries` 表 | 欄位已與 domain `SessionSummary` 對齊 (含 risk_commentary / tax_snapshots / after_tax_pnl / total_tax_paid / parameters_version) — SSoT 基礎具備 |
| `RecommendationOutcome.Validate()` | 已有 (AgentID/Symbol/Side/Window/Conviction 必填) |
| `SessionSummary.Validate()` | 本決策新增：嚴格版 (寫入路徑) + `ValidateLegacy()` (backfill 路徑) |
| 讀取路徑 | `internal/reporting/performance.go` `LoadSessionSummaries()` / `LoadSessionOutcomes()` — 本決策統一為 PG 為主 |

## 決策內容

1. **SSoT = PG `session_summaries` + `recommendation_outcomes`** (2026-08-23 拍板)
2. **JSONL (data/state/sessions)** = 導入來源，非 SSoT — 歷史資料經
   `cmd/backfill-summaries` / `cmd/reconcile-sessions` 寫入 PG 後即完成使命
   (同 capital_flow 模式)
3. **寫入者責任 (雙寫)**:
   - 即時路徑 (sim) 仍雙寫 JSONL + PG (DualWriteRepository.RecordSessionSummary)
   - **寫入強制驗證**：`RecordSessionSummary` 在 dual_write / SQLite / JSONL / PG
     四個寫入點一律呼叫 `SessionSummary.Validate()` (嚴格)，corrupted 直接回
     error **不寫入** (SessionID 必填、PortfolioValue > 0、EndingCash ≥ 0、
     OutcomeCount ≥ 0、Regime 合法值)
   - **backfill 路徑** (reconcile-sessions / SaveSessionSummary) 用
     `ValidateLegacy()`：允許「合法 0」(PortfolioValue=0 且 EndingCash=0 的
     count-only 舊行，regime 可為空)，仍拒絕「corrupted 0」(PV=0 但
     EndingCash>0 或 OrderCount>0)、負現金、負計數、非法 regime
4. **讀取路徑統一 (PG 為主 + fallback + degraded)**:
   - `ledger.NewReportOutcomeStore(cfg)`：postgres backend 包
     `PGFirstOutcomeStore` — 讀取一律先走 PG
   - **fallback 語義**：僅在 **PG 不可用 (error)** 時退回 JSONL，並標記
     **degraded** (report 輸出 `source:"jsonl"` + `degraded:true`，markdown 標註
     ⚠️ Degraded source)
   - **PG 可用但 empty 仍是權威**：不靜默混入 JSONL (那正是 8 次修復的根因)；
     資料缺口由 reconcile-sessions / backfill 處理，不由報表偷偷補
   - 其他 backend (sqlite/jsonl) 維持原語意

## 影響

- 未來績效報告相關修復/對帳都以 PG 為基準
- 新寫入的 summary 必須過嚴格 Validate；corrupted 資料無法再靜默進入任一後端
- 報表輸出增加 `source` / `degraded` 欄位 (omitempty)，degraded 時 markdown
  有明確告警
- 端到端 golden test (internal/reporting/golden_test.go) 鎖定報表輸出契約，
  任何語意改動必須 `-update` 並經 PR 審查 — 治「反覆修補」根因的護欄

## 執行

- 本文件為正式決策記錄；績效報告相關修復以 PG 為基準
- 若未來 PG 不可用 (fallback 情境)，讀取退回 JSONL 但標記 degraded
- 本決策覆蓋先前「多後端各自為政」的讀取設計 — 用戶裁決優先
