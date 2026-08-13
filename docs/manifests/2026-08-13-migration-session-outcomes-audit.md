# Audit Manifest: SQLite→PG Migration Session-Scoped Outcomes Gap

> **Audit source**: user question "有做過完整的測試與驗證嗎" → post-deploy smoke revealed performance-report trades=0
> **Goal**: Restore session-scoped outcomes (2,965 rows) to PostgreSQL so LoadSessionOutcomes/performance-report match the SQLite-era behavior
> **Scope**: migrate-data outcome migration (session-scoped rows missing); PostgresLedgerStore session_id semantics if needed. NOT in scope: other tables (quotes/screening/trades/experiments verified), report logic (correct, consumes store).
> **Created**: 2026-08-13
> **Status**: in-progress

---

## Invariant Tracker

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance Criteria | Status | Documentation Impact | Notes |
|----|---------|----------------------|-----------------|---------------------|--------|----------------------|-------|
| A01 | performance-report total_trades=0 (SQLite-era 134) | migration -outcomes-sqlite used LoadOutcomes() (global only, session_id=''); 2,965 session-scoped rows never migrated; PG session_id holds date strings (o.Window) not session-XXX-daily | cmd/migrate-data/main.go (migrateOutcomesSQLiteData) | After re-migrate: PG has session-XXX-daily rows; LoadSessionOutcomes(session-20260810-daily)>0; report real_trades>0 | pending | none | Evidence: SQLite `SELECT COUNT(*) FROM outcomes WHERE session_id!=''`=2965; PG `WHERE session_id LIKE 'session-%'`=0; SQLite LoadSessionOutcomes(session-20260810-daily)=45 vs PG=0 |

---

## Phase Tracker

### Phase A — Audit (read-only) — DONE

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Reproduce symptom | A01 | accepted | report trades=0; SQLite-era real_trades=134 (`SELECT COUNT(*) FROM outcomes WHERE session_id!='' AND passed_guards=1 AND is_synthetic=0`) |
| Identify suspect code | A01 | accepted | `migrateOutcomesSQLiteData` calls `store.LoadOutcomes()` (reads `WHERE session_id=''`) — missed `session_id!=''` |
| Root cause hypothesis | A01 | accepted | LoadOutcomes() global-only; session-scoped rows never migrated; PG session_id=o.Window(date) not session-XXX |
| Validate hypothesis | A01 | accepted | SQLite session outcomes 2,965 rows w/ session_id='session-XXX-daily' (window='' mostly); PG 0 rows `LIKE 'session-%'`; JSONL dir has no session-20260810-daily/recommendation_outcomes.jsonl |

### Phase B — Plan (DONE)

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Data analysis (3 sources + PG) | A01 | accepted | scout: PG 7,406 date-format / 43 empty / 0 session-format; SQLite 2,965 session + 3,032 global; 25 PG dates all map to session dirs (comm -23=0) |
| Design: remap PG dates + backfill SQLite | A01 | accepted | Fix = (1) UPDATE PG session_id date→session-YYYYMMDD-daily (safe, 0 orphans); (2) backfill SQLite session+global outcomes via LoadOutcomes() (preserves session_id format) |
| Define acceptance | A01 | pending | PG has session-XXX rows for all summaries; LoadSessionOutcomes(session-20260702-daily)>0; report real_trades>0 |

### Phase C — Implement

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| - | A01 | pending | - |

### Phase D — Close Out

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| - | A01 | pending | - |

---

## Backlog

| ID | Problem | Discovery Time | Proposed Round |
|----|---------|---------------|----------------|
| B01 | PG 7,406 日期 session_id 來源是 root JSONL (45,661 筆, window=日期, session_id=null) 被遷成 session_id=window；但 SQLite global (3,032, window 空) 是不同批次。兩者語義需釐清：root JSONL 是「全球聚合」還是「session 交易日」？ | 2026-08-13 | 需獨立 audit — 影響 migration 正確性，不可猜測 |
| B02 | SQLite global (3,032) vs PG 空 session_id (43) — 現行 migrateOutcomesSQLiteData 用 LoadOutcomes() 只遷到 43，2,989 未遷（NOT EXISTS 去重 key 問題？） | 2026-08-13 | 與 A01 同批修復 |
| B03 | session-20260721-daily 無 JSONL 目錄但 SQLite 有 375 筆 + PG 有 17 筆 — 重映射需確認 ghost session 風險 | 2026-08-13 | 與 A01 同批 |

---

## Commit Discipline

- Format: `<type>(manifest): #<ID> <short description>`
- One commit per ID
- PR body references this manifest

---

## Session-End State

- **Done this session**: Phase A (root cause accepted); Phase B partial (data analysis done, design NOT finalized)
- **Remaining**: Full audit of outcome data-source semantics (B01-B03) before any migration fix — root JSONL vs SQLite global vs session outcomes must be reconciled without guessing
- **Next action**: 移交新 CLI — 完整 handoff 文件見 docs/manifests/2026-08-13-outcome-provenance-audit-handoff.md
- **Uncommitted code**: manifest + handoff doc committed on branch fix/20260813-session-outcomes-migration
- **Branch / PR**: fix/20260813-session-outcomes-migration (manifest + handoff committed; no code change yet)

---

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-08-13 | 1.0 | Initial manifest | agent |
