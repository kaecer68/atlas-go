-- 000023_channel_health_consecutive_failures.up.sql
-- Issue: 6th ChannelHealthStatusError false-positive round (2026-09-08, k3
-- audit R1). Health status is now DERIVED from the consecutive-failure
-- streak (grace period per channel contract), not mirrored from the last
-- attempt. Sync the streak so the DB mirror keeps the full semantics.
ALTER TABLE channel_health ADD COLUMN IF NOT EXISTS consecutive_failures INTEGER NOT NULL DEFAULT 0;
