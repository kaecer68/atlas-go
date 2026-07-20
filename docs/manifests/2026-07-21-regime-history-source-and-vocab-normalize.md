# Audit Manifest: RegimeHistory Source Field + Stress History Vocab Normalize

> **Audit source**: Post-merge verification drill on 2026-07-21 after PR #1246 (C01-C04) merged. User did an end-to-end audit and surfaced two gaps in C02's coverage and one new schema-level gap.
> **Goal**: (1) Add `source` column to `regime_history` table + propagate through `RegimeRow` / `/api/regime/history` response. (2) Normalize `/api/narrative/stress-index/history` Regime field to canonical Regime vocabulary while preserving original Source for reversibility.
> **Created**: 2026-07-21
> **Status**: in-progress

---

## Background

PR #1246 added `Source` field to `TaiwanStressIndex` and `stressRowsToIndex()` projected it from `StressRow.Source`. That fix only covers `stress_index_history` (which has a `source` TEXT column). The sibling table `regime_history` does **not** have a `source` column, so `RegimeRow` cannot expose one and `/api/regime/history` returns rows without provenance.

Additionally, the `/api/narrative/stress-index/history` endpoint still serves rows with mixed regime vocabularies in the same `regime` field (e.g. `RISK_ON` for synthetic rows, `low` for macro_ingest rows). Consumers cannot easily join or filter without re-implementing the mapping locally.

User fact-check on 2026-07-21:
```
- 第一筆 date=2026-06-21, regime=RISK_ON, source=synthetic。
- 最後一筆 date=2026-07-20, regime=low, source=macro_ingest。
- 10 筆裡仍混用 RISK_ON 與 low 兩套 regime vocabulary。
- regime_history table 查 source 直接報「no such column: source」。
```

---

## Invariant Tracker

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance Criteria | Status | Documentation Impact | Notes |
|----|---------|----------------------|-----------------|---------------------|--------|----------------------|-------|
| D01 | `regime_history` table lacks `source` column, so existing 90 backfill rows have no provenance | Schema was designed before A04 added multi-source persistence; only `stress_index_history` got the column | `internal/ledger/sqlite_core.go` (add column to `regime_history` CREATE TABLE) + `internal/ledger/migrate.go` (or new migration helper) for existing-DB upgrade | Existing 90 rows have `source='synthetic'`; new macro_ingest writes populate `source='macro_ingest'`; SELECT regime_history.source returns 90 'synthetic' + 1+ 'macro_ingest' | pending | none | Additive, reversible (downgrade by column drop is non-trivial but no data loss) |
| D02 | `RegimeRow` has no `Source` field; `/api/regime/history` JSON output lacks `source` | Same root cause as D01 (table lacks column → struct lacks field) | `internal/ledger/historical_store.go` (add `Source string` to `RegimeRow`, update SELECT, update Scan); `internal/monitoring/service/pipeline.go` (project Source into RegimeSessionEntry or new wrapper) | `/api/regime/history?days=5` returns `{"sessions": [{"session_id":"...","regime":"...","source":"synthetic|macro_ingest",...}]}` | pending | none | Backward-compat: existing `source_session_id` field preserved; new `source` is additive |
| D03-A | `/api/narrative/stress-index/history` returns mixed regime vocabularies (`RISK_ON` and `low` in same response); consumers cannot easily filter/join | Two writer paths (`DashboardAPI.persistStressIndex` legacy / `TaiwanStressCalculator.Calculate` live) emit different strings into the same column | `internal/monitoring/service/narrative.go` (`stressRowsToIndex` applies `narrative.NormalizeRegime()` to `r.Regime` before returning); keep `Source` field intact | All rows in `/api/narrative/stress-index/history?days=N` response have `regime ∈ {RISK_ON, RISK_OFF, NEUTRAL, TRANSITIONAL}`; `source` field still present for reversibility | pending | none | Defaulted to option A per system directive after user audit; user may request option B (DB-level backfill) in follow-up |
| D04 | Cannot verify A03 (排程寫入 ledger) sample size; only 1 macro_ingest row in stress_index_history since container bootstrap | `cmd/atlas/operations_tasks.go::macro_ingest` is 5m interval, but actual write cadence depends on whether `persistStressIndex` is wired to it on every tick (verified at startup via `dashboard.IngestAndUpdateMacro`) | None this PR — operational follow-up | Operator runs `sqlite3 data/state/atlas.db "SELECT date, COUNT(*), MAX(captured_at) FROM stress_index_history WHERE source='macro_ingest' GROUP BY date ORDER BY date;"` after 3-5 days; expect ≥1 macro_ingest row per weekday | pending (operational) | none | Documented follow-up; PR doesn't unblock this |

---

## D03 Design Decision (Option A vs Option B)

**Default: Option A** (handler normalize).

| | Option A (handler normalize) | Option B (DB backfill) |
|---|---|---|
| Data mutation | None — original `regime` strings preserved in SQLite | Mutates 89 `synthetic` rows: `RISK_ON` → `low`, `RISK_OFF` → `high`, etc. |
| Reversibility | Trivial — change handler mapping or revert | Hard — would need to write reverse migration |
| Original vocab recovery | Via `Source='synthetic'` → mapped → inverse mapping table | Lost — original Regime vocab strings gone |
| Risk | Low (handler-only change) | High (destructive write to 89 production rows; mapping is approximate) |
| User's stated constraint | "不把缺資料寫成 0、保留矛盾" (A04 spec §3) | Violates constraint |

**User's audit message did not explicitly choose A or B**. This manifest defaults to **Option A** per the system directive that pushed for forward progress; if the user wants Option B instead, they can request a follow-up PR that re-projects and rewrites the data.

