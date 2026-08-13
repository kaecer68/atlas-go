# Audit Manifest: 績效報告市場狀態績效與最佳貢獻AI資料缺口

> **Audit source**: kaecer 回報 `/client/performance-report` 頁面「最佳貢獻AI」金融產業桌/礦業貴金屬 0%、「市場狀態績效」空頭/多頭資料異常 + 第三列「-」名稱缺口
> **Goal**: 溯源兩表資料缺口的根因，判定是否為實作不完全、資料源未接入，並修復計算層
> **Scope**: 前端兩表渲染正確性（已確認前端正確消費後端欄位）、`GenerateReport` 的 `calculateTopAgents` 與 `calculateRegimeBreakdown` 計算邏輯。不含底層 sim 為何缺 JSONL（另屬 BL-06/資料源問題）
> **Created**: 2026-08-13
> **Status**: in-progress

---

## Invariant Tracker

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance Criteria | Status | Documentation Impact | Notes |
|----|---------|----------------------|-----------------|---------------------|--------|----------------------|-------|
| R01 | 市場狀態績效「-」第三列 + 空頭/多頭報酬錯亂 | `calculateRegimeBreakdown` 用 `findRegimeForWindow(summaries, oc.Window)` 反查 regime，但 `oc.Window` 是評估視窗（7/14/7/16）非 session 日期 → 7/18(RISK_OFF) 誤歸 unknown；`oc.Regime` 欄位 JSONL 有值卻未使用 | `internal/reporting/performance.go` `calculateRegimeBreakdown` | 用 `oc.Regime` 重算：7/18 forward +0.0554 歸 RISK_OFF 而非 unknown；RISK_OFF avg ≠ 0 | accepted | none | 修復後 production 資料：RISK_OFF agg=+0.05545 avg=+0.79% win=100%、unknown 消失 |
| R02 | 金融產業桌/礦業貴金屬 aggregate=0/win=0/sharpe=null | 30d 內 7/25-8/5 等 session 無 JSONL（僅 SQLite outcomes，ForwardReturn=0）→ `LoadSessionOutcomes` fallback SQLite → forward=0 污染 | `internal/reporting/performance.go` `calculateTopAgents` | 排除 realTrades>0 但全 forward=0 的 agent（SQLite fallback 資料缺失特徵） | accepted | none | 修復後 top_agents 排除金融/礦業 0% 雜訊，顯示真實 agent |
| R03 | 前三名(ETF/價值/Ackman)報酬完全相同 0.01497 | ForwardReturn 是 symbol 未來報酬，多 agent 共享同一 symbol(00878.TW) 相同 forward → 非 bug，設計合理 | 無（非 bug） | 確認三 agent 的 passed 記錄為同一 symbol 00878.TW 相同 forward | accepted | none | 證據：JSONL 三 agent 同 00878.TW/0.0149755 |
| R04 | regime `unknown` session_count=0 但有 return/win_rate | regimeReturns 從 outcomes 建（無 session 關聯），regimeData 從 summaries 建；unknown 只由誤歸的 outcome 產生 → session_count 不增但 return 有值 | R01 修復後應消失 | R01 修復後無 unknown 列 | accepted | none | R01 修復後 unknown 列消失 |

---

## Phase Tracker

### Phase A — Audit (read-only)

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| 前端渲染確認（兩表正確消費後端欄位） | - | accepted | `performance-report.js:153-181` top_agents/regime_breakdown 正確渲染 |
| 定位 `calculateTopAgents` 根因 | R02 | accepted | `performance.go:461-560` 只統計 passed，SQLite fallback forward=0 |
| 定位 `calculateRegimeBreakdown` 根因 | R01 | accepted | `performance.go:619-672` 用 `findRegimeForWindow` 而非 `oc.Regime`；JSONL 7/18 regime=RISK_OFF/window=7/14 |
| 驗證前三名報酬相同 | R03 | accepted | 三 agent passed 記錄同 00878.TW/0.0149755 |
| 確認 production binary 為最新 | - | accepted | max_drawdown=0.0033/sharpe=-0.67（反映 9f7666f1 修復） |

### Phase B — Plan

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| R01: 改用 `oc.Regime` 歸屬 | R01 | pending | 見下 |
| R02: 確認資料源（不修計算） | R02 | pending | 資料源接入另案 |

### Phase C — Implement

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| `calculateRegimeBreakdown` 改用 `oc.Regime` | R01 | done | commit 見下 |
| `calculateTopAgents` 排除全 0 forward agent | R02 | done | commit 見下 |

### Phase D — Close Out

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Push branch / open PR | - | pending | - |
| Run CI / verify | - | pending | - |

---

## Backlog

| ID | Problem | Discovery Time | Proposed Round |
|----|---------|---------------|----------------|
| BL-06 | 7/22-8/13 多數 session 無 JSONL（僅 positions.json）→ forward 全 0 污染報告 | 2026-08-13 | 資料源接入審計 |
| BL-07 | outcome 未按 window 過濾（window 7/14 早於 30d cutoff 7/16 仍計入） | 2026-08-13 | 與 R01 同批或另案 |

> **Rule**: only move one backlog item into scope per session, and only after all current IDs are done or paused.

---

## Commit Discipline

- Format: `<type>(manifest): #<ID> <short description>`
- One commit per ID
- No commit without acceptance criteria passing
- PR body must reference this manifest: `See docs/manifests/2026-08-13-perf-report-regime-agent-audit.md`

---

## Session-End State

- **Done this session**: Phase A 根因確認（R01/R02/R03/R04）
- **Remaining**: R01 修復（改用 oc.Regime）、R02 資料源確認
- **Next action**: 實作 R01 修復 + 測試，驗證 regime 歸屬正確
- **Uncommitted code**: no（staged MCP 變更已 stash）
- **Branch / PR**: `fix/20260813-perf-report-regime-agent` / -
- **Paused because**: -

---

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-08-13 | 1.0 | Initial manifest | agent |
