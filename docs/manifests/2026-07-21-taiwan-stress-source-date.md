# Audit Manifest: Taiwan Stress Index Source/Date Exposure

> **Audit source**: Hermes bridge verification report surfaced that live `/api/taiwan/stress-index` and deprecated `/api/narrative/stress-index/current` do not expose `source` / `date`, while the history endpoint already does. This breaks client-side provenance joining.
> **Goal**: Add `source` and `date` to both live stress-index endpoints so consumers can disambiguate vocabulary origin and join to trading dates.
> **Scope**:
> - IN: `internal/monitoring/api/macro/handlers.go::HandleTaiwanStressIndex`
> - IN: `internal/monitoring/api/narrative/handlers.go::HandleStressIndexCurrent`
> - IN: regression tests for both handlers
> - OUT: `/api/narrative/stress-index/history` (already has `source`/`date` via `stressRowsToIndex`)
> - OUT: `/api/taiwan/stress-index` components/score/regime semantics
> **Created**: 2026-07-21
> **Status**: completed

---

## Verification Report

| Check | Result |
|-------|--------|
| `go test ./internal/monitoring/api/macro/... ./internal/monitoring/api/narrative/...` | PASS |
| `gofmt` / `go vet` | PASS |
| `/api/taiwan/stress-index` returns `date` + `source: "taiwan_calculator"` | PASS |
| `/api/narrative/stress-index/current` returns `date` + `source: "taiwan_calculator"` | PASS |
| `make check-binaries` from worktree (HEAD `f02c694c`) | ALL FRESH |
| `macro_ingest` schedule still fires on 5m cadence after container recreate | PASS |

Example response:

```json
GET /api/taiwan/stress-index
{"score":15.83,"regime":"low","components":{...},"timestamp":1784614946,"date":"2026-07-21","source":"taiwan_calculator"}

GET /api/narrative/stress-index/current
{"components":{...},"date":"2026-07-21","regime":"low","score":13.56,"source":"taiwan_calculator","timestamp":1784614946}
```

---

## Background

The `TaiwanStressIndex` struct already carries `Source` and `Date` fields for rows read from the ledger, but the in-memory `Calculate()` path leaves them empty and the two live handlers do not populate them. Result:

```json
GET /api/taiwan/stress-index
{"score":15.79,"regime":"low","components":{...},"timestamp":1784613384}

GET /api/narrative/stress-index/current
{"components":{...},"regime":"low","score":13.56,"timestamp":1784613384}
```

History rows already look like:

```json
{"score":15.49,"regime":"RISK_ON","components":{...},"timestamp":1784591887,"date":"2026-07-20","source":"macro_ingest"}
```

Adding `source` and `date` to the live endpoints makes the surface uniform and lets clients know whether the regime token is stress vocabulary (`low/alert/high/crisis`) or already normalized regime vocabulary.

---

## Invariant Tracker

| ID | Problem | Root Cause | Files to Change | Acceptance Criteria | Status |
|----|---------|------------|-----------------|---------------------|--------|
| E1 | `/api/taiwan/stress-index` response lacks `source` and `date` | Handler returns `TaiwanStressIndex` directly without populating provenance fields | `internal/monitoring/api/macro/handlers.go` | Response contains `source: "taiwan_calculator"` and `date: "YYYY-MM-DD"` | done |
| E2 | `/api/narrative/stress-index/current` response lacks `source` and `date` | Handler constructs a new map and drops `Source`/`Date` | `internal/monitoring/api/narrative/handlers.go` | Response contains `source: "taiwan_calculator"` and `date: "YYYY-MM-DD"` | done |

---

## Phase Tracker

### Phase A — Audit (done)

| Task | Status | Evidence |
|------|--------|----------|
| Reproduce E1 via curl | done | `/api/taiwan/stress-index` returns no `source`/`date` |
| Reproduce E2 via curl | done | `/api/narrative/stress-index/current` returns no `source`/`date` |
| Confirm history already exposes fields | done | `/api/narrative/stress-index/history` returns `date`/`source` |

### Phase B — Plan (done)

| Task | Status | Evidence |
|------|--------|----------|
| Map changes to files | done | Invariant Tracker above |
| Define acceptance criteria | done | Above |
| Review blast radius | done | Two handlers only; additive JSON fields, backward-compatible |

### Phase C — Implement (done)

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Populate `source`/`date` in `HandleTaiwanStressIndex` | E1 | done | commit `f02c694c` |
| Populate `source`/`date` in `HandleStressIndexCurrent` | E2 | done | commit `f02c694c` |
| Add/update tests | E1, E2 | done | `TestHandleTaiwanStressIndex_SourceAndDate`, `TestHandleStressIndexCurrent` |
| Run focused tests + gofmt + go vet | - | done | 2 packages OK |
| Commit + push | - | done | `f02c694c` |

### Phase D — Close Out (done)

| Task | Status | Evidence |
|------|--------|----------|
| Rebuild Docker image | done | image `atlas-atlas:local` digest `a53cdc43fbf4`, commit `f02c694c` |
| Restart atlas-go container | done | `docker compose up -d --no-build atlas` |
| Verify endpoints | done | curl returns `source`/`date` |
| Update manifest status | done | this file |

---

## Commit Discipline

- Format: `fix(monitoring): #E1/E2 add source and date to live stress index endpoints`
- One commit per handler change, tests ride with the relevant handler.

---

## Session-End State

- **Branch**: `fix/taiwan-stress-source-date`
- **Worktree**: `/Users/kaecer/workspace/atlas/.worktrees/fix-taiwan-stress-source-date`
- **Next action**: implement handler changes
- **Uncommitted code**: no

---

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-07-21 | 1.0 | Initial manifest | Atlas |
