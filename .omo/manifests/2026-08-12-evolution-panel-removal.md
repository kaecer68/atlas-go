# 2026-08-12 移除策略演化頁面 + 後端 outcomes 截斷修復

## Goal

1. 刪除 `/admin/evolution_panel` 與 `/client/evolution_panel` 兩個前端頁面（共用同一後端 API）。
2. 後端不刪除，僅盤查並修復「數據不是 100 就是 0」的根因。

## 根因盤查結論（代碼為真相）

**前端顯示 0 的根因**：`agent-observatory` API 的 scorecards 全 0（5 agent、hit_rate=0、sharpe=0）。

完整鏈：

| 層級 | 事實 | 證據 |
|---|---|---|
| 寫入端 | `SQLiteOutcomeStore.RecordOutcomes/RecordSessionOutcomes` 的 INSERT 只存 12 欄，**不含 Hit、ForwardReturn、Window、Skill、BenchmarkDelta**；且把 `Layer` 存入 `regime` 欄位（語義錯置） | `internal/ledger/outcome_store_sqlite.go:36-44` |
| 儲存端 | SQLite `outcomes` 表 schema 無評估欄位；全域記錄 2557 筆（5 agent） | `sqlite3 atlas.db ".schema outcomes"` |
| 讀取端 | `SQLiteOutcomeStore.LoadOutcomesFromSessions` SELECT 只有 12 欄 → scanOutcomes 產生 Hit=false、ForwardReturn=0、Window="" | `outcome_store_sqlite.go:146-155` |
| 消費者 | `LoadAgentObservatory`（無 sessionID）→ `store.LoadOutcomesFromSessions()` → `BuildScorecards` → hit_rate=0、sharpe=0 | `agent_observatory.go:37` |
| 環境 | docker `.env` 設 `ATLAS_STORE_BACKEND=sqlite` → `NewOutcomeStore` 回 SQLiteOutcomeStore（截斷版）；`a.repo` 未注入 → 不走 DualWrite（JSONL rich） | `~/.config/atlas-go/.env`、`dashboard_api.go:940-952` |
| 對照 | **rich 資料存在 per-session JSONL**（4164 筆、20 agent、hit_rate 0.25~0.74、forward_nonzero 近全數）；`ledger.Store.LoadOutcomesFromSessions`（JSONL 版）是完整實作 | `data/state/sessions/*/recommendation_outcomes.jsonl` |

**「100」的來源**：前端 `renderTrendSummary` 的 Regime 穩定度（transitions=0 時 = 100%）與 hit_rate 顯示 0% —— 皆由上述資料缺損導致。

## Invariants

| ID | Problem | Root cause | Files | Acceptance | Status |
|---|---|---|---|---|---|
| F01 | client evolution_panel 頁面存在 | 已退役頁面（admin 亦有） | client_web index.html/main.js/event-listeners.js/page-shells | /client/evolution_panel → 404、sidebar 無入口 | ✅ 完成 |
| F02 | admin evolution_panel 頁面存在 | 同上 | admin_web index.html/main.js/event-listeners.js/smoke | /admin/evolution_panel 導回首頁（admin router 無 404）、sidebar 無入口 | ✅ 完成 |
| F03 | shared evolution 檔案/css 殘留 | 同頁面資源 | shared_web pages/evolution_panel.js、page-shells/evolution_panel.js、css/pages/evolution-panel.css | 檔案刪除、main.css import 移除 | ✅ 完成 |
| F04 | 交叉 link 導向已刪頁面 | footer/narrative/portfolio 引用 | trust-footer.js、narrative.js、portfolio.js | 改導 strategies 或移除 | ✅ 完成 |
| B01 | agent-observatory scorecards 全 0 | SQLiteOutcomeStore 截斷評估欄位 | outcome_store_sqlite.go | SQLite backend 下 LoadOutcomesFromSessions/LoadSessionOutcomes 回傳含 Hit/ForwardReturn 的 rich 資料（委派 JSONL） | ✅ 完成（實機驗證 hit_rate 0.25~0.68） |

## Phase tracker

- Audit（done）：根因鏈確認（見上表）
- F01-F04：前端刪除
- B01：後端修復
- Verification：build、單元測試、實機 curl、ci-gate/ci-full

## Backlog

- `RecordSessionOutcomes` 把 `Layer` 存進 `regime` 欄位（語義錯置）—— 資料遷移風險，本次不動（SQLite 表僅作 recommendation 記錄用途）
- SQLite 全域記錄（session_id=''，2557 筆）無評估欄位 —— rich 資料以 JSONL 為唯一來源，不回填

## Commit discipline

- `refactor(client): 移除策略演化頁面（admin+client）` — F01-F04
- `fix(ledger): SQLiteOutcomeStore 委派 JSONL rich 讀取` — B01
