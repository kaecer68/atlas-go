-- Revert: drop unique index, restore non-unique index.

DROP INDEX IF EXISTS idx_export_year_month_unique;

CREATE INDEX IF NOT EXISTS idx_export_year_month ON export_statistics (year, month);
