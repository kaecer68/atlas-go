# Audit Manifest: Hermes macro/stress integration feedback (2026-07-22)

> **Audit source**: External Hermes agent feedback on atlas-mcp integration (`macro_ingest`, stress-index, regime-history endpoints).  
> **Goal**: Confirm which items are project bugs vs. Hermes misconceptions, fix the bugs, and document the clarifications needed for Hermes.  
> **Scope**: `internal/monitoring`, `internal/ledger`, `internal/narrative` paths touched by live macro ingestion and the Taiwan stress index / regime history endpoints. Out of scope: redesigning the two-vocabulary regime model, changing the 5-minute stress calculator cache, or reworking the geopolitical RSS providers.  
> **Created**: 2026-07-22  
> **Status**: completed

---

## Invariant Tracker

| ID | Problem | Root Cause | Files Changed | Acceptance Criteria | Status | Notes |
|----|---------|------------|---------------|---------------------|--------|-------|
| E5 | `/api/taiwan/stress-index` `geopolitical` component can be `0` while `geopolitical_history` has a non-zero intensity for the same date | `DashboardAPI` fetched geo only from the live provider; the on-demand endpoint had no fallback. Macro-ingest and on-demand used different resolution paths. | `internal/monitoring/dashboard_api.go` (new `resolveGeoScore` with live → SQLite → file-store fallback chain; `persistGeopolitical` now also mirrors to the file store); `internal/monitoring/service/macro.go` (on-demand `CalculateStressIndex` uses the same `resolveGeoScore` helper) | Both macro-ingest and on-demand stress index use the same geo resolution; when live fetch fails, the latest non-zero SQLite or file-store intensity is used; `geopolitical` component is no longer decoupled from `geopolitical_history` | completed | Added `TestDashboardAPI_ResolveGeoScore_FallsBackToHistoricalStore` and `FallsBackToFileStore` |
| E6 | `/api/regime/history?days=5` and `/api/narrative/stress-index/history?days=5` interpreted `days=N` differently (row-limit vs. calendar window) | `LoadRegimeHistory` used `LIMIT ?` while `GetStressIndexHistory` used date filtering; `regime_history` had synthetic daily rows so the two happened to look similar, but semantics diverged. | `internal/monitoring/service/narrative.go` (date-window filtering for stress & geo history); `internal/monitoring/service/pipeline.go` (new `LoadRegimeHistoryDays` + `buildRegimeHistoryData` helper); `internal/monitoring/api/pipeline/handlers.go` (`days` parameter now maps to calendar window, `limit` keeps row-limit semantics); `RegimeSessionEntry` gained `date` field | `?days=5` on all three history endpoints returns rows whose `date` is within the last 5 calendar days; `?limit=5` still returns up to 5 rows; response includes `date` on every session; existing and new tests pass | completed | Added `TestLoadRegimeHistoryDays_CalendarWindow` and updated stress/geo history tests to use relative dates |
| E8 | Live macro ingestion pipeline did not write `regime_history` | `DashboardAPI.applyMacroUpdate` persisted stress and geo but not a canonical regime row. | `internal/monitoring/dashboard_api.go` (new `persistRegimeHistory` derived from current stress index via `narrative.NormalizeRegime`) | After a live macro tick, `regime_history` has a `source=macro_ingest` row for the same date; existing synthetic backfill rows remain untouched; `UpsertRegime` is idempotent | completed | Added `TestDashboardAPI_PersistRegimeHistory_HappyPath` |
| E1-residual | Hermes sees `geopolitical=5.07` in stress components while `geopolitical_history` shows `intensity=39` | Expected behavior: the stress component is `intensity × scaleGeo(=1) × weightGeo(=0.13)` = 5.07. The two numbers measure different things (raw intensity vs. weighted contribution). | None | N/A — document as Hermes misconception | completed | Add to reply prompt |
| E7 | Hermes thought the 5-minute macro ingest schedule was not running because the latest row was old and `atlas-cron-macro-ingest` logs show a daily cron | The 5-minute live cadence is `atlas-go`’s internal scheduler task named `macro_ingest`, not the `atlas-cron-macro-ingest` container. Gaps are expected when upstream fetches fail or overlap protection skips a tick. | None | N/A — document as Hermes misconception | completed | Add to reply prompt |

---

## Phase Tracker

### Phase A — Audit (read-only)

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Reproduce the symptom | E5, E6, E8 | done | SQLite query showed `stress_index_history.geopolitical=0` for 2026-07-21 while `geopolitical_history.intensity=39`; `?days=5` returned different row sets; `regime_history` had no `source=macro_ingest` rows |
| Identify suspect code | E5, E6, E8 | done | `dashboard_api.go:fetchGeoScore`/`applyMacroUpdate`/`persistGeopolitical`; `narrative.go:GetStressIndexHistory`/`GetGeopoliticalHistory`; `pipeline.go:loadRegimeHistoryFromStore`; `dashboard_api.go` lacked regime writer |
| Form root cause hypothesis | E5, E6, E8 | done | See Invariant Tracker |
| Validate hypothesis with evidence | E5, E6, E8 | done | Live data + code read + scheduler status `/api/scheduler/status` shows `macro_ingest` running every 5 min |

### Phase B — Plan

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Map each ID to files + changes | all | done | See Invariant Tracker |
| Define acceptance criteria | all | done | See Invariant Tracker |
| Review blast radius | E5, E6, E8 | done | Changes are additive or behind existing handlers; `date` field addition is backward-compatible; `limit` semantics unchanged |

