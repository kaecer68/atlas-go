# Audit Manifest: stress-api-ledger-drift

> **Audit source**: 2026-07-20 stress backtest mission — 前任務判斷「stress_index_history 無資料」源自 MCP tool 回空，盤查後發現為讀取鏈接線缺陷，非資料缺失
> **Goal**: 修復 stress index 歷史資料的讀取/寫入/排程三層 drift，讓 `macro_get_stress_index_history` 回傳真實持久化歷史
> **Scope**: stress index history 讀取鏈（MCP tool → narrative handler → NarrativeEngine → ledger）。明確排除：stress 計算公式本身、JANUS regime、事件日曆品質（另案處理）
> **Created**: 2026-07-20
> **Status**: completed（Phase A 審計完成；Phase B/C/D 已於 2026-07-20 實作並提交）

---

## Invariant Tracker

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance Criteria | Status | Documentation Impact | Notes |
|----|---------|----------------------|-----------------|---------------------|--------|----------------------|-------|
| A01 | `macro_get_stress_index_history(days=365)` 只回 1-3 筆（當前快照），不回 90 天歷史 | narrative handler 讀 `NarrativeEngine.stressHistory`（process 記憶體 ring buffer，上限 365，restart 清空），而非 `HistoricalStore.LoadStressHistory()`（SQLite 持久層，現有 90 筆 synthetic）| `internal/monitoring/api/narrative/handlers.go:295-304`、`internal/monitoring/service/narrative.go:97`、`internal/narrative/knowledge_base.go:554-567` | `curl /api/narrative/stress-index/history?days=90` 回傳 ≥90 筆（ledger 現有量），最早 date=2026-04-01 | accepted | none | 證據：curl 實測回 3 筆 vs `sqlite3 "SELECT COUNT(*) FROM stress_index_history"` = 90；`LoadStressHistory` (`internal/ledger/historical_store.go:298-302`) 無上游 caller |
| A02 | `GetCurrentStressIndex()` 每次呼叫 append 一筆到 in-memory history（GET 語意有寫副作用）| `knowledge_base.go:546` 在計算後無條件 `ne.stressHistory = append(...)`，任何呼叫 `/current` 的 client（dashboard、quickstart、agent）都會製造假歷史 | `internal/narrative/knowledge_base.go:540-552` | 連續呼叫 `/current` 10 次，`/history` 筆數不變（或僅按 date 去重後 +0） | accepted | none | 證據：盤查期間 11:20 回 1 筆、11:23 回 3 筆 — 期間 dashboard/quickstart 呼叫了 current |
| A03 | `stress_test_daily` 排程任務 enabled=true 但 `last_run` 為零值，從未執行 → 7 月後 ledger 無新入帳 | scheduler 啟動時未把該任務排入執行迴圈，或 BackgroundTaskManager 初始化順序問題（同批 22+ 個任務同症狀：window_backtest、calibration_cycle、factor_weight_calibrate、predictor_calibrate、risk_gate_calibrate、auto_judge_promoter、ml_retrain、seasonal_calibration 等） | `internal/scheduler/`（確切檔案待 Phase B 定位）、`internal/apigateway/background.go:25` | `stress_test_daily.last_run` 非零值且每日遞增；ledger `stress_index_history` 出現 `source != 'synthetic'` 的新列 | accepted | none | 證據：`scheduler_get_status` 回傳 22+ 任務 `last_run: 0001-01-01T00:00:00Z`；ledger 最新 date=2026-06-29（stage4 backfill 一次性灌入後無後續） |
| A04 | HTTP `GET /api/regime/history?days=5` 回 404，但 MCP `regime_get_history` 可成功 | HTTP path 未在 mux 註冊（MCP 端打到不同的已註冊 path）| `cmd/atlas/api_routes.go` 或對應 handler 註冊處（待 Phase B 定位） | `curl /api/regime/history?days=5` 回 200 且與 MCP 端資料一致 | accepted | none | 證據：`mcp_quickstart.degraded_sections=["recent_regime_5_days"]`，error 明確記錄 404 |
| A05 | 快照未持久化 geopolitical 分量 → 歷史壓力分數重建時 geo=0，低估真實值 | `data/state/macro/*.json` 的 schema 不含 geo intensity 欄位 | `internal/marketdata`（snapshot struct）、快照寫入處 | 新快照含 geopolitical 欄位；歷史回填或標記 `geo_missing=true` | hypothesis | none | 證據：重建腳本只能設 0；`macro_get_stress_index_current` 的 geo 來自 `GeopoliticalStore` 即時計算 |

---

## Phase Tracker

