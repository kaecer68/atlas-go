-- P2 PascalCase summary fix: 補齊 domain.SessionSummary struct 中
-- 4 個 PascalCase 欄位的對應 SQL 欄位:
--   TaxSnapshots       → tax_snapshots JSONB
--   AfterTaxPnL        → after_tax_pnl DOUBLE PRECISION
--   TotalTaxPaid       → total_tax_paid DOUBLE PRECISION
--   ParametersVersion  → parameters_version TEXT
--
-- 修正前: 這 4 個欄位在 struct 中但 SQL table 沒有,
-- SaveSessionSummary/LoadSessionSummary/LoadAllSessionSummaries
-- 皆不處理,導致資料靜默丟失。
ALTER TABLE session_summaries
    ADD COLUMN IF NOT EXISTS tax_snapshots JSONB DEFAULT '[]',
    ADD COLUMN IF NOT EXISTS after_tax_pnl DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS total_tax_paid DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS parameters_version TEXT;
