-- 000030_schema_parity_ledger_columns.up.sql
-- FU-20261012-07 / FU-20261012-09: one shared table must not have two schemas.
--
-- The three shared ledger tables were created by 000003 with the names the Go
-- PostgreSQL stores write (time / agent_id / criterion / factor_scores / …),
-- while the SQLite ledger created the same table names with a legacy shape
-- (timestamp / action / reason / factor_scores_json / total_recs / …). A column
-- added to one side was invisible to the other until a production read failed
-- (2026-10-12 break_even_cost_rate P0: HTTP 503 /api/stock/win_rate).
--
-- This migration is the PG half of the reconciliation: it ADDs the legacy
-- SQLite-only columns so both backends expose exactly the same column set.
-- Nothing is dropped, renamed or retyped, and every column is nullable, so
-- existing rows and existing writers keep working; columns a backend does not
-- write stay NULL. The SQLite half is addSchemaParityColumns in
-- internal/ledger/sqlite_core.go.
--
-- The parity guard (TestSchemaParity_PGMigrationsVsSQLite, run by
-- scripts/ci/check_schema_parity.sh inside make ci-gate) fails if the two
-- sides drift again. Authoritative measurement: docs/specs/schema-parity-spec.md.

ALTER TABLE human_interventions
    ADD COLUMN IF NOT EXISTS symbol TEXT,
    ADD COLUMN IF NOT EXISTS agent_id TEXT,
    ADD COLUMN IF NOT EXISTS action TEXT,
    ADD COLUMN IF NOT EXISTS timestamp TEXT;

ALTER TABLE screening_rejects
    ADD COLUMN IF NOT EXISTS reason TEXT,
    ADD COLUMN IF NOT EXISTS timestamp TEXT,
    ADD COLUMN IF NOT EXISTS factor_scores_json TEXT;

ALTER TABLE session_summaries
    ADD COLUMN IF NOT EXISTS total_recs INTEGER,
    ADD COLUMN IF NOT EXISTS passed_guards INTEGER,
    ADD COLUMN IF NOT EXISTS rejected INTEGER,
    ADD COLUMN IF NOT EXISTS timestamp TEXT;
