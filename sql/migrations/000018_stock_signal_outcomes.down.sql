-- 000018_stock_signal_outcomes.down.sql
DROP INDEX IF EXISTS idx_stock_signal_outcomes_symbol_date;
DROP INDEX IF EXISTS idx_stock_signal_outcomes_source_date;
DROP TABLE IF EXISTS stock_signal_outcomes;