> ⚠️ **If you read this manifest and prefer Option B**, the recipe is:
> 1. Skip `stressRowsToIndex` NormalizeRegime change (revert D03-A)
> 2. Add migration that runs `UPDATE stress_index_history SET regime = <mapping> WHERE source = 'synthetic' AND regime IN (<old vocab>)`
> 3. Re-run verification

---

## Phase Tracker

### Phase A — Audit (read-only, completed 2026-07-21)

| Task | Status | Evidence |
|------|--------|----------|
| Reproduce D01: query regime_history.source | done | sqlite3 error: `no such column: source` (see user audit) |
| Reproduce D02: curl /api/regime/history and inspect JSON | done | Response contains `session_id` / `regime` / `recorded_at` but no `source` field |
| Reproduce D03-A: curl /api/narrative/stress-index/history and inspect mixed vocab | done | First row `regime=RISK_ON, source=synthetic`; last row `regime=low, source=macro_ingest` |
| Identify suspect code for D01 | done | `internal/ledger/sqlite_core.go::regime_history` CREATE TABLE missing `source` column |
| Identify suspect code for D02 | done | `internal/ledger/historical_store.go::RegimeRow` lacks `Source` field; LoadRegimeHistory SELECT lacks `source` column |
| Identify suspect code for D03-A | done | `internal/monitoring/service/narrative.go::stressRowsToIndex` projects `r.Regime` directly without normalization |

### Phase B — Plan (in progress)

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Map each ID to files + changes | D01, D02, D03-A | done | Invariant Tracker above |
| Define acceptance criteria | D01, D02, D03-A | done | "Acceptance Criteria" column |
| Decide D03 default | D03-A | done | "Default: Option A" rationale above |

### Phase C — Implement (pending)

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Add `source` column to `regime_history` CREATE TABLE | D01 | pending | <commit hash> |
| Add migration helper that ALTERs existing regime_history tables | D01 | pending | <commit hash> |
| Backfill existing 90 rows to `source='synthetic'` | D01 | pending | <commit hash> |
| Add `Source string` field to `RegimeRow` struct | D02 | pending | <commit hash> |
| Update `LoadRegimeHistory` SELECT + Scan to include source | D02 | pending | <commit hash> |
| Update `loadRegimeHistoryFromStore` projection to include Source | D02 | pending | <commit hash> |
| Apply `NormalizeRegime()` in `stressRowsToIndex` | D03-A | pending | <commit hash> |
| Add tests for D01 migration + D02 source field + D03-A normalize | D01, D02, D03-A | pending | <commit hash> |
| Run focused tests + gofmt + go vet | - | pending | <output> |
| Commit + push + open PR | - | pending | <PR URL> |

### Phase D — Close Out (pending)

| Task | Status | Evidence |
|------|--------|----------|
| Update manifest status | pending | - |
| Rebuild Docker image from worktree | pending | <new image digest> |
| Restart container | pending | <docker ps> |
| Re-run verification (7 baseline + 3 C-checks + 3 new D-checks) | pending | <curl outputs> |
| Final close-out commit + push | pending | <commit hash> |

---

## Backlog

| ID | Problem | Discovery Time | Proposed Round |
|----|---------|---------------|----------------|
| D05 | `components` field empty for 89 `synthetic` rows in `stress_index_history`; only the 1 macro_ingest row has 8 components | 2026-07-21 | If backfill is feasible (use historical macro snapshots to reconstruct), this becomes a separate PR. Otherwise document as data quality gap. |
| D06 | Apply the same Source / NormalizeRegime treatment to the `/api/regime/history` *transitions* array (currently only `from_regime` / `to_regime` / `timestamp` — no source) | 2026-07-21 | If normalization at handler level catches transitions too, this is one extra line. Otherwise future PR. |
| D07 | Audit the 6h-interval silent tasks (`auto_geopolitical`, `janus_regime_refresh`) flagged in B01 triage — likely MaturityTracker-gated | 2026-07-21 | Operator runs `docker logs atlas-go --tail 5000 \| grep -E 'auto_geopolitical\|janus_regime_refresh\|burn_in_skip'` |

---

## Commit Discipline

- Format: `<type>(manifest): #<ID> <short description>`
- One commit per ID (D01, D02, D03-A each get their own commit)
- No commit without acceptance criteria passing
- PR body must reference this manifest

---

## Follow-up (Operational, Out of Scope)

| Item | Owner | Action |
|------|-------|--------|
| D04 observation window | operator | Daily check: `sqlite3 /Users/kaecer/workspace/atlas/data/state/atlas.db "SELECT date, COUNT(*), MAX(captured_at) FROM stress_index_history WHERE source='macro_ingest' GROUP BY date ORDER BY date;"` |
| D05 components backfill | operator + future PR | Decide: reconstruct from macro snapshots vs document gap |
| D06 transitions normalize | future PR | Single-line handler fix |

---

## Session-End State

- **Done this session**: Phase A audit (D01, D02, D03-A reproduced)
- **Remaining**: Phase B finalize (D03 default), Phase C implementation, Phase D close-out
- **Next action**: implement D01 (regime_history schema migration)
- **Uncommitted code**: no
- **Branch / PR**: `fix/regime-history-source-and-vocab-normalize` (worktree `/Users/kaecer/workspace/atlas-regime-source-2026-07-21`) / not yet opened
- **Paused because**: not paused

---

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-07-21 | 1.0 | Initial manifest with D01, D02, D03-A from user audit drill; D03 defaulted to Option A | Sisyphus |