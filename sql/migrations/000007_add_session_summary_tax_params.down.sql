ALTER TABLE session_summaries
    DROP COLUMN IF EXISTS parameters_version,
    DROP COLUMN IF EXISTS total_tax_paid,
    DROP COLUMN IF EXISTS after_tax_pnl,
    DROP COLUMN IF EXISTS tax_snapshots;
