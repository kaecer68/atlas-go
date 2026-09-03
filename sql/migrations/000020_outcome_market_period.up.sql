-- 000020_outcome_market_period.up.sql
-- capital-flow Phase 2 PR-2a: recommendation_outcomes gains the
-- seven-period market classification joined from period_history at
-- outcome-write time.
--
-- market_period        TEXT NULL — bull / plateau / black_swan / …; NULL when
--                      the outcome's trading day has no period_history row
--                      (rows predating the join, or days live ingest missed).
-- market_period_source TEXT NULL — "live" when the period_history row came
--                      from live ingest (is_synthetic=0), "synthetic" for
--                      OHLCV backfill rows (is_synthetic=1), NULL when
--                      market_period is NULL. Per-period performance
--                      matrices exclude "synthetic" rows by default.
--
-- Both columns are nullable and additive: existing INSERT paths that do not
-- set them keep working. Backfill of existing rows is a separate one-off
-- command (cmd/backfill-outcome-period), not part of this migration.
ALTER TABLE recommendation_outcomes ADD COLUMN IF NOT EXISTS market_period TEXT;
ALTER TABLE recommendation_outcomes ADD COLUMN IF NOT EXISTS market_period_source TEXT;

CREATE INDEX IF NOT EXISTS idx_recommendation_outcomes_market_period
    ON recommendation_outcomes (market_period, time DESC);
