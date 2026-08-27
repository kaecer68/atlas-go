-- 000019_stock_win_rate.up.sql
-- PR 1b: per-symbol, per-source, per-window win-rate summaries.
-- NOTE: column is rolling_window, not window (window is a PostgreSQL reserved word).
CREATE TABLE IF NOT EXISTS stock_win_rate (
    symbol TEXT NOT NULL,
    source TEXT NOT NULL,
    rolling_window TEXT NOT NULL,
    observations INTEGER NOT NULL,
    hits INTEGER NOT NULL,
    win_rate DOUBLE PRECISION NOT NULL,
    wilson_lower DOUBLE PRECISION,
    wilson_upper DOUBLE PRECISION,
    confidence DOUBLE PRECISION,
    calibration_status TEXT NOT NULL,
    net_cost_rate DOUBLE PRECISION,
    avg_forward_return DOUBLE PRECISION,
    updated_at TEXT NOT NULL,
    UNIQUE(symbol, source, rolling_window)
);

CREATE INDEX IF NOT EXISTS idx_stock_win_rate_key
    ON stock_win_rate(symbol, source, rolling_window);
