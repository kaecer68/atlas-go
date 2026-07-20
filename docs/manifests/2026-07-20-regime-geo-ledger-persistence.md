# Manifest: Regime / Geopolitical Ledger Persistence

| Field | Value |
|-------|-------|
| Date | 2026-07-20 |
| Branch | `feat/regime-geo-ledger-persistence` |
| Scope | A04, A05 |
| Trigger | PR #1243 follow-up; user requested A04+A05 in a separate PR |

## A04 — `/api/regime/history` returns 404

### Symptom
`GET /api/regime/history?days=5` returns HTTP 404, while the MCP tool `regime_get_history` succeeds by calling `/api/dashboard/regime-history`.

### Root cause
`/api/regime/history` was never registered in the HTTP mux. The data path already exists at `/api/dashboard/regime-history`.

### Chosen fix
Register `/api/regime/history` as a canonical endpoint backed by the same `PipelineService.LoadRegimeHistory` path, so HTTP and MCP data remain identical.

### Acceptance criteria
- [x] `GET /api/regime/history` (query params: `limit` or `days`, both honoured since 2026-07-21; see manifest `2026-07-21-historical-store-time-and-limit-fixes.md`) returns 200 JSON.
- [x] Response shape matches MCP `regime_get_history` output.
- [x] Limit defaults to 30 and clamps to 365.

## A05 — Geopolitical risk score not persisted historically

### Symptom
Historical stress reconstruction sees geo=0 because only the latest geopolitical score is kept in `data/state/geopolitical/` (file-based `GeopoliticalStore`). There is no historical time series.

### Chosen fix (ledger history)
Add a `geopolitical_history` table to the canonical SQLite ledger and upsert the latest score during every successful macro ingestion. Expose `GET /api/geopolitical/history` for consumers. This aligns with the A01-A03 ledger-first pattern for stress and regime.

### Acceptance criteria
- [x] `geopolitical_history` table exists in SQLite schema.
- [x] `HistoricalStore` exposes `UpsertGeopolitical` and `LoadGeopoliticalHistory`.
- [x] `DashboardAPI.IngestAndUpdateMacro` persists the geo score after `UpdateMacro`.
- [x] `GET /api/geopolitical/history` returns historical geo scores.
- [x] Tests cover happy path, nil store, and upsert error.

## Implementation phases

| Phase | IDs | Work | Verification |
|-------|-----|------|--------------|
| B | A04 | Register `/api/regime/history` route | Handler test + curl 200 |
| C | A05 | Add `geopolitical_history` ledger table + methods; persist in `IngestAndUpdateMacro`; add HTTP endpoint | `go test ./internal/ledger/... ./internal/monitoring/...`, coverage ≥ 60% |
| D | — | Update manifest + create PR | PR open, CI pending |

## Done

- A04: `GET /api/regime/history` registered as canonical alias for `/api/dashboard/regime-history`; `parseLimit` max bumped to 365; `TestHandleRegimeHistory_CanonicalPath` covers both paths.
- A05:
  - Ledger: `GeopoliticalRow`, `UpsertGeopolitical`, `LoadGeopoliticalByDate{,All}`, `LoadGeopoliticalHistory{,All}` on `HistoricalStore`; `geopolitical_history` SQLite table + `idx_geopolitical_history_captured_at`.
  - Persistence: `DashboardAPI.IngestAndUpdateMacro` calls `persistGeopolitical` after `UpdateMacro` (idempotent ON CONFLICT(date)).
  - Read path: `NarrativeService.GetGeopoliticalHistory` reads ledger with days clamping (≤0 → 30, >365 → 365).
  - HTTP: `GET /api/geopolitical/history` via `apinarrative.Handlers.HandleGeopoliticalHistory`.
  - Whitelist: `/api/regime/` + `/api/geopolitical/` added to `cmd/atlas/main.go` `isPublicPath` and `internal/monitoring/api/shared/handler.go` `authFreePrefixPaths`.
  - Tests: `TestDashboardAPI_PersistGeopolitical_{HappyPath,NilStore,ZeroTimestamp,UpsertError}`; `TestSQLiteHistoricalStore_*_Geopolitical`; handler test exists; `GetGeopoliticalHistory` covered via `TestNarrativeService_*`.

## Verification

- `go test ./internal/monitoring/... ./internal/narrative/... ./internal/ledger/...` — all pass.
- `gofmt -l .` — clean.
- Aggregate coverage on changed packages: 74.6% (≥60% threshold).
- `persistGeopolitical` 100%, `HandleGeopoliticalHistory` 100%, `GetGeopoliticalHistory` 100%, `UpsertGeopolitical` 83.3%.

## Next action
Push branch and create PR for A04/A05.

- PR: https://github.com/kaecer68/atlas-go/pull/1244
