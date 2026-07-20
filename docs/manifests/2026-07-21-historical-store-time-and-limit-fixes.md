# Audit Manifest: HistoricalStore Time Format + parseLimit days Alias

> **Audit source**: A04/A05 end-to-end validation on 2026-07-21; runtime evidence against running `atlas-go` container.
> **Goal**: Fix two contract bugs surfaced during A04/A05 acceptance drill, and align A04 manifest text with handler reality.
> **Scope**:
> - IN: `internal/ledger/historical_store.go::parseTimeColumn` (add Go-native time layout)
> - IN: `internal/monitoring/api/pipeline/handlers.go::parseLimit` (accept `days` as alias for `limit`)
> - IN: `docs/manifests/2026-07-20-regime-geo-ledger-persistence.md` (correct `?days=5` → `?limit=5` wording)
> - IN: regression tests for both fixes
> - OUT: any 24-48h observation window for `geopolitical_history` accumulation (operational, documented in §Follow-up)
> - OUT: cleanup of `bin/atlas-mcp.bak-2026-07-21-pre-a04a05` and `.env.bak-2026-07-21-pre-a04a05` (preserved until user confirms rollback is unnecessary)
> **Created**: 2026-07-21
> **Status**: in-progress

---

## Background

On 2026-07-21 we re-built the `atlas-atlas` Docker image at commit `72778605`, set `ATLAS_STORE_BACKEND=sqlite`, and confirmed:

- A01 acceptance: `/api/narrative/stress-index/history?days=90` returns 90 entries ✓
- A04 acceptance: `/api/regime/history?limit=5` returns 200 JSON with `sessions + transitions + current_regime` ✓
- A05 acceptance: `geopolitical_history` table created on first init, `/api/geopolitical/history` returns persisted rows ✓

Two contract bugs surfaced during the same drill:

1. **BUG-1 — `recorded_at` / `captured_at` / `timestamp` JSON values are Go zero-time.**  Despite the SQLite rows storing real timestamps, the API surface renders `"0001-01-01T00:00:00Z"` or `-62135596800`. Root cause: `parseTimeColumn` at `internal/ledger/historical_store.go:418` only matches `time.RFC3339Nano`, `time.RFC3339`, `"2006-01-02T15:04:05.000Z"`, `"2006-01-02T15:04:05Z"`, but the data is stored as `2026-06-29 06:00:00 +0000 UTC` — Go's default `time.Time.String()` format produced by `sql.NullTime` driver.
2. **BUG-2 — `/api/regime/history?days=5` ignores the `days` parameter.**  `parseLimit` at `internal/monitoring/api/pipeline/handlers.go:46` reads only the `limit` query parameter, but the MCP tool at `cmd/atlas-mcp/server/tools_briefing.go:45` calls `/api/regime/history?days=5`.  Result: MCP briefing receives 30 entries regardless of `days`.

A docs drift also exists in the A04 manifest: it writes `?days=5` in the acceptance criteria while the handler has always read `?limit=5`.  The text is misleading for reviewers; we will fix it inline.

---

## Invariant Tracker

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance Criteria | Status | Documentation Impact | Notes |
|----|---------|----------------------|-----------------|---------------------|--------|----------------------|-------|
| B01 | `recorded_at`, `captured_at`, `timestamp` JSON fields render Go zero-time for every historical row | `parseTimeColumn` lacks Go-native `time.Time.String()` layout (`"2006-01-02 15:04:05.999999999 -0700 MST"`); SQLite TEXT columns store that format because `sql.NullTime` driver falls back to default `String()` | `internal/ledger/historical_store.go` (add layout to `timeLayouts` slice) | New unit test passes; live API returns real RFC3339 strings (no `"0001-01-01T00:00:00Z"` or `-62135596800`) | done | none | Pre-existing data + new writes are both affected |
| B02 | `/api/regime/history?days=5` ignored — handler reads `limit` only, defaulting to 30 | `parseLimit` does not consult `days` query param | `internal/monitoring/api/pipeline/handlers.go` (add `days` fallback chain) | New unit test passes; live API `?days=5` returns ≤5 entries (within DB count) | done | none | Also serves any future callers using either name |
| B03 | A04 manifest acceptance text says `?days=5` but handler reads `limit` — reviewer will repeat the same mistake | Documentation drift in `docs/manifests/2026-07-20-regime-geo-ledger-persistence.md` | `docs/manifests/2026-07-20-regime-geo-ledger-persistence.md` (acceptance criteria text) | Manifest text matches handler reality; `grep "?days=" docs/manifests/2026-07-20-*.md` shows no surviving `?days=` references | done | plan-edit | Single-doc edit; not a behaviour change |

