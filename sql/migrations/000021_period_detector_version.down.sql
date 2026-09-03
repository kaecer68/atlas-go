-- 000021_period_detector_version.down.sql
ALTER TABLE period_history DROP COLUMN IF EXISTS detector_version;
