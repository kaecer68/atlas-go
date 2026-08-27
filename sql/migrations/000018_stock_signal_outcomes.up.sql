-- 000018_stock_signal_outcomes.up.sql
-- PR 1b: per-symbol, per-trigger, per-source signal outcomes.
-- Dual-dialect DDL: PostgreSQL REAL = float4 (precision loss), so float
-- columns use DOUBLE PRECISION; SQLite accepts DOUBLE PRECISION and stores
-- it as REAL (8-byte double). INTEGER booleans are portable.
CREATE TABLE IF NOT EXISTS stock_signal_outcomes (
    symbol TEXT NOT NULL,
    trigger_date TEXT NOT NULL,
    source TEXT NOT NULL,
    forward_return DOUBLE PRECISION,
    net_forward_return DOUBLE PRECISION,
    hit INTEGER,
    cost_rate DOUBLE PRECISION,
    regime TEXT,
    created_at TEXT NOT NULL,
    UNIQUE(symbol, trigger_date, source)
);

CREATE INDEX IF NOT EXISTS idx_stock_signal_outcomes_symbol_date
    ON stock_signal_outcomes(symbol, trigger_date);
CREATE INDEX IF NOT EXISTS idx_stock_signal_outcomes_source_date
    ON stock_signal_outcomes(source, trigger_date);
