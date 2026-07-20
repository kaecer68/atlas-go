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
- [ ] `GET /api/regime/history?days=5` returns 200 JSON.
- [ ] Response shape matches MCP `regime_get_history` output.
- [ ] Limit defaults to 30 and clamps to 365.

## A05 — Geopolitical risk score not persisted historically

### Symptom
Historical stress reconstruction sees geo=0 because only the latest geopolitical score is kept in `data/state/geopolitical/` (file-based `GeopoliticalStore`). There is no historical time series.

### Chosen fix (ledger history)
Add a `geopolitical_history` table to the canonical SQLite ledger and upsert the latest score during every successful macro ingestion. Expose `GET /api/geopolitical/history` for consumers. This aligns with the A01-A03 ledger-first pattern for stress and regime.

### Acceptance criteria
- [ ] `geopolitical_history` table exists in SQLite schema.
- [ ] `HistoricalStore` exposes `UpsertGeopolitical` and `LoadGeopoliticalHistory`.
- [ ] `DashboardAPI.IngestAndUpdateMacro` persists the geo score after `UpdateMacro`.
- [ ] `GET /api/geopolitical/history` returns historical geo scores.
- [ ] Tests cover happy path, nil store, and upsert error.

## Implementation phases

| Phase | IDs | Work | Verification |
|-------|-----|------|--------------|
| B | A04 | Register `/api/regime/history` route | Handler test + curl 200 |
| C | A05 | Add `geopolitical_history` ledger table + methods; persist in `IngestAndUpdateMacro`; add HTTP endpoint | `go test ./internal/ledger/... ./internal/monitoring/...`, coverage ≥ 60% |
| D | — | Update manifest + create PR | PR open, CI pending |

## Next action
Run pre-change protocol (blast radius) then implement A04/A05.
