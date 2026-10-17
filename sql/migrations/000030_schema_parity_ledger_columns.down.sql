-- 000030_schema_parity_ledger_columns.down.sql
-- Reverses 000030. Safe to run: all 11 columns are nullable and were not
-- written by any PostgreSQL code path before the migration, so dropping them
-- loses no data. The SQLite side (addSchemaParityColumns) is not reversible —
-- dropping columns there would lose the legacy shape the SQLite readers use.
ALTER TABLE session_summaries
    DROP COLUMN IF EXISTS total_recs,
    DROP COLUMN IF EXISTS passed_guards,
    DROP COLUMN IF EXISTS rejected,
    DROP COLUMN IF EXISTS timestamp;

ALTER TABLE screening_rejects
    DROP COLUMN IF EXISTS reason,
    DROP COLUMN IF EXISTS timestamp,
    DROP COLUMN IF EXISTS factor_scores_json;

ALTER TABLE human_interventions
    DROP COLUMN IF EXISTS symbol,
    DROP COLUMN IF EXISTS agent_id,
    DROP COLUMN IF EXISTS action,
    DROP COLUMN IF EXISTS timestamp;
