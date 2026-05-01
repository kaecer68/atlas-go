-- Remove retention policies
SELECT remove_retention_policy('metrics', if_exists => TRUE);
SELECT remove_retention_policy('recommendation_outcomes', if_exists => TRUE);
SELECT remove_retention_policy('capital_flow', if_exists => TRUE);

-- Remove compression policies
SELECT remove_compression_policy('metrics', if_exists => TRUE);
SELECT remove_compression_policy('recommendation_outcomes', if_exists => TRUE);

-- Drop tables (hypertables are dropped like regular tables)
DROP TABLE IF EXISTS metrics CASCADE;
DROP TABLE IF EXISTS recommendation_outcomes CASCADE;
DROP TABLE IF EXISTS capital_flow CASCADE;
DROP TABLE IF EXISTS alerts CASCADE;
DROP TABLE IF EXISTS export_statistics CASCADE;
DROP TABLE IF EXISTS workspaces CASCADE;
DROP TABLE IF EXISTS users CASCADE;

-- Note: channel_health table is managed by migration 000001
