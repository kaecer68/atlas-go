# Audit Manifest: 重構斷裂修復 — evolution panel / industry 資料斷線

> **Audit source**: 使用者 (2026-07-26) — 策略演化頁面 Top 5/淘汰候選空洞 + 產業地圖「尚無產業資料」
> **Goal**: 修復三條重構期間的資料路徑斷裂，恢復前端正常顯示
> **Scope**: `internal/ledger/ledger.go` + `cmd/atlas/main.go` + `shared_web/static/js/pages/industry.js`
> **Created**: 2026-07-26
> **Status**: in-progress

---

## 證據蒐集

### 斷裂 1: LoadOutcomesFromSessions 路徑未對齊 sessions/ 子目錄

- **檔案**: `internal/ledger/ledger.go:127`
- **根因**: PR #532 (`07a3f85f`) 引入 `sessions/` 子目錄時，所有 RecordSession*/LoadSessionOutcomes/LoadSessionSummaries 都用了 `sessionDir()` 或 `filepath.Join(s.baseDir, "sessions")`，唯獨 `LoadOutcomesFromSessions()` 直接 `os.ReadDir(s.baseDir)`
- **證據**: `data/state/sessions/session-20260720-daily/recommendation_outcomes.jsonl` 有 27 筆 outcomes；API `/api/dashboard/agent-observatory` 回 `scorecards: []`
- **影響**: 策略演化 compact view → Agent Top 5 + 淘汰候選 永遠 empty state

### 斷裂 2: Sector allocation writer/reader 雙路徑漂移

- **Writer**: `cmd/atlas/main.go:305` → `FileClosureStore` at `sector/allocation/`
- **Reader**: `cmd/atlas/main.go:465` → `SnapshotReader` at `data/state/`
- **根因**: SA08 建構時 writer 和 reader 用了不同路徑
- **證據**: docker container 內 `data/sector/allocation/` 目錄不存在；`data/state/closure-policy.jsonl` 不存在
- **影響**: API `/api/dashboard/sector-allocation-plan` 永遠 `fallback_reason: "no_simulation_session"`

### 斷裂 3: Frontend renderIndustryMap 檢查舊 API shape

- **檔案**: `shared_web/static/js/pages/industry.js:134`
- **根因**: SA08/SA09 將 API 改為 `SectorAllocationSnapshot`（`target/current/delta`），frontend 仍檢查 `data.industries`
- **證據**: `!data.industries` → 直接 show empty state
- **影響**: 即使修復斷裂 2（API 有資料），frontend 仍無法渲染

---

## Invariant Tracker

| ID | 問題 | 根因 | 變更檔案 | 驗收 | 狀態 |
|----|------|------|---------|------|------|
| F1 | LoadOutcomesFromSessions 路徑 | `os.ReadDir(s.baseDir)` 缺 `sessions/` | `internal/ledger/ledger.go` | curl agent-observatory scorecards > 0 | pending |
| F2 | Sector alloc path drift | writer `sector/allocation/` vs reader `data/state/` | `cmd/atlas/main.go` | curl sector-allocation-plan non-fallback | pending |
| F3 | Frontend shape mismatch | `data.industries` → 應讀 `target/current/delta` | `shared_web/static/js/pages/industry.js` | 瀏覽器產業地圖顯示權重條 | pending |

---

## Phase Tracker

### Phase A — Audit ✅
- F1: 重現 + 定位 ✅
- F2: 重現 + 定位 ✅
- F3: 重現 + 定位 ✅

### Phase B — Plan
- F1: 1 行改 `s.baseDir` → `filepath.Join(s.baseDir, "sessions")`
- F2: 1 行改 reader 路徑為 `sector/allocation/`
- F3: 改 `renderIndustryMap()` 從 `data.target` 重建 industries 陣列

### Phase C — Implement
- [ ] F1: fix ledger.go LoadOutcomesFromSessions
- [ ] F2: fix main.go reader path
- [ ] F3: fix industry.js renderIndustryMap shape
- [ ] 重建 container + 瀏覽器驗證
- [ ] ci-gate + check-binaries

### Phase D — Verify
- [ ] curl agent-observatory scorecards > 0
- [ ] curl sector-allocation-plan 有 target/current/delta 數值
- [ ] 瀏覽器: evolution panel Top 5 + 淘汰候選 有 agent 名稱
- [ ] 瀏覽器: 產業地圖 顯示產業權重條而非「尚無產業資料」
- [ ] ci-gate 通過
