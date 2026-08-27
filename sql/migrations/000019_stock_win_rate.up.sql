-- 000019_stock_win_rate.up.sql
-- PR 1b: per-symbol, per-source, per-window win-rate summaries.
CREATE TABLE IF NOT EXISTS stock_win_rate (
    symbol TEXT NOT NULL,
    source TEXT NOT NULL,
    window TEXT NOT NULL,
    observations INTEGER NOT NULL,
    hits INTEGER NOT NULL,
    win_rate REAL NOT NULL,
    wilson_lower REAL,
    wilson_upper REAL,
    confidence REAL,
    calibration_status TEXT NOT NULL,
    net_cost_rate REAL,
    avg_forward_return REAL,
    updated_at TEXT NOT NULL,
    UNIQUE(symbol, source, window)
);

CREATE INDEX IF NOT EXISTS idx_stock_win_rate_key
    ON stock_win_rate(symbol, source, window);