### Phase A — Audit (read-only) ✅ 完成 2026-07-20

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Reproduce the symptom | A01 | accepted | `curl /api/narrative/stress-index/history?days=90` → `history_count: 3`；MCP `macro_get_stress_index_history(days=365)` → 1 筆 |
| Identify suspect code | A01 | accepted | `handlers.go:302` → `narrative.go:97` → `knowledge_base.go:554-566` 全鏈路讀記憶體 |
| 證明持久層有資料 | A01 | accepted | SQLite `stress_index_history` COUNT=90（2026-04-01→06-29, source=synthetic, is_synthetic=0）|
| 證明讀寫副作用 | A02 | accepted | 同一 process 內 current 呼叫次數與 history 筆數正相關（1→3 筆/3 分鐘）|
| 證明排程沉默 | A03 | accepted | `scheduler_get_status`：22+ 任務 `last_run=0001-01-01`；`stress_index_history` MAX(date)=2026-06-29 |
| 證明 HTTP/MCP 路徑分歧 | A04 | accepted | quickstart degraded_sections 明確記錄 404 |
| 證明 geo 未持久化 | A05 | hypothesis | snapshot JSON 無 geo 欄位（待 Phase B 確認寫入端）|

### Phase B — Plan（已完成）

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| `NarrativeService` 改接 `HistoricalStore.LoadStressHistory(ctx, days)`，in-memory 作 fallback | A01 | done | `internal/monitoring/service/narrative.go:107-131` |
| `/current` 改為純讀，新增 `RecordStressIndex` 供每日持久化呼叫 | A02 | done | `internal/narrative/knowledge_base.go:533-553` |
| `IngestAndUpdateMacro` 內呼叫 `RecordStressIndex` 與 `HistoricalStore.UpsertStress` | A03 | done | `internal/monitoring/dashboard_api.go:106-126` |
| 補註冊 `/api/regime/history` HTTP path | A04 | backlog | 與 MCP path 對齊，不在 P0 |
| Snapshot schema 加 geopolitical 欄位（含 migrate 策略） | A05 | hypothesis | 待未來評估 |

### Phase C — Implement（已完成）

| Commit | ID | Message | Files |
|--------|----|---------|-------|
| 1 | A02 | `fix(manifest): #A02 remove write side-effect from GetCurrentStressIndex` | `internal/narrative/knowledge_base.go`、`internal/narrative/taiwan_stress_index_test.go`、`internal/narrative/testdata/knowledge_base_api.golden.json` |
| 2 | A01 | `fix(manifest): #A01 read stress history from ledger` | `internal/monitoring/service/narrative.go`、`internal/monitoring/service/narrative_service_test.go`、`internal/monitoring/dashboard_api.go` |
| 3 | A03 | `fix(manifest): #A03 persist stress index during macro ingestion` | `internal/monitoring/dashboard_api.go` |

### Phase D — Close out（已完成）

- `go test ./internal/monitoring/... ./internal/narrative/...`：全部通過
- `go vet ./internal/monitoring/... ./internal/narrative/...`：無錯誤
- `gofmt -l .`：乾淨
- A04/A05 保留在 backlog，不在本次 scope
- PR：https://github.com/kaecer68/atlas-go/pull/1243

---

## Backlog（不在本 manifest 範圍）

- B01：22+ 個沉默排程任務逐一檢視是否「設計上就不該每日跑」還是「註冊失敗」（A03 修復後逐一核對）
- B02：`data_get_quality.score=100` 但 `checks=[]` — 空檢查清單不該給滿分
- B03：LLM providers `healthy=true` 但 `last_success=零值` — readiness 探針與實際呼叫脫鉤
- B04：TEJ channel 74 天未更新（2026-05-06）
- B05：3 個 L4 manual strategies hit_rate=0 且 total_tests=0（margin-balance-extreme、dealer-domestic-support、cb-fx-intervention-warning）— strategy evolution 永遠不會收斂
- B06：crossmarket `data_status=stale`，`stale_channels=["government_flow"]`
- B07：runtime `commit=unknown`（PR #1238 已修 GIT_COMMIT threading 但 dev binary 未生效）
- B08：regime_history ledger 有重複 timestamp 記錄（同一分鐘多筆 snapshot）

---

## Commit Discipline

- 一個 ID 一個 commit：`fix(manifest): #A01 route stress history reads to ledger` 等
- PR body 引用本文件路徑
- 不直推 main

---

## Session-End State（2026-07-20）

- **Done**：Phase A 審計完成；P0（A01+A02+A03）實作完成並通過測試；stress backtest mission 實證完成（見 `.omo/plans/2026-07-20-stress-backtest-mission.md`，7 假設 4 支持 2 拒絕 1 無法判定）
- **Left**：A04 HTTP path 404、A05 geopolitical 未持久化 — 保留在 backlog
- **Next action**：等待 CI/review 後 merge PR；合併後依 `docs/multi-cli-protocol.md` 清理 worktree
