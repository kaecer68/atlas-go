# Audit Manifest: Stress History Date Field, Regime Vocabulary Mapping, B01 Schedule Triage

> **Audit source**: Post-A04/A05/B01-B03 close-out drill on 2026-07-21. After PR #1245 merged + image rebuilt (`84d232953789`), end-to-end re-verification surfaced 4 gaps not covered by previous manifests.
> **Goal**: (1) Restore the `date` field to `/api/narrative/stress-index/history` so backtest tooling can map rows to trading dates. (2) Establish an explicit, documented mapping between the two regime vocabularies currently mixed in the same SQLite column. (3) Triage all silent scheduled tasks registered in `BackgroundTaskManager` into "designed-silent" vs "investigate" classes. (4) Defer the geopolitical_history 3-5 day accumulation observation to operational follow-up.
> **Scope**:
> - IN: `internal/narrative/taiwan_stress_index.go::TaiwanStressIndex` — add `Date` field
> - IN: `internal/monitoring/service/narrative.go::stressRowsToIndex` — populate `Date` from `r.Date`
> - IN: `internal/narrative/regime_mapping.go` (new) — explicit mapping table + test
> - IN: `internal/narrative/taiwan_stress_index.go::TaiwanStressIndex` — add `Source` field to disambiguate origin
> - IN: `internal/ledger/historical_store.go::StressRow.Source` — propagate source through read path
> - IN: New endpoint `/api/narrative/regime-mapping` to expose mapping to consumers
> - IN: B01 schedule triage table (no code changes; documentation only)
> - OUT: 3-5 day geopolitical_history accumulation observation window (deferred to operational follow-up)
> - OUT: Migration of legacy regime_history rows from mixed vocabulary to single vocabulary (future PR)
> **Created**: 2026-07-21
> **Status**: in-progress

---

## Background

After PR #1245 (B01-B03) merged and the container was rebuilt, the `/api/narrative/stress-index/history?days=90` response contains 90 entries but mixes two regime vocabularies in the same `regime` column:

```
2026-04-01  RISK_OFF    timestamp=1775109600, score=29.77   ← backfill era
2026-04-02  NEUTRAL     timestamp=1775196000, score=-3.91
...
2026-06-29  RISK_OFF    timestamp=1782712800, score=-25.15  ← backfill era
2026-07-20  low         timestamp=1784564881, score=16.06   ← live TaiwanStressCalculator
```

The `date` column is the SQLite `stress_index_history.date` PRIMARY KEY, but it is not exposed in the HTTP response — only `timestamp` (Unix epoch seconds). Downstream backtest tooling cannot join rows to trading dates without a separate lookup.

The `regime_history` and `stress_index_history` SQLite tables both have a `regime` TEXT column, but the strings written to it come from two unrelated code paths:

| Source | Vocabulary | Where defined |
|--------|-----------|---------------|
| Janus regime engine (multi-factor detector) | `RISK_ON` / `RISK_OFF` / `NEUTRAL` / `TRANSITIONAL` | `internal/domain/shared/shared.go:12-14` (`Regime` enum) |
| TaiwanStressCalculator (score-bucketed classifier) | `low` / `alert` / `high` / `crisis` | `internal/narrative/taiwan_stress_index.go:408-413` (switch on score thresholds) |

These measure different things (overall market mood vs. pressure-index severity), so a 1:1 mapping is approximate, not canonical. But consumers who read `/api/regime/history` and `/api/narrative/stress-index/history` without knowing the source experience the regime column as inconsistent.

A separate `/api/scheduler/status` query (Phase A-6 evidence) revealed that of 63 registered scheduled tasks, **36 have never executed** (`last_run == zero`). Most are designed-silent (interval ≥ 24h, container uptime ~7.5h), but three 6h-interval tasks should have fired at least once by now: `auto_geopolitical`, `janus_regime_refresh`, `e2e_chain_probe`. These need investigation.

---

