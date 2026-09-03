-- 000020_outcome_market_period.down.sql
-- Phase 2 PR-2a rollback: drop the denormalized period columns added for
-- direct-SQL per-period aggregation. The metadata JSONB column keeps the
-- rich RecommendationOutcome payload (including market_period for rows
-- written after the feature shipped), so reads through the ledger stay
-- correct after rollback; only column-level GROUP BY queries lose access.
DROP INDEX IF EXISTS idx_recommendation_outcomes_market_period;
ALTER TABLE recommendation_outcomes DROP COLUMN IF EXISTS market_period;
ALTER TABLE recommendation_outcomes DROP COLUMN IF EXISTS market_period_source;
