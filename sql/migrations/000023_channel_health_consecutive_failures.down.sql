-- 000023_channel_health_consecutive_failures.down.sql
ALTER TABLE channel_health DROP COLUMN IF EXISTS consecutive_failures;