## Invariant Tracker

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance Criteria | Status | Documentation Impact | Notes |
|----|---------|----------------------|-----------------|---------------------|--------|----------------------|-------|
| C01 | `/api/narrative/stress-index/history` rows expose `timestamp` (Unix int64) but not the trading-`date` string from the SQLite PRIMARY KEY; backtest join impossible | `TaiwanStressIndex` struct lacks `Date` field; `stressRowsToIndex` drops `r.Date` | `internal/narrative/taiwan_stress_index.go` (add `Date string \`json:"date"\`` to struct + `internal/monitoring/service/narrative.go` (populate `Date: r.Date` in `stressRowsToIndex`) | Live API returns `"date": "2026-06-29"` for every historical row; test covers happy path + empty historical store | pending | none | Backward-compatible additive change |
| C02 | `regime` column mixes two vocabularies (`RISK_ON/...` vs `low/...`); consumers cannot tell which source | Two writer paths (`DashboardAPI.persistStressIndex` for legacy, `TaiwanStressCalculator.Calculate` for live) emit different strings into the same column without provenance | `internal/narrative/taiwan_stress_index.go::TaiwanStressIndex` (add `Source string \`json:"source"\``); `internal/ledger/historical_store.go::StressRow` already has `Source` — propagate through `stressRowsToIndex` | Live API returns `"source": "macro_ingest"` for live rows; `"source": "synthetic"` for fixture/seed rows; legacy rows tagged `synthetic` | pending | none | Schema already supports it (`stress_index_history.source TEXT`); observed values in production: 89 `synthetic` + 1 `macro_ingest` |
| C03 | No documented mapping between `RISK_ON/RISK_OFF/NEUTRAL/TRANSITIONAL` and `low/alert/high/crisis`; consumers improvise ad-hoc joins | No canonical mapping exists; the two vocabularies were designed independently | New `internal/narrative/regime_mapping.go` with explicit mapping function + new HTTP endpoint `GET /api/narrative/regime-mapping` returning the table | `GetRegimeMapping()` returns `{"RISK_ON":"low", ...}`; tests cover all 4×4 combinations; endpoint reachable via curl | pending | new-doc | Mapping is explicitly approximate (the two systems measure different things); docs must warn users |
| C04 | Of 63 registered scheduled tasks, 36 have `last_run == zero`; user cannot tell which are designed-silent vs broken | Each `BackgroundTaskManager` task has `Enabled bool` and `Interval time.Duration`; tasks with interval > container-uptime correctly have zero `last_run` | Documentation only: add `docs/manifests/2026-07-21-b01-schedule-triage.md` with classification of all 36 silent tasks into "designed-silent (interval ≥ 24h, container age <interval)" vs "investigate (interval < container age, last_run still zero)" | Triage document lists every silent task with its classification + action | pending | new-doc | Read-only investigation; no code changes |
| C05 | `geopolitical_history` has only 1 row from container bootstrap; cannot confirm macro_ingest loop is healthy with N=1 | `cmd/macro-ingest` cron runs daily at 08:00 UTC (16:00 Taipei); `dashboard.IngestAndUpdateMacro` runs every 5 minutes in the atlas-go container | None this PR — operationally needs 3-5 day observation window | Operator runs `sqlite3 data/state/atlas.db "SELECT date, COUNT(*), MAX(captured_at) FROM geopolitical_history GROUP BY date;"` after 3-5 days | pending (operational) | none | Document follow-up; B01 persistGeopolitical wiring is verified to fire on container startup (1 row already proves the path) |

---

## Phase Tracker

### Phase A — Audit (read-only, completed 2026-07-21)

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Reproduce Gap 1: stress row missing `date` | - | done | curl `/api/narrative/stress-index/history?days=90` shows `regime/components/score/timestamp` but no `date` field |
| Identify suspect code for Gap 1 | - | done | `internal/narrative/taiwan_stress_index.go:15-20` defines struct without `Date`; `internal/monitoring/service/narrative.go::stressRowsToIndex` projects `Score/Regime/Timestamp` from `StressRow` and drops `Date` |
| Form root cause hypothesis for Gap 1 | - | done | Struct field never added; project function intentionally drops it |
| Validate Gap 1 hypothesis | - | done | Source line 124: `Timestamp: r.CapturedAt.Unix()` — `r.Date` is in scope but never read |
| Reproduce Gap 2: regime vocabulary mix | - | done | Same curl shows `RISK_OFF` and `low` in adjacent rows |
| Identify suspect code for Gap 2 | - | done | `internal/narrative/taiwan_stress_index.go:408-413` (Calculate writes `low/alert/high/crisis`); `internal/narrative/report_generator.go:51-62` writes `RISK_ON/RISK_OFF/NEUTRAL/TRANSITIONAL`; both end up in `stress_index_history.regime` |
| Form root cause hypothesis for Gap 2 | - | done | Two separate code paths share the same column without provenance; no explicit cross-walk |
| Validate Gap 2 hypothesis | - | done | SQLite query `SELECT date, regime FROM stress_index_history ORDER BY date` shows mixed vocabulary |
| Reproduce Gap 4: silent schedules | - | done | `curl /api/scheduler/status` returns 63 tasks; 36 with `last_run == zero` |
| Identify container uptime | - | done | container `atlas-go` started 2026-07-20T16:46:49Z; uptime ~7.5h at audit time |
| Classify each silent task | - | done | 33 of 36 silent tasks have interval ≥ 24h (designed-silent); 3 have interval = 6h (`auto_geopolitical`, `janus_regime_refresh`, `e2e_chain_probe`) and should have fired |
| Reproduce Gap 3: geopolitical_history row count | - | done | `sqlite3` query returns 1 row dated 2026-07-20 with source `macro_ingest` |
| Identify macro_ingest cadence | - | done | `cmd/atlas/operations_tasks.go:241` registers `macro_ingest` task at 5m interval INSIDE atlas-go container; cron container `atlas-cron-macro-ingest` runs `/app/macro-ingest` daily at 08:00 UTC |

### Phase B — Plan (in progress)

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Map each ID to files + changes | C01, C02, C03, C04, C05 | done | Invariant Tracker above |
| Define acceptance criteria | C01, C02, C03, C04, C05 | done | "Acceptance Criteria" column above |
| Review blast radius (code changes only) | - | done | C01/C02 touch one struct + one projection function (additive, backward-compatible); C03 adds one new file + one new endpoint; C04 is documentation-only; C05 is operational deferral |

### Phase C — Implement (pending)

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Add `Date` field to `TaiwanStressIndex` | C01 | pending | <commit hash> |
| Add `Source` field to `TaiwanStressIndex` | C02 | pending | <commit hash> |
| Populate both fields in `stressRowsToIndex` | C01, C02 | pending | <commit hash> |
| Add `TestStressHistoryRow_DateAndSource` | C01, C02 | pending | <commit hash> |
| Create `internal/narrative/regime_mapping.go` with explicit mapping | C03 | pending | <commit hash> |
| Add `TestRegimeMapping` (table-driven, 4×4 coverage) | C03 | pending | <commit hash> |
| Add `GET /api/narrative/regime-mapping` endpoint | C03 | pending | <commit hash> |
| Author `docs/manifests/2026-07-21-b01-schedule-triage.md` | C04 | pending | <commit hash> |
| Update A05 manifest notes to reference new regime mapping endpoint | C03 | pending | <commit hash> |
| Run focused tests + gofmt + go vet | - | pending | <test output> |
| Commit + push branch + open PR | - | pending | <PR URL> |

### Phase D — Close Out (pending)

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Update manifest status | - | pending | - |
| Rebuild Docker image from worktree | - | pending | <new image digest> |
| Restart `atlas-go` container | - | pending | <docker ps> |
| Re-run 7-check baseline + 3 new C-checks | - | pending | <curl outputs> |
| Final verification report | - | pending | <report> |

---

## Backlog

| ID | Problem | Discovery Time | Proposed Round |
|----|---------|---------------|----------------|
| C06 | Migrate legacy `regime_history` and `stress_index_history` rows from mixed vocabulary to canonical (e.g. project legacy `RISK_ON/OFF/NEUTRAL/TRANSITIONAL` into `low/alert/high/crisis` using the new mapping) | 2026-07-21 | After C03 mapping lands + C05 observation window shows live data accumulates correctly |
| C07 | Add 6h-interval silent task investigation: confirm whether `auto_geopolitical`, `janus_regime_refresh`, `e2e_chain_probe` should fire given container uptime 7.5h | 2026-07-21 | Operational follow-up; possibly a config bug (maturity gate? Enable flag?) |
| C08 | Consider exposing `BackgroundTaskManager.Status()` on a less-privileged endpoint (currently `/api/scheduler/status` exists but no tests cover it) | 2026-07-21 | Future PR — add CI coverage for the silent-tasks triage workflow |

> **Rule**: only move one backlog item into scope per session, and only after all current IDs are done or paused.

---

## Commit Discipline

- Format: `<type>(manifest): #<ID> <short description>`
- One commit per ID (C01/C02/C03/C04 each get their own commit; C05 is operational deferral, no commit)
- No commit without acceptance criteria passing
- PR body must reference this manifest.

---

## Follow-up (Operational, Out of Scope)

| Item | Owner | Action |
|------|-------|--------|
| Observe `geopolitical_history` row accumulation for 3-5 days | operator | Daily check: `sqlite3 /Users/kaecer/workspace/atlas/data/state/atlas.db "SELECT date, COUNT(*), MAX(captured_at) FROM geopolitical_history GROUP BY date ORDER BY date;"` |
| Investigate 3 silent 6h-interval tasks (`auto_geopolitical`, `janus_regime_refresh`, `e2e_chain_probe`) | operator | Check `cmd/atlas/{operations,stage3,calibration}_tasks.go` for maturity-gate or dep-nil guards that may suppress the first tick |
| Confirm macro_ingest 5m interval is actually running (not just registered) | operator | `curl /api/scheduler/status | jq '.[] | select(.name=="macro_ingest")'` should show `last_run` within last 5 minutes |

---

## Session-End State

- **Done this session**: Phase A audit (all 4 gaps reproduced + root causes identified)
- **Remaining**: Phase C implementation (C01/C02/C03/C04) + Phase D close-out
- **Next action**: write C01 struct change + test in worktree
- **Uncommitted code**: no
- **Branch / PR**: `fix/stress-history-date-and-regime-mapping` (worktree `/Users/kaecer/workspace/atlas-stress-regime-2026-07-21`) / not yet opened
- **Paused because**: not paused

---

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-07-21 | 1.0 | Initial manifest with C01/C02/C03/C04/C05 from post-merge verification drill | Sisyphus |