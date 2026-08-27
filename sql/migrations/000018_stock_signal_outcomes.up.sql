-- 000018_stock_signal_outcomes.up.sql
-- PR 1b: per-symbol, per-trigger, per-source signal outcomes.
-- SQLite-compatible DDL (INTEGER for booleans) so the same file can be
-- exercised in SQLite tests; PostgreSQL treats INTEGER the same way.
CREATE TABLE IF NOT EXISTS stock_signal_outcomes (
    symbol TEXT NOT NULL,
    trigger_date TEXT NOT NULL,
    source TEXT NOT NULL,
    forward_return REAL,
    net_forward_return REAL,
    hit INTEGER,
    cost_rate REAL,
    regime TEXT,
    created_at TEXT NOT NULL,
    UNIQUE(symbol, trigger_date, source)
);

CREATE INDEX IF NOT EXISTS idx_stock_signal_outcomes_symbol_date
    ON stock_signal_outcomes(symbol, trigger_date);
CREATE INDEX IF NOT EXISTS idx_stock_signal_outcomes_source_date
    ON stock_signal_outcomes(source, trigger_date);