### Phase C — Implement

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Unify geo fetch + persist geo to file store | E5 | done | `dashboard_api.go` `resolveGeoScore`; `macro.go` `resolveGeoScore`; `persistGeopolitical` mirrors to file store |
| Date-window semantics for stress/geo/regime history | E6 | done | `narrative.go` helpers `dateNDaysAgo`/`filterStressRowsByMinDate`; `pipeline.go` `LoadRegimeHistoryDays`; `handlers.go` separates `limit` and `days` |
| Add live regime writer from stress index | E8 | done | `dashboard_api.go` `persistRegimeHistory` |
| Update tests | E5, E6, E8 | done | `go test ./internal/monitoring/... ./internal/ledger/... ./internal/narrative/... ./internal/scheduler/...` passes |

### Phase D — Close Out

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Update manifest status | all | done | This file |
| Run CI / verify | all | done | `go test` green; `gofmt -l .` clean; `make check-binaries` aligned |
| Reply prompt for Hermes misconceptions | E1-residual, E7 | done | See Session-End State below |

---

## Backlog

| ID | Problem | Discovery Time | Proposed Round |
|----|---------|---------------|----------------|
| B01 | `TaiwanStressCalculator` 5-minute cache can keep `/api/taiwan/stress-index` stale after a macro tick writes a new geo score | 2026-07-22 | Future tuning if dashboard freshness becomes an issue; out of scope for this fix |
| B02 | PR #1252 manifest `.omo/manifests/2026-07-21-stress-history-and-regime-gaps.md` (original filename was renamed by the docs governance overhaul); file tracking gap noted by Hermes | 2026-07-22 | Ops / docs governance follow-up, not a code defect |

---

## Commit Discipline

- Format: `<type>(manifest): #<ID> <short description>`
- One commit per ID
- No commit without acceptance criteria passing
- PR body must reference this manifest: `See docs/manifests/2026-07-22-hermes-macro-stress-audit.md`

---

## Session-End State

- **Done this session**: E5, E6, E8 fixed and tested; manifest updated to completed; reply prompt drafted below.
- **Remaining**: None. E1-residual and E7 are Hermes misconceptions to be communicated.
- **Next action**: User replies to Hermes with the prompt below.
- **Uncommitted code**: yes — changes staged in working tree
- **Branch / PR**: to be created by user / ship workflow
- **Paused because**: -

### Reply Prompt for Hermes

> 感謝你仔細比對。本次盤查後結論如下：
>
> **已修復的 project bug（3 項）**
> 1. **E5 geo drift**：`DashboardAPI` 現在有一條統一的 `resolveGeoScore` fallback 鏈：live provider → SQLite `geopolitical_history` → on-disk `data/state/geopolitical/latest.json`。macro-ingest 與 on-demand `/api/taiwan/stress-index` 都走同一條鏈，因此兩者不會再出現一邊有 39、一邊是 0 的 drift。`persistGeopolitical` 也會同時 mirror 到 file store，讓 fallback 來源一致。
> 2. **E6 `days=N` 語意不一致**：`?days=N` 在 `/api/regime/history`、`/api/narrative/stress-index/history`、`/api/geopolitical/history` 現在都統一為「最近 N 個 calendar days」的語意；row-limit 語意保留給 `?limit=N`。`RegimeSessionEntry` 也補上 `date` 欄位，client 不再需要從 `recorded_at` 推日期。
> 3. **E8 live pipeline 不寫 regime_history**：`applyMacroUpdate` 現在會呼叫 `persistRegimeHistory`，把當前 stress index 經 `narrative.NormalizeRegime` 轉成 canonical regime 後寫入 `regime_history`，`source=macro_ingest`。注意 stress↔regime 的對位是近似的，原因已記錄在 `/api/narrative/regime-mapping`。
>
> **你誤解的兩件事（不是 bug）**
> 1. **E1-residual**：`geopolitical=5.07` 與 `geopolitical_history.intensity=39` 不一致是**預期行為**。stress component 是 `intensity × scaleGeo × weightGeo`（目前 weightGeo ≈ 0.13），所以 39 × 0.13 ≈ 5.07。兩個數字一個是 raw intensity，一個是加權後對 stress index 的貢獻，不該相等。
> 2. **E7 5min 排程**：`macro_ingest` 的 5 分鐘 cadence 是 **atlas-go 內部 scheduler task**（`cmd/atlas/operations_tasks.go`），不是 `atlas-cron-macro-ingest` 容器。`atlas-cron-macro-ingest` 是每日一次的 cron 容器。`stress_index_history` 的 captured_at 會在每次 tick 都更新（即使上游失敗也會走 fallback 路徑），但 **新日期的新 row** 只會在上游成功並產生新的 `RecordedAt` 時出現。因此「最近一筆是 1 小時前」不等於排程沒跑；請用 `/api/scheduler/status` 確認 `macro_ingest` 的 `last_run` 與 `next_run`。
>
> **建議你下次查核時直接看的證據**
> - `/api/scheduler/status` 搜 `macro_ingest`：interval 應為 300000000000 ns，last_run 在最近 5 分鐘內。
> - `sqlite3 data/state/atlas.db "SELECT date, source, captured_at FROM regime_history ORDER BY date DESC LIMIT 5;"` 看有沒有 `source=macro_ingest` 的當日/近日 row。
> - `/api/regime/history?days=5` 現在回傳的是最近 5 個日曆日的 regime；如果還沒有 live row，會回空陣列（等第一次 macro_ingest 成功後就會填入）。若要看 backfill 的 90 筆，用 `/api/regime/history?limit=90`。

---

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-07-22 | 1.0 | Initial manifest | AI |
| 2026-07-22 | 2.0 | E5/E6/E8 implemented; status → completed; added reply prompt | AI |