---

## Phase Tracker

### Phase A — Audit (read-only, completed 2026-07-21)

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Reproduce BUG-1: API surfaces zero-time | - | done | `curl http://localhost:18080/api/regime/history?limit=5` returns `"recorded_at": "0001-01-01T00:00:00Z"` for all rows |
| Identify suspect code | - | done | `internal/ledger/historical_store.go:418` (`parseTimeColumn` + `timeLayouts`); reader scans `sql.NullString` and routes through `parseTimeColumn` |
| Form root cause hypothesis | - | done | SQLite stores `"2026-06-29 06:00:00 +0000 UTC"`; `parseTimeColumn` doesn't list that layout |
| Validate hypothesis with evidence | - | done | `sqlite3 data/state/atlas.db "SELECT date, quote(captured_at) FROM geopolitical_history"` → `'2026-07-20 16:14:01.734248004 +0000 UTC'` (39 chars, Go-native format) |
| Reproduce BUG-2: `?days=5` ignored | - | done | `curl ?days=5` returns 30 entries; `curl ?limit=5` returns 5 entries |
| Identify suspect code | - | done | `internal/monitoring/api/pipeline/handlers.go:46` (`parseLimit` reads `r.URL.Query().Get("limit")`) |
| Form root cause hypothesis | - | done | Caller at `cmd/atlas-mcp/server/tools_briefing.go:45` passes `?days=5`; handler does not consult it |
| Validate hypothesis with evidence | - | done | `grep -n "parseLimit\|?days\|?limit" internal/monitoring/api/pipeline/handlers.go cmd/atlas-mcp/server/tools_briefing.go` confirms the contract gap |

### Phase B — Plan (in progress)

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Map each ID to files + changes | B01, B02, B03 | done | Invariant Tracker above |
| Define acceptance criteria | B01, B02, B03 | done | "Acceptance Criteria" column above |
| Review blast radius | - | done | Both fixes are in pure-function helpers; blast radius = the three handlers that call `parseTimeColumn` indirectly via `LoadRegimeHistory` / `LoadStressHistory` / `LoadGeopoliticalHistory`, plus `parseLimit` callers (`HandleRegimeHistory`, `HandleSessions`, `HandleMacroRadar`, etc.). d=1 callers in `internal/monitoring/api/pipeline/handlers.go` and `internal/monitoring/api/narrative/handlers.go` are stable — adding layout or alias only widens accepted inputs, does not change accepted outputs. |

### Phase C — Implement (done)

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Add Go-native time layout to `timeLayouts` | B01 | done | commit `cb66a6a2` |
| Add `days` alias to `parseLimit` | B02 | done | commit `8d17a2e8` |
| Add `TestParseTimeColumn_GoNativeFormat` and `TestParseLimit_DaysAlias` | B01, B02 | done | commits `cb66a6a2`, `8d17a2e8` |
| Update A04 manifest text | B03 | done | commit `94f55d8f` |
| Run focused tests + lsp_diagnostics | - | done | 32 packages OK; gofmt clean; go vet clean |
| Commit + push branch + open PR | - | done | branch `fix/historical-store-time-and-limit-params`; PR https://github.com/kaecer68/atlas-go/pull/1245 |

### Phase D — Close Out (done)

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Update manifest status | - | done | this file's Invariant Tracker updated |
| Rebuild Docker image from worktree | - | done | image digest `9a676edb781b` tagged as `atlas-atlas:latest` |
| Restart `atlas-go` container | - | done | container `atlas-go` Up About a minute (healthy), `[HistoricalStore] initialized` log |
| Re-run 3 endpoints + verify zero-time is gone | - | done | see §Verification Report below |
| Re-run `?days=5` and verify ≤5 entries | - | done | see §Verification Report below |
| Final verification report | - | done | see §Verification Report below |

---

## Verification Report (2026-07-21, container atlas-go image digest `9a676edb781b`)

