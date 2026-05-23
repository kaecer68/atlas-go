-- Add unique constraint on (year, month) to support ON CONFLICT upsert
-- in postgres_others.go SaveExportStats(). The existing non-unique index is
-- replaced since ON CONFLICT requires a unique/exclusion constraint target.

DROP INDEX IF EXISTS idx_export_year_month;

CREATE UNIQUE INDEX IF NOT EXISTS idx_export_year_month_unique
    ON export_statistics (year, month);
