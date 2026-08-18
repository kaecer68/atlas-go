-- 000017_backfill_session_summary_null_taxes.up.sql
-- N2 fix: backfill NULL after_tax_pnl / total_tax_paid in session_summaries.
--
-- Migration 000007 added these columns as nullable DOUBLE PRECISION without
-- backfilling existing rows. pgx scanning NULL into float64 failed, causing
-- LoadAllSessionSummaries to silently drop rows and dual-write to fall back
-- to JSONL (43 summaries observed).
--
-- domain.SessionSummary uses float64 with no tristate; the ledger path
-- (internal/ledger/postgres_ledger.go) already normalizes NULL to 0.
-- Align repository semantics with ledger by backfilling NULL to 0.
UPDATE session_summaries
SET after_tax_pnl = 0,
    total_tax_paid = 0
WHERE after_tax_pnl IS NULL
   OR total_tax_paid IS NULL;