| Check | Result |
|-------|--------|
| `[HistoricalStore] initialized` log appears in docker logs | ✓ PASS |
| `/api/regime/history?limit=5` returns 5 sessions, `recorded_at` is RFC3339 (e.g. `"2026-06-29T06:00:00Z"`) — NOT `"0001-01-01T00:00:00Z"` | ✓ PASS |
| `/api/regime/history?limit=90` returns 90 sessions, 67 transitions, `current_regime: RISK_OFF`, **0 / 90 zero-time** | ✓ PASS |
| `/api/regime/history?days=5` returns exactly 5 sessions (was 30 before B02 fix) | ✓ PASS |
| `/api/regime/history?days=1` returns exactly 1 session | ✓ PASS |
| `/api/narrative/stress-index/history?days=90` returns 90 entries, **0 / 90 zero-epoch timestamps** (was `-62135596800` before B01 fix) | ✓ PASS |
| `/api/geopolitical/history` returns 1 entry with `captured_at: "2026-07-20T16:28:20Z"` (was `"0001-01-01T00:00:00Z"` before B01 fix) | ✓ PASS |
| `ATLAS_STORE_BACKEND=sqlite` confirmed in container env | ✓ PASS |

**Coverage**: aggregate of changed packages (`internal/ledger`, `internal/monitoring/api/pipeline`) ≥ 60% (existing baseline; B01/B02 unit tests add ~50 LoC of coverage).

---

## Backlog

| ID | Problem | Discovery Time | Proposed Round |
|----|---------|---------------|----------------|
| B04 | Schedule a 24h observation window to confirm `geopolitical_history` accumulates one row per macro-ingest tick (currently 1 row at first observation) | 2026-07-21 | Operational follow-up, document in this manifest §Follow-up |
| B05 | Investigate whether legacy regime_history / stress_index_history data (rows dated 2026-04-01 ~ 2026-06-29) should be backfilled to RFC3339Nano format or left in Go-native format | 2026-07-21 | Defer until B01 lands and a fresh `cron-replay-sync` run produces new rows |

> **Rule**: only move one backlog item into scope per session, and only after all current IDs are done or paused.

---

## Commit Discipline

- Format: `<type>(manifest): #<ID> <short description>`
- One commit per ID (B01, B02, B03 will each get their own commit; tests can ride with B01/B02).
- No commit without acceptance criteria passing.
- PR body must reference this manifest.

---

## Follow-up (Operational, Out of Scope)

| Item | Owner | Action |
|------|-------|--------|
| Observe `geopolitical_history` row count after 24h natural macro-ingest cycle | operator | `sqlite3 data/state/atlas.db "SELECT COUNT(*), MAX(captured_at) FROM geopolitical_history;"` |
| Decide whether to delete `bin/atlas-mcp.bak-2026-07-21-pre-a04a05` (24.6 MB) and `~/.config/atlas-go/.env.bak-2026-07-21-pre-a04a05` (5.1 KB) | operator | Keep until 2026-07-28 to allow rollback if BUG-1/BUG-02 surface secondary issues; delete via `rm` after that window |
| Backfill legacy `regime_history` / `stress_index_history` rows to RFC3339Nano for forward consistency | future PR | Decide after observing whether cron replays naturally overwrite rows |

---

## Session-End State

- **Done this session**: B01, B02, B03 implementation + image rebuild (`84d232953789` from post-merge main HEAD `9e47c52a`) + end-to-end re-verification (all 7 checks PASS) + PR #1245 merged + worktree/branch cleanup + 1.1 GB dangling-image reclaim
- **Remaining**: B04 (24h observation window) — operational, documented in §Follow-up
- **Next action**: monitor `geopolitical_history` accumulation over next 24h; delete backup files after 2026-07-28
- **Uncommitted code**: no
- **Branch / PR**: PR https://github.com/kaecer68/atlas-go/pull/1245 MERGED into `main` at `9e47c52a`; local + remote `fix/historical-store-time-and-limit-params` branches deleted; worktree removed
- **Paused because**: not paused

---

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-07-21 | 1.0 | Initial manifest with B01/B02/B03 from A04/A05 drill | Sisyphus |
| 2026-07-21 | 1.1 | Phase C+D closed: commits `cb66a6a2` / `8d17a2e8` / `94f55d8f`; image `9a676edb781b`; PR #1245; verification 7/7 PASS | Sisyphus |
| 2026-07-21 | 1.2 | PR #1245 MERGED into main (`9e47c52a`); image rebuilt `84d232953789` from post-merge main; post-merge re-verification 7/7 PASS; worktree/branch cleanup; dangling-image reclaim 1.1 GB | Sisyphus |